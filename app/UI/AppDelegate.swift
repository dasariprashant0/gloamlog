// AppDelegate.swift - process lifecycle: start timers/observers, keep the app alive after the window closes, flush the
// page before quitting, standard About panel with the Info.plist version.
import AppKit
import SwiftUI

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ n: Notification) { AppModel.shared.start(); AppModel.shared.sources.startAuto() }

    /// `gloamlog://refresh`: Claude Code's hook says a session ended, so today's lines are looked for now (a no-op unless "Add them for me" is on).
    func application(_ app: NSApplication, open urls: [URL]) {
        if urls.contains(where: { $0.scheme == "gloamlog" && $0.host == "refresh" }) { AppModel.shared.sources.autoTick(force: true) }
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
        let v = info?["CFBundleShortVersionString"] as? String ?? "0.6.0"
        let credits = NSAttributedString(
            string: "A calm end-of-day log. Plain markdown files on your Mac. No account, no tracking. The only network request is the update check you ask for.",
            attributes: [.font: NSFont.systemFont(ofSize: 11), .foregroundColor: NSColor.secondaryLabelColor])
        NSApp.orderFrontStandardAboutPanel(options: [
            .applicationName: "Gloamlog", .applicationVersion: v, .version: info?["CFBundleVersion"] as? String ?? v, .credits: credits])
        NSApp.activate(ignoringOtherApps: true)
    }
}
