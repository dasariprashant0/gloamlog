// WeeklyReview.swift - one week's aggregate and "Copy week as markdown".
//
// API:
//   WeeklyReview.summary(weekContaining:, documents:, states:, settings:, calendar:, now:) -> WeekSummary
//       documents = [dayKey: DayDocument] (LogStore.documents(from:to:)); states = LogStore.fileStates(minWords:)
//   WeeklyReview.shift(_ date, weeks:, calendar:) -> Date          prev/next week (UI disables future)
//   WeeklyReview.markdown(for:, grouping: .day | .section, calendar:) -> String
//   WeeklyReview.itemMarkdown(label:, text:) -> String        one "- **Mon 5 Oct**: text" entry of a by-section list (also used by Review/ReviewFormats)
//   WeekSummary: weekStart, weekEnd, days:[WeekDay{day,status,doc}], loggedCount, workdayCount (scheduled days not in the
//                future), skippedCount, missingDays, sections:[WeekSection{title,isOther,items:[WeekItem{day,text}]}],
//                section(titled:) -> WeekSection?
//   days always holds all 7 days of the week (weeks with no file included); statuses come from Status.resolve with
//   since = Status.effectiveSince(setting: settings.logStartDate, states:), so days before the log start are .off and the current
//   week's later days are .future. missingDays = the week's scheduled days strictly before today, on or after the log start, that
//   are .missed or .partial (oldest first): exactly CatchUp.missing restricted to the week (same rule, CatchUp.isMissing), and
//   empty while there is no log start and no page at all. The week's days a UI should list are the scheduled ones plus any day with a page.
// By section: for every heading of settings.template (template order) the text under matching headings across the week
// (matched by normalised title; nested sub-headings stay inside, shown as bold labels), then "Other notes" = everything else
// (text before the first heading, headings not in the template). Empty groups are left out.
// By day: each non-empty page with headings re-levelled under "## Mon 5 Oct" (shallowest heading becomes ###) and empty
// sections removed. Skipped days print "_Skipped: reason_". Unlogged days are omitted (never listed as missed).
// Weeks start on calendar.firstWeekday.
import Foundation

struct WeekDay: Equatable {
    var day: String
    var status: DayStatus
    var doc: DayDocument?
}
struct WeekItem: Equatable { var day: String; var text: String }
struct WeekSection: Equatable {
    var title: String
    var isOther: Bool
    var items: [WeekItem]
}

struct WeekSummary: Equatable {
    var weekStart: String
    var weekEnd: String
    var days: [WeekDay]
    var sections: [WeekSection]
    var loggedCount: Int
    var workdayCount: Int
    var skippedCount: Int
    var missingDays: [String] = []

    /// First group whose title matches under MarkdownBody.normalizeHeading ("finished" finds "✅ Finished"; "other notes" finds the rest).
    func section(titled t: String) -> WeekSection? {
        let n = MarkdownBody.normalizeHeading(t)
        return sections.first { MarkdownBody.normalizeHeading($0.title) == n }
    }
}

enum WeekGrouping { case day, section }

enum WeeklyReview {
    static let otherTitle = "Other notes"

    static func shift(_ date: Date, weeks: Int, calendar: Calendar) -> Date {
        calendar.date(byAdding: .weekOfYear, value: weeks, to: date) ?? date
    }

    static func summary(weekContaining date: Date, documents: [String: DayDocument], states: [String: DayStatus],
                        settings: Settings, calendar: Calendar, now: Date) -> WeekSummary {
        let start = calendar.dateInterval(of: .weekOfYear, for: date)?.start ?? date
        let today = DayKey.string(now, calendar)
        let since = Status.effectiveSince(setting: settings.logStartDate, states: states)   // days before the log start are .off, never missed
        var days = [WeekDay](), missing = [String](), logged = 0, work = 0, skipped = 0
        for i in 0..<7 {                                             // all 7 days of the week, whatever the number of files (none is fine)
            guard let d = calendar.date(byAdding: .day, value: i, to: start) else { continue }
            let k = DayKey.string(d, calendar)
            let weekday = calendar.component(.weekday, from: d)
            let st = Status.resolve(day: k, fileState: states[k], now: now, calendar: calendar, weekdays: settings.weekdays, since: since)
            days.append(WeekDay(day: k, status: st, doc: documents[k]))
            if st == .logged { logged += 1 }
            if st == .skipped { skipped += 1 }
            if st != .future && settings.weekdays.contains(weekday) { work += 1 }
            if CatchUp.isMissing(day: k, weekday: weekday, status: st, today: today, since: since, weekdays: settings.weekdays) { missing.append(k) }
        }

        let template = MarkdownBody.headings(inTemplate: settings.template)
        let norm = template.map { MarkdownBody.normalizeHeading($0) }
        var buckets = [[WeekItem]](repeating: [], count: template.count)
        var other = [WeekItem]()
        for wd in days {
            guard let doc = wd.doc, !doc.isSkipped else { continue }
            let p = MarkdownBody.partition(doc.body, headings: norm)
            var per = [Int: [String]]()
            for m in p.matched { per[m.index, default: []].append(m.text) }
            for (i, texts) in per.sorted(by: { $0.key < $1.key }) {
                buckets[i].append(WeekItem(day: wd.day, text: texts.joined(separator: "\n\n")))
            }
            if !p.other.isEmpty { other.append(WeekItem(day: wd.day, text: p.other)) }
        }
        var sections = [WeekSection]()
        for i in template.indices where !buckets[i].isEmpty { sections.append(WeekSection(title: template[i], isOther: false, items: buckets[i])) }
        if !other.isEmpty { sections.append(WeekSection(title: otherTitle, isOther: true, items: other)) }

        return WeekSummary(weekStart: days.first?.day ?? "", weekEnd: days.last?.day ?? "", days: days, sections: sections,
                           loggedCount: logged, workdayCount: work, skippedCount: skipped, missingDays: missing)
    }

    static func markdown(for w: WeekSummary, grouping: WeekGrouping, calendar: Calendar) -> String {
        func short(_ k: String) -> String { DayKey.format(k, "EEE d MMM", calendar) }
        var out = "# Week of \(DayKey.format(w.weekStart, "d MMM", calendar)) – \(DayKey.format(w.weekEnd, "d MMM yyyy", calendar))\n\n"
        out += "Logged \(w.loggedCount) of \(w.workdayCount) workdays" + (w.skippedCount > 0 ? " · \(w.skippedCount) skipped" : "") + "\n\n"
        switch grouping {
        case .day:
            for d in w.days {
                guard let doc = d.doc else { continue }
                if doc.isSkipped {
                    out += "## \(short(d.day))\n_Skipped" + (doc.skipReason.isEmpty ? "" : ": \(doc.skipReason)") + "_\n\n"
                    continue
                }
                let body = MarkdownBody.shiftHeadings(in: MarkdownBody.pruneEmptySections(doc.body), minLevel: 3)
                if MarkdownBody.hasContent(body) { out += "## \(short(d.day))\n\n\(body)\n\n" }
            }
        case .section:
            for s in w.sections {
                out += "## \(s.title)\n\n"
                for it in s.items { out += itemMarkdown(label: short(it.day), text: it.text) }
                out += "\n"
            }
        }
        return out.dlTrimmed + "\n"
    }

    /// One entry of a "by section" list: "- **Mon 5 Oct**: first line", then the other lines indented (also used by the Review screen's copies).
    static func itemMarkdown(label: String, text: String) -> String {
        let lines = text.components(separatedBy: "\n")
        let first = lines[0]
        let plain = MarkdownBody.stripMarkers(first).text == first.dlTrimmed && !first.hasPrefix("```")
            && !(lines.count > 1 && first.hasPrefix("**") && first.hasSuffix("**"))
        var out = "- **\(label)**" + (plain ? ": \(first)\n" : "\n")
        for l in (plain ? Array(lines.dropFirst()) : lines) { out += l.isEmpty ? "\n" : "  \(l)\n" }
        return out
    }
}
