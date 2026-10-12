// ReviewModel.swift - the value types of the Review screen and its copies and exports (M3). Foundation only, no disk, no clock.
//
// A ReviewPeriod is an inclusive span of day keys for one of four scales. PeriodReview.build turns the day files of a period into a
// ReviewSummary: counts (logged / skipped / started / not written scheduled days), the pages to show (`days`, for "Group by Day"), the
// roll-ups per template heading plus Jots and "Other notes" (`groups`, for "Group by Section"), the tasks done and still open, and for
// a Year the 12 month rows. ReviewOptions decide what is in it, so the screen and every copy and export show the same thing.
import Foundation

enum ReviewScale: String, CaseIterable, Equatable {
    case week, month, year, range
    var title: String { rawValue.prefix(1).uppercased() + rawValue.dropFirst() }
}

/// An inclusive span of day keys ("2026-10-05" ... "2026-10-11").
struct ReviewPeriod: Equatable {
    var scale: ReviewScale
    var start: String
    var end: String
}

struct ReviewOptions: Equatable {
    var grouping: WeekGrouping = .section
    var includeSkipped = true       // off: skipped days vanish from the review, its counts and every export
    var includeJots = false         // the Jots block is its own group (or stays in place by day) only when on
}

/// logged + skipped + started + unlogged = the scheduled days that have happened (plus pages written on days off).
/// "Not logged" is started + unlogged; today is never counted as unlogged while it is still going.
struct ReviewCounts: Equatable {
    var logged = 0, skipped = 0, started = 0, unlogged = 0
    var notWritten: Int { started + unlogged }
    var total: Int { logged + skipped + started + unlogged }
    var scheduled: Int { total }                    // "Logged 4 of 5": the 5
    mutating func add(_ o: ReviewCounts) { logged += o.logged; skipped += o.skipped; started += o.started; unlogged += o.unlogged }
}

struct ReviewItem: Equatable { var day: String; var text: String }

struct ReviewGroup: Equatable {
    enum Kind: Equatable { case heading, jots, other, skipped }
    var title: String
    var kind: Kind
    var items: [ReviewItem]
}

/// completed = every ticked task of the period (once each); openAtEnd = what the last logged day still carries under the
/// carry-over headings (the same rule as "From yesterday").
struct TaskStats: Equatable {
    var completed: [ReviewItem] = []
    var openAtEnd: [String] = []
}

struct MonthRow: Equatable {
    var month: String               // "2026-09"
    var counts: ReviewCounts
    var isFuture: Bool              // the month has not started: shown as dashes, inert
}

/// A day with something to show: a page with writing, or (when included) a skip marker.
struct ReviewDay: Equatable {
    var day: String
    var status: DayStatus           // .logged, .partial or .skipped
    var words: Int
    var skipReason: String
    var body: String                // the page as shown: Jots left out unless included, empty headings pruned; "" when skipped
    var isSkipped: Bool { status == .skipped }
}

struct ReviewSummary: Equatable {
    var period: ReviewPeriod
    var options: ReviewOptions
    var counts = ReviewCounts()
    var days: [ReviewDay] = []
    var groups: [ReviewGroup] = []
    var tasks = TaskStats()
    var months: [MonthRow] = []     // Year only

    /// Nothing to show: no page and no (included) skip in the period; for a Year, no page or skip in any month.
    var isEmpty: Bool {
        period.scale == .year ? months.allSatisfy { $0.counts.logged + $0.counts.skipped + $0.counts.started == 0 } : days.isEmpty
    }
}
