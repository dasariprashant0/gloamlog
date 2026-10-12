// PeriodReview.swift - one builder for Week, Month, Year and a custom Range (M3). Pure: documents and states come in, a summary goes out.
//
// API:
//   PeriodReview.period(_ scale, containing: Date, calendar:) -> ReviewPeriod      the week (calendar.firstWeekday), month or year around a date
//   PeriodReview.range(from: Date, to: Date, calendar:) -> ReviewPeriod             either order; at most maxRangeDays (the end is kept)
//   PeriodReview.shifted(_ p, by: ±1, calendar:) -> ReviewPeriod?                   the previous / next period of the same size
//   PeriodReview.build(period:, documents:, states:, settings:, options:, calendar:, now:) -> ReviewSummary
//       documents = LogStore.documents(from:to:) (not needed for a Year); states = LogStore.fileStates(minWords:)
//   PeriodReview.splitJots(_ body, heading:) -> (rest, jots)                         the page without its Jots blocks, and what was in them
// Counts use the same rule as the week screen and Catch up (Status.resolve + CatchUp.isMissing): a day is "not logged" only when it is
// scheduled, strictly before today and on or after the log start. Skipped days are counted only while options.includeSkipped is on.
// Content: each page is cut by MarkdownBody.partition into the template headings (template order), then Jots (when included), then
// "Other notes", then the skipped days as one-line items. The page as shown by day has its Jots block split off first (unless included),
// its empty headings pruned, and is left out when nothing is written. Tasks come from the same text, so they follow the Jots option.
import Foundation

enum PeriodReview {
    static let maxRangeDays = 3_700

    // MARK: periods
    static func period(_ scale: ReviewScale, containing anchor: Date, calendar: Calendar) -> ReviewPeriod {
        let unit: Calendar.Component = scale == .month ? .month : (scale == .year ? .year : .weekOfYear)
        let k = DayKey.string(anchor, calendar)
        guard scale != .range, let iv = calendar.dateInterval(of: unit, for: anchor) else { return ReviewPeriod(scale: scale, start: k, end: k) }
        let last = calendar.date(byAdding: .day, value: -1, to: iv.end) ?? iv.start
        return ReviewPeriod(scale: scale, start: DayKey.string(iv.start, calendar), end: DayKey.string(last, calendar))
    }

    static func range(from: Date, to: Date, calendar: Calendar) -> ReviewPeriod {
        var a = DayKey.string(min(from, to), calendar)
        let b = DayKey.string(max(from, to), calendar)
        if let cap = DayKey.adding(b, -(maxRangeDays - 1), calendar), a < cap { a = cap }
        return ReviewPeriod(scale: .range, start: a, end: b)
    }

    /// How many days the period spans (0 for a malformed one).
    static func dayCount(_ p: ReviewPeriod, calendar: Calendar) -> Int {
        guard let s = DayKey.date(p.start, calendar), let e = DayKey.date(p.end, calendar) else { return 0 }
        return (calendar.dateComponents([.day], from: s, to: e).day ?? 0) + 1
    }

    static func shifted(_ p: ReviewPeriod, by n: Int, calendar: Calendar) -> ReviewPeriod? {
        guard let s = DayKey.date(p.start, calendar), DayKey.date(p.end, calendar) != nil else { return nil }
        switch p.scale {
        case .week: return calendar.date(byAdding: .day, value: 7 * n, to: s).map { period(.week, containing: $0, calendar: calendar) }
        case .month: return calendar.date(byAdding: .month, value: n, to: s).map { period(.month, containing: $0, calendar: calendar) }
        case .year: return calendar.date(byAdding: .year, value: n, to: s).map { period(.year, containing: $0, calendar: calendar) }
        case .range:
            let len = dayCount(p, calendar: calendar)
            guard let a = DayKey.adding(p.start, n * len, calendar), let b = DayKey.adding(p.end, n * len, calendar) else { return nil }
            return ReviewPeriod(scale: .range, start: a, end: b)
        }
    }

    /// Calls `body` with each day key from `a` to `b` (inclusive) and its start-of-day date. Nothing when a > b.
    static func forEachDay(from a: String, to b: String, calendar: Calendar, _ body: (String, Date) -> Void) {
        guard a <= b, var d = DayKey.date(a, calendar) else { return }
        var k = a
        while k <= b {
            body(k, d)
            guard let n = calendar.date(byAdding: .day, value: 1, to: d) else { return }
            let nk = DayKey.string(n, calendar)
            if nk <= k { return }                                   // the calendar did not move on: never loop
            d = n; k = nk
        }
    }

    // MARK: Jots
    /// The page without its Jots blocks, and the text inside them (their heading lines dropped).
    static func splitJots(_ body: String, heading: String) -> (rest: String, jots: String) {
        let lines = MarkdownBody.scan(body)
        let blocks = MarkdownBody.jotsBlocks(in: lines, jotsHeading: heading)
        guard !blocks.isEmpty else { return (body, "") }
        var inBlock = Set<Int>(), heads = Set<Int>()
        for b in blocks { heads.insert(b.heading); for i in b.heading..<b.end { inBlock.insert(i) } }
        func text(_ keep: (Int) -> Bool) -> String { MarkdownBody.trimBody(lines.indices.filter(keep).map { lines[$0].text }.joined(separator: "\n")) }
        return (text { !inBlock.contains($0) }, text { inBlock.contains($0) && !heads.contains($0) })
    }

    // MARK: build
    static func build(period p: ReviewPeriod, documents: [String: DayDocument], states: [String: DayStatus], settings: Settings,
                      options: ReviewOptions, calendar: Calendar, now: Date) -> ReviewSummary {
        let today = DayKey.string(now, calendar)
        let since = Status.effectiveSince(setting: settings.logStartDate, states: states)
        var summary = ReviewSummary(period: p, options: options)

        func count(_ a: String, _ b: String) -> ReviewCounts {
            var c = ReviewCounts()
            forEachDay(from: a, to: min(b, today), calendar: calendar) { k, d in
                let st = Status.resolve(day: k, fileState: states[k], now: now, calendar: calendar, weekdays: settings.weekdays, since: since)
                switch st {
                case .logged: c.logged += 1
                case .skipped: if options.includeSkipped { c.skipped += 1 }
                case .partial, .missed:
                    if CatchUp.isMissing(day: k, weekday: calendar.component(.weekday, from: d), status: st, today: today, since: since, weekdays: settings.weekdays) {
                        if st == .partial { c.started += 1 } else { c.unlogged += 1 }
                    }
                case .off, .future: break
                }
            }
            return c
        }

        if p.scale == .year {                                       // counts only: no page is read for a year
            guard var m = DayKey.date(p.start, calendar) else { return summary }
            for _ in 0..<12 {
                let first = DayKey.string(m, calendar)
                let next = calendar.date(byAdding: .month, value: 1, to: m) ?? m
                let last = DayKey.adding(DayKey.string(next, calendar), -1, calendar) ?? first
                let counts = count(first, last)
                summary.months.append(MonthRow(month: String(first.prefix(7)), counts: counts, isFuture: first > today))
                summary.counts.add(counts)
                m = next
            }
            return summary
        }
        summary.counts = count(p.start, p.end)

        let template = MarkdownBody.headings(inTemplate: settings.template)
        let norm = template.map { MarkdownBody.normalizeHeading($0) }
        var buckets = [[ReviewItem]](repeating: [], count: template.count)
        var other = [ReviewItem](), jots = [ReviewItem](), skipped = [ReviewItem]()
        var seenDone = Set<String>(), lastLoggedBody: String?

        for k in documents.keys.sorted() where k >= p.start && k <= p.end {
            guard let doc = documents[k] else { continue }
            if doc.isSkipped {
                guard options.includeSkipped else { continue }
                summary.days.append(ReviewDay(day: k, status: .skipped, words: 0, skipReason: doc.skipReason, body: ""))
                skipped.append(ReviewItem(day: k, text: doc.skipReason.isEmpty ? "Skipped" : doc.skipReason))
                continue
            }
            let split = splitJots(doc.body, heading: settings.capture.jotsHeading)
            let shown = options.includeJots ? doc.body : split.rest
            guard MarkdownBody.hasContent(shown) else { continue }
            let status = states[k] ?? doc.status(minWords: settings.minWords)
            summary.days.append(ReviewDay(day: k, status: status, words: doc.words, skipReason: "", body: MarkdownBody.pruneEmptySections(shown)))
            if status == .logged { lastLoggedBody = shown }

            let part = MarkdownBody.partition(split.rest, headings: norm)
            var per = [Int: [String]]()
            for m in part.matched { per[m.index, default: []].append(m.text) }
            for (i, texts) in per.sorted(by: { $0.key < $1.key }) { buckets[i].append(ReviewItem(day: k, text: texts.joined(separator: "\n\n"))) }
            if !part.other.isEmpty { other.append(ReviewItem(day: k, text: part.other)) }
            if options.includeJots, MarkdownBody.hasContent(split.jots) { jots.append(ReviewItem(day: k, text: split.jots)) }

            for l in MarkdownBody.contentLines(shown) where !l.isCode && l.checked == true {
                let text = MarkdownBody.collapse(MarkdownBody.removeNoise(l.text))
                if !text.isEmpty, seenDone.insert(MarkdownBody.itemKey(text)).inserted { summary.tasks.completed.append(ReviewItem(day: k, text: text)) }
            }
        }
        if let body = lastLoggedBody { summary.tasks.openAtEnd = CarryOver.items(in: body, headings: settings.carryOverHeadings) }

        for i in template.indices where !buckets[i].isEmpty { summary.groups.append(ReviewGroup(title: template[i], kind: .heading, items: buckets[i])) }
        if !jots.isEmpty { summary.groups.append(ReviewGroup(title: MarkdownBody.jotsHeadingText(settings.capture.jotsHeading), kind: .jots, items: jots)) }
        if !other.isEmpty { summary.groups.append(ReviewGroup(title: WeeklyReview.otherTitle, kind: .other, items: other)) }
        if !skipped.isEmpty { summary.groups.append(ReviewGroup(title: "Skipped days", kind: .skipped, items: skipped)) }
        return summary
    }
}
