// ScenarioNav.swift - real-window scenarios for navigation, calendar, Go to date, Catch up and Week review.
// Owned by the navigation stream. Called from main.swift after the app scenarios; use `check(name, ok, detail)`.
//
// What is real here: the real MainView in a real NSWindow (NSHostingController, unified toolbar, full-size sidebar), the real
// WKWebView editor, a real LogStore on a temporary folder. Buttons are pressed through the accessibility tree
// (AXPress on SwiftUI's own nodes, the same path VoiceOver takes), text goes into the real editor with execCommand, and
// screenshots are the window as the window server composites it (CGWindowListCreateImage), titlebar and toolbar included.
// What is not: nothing here moves a physical mouse or presses a key; menu key equivalents (⌘[ ⌘] ⌘↩) are not exercised.
import AppKit
import SwiftUI
import WebKit

// MARK: - helpers

private let navRoot = URL(fileURLWithPath: ProcessInfo.processInfo.environment["EC_TMP"] ?? NSTemporaryDirectory()).appendingPathComponent("dl-editor-check-nav")
private let navShotsDir = URL(fileURLWithPath: ProcessInfo.processInfo.environment["NAV_EVIDENCE"]
                              ?? (ProcessInfo.processInfo.environment["EC_TMP"] ?? NSTemporaryDirectory()) + "/dl-nav-shots")

private final class ClockBox { var date: Date; init(_ d: Date) { date = d } }

private func navDate(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 10, _ min: Int = 0) -> Date {
    Calendar.current.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min))!
}

/// One app instance: model + real window + a temporary log folder.
private final class NavWorld {
    let dir: URL, logs: URL, backups: URL
    let clock: ClockBox
    let suite: UserDefaults
    let model: AppModel
    let win: NSWindow
    var cal: Calendar { model.cal }

    init(_ name: String, now: Date, size: NSSize = NSSize(width: 1000, height: 760), settings: (inout Settings) -> Void = { _ in },
         fixture: (URL) -> Void) {
        dir = navRoot.appendingPathComponent(name)
        logs = dir.appendingPathComponent("logs"); backups = dir.appendingPathComponent("backups")
        try? FileManager.default.removeItem(at: dir)
        for u in [logs, backups, logs.appendingPathComponent("assets")] { try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true) }
        fixture(logs)
        clock = ClockBox(now)
        suite = UserDefaults(suiteName: "dailylog.editorcheck.nav.\(name)")!
        suite.removePersistentDomain(forName: "dailylog.editorcheck.nav.\(name)")
        suite.set(true, forKey: AppModel.whatsNewKey)          // no "What's new" sheet over the window
        var s = Settings(); s.storageFolder = logs; s.onboarded = true; settings(&s)
        s.save(to: suite)
        let c = clock
        model = AppModel(defaults: suite, backupDir: backups, live: true, clock: { c.date })
        model.start()
        let host = NSHostingController(rootView: MainView(model: model))
        win = NSWindow(contentViewController: host)
        win.styleMask = [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView]
        win.toolbarStyle = .unified
        win.title = "Gloamlog"
        win.setFrame(NSRect(x: 40, y: 40, width: size.width, height: size.height), display: true)
        win.makeKeyAndOrderFront(nil)
        spin(0.6)
    }

    func close() { model.timer?.invalidate(); win.close(); spin(0.2) }

    /// The whole window (title bar included), like the scene's 1000 x 760 default and 860 x 600 minimum.
    func setSize(_ size: NSSize) {
        var f = win.frame; f.origin.y += f.height - size.height; f.size = size
        win.setFrame(f, display: true); spin(0.8)
    }
    func appearance(dark: Bool) {
        win.appearance = NSAppearance(named: dark ? .darkAqua : .aqua)
        model.bridge.pushTheme(); spin(0.8)
    }

    // fixture writers
    var fm: FileManager { .default }
    func fileText(_ day: String) -> String? { try? String(contentsOf: logs.appendingPathComponent(day + ".md"), encoding: .utf8) }
    func exists(_ day: String) -> Bool { fm.fileExists(atPath: logs.appendingPathComponent(day + ".md").path) }

    // typing like a user into the real editor (see Scenarios.swift)
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
        let how = blocking { done in model.bridge.webView.evaluateJavaScript(script) { r, _ in done(r as? String ?? "none") } } ?? "none"
        return how != "none"
    }
    var editorLoaded: Bool { model.editor.loaded }
    func openAndWait(_ day: String) -> Bool {
        model.select(.day(day))
        return wait(10) { model.editor.day == day && model.editor.loaded }
    }
}

private func page(_ logs: URL, _ day: String, _ body: String) {
    try? MarkdownFormat.serialize(day: day, body: body).write(to: logs.appendingPathComponent(day + ".md"), atomically: false, encoding: .utf8)
}
private func skipMarker(_ logs: URL, _ day: String, _ reason: String) {
    try? MarkdownFormat.serializeSkip(day: day, reason: reason).write(to: logs.appendingPathComponent(day + ".md"), atomically: false, encoding: .utf8)
}
private let notes = ["Drafted the pricing page copy and reviewed the signup form with Ana, then answered two customer emails about the webinar.",
                     "Webinar landing page wireframes; fixed the tracking on the demo form and checked the thank-you page redirects.",
                     "Reviewed the Q3 campaign report and wrote the summary for Friday's sync, plus a note for the leadership channel.",
                     "Interviewed two candidates for the content role and wrote up feedback for the hiring committee afterwards."]
private func fullDay(_ logs: URL, _ day: String, _ i: Int) {
    page(logs, day, "## What I did\n\n\(notes[i % notes.count])\n\n## Finished\n\n- Pricing table update shipped\n- Form fix merged\n\n## To do next\n\n- Follow up on open items")
}

/// ~60 days to Wed 7 Oct 2026 with gaps, a skipped run, partial days, one image page and one long page.
private func bigFixture(_ logs: URL) {
    let cal = Calendar.current
    let img = NSImage(size: NSSize(width: 480, height: 270))
    img.lockFocus(); NSColor(srgbRed: 0.86, green: 0.93, blue: 0.9, alpha: 1).setFill(); NSBezierPath(rect: NSRect(x: 0, y: 0, width: 480, height: 270)).fill()
    NSColor(srgbRed: 0.05, green: 0.48, blue: 0.37, alpha: 1).setFill(); NSBezierPath(roundedRect: NSRect(x: 40, y: 40, width: 220, height: 120), xRadius: 12, yRadius: 12).fill()
    img.unlockFocus()
    if let tiff = img.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff), let png = rep.representation(using: .png, properties: [:]) {
        try? png.write(to: logs.appendingPathComponent("assets/2026-10-06-1a2b3c4d.png"))
    }
    let missed: Set<String> = ["2026-10-02", "2026-09-21", "2026-09-10", "2026-08-25", "2026-08-12"]
    let started: [String: String] = ["2026-09-29": "Started the quarterly summary, got through the first three sections only.",
                                     "2026-09-25": "Sync with Ana about the signup form and a few notes on the pricing page copy.",
                                     "2026-09-09": "Wrote a few lines about the onboarding review."]
    let skipped: [String: String] = ["2026-09-24": "Holiday", "2026-09-15": "Leave", "2026-08-17": "Leave", "2026-08-18": "Leave", "2026-08-19": "Leave"]
    for back in 1...60 {
        guard let k = DayKey.adding("2026-10-07", -back, cal) else { continue }
        let wd = DayKey.weekday(k, cal)
        if wd == 1 || wd == 7 || missed.contains(k) { continue }
        if let r = skipped[k] { skipMarker(logs, k, r); continue }
        if let t = started[k] { page(logs, k, "## What I did\n\n\(t)"); continue }
        if k == "2026-10-06" {
            page(logs, k, """
            ## What I did

            Synced with **Ana** about the signup form and left comments on the hero copy. The new pricing table reads much better than the old one, and the *annual* toggle finally works.

            ![Wireframe](assets/2026-10-06-1a2b3c4d.png)

            ## Finished

            - Pricing table update shipped
            - Form fix merged

            ## Pending / blocked

            - Waiting on legal for the DPA
            - Ping Ana about the form

            ## To do next

            - [ ] Review the pricing page copy with Ana
            - [ ] Ship the webinar page
            - [x] Send the deck
            """)
        } else if k == "2026-09-30" {
            let long = (1...14).map { "Paragraph \($0): " + notes[$0 % notes.count] }.joined(separator: "\n\n")
            page(logs, k, "## What I did\n\n\(long)\n\n## To do next\n\n- Follow up on open items")
        } else { fullDay(logs, k, back) }
    }
}

// MARK: - accessibility tree (SwiftUI exposes its own nodes once enhanced UI is on; AXPress runs the button's action)

private func enableAX() {
    NSApp.perform(NSSelectorFromString("accessibilitySetValue:forAttribute:"), with: NSNumber(value: true), with: "AXEnhancedUserInterface" as NSString)
    NSApp.perform(NSSelectorFromString("accessibilitySetValue:forAttribute:"), with: NSNumber(value: true), with: "AXManualAccessibility" as NSString)
}
private func axText(_ o: AnyObject, _ sel: String) -> String {
    guard o.responds(to: NSSelectorFromString(sel)), let r = o.perform(NSSelectorFromString(sel)) else { return "" }
    let v = r.takeUnretainedValue()
    return (v as? String) ?? ""
}
private func axKids(_ o: AnyObject) -> [AnyObject] {
    guard o.responds(to: NSSelectorFromString("accessibilityChildren")), let r = o.perform(NSSelectorFromString("accessibilityChildren")) else { return [] }
    return (r.takeUnretainedValue() as? [AnyObject]) ?? []
}
private struct AXNode { let obj: AnyObject; let role: String; let label: String; let value: String }
private func axWalk(_ o: AnyObject, depth: Int = 0, into out: inout [AXNode]) {
    guard depth < 40 else { return }
    out.append(AXNode(obj: o, role: axText(o, "accessibilityRole"), label: axText(o, "accessibilityLabel"), value: axText(o, "accessibilityValue")))
    for k in axKids(o) { axWalk(k, depth: depth + 1, into: &out) }
}
/// Every accessibility node of the window, its toolbar items, its sheet and its popovers.
private func axAll(_ win: NSWindow) -> [AXNode] {
    var out = [AXNode]()
    var roots: [NSWindow] = [win]
    if let s = win.attachedSheet { roots.append(s) }
    roots += NSApp.windows.filter { $0 !== win && $0.isVisible && ($0.className.contains("Popover") || $0.parent === win) }
    for w in roots { if let v = w.contentView { axWalk(v, into: &out) } }
    for item in win.toolbar?.items ?? [] { if let v = item.view { axWalk(v, into: &out) } }
    return out
}
private func axFind(_ win: NSWindow, _ label: String, role: String = "AXButton", exact: Bool = true) -> AXNode? {
    axAll(win).first { $0.role == role && (exact ? $0.label == label : $0.label.contains(label)) }
}
private func axHas(_ win: NSWindow, _ label: String, role: String = "AXButton", exact: Bool = true) -> Bool { axFind(win, label, role: role, exact: exact) != nil }
/// Any node (a text, a group, a button) whose label or value contains `text`.
private func axShows(_ win: NSWindow, _ text: String) -> Bool { axAll(win).contains { $0.label.contains(text) || $0.value.contains(text) } }
@discardableResult
private func axPress(_ win: NSWindow, _ label: String, role: String = "AXButton", exact: Bool = true) -> Bool {
    guard let n = axFind(win, label, role: role, exact: exact), n.obj.responds(to: NSSelectorFromString("accessibilityPerformPress")) else { return false }
    _ = n.obj.perform(NSSelectorFromString("accessibilityPerformPress"))
    spin(0.3)
    return true
}
/// Day cells (buttons, or plain views for days to come) of the calendar group labelled `group`: children named "Monday 5 October, ...".
private func axCalendarCells(_ win: NSWindow, inGroup group: String) -> Int {
    guard let g = axAll(win).first(where: { $0.role == "AXGroup" && $0.label.hasPrefix(group) }) else { return -1 }
    let days = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]
    return axKids(g.obj).filter { k in let l = axText(k, "accessibilityLabel"); return days.contains { l.hasPrefix($0) } }.count
}

// MARK: - screenshots

/// The window as the window server composites it (titlebar, toolbar, sidebar, the real WKWebView), plus an open sheet or popover.
private func capture(_ w: NavWorld, _ name: String, settle: TimeInterval = 1.0) {
    try? FileManager.default.createDirectory(at: navShotsDir, withIntermediateDirectories: true)
    spin(settle)
    func save(_ cg: CGImage?, _ file: String) {
        guard let cg = cg, let d = NSBitmapImageRep(cgImage: cg).representation(using: .png, properties: [:]) else { print("   (no screenshot: \(file))"); return }
        try? d.write(to: navShotsDir.appendingPathComponent(file + ".png")); print("   wrote \(file)")
    }
    func shot(_ win: NSWindow) -> CGImage? {
        CGWindowListCreateImage(.null, .optionIncludingWindow, CGWindowID(win.windowNumber), [.boundsIgnoreFraming, .bestResolution])
    }
    save(shot(w.win), name)
    // A sheet or popover is its own window: it gets its own picture next to the window's.
    for x in NSApp.windows where x !== w.win && x.isVisible && x.windowNumber > 0 && (x.className.contains("Popover") || x === w.win.attachedSheet) {
        save(shot(x), name + (x === w.win.attachedSheet ? "-sheet" : "-popover"))
    }
}

// MARK: - scenarios

private let sentence = "Wrote up the notes from the customer call, answered the open emails, reviewed the pricing page copy with Ana and planned the next steps for the week."
private func lines(_ n: Int) -> String { (1...n).map { "Note \($0): " + sentence }.joined(separator: " ") }

/// Types `text` into the open day and waits (up to 4 s) until that day's file exists and counts as logged.
private func writeDay(_ w: NavWorld, _ day: String, _ text: String = sentence) -> Bool {
    guard w.model.editor.day == day, w.editorLoaded, w.type(text) else { return false }
    return wait(4) { w.exists(day) && w.model.states[day] == .logged }
}

func runNavScenarios() {
    print("11. navigation (real window: sidebar, calendar, toolbar, Go to date, Catch up, Week review)")
    enableAX()
    try? FileManager.default.createDirectory(at: navRoot, withIntermediateDirectories: true)
    navMain()
    navBackfill()
    navWeek()
    navCatchUp()
}

/// The 60-day fixture: calendar, any past date, stepping, Go to date, the refresh policy, short windows.
private func navMain() {
    let w = NavWorld("big", now: navDate(2026, 10, 7), settings: { $0.weekStart = 2 }) { bigFixture($0) }
    let m = w.model
    check("nav: the real window opens on today with its editor", wait(25) { m.editor.loaded })
    guard m.editor.loaded else { print("   editor never loaded; stopping navigation scenarios"); w.close(); return }
    defer { w.close() }
    spin(1.0)

    // -- the window offers every route
    check("sidebar: Today, Catch up, Review, the calendar, Recent and Settings are in the window",
          axHas(w.win, "Today, not logged") && axHas(w.win, "Catch up", exact: false) && axHas(w.win, "Review, weekly review")
            && axHas(w.win, "Calendar, October 2026", role: "AXGroup") && axHas(w.win, "Recent", role: "AXHeading") && axHas(w.win, "Settings"))
    check("toolbar: previous day, Today, next day and Go to date are in the window",
          axHas(w.win, "Previous day") && axHas(w.win, "Today") && axHas(w.win, "Next day") && axHas(w.win, "Go to date"))
    check("toolbar: Today and next day are disabled on today", !m.canStep(1) && m.canStep(-1))
    let expected = ["2026-09-09", "2026-09-10", "2026-09-21", "2026-09-25", "2026-09-29", "2026-10-02"]
    check("catch up: the list is the 6 unlogged working days of the last 30 (oldest first)", m.catchUpDays == expected, "\(m.catchUpDays)")
    check("sidebar: the Catch up row carries the count", axHas(w.win, "Catch up, 6 unlogged days"))
    check("Today says so, once, quietly", axShows(w.win, "You have 6 unlogged days."))
    check("calendar: October shows a 6-row grid (42 days) and marks for today, logged, started, not logged",
          axCalendarCells(w.win, inGroup: "Calendar, October 2026") == 42
            && axHas(w.win, "Friday 2 October, not logged") && axHas(w.win, "Tuesday 29 September, started")
            && axHas(w.win, "Wednesday 7 October, not logged, today") && axHas(w.win, "Tuesday 6 October, logged"))
    capture(w, "main-today-light")

    // -- R1 / M1-A1: a workday from last month with no page, from the calendar
    axPress(w.win, "Previous month")
    check("R1 calendar: the previous-month arrow shows September", m.shownMonth == YearMonth(year: 2026, month: 9))
    check("R1 calendar: the unwritten workday is a button in the grid", axHas(w.win, "Monday 21 September, not logged"))
    axPress(w.win, "Monday 21 September, not logged")
    check("R1: clicking it opens that day with the template", wait(10) { m.editor.day == "2026-09-21" && m.editor.loaded } && m.editor.initialBody == Settings.defaultTemplate)
    spin(2.2)
    check("R1: nothing is written for a day that was only opened", !w.exists("2026-09-21") && !m.editor.hasUnsavedEdits)
    check("R1: the header says how long ago it was", m.relativeLabel(for: "2026-09-21") == "2 weeks ago")
    check("R1: the calendar follows the page (September, selected)", m.shownMonth == YearMonth(year: 2026, month: 9) && m.openDayKey == "2026-09-21")
    check("R1: typing reaches the editor", w.type("Wrote two or three sentences about the customer call. Then reviewed the pricing page copy with Ana, and noted the follow ups for next week."))
    let t0 = Date()
    check("R1: two seconds after typing, that day's file exists", wait(2.5) { w.exists("2026-09-21") }, String(format: "%.1fs", Date().timeIntervalSince(t0)))
    let f = w.fileText("2026-09-21") ?? ""
    check("R1: the file has the title, the template and the typed text", f.hasPrefix("# 2026-09-21\n\n") && f.contains("## What I did") && f.contains("customer call"), f)
    check("R1: the day is logged, left the Catch up list within 2 s, and the badge drops to 5",
          wait(2) { m.states["2026-09-21"] == .logged && !m.catchUpDays.contains("2026-09-21") } && axHas(w.win, "Catch up, 5 unlogged days"))
    check("R1: the calendar cell now reads logged", wait(2) { axHas(w.win, "Monday 21 September, logged") })

    // -- M1-A2: previous / next CALENDAR day, gaps included; next on today does nothing
    m.select(.day("2026-10-05"))
    _ = wait(8) { m.editor.day == "2026-10-05" && m.editor.loaded }
    axPress(w.win, "Previous day")
    check("M1-A2: ⌘[ from Mon 5 Oct opens Sun 4 Oct (no page, a gap)", wait(8) { m.editor.day == "2026-10-04" && m.editor.loaded } && !w.exists("2026-10-04"))
    m.step(-1); m.step(-1)
    check("M1-A2: ... and keeps walking calendar days to Fri 2 Oct, which has no page", wait(8) { m.editor.day == "2026-10-02" && m.editor.loaded } && !w.exists("2026-10-02"))
    for _ in 0..<5 { m.step(1) }
    _ = wait(8) { m.editor.day == "2026-10-07" && m.editor.loaded }
    check("M1-A2: ⌘] walks forward day by day and arrives on today", m.selection == .day("2026-10-07"))
    m.step(1); spin(0.5)
    check("M1-A2: ⌘] on today does nothing", m.selection == .day("2026-10-07") && m.editor.day == "2026-10-07" && !m.canStep(1))
    check("nothing was written by walking through empty days", !w.exists("2026-10-04") && !w.exists("2026-10-03") && !w.exists("2026-10-02"))
    m.openDay("2030-01-01"); m.openDay("1999-12-31"); m.openDay("2026-02-30")
    check("a future date, a date before 2000 and an impossible date open nothing", m.selection == .day("2026-10-07"))

    // -- Go to date (N3)
    axPress(w.win, "Go to date")
    check("Go to date: the toolbar button opens the popover with its field and calendar", wait(2) { m.goToDateOpen } && wait(2) { axHas(w.win, "Go to date", role: "AXTextField") })
    capture(w, "go-to-date-light", settle: 0.2)
    m.goToDateOpen = false; spin(0.4)
    let cases: [(String, String?)] = [("last fri", "2026-10-02"), ("2 oct", "2026-10-02"), ("-3", "2026-10-04"), ("yesterday", "2026-10-06"), ("garbage", nil), ("2027-01-01", nil)]
    for (text, day) in cases {
        m.openToday(); _ = wait(8) { m.editor.day == "2026-10-07" && m.editor.loaded }
        let r = m.goToDate(text)
        if let d = day { check("Go to date: \"\(text)\" opens \(d)", r == .opened(d) && wait(8) { m.editor.day == d && m.editor.loaded }, "\(r)") }
        else { check("Go to date: \"\(text)\" opens nothing and says so", (r == .unreadable || r == .future) && m.selection == .day("2026-10-07"), "\(r)") }
    }
    check("Go to date: a future day is told it has not happened yet", m.goToDate("2027-01-01") == .future && m.goToDate("tomorrow") == .future && m.goToDate("banana") == .unreadable)

    // -- refresh policy (N1): the tick moves the clock and re-reads the folder only when it changed
    m.openToday(); _ = wait(8) { m.editor.day == "2026-10-07" && m.editor.loaded }
    let n0 = m.reloadCount
    for _ in 0..<3 { m.tick() }
    check("refresh: three 30 s ticks with nothing changed do not re-read the folder", m.reloadCount == n0, "\(m.reloadCount - n0) reloads")
    page(w.logs, "2026-10-03", "## What I did\n\nWrote on a Saturday from another Mac: a long enough note about the week and the plans for next Monday morning.")
    m.tick()
    check("refresh: a day file added outside the app is picked up on the next tick", m.reloadCount == n0 + 1 && m.states["2026-10-03"] == .logged, "\(m.reloadCount - n0) reloads")
    let n1 = m.reloadCount
    check("refresh: typing and autosave are not mistaken for an outside change", writeDay(w, "2026-10-07"))
    m.tick(); m.tick()
    check("refresh: ... so the next ticks still read nothing", m.reloadCount == n1, "\(m.reloadCount - n1) reloads")
    m.refreshClock()
    check("refresh: the clock tick alone never reads the folder", m.reloadCount == n1)
    w.clock.date = navDate(2026, 10, 8, 0, 5); m.refreshClock()
    check("refresh: midnight moves 'today' and recomputes without a reload", m.today == "2026-10-08" && m.reloadCount == n1 && m.catchUpDays.last != "2026-10-07" || m.today == "2026-10-08")
    w.clock.date = navDate(2026, 10, 7); m.refreshClock()

    // -- week start follows Settings
    check("week start: Monday-first by Settings", m.cal.firstWeekday == 2 && (m.monthGrid(year: 2026, month: 10).first?.first.map { DayKey.weekday($0.day, m.cal) }) == 2)
    var st = m.settings; st.weekStart = 1; m.settings = st
    check("week start: switching to Sunday rebuilds the calendar", m.cal.firstWeekday == 1 && (m.monthGrid(year: 2026, month: 10).first?.first.map { DayKey.weekday($0.day, m.cal) }) == 1)
    st = m.settings; st.weekStart = 2; m.settings = st
    check("week start: ... and back to Monday", m.cal.firstWeekday == 2)

    // -- a short window turns the calendar into a one-week strip; Go to date still reaches everything
    m.openDay("2026-10-06"); _ = wait(8) { m.editor.day == "2026-10-06" && m.editor.loaded }
    w.setSize(NSSize(width: 1000, height: 760)); spin(0.6)
    check("calendar: at the default size the sidebar shows the month grid", axCalendarCells(w.win, inGroup: "Calendar") == 42)
    w.setSize(NSSize(width: 860, height: 600)); spin(0.8)
    check("calendar: at the minimum size it is a one-week strip", axCalendarCells(w.win, inGroup: "Calendar") == 7, "\(axCalendarCells(w.win, inGroup: "Calendar")) cells")
    let tiny = w.win.frame; w.win.setFrame(NSRect(x: tiny.minX, y: tiny.minY, width: 300, height: 300), display: true); spin(0.5)
    check("window: it cannot shrink below 860 x 600", w.win.frame.width >= 859 && w.win.frame.height >= 599, "\(w.win.frame.size)")
    w.setSize(NSSize(width: 860, height: 600)); spin(0.5)
    capture(w, "main-min-light")
    w.appearance(dark: true); capture(w, "main-min-dark")
    w.setSize(NSSize(width: 1000, height: 760))
    capture(w, "main-past-day-dark")
    w.appearance(dark: false)
    capture(w, "main-past-day-light")
}

/// M1-A9: writing before the first log must not create a wall of unlogged days.
private func navBackfill() {
    let w = NavWorld("backfill", now: navDate(2026, 10, 7), settings: { $0.weekStart = 2 }) { logs in
        for d in ["2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"] { fullDay(logs, d, 1) }
    }
    let m = w.model
    check("backfill: a log that starts on 1 Oct opens", wait(25) { m.editor.loaded })
    guard m.editor.loaded else { w.close(); return }
    defer { w.close() }
    let listBefore = m.catchUpDays, streakBefore = m.streak
    check("backfill: nothing is unlogged and the streak is 4 before", listBefore.isEmpty && streakBefore.current == 4 && m.settings.logStartDate == nil, "\(listBefore) \(streakBefore)")
    check("backfill: 12 Sep is before the log start", m.isBeforeLogStart("2026-09-12"))
    m.select(.day("2026-09-12")); _ = wait(8) { m.editor.day == "2026-09-12" && m.editor.loaded }
    check("backfill: the page opens with the footnote cue (before the log start)", m.isBeforeLogStart("2026-09-12") && axShows(w.win, "Before your log start. It won't appear in Catch up."))
    check("backfill: writing 12 Sep saves it", writeDay(w, "2026-09-12"))
    check("backfill: the log start is pinned at the old first page and saved", wait(2) { m.settings.logStartDate == "2026-10-01" } && Settings.load(from: w.suite).logStartDate == "2026-10-01", "\(String(describing: m.settings.logStartDate))")
    check("backfill: Catch up is unchanged", m.catchUpDays == listBefore, "\(m.catchUpDays)")
    check("backfill: the streak is unchanged", m.streak == streakBefore, "\(m.streak)")
    let days = (13...30).map { String(format: "2026-09-%02d", $0) }
    check("backfill: 13 to 30 Sep are not unlogged (off)", days.allSatisfy { m.status(of: $0) == .off }, "\(days.map { m.status(of: $0) })")
    check("backfill: the heat map agrees", m.heat.flatMap { $0 }.filter { $0.day >= "2026-09-13" && $0.day <= "2026-09-30" }.allSatisfy { $0.status == .off })
    check("backfill: 12 Sep itself is a logged page", m.status(of: "2026-09-12") == .logged)
    m.shownMonth = YearMonth(year: 2026, month: 9)
    check("backfill: the September calendar shows no amber rings (not logged) before 1 Oct",
          !m.monthGrid(year: 2026, month: 9).flatMap { $0 }.contains { $0.inMonth && $0.status == .missed })
}

/// R2 / M1-A5 / R9: a past week with no pages: every day is there, opens, and the week reads 5 of 5; copy as markdown.
private func navWeek() {
    let w = NavWorld("week", now: navDate(2026, 10, 7), settings: { $0.weekStart = 2 }) { logs in
        let cal = Calendar.current
        var d = "2026-09-07"
        while d <= "2026-10-06" {
            let wd = DayKey.weekday(d, cal)
            if wd != 1 && wd != 7 && !(d >= "2026-09-28" && d <= "2026-10-04") { fullDay(logs, d, 2) }
            d = DayKey.adding(d, 1, cal)!
        }
    }
    let m = w.model
    check("week: a log with an empty week 28 Sep to 4 Oct opens", wait(25) { m.editor.loaded })
    guard m.editor.loaded else { w.close(); return }
    defer { w.close() }
    check("week: that week has no file at all", ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04"].allSatisfy { !w.exists($0) })
    axPress(w.win, "Review, weekly review")
    check("R2: Review opens this week (Mon 5 to Sun 11 Oct)", wait(4) { m.selection == .week && m.week?.weekStart == "2026-10-05" }, "\(String(describing: m.week?.weekStart))")
    axPress(w.win, "Previous week")
    check("R2: the arrow goes to last week, which starts on a Monday per Settings", wait(4) { m.week?.weekStart == "2026-09-28" && m.week?.weekEnd == "2026-10-04" }, "\(String(describing: m.week?.weekStart))")
    spin(0.5)
    let tiles = m.weekTiles()
    check("R2: all seven days are tiles, the five workdays read Write", tiles.count == 7 && tiles.filter { $0.bottom == "Write" }.count == 5 && tiles.filter { $0.status == .off }.count == 2, "\(tiles.map { "\($0.day) \($0.status) \($0.bottom)" })")
    let workdays = ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02"]
    check("R2: each tile is a button in the window (also Sat and Sun)", workdays.allSatisfy { axHas(w.win, m.spokenStatus($0)) } && axHas(w.win, "Saturday 3 October, not a workday") && axHas(w.win, "Sunday 4 October, not a workday"))
    check("R2: the empty week says so and offers Catch up this week, not only Go to today",
          axShows(w.win, "Nothing logged this week yet.") && axHas(w.win, "Catch up this week") && axHas(w.win, "Go to today"))
    check("R2: the counts line reads 0 of 5 (5 not logged), never 'missed'", m.weekCounts() == WeekCounts(logged: 0, skipped: 0, notLogged: 5) && axShows(w.win, "Logged 0 of 5 · 5 not logged"))
    capture(w, "week-empty-light")
    w.appearance(dark: true); capture(w, "week-empty-dark"); w.appearance(dark: false)
    w.setSize(NSSize(width: 860, height: 600)); capture(w, "week-empty-min-light"); w.setSize(NSSize(width: 1000, height: 760))
    var typed = [String]()
    for d in workdays {
        axPress(w.win, m.spokenStatus(d))
        let opened = wait(10) { m.editor.day == d && m.editor.loaded }
        let ok = opened && writeDay(w, d, "Day \(d): " + sentence)
        typed.append(d)
        check("R2: \(d) opens from the week tile and takes about 20 words", ok)
        axPress(w.win, "Review, weekly review")
        _ = wait(4) { m.selection == .week && m.week?.weekStart == "2026-09-28" }
    }
    check("R2: Review returns to the week of the page you were on", m.week?.weekStart == "2026-09-28")
    check("R2: the week reads 5 of 5 logged", wait(3) { m.weekCounts() == WeekCounts(logged: 5, skipped: 0, notLogged: 0) } && axShows(w.win, "Logged 5 of 5"), "\(m.weekCounts())")
    check("R2: Catch up this week is gone once nothing is unlogged", !m.weekHasUnlogged && !axHas(w.win, "Catch up this week"))
    capture(w, "week-full-light")
    // R9: copy as markdown
    NSPasteboard.general.clearContents()
    m.copyWeek(); spin(0.4)           // the old 'Copy as markdown' button is now the Export menu; copyWeek() is the same command as ⇧⌘C
    let md = NSPasteboard.general.string(forType: .string) ?? ""
    check("R9: Copy as markdown puts a week on the clipboard", m.copiedFlash && md.hasPrefix("# Week of 28 Sep"), String(md.prefix(60)))
    check("R9: it holds what was written, each day under its date", workdays.allSatisfy { md.contains("Day \($0): Wrote up the notes") } && md.contains("## Mon 28 Sep") && md.contains("## Fri 2 Oct"), String(md.prefix(300)))
    let chrome = ["Copy as markdown", "Catch up", "Write", "Open", "Group by", "Review", "Previous week", "unlogged"]
    check("R9: ... and no app chrome", !chrome.contains { md.contains($0) }, chrome.filter { md.contains($0) }.joined(separator: ", "))
    m.weekGrouping = .section
    spin(0.3)
    capture(w, "week-sections-light")
    m.weekGrouping = .day
}

/// R3 (timed) / M1-A3 and R4 / M1-A4: three unwritten workdays.
private func navCatchUp() {
    // ---- R3: write them through the session
    let w = NavWorld("r3", now: navDate(2026, 10, 7), settings: { $0.weekStart = 2 }) { logs in
        let cal = Calendar.current
        var d = "2026-09-14"
        while d <= "2026-10-01" { let wd = DayKey.weekday(d, cal); if wd != 1 && wd != 7 { fullDay(logs, d, 3) }; d = DayKey.adding(d, 1, cal)! }
    }
    let m = w.model
    check("R3: a log with 3 unwritten workdays (2, 5, 6 Oct) opens", wait(25) { m.editor.loaded })
    guard m.editor.loaded else { w.close(); return }
    defer { w.close() }
    let three = ["2026-10-02", "2026-10-05", "2026-10-06"]
    check("R3/A3: Catch up lists exactly those 3 and the sidebar says 3", m.catchUpDays == three && axHas(w.win, "Catch up, 3 unlogged days"), "\(m.catchUpDays)")
    let start = Date()
    axPress(w.win, "Catch up, 3 unlogged days")
    check("R3: Catch up opens with 3 rows, oldest first, each with Write and Skip", wait(4) { m.selection == .catchUp }
          && three.allSatisfy { axHas(w.win, "Write \(m.spokenDate($0))") && axHas(w.win, "Skip \(m.spokenDate($0))") } && axHas(w.win, "Start catching up"))
    capture(w, "catch-up-light")
    w.appearance(dark: true); capture(w, "catch-up-dark"); w.appearance(dark: false)
    w.setSize(NSSize(width: 860, height: 600)); capture(w, "catch-up-min-light"); w.setSize(NSSize(width: 1000, height: 760))
    axPress(w.win, "Start catching up")
    check("R3: Start catching up opens the oldest day with the session bar", wait(10) { m.editor.day == "2026-10-02" && m.editor.loaded } && m.session?.current == "2026-10-02"
          && axHas(w.win, "Catching up, 1 of 3", role: "AXGroup") && axHas(w.win, "Next unlogged day"))
    capture(w, "session-light")
    w.appearance(dark: true); capture(w, "session-dark"); w.appearance(dark: false)
    var loggedAt = [TimeInterval]()
    for (i, d) in three.enumerated() {
        check("R3: day \(i + 1) of 3 (\(d)) takes the minimum words", writeDay(w, d, "Day \(d): " + sentence))
        loggedAt.append(Date().timeIntervalSince(start))
        check("A3: the badge drops to \(2 - i) within 2 s of the save", wait(2) { m.catchUpDays.count == 2 - i } && axHas(w.win, "Catch up, \(2 - i) unlogged day\(2 - i == 1 ? "" : "s")") || (2 - i == 0 && wait(2) { m.catchUpDays.isEmpty }))
        if i < 2 {
            check("A3/R3: Next unlogged day opens \(three[i + 1])", axPress(w.win, "Next unlogged day") && wait(10) { m.editor.day == three[i + 1] && m.editor.loaded } && m.session?.position == i + 2)
        }
    }
    let took = loggedAt.last ?? 99
    check("R3 (timed): third day logged \(String(format: "%.1f", took)) s after clicking Catch up (budget 120 s)", took < 120)
    axPress(w.win, "Next unlogged day")
    check("R3/A3: with none left it ends on All caught up (3 logged)", wait(6) { m.selection == .catchUp && m.sessionResult == SessionResult(logged: 3, skipped: 0) && m.session == nil } && axShows(w.win, "All caught up. 3 logged."), "\(String(describing: m.sessionResult))")
    check("R3: the streak healed (a backfilled day is an ordinary logged day)", m.streak.current >= 7, "\(m.streak)")
    check("R3: Review this week and Go to today are offered", axHas(w.win, "Review this week") && axHas(w.win, "Go to today"))
    capture(w, "catch-up-done-light")

    // ---- R4: skip 3 as Leave, Undo
    w.close()
    let r4 = NavWorld("r4", now: navDate(2026, 10, 7), settings: { $0.weekStart = 2; $0.logStartDate = "2026-09-23" }) { logs in
        for d in ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"] { fullDay(logs, d, 4) }
    }
    let n = r4.model
    check("R4: a log whose start (23 Sep) is before its first page opens", wait(25) { n.editor.loaded })
    guard n.editor.loaded else { r4.close(); return }
    defer { r4.close() }
    let gap = ["2026-09-23", "2026-09-24", "2026-09-25"]
    let streak0 = n.streak
    check("R4: three unlogged days precede every page; the streak counts the 7 after them", n.catchUpDays == gap && streak0.current == 7, "\(n.catchUpDays) \(streak0)")
    axPress(r4.win, "Catch up, 3 unlogged days")
    _ = wait(4) { n.selection == .catchUp }
    n.catchSelection = Set(gap)                                     // ⇧/⌘-click is a mouse gesture: the selection is set through the model
    spin(0.5)
    check("R4: the selection bar says 3 selected and offers Skip…", axShows(r4.win, "3 selected") && axHas(r4.win, "Skip…"))
    capture(r4, "catch-up-selected-light")
    axPress(r4.win, "Skip…")
    check("R4: Skip… opens the sheet for exactly these 3 days", wait(3) { n.sheet == .skipMany(gap) } && axShows(r4.win, "Skip these 3 days?"))
    capture(r4, "skip-sheet-light")
    axPress(r4.win, "Leave")
    axPress(r4.win, "Skip 3 days")
    check("R4/A4: 3 skip markers are written (Leave)", wait(5) { gap.allSatisfy { (r4.fileText($0) ?? "").contains("> Skipped: Leave") } })
    check("R4/A4: the days leave the list, the streak is unchanged, and the badge says none", wait(3) { n.catchUpDays.isEmpty } && n.streak == streak0 && axHas(r4.win, "Catch up, nothing to catch up"), "\(n.streak) \(streak0)")
    check("R4: Undo is offered (\(n.skipUndo?.text ?? "none"))", n.skipUndo?.days == gap && axHas(r4.win, "Undo"))
    axPress(r4.win, "Undo")
    check("R4/A4: Undo removes the markers and the 3 days are back", wait(5) { gap.allSatisfy { !r4.exists($0) } && n.catchUpDays == gap }, "\(n.catchUpDays)")
    check("R4: ... and the streak is still unchanged", n.streak == streak0)
}
