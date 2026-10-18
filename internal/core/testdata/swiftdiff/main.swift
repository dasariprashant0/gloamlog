// main.swift - the Swift side of the chunk B differential test (internal/core/swiftdiff_test.go).
// Reads cases (chunkb_cases.json), runs the SWIFT Core on each and writes the outputs (chunkb_swift.json). Regenerate with
// internal/core/testdata/swiftdiff/run.sh whenever a case changes; the Go test compares its own output with the stored file.
import Foundation

func esc(_ s: String) -> String { s.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\n", with: "\\n") }

func calendar(_ c: [String: Any]) -> Calendar {
    var cal = Calendar(identifier: .gregorian)
    cal.timeZone = TimeZone(identifier: c["tz"] as? String ?? "America/New_York")!
    cal.firstWeekday = c["fw"] as? Int ?? 2
    if let l = c["locale"] as? String { cal.locale = Locale(identifier: l) }
    return cal
}
/// "2026-10-07T14:32" read in the case's zone.
func local(_ s: String, _ cal: Calendar) -> Date {
    let p = s.split(whereSeparator: { "-T:".contains($0) }).map { Int($0)! }
    return cal.date(from: DateComponents(year: p[0], month: p[1], day: p[2], hour: p.count > 3 ? p[3] : 12, minute: p.count > 4 ? p[4] : 0))!
}
func scale(_ s: String) -> ReviewScale { ReviewScale(rawValue: s)! }
func period(_ c: [String: Any]) -> ReviewPeriod { ReviewPeriod(scale: scale(c["scale"] as! String), start: c["start"] as! String, end: c["end"] as! String) }
func ps(_ p: ReviewPeriod?) -> String { p.map { "\($0.start) \($0.end)" } ?? "<nil>" }
func item(_ c: [String: Any]) -> CaptureItem {
    CaptureItem(id: "x", ts: Date(timeIntervalSince1970: 0), day: "2026-10-07", text: c["text"] as! String, stamp: c["stamp"] as? String, todo: c["todo"] as? Bool ?? false, source: "hotkey")
}
func status(_ s: String) -> DayStatus { DayStatus(rawValue: s)! }

func dump(_ s: ReviewSummary) -> String {
    var o = ["counts \(s.counts.logged) \(s.counts.skipped) \(s.counts.started) \(s.counts.unlogged)"]
    for d in s.days { o.append("day|\(d.day)|\(d.status.rawValue)|\(d.words)|\(esc(d.skipReason))|\(esc(d.body))") }
    for g in s.groups {
        o.append("group|\(g.kind)|\(esc(g.title))")
        for i in g.items { o.append("  item|\(i.day)|\(esc(i.text))") }
    }
    for t in s.tasks.completed { o.append("done|\(t.day)|\(esc(t.text))") }
    for t in s.tasks.openAtEnd { o.append("open|\(esc(t))") }
    for m in s.months { o.append("month|\(m.month)|\(m.counts.logged) \(m.counts.skipped) \(m.counts.started) \(m.counts.unlogged)|\(m.isFuture)") }
    o.append("empty|\(s.isEmpty)")
    return o.joined(separator: "\n")
}

func run(_ c: [String: Any]) -> String {
    let kind = c["kind"] as! String
    let images: ((String) -> String?)? = (c["images"] as? Bool ?? false) ? { $0.hasSuffix(".png") ? "data:image/png;base64,AAA" : nil } : nil
    switch kind {
    case "mdbody": return MarkdownHTML.body(c["md"] as! String, images: images)
    case "mdplain": return MarkdownHTML.plain(c["md"] as! String)
    case "mdfragment": return MarkdownHTML.fragment(c["md"] as! String)
    case "mddoc": return MarkdownHTML.document(title: c["title"] as! String, md: c["md"] as! String, images: images)
    case "imagetargets": return MarkdownHTML.imageTargets(in: c["md"] as! String).joined(separator: "\n")
    case "render": return CapturePlacement.render(item(c))
    case "apply": return CapturePlacement.apply(item(c), to: c["body"] as! String, jotsHeading: c["heading"] as? String ?? "Jots")
    case "jotkeys": return CapturePlacement.jotKeys(in: c["body"] as! String, jotsHeading: c["heading"] as? String ?? "Jots").sorted().joined(separator: "\n")
    case "lastjotkey": return CapturePlacement.lastJotKey(in: c["body"] as! String, jotsHeading: c["heading"] as? String ?? "Jots") ?? "<nil>"
    case "key": return CapturePlacement.key(of: c["md"] as! String)
    case "period":
        let cal = calendar(c)
        return ps(PeriodReview.period(scale(c["scale"] as! String), containing: local(c["anchor"] as! String, cal), calendar: cal))
    case "shift":
        let cal = calendar(c)
        return ps(PeriodReview.shifted(period(c), by: c["n"] as! Int, calendar: cal))
    case "range":
        let cal = calendar(c)
        return ps(PeriodReview.range(from: local(c["from"] as! String, cal), to: local(c["to"] as! String, cal), calendar: cal))
    case "daycount":
        return String(PeriodReview.dayCount(period(c), calendar: calendar(c)))
    case "title": return ReviewFormats.title(period(c), calendar: calendar(c), dash: c["dash"] as? Bool ?? false)
    case "filename": return ReviewFormats.fileName(period(c), calendar: calendar(c))
    case "countsline":
        let n = c["counts"] as! [Int]
        return ReviewFormats.countsLine(ReviewCounts(logged: n[0], skipped: n[1], started: n[2], unlogged: n[3])) ?? "<nil>"
    case "tasksline":
        let t = TaskStats(completed: (0..<(c["done"] as! Int)).map { ReviewItem(day: "2026-10-05", text: "t\($0)") },
                          openAtEnd: (0..<(c["open"] as! Int)).map { "o\($0)" })
        return ReviewFormats.tasksLine(t) ?? "<nil>"
    case "rowline":
        let n = c["counts"] as! [Int]
        return ReviewFormats.rowLine(MonthRow(month: "2026-09", counts: ReviewCounts(logged: n[0], skipped: n[1], started: n[2], unlogged: n[3]), isFuture: c["future"] as? Bool ?? false))
    case "reminder":
        let cal = calendar(c)
        var s = Settings()
        s.weekdays = Set(c["weekdays"] as! [Int])
        s.reminderMinutes = c["minutes"] as! Int
        s.logStartDate = c["logStart"] as? String
        s.closedAppReminders = c["closedApp"] as? Bool ?? true
        var st = [String: DayStatus]()
        for (k, v) in (c["states"] as? [String: String]) ?? [:] { st[k] = status(v) }
        let now = local(c["now"] as! String, cal)
        let plan = ReminderScheduler.plan(now: now, calendar: cal, settings: s, states: st, horizon: c["horizon"] as? Int ?? 14)
        var o = plan.map { "\($0.id)|\(ReminderScheduler.key($0.fire))|\($0.title)|\($0.body)|\($0.category)" }
        if let n = ReminderScheduler.next(plan, now: now, calendar: cal) { o.append("next \(ReminderScheduler.label(n, calendar: cal))") } else { o.append("next <nil>") }
        return o.joined(separator: "\n")
    case "reminderdiff":
        let cal = calendar(c)
        func fire(_ a: [Int]) -> DateComponents { DateComponents(year: a[0], month: a[1], day: a[2], hour: a[3], minute: a[4]) }
        let planned = (c["planned"] as! [[String: Any]]).map {
            PlannedReminder(id: $0["id"] as! String, day: "", fire: fire($0["fire"] as! [Int]), title: $0["title"] as! String, body: $0["body"] as! String)
        }
        let pending = (c["pending"] as! [[String: Any]]).map {
            PendingReminder(id: $0["id"] as! String, fire: ($0["fire"] as? [Int]).map(fire), title: $0["title"] as! String, body: $0["body"] as! String,
                            category: $0["category"] as? String ?? ReminderScheduler.category)
        }
        _ = cal
        let d = ReminderScheduler.diff(planned: planned, pending: pending)
        return "add \(d.add.map { $0.id }.joined(separator: ","))\nremove \(d.remove.joined(separator: ","))"
    case "reminderlabel":
        let cal = calendar(c)
        let d = local(c["at"] as! String, cal)
        let t = ReminderScheduler.copy(at: d, calendar: cal)
        return "\(ReminderScheduler.label(d, calendar: cal))\n\(t.title)\n\(t.body)"
    case "review":
        let cal = calendar(c)
        var s = Settings()
        let st = c["settings"] as! [String: Any]
        s.template = st["template"] as? String ?? Settings.defaultTemplate
        s.weekdays = Set(st["weekdays"] as? [Int] ?? [2, 3, 4, 5, 6])
        s.minWords = st["minWords"] as? Int ?? 20
        s.carryOverHeadings = st["carry"] as? [String] ?? Settings.defaultCarryOverHeadings
        s.logStartDate = st["logStart"] as? String
        s.capture.jotsHeading = st["jotsHeading"] as? String ?? "Jots"
        s.capture.jotsCountTowardLogged = st["jotsCount"] as? Bool ?? false
        LogStore.jotsRules = (heading: s.capture.jotsHeading, countTowardLogged: s.capture.jotsCountTowardLogged)
        defer { LogStore.jotsRules = (heading: "Jots", countTowardLogged: false) }
        var docs = [String: DayDocument]()
        for d in c["docs"] as! [[String: Any]] {
            let day = d["day"] as! String
            docs[day] = DayDocument(day: day, body: d["body"] as? String ?? "", isSkipped: d["skipped"] as? Bool ?? false, skipReason: d["reason"] as? String ?? "")
        }
        var states = [String: DayStatus]()
        for (k, v) in (c["states"] as? [String: String]) ?? [:] { states[k] = status(v) }
        if (c["autostates"] as? Bool ?? false) { for (k, d) in docs { states[k] = d.isSkipped ? .skipped : d.status(minWords: s.minWords) } }
        let o = c["options"] as? [String: Any] ?? [:]
        let opts = ReviewOptions(grouping: (o["grouping"] as? String ?? "section") == "day" ? .day : .section,
                                 includeSkipped: o["skipped"] as? Bool ?? true, includeJots: o["jots"] as? Bool ?? false)
        let now = local(c["now"] as! String, cal)
        let p: ReviewPeriod
        if let a = c["anchor"] as? String { p = PeriodReview.period(scale(c["scale"] as! String), containing: local(a, cal), calendar: cal) }
        else { p = period(c) }
        let sum = PeriodReview.build(period: p, documents: docs, states: states, settings: s, options: opts, calendar: cal, now: now)
        return "period \(p.start) \(p.end)\n" + dump(sum) + "\n--- markdown\n" + ReviewFormats.markdown(sum, calendar: cal) + "--- plain\n" + ReviewFormats.plain(sum, calendar: cal)
            + "--- html\n" + ReviewFormats.html(sum, calendar: cal, images: nil) + "\n--- rich\n" + ReviewFormats.richHTML(sum, calendar: cal)
    case "splitjots":
        let r = PeriodReview.splitJots(c["body"] as! String, heading: c["heading"] as? String ?? "Jots")
        return "rest:\(esc(r.rest))\njots:\(esc(r.jots))"
    case "journalline":
        let it = CaptureItem(id: c["id"] as! String, ts: Date(timeIntervalSince1970: c["ts"] as! Double), day: c["day"] as! String, text: c["text"] as! String,
                             stamp: c["stamp"] as? String, todo: c["todo"] as? Bool ?? false, source: c["source"] as! String)
        return CaptureJournal.line(for: it) + "\n" + CaptureJournal.ackLine(it.id)
    case "normalise":
        guard let n = DayWriter.normalise(c["raw"] as! String, todoShorthand: c["shortcut"] as? Bool ?? true) else { return "<nil>" }
        return "\(n.todo)|\(esc(n.text))"
    default: return "<unknown kind \(kind)>"
    }
}

let args = CommandLine.arguments
let cases = try JSONSerialization.jsonObject(with: Data(contentsOf: URL(fileURLWithPath: args[1]))) as! [[String: Any]]
var out = [String: String]()
for c in cases { out[c["id"] as! String] = run(c) }
let data = try JSONSerialization.data(withJSONObject: out, options: [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes])
try data.write(to: URL(fileURLWithPath: args[2]))
print("ran \(cases.count) cases")
