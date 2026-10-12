// AppModel+Browse.swift - search (debounced, saved files only) and the weekly review.
import SwiftUI
import AppKit
import Combine

extension AppModel {
    // MARK: search
    func bindSearch() {
        $searchText.removeDuplicates().sink { [weak self] q in
            guard let self = self else { return }
            let empty = q.dlTrimmed.isEmpty
            if !empty, self.selection != .search { self.returnSelection = self.selection; self.selection = .search }
            if empty { self.searchHits = []; if self.selection == .search { self.selection = self.returnSelection } }
        }.store(in: &cancellables)
        Publishers.CombineLatest($searchText, $searchHeading).debounce(for: .milliseconds(150), scheduler: RunLoop.main)
            .sink { [weak self] q, h in self?.runSearch(q, h) }.store(in: &cancellables)
    }

    func runSearch(_ q: String, _ heading: String?) {
        guard !q.dlTrimmed.isEmpty else { searchHits = []; return }
        // What was typed a moment ago must be on disk to be found.
        flushEditor { [weak self] in
            guard let self = self else { return }
            self.searchHits = (try? self.store.search(q, heading: heading)) ?? []
            self.searchCursor = 0
        }
    }
    var searchDayCount: Int { Set(searchHits.map { $0.day }).count }
    /// Headings offered in the search filter: the template's headings.
    var searchHeadingChoices: [String] { MarkdownBody.headings(inTemplate: settings.template) }

    func moveSearchCursor(_ d: Int) {
        guard selection == .search, !searchHits.isEmpty else { return }
        searchCursor = (searchCursor + d + searchHits.count) % searchHits.count
    }
    func focusSearch() { searchFocusTick += 1 }
    func clearSearch() { searchText = "" }

    /// Opens the hit's day and scrolls the matched text into view.
    func open(hit: SearchHit) {
        let day = hit.day
        let needle = hit.matchRange.map { String(hit.snippet[$0]) } ?? ""
        searchText = ""
        openDay(day)
        if !needle.isEmpty {
            // Give the page a moment to load, then select the first occurrence (selection only, no edit).
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) { [weak self] in self?.bridgeStorage?.reveal(text: needle) }
        }
    }

    // MARK: weekly review
    func refreshWeek() {
        let probe = WeeklyReview.summary(weekContaining: weekDate, documents: [:], states: states, settings: settings, calendar: cal, now: now)
        let docs = (try? store.documents(from: probe.weekStart, to: probe.weekEnd)) ?? [:]
        week = WeeklyReview.summary(weekContaining: weekDate, documents: docs, states: states, settings: settings, calendar: cal, now: now)
        review.rebuild()                                     // the Review (any scale) follows the same data and the same anchor date
    }
    /// The next period of the shown scale is not in the future (the week review's ›, ⌘]).
    var canShowNextWeek: Bool { review.canStep(1) }
    var isCurrentWeek: Bool { cal.isDate(weekDate, equalTo: now, toGranularity: .weekOfYear) }
    func goThisWeek() { weekDate = now; refreshWeek() }

    func weekMarkdown() -> String {
        guard let w = week else { return "" }
        return WeeklyReview.markdown(for: w, grouping: weekGrouping, calendar: cal)
    }
    /// ⇧⌘C on the Review: Markdown of exactly what is shown (any scale, with the Skipped days and Jots choices); the week's is the v0.3 copy.
    func copyWeek() {
        guard week != nil else { return }
        review.copy(.markdown)
    }
    func weekTitle(_ w: WeekSummary) -> String {
        let sameMonth = w.weekStart.prefix(7) == w.weekEnd.prefix(7), sameYear = w.weekStart.prefix(4) == w.weekEnd.prefix(4)
        let from = DayKey.format(w.weekStart, sameMonth ? "d" : (sameYear ? "d MMM" : "d MMM yyyy"), cal)
        return "Week of \(from) to \(DayKey.format(w.weekEnd, "d MMM yyyy", cal))"
    }

    // MARK: the week's days (every day of the week is a tile that opens, empty weeks included)
    /// The seven days of the shown week, first weekday per Settings. Status comes from `status(of:)`, so days before the
    /// log start read as muted, never as unlogged.
    func weekTiles() -> [WeekTile] {
        guard let w = week else { return [] }
        return (0..<7).compactMap { i in
            guard let d = DayKey.adding(w.weekStart, i, cal) else { return nil }
            let st = status(of: d)
            let words = w.days.first(where: { $0.day == d })?.doc?.words ?? 0
            let bottom: String
            switch st {
            case .logged, .partial: bottom = Fmt.plural(words, "word")
            case .skipped: let r = skipReasons[d] ?? ""; bottom = r.isEmpty ? "Skipped" : r
            case .missed: bottom = "Write"
            case .off, .future: bottom = ""
            }
            return WeekTile(day: d, weekday: DayKey.format(d, "EEE", cal), number: Int(d.suffix(2)) ?? 0, status: st,
                            bottom: bottom, isToday: d == today, opens: d <= today)
        }
    }

    /// "Logged 4 of 5 · 1 skipped · 2 not logged": every working day that has happened is exactly one of the three.
    /// Today counts once it is written or skipped, never as "not logged" while the day is still going.
    func weekCounts() -> WeekCounts {
        var c = WeekCounts()
        for t in weekTiles() {
            switch t.status {
            case .logged: c.logged += 1
            case .skipped: c.skipped += 1
            case .partial, .missed: if isWorkday(t.day) && t.day < today && !isBeforeLogStart(t.day) { c.notLogged += 1 }
            case .off, .future: break
            }
        }
        return c
    }
    var weekHasUnlogged: Bool { weekCounts().notLogged > 0 }

    /// "Catch up this week": the Catch up screen scoped to the shown week.
    func catchUpThisWeek() {
        guard let w = week else { return }
        select(.catchUp)
        catchRange = w.weekStart...w.weekEnd
    }
}

struct WeekTile: Identifiable, Equatable {
    var id: String { day }
    var day: String
    var weekday: String          // "Mon"
    var number: Int
    var status: DayStatus
    var bottom: String           // "63 words", "Write", "Leave", or nothing
    var isToday: Bool
    var opens: Bool              // false for days that have not happened yet
}
struct WeekCounts: Equatable {
    var logged = 0, skipped = 0, notLogged = 0
    var total: Int { logged + skipped + notLogged }
}
