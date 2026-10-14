// DayWriter.swift - the single place a captured note is routed. Main thread only. Owned by the core stream (task K3).
// CONTRACT (signatures frozen):
//   capture(text:source:)  normalise (trim, strip control chars except newlines, cap 4,000, "[]" to-do shorthand, timestamp per prefs), return nil
//                          when nothing is left; journal.append (durable) FIRST; then route:
//                            - page open for that day and loaded -> page.appendJot (live editor transaction); on .appended the editor's
//                              own autosave writes the file, and acknowledgeSaved(day:body:) acks the note once the file holds it;
//                              .notLoaded -> keep waiting (retried by replayJournal); .stale -> re-route once; .alreadyPresent -> ack when saved
//                            - no open page for that day -> load the file (or ""), CapturePlacement.apply, LogStore.save, ack
//                            - the folder is unavailable or the save fails -> the note stays in the journal (waiting > 0), nothing is lost
//   replayJournal()        re-route every pending item (launch, folder returns, wake, app activation); idempotent via CapturePlacement.contains
//   waiting                journal items not yet acknowledged
//   acknowledgeSaved       the UI calls this after every successful page save; acks pending items whose line is now in the written body
// Details (what the contract leaves open):
//   * Normalising: CR, CRLF, NEL, U+2028 and U+2029 become "\n"; tab, VT and FF become a space; every other control character is dropped;
//     the text is trimmed and capped at 4,000 characters. "[]" or "[ ]" at the start makes a to-do when todoShorthand is on (no time stamp,
//     the marker is removed; a note that is only the marker is empty). The time stamp is HH:mm of `clock()` in `calendar()`, the day is the
//     day of `clock()` in `calendar()` at capture time and never changes: a note captured at 23:59 and delivered after midnight still
//     belongs to the day it was captured on.
//   * CaptureReceipt.waiting and `waiting` are literal: every note that is not acknowledged yet counts, so right after a capture into an OPEN
//     page it is 1 until that page's autosave has been reported through acknowledgeSaved. isInOpenPage(id) tells that case apart from a note
//     that waits for the folder.
//   * The writer never writes a day's file while an editor exists for that day (loaded or not, orphans and hidden windows included): the
//     live page owns the text. Only a day with no page at all goes through LogStore, one load + one save per day for all its pending notes
//     in capture order, so a replay of a backlog makes one backup, not one per note.
//   * A capture routes ALL pending notes in order, not only the new one: notes that waited for the folder arrive before newer ones.
//   * Idempotent delivery: the file path checks CapturePlacement.contains (or the note's line as the page editor spelled it when it took the
//     note, `editorKeys`: a bare link comes back as <https://...>) before it adds a line, and acknowledges after a successful save
//     even when the page could not show the line, so a replay can never add a note twice. The editor path sends every note with the editor's
//     own "unless present" rule. A skipped day (marker file) is written over, exactly like "write a log anyway": the note is never kept
//     out of the page.
//   * The journal is the durability point. If its write fails (disk full), the note is still routed, kept in memory as "not journaled",
//     and appended to the journal at the next capture or replay; waiting counts it.
//   * Transient answers (.notLoaded, .stale twice) are retried by a bounded timer (retryDelay, maxAutoRetries; `scheduleRetry` is the seam
//     tests replace); every replayJournal() starts the budget again. A call to the editor that never answers is sent again after 5 s.
//   * The first replayJournal() of a writer compacts the journal (acknowledged notes are dropped).
import Foundation

final class DayWriter {
    private let store: LogStore
    private let journal: CaptureJournal
    private let pages: EditorLookup
    private let prefs: () -> CapturePrefs
    private let clock: () -> Date
    private let calendar: () -> Calendar

    /// A note handed to a live page: `confirmed` once the page answered .appended (or .alreadyPresent and the file does not hold it yet).
    private struct Sent { weak var page: PageHandle?; var at: Date; var confirmed: Bool }

    private var queue = [CaptureItem]()                 // not acknowledged yet, in capture order: the journal's pending notes plus unjournaled ones
    private var loaded = false
    private var unjournaled = Set<String>()             // ids whose journal write failed
    private var sent = [String: Sent]()
    private var editorKeys = [String: String]()         // note id -> its line as the page editor wrote it (it may spell it differently from `render`)
    private var staleRetried = Set<String>()
    private var routing = false, again = false, needsRetry = false, retryScheduled = false, compacted = false
    private var autoRetries = 0

    var retryDelay: TimeInterval = 0.5
    var maxAutoRetries = 12
    var scheduleRetry: (TimeInterval, @escaping () -> Void) -> Void = { delay, work in
        DispatchQueue.main.asyncAfter(deadline: .now() + delay, execute: work)
    }

    init(store: LogStore, journal: CaptureJournal, pages: EditorLookup, prefs: @escaping () -> CapturePrefs,
         clock: @escaping () -> Date, calendar: @escaping () -> Calendar) {
        self.store = store; self.journal = journal; self.pages = pages
        self.prefs = prefs; self.clock = clock; self.calendar = calendar
    }

    // MARK: public
    func capture(text: String, source: String) -> CaptureReceipt? {
        let p = prefs()
        guard let n = Self.normalise(text, todoShorthand: p.todoShorthand) else { return nil }
        let now = clock(), cal = calendar()
        let hm = cal.dateComponents([.hour, .minute], from: now)
        let item = CaptureItem(id: UUID().uuidString, ts: now, day: DayKey.string(now, cal), text: n.text,
                               stamp: (p.timestamps && !n.todo) ? String(format: "%02ld:%02ld", hm.hour ?? 0, hm.minute ?? 0) : nil,
                               todo: n.todo, source: source)
        load()
        do { try journal.append(item) } catch { unjournaled.insert(item.id) }      // durable first; if the disk refuses, keep it in memory
        queue.append(item)
        autoRetries = 0
        routeAll()
        return CaptureReceipt(id: item.id, waiting: queue.count)
    }

    func replayJournal() {
        load()
        autoRetries = 0
        staleRetried.removeAll()
        routeAll(resend: true)
        if !compacted { compacted = true; try? journal.compact() }               // at launch: drop what is acknowledged
    }

    var waiting: Int { load(); return queue.count }

    func acknowledgeSaved(day: String, body: String) {
        load()
        let mine = queue.filter { $0.day == day }
        guard !mine.isEmpty else { return }
        let keys = CapturePlacement.jotKeys(in: body, jotsHeading: prefs().jotsHeading)
        for item in mine where holds(item, keys) { ack(item) }
    }

    /// True when `keys` (the jot keys of a page or a file) hold the note: as `render` wrote it, or as the page editor spelled it when it
    /// took the note (a link it writes as <https://...>, for one, which `render` never produces). Without the second form such a note
    /// would never be acknowledged, stay "waiting" for ever and be added a second time when it is next delivered through the file.
    private func holds(_ item: CaptureItem, _ keys: Set<String>) -> Bool {
        keys.contains(CapturePlacement.key(of: CapturePlacement.render(item))) || (editorKeys[item.id].map { keys.contains($0) } ?? false)
    }

    /// True while the note sits in a live editor page that has accepted it and will save it: it is not in the file yet, but it is not
    /// waiting for the folder either. A panel should say "kept on this Mac" only for a note where this is false and `waiting > 0`.
    func isInOpenPage(_ id: String) -> Bool {
        guard let s = sent[id], s.confirmed, let page = s.page, let item = queue.first(where: { $0.id == id }) else { return false }
        return pages.page(for: item.day) === page
    }

    // MARK: normalising
    /// nil when nothing is left. Internal so it can be read next to the contract.
    static func normalise(_ raw: String, todoShorthand: Bool) -> (text: String, todo: Bool)? {
        var scalars = String.UnicodeScalarView()
        var afterCR = false
        for u in raw.unicodeScalars {
            switch u.value {
            case 0x0A: if !afterCR { scalars.append("\n") }                      // the LF of a CRLF is not a second break
            case 0x0D, 0x85, 0x2028, 0x2029: scalars.append("\n")
            case 0x09, 0x0B, 0x0C: scalars.append(" ")
            default: if u.properties.generalCategory != .control { scalars.append(u) }
            }
            afterCR = u.value == 0x0D
        }
        var text = String(scalars).dlTrimmed
        var todo = false
        if todoShorthand {
            if text.hasPrefix("[]") { text = String(text.dropFirst(2)).dlTrimmed; todo = true }
            else if text.hasPrefix("[ ]") { text = String(text.dropFirst(3)).dlTrimmed; todo = true }
        }
        if text.count > 4000 { text = String(text.prefix(4000)).dlTrimmed }
        if text.isEmpty { return nil }
        return (text, todo)
    }

    // MARK: state
    private func load() {
        guard !loaded else { return }
        loaded = true
        queue = journal.pending()
    }
    private func isPending(_ id: String) -> Bool { queue.contains { $0.id == id } }

    /// Acknowledged: gone from the queue and, when it was ever journaled, from the journal.
    private func ack(_ item: CaptureItem) {
        queue.removeAll { $0.id == item.id }
        sent[item.id] = nil
        editorKeys[item.id] = nil
        staleRetried.remove(item.id)
        if unjournaled.remove(item.id) == nil { try? journal.ack(item.id) }
    }

    private func persistUnjournaled() {
        for item in queue where unjournaled.contains(item.id) {
            if (try? journal.append(item)) != nil { unjournaled.remove(item.id) }
        }
    }

    // MARK: routing
    /// One pass at a time: a completion that asks for another pass while one is running only sets `again`.
    private func routeAll(resend: Bool = false) {
        if routing { again = true; return }
        routing = true
        defer { routing = false }
        needsRetry = false
        var passes = 0, doResend = resend
        repeat {
            again = false
            passes += 1
            routePass(resend: doResend)
            doResend = false
        } while again && passes < 3
        scheduleRetryIfNeeded()
    }

    private func routePass(resend: Bool) {
        persistUnjournaled()
        let heading = prefs().jotsHeading
        var days = [String]()
        for item in queue where !days.contains(item.day) { days.append(item.day) }
        for day in days {
            let items = queue.filter { $0.day == day }
            if let page = pages.page(for: day) { send(items, to: page, resend: resend) }
            else { deliverToFile(day, items, heading: heading) }
        }
    }

    /// The editor path: hand each pending note of the day to the live page, in order.
    private func send(_ items: [CaptureItem], to page: PageHandle, resend: Bool) {
        guard page.isLoaded else { needsRetry = true; return }
        for item in items where isPending(item.id) {
            if let s = sent[item.id], s.page === page {
                if s.confirmed { if !resend { continue } }                                // already in the page, waiting for its autosave
                else if clock().timeIntervalSince(s.at) < 5 { continue }                  // still being answered
            }
            sent[item.id] = Sent(page: page, at: clock(), confirmed: false)
            page.appendJot(item) { [weak self, weak page] result in self?.completed(item, page: page, result) }
        }
    }

    private func completed(_ item: CaptureItem, page: PageHandle?, _ result: JotAppendResult) {
        guard isPending(item.id) else { sent[item.id] = nil; return }                   // acknowledged while the call was in flight
        switch result {
        case .appended(let md):
            if let k = CapturePlacement.lastJotKey(in: md, jotsHeading: prefs().jotsHeading) { editorKeys[item.id] = k }   // the note is the last item of that section
            sent[item.id] = Sent(page: page, at: clock(), confirmed: true)               // the editor's autosave writes it; acknowledgeSaved acks it
        case .alreadyPresent:
            if fileHolds(item) { ack(item) } else { sent[item.id] = Sent(page: page, at: clock(), confirmed: true) }
        case .notLoaded:
            sent[item.id] = nil; needsRetry = true
        case .stale:
            sent[item.id] = nil
            if staleRetried.insert(item.id).inserted { routeAll() } else { needsRetry = true }   // re-route once, then leave it to the retry
        }
        scheduleRetryIfNeeded()
    }

    /// True when the day's file already holds the note's line (never for a skipped day or an unreadable file).
    private func fileHolds(_ item: CaptureItem) -> Bool {
        guard let doc = try? store.load(item.day), !doc.isSkipped else { return false }
        return holds(item, CapturePlacement.jotKeys(in: doc.body, jotsHeading: prefs().jotsHeading))
    }

    /// The file path: nobody has this day open. Load once, add every pending note that is not in the page yet, save once, acknowledge.
    /// Any failure (folder missing or read-only, unreadable page, failed write) leaves every note pending.
    private func deliverToFile(_ day: String, _ items: [CaptureItem], heading: String) {
        do {
            let doc = try store.load(day)                                                // nil = no file yet
            var body = doc?.body ?? ""                                                   // a skip marker has an empty body: it is written over
            var changed = false
            for item in items where isPending(item.id) {
                if holds(item, CapturePlacement.jotKeys(in: body, jotsHeading: heading)) { continue }
                body = CapturePlacement.apply(item, to: body, jotsHeading: heading)
                changed = true
            }
            if changed { try store.save(day: day, body: body) }
            for item in items where isPending(item.id) { ack(item) }
        } catch {
            // stays in the journal: replayJournal() delivers it when the folder (or the page) is back
        }
    }

    private func scheduleRetryIfNeeded() {
        guard needsRetry, !retryScheduled, autoRetries < maxAutoRetries, !queue.isEmpty else { return }
        needsRetry = false; retryScheduled = true; autoRetries += 1
        scheduleRetry(retryDelay) { [weak self] in
            guard let self = self else { return }
            self.retryScheduled = false
            self.routeAll()
        }
    }
}
