// JotPanel.swift - the floating Jot panel and the object that runs it (M2, task U2; DESIGN_V4 section 8, ARCHITECTURE 5.D).
//
// The panel is an NSPanel that takes the keyboard WITHOUT activating Gloamlog or raising its window: `.nonactivatingPanel`,
// `canBecomeKey`, `.floating`, and `.canJoinAllSpaces` + `.fullScreenAuxiliary` so it can appear over a full-screen app. Because
// Gloamlog is never made the active app, the previous app stays active and "focus returns to the previous app" is simply true
// when the panel closes (a defensive re-activation covers the case where something activated us anyway).
//
// What the user can lose here: nothing. Return hands the text to AppModel.jot, which makes it durable in the journal before the
// panel says "Added". Esc discards the draft on purpose and writes nothing. Clicking away hides the panel but keeps the draft in
// memory, so a stray click never costs a half-typed note; the next shortcut press brings it back.
import SwiftUI
import AppKit
import Combine

final class JotPanel: NSPanel {
    var onCancel: (() -> Void)?
    var onKeyChange: ((Bool) -> Void)?

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }
    override func cancelOperation(_ sender: Any?) { onCancel?() }
    override func becomeKey() { super.becomeKey(); onKeyChange?(true) }
    override func resignKey() { super.resignKey(); onKeyChange?(false) }
}

/// What AppModel tells the panel after a note was handed over.
enum JotOutcome: Equatable {
    case added          // in today's page, or on its way through the open editor (a normal autosave away)
    case kept           // durable in the journal, but the log folder is unavailable: it is added when the folder returns
    case failed         // nothing was saved: the note is still in the field
}

final class JotPanelController: NSObject {
    let model = JotModel()
    private weak var app: AppModel?
    private(set) var panel: JotPanel?
    private var host: NSHostingView<JotView>?
    private var source = "hotkey"
    private var previousApp: NSRunningApplication?
    private var closeWork: DispatchWorkItem?
    private var sizeSink: AnyCancellable?
    private var showing = false
    private var shownAt = Date.distantPast

    init(model app: AppModel) { self.app = app; super.init() }

    var isVisible: Bool { panel?.isVisible == true }
    var reduceMotion: Bool { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion }

    // MARK: show

    /// Opens the panel on the screen the pointer is on, with the field focused. `source` ("hotkey", "menu", "menubar") is stored
    /// with the note. Pressing the shortcut again while it is open just brings the field back into focus.
    func show(source: String) {
        guard let app = app else { return }
        closeWork?.cancel()
        self.source = source
        if model.phase == .added || model.phase == .kept { model.text = ""; model.fieldHeight = JotMetrics.minField }
        model.phase = .typing                                              // a draft (unsent text) comes back as it was left
        model.stamp = app.settings.capture.timestamps ? JotPanelController.stamp(app.clock(), app.cal) : nil
        let p = panel ?? makePanel()
        if p.isVisible { focusField(); return }
        previousApp = NSWorkspace.shared.frontmostApplication.flatMap { $0.processIdentifier == ProcessInfo.processInfo.processIdentifier ? nil : $0 }
        resize(p)
        position(p)
        present(p)
    }

    /// "14:32": the same 24-hour form the note's own time stamp has.
    static func stamp(_ d: Date, _ cal: Calendar) -> String {
        let c = cal.dateComponents([.hour, .minute], from: d)
        return String(format: "%02d:%02d", c.hour ?? 0, c.minute ?? 0)
    }

    private func makePanel() -> JotPanel {
        let view = JotView(model: model, onSubmit: { [weak self] in self?.submit() }, onCancel: { [weak self] in self?.cancel() })
        let h = NSHostingView(rootView: view)
        h.sizingOptions = []
        let p = JotPanel(contentRect: NSRect(x: 0, y: 0, width: JotMetrics.width, height: JotMetrics.panelHeight(field: model.fieldHeight)),
                         styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: true)
        p.contentView = h
        p.title = JotCopy.title
        p.setAccessibilityLabel(JotCopy.title)
        p.isFloatingPanel = true
        p.level = .floating
        p.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .ignoresCycle]
        p.hidesOnDeactivate = false
        p.isReleasedWhenClosed = false
        p.isOpaque = false; p.backgroundColor = .clear; p.hasShadow = true
        p.animationBehavior = .none
        p.isMovableByWindowBackground = true
        p.onCancel = { [weak self] in self?.cancel() }
        p.onKeyChange = { [weak self] key in
            guard let self = self else { return }
            self.model.isKey = key
            if !key { self.keyWasLost() }
        }
        host = h; panel = p
        h.layoutSubtreeIfNeeded()                                           // the text field exists before it is focused
        sizeSink = model.$fieldHeight.dropFirst().receive(on: DispatchQueue.main).sink { [weak self] _ in
            if let p = self?.panel, p.isVisible { self?.resize(p) }
        }
        return p
    }

    /// The panel's height follows the field (1 to 6 lines); the top edge stays where it is.
    private func resize(_ p: JotPanel) {
        let height = JotMetrics.panelHeight(field: model.fieldHeight)
        guard abs(p.frame.height - height) > 0.5 else { return }
        var f = p.frame
        f.origin.y += f.height - height
        f.size = NSSize(width: JotMetrics.width, height: height)
        p.setFrame(f, display: true, animate: false)
        p.invalidateShadow()
    }

    /// Centred on the screen the pointer is on, the top edge 22% down its visible frame (DESIGN_V4 section 8).
    func position(_ p: JotPanel) {
        let mouse = NSEvent.mouseLocation
        let screen = NSScreen.screens.first { NSMouseInRect(mouse, $0.frame, false) } ?? NSScreen.main ?? NSScreen.screens.first
        let vf = screen?.visibleFrame ?? NSRect(x: 0, y: 0, width: 1440, height: 900)
        let top = vf.maxY - vf.height * 0.22
        p.setFrameOrigin(NSPoint(x: (vf.midX - p.frame.width / 2).rounded(), y: (top - p.frame.height).rounded()))
    }

    private func present(_ p: JotPanel) {
        let final = p.frame
        let moves = !reduceMotion
        if moves { p.setFrameOrigin(NSPoint(x: final.minX, y: final.minY - 6)) }     // rises 6 pt into place (Reduce Motion: fade only)
        p.alphaValue = 0
        p.orderFrontRegardless()
        p.makeKeyAndOrderFront(nil)
        showing = true; shownAt = Date()
        focusField()
        NSAnimationContext.runAnimationGroup { ctx in
            ctx.duration = moves ? 0.12 : 0.1
            ctx.timingFunction = CAMediaTimingFunction(name: .easeOut)
            p.animator().alphaValue = 1
            if moves { p.animator().setFrame(final, display: true) }
        }
    }

    func focusField() {
        guard let p = panel, let tv = JotPanelController.findTextView(in: p.contentView) else { return }
        p.makeKeyAndOrderFront(nil)
        p.makeFirstResponder(tv)
        tv.setSelectedRange(NSRange(location: (tv.string as NSString).length, length: 0))
    }

    static func findTextView(in v: NSView?) -> JotNSTextView? {
        guard let v = v else { return nil }
        if let t = v as? JotNSTextView { return t }
        for s in v.subviews { if let t = findTextView(in: s) { return t } }
        return nil
    }

    // MARK: leave

    /// Esc: nothing is written and the draft is thrown away.
    func cancel() { hide(clearing: true) }

    /// Closes the panel. `clearing` throws the text away (cancelled, or added); otherwise it stays as a draft.
    func hide(clearing: Bool, restoreFocus: Bool = true) {
        closeWork?.cancel()
        if clearing { model.text = ""; model.fieldHeight = JotMetrics.minField; model.phase = .typing }
        guard let p = panel, p.isVisible else { showing = false; return }
        let wasKey = p.isKeyWindow
        showing = false
        let done = { [weak self] in
            p.orderOut(nil)
            p.alphaValue = 1
            if restoreFocus, wasKey { self?.returnFocusToPreviousApp() }
        }
        NSAnimationContext.runAnimationGroup({ ctx in
            ctx.duration = reduceMotion ? 0.1 : 0.08
            ctx.timingFunction = CAMediaTimingFunction(name: .easeIn)
            p.animator().alphaValue = 0
        }, completionHandler: done)
    }

    /// Gloamlog is not activated by the panel, so the previous app is still the active one. Only when something did activate us
    /// is the previous app brought back.
    private func returnFocusToPreviousApp() {
        defer { previousApp = nil }
        guard NSApp.isActive, let prev = previousApp, !prev.isTerminated else { return }
        prev.activate(options: [])
    }

    /// The panel lost the keyboard (a click elsewhere): a note being typed stays as a draft, the panel steps out of the way.
    private func keyWasLost() {
        // The system may shuffle key status while the panel is coming up: a loss in the first moments is not the user clicking away.
        guard showing, Date().timeIntervalSince(shownAt) > 0.4, model.phase == .typing || model.phase == .failed else { return }
        hide(clearing: false, restoreFocus: false)
    }

    // MARK: submit

    /// Return (or Try again).
    func submit() {
        guard let app = app, model.phase == .typing || model.phase == .failed else { return }
        let raw = model.text
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty || trimmed == "[]" { return }                      // an empty Return does nothing
        switch app.jotOutcome(text: raw, source: source) {
        case .added:
            model.phase = .added
            if app.live { UIAnnounce.say(JotCopy.addedSpoken) }
            closeLater(after: JotMetrics.addedHold)
        case .kept:
            model.phase = .kept
            if app.live { UIAnnounce.say(JotCopy.keptSpoken) }
            closeLater(after: JotMetrics.keptHold)
        case .failed:
            model.phase = .failed
            if app.live { UIAnnounce.say(JotCopy.failed, assertive: true) }
        }
    }

    private func closeLater(after: TimeInterval) {
        closeWork?.cancel()
        let w = DispatchWorkItem { [weak self] in self?.hide(clearing: true) }
        closeWork = w
        DispatchQueue.main.asyncAfter(deadline: .now() + after, execute: w)
    }
}
