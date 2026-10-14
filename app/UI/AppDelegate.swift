// AppDelegate.swift - process lifecycle: start timers/observers, keep the app alive after the window closes, flush the
// page before quitting, standard About panel with the Info.plist version.
import AppKit
import SwiftUI

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ n: Notification) { AppModel.shared.start(); AppModel.shared.sources.startAuto() }

    /// `gloamlog://refresh`: Claude Code's hook says a session ended, so today's lines are looked for now (a no-op unless "Add them for me" is on).
    func application(_ app: NSApplication, open urls: [URL]) {
        if urls.contains(where: { $0.scheme == "gloamlog" && $0.host == "refresh" }) { AppModel.shared.sources.autoTick(force: true); AppModel.shared.drainInbox() }
        if urls.contains(where: { $0.scheme == "gloamlog" && $0.host == "layout" }) { AppDelegate.writeLayoutReport() }
    }

    /// Diagnostic for "the window is drawn wrong": writes /tmp/gloamlog-layout.txt (window and view frames, scroll offsets, safe areas) and
    /// /tmp/gloamlog-layout.png (the window's SwiftUI content; the web editor does not show in it). Run `open gloamlog://layout` or use
    /// Gloamlog > Save Layout Report while it looks wrong. Nothing is sent anywhere.
    static func writeLayoutReport() {
        guard let w = NSApp.windows.first(where: { $0.isVisible && $0.title == "Gloamlog" }) ?? NSApp.keyWindow, let cv = w.contentView else { return }
        var lines = ["window frame \(w.frame) contentLayoutRect \(w.contentLayoutRect) visibleScreen \(w.screen?.visibleFrame ?? .zero)",
                     "styleMask \(w.styleMask.rawValue) titleVisibility \(w.titleVisibility.rawValue) toolbarVisible \(w.toolbar?.isVisible ?? false) fullScreen \(w.styleMask.contains(.fullScreen))",
                     "contentView frame \(cv.frame) bounds \(cv.bounds) fitting \(cv.fittingSize) safeArea \(cv.safeAreaInsets)"]
        func dump(_ v: NSView, _ depth: Int) {
            guard depth < 14 else { return }
            var l = String(repeating: "  ", count: depth) + "\(type(of: v)) frame=\(v.frame.integral)"
            if let sc = v as? NSScrollView { l += " SCROLL offsetY=\(Int(sc.contentView.bounds.origin.y)) docH=\(Int(sc.documentView?.frame.height ?? 0))" }
            if v.safeAreaInsets.top != 0 { l += " safeTop=\(v.safeAreaInsets.top)" }
            lines.append(l)
            v.subviews.forEach { dump($0, depth + 1) }
        }
        dump(cv.superview ?? cv, 0)
        try? lines.joined(separator: "\n").write(toFile: "/tmp/gloamlog-layout.txt", atomically: true, encoding: .utf8)
        if let rep = cv.bitmapImageRepForCachingDisplay(in: cv.bounds) {
            cv.cacheDisplay(in: cv.bounds, to: rep)
            try? rep.representation(using: .png, properties: [:])?.write(to: URL(fileURLWithPath: "/tmp/gloamlog-layout.png"))
        }
    }

    /// Quit: ask the editor for its final text and save it first (async, with a 1.5 s ceiling). If the text cannot be
    /// written the user is asked before it is lost.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        let m = AppModel.shared
        guard m.live, m.editorNeedsQuitFlush else { return .terminateNow }
        m.flushForQuit { saved in
            if saved { NSApp.reply(toApplicationShouldTerminate: true) }
            else { NSApp.reply(toApplicationShouldTerminate: m.confirmQuitWithUnsavedEdits()) }
        }
        return .terminateLater
    }

    // Closing the window keeps the app alive; that is what makes the reminder work.
    func applicationShouldTerminateAfterLastWindowClosed(_ a: NSApplication) -> Bool { false }
    func applicationShouldHandleReopen(_ a: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { AppModel.shared.showMainWindow(activate: true) }
        return true
    }

    /// Right-click on the Dock icon (DESIGN_V4 section 1): Today, Jot, Catch Up, Settings.
    func applicationDockMenu(_ sender: NSApplication) -> NSMenu? {
        let menu = NSMenu()
        for (title, action) in [("Today", #selector(dockToday)), ("Jot…", #selector(dockJot)),
                                ("Catch Up", #selector(dockCatchUp)), ("Settings…", #selector(dockSettings))] {
            let item = NSMenuItem(title: title, action: action, keyEquivalent: "")
            item.target = self
            menu.addItem(item)
        }
        return menu
    }
    @objc private func dockToday() { let m = AppModel.shared; m.showMainWindow(activate: true); m.openToday() }
    @objc private func dockJot() { AppModel.shared.showJotPanel(source: "menu") }
    @objc private func dockCatchUp() { let m = AppModel.shared; m.showMainWindow(activate: true); m.select(.catchUp) }
    @objc private func dockSettings() { AppModel.shared.openSettings() }

    static func showAbout() {
        let info = Bundle.main.infoDictionary
        let v = info?["CFBundleShortVersionString"] as? String ?? "0.6.4"
        let credits = NSAttributedString(
            string: "A calm end-of-day log. Plain markdown files on your Mac. No account, no tracking. The only network request is the update check you ask for.",
            attributes: [.font: NSFont.systemFont(ofSize: 11), .foregroundColor: NSColor.secondaryLabelColor])
        NSApp.orderFrontStandardAboutPanel(options: [
            .applicationName: "Gloamlog", .applicationVersion: v, .version: info?["CFBundleVersion"] as? String ?? v, .credits: credits])
        NSApp.activate(ignoringOtherApps: true)
    }
}
