// JotView.swift - what the Jot panel shows (M2, task U2; docs/v1/DESIGN_V4.md section 8, UX_FLOWS 3.7):
//
//   Jot to Today                                          14:32      header 28: 13/600, the time the note will carry 12/400
//   call with Sam about pricing                                      field 15/400, 1 to 6 lines (44 to 144)
//   Return adds · ⇧Return new line · Esc cancels                     footer 28: 12/400
//
// The field is an NSTextView, not a SwiftUI TextField: macOS 13's TextField cannot tell Return from Shift-Return, and a note
// may be several lines. Return adds, Shift-Return (or Option-Return) starts a new line, Esc cancels; while an input method has
// text in progress Return belongs to it. States: typing, added ("Added to Today", 600 ms), kept (the folder is unavailable),
// failed (the note is still in the field, Try again).
import SwiftUI
import AppKit

enum JotMetrics {
    static let width: CGFloat = 520                  // DESIGN_V4 section 2: jotW
    static let padding: CGFloat = 16
    static let rowH: CGFloat = 28                    // header and footer
    static let gap: CGFloat = 8
    static let fieldPad: CGFloat = 12                // inside the field, top and bottom
    static let minField: CGFloat = 44                // one line
    static let maxField: CGFloat = 144               // six lines; longer notes scroll inside the field
    static let fontSize: CGFloat = 15
    static let addedHold: TimeInterval = 0.6         // "Added to Today", then the panel closes
    static let keptHold: TimeInterval = 2.6          // a message the user has to read
    static func panelHeight(field: CGFloat) -> CGFloat { padding * 2 + rowH * 2 + gap * 2 + field }
}

/// The three tokens the AppKit text view needs, as dynamic NSColors with the same values as Theme.textPrimary, Theme.textSecondary and
/// Theme.accent (converting a SwiftUI Color with NSColor(_:) can freeze it to the appearance it was made in).
enum JotColors {
    private static func dyn(_ light: UInt32, _ dark: UInt32) -> NSColor {
        NSColor(name: nil) { a in
            let v = a.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua ? dark : light
            return NSColor(srgbRed: CGFloat((v >> 16) & 0xFF) / 255, green: CGFloat((v >> 8) & 0xFF) / 255, blue: CGFloat(v & 0xFF) / 255, alpha: 1)
        }
    }
    static let text = dyn(0x1B201E, 0xE8ECEA)            // Theme.textPrimary
    static let secondary = dyn(0x566059, 0xA3ADA7)       // Theme.textSecondary
    static let accent = dyn(0x0E7A5F, 0x45C29C)          // Theme.accent
}

enum JotCopy {
    static let title = "Jot to Today"
    static let placeholder = "Jot a line for today…"
    static let hint = "Return adds · ⇧Return new line · Esc cancels"
    static let hintSpoken = "Return adds the note. Shift Return starts a new line. Escape cancels."
    static let added = "Added to Today"
    static let addedSpoken = "Added to today's log"
    static let kept = "Kept on this Mac. It will be added when your log folder is back."
    static let keptSpoken = "Saved on this Mac. It will be added when your log folder is back."
    static let failed = "Couldn't save. Your note is still here."
}

final class JotModel: ObservableObject {
    enum Phase: Equatable { case typing, added, kept, failed }
    @Published var text = ""
    @Published var phase = Phase.typing
    @Published var fieldHeight = JotMetrics.minField
    /// The time the note will carry ("14:32"); nil when time stamps are off.
    @Published var stamp: String?
    /// The panel is the key window: the field draws its focus ring.
    @Published var isKey = true

    var isEditable: Bool { phase == .typing || phase == .failed }
}

struct JotView: View {
    @ObservedObject var model: JotModel
    let onSubmit: () -> Void
    let onCancel: () -> Void
    @Environment(\.colorScheme) private var scheme

    var body: some View {
        VStack(alignment: .leading, spacing: JotMetrics.gap) {
            header
            field
            footer
        }
        .padding(JotMetrics.padding)
        .frame(width: JotMetrics.width, height: JotMetrics.panelHeight(field: model.fieldHeight), alignment: .topLeading)
        .background(Theme.rect(Theme.radiusLg).fill(Theme.surface))
        .overlay(Theme.rect(Theme.radiusLg).stroke(scheme == .dark ? Theme.borderStrong : Theme.border, lineWidth: 1))
        .accessibilityElement(children: .contain)
        .accessibilityLabel(JotCopy.title)
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(JotCopy.title).font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary).accessibilityAddTraits(.isHeader)
            Spacer(minLength: Theme.s3)
            if let s = model.stamp {
                Text(s).font(Theme.font(12)).monospacedDigit().foregroundColor(Theme.textSecondary)
                    .accessibilityLabel("Time \(s)")
            }
        }.frame(height: JotMetrics.rowH)
    }

    private var field: some View {
        JotTextField(model: model, onSubmit: onSubmit, onCancel: onCancel)
            .frame(height: model.fieldHeight - 2 * JotMetrics.fieldPad)
            .padding(.horizontal, JotMetrics.fieldPad).padding(.vertical, JotMetrics.fieldPad)
            .background(Theme.rect().fill(Theme.bg))
            .overlay(Theme.rect().stroke(model.isKey && model.isEditable ? Theme.accent : Theme.border, lineWidth: model.isKey && model.isEditable ? 1.5 : 1))
            .opacity(model.phase == .added || model.phase == .kept ? 0.65 : 1)
    }

    @ViewBuilder private var footer: some View {
        HStack(spacing: 6) {
            switch model.phase {
            case .typing:
                Text(JotCopy.hint).font(Theme.font(12)).foregroundColor(Theme.textSecondary)
                    .accessibilityLabel(JotCopy.hintSpoken)
            case .added:
                Image(systemName: "checkmark").font(Theme.font(12, .bold)).accessibilityHidden(true)
                Text(JotCopy.added).font(Theme.font(13, .semibold))
            case .kept:
                Image(systemName: "info.circle").font(Theme.font(13)).foregroundColor(Theme.textSecondary).accessibilityHidden(true)
                Text(JotCopy.kept).font(Theme.font(12)).foregroundColor(Theme.textPrimary).lineLimit(1).minimumScaleFactor(0.9)
            case .failed:
                Image(systemName: "exclamationmark.triangle").font(Theme.font(13)).foregroundColor(Theme.danger).accessibilityHidden(true)
                Text(JotCopy.failed).font(Theme.font(12)).foregroundColor(Theme.danger)
                Spacer(minLength: Theme.s2)
                Button("Try again", action: onSubmit).buttonStyle(TextButtonStyle())
            }
            if model.phase == .typing || model.phase == .added { Spacer(minLength: 0) }
        }
        .foregroundColor(model.phase == .added ? Theme.accentText : Theme.textSecondary)
        .frame(height: JotMetrics.rowH, alignment: .leading)
    }
}

// MARK: - the field

/// Multi-line plain text. Return adds, Shift-Return or Option-Return start a new line, Esc cancels. Return belongs to an input
/// method while it has text in progress. The placeholder is drawn, not typed, so it is never part of the note.
final class JotNSTextView: NSTextView {
    var onSubmit: (() -> Void)?
    var onCancel: (() -> Void)?
    var placeholder = ""

    override var acceptsFirstResponder: Bool { true }

    override func keyDown(with event: NSEvent) {
        let isReturn = event.keyCode == 36 || event.keyCode == 76          // Return, keypad Enter
        if isReturn, !hasMarkedText() {
            let m = event.modifierFlags.intersection(.deviceIndependentFlagsMask).subtracting([.numericPad, .function])
            if m.contains(.shift) || m.contains(.option) { insertNewline(nil); return }
            if m.subtracting(.command).isEmpty { onSubmit?(); return }
        }
        super.keyDown(with: event)
    }

    override func cancelOperation(_ sender: Any?) { onCancel?() }

    override func draw(_ dirtyRect: NSRect) {
        super.draw(dirtyRect)
        guard string.isEmpty, !hasMarkedText(), !placeholder.isEmpty else { return }
        let x = textContainerOrigin.x + (textContainer?.lineFragmentPadding ?? 0)
        (placeholder as NSString).draw(at: NSPoint(x: x, y: textContainerOrigin.y),
                                       withAttributes: [.font: font ?? NSFont.systemFont(ofSize: JotMetrics.fontSize),
                                                        .foregroundColor: JotColors.secondary])
    }
}

struct JotTextField: NSViewRepresentable {
    @ObservedObject var model: JotModel
    let onSubmit: () -> Void
    let onCancel: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeNSView(context: Context) -> NSScrollView {
        let tv = JotNSTextView(frame: NSRect(x: 0, y: 0, width: JotMetrics.width, height: 24))
        let font = NSFont.systemFont(ofSize: JotMetrics.fontSize)
        tv.font = font
        tv.textColor = JotColors.text
        tv.insertionPointColor = JotColors.accent
        tv.typingAttributes = [.font: font, .foregroundColor: JotColors.text]
        tv.isRichText = false; tv.importsGraphics = false; tv.allowsUndo = true
        tv.drawsBackground = false
        tv.isAutomaticQuoteSubstitutionEnabled = false; tv.isAutomaticDashSubstitutionEnabled = false
        tv.isAutomaticTextReplacementEnabled = false; tv.isAutomaticSpellingCorrectionEnabled = false
        tv.isContinuousSpellCheckingEnabled = false
        tv.textContainerInset = .zero
        tv.textContainer?.lineFragmentPadding = 0
        tv.textContainer?.widthTracksTextView = true
        tv.isVerticallyResizable = true; tv.isHorizontallyResizable = false
        tv.autoresizingMask = [.width]
        tv.minSize = NSSize(width: 0, height: 0); tv.maxSize = NSSize(width: CGFloat.greatestFiniteMagnitude, height: CGFloat.greatestFiniteMagnitude)
        tv.placeholder = JotCopy.placeholder
        tv.setAccessibilityLabel(JotCopy.title)
        tv.setAccessibilityHelp(JotCopy.hintSpoken)
        tv.delegate = context.coordinator
        let sv = NSScrollView()
        sv.drawsBackground = false; sv.borderType = .noBorder
        sv.hasVerticalScroller = true; sv.autohidesScrollers = true; sv.scrollerStyle = .overlay; sv.hasHorizontalScroller = false
        sv.documentView = tv
        return sv
    }

    func updateNSView(_ sv: NSScrollView, context: Context) {
        guard let tv = sv.documentView as? JotNSTextView else { return }
        context.coordinator.parent = self
        tv.onSubmit = onSubmit; tv.onCancel = onCancel
        tv.isEditable = model.isEditable
        if tv.string != model.text {                          // reset after a note was added, or a draft came back
            tv.string = model.text
            tv.setSelectedRange(NSRange(location: (tv.string as NSString).length, length: 0))
            DispatchQueue.main.async { context.coordinator.reportHeight(tv) }
        }
        tv.needsDisplay = true
    }

    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: JotTextField
        init(_ p: JotTextField) { parent = p }

        func textDidChange(_ n: Notification) {
            guard let tv = n.object as? JotNSTextView else { return }
            if parent.model.text != tv.string { parent.model.text = tv.string }
            if parent.model.phase == .failed { parent.model.phase = .typing }
            reportHeight(tv)
        }

        /// One line is 44, each further line adds its height, six lines is 144; beyond that the field scrolls.
        func reportHeight(_ tv: NSTextView) {
            guard let lm = tv.layoutManager, let tc = tv.textContainer else { return }
            lm.ensureLayout(for: tc)
            let content = max(lm.usedRect(for: tc).height, lm.defaultLineHeight(for: tv.font ?? NSFont.systemFont(ofSize: JotMetrics.fontSize)))
            let h = min(max(content + 2 * JotMetrics.fieldPad, JotMetrics.minField), JotMetrics.maxField)
            if abs(parent.model.fieldHeight - h) > 0.5 { parent.model.fieldHeight = h }
        }
    }
}
