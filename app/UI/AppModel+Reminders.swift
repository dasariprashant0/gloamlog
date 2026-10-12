// AppModel+Reminders.swift - timer, ReminderPlanner.decide, notifications, window raising, system observers.
import SwiftUI
import AppKit

extension AppModel {
    func start() {
        guard live else { return }
        Notifier.shared.setup(model: self)
        Notifier.shared.refreshState { [weak self] s in self?.notifState = s }
        ClosedAppReminders.shared.install(model: self)                 // M5a: macOS holds the next 14 reminders (re-plans itself on every change)
        activate(editor, replacing: nil, focus: sheet == nil)          // creates the editor host and loads today's page
        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in self?.tick() }
        timer?.tolerance = 5
        let nc = NotificationCenter.default, ws = NSWorkspace.shared.notificationCenter
        for n in [Notification.Name.NSCalendarDayChanged, .NSSystemClockDidChange, .NSSystemTimeZoneDidChange] {
            nc.addObserver(forName: n, object: nil, queue: .main) { [weak self] _ in self?.refreshClock() }
        }
        nc.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in
            self?.reloadIfFolderChanged(); self?.editor?.checkExternalChange(); self?.editor?.retryIfNeeded()
            Notifier.shared.refreshState { s in self?.notifState = s }
        }
        nc.addObserver(forName: NSApplication.didResignActiveNotification, object: nil, queue: .main) { [weak self] _ in self?.flushEditor() }
        nc.addObserver(forName: NSWindow.willCloseNotification, object: nil, queue: .main) { [weak self] _ in self?.flushEditor() }
        ws.addObserver(forName: NSWorkspace.willSleepNotification, object: nil, queue: .main) { [weak self] _ in self?.flushEditor() }
        ws.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { [weak self] _ in
            self?.screensAsleep = false
            DispatchQueue.main.asyncAfter(deadline: .now() + 15) { self?.tick() }   // let the unlock settle
        }
        ws.addObserver(forName: NSWorkspace.screensDidSleepNotification, object: nil, queue: .main) { [weak self] _ in self?.screensAsleep = true }
        ws.addObserver(forName: NSWorkspace.screensDidWakeNotification, object: nil, queue: .main) { [weak self] _ in self?.screensAsleep = false }
        ws.addObserver(forName: NSWorkspace.sessionDidResignActiveNotification, object: nil, queue: .main) { [weak self] _ in self?.screensAsleep = true }
        ws.addObserver(forName: NSWorkspace.sessionDidBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in self?.screensAsleep = false }
        tick()
    }

    var mainWindow: NSWindow? { NSApp.windows.first { $0.title == "Gloamlog" && !($0 is NSPanel) } }
    var windowVisible: Bool { if let w = mainWindow { return w.isVisible && !w.isMiniaturized }; return false }

    /// Every 30 s: move the clock (and the day over at midnight), then re-read the pages only if the folder changed.
    func tick() {
        refreshClock()
        reloadIfFolderChanged()
        editor?.checkExternalChange()
        editor?.retryIfNeeded()
        retryOrphans()
        guard !screensAsleep else { return }
        let (action, next) = ReminderPlanner.decide(now: now, calendar: cal, settings: settings, todayStatus: todayStatus,
                                                    state: planner, windowVisible: windowVisible)
        if next != planner { planner = next }
        perform(action)
    }

    func perform(_ a: ReminderAction) {
        switch a {
        case .none: break
        case .notify: postReminder()
        case .bringToFront(let notify):
            if notify { postReminder() }
            showMainWindow(activate: true)
            openDay(today, focus: true)
            if !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion { NSApp.requestUserAttention(.informationalRequest) }
        case .reopen:
            showMainWindow(activate: false)     // orderFrontRegardless, never steals keystrokes
        }
    }

    func postReminder() {
        let mins = cal.component(.hour, from: now) * 60 + cal.component(.minute, from: now)
        if mins - settings.reminderMinutes > 5 {
            Notifier.shared.post(title: "Today isn't logged yet",
                                 body: "It's \(Fmt.time(now)). Open Gloamlog whenever you're ready.", snoozeMinutes: settings.snoozeMinutes)
        } else {
            Notifier.shared.post(title: "Time to write up your day",
                                 body: "It takes a few minutes. Gloamlog is ready when you are.", snoozeMinutes: settings.snoozeMinutes)
        }
    }

    /// activate=false uses orderFrontRegardless so another app keeps keyboard focus (UX_SPEC 5.2).
    func showMainWindow(activate: Bool) {
        if let w = mainWindow {
            if w.isMiniaturized { w.deminiaturize(nil) }
            if activate { w.makeKeyAndOrderFront(nil); NSApp.activate(ignoringOtherApps: true) } else { w.orderFrontRegardless() }
        } else if live {
            openWindowAction?()
            if activate { NSApp.activate(ignoringOtherApps: true) }
        }
    }

    // MARK: snooze
    var snoozesLeft: Int? { ReminderPlanner.snoozesLeft(state: planner, settings: settings) }
    var snoozeLabel: String {
        let base = "Remind me in \(settings.snoozeMinutes) min"
        if let left = snoozesLeft { return left == 0 ? "No snoozes left today" : "\(base) · \(left) left" }
        return base
    }
    var canSnooze: Bool { (snoozesLeft ?? 1) > 0 }

    func snooze() {
        guard let next = ReminderPlanner.snooze(state: planner, now: clock(), settings: settings, calendar: cal) else { return }
        planner = next
        if live { Notifier.shared.cancelAll() }
    }
}
