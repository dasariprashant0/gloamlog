// Models.swift - core value types for Gloamlog v0.3 (Foundation only). See docs/EDITOR_CONTRACT.md.
//
// API:
//   DayKey            "yyyy-MM-dd" day strings: string(date,cal) / date(key,cal) / adding(key,n,cal) / weekday(key,cal) / format(key,fmt,cal)
//   DayStatus         logged | partial | skipped | missed | off | future
//   DayDocument       one day = ONE free-form markdown page:
//                       {day, body, isSkipped, skipReason}  +  words, hasContent, status(minWords:)
//                     `body` never contains the "# yyyy-MM-dd" title line (LogStore adds/strips it).
//                     Logged rule: words >= minWords. partial = 1...minWords-1 words. 0 words -> .missed (= no log).
//                     Task lines under a "Carried over from ..." heading are not words (a Carry over click alone never logs a day).
//                     Jots (M2): the words of the Jots block (LogStore.jotsRules.heading) are not words unless LogStore.jotsRules.countTowardLogged
//                     is on, and a leading HH:mm of a jot is never a word. A page with no words but at least one jot is .partial ("started"),
//                     whatever the switch says, so a jots-only day stays in Catch up and the reminder still asks for the write-up.
// All dates are passed in with a Calendar; nothing here reads the clock.
import Foundation

extension String {
    /// Whitespace/newline trimmed copy (named to avoid clashing with v0.1's `trimmed`).
    var dlTrimmed: String { trimmingCharacters(in: .whitespacesAndNewlines) }
}

enum DayKey {
    /// Shape check only ("2026-10-06"); guards file names against path tricks.
    static func isWellFormed(_ s: String) -> Bool {
        let u = Array(s.utf8)
        guard u.count == 10 else { return false }
        for (i, b) in u.enumerated() {
            if i == 4 || i == 7 { if b != 45 { return false } } else if b < 48 || b > 57 { return false }
        }
        return true
    }
    static func string(_ date: Date, _ cal: Calendar) -> String {
        let c = cal.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04ld-%02ld-%02ld", c.year ?? 0, c.month ?? 0, c.day ?? 0)
    }
    /// Start of that day in `cal`'s time zone; nil for malformed or impossible dates (2026-02-30).
    static func date(_ key: String, _ cal: Calendar) -> Date? {
        guard isWellFormed(key) else { return nil }
        let p = key.split(separator: "-").compactMap { Int($0) }
        guard p.count == 3, let d = cal.date(from: DateComponents(year: p[0], month: p[1], day: p[2])) else { return nil }
        return string(d, cal) == key ? d : nil
    }
    /// Calendar-based day arithmetic (DST safe: never adds 86400 seconds).
    static func adding(_ key: String, _ days: Int, _ cal: Calendar) -> String? {
        guard let d = date(key, cal), let n = cal.date(byAdding: .day, value: days, to: d) else { return nil }
        return string(n, cal)
    }
    /// Calendar weekday number, 1 = Sunday ... 7 = Saturday.
    static func weekday(_ key: String, _ cal: Calendar) -> Int {
        guard let d = date(key, cal) else { return 0 }
        return cal.component(.weekday, from: d)
    }
    /// Formatted with `format` (e.g. "EEE d MMM"), English names, in cal's time zone.
    static func format(_ key: String, _ format: String, _ cal: Calendar) -> String {
        guard let d = date(key, cal) else { return key }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX"); f.calendar = cal; f.timeZone = cal.timeZone
        f.dateFormat = format
        return f.string(from: d)
    }
}

enum DayStatus: String, Codable, Equatable {
    case logged   // words >= minWords
    case partial  // 1...minWords-1 words
    case skipped  // skip marker file
    case missed   // scheduled day, no log (today unlogged is also .missed; UI draws today's ring)
    case off      // not a scheduled day, no log (or before the first ever entry)
    case future   // after today
}

/// One day's page. `body` is plain markdown without the "# yyyy-MM-dd" title line.
struct DayDocument: Equatable {
    var day: String
    var body: String
    var isSkipped: Bool
    var skipReason: String

    init(day: String, body: String = "", isSkipped: Bool = false, skipReason: String = "") {
        self.day = day; self.body = body; self.isSkipped = isSkipped; self.skipReason = skipReason
    }

    /// Words per the contract rule (headings, empty list/task markers, code-fence lines, images, HTML comments ignored;
    /// task lines under a "Carried over from ..." heading never count, see MarkdownBody). Jots do not count unless
    /// LogStore.jotsRules.countTowardLogged is on (MarkdownBody.loggedWords): this is what "logged" is measured in.
    var words: Int {
        if isSkipped { return 0 }
        let rules = LogStore.jotsRules
        return MarkdownBody.loggedWords(in: body, jotsHeading: rules.heading, jotsCount: rules.countTowardLogged)
    }
    /// How many notes the page's Jots block holds (top-level list items), whatever the rule counts as words.
    var jotCount: Int { isSkipped ? 0 : MarkdownBody.counts(in: body, jotsHeading: LogStore.jotsRules.heading).jotCount }
    /// True if the page holds anything beyond headings / empty markers (an image or a carried-over block alone counts).
    var hasContent: Bool { !isSkipped && MarkdownBody.hasContent(body) }

    /// .skipped / .logged (words >= minWords) / .partial (1...minWords-1, or no words but at least one jot) / .missed (nothing written:
    /// treat as "no log"). One scan of the page: the words and the jot count come from the same MarkdownBody.counts.
    func status(minWords: Int) -> DayStatus {
        if isSkipped { return .skipped }
        let rules = LogStore.jotsRules
        let c = MarkdownBody.counts(in: body, jotsHeading: rules.heading)
        let w = c.words + (rules.countTowardLogged ? c.jotWords : 0)
        if w >= max(1, minWords) { return .logged }
        return (w > 0 || c.jotCount > 0) ? .partial : .missed
    }
}
