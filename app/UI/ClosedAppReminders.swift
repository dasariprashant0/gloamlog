// ClosedAppReminders.swift - hands ReminderScheduler's plan to macOS so the end-of-day reminder arrives while Gloamlog is quit
// (M5a, docs/v1/ARCHITECTURE.md 5.E). ReminderPlanner stays the authority while the app runs; this keeps macOS's pending requests
// equal to the plan, answers the buttons on the notification, and stops the running app from adding a second notification.
//
// API:
//   ClosedAppReminders.shared.install(model:)         once, from AppModel.start(): subscribes and plans
//   ClosedAppReminders.shared.replan(force:)          cheap and safe to call any time (a plan equal to the last one applied does nothing)
//   ClosedAppReminders.shared.postInApp(_:center:)    Notifier.post goes through here: one notification a day
//   ClosedAppReminders.shared.handle(action:userInfo:model:)   Notifier's delegate: banner click, Open, Snooze, Skip today
//   AppModel.nextReminderText                         "Thu 8 Oct, 4:55 pm" for Settings > Reminders
// Re-plans at launch, app activation, wake, midnight, clock and time-zone changes, and a turn after the day states, the settings or
// the notification permission change (a save, skip, restore, new time or the switch). Logging or skipping today also cancels its
// request in the same moment. Nothing is added unless macOS says notifications are allowed. Every failure is silent: the running
// app's own reminder is the fallback.
import Foundation
import UserNotifications
import AppKit
import Combine

final class ClosedAppReminders {
    static let shared = ClosedAppReminders()
    private weak var model: AppModel?
    private var bag = Set<AnyCancellable>()
    private var runningSince: Date?
    private var again = false, againForced = false
    private var lastSignature: String?
    private var cancelledDay: String?
    /// A notification response in the first seconds of the process is what launched it.
    private let started = Date()

    // MARK: re-planning

    func install(model m: AppModel) {
        guard model == nil, m.live, Notifier.shared.center != nil else { return }
        model = m
        // @Published publishes before it stores, so the full re-plan runs a turn later (receive(on:)), when the new value is in place.
        Publishers.Merge3(m.$states.map { _ in () }, m.$settings.map { _ in () }, m.$notifState.removeDuplicates().map { _ in () })
            .receive(on: DispatchQueue.main)
            .sink { [weak self] _ in self?.replan() }
            .store(in: &bag)
        m.$states.sink { [weak self] states in self?.cancelIfDone(states) }.store(in: &bag)      // at once: the app may quit a moment later
        let nc = NotificationCenter.default, ws = NSWorkspace.shared.notificationCenter
        for n in [NSApplication.didBecomeActiveNotification, .NSCalendarDayChanged, .NSSystemClockDidChange, .NSSystemTimeZoneDidChange] {
            nc.addObserver(forName: n, object: nil, queue: .main) { [weak self] _ in self?.replan(force: true) }
        }
        ws.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { [weak self] _ in self?.replan(force: true) }
        replan(force: true)
    }

    /// `force` re-reads what macOS holds even when nothing about the plan changed (launch, activation, wake, midnight).
    func replan(force: Bool = false) {
        guard let m = model, let c = Notifier.shared.center else { return }
        if let since = runningSince, Date().timeIntervalSince(since) < 20 { again = true; againForced = againForced || force; return }
        let now = m.clock(), cal = m.cal, settings = m.settings, today = DayKey.string(now, cal)
        let planned = ReminderScheduler.plan(now: now, calendar: cal, settings: settings, states: m.states)
        let dayDone = m.states[today] == .logged || m.states[today] == .skipped
        let signature = "\(dayDone)|" + planned.map { "\($0.id)|\(ReminderScheduler.key($0.fire))|\($0.title)|\($0.body)" }.joined(separator: ";")
        guard force || signature != lastSignature else { return }
        runningSince = Date()
        Notifier.shared.refreshState { [weak self, weak m] state in                       // on the main queue
            guard let self = self else { return }
            if let m = m, m.notifState != state { m.notifState = state }
            guard state == .allowed else { self.finish(); return }                        // nothing could be delivered: the running app reminds
            c.getPendingNotificationRequests { pending in
                let d = ReminderScheduler.diff(planned: planned, pending: pending.map(Self.flatten))
                c.removePendingNotificationRequests(withIdentifiers: d.remove)
                for p in d.add { c.add(Self.request(p)) { _ in } }
                if dayDone || !settings.closedAppReminders {                              // today needs nothing left over: banner or snooze
                    let ids = [ReminderScheduler.id(for: today), ReminderScheduler.snoozeID(for: today)]
                    c.removePendingNotificationRequests(withIdentifiers: ids)
                    c.removeDeliveredNotifications(withIdentifiers: ids)
                }
                DispatchQueue.main.async { self.lastSignature = signature; self.finish() }
            }
        }
    }

    /// Logging or skipping today silences its pending request (and any banner or snooze) in the same moment, with one call that
    /// needs no answer, so a Quit right after the last word still leaves nothing to fire (M5-A2).
    private func cancelIfDone(_ states: [String: DayStatus]) {
        guard let m = model, let c = Notifier.shared.center else { return }
        let today = DayKey.string(m.clock(), m.cal)
        guard states[today] == .logged || states[today] == .skipped else { cancelledDay = nil; return }
        guard cancelledDay != today else { return }
        cancelledDay = today
        let ids = [ReminderScheduler.id(for: today), ReminderScheduler.snoozeID(for: today)]
        c.removePendingNotificationRequests(withIdentifiers: ids)
        c.removeDeliveredNotifications(withIdentifiers: ids)
    }

    private func finish() {
        runningSince = nil
        guard again else { return }
        let forced = againForced
        again = false; againForced = false
        replan(force: forced)
    }

    // MARK: one notification a day

    /// The running app's own reminder (Notifier.post). When macOS delivered the scheduled one (or a snooze) in the last two
    /// minutes, that notification stands; otherwise the scheduled ones are cancelled and this one replaces anything older.
    func postInApp(_ request: UNNotificationRequest, center c: UNUserNotificationCenter) {
        let day = model.map { DayKey.string($0.clock(), $0.cal) } ?? DayKey.string(Date(), Calendar.current)
        let ids = [ReminderScheduler.id(for: day), ReminderScheduler.snoozeID(for: day)]
        c.getDeliveredNotifications { delivered in
            if delivered.contains(where: { ids.contains($0.request.identifier) && Date().timeIntervalSince($0.date) < 120 }) { return }
            c.removePendingNotificationRequests(withIdentifiers: ids)
            c.removeDeliveredNotifications(withIdentifiers: ids)
            c.add(request) { _ in }
        }
    }

    // MARK: the banner and its buttons

    /// A click on the banner (also the one that launched Gloamlog), Open, Snooze or Skip today.
    func handle(action: String, userInfo: [AnyHashable: Any], model m: AppModel) {
        let day = (userInfo["day"] as? String).flatMap { DayKey.isWellFormed($0) ? $0 : nil } ?? m.today
        guard day == m.today else { open(m, day: day); return }                          // an old banner: show the day it is about
        switch action {
        case "snooze" where m.canSnooze: snooze(m)
        case "skip": skip(m, day: day)
        default:                                                                         // the banner, Open, and Snooze with none left (Strict)
            if !m.planner.firstFired { m.planner.firstFired = true; m.planner.lastFire = m.clock() }   // they have seen it: no second one 2 min after a launch
            open(m, day: day)
        }
    }

    private func open(_ m: AppModel, day: String) {
        m.showMainWindow(activate: true)
        m.openDay(day, focus: true)
    }

    /// Snooze counts toward Strict's two a day and holds the in-app reminder (as the menu-bar button does); the one-off request
    /// is what brings it back if Gloamlog is closed by then.
    private func snooze(_ m: AppModel) {
        m.snooze()
        hideIfLaunched(m)
        guard m.settings.closedAppReminders, let c = Notifier.shared.center, let until = m.planner.snoozedUntil else { return }
        let now = m.clock(), cal = m.cal
        let minutes = cal.component(.hour, from: until) * 60 + cal.component(.minute, from: until)
        guard until > now, cal.isDate(until, inSameDayAs: now), minutes < ReminderPlanner.cutoffMinutes else { return }
        let text = ReminderScheduler.copy(at: until, calendar: cal)
        c.add(UNNotificationRequest(identifier: ReminderScheduler.snoozeID(for: m.today),
                                    content: Self.content(day: m.today, title: text.title, body: text.body),
                                    trigger: UNTimeIntervalNotificationTrigger(timeInterval: until.timeIntervalSince(now), repeats: false))) { _ in }
    }

    /// The existing skip rules (LogStore.skip): a day with writing is refused, so the page opens with the reason instead.
    private func skip(_ m: AppModel, day: String) {
        m.skip(day: day, through: nil, reason: "") { [weak self] failure in
            if let f = failure { m.pageNotice = f.text; self?.open(m, day: day) }
            else { self?.hideIfLaunched(m) }
        }
    }

    /// Snooze and Skip today are not foreground actions. When one launched Gloamlog, the window it opened is put away again.
    private func hideIfLaunched(_ m: AppModel) {
        guard Date().timeIntervalSince(started) < 8 else { return }
        for delay in [0.2, 1.0, 2.5] { DispatchQueue.main.asyncAfter(deadline: .now() + delay) { m.mainWindow?.orderOut(nil) } }
    }

    // MARK: requests

    private static func content(day: String, title: String, body: String, category: String = ReminderScheduler.category) -> UNMutableNotificationContent {
        let c = UNMutableNotificationContent()
        c.title = title; c.body = body; c.categoryIdentifier = category
        c.threadIdentifier = "gloamlog.reminders"; c.userInfo = ["day": day]
        return c
    }

    private static func request(_ p: PlannedReminder) -> UNNotificationRequest {
        UNNotificationRequest(identifier: p.id, content: content(day: p.day, title: p.title, body: p.body, category: p.category),
                              trigger: UNCalendarNotificationTrigger(dateMatching: p.fire, repeats: false))
    }

    private static func flatten(_ r: UNNotificationRequest) -> PendingReminder {
        PendingReminder(id: r.identifier, fire: (r.trigger as? UNCalendarNotificationTrigger)?.dateComponents,
                        title: r.content.title, body: r.content.body, category: r.content.categoryIdentifier)
    }
}

extension AppModel {
    /// "Thu 8 Oct, 4:55 pm": the first reminder of the plan macOS holds ("none planned" when nothing is left to remind about).
    var nextReminderText: String {
        let plan = ReminderScheduler.upcoming(now: now, calendar: cal, settings: settings, states: states)
        return ReminderScheduler.next(plan, now: now, calendar: cal).map { ReminderScheduler.label($0, calendar: cal) } ?? "none planned"
    }
}
