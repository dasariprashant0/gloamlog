// SourcesController.swift - the state behind "Suggested from your work" (M6): the suggestions for the open day, the project list in
// Settings > Sources, the first-enable consent, and Insert. AppModel owns one (`model.sources`, created on first use). Main thread only.
//
// Reading happens off the main thread on one utility queue, only while a source is on AND the owner accepted the consent sheet,
// and only for ticked projects (ProjectPolicy). A scan can be cancelled (Stop, another day, a settings change) and carries a
// generation, so an old answer never replaces a newer one. Nothing is stored: results live in memory until the app quits; the next
// look re-reads (the Claude source keeps a per-file cache in memory, keyed by path, modified time and size).
//
// Insert goes through the editor's `appendToSection` (the safe-append path quick capture uses): ONE transaction on the live page,
// so the caret, the text being typed and undo are untouched, and ⌘Z removes exactly what was inserted. The bullets are already
// redacted (ActivityDigest) and markdown-escaped; the editor's `unlessPresent` and the "Added" check keep them from going in twice.
import SwiftUI
import AppKit
import Combine

enum SourceKind: String, Identifiable { case claude, git; var id: String { rawValue } }

final class SourcesController: ObservableObject {
    unowned let model: AppModel

    // the strip
    @Published private(set) var bullets: [SuggestedBullet] = []
    @Published private(set) var loading = false
    @Published private(set) var progressDone = 0
    @Published private(set) var progressTotal = 0
    @Published private(set) var loadedDay: String?
    @Published private(set) var notes: [String] = []
    @Published private(set) var hiddenLine: String?
    @Published private(set) var lastScanLine: String?
    @Published private(set) var inserting = false
    @Published var selected = Set<String>()
    @Published private(set) var hiddenDays: Set<String>
    @Published var undoDay: String?

    // Settings > Sources
    @Published private(set) var projects: [SourceProject] = []
    @Published private(set) var projectsLoading = false
    @Published private(set) var projectsLoaded = false
    @Published var consentFor: SourceKind?

    private let claude = ClaudeCodeSource()
    private let queue = DispatchQueue(label: "gloamlog.sources", qos: .utility)
    private var control: ScanControl?
    private var generation = 0
    private var projectGeneration = 0
    private var loadedKey = ""
    private var loadingKey = ""
    private var loadedAt = Date.distantPast
    private var discoveryKey = ""
    private var projectsVisible = false
    private var settleWork: DispatchWorkItem?
    private var bag = Set<AnyCancellable>()
    private static let hiddenKey = "dailylog.sources.hiddenDays"
    static let staleAfter: TimeInterval = 300
    /// Claude Code deletes sessions after about this many days unless its owner changed that.
    static let retentionDays = 30

    init(model: AppModel) {
        self.model = model
        hiddenDays = Set(model.defaults.stringArray(forKey: SourcesController.hiddenKey) ?? [])
        // Every change of the open page or of the Sources settings re-evaluates what to show (on the next turn: both publish before
        // the new value is stored, and nothing may be published from inside a view update).
        model.$editor.sink { [weak self] _ in DispatchQueue.main.async { self?.refresh() } }.store(in: &bag)
        model.$settings.map { $0.sources }.removeDuplicates().sink { [weak self] _ in
            DispatchQueue.main.async { self?.settingsChanged() }
        }.store(in: &bag)
    }

    // MARK: what is shown, and when
    private var prefs: SourcesPrefs { model.settings.sources }
    /// The day whose page is open and editable: the only day the strip is for.
    var shownDay: String? {
        guard case .day(let k) = model.selection, let e = model.editor, e.day == k, e.showsEditor else { return nil }
        return k
    }
    /// The strip exists for this day: a source is on, the consent was given, and the owner did not say "Not today".
    func isActive(for day: String) -> Bool { prefs.anyOn && prefs.consented && !hiddenDays.contains(day) }

    private func key(_ p: SourcesPrefs, _ day: String) -> String {
        "\(day)|\(p.claude)|\(p.git)|\(p.allProjects)|\(p.allowed.map { $0.lowercased() }.sorted().joined(separator: ","))|\(p.gitRoots.joined(separator: ","))"
    }

    private func settingsChanged() {
        let p = prefs
        // Ticking several projects in a row looks again once, shortly after the last tick.
        settleWork?.cancel()
        let w = DispatchWorkItem { [weak self] in self?.refresh() }
        settleWork = w
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.4, execute: w)
        let dk = "\(p.claude)|\(p.git)|\(p.gitRoots.joined(separator: ","))"          // ticking a project does not need a new search for projects
        if dk != discoveryKey { discoveryKey = dk; if projectsVisible { refreshProjects() } }
    }

    /// Looks at the open day unless it already did, recently, with the same settings. `force` always looks again.
    func refresh(force: Bool = false) {
        guard let day = shownDay, isActive(for: day) else { cancel(); if loadedDay != nil { clear() }; return }
        let k = key(prefs, day)
        if !force, !loading, k == loadedKey, loadedDay == day, Date().timeIntervalSince(loadedAt) < SourcesController.staleAfter { return }
        if !force, loading, k == loadingKey { return }
        load(day: day, key: k)
    }

    private func clear() {
        bullets = []; notes = []; hiddenLine = nil; selected = []; loadedDay = nil; loadedKey = ""; loadingKey = ""; loading = false
    }

    func cancel() {
        control?.cancel(); control = nil
        generation += 1
        if loading { loading = false }
    }

    private func load(day: String, key k: String) {
        control?.cancel()
        generation += 1
        let gen = generation
        let ctl = ScanControl()
        control = ctl
        loadingKey = k
        if loadedDay != day { bullets = []; notes = []; hiddenLine = nil; selected = [] }
        loading = true; progressDone = 0; progressTotal = 0
        ctl.onProgress = { [weak self] done, total in
            DispatchQueue.main.async { if let s = self, s.generation == gen { s.progressDone = done; s.progressTotal = total } }
        }
        let p = prefs, cal = model.cal, today = model.today, claude = self.claude
        queue.async { [weak self] in
            let (digest, stats, notices) = SourcesController.scan(day: day, prefs: p, cal: cal, claude: claude, ctl: ctl)
            DispatchQueue.main.async {
                guard let s = self, s.generation == gen, !ctl.isCancelled else { return }
                s.finish(day: day, key: k, today: today, prefs: p, digest: digest, stats: stats, notices: notices)
            }
        }
    }

    /// One look at `day`: Git first (when a commit is seen by both sources, the one with the right time and subject wins), then Claude Code.
    private static func scan(day: String, prefs p: SourcesPrefs, cal: Calendar, claude: ClaudeCodeSource, ctl: ScanControl) -> (DigestResult, ScanStats, [String]) {
        var items = [ActivityItem](), stats = ScanStats(), notices = [String]()
        func absorb(_ r: SourceResult) { items += r.items; stats.add(r.stats); notices += r.notices }
        if p.git && !ctl.isCancelled { absorb(GitSource(roots: p.gitRoots).activity(day: day, calendar: cal, policy: p.policy, control: ctl)) }
        if p.claude && !ctl.isCancelled { absorb(claude.activity(day: day, calendar: cal, policy: p.policy, control: ctl)) }
        return (ActivityDigest.bullets(from: items), stats, notices)
    }

    private func finish(day: String, key k: String, today: String, prefs p: SourcesPrefs, digest: DigestResult, stats: ScanStats, notices: [String]) {
        var notes = notices
        if stats.skipped > 0 { notes.append("\(Fmt.plural(stats.skipped, "record")) skipped") }
        if stats.formatLooksChanged { notes.append("Claude Code's file format may have changed, so this list may be incomplete.") }
        if digest.bullets.isEmpty && p.claude, let d = DayKey.date(day, model.cal), let t = DayKey.date(today, model.cal),
           let ago = model.cal.dateComponents([.day], from: d, to: t).day, ago > SourcesController.retentionDays {
            notes.append("Claude Code no longer keeps sessions this old.")
        }
        bullets = digest.bullets
        hiddenLine = Redactor.summary(digest.hidden)
        self.notes = notes
        selected = selected.intersection(Set(digest.bullets.map { $0.id }))
        loadedDay = day; loadedKey = k; loadedAt = Date(); loading = false; control = nil
        lastScanLine = "Last look: \(DayKey.format(day, "EEE d MMM", model.cal)), \(Fmt.plural(stats.files, "file")) read, \(Fmt.plural(stats.skipped, "record")) skipped."
    }

    // MARK: "Not today"
    func hideToday() {
        guard let d = loadedDay ?? shownDay else { return }
        cancel()
        hiddenDays.insert(d)
        model.defaults.set(Array(hiddenDays.sorted().suffix(90)), forKey: SourcesController.hiddenKey)
        undoDay = d
        DispatchQueue.main.asyncAfter(deadline: .now() + 8) { [weak self] in if self?.undoDay == d { self?.undoDay = nil } }
    }
    func undoHide(_ d: String) {
        hiddenDays.remove(d)
        model.defaults.set(Array(hiddenDays.sorted().suffix(90)), forKey: SourcesController.hiddenKey)
        undoDay = nil
        refresh(force: true)
    }

    // MARK: insert
    func toggleAll(_ items: [SuggestedBullet]) {
        let ids = Set(items.map { $0.id })
        selected = selected.isSuperset(of: ids) ? selected.subtracting(ids) : selected.union(ids)
    }

    func insertSelected() { insert(bullets.filter { selected.contains($0.id) }) }

    /// Adds `items` as bullets under the Settings heading, in one editor transaction.
    func insert(_ items: [SuggestedBullet], day: String? = nil) {
        guard !items.isEmpty, !inserting, let e = model.editor, e.loaded, e.showsEditor, e.day == (day ?? loadedDay),
              let b = model.bridgeIfCreated, b.isReady else { return }
        let page = ActivityDigest.plainText(e.latest)
        let fresh = items.filter { !ActivityDigest.isAdded($0, inPlainPage: page) }
        let ids = Set(items.map { $0.id })
        guard !fresh.isEmpty else { selected.subtract(ids); return }
        inserting = true
        b.appendToSection(token: e.token, heading: prefs.headingText, markdown: ActivityDigest.markdown(fresh), unlessPresent: true) { [weak self, weak e] outcome in
            guard let self = self else { return }
            self.inserting = false
            switch outcome {
            case .appended(let md):
                e?.programmaticEdit(to: md)                           // the editor's own text is the truth; autosave writes it
                self.selected.subtract(ids)
                if self.model.live { UIAnnounce.say("Added \(Fmt.plural(fresh.count, "line"))") }
            case .alreadyPresent: self.selected.subtract(ids)
            case .stale, .failed: self.model.pageNotice = "Couldn't add those lines. Your page is unchanged. Try again."
            }
        }
    }

    // MARK: auto-add
    // While "Add them for me" is on, today's lines go onto today's page by themselves: every 2 minutes, when Claude Code says a
    // session ended (the hook runs `open -g gloamlog://refresh`), and at launch. It reads every session and tab, not only the
    // folder in front of you. The session summary line is left out of today (its time and count would be stale the moment it is written).
    // An open page takes the lines through the editor; a day nobody has open is written to its file, once per line; a page that is
    // open but not ready is skipped until the next look.
    private var autoBusy = false
    private var autoAt = Date.distantPast
    private var autoTimer: Timer?

    func startAuto() {
        autoTimer?.invalidate()
        autoTimer = Timer.scheduledTimer(withTimeInterval: 120, repeats: true) { [weak self] _ in self?.autoTick() }
        DispatchQueue.main.asyncAfter(deadline: .now() + 3) { [weak self] in self?.autoTick(force: true) }
    }

    func autoTick(force: Bool = false) {
        let p = prefs
        guard p.autoAdd, p.anyOn, p.consented, !autoBusy, model.live else { return }
        guard force || Date().timeIntervalSince(autoAt) >= 20 else { return }
        autoBusy = true; autoAt = Date()
        let day = model.today, cal = model.cal, claude = self.claude
        queue.async { [weak self] in
            let (digest, _, _) = SourcesController.scan(day: day, prefs: p, cal: cal, claude: claude, ctl: ScanControl())
            DispatchQueue.main.async {
                guard let s = self else { return }
                s.autoBusy = false
                s.autoWrite(day: day, bullets: digest.bullets.filter { $0.kind != .session })
            }
        }
    }

    private func autoWrite(day: String, bullets: [SuggestedBullet]) {
        guard !bullets.isEmpty, prefs.autoAdd, day == model.today else { return }
        let hasEditor = model.editor?.day == day || model.orphans.contains { $0.day == day }
        if hasEditor {
            if model.editor?.day == day { insert(bullets, day: day) }          // the live page owns its text; hidden pages wait for the next look
            return
        }
        let doc: DayDocument?
        do { doc = try model.store.load(day) } catch { return }                // a missing file is a new page; an unreadable one is left alone
        if doc?.isSkipped == true { return }
        var body = doc?.body ?? ""
        let page = ActivityDigest.plainText(body)
        let fresh = bullets.filter { !ActivityDigest.isAdded($0, inPlainPage: page) }
        guard !fresh.isEmpty else { return }
        for b in fresh {
            let item = CaptureItem(id: UUID().uuidString, ts: Date(), day: day, text: ActivityDigest.escape(b.text), stamp: nil, todo: false, source: "claude")
            body = CapturePlacement.apply(item, to: body, jotsHeading: prefs.headingText)
        }
        if (try? model.store.save(day: day, body: body)) != nil { model.fileChangedUnderneath(day) }
    }

    /// The Claude Code Stop hook as settings.json wants it; only runs when Gloamlog is open.
    func copyHook() {
        let json = """
        "hooks": { "Stop": [ { "hooks": [ { "type": "command", "command": "pgrep -x Gloamlog >/dev/null && open -g gloamlog://refresh || true" } ] } ] }
        """
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(json, forType: .string)
    }

    // MARK: Settings > Sources
    func setSource(_ kind: SourceKind, on: Bool) {
        if on && !prefs.consented { consentFor = kind; return }          // nothing is switched on before the sheet is accepted
        apply(kind, on)
    }
    private func apply(_ kind: SourceKind, _ on: Bool) {
        switch kind {
        case .claude: model.settings.sources.claude = on
        case .git: model.settings.sources.git = on
        }
    }
    func acceptConsent() {
        guard let k = consentFor else { return }
        model.settings.sources.consented = true
        apply(k, true)
        consentFor = nil
    }
    func declineConsent() { consentFor = nil }

    func setAllowed(_ root: String, _ on: Bool) { model.settings.sources.setAllowed(root, on) }
    func removeGitRoot(_ path: String) { model.settings.sources.gitRoots.removeAll { $0 == path } }

    /// "Add folder…": a repository, or a folder that holds repositories (looked at up to 3 levels down).
    func addGitFolder() {
        let p = NSOpenPanel()
        p.canChooseDirectories = true; p.canChooseFiles = false; p.canCreateDirectories = false
        p.allowsMultipleSelection = false; p.prompt = "Add"
        p.message = "Choose a Git repository, or a folder that holds your repositories."
        guard p.runModal() == .OK, let url = p.url else { return }
        let path = SourcePath.normalize(url.path)
        if !model.settings.sources.gitRoots.contains(where: { SourcePath.normalize($0).lowercased() == path.lowercased() }) {
            model.settings.sources.gitRoots.append(path)
        }
    }

    /// Called by the pane while it is on screen.
    func paneAppeared() { projectsVisible = true; refreshProjects() }
    func paneDisappeared() { projectsVisible = false }

    func refreshProjects() {
        let p = prefs
        guard p.anyOn, p.consented else { projects = []; projectsLoaded = false; projectsLoading = false; return }
        projectGeneration += 1
        let gen = projectGeneration
        projectsLoading = true
        let claude = self.claude
        queue.async { [weak self] in
            let ctl = ScanControl()
            var by = [String: SourceProject]()
            func merge(_ x: SourceProject) {
                var m = by[x.id] ?? x
                m.sources.formUnion(x.sources)
                if let a = x.lastActive, a > (m.lastActive ?? .distantPast) { m.lastActive = a }
                by[x.id] = m
            }
            if p.claude { claude.projects(control: ctl).forEach(merge) }
            if p.git { GitSource(roots: p.gitRoots).projects(control: ctl).forEach(merge) }
            for a in p.allowed {                                      // a ticked (or skipped) project that is not found stays, so it can be changed back
                let n = SourcePath.normalize(a)
                if by[n.lowercased()] == nil { by[n.lowercased()] = SourceProject(root: n, name: SourcePath.name(of: n), sources: [], lastActive: nil) }
            }
            let policy = p.policy
            let list = by.values.sorted { x, y in
                let ax = policy.root(for: x.root) != nil, ay = policy.root(for: y.root) != nil
                if ax != ay { return ax }
                if x.lastActive != y.lastActive { return (x.lastActive ?? .distantPast) > (y.lastActive ?? .distantPast) }
                return x.name.lowercased() < y.name.lowercased()
            }
            DispatchQueue.main.async {
                guard let s = self, s.projectGeneration == gen else { return }
                s.projects = list; s.projectsLoading = false; s.projectsLoaded = true
            }
        }
    }
}
