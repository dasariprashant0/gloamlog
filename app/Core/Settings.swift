// Settings.swift - persisted preferences.
//
// API:
//   Settings                 Codable struct; Settings() = defaults (Strict, 16:55, Mon-Fri, 15 min snooze, 20 words, default template)
//   Settings.load(from:)     / save(to:)   via any KeyValueStore (UserDefaults conforms; MemoryStore for tests)
//   Settings.normalized()    clamps invalid values (call after editing; load already does)
//   Settings.isWorkday(weekday:) / defaultFolder / defaultTemplate / defaultCarryOverHeadings / minWordsRange
//   ReminderMode             .strict | .gentle
// v0.3 fields (replace the v0.2 `sections` array):
//   template            markdown pre-filled into a NEW day in the editor only (never written until the user edits)
//   carryOverHeadings   headings whose lines are carried to the next day (matched by MarkdownBody.normalizeHeading)
//   minWords            logged rule: words(body) >= minWords (1...500, default 20); carried-over task lines do not count
// Old settings JSON decodes tolerantly: an old `sections` array with no `template` becomes "## <title>" blocks;
// `carryOverSectionID` and other unknown keys are ignored.
// Weekdays use Calendar numbering: 1 = Sunday ... 7 = Saturday.
import Foundation

enum ReminderMode: String, Codable { case strict, gentle }
enum AppAppearance: String, Codable, CaseIterable { case system, light, dark }

protocol KeyValueStore: AnyObject {
    func data(forKey key: String) -> Data?
    func setData(_ data: Data?, forKey key: String)
}
extension UserDefaults: KeyValueStore {
    func setData(_ data: Data?, forKey key: String) { set(data as Any?, forKey: key) }
}
final class MemoryStore: KeyValueStore {
    var values: [String: Data] = [:]
    func data(forKey key: String) -> Data? { values[key] }
    func setData(_ data: Data?, forKey key: String) { values[key] = data }
}

struct Settings: Codable, Equatable {
    static let storageKey = "dailylog.settings.v2"   // unchanged on purpose: v0.2 settings are found and migrated
    static let snoozeChoices = [5, 10, 15, 20, 30]
    static let minWordsRange = 1...500
    static let defaultMinWords = 20
    static let defaultTemplate = "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next"
    static let defaultCarryOverHeadings = ["To do next", "Pending / blocked"]
    static var defaultFolder: URL { URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent("Gloamlog") }

    var reminderMinutes = 16 * 60 + 55            // minutes after midnight, local time
    var weekdays: Set<Int> = [2, 3, 4, 5, 6]      // Mon-Fri
    var mode: ReminderMode = .strict
    var snoozeMinutes = 15
    var storageFolder: URL = Settings.defaultFolder
    var launchAtLogin = true
    var template: String = Settings.defaultTemplate
    var carryOverHeadings: [String] = Settings.defaultCarryOverHeadings
    var minWords = Settings.defaultMinWords
    var onboarded = false
    // v0.4 (M1): real-world navigation and look. All optional with safe defaults, so older saves decode unchanged.
    var logStartDate: String? = nil               // "yyyy-MM-dd": days before it are never "missed"; nil = the first log in the folder
    var appearance: AppAppearance = .system
    var weekStart: Int? = nil                     // nil = follow the system calendar; else a Calendar weekday 1...7 (1 = Sunday)
    var catchUpWindowDays = 30                    // how far back "Catch up" looks
    static let catchUpWindowRange = 7...365
    var capture = CapturePrefs()                  // M2: quick capture (shortcut, timestamps, Jots heading)
    var closedAppReminders = true                 // M5a: macOS delivers the end-of-day reminder while Gloamlog is quit (this Mac only)

    init() {}

    enum CodingKeys: String, CodingKey {
        case reminderMinutes, weekdays, mode, snoozeMinutes, storageFolder, launchAtLogin
        case template, carryOverHeadings, minWords, onboarded
        case logStartDate, appearance, weekStart, catchUpWindowDays, capture, closedAppReminders
    }
    private enum LegacyKeys: String, CodingKey { case sections }
    private struct LegacySection: Decodable { var title: String }

    // Tolerant decoding: keys missing from older/newer saves fall back to defaults.
    init(from d: Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        reminderMinutes = try c.decodeIfPresent(Int.self, forKey: .reminderMinutes) ?? reminderMinutes
        weekdays = try c.decodeIfPresent(Set<Int>.self, forKey: .weekdays) ?? weekdays
        mode = try c.decodeIfPresent(ReminderMode.self, forKey: .mode) ?? mode
        snoozeMinutes = try c.decodeIfPresent(Int.self, forKey: .snoozeMinutes) ?? snoozeMinutes
        storageFolder = try c.decodeIfPresent(URL.self, forKey: .storageFolder) ?? storageFolder
        launchAtLogin = try c.decodeIfPresent(Bool.self, forKey: .launchAtLogin) ?? launchAtLogin
        carryOverHeadings = try c.decodeIfPresent([String].self, forKey: .carryOverHeadings) ?? carryOverHeadings
        minWords = try c.decodeIfPresent(Int.self, forKey: .minWords) ?? minWords
        onboarded = try c.decodeIfPresent(Bool.self, forKey: .onboarded) ?? onboarded
        logStartDate = try c.decodeIfPresent(String.self, forKey: .logStartDate)
        appearance = try c.decodeIfPresent(AppAppearance.self, forKey: .appearance) ?? appearance
        weekStart = try c.decodeIfPresent(Int.self, forKey: .weekStart)
        catchUpWindowDays = try c.decodeIfPresent(Int.self, forKey: .catchUpWindowDays) ?? catchUpWindowDays
        capture = try c.decodeIfPresent(CapturePrefs.self, forKey: .capture) ?? capture
        closedAppReminders = try c.decodeIfPresent(Bool.self, forKey: .closedAppReminders) ?? closedAppReminders
        if let t = try c.decodeIfPresent(String.self, forKey: .template) {
            template = t
        } else if let lc = try? d.container(keyedBy: LegacyKeys.self),
                  let old = try? lc.decodeIfPresent([LegacySection].self, forKey: .sections) {
            let titles = old.map { $0.title.dlTrimmed }.filter { !$0.isEmpty }
            if !titles.isEmpty { template = titles.map { "## \($0)" }.joined(separator: "\n\n") }
        }
    }

    func isWorkday(weekday: Int) -> Bool { weekdays.contains(weekday) }

    func normalized() -> Settings {
        var s = self
        s.reminderMinutes = min(max(reminderMinutes, 0), 24 * 60 - 1)
        s.weekdays = weekdays.filter { (1...7).contains($0) }
        if s.weekdays.isEmpty { s.weekdays = [2, 3, 4, 5, 6] }
        if !Settings.snoozeChoices.contains(snoozeMinutes) { s.snoozeMinutes = 15 }
        s.minWords = min(max(minWords, Settings.minWordsRange.lowerBound), Settings.minWordsRange.upperBound)
        s.carryOverHeadings = carryOverHeadings.map { $0.dlTrimmed }.filter { !$0.isEmpty }
        s.catchUpWindowDays = min(max(catchUpWindowDays, Settings.catchUpWindowRange.lowerBound), Settings.catchUpWindowRange.upperBound)
        if let d = logStartDate, !DayKey.isWellFormed(d) { s.logStartDate = nil }
        if let w = weekStart, !(1...7).contains(w) { s.weekStart = nil }
        return s
    }

    static func load(from store: KeyValueStore, key: String = storageKey) -> Settings {
        guard let d = store.data(forKey: key), let s = try? JSONDecoder().decode(Settings.self, from: d) else { return Settings() }
        return s.normalized()
    }
    func save(to store: KeyValueStore, key: String = Settings.storageKey) {
        store.setData(try? JSONEncoder().encode(normalized()), forKey: key)
    }
}
