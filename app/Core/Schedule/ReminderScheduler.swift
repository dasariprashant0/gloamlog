// ReminderScheduler.swift - the plan for end-of-day reminders that macOS itself delivers while Gloamlog is closed
// (M5a, docs/v1/ARCHITECTURE.md 5.E). Pure (Foundation only): the same inputs give the same plan, and a plan that has been
// applied diffs to nothing. ReminderPlanner stays the authority while the app runs; app/UI/ClosedAppReminders.swift applies this.
//
// API:
//   ReminderScheduler.upcoming(now:calendar:settings:states:horizon:) -> [PlannedReminder]   what the next 14 scheduled days need
//   ReminderScheduler.plan(...)                same, but [] while Settings.closedAppReminders is off: what macOS should hold
//   ReminderScheduler.diff(planned:pending:)   -> (add, remove): the least change; only ids under "gloamlog.reminder." are touched
//   ReminderScheduler.next(_:now:calendar:)    the first fire time after now;  label(_:calendar:) -> "Thu 8 Oct, 4:55 pm"
//   ReminderScheduler.copy(at:calendar:)       title and body (UX_FLOWS 5.3): never page text, a streak or a name
//   ReminderScheduler.id(for:) / snoozeID(for:)   "gloamlog.reminder.2026-10-09" / "gloamlog.snooze.2026-10-09"
// Rules: from today on, the next 14 days whose weekday is in Settings.weekdays. A day gets no reminder when it is logged or
// skipped (a skip range may reach future days), is before the log start date, or is today with its time already past. A
// started day (a few words, or only jots) is not logged, so it keeps its reminder.
import Foundation

/// One end-of-day request for macOS: `fire` is year, month, day, hour and minute, which the system resolves in the Mac's zone.
struct PlannedReminder: Equatable {
    var id: String
    var day: String
    var fire: DateComponents
    var title: String
    var body: String
    var category = ReminderScheduler.category      // the Open, Snooze and Skip today buttons
}

/// What the notification center holds for us (a UNNotificationRequest flattened so the diff stays pure).
struct PendingReminder: Equatable {
    var id: String
    var fire: DateComponents?
    var title: String
    var body: String
    var category = ReminderScheduler.category
}

enum ReminderScheduler {
    static let horizon = 14
    static let idPrefix = "gloamlog.reminder."
    static let snoozePrefix = "gloamlog.snooze."
    /// The notification category with the Open, Snooze and Skip today buttons (registered by Notifier.setup).
    static let category = "reminder"

    static func id(for day: String) -> String { idPrefix + day }
    static func snoozeID(for day: String) -> String { snoozePrefix + day }

    /// The reminders the next `horizon` scheduled days need, whether or not macOS is asked to hold them.
    static func upcoming(now: Date, calendar: Calendar, settings: Settings, states: [String: DayStatus],
                         horizon: Int = ReminderScheduler.horizon) -> [PlannedReminder] {
        var out = [PlannedReminder]()
        let start = calendar.startOfDay(for: now)
        var scheduled = 0
        for offset in 0..<(horizon * 7 + 7) {              // a scheduled weekday comes round every week, so this always reaches `horizon`
            if scheduled == horizon { break }
            guard let date = calendar.date(byAdding: .day, value: offset, to: start),
                  settings.weekdays.contains(calendar.component(.weekday, from: date)) else { continue }
            scheduled += 1
            let day = DayKey.string(date, calendar)
            var fire = calendar.dateComponents([.year, .month, .day], from: date)
            fire.hour = settings.reminderMinutes / 60; fire.minute = settings.reminderMinutes % 60
            let at = calendar.date(from: fire) ?? date
            if at <= now { continue }                                                  // today, and its time has passed
            if let s = states[day], s == .logged || s == .skipped { continue }
            if let s = settings.logStartDate, day < s { continue }
            let text = copy(at: at, calendar: calendar)
            out.append(PlannedReminder(id: id(for: day), day: day, fire: fire, title: text.title, body: text.body))
        }
        return out
    }

    /// What macOS should be holding: `upcoming`, or nothing while "Remind me when Gloamlog is closed" is off.
    static func plan(now: Date, calendar: Calendar, settings: Settings, states: [String: DayStatus],
                     horizon: Int = ReminderScheduler.horizon) -> [PlannedReminder] {
        settings.closedAppReminders ? upcoming(now: now, calendar: calendar, settings: settings, states: states, horizon: horizon) : []
    }

    /// The least change that makes `pending` equal `planned`. A request with the same id is replaced when its time or text differs.
    static func diff(planned: [PlannedReminder], pending: [PendingReminder]) -> (add: [PlannedReminder], remove: [String]) {
        let mine = pending.filter { $0.id.hasPrefix(idPrefix) }
        var have = [String: PendingReminder]()
        for p in mine { have[p.id] = p }
        let wanted = Set(planned.map { $0.id })
        let add = planned.filter { p in
            guard let h = have[p.id] else { return true }
            return h.title != p.title || h.body != p.body || h.category != p.category || key(h.fire) != key(p.fire)
        }
        return (add, mine.map { $0.id }.filter { !wanted.contains($0) })
    }

    /// The first planned fire time after `now`.
    static func next(_ plan: [PlannedReminder], now: Date, calendar: Calendar) -> Date? {
        plan.compactMap { calendar.date(from: $0.fire) }.filter { $0 > now }.min()
    }

    /// "Thu 8 Oct, 4:55 pm" (the time follows the Mac's 12 or 24 hour setting).
    static func label(_ date: Date, calendar: Calendar) -> String {
        "\(DayKey.format(DayKey.string(date, calendar), "EEE d MMM", calendar)), \(timeText(date, calendar))"
    }

    /// UX_FLOWS 5.3: informational, never an order.
    static func copy(at fire: Date, calendar: Calendar) -> (title: String, body: String) {
        ("Today isn't logged yet", "It's \(timeText(fire, calendar)). Open Gloamlog when you're ready.")
    }

    /// Compares only the five fields a plan sets (the system may hand back a calendar or zone as well).
    static func key(_ c: DateComponents?) -> String {
        guard let c = c else { return "" }
        return "\(c.year ?? 0)-\(c.month ?? 0)-\(c.day ?? 0) \(c.hour ?? 0):\(c.minute ?? 0)"
    }

    private static func timeText(_ date: Date, _ calendar: Calendar) -> String {
        let f = DateFormatter()
        f.locale = calendar.locale ?? Locale.current; f.timeZone = calendar.timeZone
        f.dateStyle = .none; f.timeStyle = .short
        return f.string(from: date).lowercased()
    }
}
