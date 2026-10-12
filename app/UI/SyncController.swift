// SyncController.swift - sync through a shared folder (M5b, lean). Gloamlog runs no sync: the folder provider (iCloud Drive,
// Dropbox, ...) moves the files. This watches the folder, keeps count of what is still coming from the cloud and which conflict
// copies are here, and settles a conflict without ever losing a line.
//
//  * A change in the folder (file events, or a poll for network volumes) runs the same checks as the 30 s tick, at once: the
//    stamp-gated reload and the open page's external-change check (a clean page reloads, one with unsaved text keeps it).
//  * Days whose page is not on this Mac yet (iCloud placeholders, dataless files) are counted, asked for from the provider, and
//    never treated as "no log": LogStore refuses to read, write or replace them, and opening one shows "Downloading ...".
//  * A conflict copy ("2026-10-08 2.md", "... (conflicted copy).md") is offered in a sheet: Keep mine, Take theirs or Keep both.
//    The other copy is first written into <backups>/conflicts (checked byte for byte), then the chosen text goes through the normal
//    save path (which backs the old page up), and only then does the other copy leave the folder. Identical copies move silently.
import SwiftUI
import AppKit
import Combine

enum ConflictChoice { case keepMine, takeTheirs, keepBoth }

/// Both versions of a conflicted day, read from disk.
struct ConflictSides {
    let mine: DayDocument?                 // nil: this Mac has no page for the day
    let mineDate: Date?
    let theirs: DayDocument
    let theirsDate: Date?
    var mineBody: String { (mine?.isSkipped ?? true) ? "" : (mine?.body ?? "") }
}

struct SyncProblem: Error {
    let message: String
    init(_ message: String) { self.message = message }
}

final class SyncController: ObservableObject {
    unowned let model: AppModel
    @Published private(set) var kind: FolderKind = .local
    @Published private(set) var scan = SyncScan()
    @Published private var dismissed = Set<String>()          // conflict copies whose page banner was closed
    private(set) var blocked = Set<String>()                  // days whose page is not on this Mac at all yet (scan.blockedDays, kept for Catch up)

    private var watcher: FolderWatcher?
    private var scanner: FolderScanner?
    private let queue = DispatchQueue(label: "gloamlog.sync.scan", qos: .utility)
    private let downloadQueue = DispatchQueue(label: "gloamlog.sync.download", qos: .utility)
    private var generation = 0
    private var scanning = false, rescanAfter = false
    private var lastHandled = Date.distantPast
    private var handlePending = false
    private var requested: [String: Date] = [:]
    private var inFlight = Set<String>()
    private var timer: Timer?
    private var started = false
    private var cancellables = Set<AnyCancellable>()

    init(model: AppModel) {
        self.model = model
        // views that read `model.sync` are redrawn through the model
        objectWillChange.sink { [weak model] _ in model?.objectWillChange.send() }.store(in: &cancellables)
    }

    // MARK: lifecycle
    func start() {
        guard !started else { return }
        started = true
        // another log folder: watch that one (a hop to the next turn, so the model has swapped its store by then)
        model.$settings.map { $0.storageFolder }.removeDuplicates().dropFirst().receive(on: DispatchQueue.main)
            .sink { [weak self] _ in self?.restart() }.store(in: &cancellables)
        NotificationCenter.default.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in
            self?.refresh()
        }
        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in self?.refresh() }
        timer?.tolerance = 5
        restart()
    }

    func restart() {
        watcher?.stop(); watcher = nil
        generation += 1
        requested = [:]
        let dir = model.store.dir
        kind = FolderKind.detect(dir)
        scanner = FolderScanner(dir: dir, iCloud: kind.isICloud)
        if scan != SyncScan() { scan = SyncScan(); blocked = []; model.recompute() }
        watcher = FolderWatchers.start(folder: dir, kind: kind) { [weak self] in self?.changed() }
        refresh()
    }

    // MARK: reacting to the folder
    /// The folder changed. At most one round of work every 2 s (a provider restoring thousands of files must not stall the window).
    func changed() {
        let wait = 2 - Date().timeIntervalSince(lastHandled)
        if wait <= 0 { handle(); return }
        guard !handlePending else { return }
        handlePending = true
        DispatchQueue.main.asyncAfter(deadline: .now() + wait) { [weak self] in self?.handlePending = false; self?.handle() }
    }

    private func handle() {
        lastHandled = Date()
        model.reloadIfFolderChanged()            // day files added, removed or edited: states, sidebar, calendar
        model.editor?.checkExternalChange()      // the open page: a clean one reloads, one with unsaved text keeps it and says so
        refresh()
    }

    /// Looks at the folder again (off the main thread) for downloads and conflict copies.
    func refresh() {
        guard let scanner = scanner else { return }
        if scanning { rescanAfter = true; return }
        scanning = true
        let gen = generation, backups = model.backupDir
        queue.async { [weak self] in
            let result = SyncController.settleIdentical(scanner.scan(), dir: scanner.dir, backups: backups)
            DispatchQueue.main.async { self?.finish(result, generation: gen) }
        }
    }

    private func finish(_ result: SyncScan, generation gen: Int) {
        scanning = false
        if gen == generation { apply(result) }
        if rescanAfter { rescanAfter = false; refresh() }
    }

    private func apply(_ s: SyncScan) {
        let blockedBefore = blocked
        if s != scan { scan = s }
        blocked = s.blockedDays
        requestDownloads(s.remote, force: false)
        if blocked != blockedBefore { model.recompute() }                // Catch up leaves out days that are only still downloading
        // the open page was waiting for its day, and the day is here: open it for real
        if let e = model.editor, e.downloading, model.selection == .day(e.day), !model.store.isNotLocal(e.day) {
            model.reload()
            model.openDay(e.day, force: true)
        }
    }

    /// A conflict copy with exactly the same page as the real file is a plain duplicate: it moves to the safety copies and nobody is asked.
    private static func settleIdentical(_ s: SyncScan, dir: URL, backups: URL) -> SyncScan {
        guard !s.conflicts.isEmpty else { return s }
        var out = s
        let blocked = s.blockedDays
        out.conflicts = s.conflicts.filter { c in
            let copy = dir.appendingPathComponent(c.name), main = dir.appendingPathComponent(c.day + ".md")
            guard !blocked.contains(c.day), !CloudFile.isDataless(atPath: main.path),
                  let a = try? String(contentsOf: main, encoding: .utf8), let b = try? String(contentsOf: copy, encoding: .utf8),
                  MarkdownFormat.parse(day: c.day, raw: a) == MarkdownFormat.parse(day: c.day, raw: b),
                  let kept = try? ConflictFiles.keep(copy, in: backups, now: Date()),
                  (try? ConflictFiles.release(copy, keptAt: kept)) != nil else { return true }
            return false
        }
        return out
    }

    // MARK: downloads
    /// Asks the provider for every file that is not here yet (once every 2 minutes per file unless `force`).
    private func requestDownloads(_ remote: [SyncScan.Remote], force: Bool) {
        let now = Date(), iCloud = kind.isICloud, dir = model.store.dir
        for r in remote where !inFlight.contains(r.name) && (force || (requested[r.name].map { now.timeIntervalSince($0) > 120 } ?? true)) {
            requested[r.name] = now
            inFlight.insert(r.name)
            let url = dir.appendingPathComponent(r.name), name = r.name
            downloadQueue.async { [weak self] in
                CloudFile.requestDownload(url, iCloud: iCloud)
                DispatchQueue.main.async { self?.inFlight.remove(name) }
            }
        }
    }

    /// "Download all" in Settings.
    func downloadAll() {
        requestDownloads(scan.remote, force: true)
        refresh()
    }

    /// One day's page, asked for by name: the page for a day that is not here yet calls this when it opens (the scan may not have seen the day yet).
    func download(day: String) {
        requestDownloads([SyncScan.Remote(name: day + ".md", day: day, isConflictCopy: false, hasLocalText: false)], force: true)
        refresh()
    }

    // MARK: what the views show
    var conflicts: [ConflictCopy] { scan.conflicts }
    var pendingDays: [String] { scan.pendingDays }
    func isBlocked(_ day: String) -> Bool { blocked.contains(day) }
    /// "iCloud", or the provider's name, for "Downloading from iCloud".
    var sourceName: String {
        switch kind {
        case .iCloudDrive: return "iCloud"
        case .cloud(let p): return p
        default: return "your cloud folder"
        }
    }
    var bannerVisible: Bool { conflicts.contains { !dismissed.contains($0.name) } }
    func dismissBanner() { dismissed = Set(conflicts.map { $0.name }) }

    /// Opens the conflict sheet over the main window.
    func review() {
        model.showMainWindow(activate: true)
        model.sheet = .conflicts
    }

    // MARK: resolving a conflict
    private func modificationDate(_ url: URL) -> Date? { (try? FileManager.default.attributesOfItem(atPath: url.path))?[.modificationDate] as? Date }

    /// Both versions as they are on disk now. Throws a sentence for the user when either cannot be read yet.
    func readSides(_ c: ConflictCopy) throws -> ConflictSides {
        let store = model.store
        let copyURL = store.dir.appendingPathComponent(c.name)
        let mine: DayDocument?
        do { mine = try store.load(c.day) }
        catch LogError.notLocal { throw SyncProblem("This day is still downloading. Try again in a moment.") }
        catch { throw SyncProblem("Couldn't read this day. \(error.localizedDescription)") }
        guard !CloudFile.isDataless(atPath: copyURL.path), let raw = try? String(contentsOf: copyURL, encoding: .utf8) else {
            throw SyncProblem("The other copy can't be read yet. Try again in a moment.")
        }
        return ConflictSides(mine: mine, mineDate: modificationDate(store.url(for: c.day)),
                             theirs: MarkdownFormat.parse(day: c.day, raw: raw), theirsDate: modificationDate(copyURL))
    }

    /// Applies the choice. `completion` gets nil on success, else a sentence for the user (nothing was lost: a failure leaves the
    /// folder as it was, or with an extra safety copy). The open page is saved first, so what was just typed is part of "mine".
    func resolve(_ c: ConflictCopy, _ choice: ConflictChoice, completion: @escaping (String?) -> Void) {
        model.flushEditor { [weak self] in
            guard let self = self else { return }
            let problem = self.perform(c, choice)
            if problem == nil { self.afterResolved(c.day) }
            completion(problem)
        }
    }

    private func perform(_ c: ConflictCopy, _ choice: ConflictChoice) -> String? {
        let store = model.store
        let unsaved = "This page has edits that couldn't be saved yet. Fix that first, so nothing you typed is lost."
        if let e = model.editor, e.day == c.day, e.hasUnsavedEdits { return unsaved }
        if model.orphans.contains(where: { $0.day == c.day && $0.hasUnsavedEdits }) { return unsaved }
        let sides: ConflictSides
        do { sides = try readSides(c) } catch let p as SyncProblem { return p.message } catch { return error.localizedDescription }

        // the text the page will have; nil leaves the page file alone
        var newBody: String?
        switch choice {
        case .keepMine: newBody = nil
        case .takeTheirs:
            guard MarkdownBody.hasContent(sides.theirs.body) else { return "The other copy has no text to take." }
            newBody = sides.theirs.body
        case .keepBoth:
            let merged = BothVersions.merge(mine: sides.mineBody, theirs: sides.theirs.body)
            newBody = merged == sides.mineBody ? nil : merged
        }
        if choice == .keepMine && sides.mine == nil && MarkdownBody.hasContent(sides.theirs.body) {
            return "This Mac has no page for this day yet. Choose Take theirs or Keep both."
        }

        let copyURL = store.dir.appendingPathComponent(c.name)
        // 1. both versions go into the safety copies first (copied and checked; nothing is touched yet). The page's own backup before a save
        //    is "best effort" in the core, so when the page is about to change this makes sure this Mac's version is kept too.
        let kept: URL
        do {
            kept = try ConflictFiles.keep(copyURL, in: model.backupDir, now: model.clock())
            if newBody != nil && sides.mine != nil {
                _ = try ConflictFiles.keep(store.url(for: c.day), in: model.backupDir, now: model.clock(), name: "\(c.day) (this Mac).md")
            }
        } catch { return "Couldn't keep a safety copy of the versions, so nothing was changed." }
        // 2. the chosen text goes through the normal save path, which backs the page up first (forced, not throttled)
        if let body = newBody {
            let interval = store.minBackupInterval
            store.minBackupInterval = 0
            defer { store.minBackupInterval = interval }
            do { try store.save(day: c.day, body: body) }
            catch { return "Couldn't save the page. Nothing was removed. \(error.localizedDescription)" }
        }
        // 3. only now does the other copy leave the shared folder; if that fails it stays and is offered again, harmlessly
        try? ConflictFiles.release(copyURL, keptAt: kept)
        return nil
    }

    private func afterResolved(_ day: String) {
        let dir = model.store.dir
        scan.conflicts.removeAll { !FileManager.default.fileExists(atPath: dir.appendingPathComponent($0.name).path) }   // the sheet reads this list next
        model.didSave(day: day)
        if let e = model.editor, e.day == day, model.selection == .day(day) { e.abandon(); model.openDay(day, force: true) }
        refresh()
        if model.live { UIAnnounce.say("Conflict settled. The other version is kept in your safety copies.") }
    }
}
