// Screens.swift - REAL screenshots of the page UI with the real editor: the SwiftUI views are drawn with cacheDisplay and
// the WKWebView (which cacheDisplay cannot capture) is added with WKWebView.takeSnapshot, composited at its frame.
// Usage: dailylog-editor-check <index.html> --screenshots <outdir>     (needs real WebKit: run outside the sandbox)
import AppKit
import SwiftUI
import WebKit

private let shotRoot = URL(fileURLWithPath: ProcessInfo.processInfo.environment["EC_TMP"] ?? NSTemporaryDirectory()).appendingPathComponent("dl-real-shots")

func runScreenshots(_ outDir: String) {
    print("10. real screenshots -> \(outDir)")
    try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)
    let logs = shotRoot.appendingPathComponent("logs"), backups = shotRoot.appendingPathComponent("backups")
    for u in [logs, backups] { try? FileManager.default.removeItem(at: u); try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true) }
    let assets = logs.appendingPathComponent("assets"); try? FileManager.default.createDirectory(at: assets, withIntermediateDirectories: true)

    let cal = Calendar.current
    func page(_ day: String, _ body: String) {
        try? MarkdownFormat.serialize(day: day, body: body).write(to: logs.appendingPathComponent(day + ".md"), atomically: false, encoding: .utf8)
    }
    let img = NSImage(size: NSSize(width: 480, height: 270))
    img.lockFocus(); NSColor(srgbRed: 0.86, green: 0.93, blue: 0.9, alpha: 1).setFill(); NSBezierPath(rect: NSRect(x: 0, y: 0, width: 480, height: 270)).fill()
    NSColor(srgbRed: 0.05, green: 0.48, blue: 0.37, alpha: 1).setFill(); NSBezierPath(roundedRect: NSRect(x: 40, y: 40, width: 220, height: 120), xRadius: 12, yRadius: 12).fill()
    img.unlockFocus()
    if let tiff = img.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff), let png = rep.representation(using: .png, properties: [:]) {
        try? png.write(to: assets.appendingPathComponent("2026-10-05-1a2b3c4d.png"))
    }
    let notes = ["Drafted the pricing page copy and reviewed the signup form with Ana.", "Webinar landing page wireframes; fixed the tracking on the demo form.",
                 "Reviewed the Q3 campaign report and wrote the summary for Friday's sync.", "Interviewed two candidates for the content role."]
    for back in 1...60 {
        guard let k = DayKey.adding("2026-10-06", -back, cal) else { continue }
        let wd = DayKey.weekday(k, cal)
        if wd == 1 || wd == 7 || back % 11 == 4 { continue }
        if back % 9 == 3 { try? MarkdownFormat.serializeSkip(day: k, reason: "Holiday").write(to: logs.appendingPathComponent(k + ".md"), atomically: false, encoding: .utf8); continue }
        if back == 1 {
            page(k, """
            ## What I did

            Synced with **Ana** about the signup form and left comments on the hero copy. The new pricing table reads much better than the old one, and the *annual* toggle finally works.

            ![Wireframe](assets/2026-10-05-1a2b3c4d.png)

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
        } else {
            page(k, "## What I did\n\n\(notes[back % notes.count])\n\n## Finished\n\n- Pricing table update shipped\n- Form fix merged\n\n## To do next\n\n- Follow up on open items")
        }
    }

    var fakeNow = cal.date(from: DateComponents(year: 2026, month: 10, day: 6, hour: 10, minute: 0))!
    let suite = UserDefaults(suiteName: "dailylog.editorcheck.shots")!
    suite.removePersistentDomain(forName: "dailylog.editorcheck.shots")
    UserDefaults.standard.set(false, forKey: "yesterdayExpanded")
    var s = Settings(); s.storageFolder = logs; s.onboarded = true
    s.save(to: suite)
    let model = AppModel(defaults: suite, backupDir: backups, live: true, clock: { fakeNow })
    model.start()

    let root = HStack(spacing: 0) { SidebarView(model: model).frame(width: 240); DetailRouter(model: model).frame(maxWidth: .infinity) }
        .frame(width: 1000, height: 760)
    let host = NSHostingView(rootView: AnyView(root))
    host.frame = NSRect(x: 0, y: 0, width: 1000, height: 760)
    let win = NSWindow(contentRect: host.frame, styleMask: [.titled], backing: .buffered, defer: false)
    win.contentView = host
    win.orderFrontRegardless()
    guard wait(25, until: { model.editor.loaded }) else { print("   editor never loaded"); return }

    func webView() -> WKWebView { model.bridge.webView }
    func capture(_ name: String, dark: Bool = false) {
        win.appearance = NSAppearance(named: dark ? .darkAqua : .aqua)
        model.bridge.pushTheme()
        spin(1.0)
        host.layoutSubtreeIfNeeded()
        guard let rep = host.bitmapImageRepForCachingDisplay(in: host.bounds) else { return }
        host.cacheDisplay(in: host.bounds, to: rep)
        let web: NSImage? = blocking(10) { (done: @escaping (NSImage?) -> Void) in
            webView().takeSnapshot(with: nil) { image, _ in done(image) }
        } ?? nil
        let out = NSImage(size: host.bounds.size)
        out.lockFocus()
        rep.draw(in: NSRect(origin: .zero, size: host.bounds.size))
        if let w = web {
            let r = host.convert(webView().bounds, from: webView())
            let y = host.isFlipped ? host.bounds.height - r.maxY : r.minY
            w.draw(in: NSRect(x: r.minX, y: y, width: r.width, height: r.height))
        }
        out.unlockFocus()
        if let tiff = out.tiffRepresentation, let r2 = NSBitmapImageRep(data: tiff), let png = r2.representation(using: .png, properties: [:]) {
            try? png.write(to: URL(fileURLWithPath: "\(outDir)/\(name).png")); print("   wrote \(name)")
        }
    }
    func dom(_ label: String) {
        let script = """
        (function(){ var pm = document.querySelector('.ProseMirror'); var kids = Array.from(pm.children).slice(0, 4).map(function(c){ var cs = getComputedStyle(c); return c.tagName + '.' + c.className + ' mt=' + cs.marginTop + ' top=' + Math.round(c.getBoundingClientRect().top) + ' h=' + Math.round(c.getBoundingClientRect().height); });
          var cs = getComputedStyle(pm); return label + ' | pm padTop=' + cs.paddingTop + ' focused=' + pm.classList.contains('ProseMirror-focused') + ' || ' + kids.join(' ; '); })()
        """.replacingOccurrences(of: "label", with: "'\\(label)'")
        let r = blocking { (done: @escaping (Any?) -> Void) in webView().evaluateJavaScript(script) { r, e in done(r ?? String(describing: e)) } } ?? nil
        print("   dom: \(r ?? "nil")")
    }
    dom("unfocused")
    model.focusEditor(); spin(0.6)
    dom("focused")
    // 1. a new day: template headings, the From yesterday strip, nothing written
    capture("real-1-new-day"); capture("real-1-new-day-dark", dark: true)
    // 2. a few words typed under the first heading (progress label counts words)
    model.focusEditor()
    let moveToFirst = """
    (function(){ var h = document.querySelector('.ProseMirror h2'); var r = document.createRange(); r.selectNodeContents(h); r.collapse(false);
      var sel = window.getSelection(); sel.removeAllRanges(); sel.addRange(r); document.execCommand('insertParagraph');
      document.execCommand('insertText', false, 'Reviewed the new pricing page and left comments on the hero copy. Synced with Ana about the form.'); return true; })()
    """
    _ = blocking { (done: @escaping (Any?) -> Void) in webView().evaluateJavaScript(moveToFirst) { r, _ in done(r) } }
    spin(1.8)
    capture("real-2-typed"); capture("real-2-typed-dark", dark: true)
    // 3. the strip expanded + the reminder line (17:05)
    UserDefaults.standard.set(true, forKey: "yesterdayExpanded")
    fakeNow = cal.date(from: DateComponents(year: 2026, month: 10, day: 6, hour: 17, minute: 5))!
    model.refreshClock()
    capture("real-3-strip-and-reminder")
    // 4. a past day with an image, lists and tasks
    model.openDay("2026-10-05")
    _ = wait(8) { model.editor.day == "2026-10-05" && model.editor.loaded }
    spin(0.8)
    capture("real-4-past-day"); capture("real-4-past-day-dark", dark: true)
    // 5. a blank-page template: the editor's placeholder
    var blank = model.settings; blank.template = ""; model.settings = blank
    model.openDay("2026-10-03"); _ = wait(8) { model.editor.day == "2026-10-03" && model.editor.loaded }
    spin(0.6)
    capture("real-5-blank-page")
    // 6. the real MainView (NavigationSplitView) around the real editor
    model.openDay("2026-10-05"); _ = wait(8) { model.editor.day == "2026-10-05" && model.editor.loaded }
    host.rootView = AnyView(MainView(model: model).frame(width: 1000, height: 760))
    spin(1.5)
    capture("real-6-mainview")
    // 7. Sources on (every session and tab): how tall do the sidebar, the page and the whole window insist on being?
    var withSources = model.settings
    withSources.sources.claude = true; withSources.sources.consented = true; withSources.sources.allProjects = true
    model.settings = withSources
    model.openDay(model.today); _ = wait(8) { model.editor.day == model.today && model.editor.loaded }
    spin(4)
    func fit<V: View>(_ name: String, _ v: V, width: CGFloat) {
        let size = NSHostingController(rootView: v).sizeThatFits(in: CGSize(width: width, height: 100))        // the least height this view needs at that width
        print("   fit \(name): min height \(Int(size.height)) at width \(Int(width))")
    }
    fit("sidebar", SidebarView(model: model), width: 260)
    fit("page", DetailRouter(model: model), width: 1180)
    fit("window", MainView(model: model), width: 1440)
}
