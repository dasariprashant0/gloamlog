// ScenarioCapture.swift - real-window and integration scenarios for quick capture (Jot): the global shortcut, the real panel, a note into an
// open page (with typed text, undo), the day that has only jots, the menu-bar field, the log folder going away, a relaunch after a crash,
// Settings > Shortcuts, and notes that look like markdown. Called from main.swift; use `check(name, ok, detail)`.
//
// What is real: a real AppModel (live: true) over a temporary log folder with its own capture journal, the real WKWebView page editor, real
// NSWindows (MainView, the Jot NSPanel, a hosted MenuBarView, the Settings window), real key events sent with window.sendEvent, the
// accessibility tree for what the screen shows, a private HotKeyCenter registering ⌃⌥⇧⌘F17..F19 through Carbon (never the owner's ⌃⌥J).
// What is not: a physical key press in ANOTHER app (see the report): the press callback is driven through Carbon's own event dispatch.
import AppKit
import Carbon.HIToolbox
import SwiftUI

private let capNow = qaDate(2026, 10, 7, 14, 32)                  // Wed 7 Oct 2026, 14:32: the model's injected clock
private let capToday = "2026-10-07"
private let capYesterday = "2026-10-06"
private let capYesterdayBody = "## What I did\n\nReviewed the pricing page and wrote the summary for the Friday sync with the whole team and sent it round."
private let capAll = UInt32(controlKey | optionKey | shiftKey | cmdKey)
private let capF17 = HotKeySpec(keyCode: 64, carbonModifiers: capAll)    // test combinations only
private let capF18 = HotKeySpec(keyCode: 79, carbonModifiers: capAll)
private let capF19 = HotKeySpec(keyCode: 80, carbonModifiers: capAll)

func runCaptureScenarios() {
    guard QA.wants("capture") else { return }
    print("12. quick capture (shortcut, journal, markdown-looking notes, Jot panel, open page, jots-only day, folder away, Shortcuts pane)")
    let p0 = passes, f0 = failures, s0 = QA.skipTotal
    qaEnableAX()
    try? FileManager.default.createDirectory(at: QA.root, withIntermediateDirectories: true)
    capHotKey()
    capJournal()
    capMarkdownLooking()
    capNoWindow()
    if QA.headless { QA.summary("capture (headless part only)", passes0: p0, failures0: f0, skips0: s0); return }
    capPanel()
    capOpenPage()
    capJotsOnlyDay()
    capFolderAway()
    capShortcuts()
    QA.summary("capture", passes0: p0, failures0: f0, skips0: s0)
}

/// Waits (up to `timeout`) until `cond(file text)` holds; returns the seconds it took, nil when it never did.
private func capWaitFile(_ w: QAWorld, _ day: String = capToday, timeout: TimeInterval = 4, _ cond: (String) -> Bool) -> TimeInterval? {
    let t0 = Date()
    return wait(timeout) { cond(w.fileText(day) ?? "") } ? Date().timeIntervalSince(t0) : nil
}
private func capBody(_ text: String?, day: String = capToday) -> String { text.map { MarkdownFormat.parse(day: day, raw: $0).body } ?? "" }

// MARK: - a. the global shortcut

private func capHotKey() {
    print("   a. the global shortcut: registration, the real Carbon handler, the rules")
    let center = HotKeyCenter(), other = HotKeyCenter()
    var presses = 0, otherPresses = 0
    center.onPressed = { presses += 1 }
    other.onPressed = { otherPresses += 1 }
    check("hotkey: ⌃⌥⇧⌘F17 (a test combination, never the owner's ⌃⌥J) registers", center.register(capF17) == noErr && center.registered == capF17)
    check("hotkey: the same combination twice inside one process answers -9878 (eventHotKeyExistsErr)",
          other.register(capF17) == HotKeyCenter.existsStatus && other.registered == nil)
    check("hotkey: a press sent through Carbon's own dispatch runs the real handler and reaches onPressed on the main queue",
          center.deliverPressForTesting() && wait(2) { presses == 1 }, "presses=\(presses)")
    spin(0.3)
    check("hotkey: ... exactly once, and never for the other center", presses == 1 && otherPresses == 0, "\(presses) \(otherPresses)")
    check("hotkey: a second center holds its own combination (F18)", other.register(capF18) == noErr && other.registered == capF18)
    _ = other.deliverPressForTesting(); spin(0.4)
    check("hotkey: a press for one center never fires the other's callback", otherPresses == 1 && presses == 1, "\(presses) \(otherPresses)")
    check("hotkey: moving the first center to F19 succeeds", center.register(capF19) == noErr && center.registered == capF19)
    let third = HotKeyCenter()
    check("hotkey: the old combination (F17) is released at once, the new one (F19) is held",
          third.register(capF17) == noErr && HotKeyCenter().register(capF19) == HotKeyCenter.existsStatus)
    center.unregister()
    check("hotkey: Off: nothing is registered, a press cannot be delivered, F19 is free again",
          center.registered == nil && !center.deliverPressForTesting() && HotKeyCenter().register(capF19) == noErr)
    spin(0.3)
    check("hotkey: ... and no further press reached the callback", presses == 1, "\(presses)")
    func spec(_ mods: Int) -> HotKeySpec { HotKeySpec(keyCode: 38, carbonModifiers: UInt32(mods)) }
    check("hotkey: option-only, option-shift-only and no modifier are refused; control or command is required",
          !HotKeyCenter.isAllowed(spec(optionKey)) && !HotKeyCenter.isAllowed(spec(optionKey | shiftKey)) && !HotKeyCenter.isAllowed(spec(0))
            && HotKeyCenter.isAllowed(spec(controlKey)) && HotKeyCenter.isAllowed(spec(cmdKey)) && HotKeyCenter.isAllowed(HotKeySpec.defaultJot))
    check("hotkey: the default is control-option-J (key 38, 0x1800) and an unused combination is not a macOS shortcut",
          HotKeySpec.defaultJot.keyCode == 38 && HotKeySpec.defaultJot.carbonModifiers == 0x1800 && !HotKeyCenter.isSystemShortcut(capF17))
    check("hotkey: names are written the way macOS prints them", capF18.display == "⌃⌥⇧⌘F18" && capF18.spoken == "Control Option Shift Command F18", capF18.display)
    third.unregister(); other.unregister()
}

// MARK: - the writer over a real folder (no UI): crash, torn journal, folder away, midnight

private final class CapPage: PageHandle {
    let day: String
    var isLoaded = true
    var received = [CaptureItem]()
    var reply = ""                                           // the whole page as the editor answers once it has taken the note
    init(day: String) { self.day = day }
    func appendJot(_ item: CaptureItem, completion: @escaping (JotAppendResult) -> Void) {
        received.append(item)
        completion(.appended(normalizedMarkdown: reply))     // the page takes it; its own autosave would write the file (it never does here)
    }
}
private final class CapLookup: EditorLookup {
    var pages = [String: PageHandle]()
    func page(for day: String) -> PageHandle? { pages[day] }
}

private struct CapCore {
    let dir: URL, logs: URL, backups: URL, journalDir: URL
    let store: LogStore
    init(_ name: String) {
        dir = QA.root.appendingPathComponent(name)
        try? FileManager.default.removeItem(at: dir)
        logs = dir.appendingPathComponent("logs"); backups = dir.appendingPathComponent("backups"); journalDir = dir.appendingPathComponent("capture")
        for u in [logs, backups] { try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true) }
        store = LogStore(dir: logs, backupDir: backups)
    }
    func writer(pages: EditorLookup = CapLookup(), at now: @escaping () -> Date = { capNow }) -> DayWriter {
        DayWriter(store: store, journal: CaptureJournal(dir: journalDir), pages: pages, prefs: { CapturePrefs() }, clock: now, calendar: { Calendar.current })
    }
    var pending: [CaptureItem] { CaptureJournal(dir: journalDir).pending() }
    func file(_ day: String = capToday) -> String? { qaRead(logs, day) }
}

private func capJournal() {
    print("   g. journal and writer over a real folder: crash, torn journal, folder away, midnight")
    let line = "- 14:32 call with Sam about pricing"
    let expected = "# 2026-10-07\n\n## Jots\n\n\(line)\n"

    // a page took the note but never saved it, then the app died (no ack, no file)
    do {
        let c = CapCore("cap-crash-page")
        let lookup = CapLookup(); let page = CapPage(day: capToday); lookup.pages[capToday] = page
        var w1: DayWriter? = c.writer(pages: lookup)
        let r = w1?.capture(text: "call with Sam about pricing", source: "hotkey")
        check("crash: a note handed to an open page is waiting (1) until that page has saved it", r?.waiting == 1 && page.received.count == 1)
        check("crash: it was durable in the journal before the page ever saw it", c.pending.count == 1 && c.pending[0].text == "call with Sam about pricing")
        w1 = nil                                                              // the app is gone
        check("crash: nothing was written to the log folder", !qaExists(c.logs, capToday))
        let w2 = c.writer()
        check("crash: the next run knows the note is waiting", w2.waiting == 1)
        w2.replayJournal()
        check("crash: replay writes it: exactly one line in the file, the journal is clear", c.file() == expected && w2.waiting == 0 && c.pending.isEmpty, String(describing: c.file()))
        w2.replayJournal()
        c.writer().replayJournal()
        check("crash: a second replay (and a third writer's) adds no duplicate", qaCount(c.file() ?? "", line) == 1 && c.file() == expected)
    }
    // the file was written but the acknowledgement never was (killed between the save and the ack)
    do {
        let c = CapCore("cap-crash-ack")
        let item = CaptureItem(id: UUID().uuidString, ts: capNow, day: capToday, text: "call with Sam about pricing", stamp: "14:32", todo: false, source: "hotkey")
        try? CaptureJournal(dir: c.journalDir).append(item)
        _ = try? c.store.save(day: capToday, body: CapturePlacement.apply(item, to: "", jotsHeading: "Jots"))
        let before = qaBytes(c.logs.appendingPathComponent(capToday + ".md"))
        let w = c.writer()
        check("crash after save: the note still counts as waiting for the new run", w.waiting == 1)
        w.replayJournal()
        check("crash after save: replay finds the line already there: no second copy, the file is byte-identical, the note is acknowledged",
              qaBytes(c.logs.appendingPathComponent(capToday + ".md")) == before && c.pending.isEmpty && w.waiting == 0)
    }
    // the page editor writes a bare link as <https://...>: the note must still be recognised as saved, and never added a second time
    do {
        let link = "https://example.com/pricing for Sam", spelled = "- 14:32 <https://example.com/pricing> for Sam"
        let c = CapCore("cap-spelling")
        let lookup = CapLookup(); let page = CapPage(day: capToday); lookup.pages[capToday] = page
        page.reply = "## Jots\n\n" + spelled
        let w = c.writer(pages: lookup)
        _ = w.capture(text: link, source: "hotkey")
        _ = try? c.store.save(day: capToday, body: "## Jots\n\n" + spelled)                 // what the page's autosave writes
        w.acknowledgeSaved(day: capToday, body: "## Jots\n\n" + spelled)
        check("editor spelling: a note the page editor wrote as <https://...> is acknowledged once the page saved it", w.waiting == 0 && c.pending.isEmpty, "waiting=\(w.waiting) pending=\(c.pending.map { $0.text })")

        let d = CapCore("cap-spelling-2")
        let lookup2 = CapLookup(); let page2 = CapPage(day: capToday); lookup2.pages[capToday] = page2
        page2.reply = "## Jots\n\n" + spelled
        let w2 = d.writer(pages: lookup2)
        _ = w2.capture(text: link, source: "hotkey")
        _ = try? d.store.save(day: capToday, body: "## Jots\n\n" + spelled)                // saved, but the acknowledgement never came
        let before = qaBytes(d.logs.appendingPathComponent(capToday + ".md"))
        lookup2.pages[capToday] = nil                                                          // the page is gone: the next delivery goes through the file
        w2.replayJournal()
        check("editor spelling: delivered through the file after the page saved it, the note is not added a second time",
              qaBytes(d.logs.appendingPathComponent(capToday + ".md")) == before && qaCount(d.file() ?? "", "pricing") == 1 && w2.waiting == 0 && d.pending.isEmpty, String(describing: d.file()))
    }
    // a power cut left half a journal line behind
    do {
        let c = CapCore("cap-torn")
        let w1 = c.writer()
        _ = w1.capture(text: "first note", source: "hotkey")
        qaChmod(c.logs, 0o555)
        _ = w1.capture(text: "second note while the folder is read-only", source: "hotkey")
        qaChmod(c.logs, 0o755)
        if let fh = try? FileHandle(forWritingTo: CaptureJournal(dir: c.journalDir).fileURL) {
            _ = try? fh.seekToEnd(); try? fh.write(contentsOf: Data("{\"v\":1,\"id\":\"torn".utf8)); try? fh.close()
        }
        let w2 = c.writer()
        check("torn journal: the notes before the torn line are still pending, the fragment is ignored", w2.waiting == 1 && c.pending.map { $0.text } == ["second note while the folder is read-only"], "\(c.pending.map { $0.text })")
        _ = w2.capture(text: "third note after the tear", source: "hotkey")
        check("torn journal: a later note never glues onto the fragment: it is delivered with the waiting one, in order",
              w2.waiting == 0 && c.pending.isEmpty && capBody(c.file()).contains("second note while the folder is read-only\n- 14:32 third note after the tear")
                && qaCount(c.file() ?? "", "first note") == 1, String(describing: c.file()))
    }
    // the folder is away: nothing is lost, delivery happens once when it is back
    do {
        let c = CapCore("cap-away")
        let w = c.writer()
        qaChmod(c.logs, 0o555)
        let r = w.capture(text: "call with Sam about pricing", source: "menubar")
        check("folder away: the note is kept (waiting 1), nothing is written, the journal holds it", r?.waiting == 1 && !qaExists(c.logs, capToday) && c.pending.count == 1)
        w.replayJournal()
        check("folder away: replaying while it is still away changes nothing and loses nothing", w.waiting == 1 && !qaExists(c.logs, capToday) && c.pending.count == 1)
        qaChmod(c.logs, 0o755)
        w.replayJournal()
        check("folder back: delivered exactly once", c.file() == expected && w.waiting == 0 && c.pending.isEmpty, String(describing: c.file()))
        w.replayJournal(); c.writer().replayJournal()
        check("folder back: more replays add nothing", qaCount(c.file() ?? "", line) == 1)
    }
    // a note captured at 23:59 belongs to that day, whenever it is delivered
    do {
        let c = CapCore("cap-midnight")
        var now = qaDate(2026, 10, 7, 23, 59)
        let w = c.writer(at: { now })
        qaChmod(c.logs, 0o555)
        _ = w.capture(text: "last thought of the day", source: "hotkey")
        now = qaDate(2026, 10, 8, 0, 4)
        qaChmod(c.logs, 0o755)
        w.replayJournal()
        check("midnight: a note taken at 23:59 and delivered after midnight is on the day it was taken, with its own time",
              capBody(c.file("2026-10-07"), day: "2026-10-07").contains("- 23:59 last thought of the day") && !qaExists(c.logs, "2026-10-08"), String(describing: c.file("2026-10-07")))
    }
}

// MARK: - i. notes that look like markdown never change the page (the file route)

private let capNasty = ["# x", "## Heading two", "- x", "+ x", "* x", "1. x", "12) x", "> quote", "---", "***", "===", "- - -", "```", "~~~x",
                        "<!-- hidden", "see <!-- x", "a\n# b\n- c\n> d\n1. e", "#hashtag", "| a | b |", "  - indented"]

private func capMarkdownLooking() {
    print("   i. notes that look like markdown (file route)")
    let c = CapCore("cap-md")
    let seed = "## What I did\n\nSome real writing under the first heading, enough words to read as a proper page for the day.\n\n## Finished\n\n- Existing item\n\n## To do next\n\n- [ ] existing task"
    _ = try? c.store.save(day: capToday, body: seed)
    let w = c.writer()
    for t in capNasty { _ = w.capture(text: t, source: "menubar") }
    _ = w.capture(text: "a note after the open comment", source: "menubar")
    let body = capBody(c.file())
    let heads = MarkdownBody.sections(of: body).filter { $0.level > 0 }.map { $0.normalizedTitle }
    check("markdown-looking notes: no heading appears but the page's own and Jots", heads == ["what i did", "finished", "to do next", "jots"], "\(heads)")
    check("markdown-looking notes: everything above the Jots block is untouched", body.hasPrefix(seed))
    let n = MarkdownBody.counts(in: body)
    check("markdown-looking notes: each one is exactly one list item (a note after '<!--' still counts: the comment hides nothing)",
          n.jotCount == capNasty.count + 1, "\(n.jotCount) of \(capNasty.count + 1)")
    var other = 0
    for node in MarkdownHTML.nodes(body) {
        switch node {
        case .heading, .paragraph, .item: break
        default: other += 1                                       // a quote, rule, code block or table made by a note
        }
    }
    check("markdown-looking notes: the page's block parser sees no quote, rule, code block or table, and no extra paragraph from them", other == 0 && MarkdownHTML.nodes(body).filter { if case .paragraph = $0 { return true }; return false }.count == 1, "\(other)")
    check("markdown-looking notes: the file round-trips unchanged", MarkdownFormat.serialize(day: capToday, body: body) == c.file())
    w.replayJournal()
    check("markdown-looking notes: all acknowledged, a replay adds nothing", w.waiting == 0 && MarkdownBody.counts(in: capBody(c.file())).jotCount == capNasty.count + 1)
    // The page editor writes a typed "<!--" as "\<!--". That is plain text, not a comment: it must hide nothing of what follows.
    let editorSpelling = "## Jots\n\n- 14:32 see \\<!-- x\n- 14:32 a note after it\n\n## Later\n\nwords after the opener here"
    check("markdown-looking notes: '\\<!--' (how the page editor writes a typed '<!--') opens no comment: later notes, headings and words still count",
          MarkdownBody.counts(in: editorSpelling).jotCount == 2 && MarkdownBody.sections(of: editorSpelling).filter { $0.level > 0 }.count == 2 && MarkdownBody.words(in: editorSpelling) >= 9,
          "\(MarkdownBody.counts(in: editorSpelling)) words=\(MarkdownBody.words(in: editorSpelling))")
    check("markdown-looking notes: a real '<!-- ... -->' comment is still a comment (and an escaped backslash before it does not escape the '<')",
          MarkdownBody.words(in: "keep <!-- hidden words --> shown") == 2 && MarkdownBody.words(in: "keep \\\\<!-- hidden words --> shown") == 2, "\(MarkdownBody.words(in: "keep <!-- hidden words --> shown")) \(MarkdownBody.words(in: "keep \\\\<!-- hidden words --> shown"))")
}

// MARK: - b/i. the model with no window at all, and with its window closed

/// Shared by the two worlds below: a note arrives while the page editor's web view is not in any window.
private func capDetachedChecks(_ w: QAWorld, _ label: String) {
    let m = w.model!
    check("\(label): the editor's web view is not in a window", m.bridge.webView.window == nil)
    let first = "# 2026-10-07\n\n## Jots\n\n- 14:32 call with Sam about pricing\n"
    let r = m.jot(text: "call with Sam about pricing", source: "hotkey")
    check("\(label): a note is accepted at once (receipt)", r != nil && r!.waiting <= 1)
    let t = capWaitFile(w, timeout: 10) { $0.contains("- 14:32 call with Sam about pricing") }
    check("\(label): ... and is in today's file within 2 s", t != nil && t! < 2.0, (t.map { String(format: "%.1fs", $0) } ?? "never: \(String(describing: w.fileText(capToday)))") + " (\(QA.loadNote))")
    check("\(label): ... the file is the title and the Jots block only (a jot never writes the template)", w.fileText(capToday) == first, String(describing: w.fileText(capToday)))
    check("\(label): ... nothing is left waiting", w.settleNotes(5), "waiting=\(m.notesWaiting) pending=\(w.journalPending.count)")
    _ = m.jot(text: "second note", source: "menubar")
    check("\(label): the next note joins the same list", capWaitFile(w, timeout: 4) { $0.contains("- 14:32 call with Sam about pricing\n- 14:32 second note") } != nil, String(describing: w.fileText(capToday)))
    check("\(label): ... and is acknowledged", w.settleNotes(5))
}

private func capNoWindow() {
    print("   b. a note while the window is closed or was never opened (the page editor's web view detached)")
    do {
        let w = QAWorld("cap-nowindow", now: capNow, window: false) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
        defer { w.close() }
        guard w.waitLoaded(capToday) else { check("no window: the page editor of a model that never had a window loads", false, "never loaded"); return }
        capDetachedChecks(w, "no window")
    }
    do {
        let w = QAWorld("cap-closed", now: capNow, window: !QA.headless) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
        defer { w.close() }
        guard w.waitLoaded(capToday) else { check("window closed: the page editor loads", false, "never loaded"); return }
        let typed = "Wrote the first notes of the day before closing the window, standup and the pricing page review."
        check("window closed: text typed in the open page", w.win == nil || w.type(typed))
        w.closeWindow()
        let m = w.model!
        let r = m.jot(text: "note after the window was closed", source: "hotkey")
        check("window closed: a note is accepted", r != nil)
        let t = capWaitFile(w, timeout: 4) { $0.contains("- 14:32 note after the window was closed") && (QA.headless || $0.contains(typed)) }
        check("window closed: the note and the text typed before are both in the file within 2 s", t != nil && t! < 2.0, String(describing: w.fileText(capToday)))
        check("window closed: nothing left waiting", w.settleNotes(5))

        // Notes the page editor writes differently from how they were typed (a link, an address, markup, symbols). Once the page has saved
        // them they are in the file, so each one must be acknowledged; and a later replay through the file must not add one a second time.
        let probe = ["https://example.com/pricing for Sam", "check _this_ and __that__ before sending", "mail bob@example.com about the quote",
                     "see www.example.com today", "ping @alex about the launch", "email a.b+c@sub.example.org please",
                     "query https://example.com/a?x=1&y=2#frag done", "generics List<String> and if a < b then c > d", "tag <b>bold</b> inline html",
                     "path ~/Documents/my_file_v2.txt and C:\\temp\\x", "5 * 3 = 15 and 2 * 4 = 8", "[link text](https://example.com) a markdown link",
                     "`code` and ``double`` ticks", "~~struck~~ text", "tab\tseparated and  double  spaces", "emoji 😀 and 日本語 text",
                     "quote \"double\" and 'single' & &amp; entity", "mid-line - [ ] is not a task", "image ![alt](https://example.com/x.png) inline"]
        func items(_ day: String = capToday) -> Int { MarkdownBody.counts(in: capBody(w.fileText(day), day: day)).jotCount }
        let n0 = items()
        for t in probe { _ = m.jot(text: t, source: "hotkey") }
        let landed = capWaitFile(w, timeout: 6) { MarkdownBody.counts(in: capBody($0)).jotCount >= n0 + probe.count }
        check("window closed: \(probe.count) notes with links, addresses, markup and symbols all reach the file", landed != nil, "items \(n0) -> \(items())\n\(String(describing: w.fileText(capToday)))")
        let acked = w.settleNotes(10)
        check("window closed: ... and every one is acknowledged once the page saved it (nothing stays 'waiting')", acked, "waiting=\(m.notesWaiting) unrecognised=\(w.journalPending.map { $0.text })\nfile=\(String(describing: w.fileText(capToday)))")
        // today's page is no longer the open one: replays now go through the file, which must not add a note a second time
        if w.open(capYesterday) {
            let n1 = items()
            m.replayCapture(); m.tick(); spin(1.2)
            check("window closed: a replay through the file adds no note a second time", items() == n1 && m.notesWaiting == 0, "items \(n1) -> \(items()) waiting=\(m.notesWaiting)\nfile=\(String(describing: w.fileText(capToday)))")
        }
    }
}

// MARK: - b. the real Jot panel

/// Types into the panel's field with key events (the fallback, if this session cannot deliver characters, is the text-input route).
private func capTypePanel(_ panel: JotPanel, _ text: String) -> String {
    guard let tv = JotPanelController.findTextView(in: panel.contentView) else { return "no field" }
    panel.makeFirstResponder(tv)
    qaTypeKeys(panel, text)
    if tv.string == text { return "keys" }
    tv.setSelectedRange(NSRange(location: 0, length: (tv.string as NSString).length))
    tv.insertText(text, replacementRange: tv.selectedRange())
    return tv.string == text ? "insertText" : "failed"
}

private func capPanel() {
    print("   b. the Jot panel (a real NSPanel, real key events), the window closed")
    let w = QAWorld("cap-panel", now: capNow) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
    defer { w.close() }
    guard w.waitLoaded(capToday) else { check("panel: the page editor loads", false, "never loaded"); return }
    w.closeWindow()
    let m = w.model!, jp = m.jotPanel
    m.showJotPanel(source: "hotkey")
    spin(0.6)
    guard let panel = jp.panel, let tv = JotPanelController.findTextView(in: panel.contentView) else { check("panel: the shortcut route shows the panel with its text field", false, "no panel"); return }
    check("panel: it is visible, floating, non-activating, 520 pt wide, over every Space and full-screen app",
          panel.isVisible && panel.isFloatingPanel && panel.styleMask.contains(.nonactivatingPanel) && abs(panel.frame.width - 520) < 1
            && panel.collectionBehavior.contains(.canJoinAllSpaces) && panel.collectionBehavior.contains(.fullScreenAuxiliary) && panel.canBecomeKey && !panel.canBecomeMain)
    check("panel: the text field has the keyboard", panel.firstResponder === tv)
    check("panel: the header names it and shows the time the note will carry", qaAXShows(panel, "Jot to Today") && qaAXShows(panel, "14:32"))
    qaSavePicture(panel, "capture-panel")

    // Esc writes nothing
    let how = capTypePanel(panel, "should not be saved")
    print("     (panel typing used: \(how))")
    check("panel: text typed with key events reaches the field", tv.string == "should not be saved" && jp.model.text == "should not be saved", "\(how) field=\(tv.string.debugDescription) model=\(jp.model.text.debugDescription)")
    qaPressKey(panel, keyCode: QAKeyCode.esc, chars: "\u{1B}")
    let escAt = Date()
    let escClosed = wait(5) { !panel.isVisible }
    let escSecs = Date().timeIntervalSince(escAt)
    check("panel: Esc closes the panel (at once: the fade is 0.08 s) and throws the draft away", escClosed && escSecs < 1.5 && jp.model.text == "",
          "visible=\(panel.isVisible) after \(String(format: "%.2f", escSecs)) s text=\(jp.model.text.debugDescription) (\(QA.loadNote))")
    spin(1.2)
    check("panel: Esc wrote nothing: no file, nothing in the journal, nothing waiting", !w.exists(capToday) && w.journalPending.isEmpty && m.notesWaiting == 0)

    // an empty Return does nothing
    m.showJotPanel(source: "hotkey"); spin(0.5)
    qaPressKey(panel, keyCode: QAKeyCode.ret, chars: "\r")
    spin(0.5)
    check("panel: Return on an empty field does nothing", panel.isVisible && jp.model.phase == .typing && !w.exists(capToday) && w.journalPending.isEmpty)

    // Return writes the note within 2 s
    _ = capTypePanel(panel, "call with Sam about pricing")
    let t0 = Date()
    qaPressKey(panel, keyCode: QAKeyCode.ret, chars: "\r")
    spin(0.2)
    check("panel: after Return the footer reads 'Added to Today'", jp.model.phase == .added && qaAXShows(panel, "Added to Today"), "\(jp.model.phase)")
    let t = capWaitFile(w, timeout: 4) { $0.contains("- 14:32 call with Sam about pricing") }
    check("panel: Return writes '- 14:32 call with Sam about pricing' under '## Jots' in today's file within 2 s",
          t != nil && Date().timeIntervalSince(t0) < 2.0, String(describing: w.fileText(capToday)))
    check("panel: the file is exactly the title and the Jots block", w.fileText(capToday) == "# 2026-10-07\n\n## Jots\n\n- 14:32 call with Sam about pricing\n", String(describing: w.fileText(capToday)))
    check("panel: it closes itself after the confirmation and the field is empty again", wait(2.5) { !panel.isVisible } && jp.model.text == "")
    check("panel: the note is acknowledged", w.settleNotes(5))

    // Shift-Return adds a line to the same note
    m.showJotPanel(source: "hotkey"); spin(0.5)
    _ = capTypePanel(panel, "line one")
    qaPressKey(panel, keyCode: QAKeyCode.ret, chars: "\r", flags: [.shift])
    _ = capTypePanel2(panel, "line two", append: true)
    check("panel: Shift-Return starts a new line instead of adding", jp.model.text == "line one\nline two" && jp.model.phase == .typing, jp.model.text.debugDescription)
    qaPressKey(panel, keyCode: QAKeyCode.ret, chars: "\r")
    let two = capWaitFile(w, timeout: 4) { $0.contains("line one") && $0.contains("line two") }
    let body = capBody(w.fileText(capToday))
    let item = CaptureItem(id: "x", ts: capNow, day: capToday, text: "line one\nline two", stamp: "14:32", todo: false, source: "hotkey")
    check("panel: the two lines are ONE note (one list item, the second line indented under it)",
          two != nil && CapturePlacement.contains(item, in: body, jotsHeading: "Jots") && MarkdownBody.counts(in: body).jotCount == 2, body)
    _ = wait(2.5) { !panel.isVisible }

    // [] makes a to-do, no time stamp
    m.showJotPanel(source: "hotkey"); spin(0.5)
    _ = capTypePanel(panel, "[] buy milk")
    qaPressKey(panel, keyCode: QAKeyCode.ret, chars: "\r")
    check("panel: '[] buy milk' becomes a to-do without a time", capWaitFile(w, timeout: 4) { $0.contains("- [ ] buy milk") } != nil, String(describing: w.fileText(capToday)))
    _ = wait(2.5) { !panel.isVisible }
    check("panel: every note is acknowledged and the page has three notes", w.settleNotes(5) && MarkdownBody.counts(in: capBody(w.fileText(capToday))).jotCount == 3)

    // pressing the shortcut again while the panel is open only brings the field back
    m.showJotPanel(source: "hotkey"); spin(0.4)
    _ = capTypePanel(panel, "a draft")
    m.showJotPanel(source: "hotkey"); spin(0.3)
    check("panel: the shortcut pressed while it is open keeps the draft and the keyboard", panel.isVisible && jp.model.text == "a draft" && panel.firstResponder === tv)
    jp.cancel(); _ = wait(1.5) { !panel.isVisible }
}

private func capTypePanel2(_ panel: JotPanel, _ text: String, append: Bool) -> String {
    guard let tv = JotPanelController.findTextView(in: panel.contentView) else { return "no field" }
    let before = tv.string
    qaTypeKeys(panel, text)
    if tv.string == before + text { return "keys" }
    tv.setSelectedRange(NSRange(location: (before as NSString).length, length: (tv.string as NSString).length - (before as NSString).length))
    tv.insertText(text, replacementRange: tv.selectedRange())
    return tv.string == before + text ? "insertText" : "failed"
}

// MARK: - c. the page is open and has text I just typed; e. the menu-bar field; the editor detached

private func capOpenPage() {
    print("   c/e. a note into an open page with typed text, undo, the menu-bar field, the Review screen, notes that look like markdown")
    let w = QAWorld("cap-open", now: capNow) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
    defer { w.close() }
    guard w.waitLoaded(capToday) else { check("open page: today's page loads", false, "never loaded"); return }
    let m = w.model!
    w.win?.makeKeyAndOrderFront(nil)
    spin(1.0)
    let typed = "Wrote the first notes of the day, covering standup and the pricing page review with the team."
    let typed2 = " A last sentence typed just before the note arrives."
    check("open page: text typed into the real editor", w.type(typed))
    spin(0.35)                                                             // the editor's 300 ms change debounce has just fired; the 600 ms save has not
    check("open page: more text typed, and the note comes straight after it (inside the editor's 300 ms debounce)", w.type(typed2, enter: false))
    let pre = w.editorText() ?? ""
    check("open page: the typed text is in the page and nothing is on disk yet", pre.contains(typed) && pre.contains(typed2) && !w.exists(capToday), pre)
    let r = m.jot(text: "call with Sam about pricing", source: "hotkey")
    check("open page: a note is accepted while the page has unsaved text", r != nil)
    check("open page: the note appears in the open page", wait(3) { (w.editorText() ?? "").contains("- 14:32 call with Sam about pricing") }, w.editorText() ?? "nil")
    let live = w.editorText() ?? ""
    check("open page: the text typed a moment ago is intact, before the new '## Jots' block", live.contains(typed) && live.contains(typed2) && (live.range(of: typed2)?.lowerBound ?? live.endIndex) < (live.range(of: "## Jots")?.lowerBound ?? live.startIndex), live)
    check("open page: Swift adopted the editor's text (the page knows it has the note and the typed text)", m.editor.latest.contains("- 14:32 call with Sam about pricing") && m.editor.latest.contains(typed2))
    let t = capWaitFile(w, timeout: 5) { $0.contains("call with Sam about pricing") && $0.contains(typed2) }
    check("open page: the file ends up with both, each exactly once", t != nil && qaCount(w.fileText(capToday) ?? "", typed) == 1 && qaCount(w.fileText(capToday) ?? "", typed2.trimmingCharacters(in: .whitespaces)) == 1 && qaCount(w.fileText(capToday) ?? "", "call with Sam about pricing") == 1, String(describing: w.fileText(capToday)))
    check("open page: the note is acknowledged once the page saved it (nothing waiting)", w.settleNotes(5), "waiting=\(m.notesWaiting) pending=\(w.journalPending.count)")

    // ⌘Z removes only the note
    check("open page: ⌘Z (the editor's undo) removes the note", w.undo())
    let afterUndo = w.editorText() ?? ""
    check("open page: ... and only the note: the page is exactly what it was before it (typed text intact)", afterUndo == pre, "pre=\(pre.debugDescription)\nafter=\(afterUndo.debugDescription)")
    check("open page: the undo reaches the file by autosave", capWaitFile(w, timeout: 5) { !$0.contains("call with Sam about pricing") && $0.contains(typed2.trimmingCharacters(in: .whitespaces)) } != nil, String(describing: w.fileText(capToday)))
    m.replayCapture(); m.tick(); spin(1.2)
    check("open page: the undone note does not come back (it was acknowledged)", !(w.editorText() ?? "").contains("call with Sam about pricing") && !(w.fileText(capToday) ?? "").contains("call with Sam about pricing") && m.notesWaiting == 0)

    // e. the menu-bar popover's Jot field
    let pop = qaHostWindow(MenuBarView(model: m), width: 300)
    spin(0.8)
    if let tf = qaFindTextField(in: pop.contentView, placeholder: JotCopy.placeholder) ?? qaFindTextField(in: pop.contentView) {
        pop.makeKeyAndOrderFront(nil)
        pop.makeFirstResponder(tf)
        spin(0.3)
        if let ed = pop.firstResponder as? NSTextView {
            ed.insertText("popover note about the quote", replacementRange: NSRange(location: NSNotFound, length: 0))
            spin(0.2)
            qaPressKey(pop, keyCode: QAKeyCode.ret, chars: "\r")
            spin(0.2)
            check("popover: Return in the Jot field shows 'Added' and clears the field", qaAXShows(pop, "Added") && ed.string.isEmpty, "field=\(ed.string.debugDescription)")
            check("popover: the note is in the open page and in the file, like a panel note",
                  wait(3) { (w.editorText() ?? "").contains("- 14:32 popover note about the quote") } && capWaitFile(w, timeout: 5) { $0.contains("- 14:32 popover note about the quote") } != nil, w.editorText() ?? "nil")
        } else { qaFocusCheck("popover: the Jot field takes the keyboard", false, "first responder is \(String(describing: pop.firstResponder))") }
    } else { check("popover: the menu-bar popover has a Jot field", false, "no text field in the hosted view") }
    pop.orderOut(nil)
    check("popover: acknowledged", w.settleNotes(5))

    // the Review screen is showing: the page editor's web view is detached from the window, the note still lands
    m.select(.week)
    check("Review screen: the editor's web view is detached while another screen shows", wait(3) { m.selection == .week && m.bridge.webView.window == nil }, "selection=\(m.selection) window=\(String(describing: m.bridge.webView.window))")
    _ = m.jot(text: "noted while the Review screen was showing", source: "menubar")
    check("Review screen: the note reaches today's file within 2.5 s", capWaitFile(w, timeout: 4) { $0.contains("- 14:32 noted while the Review screen was showing") } != nil, String(describing: w.fileText(capToday)))
    m.openToday()
    check("Review screen: back on Today the page shows it, and the typed text is still there",
          wait(8) { m.editor.loaded } && wait(3) { (w.editorText() ?? "").contains("noted while the Review screen was showing") } && (w.editorText() ?? "").contains(typed))
    check("Review screen: acknowledged", w.settleNotes(5))

    // i. notes that look like markdown, through the editor
    for text in capNasty { _ = m.jot(text: text, source: "menubar") }
    _ = m.jot(text: "a note after the open comment", source: "menubar")
    let expectedItems = 1 + 1 + capNasty.count + 1     // the popover note, the Review-screen note, the markdown-looking ones (the multi-line one is one item) and the closing note
    let settled = w.settleNotes(10)
    check("markdown-looking notes (editor route): all are acknowledged", settled, "waiting=\(m.notesWaiting) pending=\(w.journalPending.map { $0.text })")
    let dom = w.evalString("""
    (function () { var pm = document.querySelector('.ProseMirror'); var q = function (s) { return pm.querySelectorAll(s).length; };
      var jots = Array.prototype.find.call(pm.querySelectorAll('h2'), function (h) { return h.textContent.trim() === 'Jots'; });
      var li = 0; for (var n = jots ? jots.nextElementSibling : null; n && !/^H[1-6]$/.test(n.tagName); n = n.nextElementSibling) li += n.querySelectorAll('li').length;
      return JSON.stringify({h1: q('h1'), h2: q('h2'), h3: q('h3,h4,h5,h6'), quote: q('blockquote'), ol: q('ol'), pre: q('pre'), hr: q('hr'), li: li}); })()
    """)
    check("markdown-looking notes (editor route): no extra heading, quote, numbered list, code block or rule appears in the page",
          dom.contains("\"h1\":0") && dom.contains("\"h2\":6") && dom.contains("\"h3\":0") && dom.contains("\"quote\":0") && dom.contains("\"ol\":0") && dom.contains("\"pre\":0") && dom.contains("\"hr\":0"), dom)
    let file = capBody(w.fileText(capToday))
    let n = MarkdownBody.counts(in: file)
    check("markdown-looking notes (editor route): the file holds each as one list item and the page above is untouched in structure",
          n.jotCount == expectedItems && dom.contains("\"li\":\(expectedItems)") && MarkdownBody.sections(of: file).filter { $0.level > 0 }.count == 6, "jotCount=\(n.jotCount) expected=\(expectedItems) dom=\(dom)\n\(file)")
    check("open page: everything typed at the start is still exactly once in the file", qaCount(w.fileText(capToday) ?? "", typed) == 1)
}

// MARK: - d. a day that has only jots

private func capJotsOnlyDay() {
    print("   d. a day that has only jots")
    let w = QAWorld("cap-jotsonly", now: capNow) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
    defer { w.close() }
    guard w.waitLoaded(capToday) else { check("jots-only: the model loads", false, "never loaded"); return }
    let m = w.model!
    check("jots-only: yesterday's page opens, so today is not the open page", w.open(capYesterday))
    _ = m.jot(text: "first jot", source: "hotkey")
    _ = m.jot(text: "second jot", source: "hotkey")
    let file = "# 2026-10-07\n\n## Jots\n\n- 14:32 first jot\n- 14:32 second jot\n"
    check("jots-only: with today not open the notes go straight into its file", w.fileText(capToday) == file, String(describing: w.fileText(capToday)))
    check("jots-only: nothing is waiting", m.notesWaiting == 0 && w.journalPending.isEmpty)
    check("jots-only: the day reads started (partial), not logged", m.states[capToday] == .partial && m.status(of: capToday) == .partial, "\(String(describing: m.states[capToday]))")
    check("jots-only: the sidebar says 'Today, started, not logged yet'", wait(3) { w.has("Today, started, not logged yet") })
    let url = w.logs.appendingPathComponent(capToday + ".md")
    let bytes0 = qaBytes(url), mtime0 = qaMTime(url)
    check("jots-only: opening it shows the template headings above the jots", w.open(capToday) && m.editor.initialBody == Settings.defaultTemplate + "\n\n## Jots\n\n- 14:32 first jot\n- 14:32 second jot", m.editor.initialBody)
    let heads = w.evalString("JSON.stringify(Array.prototype.map.call(document.querySelectorAll('.ProseMirror h2'), function (h) { return h.textContent.trim(); }))")
    check("jots-only: the page has the five template headings and then Jots", heads == "[\"What I did\",\"Finished\",\"Started\",\"Pending / blocked\",\"To do next\",\"Jots\"]", heads)
    spin(2.5)
    check("jots-only: nothing extra is written to disk until the user types (same bytes, same time)", qaBytes(url) == bytes0 && qaMTime(url) == mtime0 && !m.editor.hasUnsavedEdits)
    check("jots-only: the header reads 0 words and 2 jots, not logged", m.editor.words == 0 && m.editor.jotCount == 2 && !m.editor.isLogged && w.shows("2 jots"), "words=\(m.editor.words) jots=\(m.editor.jotCount)")
    check("jots-only: it still reads started in the sidebar", w.has("Today, started, not logged yet"))
    let wrote = w.evalBool("(function () { var p = document.querySelector('.ProseMirror > h2 + p'); if (!p) return false; var r = document.createRange(); r.selectNodeContents(p); r.collapse(true); var s = window.getSelection(); s.removeAllRanges(); s.addRange(r); return document.execCommand('insertText', false, 'Typed straight under the first heading.'); })()")
    check("jots-only: typing under the first heading", wrote)
    check("jots-only: the template headings reach the file together with the user's own words, and the jots stay where they were",
          capWaitFile(w, timeout: 6) { $0.contains("## What I did\n\nTyped straight under the first heading.\n\n## Finished") && $0.contains("## Jots\n\n- 14:32 first jot\n- 14:32 second jot") } != nil, String(describing: w.fileText(capToday)))
    check("jots-only: exactly one Jots block and one copy of each jot", qaCount(w.fileText(capToday) ?? "", "## Jots") == 1 && qaCount(w.fileText(capToday) ?? "", "first jot") == 1)
    // with the day's page open, a jot still joins the list under Jots
    _ = m.jot(text: "third jot while the page is open", source: "menubar")
    check("jots-only: a third jot with the page open joins the Jots list, the typed text stays", capWaitFile(w, timeout: 5) { $0.contains("- 14:32 second jot\n- 14:32 third jot while the page is open") && $0.contains("Typed straight under the first heading.") } != nil, String(describing: w.fileText(capToday)))
}

// MARK: - f/g. the log folder goes away; a crash and a relaunch

private func capFolderAway() {
    print("   f/g. the log folder goes away, then comes back; a relaunch after a crash")
    let w = QAWorld("cap-folder", now: capNow) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
    defer { qaChmod(w.logs, 0o755); w.close() }
    guard w.waitLoaded(capToday) else { check("folder away: the page editor loads", false, "never loaded"); return }
    let m = w.model!, jp = m.jotPanel
    let typed = "Typed before the folder goes away, so today's page is already on disk and open."
    check("folder away: typing in the open page", w.type(typed))
    check("folder away: the page is saved", wait(5) { w.exists(capToday) })

    // 1. today's page is open
    qaChmod(w.logs, 0o555)
    m.showJotPanel(source: "hotkey"); spin(0.5)
    if let panel = jp.panel {
        _ = capTypePanel(panel, "call with Sam about pricing")
        qaPressKey(panel, keyCode: QAKeyCode.ret, chars: "\r")
        spin(0.4)
        check("folder away: the panel says the note is kept on this Mac", jp.model.phase == .kept && qaAXShows(panel, "Kept on this Mac"), "\(jp.model.phase)")
        qaSavePicture(panel, "capture-panel-kept")
    } else { check("folder away: the panel shows", false, "no panel") }
    check("folder away: the note waits (1), the sidebar says so, the journal holds it", m.notesWaiting == 1 && w.journalPending.count == 1 && wait(2) { w.shows("1 jot waiting") }, "waiting=\(m.notesWaiting)")
    spin(1.5)
    check("folder away: it is in the page but not in the file, and the page knows it could not save", (w.editorText() ?? "").contains("call with Sam about pricing") && !(w.fileText(capToday) ?? "").contains("call with Sam") && m.editor.errorText != nil, String(describing: m.editor.errorText))
    qaChmod(w.logs, 0o755)
    m.retryFolder()
    check("folder back: the note is in the file exactly once and nothing is waiting", wait(6) { qaCount(w.fileText(capToday) ?? "", "- 14:32 call with Sam about pricing") == 1 } && w.settleNotes(6), "waiting=\(m.notesWaiting) file=\(String(describing: w.fileText(capToday)))")
    check("folder back: the typed text is still there once", qaCount(w.fileText(capToday) ?? "", typed) == 1)
    m.replayCapture(); m.reload(); m.tick(); spin(1.2)
    check("folder back: further replays add nothing", qaCount(w.fileText(capToday) ?? "", "call with Sam about pricing") == 1 && qaCount(w.editorText() ?? "", "call with Sam about pricing") == 1)
    check("folder back: the sidebar indicator is gone", !w.shows("jot waiting"))

    // 2. today's page is not open
    check("folder away (page not open): yesterday's page is open", w.open(capYesterday))
    qaChmod(w.logs, 0o555)
    let outcome = m.jotOutcome(text: "second note while away", source: "menubar")
    check("folder away (page not open): the outcome is 'kept on this Mac'", outcome == .kept, "\(outcome)")
    check("folder away (page not open): it waits, the file does not have it", m.notesWaiting == 1 && !(w.fileText(capToday) ?? "").contains("second note while away"))
    qaChmod(w.logs, 0o755)
    m.tick()                                                                 // what the 30 s timer does: a waiting note is retried every time
    check("folder back (page not open): the next tick delivers it exactly once", wait(5) { qaCount(w.fileText(capToday) ?? "", "second note while away") == 1 } && w.settleNotes(5), "waiting=\(m.notesWaiting)")
    m.tick(); m.replayCapture(); spin(0.8)
    check("folder back (page not open): ... and stays once", qaCount(w.fileText(capToday) ?? "", "second note while away") == 1)

    // 3. crash while the folder is away, relaunch once it is back
    qaChmod(w.logs, 0o555)
    _ = m.jotOutcome(text: "kept across a crash", source: "hotkey")
    check("crash: the note is waiting and in the journal", m.notesWaiting == 1 && w.journalPending.map { $0.text } == ["kept across a crash"])
    qaChmod(w.logs, 0o755)
    w.launch(window: true)                                                   // the old run is dropped: no flush, no ack
    let m2 = w.model!
    check("relaunch: the new run knows the waiting note from the journal at once", m2.notesWaiting == 1 && m2.journal.pending().count == 1, "waiting=\(m2.notesWaiting)")
    check("relaunch: it is delivered exactly once, whatever page is open", wait(10) { qaCount(w.fileText(capToday) ?? "", "kept across a crash") == 1 } && w.settleNotes(8), "waiting=\(m2.notesWaiting) file=\(String(describing: w.fileText(capToday)))")
    spin(1.5)
    check("relaunch: and not again later", qaCount(w.fileText(capToday) ?? "", "kept across a crash") == 1 && qaCount(w.fileText(capToday) ?? "", "second note while away") == 1 && qaCount(w.fileText(capToday) ?? "", "call with Sam") == 1)
}

// MARK: - h. Settings > Shortcuts

private func capShortcuts() {
    print("   h. Settings > Shortcuts: record, old one released, taken, Off, Reset")
    let w = QAWorld("cap-shortcuts", now: capNow) { logs in qaPage(logs, capYesterday, capYesterdayBody) }
    defer { w.close() }
    guard w.waitLoaded(capToday) else { check("shortcuts: the model loads", false, "never loaded"); return }
    let m = w.model!
    let center = HotKeyCenter()
    m.settings.capture.hotKey = capF17                          // BEFORE the center is installed: the owner's ⌃⌥J is never registered here
    m.installHotKeys(center)
    check("shortcuts: the model registers the shortcut from Settings through its center", center.registered == capF17 && m.hotKeyFailure == nil)
    check("shortcuts: pressing it opens the Jot panel (hotkey -> panel)", center.deliverPressForTesting() && wait(2) { m.jotPanel.isVisible })
    m.jotPanel.cancel(); _ = wait(1.5) { !m.jotPanel.isVisible }

    m.openSettings(pane: .shortcuts)
    spin(1.0)
    guard let sw = SettingsWindowController.shared.window else { check("shortcuts: the Settings window opens", false); return }
    check("shortcuts: the pane shows the shortcut as macOS prints it, and the switches", qaAXShows(sw, "Jot from anywhere") && qaAXFind(sw, "Jot shortcut")?.value.contains("Control Option Shift Command F17") == true && qaAXShows(sw, "Time stamp"), String(describing: qaAXFind(sw, "Jot shortcut")?.value))
    qaSavePicture(sw, "capture-shortcuts")

    func record(_ spec: HotKeySpec, chars: String) -> Bool {
        guard qaAXPress(sw, "Jot shortcut") else { return false }
        spin(0.3)
        var f: NSEvent.ModifierFlags = []
        if spec.hasControl { f.insert(.control) }; if spec.hasOption { f.insert(.option) }; if spec.hasShift { f.insert(.shift) }; if spec.hasCommand { f.insert(.command) }
        guard let e = qaKeyEvent(sw, chars, keyCode: UInt16(spec.keyCode), flags: f) else { return false }
        NSApp.sendEvent(e)
        spin(0.5)
        return true
    }
    let f18 = String(UnicodeScalar(UInt32(NSF18FunctionKey))!)
    var viaPane = record(capF18, chars: f18) && wait(1.5) { m.settings.capture.hotKey == capF18 }
    if !viaPane {
        qaFocusCheck("shortcuts: recording a new combination through the pane's recorder (⌃⌥⇧⌘F18)", false, "settings=\(String(describing: m.settings.capture.hotKey)) recording button=\(String(describing: qaAXFind(sw, "Jot shortcut")?.value))")
        if m.settings.capture.hotKey != capF18 { _ = m.setJotHotKey(capF18) }       // the same action the recorder calls, so the rest can still be checked
    } else { check("shortcuts: recording a new combination through the pane's recorder (⌃⌥⇧⌘F18) sets it", true); viaPane = true }
    check("shortcuts: it takes effect at once: the center holds F18", center.registered == capF18 && m.hotKeyFailure == nil)
    let probe = HotKeyCenter()
    check("shortcuts: the old combination (F17) stops: it is free for anyone, and F18 is held", probe.register(capF17) == noErr && HotKeyCenter().register(capF18) == HotKeyCenter.existsStatus)
    probe.unregister()
    check("shortcuts: the pane shows the new combination", wait(2) { qaAXFind(sw, "Jot shortcut")?.value.contains("F18") == true }, String(describing: qaAXFind(sw, "Jot shortcut")?.value))
    check("shortcuts: the new one opens the panel", center.deliverPressForTesting() && wait(2) { m.jotPanel.isVisible })
    m.jotPanel.cancel(); _ = wait(1.5) { !m.jotPanel.isVisible }
    check("shortcuts: it is stored with the settings (survives a new model over the same defaults)", Settings.load(from: w.suite).capture.hotKey == capF18)

    // a combination another app (here: another holder in this process) already owns
    let blocker = HotKeyCenter()
    check("shortcuts: setup: something else holds ⌃⌥⇧⌘F19", blocker.register(capF19) == noErr)
    let refused = m.setJotHotKey(capF19)
    check("shortcuts: a combination already taken is refused: 'already used by another app', and the working one stays", refused?.reason == .taken && m.hotKeyFailure?.message.contains("already used by another app") == true && m.settings.capture.hotKey == capF18 && center.registered == capF18, String(describing: refused))
    check("shortcuts: the pane shows that error", wait(2) { qaAXShows(sw, "already used by another app") })
    blocker.unregister()
    // the rules
    let optionOnly = HotKeySpec(keyCode: 38, carbonModifiers: UInt32(optionKey))
    check("shortcuts: option-only is refused with a reason and changes nothing", m.setJotHotKey(optionOnly)?.reason == .notAllowed && m.settings.capture.hotKey == capF18 && center.registered == capF18)
    // a combination macOS itself owns (when this Mac has an enabled one with control or command)
    var sys: HotKeySpec?
    var list: Unmanaged<CFArray>?
    if CopySymbolicHotKeys(&list) == noErr, let items = list?.takeRetainedValue() as? [[String: Any]] {
        let mask = UInt32(controlKey | optionKey | shiftKey | cmdKey)
        for d in items {
            guard let code = d[kHISymbolicHotKeyCode as String] as? Int, let mods = d[kHISymbolicHotKeyModifiers as String] as? Int,
                  (d[kHISymbolicHotKeyEnabled as String] as? Bool) ?? ((d[kHISymbolicHotKeyEnabled as String] as? Int ?? 0) != 0) else { continue }
            let s = HotKeySpec(keyCode: UInt32(code), carbonModifiers: UInt32(truncatingIfNeeded: mods) & mask)
            if s.keyCode < 128 && HotKeyCenter.isAllowed(s) { sys = s; break }
        }
    }
    if let s = sys {
        check("shortcuts: a combination macOS itself uses (\(s.display)) is refused as a system shortcut", m.setJotHotKey(s)?.reason == .system && m.settings.capture.hotKey == capF18 && center.registered == capF18, String(describing: m.hotKeyFailure))
    } else { print("     (no enabled macOS shortcut with control or command on this Mac: the system-shortcut rule was not exercised)") }
    // Esc while recording changes nothing
    if qaAXPress(sw, "Jot shortcut"), let esc = qaKeyEvent(sw, "\u{1B}", keyCode: QAKeyCode.esc) {
        spin(0.2); NSApp.sendEvent(esc); spin(0.4)
        check("shortcuts: Esc while recording cancels and changes nothing", m.settings.capture.hotKey == capF18 && center.registered == capF18 && qaAXFind(sw, "Jot shortcut")?.value.contains("Recording") != true)
    }
    // Off
    m.setJotHotKey(nil)
    check("shortcuts: Off removes it: nothing registered, the setting is off, F18 is free", center.registered == nil && m.settings.capture.hotKey == nil && HotKeyCenter().register(capF18) == noErr && m.hotKeyFailure == nil)
    check("shortcuts: ... and the pane says it is off", wait(2) { qaAXFind(sw, "Jot shortcut")?.value == "Off" || qaAXShows(sw, "Off. Use the Jot field") }, String(describing: qaAXFind(sw, "Jot shortcut")?.value))
    check("shortcuts: with it off a press cannot be delivered", !center.deliverPressForTesting())
    // Reset goes back to control-option-J: with no center installed, so that the owner's real combination is never registered here
    center.unregister()
    m.hotKeys = nil
    check("shortcuts: Reset puts the default back (⌃⌥J) in the settings", qaAXPress(sw, "Reset") && wait(2) { m.settings.capture.hotKey == HotKeySpec.defaultJot }, "hotKey=\(String(describing: m.settings.capture.hotKey))")
    // the switches
    let before = m.settings.capture.timestamps
    qaAXPress(sw, "Time stamp", roles: ["AXCheckBox", "AXSwitch", "AXButton", "AXToggle"])
    check("shortcuts: the Time stamp switch flips the setting", wait(2) { m.settings.capture.timestamps != before }, "timestamps=\(m.settings.capture.timestamps)")
    m.settings.capture.timestamps = true
    sw.orderOut(nil)
}
