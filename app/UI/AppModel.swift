// AppModel.swift - the single ObservableObject that owns LogStore, Settings, planner state, navigation and the open page.
// Everything runs on the main thread by convention (no @MainActor: closures from Timer/NotificationCenter stay simple).
// Extensions: +Editor (page switching, flush, bridge delegate), +Day (skip, carry-over, versions), +Reminders (timer,
// notifications, window), +Storage (folder, login item), +Browse (search, weekly review), +Calendar (any-date navigation,
// month grid, Go to date), +CatchUp (the unlogged-days list, the session, batch skip), +Capture (quick capture: the journal,
// the writer, the global shortcut and the Jot panel).
import SwiftUI
import Combine
import AppKit

enum Destination: Hashable { case day(String), catchUp, week, search }

enum ActiveSheet: Identifiable, Equatable {
    case skip(String), skipMany([String]), onboarding, whatsNew, restore(String)
    var id: String {
        switch self {
        case .skip(let d): return "skip-\(d)"
        case .skipMany(let d): return "skipmany-\(d.joined(separator: ","))"
        case .onboarding: return "onboarding"
        case .whatsNew: return "whatsnew"
        case .restore(let d): return "restore-\(d)"
        }
    }
}

enum FolderProblem: Equatable {
    case missing(String), notWritable(String)
    var path: String { switch self { case .missing(let p), .notWritable(let p): return p } }
}

enum NotifState { case unknown, allowed, denied }

final class AppModel: ObservableObject {
    static let shared = AppModel()
    static let plannerKey = "dailylog.planner.v1"
    static let whatsNewKey = "dailylog.whatsNew.0.6"
    static let catchNoticeKey = "dailylog.catchNoticeDay"
    /// Safety copies live outside the storage folder so they never sync with the logs.
    static var defaultBackupDir: URL {
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/Gloamlog/backups")
    }
    /// Autosave overwrites while the newest backup is younger than this do not copy again, so the 10 slots span a long session.
    static let backupInterval: TimeInterval = 120

    /// The calendar the whole app uses; rebuilt from `AppModel.makeCalendar` when Settings changes the week start.
    var cal = Calendar.current
    let defaults: UserDefaults
    let live: Bool                       // false in the snapshot harness: no timers, web view, notifications or login item
    var clock: () -> Date
    let backupDir: URL
    var store: LogStore { didSet { storeReplaced() } }
    /// Quick capture (M2), see AppModel+Capture.swift. The journal lives beside the safety copies, outside the log folder.
    let captureDir: URL
    var journal: CaptureJournal
    var writer: DayWriter!
    /// Notes captured and not yet in a page's file (they are safe in the journal). Shown in the sidebar and the popover.
    @Published var notesWaiting = 0
    /// Why the Jot shortcut is not working (Settings > Shortcuts shows it), nil when it is.
    @Published var hotKeyFailure: HotKeyFailure?
    /// The real app's global shortcut; nil in every harness, so a check can never take the owner's real combination.
    var hotKeys: HotKeyCenter?
    lazy var jotPanel: JotPanelController = JotPanelController(model: self)
    /// The Review screen's state and every copy and export of it (M3), see ReviewController.swift.
    lazy var review = ReviewController(model: self)

    @Published var now: Date
    @Published var settings: Settings { didSet { settingsChanged(old: oldValue) } }
    @Published var states: [String: DayStatus] = [:]
    @Published var skipReasons: [String: String] = [:]
    @Published var folderProblem: FolderProblem?
    @Published var streak = StreakResult(current: 0, best: 0)
    @Published var heat: [[HeatCell]] = []
    /// Unlogged working days inside the catch-up window (oldest first). One list feeds the sidebar badge, the screen, the Today
    /// notice and the session; it is recomputed from `states` whenever a page is saved, skipped or restored.
    @Published var catchUpDays: [String] = []
    @Published var selection: Destination
    @Published var editor: DayEditor!
    @Published var sheet: ActiveSheet?
    @Published var planner: PlannerState { didSet { persistPlanner() } }
    @Published var notifState: NotifState = .unknown
    @Published var rolloverDismissed = false
    @Published var nagDismissedDay = ""
    @Published var catchNoticeDay = ""           // the day the Today notice was last dismissed (once a day)
    @Published var editorProblem: String?
    @Published var pageNotice: String?          // transient: an image that could not be added, etc.
    /// Pages the user has left whose last edits could not be written (folder unavailable). Kept in memory and retried, so
    /// leaving a page never discards unsaved text; the quit path asks before giving them up.
    @Published var orphans: [DayEditor] = []
    // navigation
    @Published var shownMonth: YearMonth         // the month the sidebar calendar shows (follows the open page)
    @Published var stripDay: String              // a day in the week the sidebar's one-week strip shows (short windows)
    @Published var goToDateOpen = false
    // catch up
    @Published var session: CatchSession?
    @Published var sessionResult: SessionResult?
    @Published var skipUndo: SkipUndo?
    @Published var catchRange: ClosedRange<String>?   // "Catch up this week" scopes the screen to those days
    @Published var catchSelection: Set<String> = []   // rows picked on the Catch up screen (⇧-click, ⌘-click)
    // From yesterday
    @Published var carryCard: CarryCard?
    @Published var carryUndoBody: String?
    // search
    @Published var searchText = ""
    @Published var searchHeading: String?
    @Published var searchHits: [SearchHit] = []
    @Published var searchCursor = 0
    @Published var searchFocusTick = 0
    var returnSelection: Destination
    // weekly review
    @Published var weekDate: Date
    @Published var week: WeekSummary?
    @Published var weekGrouping: WeekGrouping = .day
    @Published var copiedFlash = false
    // storage
    @Published var storageSummary: FolderSummary?
    @Published var storageResult: StorageResult?
    @Published var loginMessage: String?

    var cancellables = Set<AnyCancellable>()
    var timer: Timer?
    var screensAsleep = false
    var openWindowAction: (() -> Void)?
    var bridgeStorage: EditorBridge?
    var retiredMarkdown: String?         // the previous page's final text: a late change event equal to it is not the new page's
    /// What the log folder looked like at the last read (names, times, sizes of the day files). The 30 s tick and app
    /// activation re-read the pages only when this changes.
    var lastStamp: String?
    private(set) var reloadCount = 0

    init(defaults: UserDefaults = .standard, backupDir: URL = AppModel.defaultBackupDir, live: Bool = true,
         clock: @escaping () -> Date = Date.init, captureDir: URL? = nil) {
        self.defaults = defaults; self.live = live; self.clock = clock; self.backupDir = backupDir
        // The capture journal sits next to the safety copies (~/Library/Application Support/Gloamlog/capture), so a harness that
        // gives its own backup folder gets its own journal and can never read or deliver the owner's real notes.
        let capture = captureDir ?? backupDir.deletingLastPathComponent().appendingPathComponent("capture", isDirectory: true)
        self.captureDir = capture
        journal = CaptureJournal(dir: capture)
        // Once-only carry-over from the app's earlier name. Only for the real app (the standard defaults), never for the
        // test/snapshot harnesses, which pass their own throwaway suites and must not touch the user's real data.
        if live && defaults === UserDefaults.standard { LegacyMigration.run(into: defaults) }
        var s = Settings.load(from: defaults)
        let isV1 = defaults.object(forKey: Settings.storageKey) == nil && defaults.object(forKey: "remindMinutes") != nil
        if isV1 { s.reminderMinutes = defaults.integer(forKey: "remindMinutes"); s.onboarded = true }
        if !s.onboarded { s.launchAtLogin = false }     // login item is opt-in: never registered silently
        let loaded = s.normalized()
        settings = loaded
        cal = AppModel.makeCalendar(weekStart: loaded.weekStart)
        let n = clock()
        now = n
        let today = DayKey.string(n, cal)
        selection = .day(today); returnSelection = .day(today); weekDate = n
        shownMonth = YearMonth(day: today); stripDay = today
        catchNoticeDay = defaults.string(forKey: AppModel.catchNoticeKey) ?? ""
        store = AppModel.makeStore(dir: s.storageFolder, backupDir: backupDir)
        if let d = defaults.data(forKey: AppModel.plannerKey), let p = try? JSONDecoder().decode(PlannerState.self, from: d) {
            planner = PlannerState.atLaunch(previous: p, now: n, calendar: cal)
        } else { planner = PlannerState.atLaunch(previous: nil, now: n, calendar: cal) }
        if !live { planner.graceUntil = nil }
        installCapture()
        reload()
        editor = DayEditor(day: today, model: self)
        refreshCarry()
        bindSearch()
        if live { decideFirstRun(isV1: isV1); startCapture(realApp: defaults === UserDefaults.standard) }
    }

    static func makeStore(dir: URL, backupDir: URL) -> LogStore {
        LogStore(dir: dir, backupDir: backupDir, minBackupInterval: backupInterval)
    }

    private func decideFirstRun(isV1: Bool) {
        if !settings.onboarded { sheet = .onboarding }
        else if defaults.object(forKey: AppModel.whatsNewKey) == nil { sheet = .whatsNew }
    }

    // MARK: derived
    var today: String { DayKey.string(now, cal) }
    /// The day before which nothing counts as unlogged: the log start date (Settings), else the first page ever written.
    var since: String? { Status.effectiveSince(setting: settings.logStartDate, states: states) }
    var todayStatus: DayStatus { status(of: today) }
    var snoozed: Bool { if let u = planner.snoozedUntil { return now < u }; return false }
    var isDue: Bool {
        ReminderPlanner.isPending(now: now, calendar: cal, settings: settings, todayStatus: todayStatus) && !snoozed
    }
    func status(of day: String) -> DayStatus {
        Status.resolve(day: day, fileState: states[day], now: now, calendar: cal, weekdays: settings.weekdays, since: since)
    }
    func isWorkday(_ day: String) -> Bool { settings.weekdays.contains(DayKey.weekday(day, cal)) }
    func longDate(_ day: String) -> String { DayKey.format(day, "EEEE, d MMMM yyyy", cal) }
    func shortDate(_ day: String) -> String { DayKey.format(day, "EEE d MMM", cal) }
    func spokenDate(_ day: String) -> String { DayKey.format(day, "EEEE d MMMM", cal) }
    var reminderLabel: String { Fmt.clock(minutes: settings.reminderMinutes, calendar: cal, now: now) }
    var folderDisplay: String { (settings.storageFolder.path as NSString).abbreviatingWithTildeInPath }
    var minWords: Int { settings.minWords }

    /// Pages with a file (logged, started or skipped), except today, newest first: the sidebar's Recent list.
    var historyDays: [String] {
        var set = Set(states.keys)
        set.remove(today)
        return set.sorted(by: >)
    }

    // MARK: data
    func reload() {
        reloadCount += 1
        let hadProblem = folderProblem != nil
        do { try store.checkFolder(); folderProblem = nil }
        catch LogError.folderMissing(let p) { folderProblem = .missing(p) }
        catch LogError.folderNotWritable(let p) { folderProblem = .notWritable(p) }
        catch { }
        if hadProblem && folderProblem == nil { replayCaptureSoon() }        // the folder is back: deliver the notes kept meanwhile
        let st = (try? store.fileStates(minWords: settings.minWords)) ?? [:]
        states = st
        var reasons = [String: String]()
        for (d, s) in st where s == .skipped { if let e = (try? store.load(d)) ?? nil { reasons[d] = e.skipReason } }
        skipReasons = reasons
        lastStamp = currentStamp()
        recompute()
    }

    /// The cheap check behind the 30 s tick and app activation: pages are read again only when a day file was added,
    /// removed or changed (or the folder's health changed), never on a timer alone.
    func reloadIfFolderChanged() {
        var problem: FolderProblem?
        do { try store.checkFolder() }
        catch LogError.folderMissing(let p) { problem = .missing(p) }
        catch LogError.folderNotWritable(let p) { problem = .notWritable(p) }
        catch { }
        let stamp = currentStamp()
        if problem != folderProblem || stamp == nil || stamp != lastStamp { reload() }
        if notesWaiting > 0 { replayCaptureSoon() }          // a note that is waiting is retried on every tick, even if nobody noticed the folder blink
    }

    /// Streak, heatmap and the catch-up list from the in-memory states (no disk). Also used after a single day's save.
    func recompute() {
        streak = Streak.compute(states: states, now: now, calendar: cal, weekdays: settings.weekdays, since: since)
        heat = Heatmap.weeks(states: states, now: now, calendar: cal, weekdays: settings.weekdays, since: settings.logStartDate)
        catchUpDays = CatchUp.missing(states: states, now: now, calendar: cal, weekdays: settings.weekdays,
                                      logStart: settings.logStartDate, windowDays: settings.catchUpWindowDays)
        if selection == .week { refreshWeek() }
    }

    /// One day was written: update just that day's state, not every file.
    func didSave(day: String) {
        let doc = (try? store.load(day)) ?? nil
        let st = doc?.status(minWords: settings.minWords)
        if let s = st, s != .missed { pinLogStart(before: day); states[day] = s } else { states[day] = nil }
        if st == .skipped { skipReasons[day] = doc?.skipReason ?? "" } else { skipReasons[day] = nil }
        folderProblem = nil
        recompute()
        lastStamp = currentStamp()        // our own write is not an outside change
        if live && todayStatus == .logged { Notifier.shared.cancelAll() }
        if st == .logged && day == today && live { UIAnnounce.say("Logged for today") }
    }

    /// Writing a day earlier than the first page must not turn the gap into a wall of unlogged days: the log start is
    /// pinned where it was before this page (Settings, Page can move it later). Only while the user never set one.
    func pinLogStart(before day: String) {
        guard settings.logStartDate == nil, let first = states.keys.min(), day < first else { return }
        settings.logStartDate = first
    }

    func settingsChanged(old: Settings) {
        let words = min(max(settings.minWords, Settings.minWordsRange.lowerBound), Settings.minWordsRange.upperBound)
        if words != settings.minWords { settings.minWords = words; return }          // re-enters with a valid value
        settings.save(to: defaults)
        // Pinning the log start (a backfill) is not a user setting change: it never delays a reminder.
        var sameStart = old; sameStart.logStartDate = settings.logStartDate
        if live && sameStart != settings { planner = planner.suppressed(until: clock().addingTimeInterval(ReminderPlanner.graceAfterSettingsChange)) }
        if old.weekStart != settings.weekStart { cal = AppModel.makeCalendar(weekStart: settings.weekStart) }
        if old.weekdays != settings.weekdays || old.reminderMinutes != settings.reminderMinutes || old.minWords != settings.minWords { reload() }
        else if old.logStartDate != settings.logStartDate || old.catchUpWindowDays != settings.catchUpWindowDays || old.weekStart != settings.weekStart { recompute() }
        if old.carryOverHeadings != settings.carryOverHeadings { refreshCarry() }
        if old.capture.hotKey != settings.capture.hotKey { applyHotKey() }
        if old.capture.jotsHeading != settings.capture.jotsHeading || old.capture.jotsCountTowardLogged != settings.capture.jotsCountTowardLogged {
            applyJotsRules(); reload()                    // what counts as a logged day changed: every status follows
        }
        // A page nobody has touched follows the template; a page with writing never does. A day with only jots counts as
        // untouched too (its template is shown above the jots, editor-only).
        if old.template != settings.template, let e = editor, !e.userEdited, !e.isSkipped, case .day = selection,
           e.diskBody == nil || MarkdownBody.isJotsOnly(e.diskBody ?? "", jotsHeading: settings.capture.jotsHeading) {
            openDay(e.day, force: true)
        }
    }

    func persistPlanner() { defaults.set(try? JSONEncoder().encode(planner), forKey: AppModel.plannerKey) }

    // MARK: navigation
    /// Every route in the app (sidebar, toolbar, calendar, menus) goes through here.
    func select(_ d: Destination) {
        switch d {
        case .day(let k): openDay(k)
        case .catchUp:
            session = nil; sessionResult = nil; catchRange = nil; catchSelection = []
            selection = .catchUp
            reload()                                         // the one screen where a stale list would mislead
            flushEditor { [weak self] in self?.recompute() } // include text typed a moment ago
        case .week:
            session = nil; sessionResult = nil
            // The review of the page you are on (its own week); from anywhere else, this week.
            if case .day(let k) = selection, let d = DayKey.date(k, cal) { weekDate = d } else { weekDate = clock() }
            selection = .week
            refreshWeek()
            flushEditor { [weak self] in self?.refreshWeek() }       // include text typed a moment ago
        case .search: selection = .search
        }
        if case .catchUp = d {} else { sessionResult = nil; catchRange = nil }
        if case .search = d {} else if !searchText.isEmpty { searchText = "" }
    }
    func openToday() { select(.day(today)) }

    /// Previous/next CALENDAR day (⌘[ / ⌘]), gaps included; previous/next week on the review. ⌘] on today does nothing.
    func step(_ dir: Int) {
        switch selection {
        case .week:
            review.step(dir)                                 // the shown period (week, month, year or range) moves; it never goes into the future
        case .day(let cur):
            guard let next = DayKey.adding(cur, dir, cal), canOpen(next) else { return }
            openDay(next)
        default: return
        }
    }
}

/// Midnight handling: called by the timer and by day/clock/wake notifications. It moves the clock and the day over; it
/// never reads the log folder (see `reloadIfFolderChanged`).
extension AppModel {
    func refreshClock() {
        let old = today
        now = clock()
        if today != old {
            rolloverDismissed = false
            planner = PlannerState(day: today, graceUntil: planner.graceUntil)
            recompute()                                 // every status is relative to today
            refreshCarry()
        }
    }
}
