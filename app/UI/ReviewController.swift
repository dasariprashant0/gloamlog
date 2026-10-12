// ReviewController.swift - the state and actions behind the Review screen (M3): scale, period, options, the built summary, and every
// copy, save, PDF and Share. AppModel owns one (`model.review`, created on first use). Main thread only.
//
// The period of Week, Month and Year is the one around `model.weekDate` (so the harnesses and ⌘[ ⌘] keep working); a Range keeps its own
// two dates. Scale, the two checkboxes and the grouping the person chose are remembered in UserDefaults (view state, not Settings).
// `summary` is rebuilt (rebuild()) whenever the period, the options or the data change: at once for up to 100 days and for a Year (states
// only), off the main thread for longer ranges, tagged with a generation so an old answer never replaces a newer one. Every copy and
// export is written from the summary that is shown, so the screen and the file always agree (M3-A7).
import SwiftUI
import AppKit
import Combine
import UniformTypeIdentifiers

enum CopyFlavor { case markdown, plain, rich }
enum SaveKind: String, CaseIterable, Equatable {
    case markdown, folder, zip, pdf
    var title: String {
        switch self {
        case .markdown: return "Markdown file with an assets folder"
        case .folder: return "Folder with the Markdown and its images"
        case .zip: return "Zip of that folder"
        case .pdf: return "PDF"
        }
    }
}

struct SaveFailure: Equatable {
    var name: String
    var kind: SaveKind
    var url: URL
    var text: String
    var detail: String?
}

final class ReviewController: ObservableObject {
    unowned let model: AppModel
    @Published var scale: ReviewScale { didSet { remember() } }
    @Published var rangeFrom: Date
    @Published var rangeTo: Date
    @Published var includeSkipped: Bool { didSet { remember(); rebuild() } }
    @Published var includeJots: Bool { didSet { remember(); rebuild() } }
    /// nil = the scale's own default: Day for a week, Section for longer periods.
    @Published var groupingChoice: WeekGrouping? { didSet { remember() } }
    @Published private(set) var summary: ReviewSummary?
    @Published var exportOpen = false
    @Published var busy = false
    @Published var savedLine: String?
    @Published var savedURL: URL?
    @Published var failure: SaveFailure?
    weak var shareAnchor: NSView?

    private var generation = 0
    private var bag = Set<AnyCancellable>()
    private enum Keys { static let scale = "review.scale", skipped = "review.skipped", jots = "review.jots", grouping = "review.grouping" }

    init(model: AppModel) {
        self.model = model
        let d = model.defaults
        scale = ReviewScale(rawValue: d.string(forKey: Keys.scale) ?? "") ?? .week
        includeSkipped = d.object(forKey: Keys.skipped) as? Bool ?? true
        includeJots = d.bool(forKey: Keys.jots)
        switch d.string(forKey: Keys.grouping) {
        case "day"?: groupingChoice = .day
        case "section"?: groupingChoice = .section
        default: groupingChoice = nil
        }
        let first = model.cal.dateInterval(of: .month, for: model.now)?.start ?? model.now
        rangeFrom = first; rangeTo = model.now
        // Anything that watches the model (menu labels, the sidebar) sees a change here too.
        objectWillChange.sink { [weak model] _ in DispatchQueue.main.async { model?.objectWillChange.send() } }.store(in: &bag)
        NotificationCenter.default.addObserver(forName: NSApplication.willTerminateNotification, object: nil, queue: .main) { _ in ReviewShare.removeAll() }
    }

    private func remember() {
        let d = model.defaults
        d.set(scale.rawValue, forKey: Keys.scale)
        d.set(includeSkipped, forKey: Keys.skipped)
        d.set(includeJots, forKey: Keys.jots)
        d.set(groupingChoice.map { $0 == .day ? "day" : "section" }, forKey: Keys.grouping)
    }

    // MARK: what is shown
    private var cal: Calendar { model.cal }
    var period: ReviewPeriod {
        scale == .range ? PeriodReview.range(from: rangeFrom, to: rangeTo, calendar: cal)
                        : PeriodReview.period(scale, containing: model.weekDate, calendar: cal)
    }
    /// Day for a week (whatever the Group by control of the v0.4 screen said), Section for anything longer, unless the person chose.
    var grouping: WeekGrouping { groupingChoice ?? (scale == .week ? model.weekGrouping : .section) }
    var options: ReviewOptions { ReviewOptions(grouping: grouping, includeSkipped: includeSkipped, includeJots: includeJots) }
    /// The built summary with the grouping as chosen now: grouping only changes how it is written, never what is built.
    var shown: ReviewSummary? {
        guard var s = summary else { return nil }
        s.options.grouping = grouping
        return s
    }
    var noun: String {
        switch scale { case .week: return "Week"; case .month: return "Month"; case .year: return "Year"; case .range: return "Period" }
    }
    var isCurrent: Bool { let p = period, t = model.today; return p.start <= t && t <= p.end }
    var catchUpTitle: String { scale == .week ? "Catch up this week" : (scale == .month ? "Catch up this month" : "Catch up this range") }

    // MARK: building
    func rebuild() {
        let p = period, o = options, states = model.states, settings = model.settings, cal = self.cal, now = model.now, store = model.store
        generation += 1
        let gen = generation
        func make() -> ReviewSummary {
            let docs = p.scale == .year ? [:] : ((try? store.documents(from: p.start, to: p.end)) ?? [:])
            return PeriodReview.build(period: p, documents: docs, states: states, settings: settings, options: o, calendar: cal, now: now)
        }
        if p.scale == .year || PeriodReview.dayCount(p, calendar: cal) <= 100 { summary = make(); return }
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            let s = make()
            DispatchQueue.main.async { if let self = self, gen == self.generation { self.summary = s } }
        }
    }

    /// The summary of the period on screen, built now if the last build is for another period or still running.
    func current() -> ReviewSummary {
        if let s = shown, s.period == period { return s }
        rebuild()
        if let s = shown, s.period == period { return s }
        let p = period
        let docs = p.scale == .year ? [:] : ((try? model.store.documents(from: p.start, to: p.end)) ?? [:])
        var s = PeriodReview.build(period: p, documents: docs, states: model.states, settings: model.settings, options: options, calendar: cal, now: model.now)
        s.options.grouping = grouping
        return s
    }

    // MARK: moving around
    private func changed() { savedLine = nil; savedURL = nil; failure = nil; model.refreshWeek() }

    func setScale(_ s: ReviewScale) {
        guard s != scale else { return }
        let p = period
        if s == .range {                                            // a range starts as the period that was showing, up to today
            rangeFrom = DayKey.date(p.start, cal) ?? model.now
            rangeTo = DayKey.date(min(p.end, model.today), cal) ?? model.now
        } else if scale == .range {
            model.weekDate = DayKey.date(p.end, cal) ?? model.now
        }
        scale = s
        changed()
    }

    /// Opens the Review (if it is not showing) at that scale: the menu's Week, Month, Year, Range.
    func show(_ s: ReviewScale) {
        if model.selection != .week { model.select(.week) }
        setScale(s)
    }

    func setRange(from: Date? = nil, to: Date? = nil) {
        if let f = from { rangeFrom = f; if rangeTo < f { rangeTo = f } }
        if let t = to { rangeTo = t; if rangeFrom > t { rangeFrom = t } }
        changed()
    }

    func canStep(_ dir: Int) -> Bool {
        guard let next = PeriodReview.shifted(period, by: dir, calendar: cal) else { return false }
        return dir < 0 ? next.end >= AppModel.earliestDay : next.start <= model.today
    }
    func step(_ dir: Int) {
        guard canStep(dir), let next = PeriodReview.shifted(period, by: dir, calendar: cal) else { return }
        if scale == .range { rangeFrom = DayKey.date(next.start, cal) ?? rangeFrom; rangeTo = DayKey.date(min(next.end, model.today), cal) ?? rangeTo }
        else { model.weekDate = DayKey.date(next.start, cal) ?? model.weekDate }
        changed()
    }
    func goThis() { model.weekDate = model.now; changed() }

    func lastWeek() {
        scale = .week
        model.weekDate = cal.date(byAdding: .weekOfYear, value: -1, to: model.now) ?? model.now
        changed()
    }
    func thisMonth() { scale = .month; model.weekDate = model.now; changed() }
    func openMonth(_ month: String) {                               // a row of the Year opens that month
        scale = .month
        model.weekDate = DayKey.date(month + "-01", cal) ?? model.now
        changed()
    }

    /// Catch up, limited to the shown period.
    func catchUp() {
        if scale == .week { model.catchUpThisWeek(); return }
        let p = period, hi = min(p.end, model.today)
        guard p.start <= hi else { return }
        model.select(.catchUp)
        model.catchRange = p.start...hi
    }

    // MARK: copying
    private func text(_ f: CopyFlavor, markdown md: String) {
        switch f {
        case .markdown: ReviewPasteboard.put(md)
        case .plain: ReviewPasteboard.put(MarkdownHTML.plain(md))
        case .rich: ReviewPasteboard.putRich(markdown: md)
        }
        model.copiedFlash = true
        if model.live { UIAnnounce.say("Copied") }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2) { [weak model] in model?.copiedFlash = false }
    }

    func copy(_ f: CopyFlavor) { text(f, markdown: ReviewFormats.markdown(current(), calendar: cal)) }

    /// The menu bar's "Copy this week's summary": the week around today, by day, as rich text, without opening the window.
    func copyThisWeekSummary(_ f: CopyFlavor = .rich) {
        let p = PeriodReview.period(.week, containing: model.now, calendar: cal)
        let docs = (try? model.store.documents(from: p.start, to: p.end)) ?? [:]
        var o = options
        o.grouping = groupingChoice ?? .day
        let s = PeriodReview.build(period: p, documents: docs, states: model.states, settings: model.settings, options: o, calendar: cal, now: model.now)
        text(f, markdown: ReviewFormats.markdown(s, calendar: cal))
    }

    /// The open page (what is in the editor now, saved or not) as Markdown, plain text or rich text.
    func copyPage(_ f: CopyFlavor) {
        guard let e = model.editor else { return }
        let put: (String) -> Void = { [weak self] md in self?.text(f, markdown: md) }
        if e.loaded, let b = model.bridgeStorage { b.read(token: e.token) { put($0 ?? e.latest) } } else { put(e.latest) }
    }

    // MARK: saving
    private func plan(_ s: ReviewSummary, name: String) -> ExportPlan {
        ExportBundle.plan(name: name, markdown: ReviewFormats.markdown(s, calendar: cal), dayCount: s.period.scale == .year ? s.counts.logged : s.days.count,
                          assets: model.store.assets)
    }
    /// What an export would hold (for the sheet): days and the images it would copy. Nothing is written; nil while a long range is still being read.
    func previewPlan() -> ExportPlan? {
        guard let s = shown, s.period == period else { return nil }
        return plan(s, name: ReviewFormats.fileName(s.period, calendar: cal))
    }

    /// Asks where, then writes. `.folder` asks for the folder to put the new folder in.
    func save(_ kind: SaveKind) {
        let s = current()
        let name = ReviewFormats.fileName(s.period, calendar: cal)
        if kind == .folder {
            let panel = NSOpenPanel()
            panel.canChooseDirectories = true; panel.canChooseFiles = false; panel.canCreateDirectories = true
            panel.prompt = "Choose"; panel.message = "Choose where to put the “\(name)” folder."
            panel.begin { [weak self] r in if r == .OK, let u = panel.url { self?.run(.folder, to: u, summary: s) } }
            return
        }
        let panel = NSSavePanel()
        panel.canCreateDirectories = true
        switch kind {
        case .markdown:
            panel.allowedContentTypes = [UTType(filenameExtension: "md") ?? .plainText]
            panel.nameFieldStringValue = name + ".md"
            panel.message = "The images this review uses go in an assets folder beside the file."
        case .zip: panel.allowedContentTypes = [.zip]; panel.nameFieldStringValue = name + ".zip"
        default: panel.allowedContentTypes = [.pdf]; panel.nameFieldStringValue = name + ".pdf"
        }
        panel.begin { [weak self] r in if r == .OK, let u = panel.url { self?.run(kind, to: u, summary: s) } }
    }

    func retry() { if let f = failure { run(f.kind, to: f.url, summary: current()) } }
    func chooseAnotherPlace() { if let f = failure { failure = nil; save(f.kind) } }
    func dismissResult() { savedLine = nil; savedURL = nil; failure = nil }
    func showInFinder() { if let u = savedURL { NSWorkspace.shared.activateFileViewerSelecting([u]) } }

    private func saved(_ line: String, _ url: URL) {
        failure = nil; savedLine = line; savedURL = url
        if model.live { UIAnnounce.say(line) }
    }
    private func failed(_ kind: SaveKind, _ url: URL) {
        savedLine = nil; savedURL = nil
        failure = kind == .pdf
            ? SaveFailure(name: url.lastPathComponent, kind: kind, url: url, text: "Couldn’t make the PDF.", detail: "Markdown is still available.")
            : SaveFailure(name: url.lastPathComponent, kind: kind, url: url, text: "Couldn’t save “\(url.lastPathComponent)”.",
                          detail: "The folder may be read-only or the disk full.")
        if model.live { UIAnnounce.say(failure?.text ?? "Couldn’t save", assertive: true) }
    }

    private func run(_ kind: SaveKind, to url: URL, summary s: ReviewSummary) {
        failure = nil
        let name = ReviewFormats.fileName(s.period, calendar: cal)
        switch kind {
        case .markdown, .folder, .zip:
            let p = plan(s, name: kind == .markdown ? url.deletingPathExtension().lastPathComponent : name)
            do {
                let out = try ExportBundle.write(p, layout: kind == .markdown ? .file(url) : (kind == .folder ? .folder(url) : .zip(url)))
                saved(p.resultLine, out)
            } catch { failed(kind, kind == .folder ? url.appendingPathComponent(name) : url) }
        case .pdf:
            busy = true
            let md = ReviewFormats.markdown(s, calendar: cal), title = ReviewFormats.title(s.period, calendar: cal), assets = model.store.assets
            DispatchQueue.global(qos: .userInitiated).async { [weak self] in         // images are scaled off the main thread
                let html = MarkdownHTML.document(title: title, md: md) { ReviewImages.dataURI($0, assets: assets) }
                DispatchQueue.main.async {
                    PDFMaker.make(html: html, to: url) { ok in
                        guard let self = self else { return }
                        self.busy = false
                        if ok { self.saved("Saved the PDF.", url) } else { self.failed(.pdf, url) }
                    }
                }
            }
        }
    }

    // MARK: sharing
    /// Writes the file only now, then opens the system share sheet on it. Nothing is sent until a target is chosen there.
    func share() {
        let s = current()
        let p = plan(s, name: ReviewFormats.fileName(s.period, calendar: cal))
        do {
            let file = try ReviewShare.fileForSharing(plan: p)
            guard let view = shareAnchor ?? NSApp.keyWindow?.contentView ?? NSApp.mainWindow?.contentView else { return }
            let anchor = shareAnchor == nil ? NSRect(x: view.bounds.maxX - 120, y: view.bounds.maxY - 40, width: 100, height: 28) : view.bounds
            NSSharingServicePicker(items: [file]).show(relativeTo: anchor, of: view, preferredEdge: .minY)
        } catch {
            failed(.markdown, URL(fileURLWithPath: p.name + ".md"))
        }
    }
}

/// An invisible view behind the Export button: the system share sheet points at it.
struct ShareAnchor: NSViewRepresentable {
    let review: ReviewController
    func makeNSView(context: Context) -> NSView {
        let v = NSView()
        DispatchQueue.main.async { review.shareAnchor = v }
        return v
    }
    func updateNSView(_ v: NSView, context: Context) { if review.shareAnchor !== v { DispatchQueue.main.async { review.shareAnchor = v } } }
}
