// AppModel+Settings.swift - owned by the Settings stream (M1 tasks S1-S5).
// openSettings(pane:) is the one door to the Settings window (⌘, menu item, menu-bar popover, sidebar gear, banners).
// The effects of a settings change that the model itself does not carry out (app-wide appearance, re-planning the
// reminder, refreshing history views) are subscribed here, so no other file has to call anything.
import SwiftUI
import AppKit
import Combine
import ObjectiveC

enum SettingsPane: String, CaseIterable, Identifiable {
    case general, reminders, page, storage, shortcuts, sources, about
    var id: String { rawValue }
}

extension AppModel {
    /// The calendar the whole app uses. `Settings.weekStart` (nil = follow the system) sets the first weekday, so the
    /// month grid, Week review and heatmap all start the week where the user wants. AppModel.cal must be built from this.
    static func makeCalendar(weekStart: Int?) -> Calendar {
        var c = Calendar.current
        if let w = weekStart, (1...7).contains(w) { c.firstWeekday = w }
        return c
    }

    /// Opens the Settings window in front, optionally on a pane (nil = the pane it was last on).
    /// Callable from the sidebar gear, menus, banners and the menu-bar popover.
    func openSettings(pane: SettingsPane? = nil) {
        installSettingsEffects()
        SettingsWindowController.shared.show(model: self, pane: pane)
    }

    /// "Run setup again…" (Settings > About): the setup sheet over the main window. Pages and settings stay as they are
    /// until a step is changed.
    func runSetupAgain() {
        showMainWindow(activate: true)
        sheet = .onboarding
    }

    // MARK: preferences that are not part of `Settings`
    static let showMenuBarKey = "showMenuBar"
    /// The menu-bar item. Lives in the model's defaults (the real app's are `UserDefaults.standard`, which the scene's
    /// `@AppStorage("showMenuBar")` observes), so a test suite never touches the owner's real preference.
    var showMenuBarItem: Bool {
        get { defaults.object(forKey: AppModel.showMenuBarKey) as? Bool ?? true }
        set { objectWillChange.send(); defaults.set(newValue, forKey: AppModel.showMenuBarKey) }
    }

    // MARK: reminder summary
    var nextReminder: Date? {
        ReminderSchedule.next(now: now, calendar: cal, weekdays: settings.weekdays, reminderMinutes: settings.reminderMinutes,
                              todayDone: todayStatus == .logged || todayStatus == .skipped)
    }
    /// "Tomorrow, 4:55 pm": the short form for the menu-bar popover. Settings > Reminders shows `nextReminderText` ("Thu 8 Oct, 4:55 pm").
    var nextReminderLabel: String {
        guard let d = nextReminder else { return "none planned" }
        return ReminderSchedule.label(for: d, now: now, calendar: cal)
    }

    // MARK: effects
    /// Subscribes to `settings` once per model. Called from the app's init, from `openSettings` and by tests.
    func installSettingsEffects() {
        guard objc_getAssociatedObject(self, &SettingsEffects.associationKey) == nil else { return }
        objc_setAssociatedObject(self, &SettingsEffects.associationKey, SettingsEffects(model: self), .OBJC_ASSOCIATION_RETAIN_NONATOMIC)
    }

    /// What a settings change must do beyond what AppModel.settingsChanged does (it saves, rebuilds `cal` on a new week
    /// start and recomputes streak, heatmap and the catch-up list when the log start, window or calendar changed).
    fileprivate func applySettingsChange(from old: Settings, to new: Settings) {
        // A later time or different days must be able to fire again today, even if the reminder already fired.
        if old.reminderMinutes != new.reminderMinutes || old.weekdays != new.weekdays { replanReminder() }
        // The notification's Snooze button carries the length in its title.
        if old.snoozeMinutes != new.snoozeMinutes, live { Notifier.shared.setup(model: self) }
    }

    /// Forgets today's firing history (not the snoozes used), so the new time or days apply from now.
    func replanReminder() {
        var p = planner
        p.firstFired = false; p.snoozedUntil = nil; p.lastFire = nil; p.hiddenSince = nil; p.reopenCount = 0
        planner = p
        if live { Notifier.shared.cancelAll() }
    }

    /// "Reset to defaults" for one pane. Pages, the log folder and the backups are never touched.
    func resetSettings(_ pane: SettingsPane) {
        let d = Settings()
        switch pane {
        case .general:
            showMenuBarItem = true
            if loginEnabled { setLogin(false) }
        case .reminders, .page, .storage, .shortcuts, .sources, .about: break
        }
        var s = settings                                  // after setLogin, which also edits `settings`
        switch pane {
        case .general:
            s.appearance = d.appearance; s.weekStart = d.weekStart
        case .reminders:
            s.reminderMinutes = d.reminderMinutes; s.weekdays = d.weekdays; s.mode = d.mode; s.snoozeMinutes = d.snoozeMinutes
            s.closedAppReminders = d.closedAppReminders
        case .page:
            s.minWords = d.minWords; s.template = d.template; s.carryOverHeadings = d.carryOverHeadings
            s.catchUpWindowDays = d.catchUpWindowDays; s.logStartDate = d.logStartDate
        case .storage, .shortcuts, .sources, .about: return
        }
        if s != settings { settings = s }
    }
}

/// Holds the subscription for one model; released with it (an associated object, so AppModel needs no new property).
final class SettingsEffects {
    fileprivate static var associationKey: UInt8 = 0
    private var bag = Set<AnyCancellable>()

    init(model: AppModel) {
        Self.apply(model.settings.appearance)
        var last = model.settings
        // `$settings` fires before the value is stored: appearance uses the value it is given, the rest runs next turn.
        model.$settings.dropFirst().sink { [weak model] new in
            let old = last; last = new
            if old.appearance != new.appearance { Self.apply(new.appearance) }
            DispatchQueue.main.async { model?.applySettingsChange(from: old, to: new) }
        }.store(in: &bag)
    }

    /// System follows macOS; Light and Dark pin every window, the menu-bar popover and the page editor.
    static func apply(_ a: AppAppearance) {
        let app = NSApplication.shared
        switch a {
        case .system: app.appearance = nil
        case .light: app.appearance = NSAppearance(named: .aqua)
        case .dark: app.appearance = NSAppearance(named: .darkAqua)
        }
    }
}

/// When the next end-of-day reminder is due. Pure; the planner itself decides what happens at that time.
enum ReminderSchedule {
    /// The first reminder time after `now` on a scheduled weekday. Today counts only while its time is still ahead and
    /// the day is not yet logged or skipped.
    static func next(now: Date, calendar: Calendar, weekdays: Set<Int>, reminderMinutes: Int, todayDone: Bool) -> Date? {
        let start = calendar.startOfDay(for: now)
        for offset in 0...7 {
            guard let day = calendar.date(byAdding: .day, value: offset, to: start),
                  weekdays.contains(calendar.component(.weekday, from: day)),
                  let at = calendar.date(bySettingHour: reminderMinutes / 60, minute: reminderMinutes % 60, second: 0, of: day)
            else { continue }
            if offset == 0 && (todayDone || at <= now) { continue }
            return at
        }
        return nil
    }

    /// "Today, 4:55 pm" / "Tomorrow, 4:55 pm" / "Mon 12 Oct, 4:55 pm".
    static func label(for date: Date, now: Date, calendar: Calendar) -> String {
        let time = Fmt.time(date)
        if calendar.isDate(date, inSameDayAs: now) { return "Today, \(time)" }
        if let t = calendar.date(byAdding: .day, value: 1, to: now), calendar.isDate(date, inSameDayAs: t) { return "Tomorrow, \(time)" }
        return "\(DayKey.format(DayKey.string(date, calendar), "EEE d MMM", calendar)), \(time)"
    }
}
