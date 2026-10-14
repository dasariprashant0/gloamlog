// ScenarioKit.swift - helpers shared by the QA scenarios (ScenarioCapture, ScenarioSync, ScenarioReview, ScenarioSources, ScenarioReminders).
// NOT shipped. Everything here works on throwaway state: a temporary folder per world (under EC_TMP), a throwaway defaults suite,
// its own capture journal, the model's injected clock, a private HotKeyCenter. Nothing touches ~/Gloamlog, the owner's defaults,
// ~/.claude or real notifications (a bare binary has no notification center).
//
// QAWorld is one app instance: a real AppModel (live: true: timers, folder watcher, real WKWebView editor) over a temporary log
// folder, optionally inside a real NSWindow with the real MainView. Its window can be closed again (the editor's web view is then
// detached, exactly what the app is in when its window is closed or the Review screen is showing).
//
// Environment: EC_ONLY=capture,sync,review,sources,reminders (subset, see main.swift) and EC_HEADLESS=1 (only the scenarios that
// need no visible window: handy while writing checks, takes no focus).
import AppKit
import SwiftUI
import WebKit

func qaDate(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 10, _ min: Int = 0) -> Date {
    Calendar.current.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min))!
}

enum QA {
    static let env = ProcessInfo.processInfo.environment
    static let root = URL(fileURLWithPath: env["EC_TMP"] ?? NSTemporaryDirectory()).appendingPathComponent("dl-editor-check-qa")
    static let headless = env["EC_HEADLESS"] == "1"
    static let evidence = URL(fileURLWithPath: env["QA_EVIDENCE"] ?? (env["EC_TMP"] ?? NSTemporaryDirectory()) + "/dl-qa-shots")
    static var skips: [String: Int] = [:]

    /// True when EC_ONLY is not set, or names `area`.
    static func wants(_ area: String) -> Bool {
        guard let only = env["EC_ONLY"] else { return true }
        return only.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }.contains(area)
    }

    static func skip(_ name: String, _ why: String) {
        skips[why, default: 0] += 1
        print("  skip  \(name)  (\(why))")
    }

    static func summary(_ area: String, passes0: Int, failures0: Int, skips0: Int) {
        let s = skips.values.reduce(0, +) - skips0
        print("   \(area): \(passes - passes0) passed, \(failures - failures0) failed, \(s) skipped")
    }
    static var skipTotal: Int { skips.values.reduce(0, +) }

    /// One-minute load average and core count: a timing check that fails on a busy Mac says so.
    static var loadNote: String {
        var l = [Double](repeating: 0, count: 1)
        getloadavg(&l, 1)
        return String(format: "load %.1f on %d cores", l[0], ProcessInfo.processInfo.activeProcessorCount)
    }

    // MARK: focus
    private static var focusCache: (at: Date, ok: Bool)?
    /// Can this process make a window key right now? A control experiment: when it cannot (screen locked, the owner is typing in
    /// another app), a window of ours that is not key says nothing about our code, so such a check is skipped, never passed.
    static func canTakeFocus() -> Bool {
        if let c = focusCache, Date().timeIntervalSince(c.at) < 20 { return c.ok }
        var ok = false
        let d = (CGSessionCopyCurrentDictionary() as? [String: Any]) ?? [:]
        let locked = (d["CGSSessionScreenIsLocked"] as? Bool) ?? ((d["CGSSessionScreenIsLocked"] as? NSNumber)?.boolValue ?? false)
        if !locked && env["EC_FORCE_LOCKED"] != "1" {
            let p = QAKeyWindow(contentRect: NSRect(x: 8, y: 8, width: 24, height: 24), styleMask: [.borderless], backing: .buffered, defer: false)
            p.isReleasedWhenClosed = false; p.alphaValue = 0.01
            p.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            ok = wait(1.0) { p.isKeyWindow }
            p.orderOut(nil)
        }
        focusCache = (Date(), ok)
        return ok
    }
}

/// A check whose truth needs a key window or a live click: when it fails and this process cannot take focus, it is SKIPPED (never a pass).
func qaFocusCheck(_ name: String, _ ok: Bool, _ detail: @autoclosure () -> String = "") {
    if ok { check(name, true); return }
    if !QA.canTakeFocus() { QA.skip(name, "another app has focus or the screen is locked"); return }
    check(name, false, detail())
}

final class QAKeyWindow: NSWindow { override var canBecomeKey: Bool { true } }

// MARK: - pasteboard (the owner's clipboard is put back after a check that copies)

struct QAPasteboardGuard {
    private let saved: [[(NSPasteboard.PasteboardType, Data)]]
    init() {
        saved = (NSPasteboard.general.pasteboardItems ?? []).map { item in item.types.compactMap { t in item.data(forType: t).map { (t, $0) } } }
    }
    func restore() {
        let pb = NSPasteboard.general
        pb.clearContents()
        let items = saved.map { entries -> NSPasteboardItem in
            let it = NSPasteboardItem()
            for (t, d) in entries { it.setData(d, forType: t) }
            return it
        }
        if !items.isEmpty { pb.writeObjects(items) }
    }
}

// MARK: - key events (window.sendEvent: the same path a keystroke takes once it is in the window)

enum QAKeyCode { static let ret: UInt16 = 36, esc: UInt16 = 53, delete: UInt16 = 51 }

func qaKeyEvent(_ win: NSWindow, _ chars: String, keyCode: UInt16 = 0, flags: NSEvent.ModifierFlags = [], down: Bool = true) -> NSEvent? {
    NSEvent.keyEvent(with: down ? .keyDown : .keyUp, location: .zero, modifierFlags: flags, timestamp: ProcessInfo.processInfo.systemUptime,
                     windowNumber: win.windowNumber, context: nil, characters: chars, charactersIgnoringModifiers: chars.lowercased(),
                     isARepeat: false, keyCode: keyCode)
}
func qaPressKey(_ win: NSWindow, keyCode: UInt16, chars: String, flags: NSEvent.ModifierFlags = []) {
    if let d = qaKeyEvent(win, chars, keyCode: keyCode, flags: flags) { win.sendEvent(d) }
    if let u = qaKeyEvent(win, chars, keyCode: keyCode, flags: flags, down: false) { win.sendEvent(u) }
    spin(0.04)
}
/// Types `text` one key at a time (a new line is the Return key).
func qaTypeKeys(_ win: NSWindow, _ text: String) {
    for ch in text {
        if ch == "\n" { qaPressKey(win, keyCode: QAKeyCode.ret, chars: "\r"); continue }
        let s = String(ch)
        if let d = qaKeyEvent(win, s) { win.sendEvent(d) }
        if let u = qaKeyEvent(win, s, down: false) { win.sendEvent(u) }
    }
    spin(0.05)
}

// MARK: - accessibility tree (SwiftUI exposes its own nodes once enhanced UI is on; AXPress runs the control's action)

func qaEnableAX() {
    NSApp.perform(NSSelectorFromString("accessibilitySetValue:forAttribute:"), with: NSNumber(value: true), with: "AXEnhancedUserInterface" as NSString)
    NSApp.perform(NSSelectorFromString("accessibilitySetValue:forAttribute:"), with: NSNumber(value: true), with: "AXManualAccessibility" as NSString)
}
private func qaAXText(_ o: AnyObject, _ sel: String) -> String {
    guard o.responds(to: NSSelectorFromString(sel)), let r = o.perform(NSSelectorFromString(sel)) else { return "" }
    let v = r.takeUnretainedValue()
    if let s = v as? String { return s }
    if let n = v as? NSNumber { return n.stringValue }
    return ""
}
private func qaAXKids(_ o: AnyObject) -> [AnyObject] {
    guard o.responds(to: NSSelectorFromString("accessibilityChildren")), let r = o.perform(NSSelectorFromString("accessibilityChildren")) else { return [] }
    return (r.takeUnretainedValue() as? [AnyObject]) ?? []
}
struct QAAXNode { let obj: AnyObject; let role: String; let label: String; let value: String }
private func qaAXWalk(_ o: AnyObject, depth: Int = 0, into out: inout [QAAXNode]) {
    guard depth < 40 else { return }
    out.append(QAAXNode(obj: o, role: qaAXText(o, "accessibilityRole"), label: qaAXText(o, "accessibilityLabel"), value: qaAXText(o, "accessibilityValue")))
    for k in qaAXKids(o) { qaAXWalk(k, depth: depth + 1, into: &out) }
}
/// Every accessibility node of the window, its toolbar items, its sheet and its popovers.
func qaAXAll(_ win: NSWindow) -> [QAAXNode] {
    var out = [QAAXNode]()
    var roots: [NSWindow] = [win]
    if let s = win.attachedSheet { roots.append(s) }
    roots += NSApp.windows.filter { $0 !== win && $0.isVisible && ($0.className.contains("Popover") || $0.parent === win) }
    for w in roots { if let v = w.contentView { qaAXWalk(v, into: &out) } }
    for item in win.toolbar?.items ?? [] { if let v = item.view { qaAXWalk(v, into: &out) } }
    return out
}
func qaAXFind(_ win: NSWindow, _ label: String, roles: [String] = ["AXButton"], exact: Bool = true) -> QAAXNode? {
    qaAXAll(win).first { n in (roles.isEmpty || roles.contains(n.role)) && (exact ? n.label == label : n.label.contains(label)) }
}
func qaAXHas(_ win: NSWindow, _ label: String, roles: [String] = ["AXButton"], exact: Bool = true) -> Bool { qaAXFind(win, label, roles: roles, exact: exact) != nil }
/// Any node (a text, a group, a button, a field) whose label or value contains `text`.
func qaAXShows(_ win: NSWindow, _ text: String) -> Bool { qaAXAll(win).contains { $0.label.contains(text) || $0.value.contains(text) } }
@discardableResult
func qaAXPress(_ win: NSWindow, _ label: String, roles: [String] = ["AXButton"], exact: Bool = true) -> Bool {
    guard let n = qaAXFind(win, label, roles: roles, exact: exact), n.obj.responds(to: NSSelectorFromString("accessibilityPerformPress")) else { return false }
    _ = n.obj.perform(NSSelectorFromString("accessibilityPerformPress"))
    spin(0.3)
    return true
}

// MARK: - views

func qaFindTextField(in view: NSView?, placeholder: String? = nil) -> NSTextField? {
    guard let view = view else { return nil }
    if let tf = view as? NSTextField, tf.isEditable, placeholder == nil || tf.placeholderString == placeholder { return tf }
    for s in view.subviews { if let f = qaFindTextField(in: s, placeholder: placeholder) { return f } }
    return nil
}

/// A borderless key-capable window around a SwiftUI view (the menu-bar popover's own shape).
func qaHostWindow<V: View>(_ v: V, width: CGFloat, height: CGFloat? = nil) -> NSWindow {
    let host = NSHostingView(rootView: v)
    host.layoutSubtreeIfNeeded()
    host.frame = NSRect(x: 0, y: 0, width: width, height: height ?? max(host.fittingSize.height, 120))
    let win = QAKeyWindow(contentRect: host.frame, styleMask: [.borderless], backing: .buffered, defer: false)
    win.contentView = host
    win.isReleasedWhenClosed = false
    win.setFrameOrigin(NSPoint(x: 80, y: 120))
    win.makeKeyAndOrderFront(nil)
    return win
}

/// What the window server shows of a window (nil when it drew nothing).
func qaPicture(_ w: NSWindow) -> CGImage? {
    CGWindowListCreateImage(.null, .optionIncludingWindow, CGWindowID(w.windowNumber), [.boundsIgnoreFraming, .bestResolution])
}
func qaSavePicture(_ w: NSWindow, _ name: String) {
    guard let cg = qaPicture(w), let png = NSBitmapImageRep(cgImage: cg).representation(using: .png, properties: [:]) else { return }
    try? FileManager.default.createDirectory(at: QA.evidence, withIntermediateDirectories: true)
    try? png.write(to: QA.evidence.appendingPathComponent(name + ".png"))
}

// MARK: - files

func qaWrite(_ dir: URL, _ day: String, _ text: String) {
    try? text.write(to: dir.appendingPathComponent(day + ".md"), atomically: false, encoding: .utf8)
}
func qaPage(_ dir: URL, _ day: String, _ body: String) { qaWrite(dir, day, MarkdownFormat.serialize(day: day, body: body)) }
func qaRead(_ dir: URL, _ day: String) -> String? { try? String(contentsOf: dir.appendingPathComponent(day + ".md"), encoding: .utf8) }
func qaExists(_ dir: URL, _ day: String) -> Bool { FileManager.default.fileExists(atPath: dir.appendingPathComponent(day + ".md").path) }
func qaBytes(_ url: URL) -> Data? { try? Data(contentsOf: url) }
func qaCount(_ haystack: String, _ needle: String) -> Int {
    guard !needle.isEmpty else { return 0 }
    return haystack.components(separatedBy: needle).count - 1
}
func qaChmod(_ url: URL, _ mode: Int) { try? FileManager.default.setAttributes([.posixPermissions: mode], ofItemAtPath: url.path) }
func qaMTime(_ url: URL) -> Date? { (try? FileManager.default.attributesOfItem(atPath: url.path))?[.modificationDate] as? Date }
func qaNames(_ dir: URL) -> [String] { ((try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? []).sorted() }

// MARK: - one app instance

final class QAClock { var date: Date; init(_ d: Date) { date = d } }

final class QAWorld {
    let name: String
    let dir: URL, logs: URL, backups: URL
    let suiteName: String
    let suite: UserDefaults
    let clock: QAClock
    private let settingsEdit: (inout Settings) -> Void
    private(set) var model: AppModel!
    private(set) var win: NSWindow?
    private(set) var host: NSHostingController<MainView>?

    var cal: Calendar { model.cal }
    var today: String { DayKey.string(clock.date, model.cal) }

    /// `logsPath` lets a world's log folder live under a path that FolderKind reads as iCloud Drive ("Library/Mobile Documents/com~apple~CloudDocs/...").
    /// `fixture` comes first among the closures so that a trailing closure is the fixture: `QAWorld("x", now: d) { logs in ... }`.
    init(_ name: String, now: Date, window: Bool = true, logsPath: String = "logs", size: NSSize = NSSize(width: 1000, height: 760),
         start: Bool = true, fixture: (URL) -> Void = { _ in }, settings: @escaping (inout Settings) -> Void = { _ in }) {
        self.name = name
        dir = QA.root.appendingPathComponent(name)
        logs = dir.appendingPathComponent(logsPath, isDirectory: true)
        backups = dir.appendingPathComponent("backups", isDirectory: true)
        try? FileManager.default.removeItem(at: dir)
        for u in [logs, backups, logs.appendingPathComponent("assets")] { try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true) }
        fixture(logs)
        clock = QAClock(now)
        suiteName = "dailylog.editorcheck.qa.\(name)"
        suite = UserDefaults(suiteName: suiteName)!
        suite.removePersistentDomain(forName: suiteName)
        settingsEdit = settings
        if start { launch(window: window, size: size) }
    }

    /// A fresh AppModel over the same folders and defaults (what a relaunch is). The previous one, if any, is dropped without a flush.
    func launch(window: Bool = true, size: NSSize = NSSize(width: 1000, height: 760)) {
        dropModel()
        suite.set(true, forKey: AppModel.whatsNewKey)            // no "What's new" sheet over the window
        var s = Settings.load(from: suite)
        s.storageFolder = logs; s.onboarded = true
        settingsEdit(&s)
        s.save(to: suite)
        let c = clock
        model = AppModel(defaults: suite, backupDir: backups, live: true, clock: { c.date })
        model.start()
        if window { openWindow(size: size) }
    }

    func openWindow(size: NSSize = NSSize(width: 1000, height: 760)) {
        let h = NSHostingController(rootView: MainView(model: model))
        let w = NSWindow(contentViewController: h)
        w.styleMask = [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView]
        w.toolbarStyle = .unified
        w.title = "Gloamlog"
        w.isReleasedWhenClosed = false
        w.setFrame(NSRect(x: 40, y: 40, width: size.width, height: size.height), display: true)
        w.makeKeyAndOrderFront(nil)
        host = h; win = w
        spin(0.6)
    }

    /// The window goes away; the page editor's web view stays with the model, detached from any window (what the app is in when its
    /// window is closed). The app keeps running.
    func closeWindow() {
        guard let w = win else { return }
        w.orderOut(nil)
        w.contentViewController = nil
        w.close()
        host = nil; win = nil
        spin(0.4)
    }

    /// Leaves without saving anything more (a crash, as far as the folder and the journal can tell).
    private func dropModel() {
        closeWindow()
        model?.timer?.invalidate()
        model = nil
        spin(0.3)
    }

    func close() {
        qaChmod(logs, 0o755)                                      // a scenario that stopped half way must not leave a read-only folder behind
        dropModel()
        suite.removePersistentDomain(forName: suiteName)
        LogStore.jotsRules = (heading: "Jots", countTowardLogged: false)
    }

    // MARK: files
    func exists(_ day: String) -> Bool { qaExists(logs, day) }
    func fileText(_ day: String) -> String? { qaRead(logs, day) }
    func write(_ day: String, _ text: String) { qaWrite(logs, day, text) }
    func page(_ day: String, _ body: String) { qaPage(logs, day, body) }

    // MARK: the editor
    var loaded: Bool { model.editor?.loaded == true }
    @discardableResult func waitLoaded(_ day: String? = nil, timeout: TimeInterval = 25) -> Bool {
        wait(timeout) { self.model.editor.loaded && (day == nil || self.model.editor.day == day) }
    }
    @discardableResult func open(_ day: String) -> Bool {
        model.select(.day(day))
        return wait(12) { self.model.editor.day == day && self.model.editor.loaded }
    }
    func runJS(_ js: String) -> Any? {
        blocking(10) { done in self.model.bridge.webView.evaluateJavaScript(js) { r, _ in done(r) } } ?? nil
    }
    func evalBool(_ js: String) -> Bool { (runJS(js) as? Bool) ?? false }
    func evalInt(_ js: String) -> Int { (runJS(js) as? Int) ?? -1 }
    func evalString(_ js: String) -> String { (runJS(js) as? String) ?? "" }
    /// The page's markdown as the editor holds it right now.
    func editorText() -> String? {
        blocking { (done: @escaping (String?) -> Void) in self.model.bridge.read(token: self.model.editor.token) { done($0) } } ?? nil
    }

    /// Types like a user: the caret goes to the end of the last editable block, under a heading Enter first.
    @discardableResult
    func type(_ t: String, enter: Bool = true) -> Bool {
        let script = """
        (function (t) {
          var v = document.querySelector('.ProseMirror'); v.focus();
          var blocks = v.querySelectorAll('h1,h2,h3,h4,h5,h6,p,li,pre,blockquote');
          var last = blocks.length ? blocks[blocks.length - 1] : v;
          var r = document.createRange(); r.selectNodeContents(last); r.collapse(false);
          var sel = window.getSelection(); sel.removeAllRanges(); sel.addRange(r);
          if (\(enter ? "true" : "false") && /^H[1-6]$/.test(last.tagName)) document.execCommand('insertParagraph');
          if (document.execCommand('insertText', false, t)) return 'exec';
          last.appendChild(document.createTextNode(t)); return 'dom';
        })(\(JS.literal(t)))
        """
        let how = (runJS(script) as? String) ?? "none"
        return how != "none"
    }

    /// ⌘Z in the page: a key event the editor's keymap answers (the way WebKit hands it over), then the native command as a fallback.
    @discardableResult
    func undo() -> Bool {
        let before = editorText()
        _ = runJS("""
        (function () { var pm = document.querySelector('.ProseMirror'); pm.focus();
          return pm.dispatchEvent(new KeyboardEvent('keydown', {key: 'z', code: 'KeyZ', keyCode: 90, which: 90, metaKey: true, bubbles: true, cancelable: true})); })()
        """)
        if wait(1.0, until: { self.editorText() != before }) { return true }
        _ = runJS("document.querySelector('.ProseMirror').focus(); document.execCommand('undo')")
        return wait(1.0) { self.editorText() != before }
    }

    // MARK: capture
    var journalPending: [CaptureItem] { model.journal.pending() }
    /// Waits until every note is in its file (the journal has nothing pending).
    func settleNotes(_ t: TimeInterval = 6) -> Bool { wait(t) { self.model.notesWaiting == 0 && self.journalPending.isEmpty } }

    // MARK: AX
    func shows(_ text: String) -> Bool { win.map { qaAXShows($0, text) } ?? false }
    func has(_ label: String, roles: [String] = ["AXButton"], exact: Bool = true) -> Bool { win.map { qaAXHas($0, label, roles: roles, exact: exact) } ?? false }
    @discardableResult func press(_ label: String, roles: [String] = ["AXButton"], exact: Bool = true) -> Bool {
        win.map { qaAXPress($0, label, roles: roles, exact: exact) } ?? false
    }
}
