// ReviewFormats.swift - the words of a review: titles, file names, the counts line, and the Markdown / Plain text / HTML writers (M3).
//
// API:
//   ReviewFormats.title(_ period, calendar:, dash:)      "Week of 5 to 11 Oct 2026", "September 2026", "2026", "5 Sep to 12 Oct 2026"
//                                                        (dash: the Markdown heading of a week, "Week of 5 Oct – 11 Oct 2026", as v0.3 copied it)
//   ReviewFormats.fileName(_ period, calendar:)           "Gloamlog 2026-W41", "Gloamlog 2026-10", "Gloamlog 2026", "Gloamlog 2026-09-05 to 2026-10-12"
//   ReviewFormats.countsLine(_ counts) -> String?         "Logged 4 of 5 · 1 skipped · 2 not logged" (never "missed"); nil when nothing counts
//   ReviewFormats.tasksLine(_ tasks) -> String?           "12 tasks done · 3 still open"; nil when there are none
//   ReviewFormats.markdown(_ summary, calendar:)          by Day: "## Mon 5 Oct" + the page; by Section: "## Finished" + "- **Mon 5 Oct**: text";
//                                                        a Year: one bullet per month. The week's Markdown is the v0.3 copy plus the options.
//   ReviewFormats.plain(_ summary, calendar:)             the same without markup, for chat boxes
//   ReviewFormats.html(_ summary, calendar:, images:)      a printable page (PDF); `images` answers a data: URI for an image path, or nil for alt text
//   ReviewFormats.richHTML(_ summary, calendar:)           a page for the pasteboard (alt text for images)
import Foundation

enum ReviewFormats {
    static func plural(_ n: Int, _ word: String) -> String { "\(n) \(word)\(n == 1 ? "" : "s")" }

    private static func span(_ a: String, _ b: String, _ cal: Calendar) -> String {
        if a == b { return DayKey.format(a, "d MMM yyyy", cal) }
        let sameMonth = a.prefix(7) == b.prefix(7), sameYear = a.prefix(4) == b.prefix(4)
        return "\(DayKey.format(a, sameMonth ? "d" : (sameYear ? "d MMM" : "d MMM yyyy"), cal)) to \(DayKey.format(b, "d MMM yyyy", cal))"
    }

    static func title(_ p: ReviewPeriod, calendar cal: Calendar, dash: Bool = false) -> String {
        switch p.scale {
        case .week:
            if dash { return "Week of \(DayKey.format(p.start, "d MMM", cal)) – \(DayKey.format(p.end, "d MMM yyyy", cal))" }
            return "Week of \(span(p.start, p.end, cal))"
        case .month: return DayKey.format(p.start, "MMMM yyyy", cal)
        case .year: return String(p.start.prefix(4))
        case .range: return span(p.start, p.end, cal)
        }
    }

    static func fileName(_ p: ReviewPeriod, calendar cal: Calendar) -> String {
        switch p.scale {
        case .week:
            var iso = Calendar(identifier: .iso8601)                // the week number is the ISO one whatever day the week starts on
            iso.timeZone = cal.timeZone
            guard let mid = DayKey.adding(p.start, 3, cal), let d = DayKey.date(mid, iso) else { return "Gloamlog \(p.start)" }
            return String(format: "Gloamlog %04ld-W%02ld", iso.component(.yearForWeekOfYear, from: d), iso.component(.weekOfYear, from: d))
        case .month: return "Gloamlog \(p.start.prefix(7))"
        case .year: return "Gloamlog \(p.start.prefix(4))"
        case .range: return "Gloamlog \(p.start) to \(p.end)"
        }
    }

    static func countsLine(_ c: ReviewCounts) -> String? {
        guard c.total > 0 else { return nil }
        return "Logged \(c.logged) of \(c.total)" + (c.skipped > 0 ? " · \(c.skipped) skipped" : "") + (c.notWritten > 0 ? " · \(c.notWritten) not logged" : "")
    }

    static func tasksLine(_ t: TaskStats) -> String? {
        guard !t.completed.isEmpty || !t.openAtEnd.isEmpty else { return nil }
        return [t.completed.isEmpty ? nil : "\(plural(t.completed.count, "task")) done", t.openAtEnd.isEmpty ? nil : "\(t.openAtEnd.count) still open"]
            .compactMap { $0 }.joined(separator: " · ")
    }

    /// A month row as one line: "20 logged · 1 skipped · 2 not logged", or a dash for a month that has not started.
    static func rowLine(_ r: MonthRow) -> String {
        if r.isFuture { return "–" }
        let c = r.counts
        return ["\(c.logged) logged", c.skipped > 0 ? "\(c.skipped) skipped" : nil, c.notWritten > 0 ? "\(c.notWritten) not logged" : nil].compactMap { $0 }.joined(separator: " · ")
    }

    static func markdown(_ s: ReviewSummary, calendar cal: Calendar) -> String {
        func short(_ k: String) -> String { DayKey.format(k, "EEE d MMM", cal) }
        var out = "# \(title(s.period, calendar: cal, dash: true))\n\n"
        if let c = countsLine(s.counts) { out += c + "\n\n" }
        if let t = tasksLine(s.tasks) { out += t + "\n\n" }
        if s.period.scale == .year {
            let rows = s.months.filter { !$0.isFuture && $0.counts.total > 0 }
            for r in rows { out += "- **\(DayKey.format(r.month + "-01", "MMMM", cal))**: \(rowLine(r))\n" }
            return rows.isEmpty ? out + "_Nothing written in this period._\n" : out.dlTrimmed + "\n"
        }
        if s.days.isEmpty { return out + "_Nothing written in this period._\n" }
        switch s.options.grouping {
        case .day:
            for d in s.days {
                if d.isSkipped {
                    out += "## \(short(d.day))\n_Skipped" + (d.skipReason.isEmpty ? "" : ": \(d.skipReason)") + "_\n\n"
                } else {
                    out += "## \(short(d.day))\n\n\(MarkdownBody.shiftHeadings(in: d.body, minLevel: 3))\n\n"
                }
            }
        case .section:
            for g in s.groups {
                out += "## \(g.title)\n\n"
                for it in g.items { out += WeeklyReview.itemMarkdown(label: short(it.day), text: it.text) }
                out += "\n"
            }
        }
        return out.dlTrimmed + "\n"
    }

    static func plain(_ s: ReviewSummary, calendar cal: Calendar) -> String { MarkdownHTML.plain(markdown(s, calendar: cal)) }

    static func html(_ s: ReviewSummary, calendar cal: Calendar, images: ((String) -> String?)?) -> String {
        MarkdownHTML.document(title: title(s.period, calendar: cal), md: markdown(s, calendar: cal), images: images)
    }

    static func richHTML(_ s: ReviewSummary, calendar cal: Calendar) -> String { MarkdownHTML.fragment(markdown(s, calendar: cal)) }
}
