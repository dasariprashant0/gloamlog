// LogStore.swift - reads/writes the markdown day files in the storage folder (one free-form page per day).
//
// API (all throw LogError unless noted):
//   LogStore(dir:, backupDir: nil, minBackupInterval: 0, clock: Date.init)
//                                     backupDir = where safety copies go (nil = no backups). The UI passes
//                                     ~/Library/Application Support/Gloamlog/backups so they never sync with the logs.
//   ensureFolder()                    create the folder (onboarding / "Choose folder")
//   checkFolder()                     .folderMissing / .folderNotWritable
//   listDays() -> [String]            day keys that have a file, newest first (the assets/ folder, a backups folder inside
//                                     the storage folder and foreign files are ignored)
//   load(day) -> DayDocument?         nil = no file. v0.1/v0.2 files load as plain markdown (see MarkdownFormat)
//                                     Throws .notLocal while the day's page is still in the cloud (iCloud placeholder, dataless file):
//                                     so do save/skip/unskip/restoreBackup. isNotLocal(day) says so without reading (Core/Sync/CloudFile.swift)
//   save(day:body:) -> SaveOutcome    atomic; writes "# day\n\n<body>\n". Replaces a skip marker ("write a log anyway").
//                                     A whitespace-only body deletes an existing log file instead (an emptied page is "no log")
//                                     and never touches a skip marker. Identical content is not rewritten. The UI must only
//                                     call this after a real edit. @discardableResult: see Backups.
//   skip(day, reason:)                writes "> Skipped: reason"; throws .hasContent if the day has writing (headings alone are overwritten)
//   skipRange(from:to:reason:weekdays:calendar:) -> [String]   one file per scheduled day; days with writing are left alone
//   unskip(day) -> Bool               deletes the marker only
//   fileStates(minWords:) -> [String: DayStatus]   logged/partial/skipped per file; days with 0 words are omitted
//                                     (input for Status/Streak/Heatmap/WeeklyReview). Follows LogStore.jotsRules: jots are not words
//                                     (unless countTowardLogged), and a day with only jots is .partial, so it is in the map.
//   documents(from:to:) -> [String: DayDocument] / allDocuments()
//   search(query, heading:) -> [SearchHit]         see Search.swift
//   folderSummary() -> FolderSummary  counts for Settings > Storage
//   folderStamp() -> String           changes when a day file is added, removed, renamed, edited or touched; stat only, no reads
//   assets -> AssetStore              images for this folder
//
// Writes (day files and backups) go through AtomicFile: a hidden ".gloamlog-tmp-<pid>-<uuid>" file, then rename. A SIGKILL can leave
//   at most that one hidden file, which the next store's first write removes (tests/run-kill-tests.sh proves it: 40 killed rounds).
//   skipDays/unskipDays (batch skip + undo, Calendar/CatchUp.swift) and folderStamp() (change detection without reading files) live
//   here and there; see their comments.
//
// Backups (safety net so autosave can never destroy a day of writing; only when backupDir is set):
//   Before save() overwrites OR deletes an existing day file, the old file is copied byte-for-byte to
//   <backupDir>/<yyyy-MM-dd>/<yyyyMMdd-HHmmss>.md (UTC time stamp, atomic write) and only the newest 10 per day are kept
//   (LogStore.maxBackupsPerDay). No backup when: the new content equals the file, the file is a skip marker, or the file
//   has nothing but its title. Names always increase (a same-second save or a clock set back still sorts newest), so
//   pruning never drops the latest copy. minBackupInterval > 0 (seconds) makes overwrites skip the copy while the
//   newest backup is younger than that, so the 10 slots span longer in a typing session (deletes/restores always copy).
//   A failing backup NEVER fails the save: the write/delete still happens and the problem is reported twice,
//     save(...) -> SaveOutcome{backedUp, backupWarning}   for that call, and
//     lastBackupWarning: String?                          latest backup/prune problem, nil after the next good backup.
//   Backups are never listed by listDays/fileStates/search/documents/folderSummary.
//   listBackups(day:) -> [BackupInfo{date, url, words}]   newest first; [] without backupDir (does not throw)
//   restoreBackup(day:backup:) -> DayDocument             first backs up the current file (throws .backupFailed if that
//                                                         is impossible and nothing was changed), then writes the old copy back.
//                                                         .backupsDisabled without backupDir; .io for an unknown/forged backup.
import Foundation

enum LogError: Error, Equatable {
    case folderMissing(String)
    case folderNotWritable(String)
    case badDay(String)
    case hasContent(String)          // refusing to overwrite a day that has writing
    case unsupportedAsset(String)    // extension not in png/jpg/jpeg/gif/webp
    case assetTooLarge(Int)          // bytes; max is AssetStore.maxBytes (20 MB)
    case emptyAsset
    case badPath(String)             // e.g. assets/ is a symlink leaving the storage folder
    case backupsDisabled             // restoreBackup on a store created without backupDir
    case backupFailed(String)        // restoreBackup could not make its safety copy of the current page (nothing changed)
    case notLocal(String)            // the day's page is still in the cloud (iCloud placeholder or dataless file): never read, changed or replaced until it is here
    case io(String)
}

struct FolderSummary: Equatable { var logs = 0, skipped = 0, unrecognized = 0 }

/// One safety copy of a day file. `date` is the (UTC) time stamp in its file name; `words` counts the saved page.
struct BackupInfo: Equatable {
    var date: Date
    var url: URL
    var words: Int
}

/// What save() did besides writing: whether the old file was copied, and any non-fatal backup problem to show the user.
struct SaveOutcome: Equatable {
    var backedUp = false
    var backupWarning: String?
}

/// Backup file names: "yyyyMMdd-HHmmss.md" in UTC, so DST and time zones cannot reorder or duplicate them.
private enum BackupName {
    static let ext = ".md"
    private static var calendar: Calendar {
        var c = Calendar(identifier: .gregorian); c.timeZone = TimeZone(identifier: "GMT")!; return c
    }
    static func stem(_ d: Date) -> String {
        let c = calendar.dateComponents([.year, .month, .day, .hour, .minute, .second], from: d)
        return String(format: "%04ld%02ld%02ld-%02ld%02ld%02ld", c.year ?? 0, c.month ?? 0, c.day ?? 0, c.hour ?? 0, c.minute ?? 0, c.second ?? 0)
    }
    static func date(fromFile name: String) -> Date? {
        guard name.hasSuffix(ext) else { return nil }
        let u = Array(name.dropLast(ext.count).utf8)
        guard u.count == 15, u[8] == 45, u.enumerated().allSatisfy({ $0.offset == 8 || ($0.element >= 48 && $0.element <= 57) }) else { return nil }
        func n(_ a: Int, _ b: Int) -> Int { Int(String(decoding: u[a..<b], as: UTF8.self)) ?? 0 }
        let parts = DateComponents(year: n(0, 4), month: n(4, 6), day: n(6, 8), hour: n(9, 11), minute: n(11, 13), second: n(13, 15))
        guard let d = calendar.date(from: parts), stem(d) == String(decoding: u, as: UTF8.self) else { return nil }
        return d
    }
}

final class LogStore {
    static let maxBackupsPerDay = 10

    /// The Jots rule behind "logged" (M2): which heading holds the jots, and whether their words count toward a day being logged.
    /// The app sets it from Settings.capture; DayDocument.words/status, fileStates and everything built on them follow it.
    /// Both spellings compile: `LogStore.jotsRules = (heading: "Jots", countTowardLogged: false)` and `LogStore.JotsRules(heading:countTowardLogged:)`.
    /// Guarded by a lock: background readers (fileStates off the main thread) never see a half-written value.
    typealias JotsRules = (heading: String, countTowardLogged: Bool)
    private static let jotsRulesLock = NSLock()
    private static var storedJotsRules: JotsRules = (heading: "Jots", countTowardLogged: false)
    static var jotsRules: JotsRules {
        get { jotsRulesLock.lock(); defer { jotsRulesLock.unlock() }; return storedJotsRules }
        set { jotsRulesLock.lock(); defer { jotsRulesLock.unlock() }; storedJotsRules = newValue }
    }

    let dir: URL
    let backupDir: URL?
    var minBackupInterval: TimeInterval
    private(set) var lastBackupWarning: String?
    private let clock: () -> Date
    private let fm = FileManager.default

    init(dir: URL, backupDir: URL? = nil, minBackupInterval: TimeInterval = 0, clock: @escaping () -> Date = { Date() }) {
        self.dir = dir; self.backupDir = backupDir; self.minBackupInterval = minBackupInterval; self.clock = clock
    }

    var assets: AssetStore { AssetStore(dir: dir) }

    // MARK: folder
    func ensureFolder() throws {
        do { try fm.createDirectory(at: dir, withIntermediateDirectories: true) }
        catch { throw LogError.folderNotWritable(dir.path) }
        try checkFolder()
    }
    func checkFolder(writable: Bool = true) throws {
        var isDir: ObjCBool = false
        guard fm.fileExists(atPath: dir.path, isDirectory: &isDir), isDir.boolValue else { throw LogError.folderMissing(dir.path) }
        if writable && !fm.isWritableFile(atPath: dir.path) { throw LogError.folderNotWritable(dir.path) }
    }

    func url(for day: String) -> URL { dir.appendingPathComponent(day + ".md") }
    private func validate(_ day: String) throws { if !DayKey.isWellFormed(day) { throw LogError.badDay(day) } }

    /// True when the day's page is in the folder but not (fully) on this Mac yet: an iCloud placeholder or a dataless file (Core/Sync/CloudFile.swift).
    /// load, save, skip, unskip and restore refuse such a day with LogError.notLocal, so a day that is merely still downloading is never
    /// read as "no log" and never gets a page created over it.
    func isNotLocal(_ day: String) -> Bool { CloudFile.isNotLocal(dir: dir, day: day) }
    private func requireLocal(_ day: String) throws { if isNotLocal(day) { throw LogError.notLocal(day) } }
    /// load() for loops over many days: a day that is not here yet is left out instead of failing the whole call.
    private func loadIfLocal(_ day: String) throws -> DayDocument? {
        do { return try load(day) } catch LogError.notLocal { return nil }
    }

    // MARK: read
    private func dayFiles() throws -> (days: [String], other: Int) {
        try checkFolder(writable: false)
        let names: [String]
        do { names = try fm.contentsOfDirectory(atPath: dir.path) } catch { throw LogError.io(error.localizedDescription) }
        var days = [String](), other = 0
        var reserved: Set<String> = [AssetStore.folderName]
        if let b = backupDir, b.deletingLastPathComponent().standardizedFileURL.path == dir.standardizedFileURL.path {
            reserved.insert(b.lastPathComponent)   // a backups folder inside the storage folder is not "unrecognised"
        }
        for n in names where !n.hasPrefix(".") && !reserved.contains(n) {
            if n.hasSuffix(".md"), DayKey.isWellFormed(String(n.dropLast(3))) { days.append(String(n.dropLast(3))) } else { other += 1 }
        }
        return (days.sorted(by: >), other)
    }
    func listDays() throws -> [String] { try dayFiles().days }

    func load(_ day: String) throws -> DayDocument? {
        try validate(day)
        try checkFolder(writable: false)
        try requireLocal(day)
        guard fm.fileExists(atPath: url(for: day).path) else { return nil }
        let text: String
        do { text = try String(contentsOf: url(for: day), encoding: .utf8) } catch { throw LogError.io(error.localizedDescription) }
        return MarkdownFormat.parse(day: day, raw: text)
    }

    func fileStates(minWords: Int) throws -> [String: DayStatus] {
        var out = [String: DayStatus]()
        for d in try listDays() {
            guard let doc = try? load(d) else { continue }
            let s = doc.status(minWords: minWords)
            if s != .missed { out[d] = s }
        }
        return out
    }
    func documents(from: String, to: String) throws -> [String: DayDocument] {
        var out = [String: DayDocument]()
        for d in try listDays() where d >= from && d <= to { if let e = try loadIfLocal(d) { out[d] = e } }
        return out
    }
    func allDocuments() throws -> [DayDocument] { try listDays().compactMap { try loadIfLocal($0) } }

    func folderSummary() throws -> FolderSummary {
        var s = FolderSummary()
        let f = try dayFiles(); s.unrecognized = f.other
        for d in f.days { if let e = try loadIfLocal(d) { if e.isSkipped { s.skipped += 1 } else if e.hasContent { s.logs += 1 } } }
        return s
    }

    /// A string that changes whenever a day file is added, removed, renamed, edited or touched: the sorted (name, mtime in ns, size)
    /// of the YYYY-MM-DD.md files, one per line. Only readdir and stat are called, no file is opened or read (3,650 files take a few
    /// ms even in an unoptimised build), so the UI can poll it and reload only when it changed. assets/, a backups folder, hidden,
    /// foreign files and folders are ignored; a symlinked day file counts through its target. A missing folder throws like the other reads.
    func folderStamp() throws -> String {
        try checkFolder(writable: false)
        guard let handle = opendir(dir.path) else { throw LogError.io(String(cString: strerror(errno))) }
        defer { closedir(handle) }
        let fd = dirfd(handle)
        var rows = [(key: Int, line: String)]()
        while let entry = readdir(handle) {
            withUnsafePointer(to: &entry.pointee.d_name) { field in
                field.withMemoryRebound(to: CChar.self, capacity: 14) { name in
                    guard let key = LogStore.dayNumber(ofFileName: name) else { return }       // not YYYY-MM-DD.md: never even stat'ed
                    var st = stat()
                    guard fstatat(fd, name, &st, 0) == 0, (st.st_mode & S_IFMT) == S_IFREG else { return }
                    // a dataless file (its bytes arrive later from the cloud) gets a marker, so the arrival changes the stamp
                    let cloud = (st.st_flags & CloudFile.datalessFlag) != 0 ? " dataless" : ""
                    rows.append((key, "\(String(cString: name)) \(st.st_mtimespec.tv_sec).\(st.st_mtimespec.tv_nsec) \(st.st_size)\(cloud)"))
                }
            }
        }
        rows.sort { $0.key < $1.key }
        return rows.map { $0.line }.joined(separator: "\n")
    }
    /// 20261005 for the NUL-terminated name "2026-10-05.md" (digits at the date positions, nothing before or after); nil for any other name.
    private static func dayNumber(ofFileName p: UnsafePointer<CChar>) -> Int? {
        guard p[4] == 45, p[7] == 45, p[10] == 46, p[11] == 109, p[12] == 100, p[13] == 0 else { return nil }   // - - . m d NUL
        var n = 0
        for i in [0, 1, 2, 3, 5, 6, 8, 9] {
            let digit = Int(p[i]) - 48
            guard digit >= 0 && digit <= 9 else { return nil }
            n = n * 10 + digit
        }
        return n
    }

    // MARK: write
    private var swept = false
    /// The first write or backup of a store removes the temp files a killed writer left behind (AtomicFile), in the log folder and the
    /// backups folder. Reading never sweeps. Once per store, i.e. once per launch.
    private func sweepOnce() {
        guard !swept else { return }
        swept = true
        AtomicFile.sweep(in: dir)
        if let b = backupDir { AtomicFile.sweep(in: b) }
    }

    private func write(_ text: String, day: String) throws {
        try checkFolder()
        try requireLocal(day)
        sweepOnce()
        do { try AtomicFile.write(Data(text.utf8), to: url(for: day)) }
        catch let e as NSError {
            let noPermission = (e.domain == NSCocoaErrorDomain && (e.code == NSFileWriteNoPermissionError || e.code == NSFileWriteVolumeReadOnlyError))
                || (e.domain == NSPOSIXErrorDomain && [EACCES, EPERM, EROFS].contains(Int32(e.code)))
            if noPermission { throw LogError.folderNotWritable(dir.path) }
            throw LogError.io(e.localizedDescription)
        }
    }

    @discardableResult
    func save(day: String, body: String) throws -> SaveOutcome {
        try validate(day)
        try checkFolder()
        try requireLocal(day)                // before anything is read, backed up, written or deleted
        let file = url(for: day)
        let exists = fm.fileExists(atPath: file.path)
        let oldData: Data? = exists ? (try? Data(contentsOf: file)) : nil
        let oldText = oldData.flatMap { String(data: $0, encoding: .utf8) }
        var out = SaveOutcome()
        if MarkdownBody.trimBody(body).isEmpty {
            guard exists else { return out }
            if let t = oldText, MarkdownFormat.parse(day: day, raw: t).isSkipped { return out }
            note(backUpCurrent(day: day, data: oldData, text: oldText, throttled: false), in: &out)
            do { try fm.removeItem(at: file) } catch { throw LogError.io(error.localizedDescription) }
            return out
        }
        let text = MarkdownFormat.serialize(day: day, body: body)
        if oldText == text { return out }   // identical: nothing to write, nothing to back up
        if exists { note(backUpCurrent(day: day, data: oldData, text: oldText, throttled: true), in: &out) }
        try write(text, day: day)
        return out
    }

    // MARK: backups
    private enum BackupResult: Equatable { case notNeeded, saved(String?), failed(String) }

    private func note(_ r: BackupResult, in out: inout SaveOutcome) {
        switch r {
        case .notNeeded: break
        case .saved(let warning): out.backedUp = true; out.backupWarning = warning; lastBackupWarning = warning
        case .failed(let message): out.backupWarning = message; lastBackupWarning = message
        }
    }

    private func backupDayDir(_ day: String) -> URL? {
        guard let b = backupDir, DayKey.isWellFormed(day) else { return nil }
        return b.appendingPathComponent(day, isDirectory: true)
    }
    /// Valid backup file names in a day folder, newest first (names sort chronologically).
    private func backupNames(in d: URL) -> [String] {
        ((try? fm.contentsOfDirectory(atPath: d.path)) ?? []).filter { BackupName.date(fromFile: $0) != nil }.sorted(by: >)
    }

    /// Copies the current day file (`data`, already read by the caller) into the backup folder, then prunes. Never throws.
    private func backUpCurrent(day: String, data: Data?, text: String?, throttled: Bool) -> BackupResult {
        guard let dayDir = backupDayDir(day), let root = backupDir else { return .notNeeded }
        sweepOnce()
        if let t = text {
            let page = MarkdownFormat.parse(day: day, raw: t)
            if page.isSkipped || MarkdownBody.trimBody(page.body).isEmpty { return .notNeeded }   // nothing worth keeping
        }
        guard let bytes = data else { return .failed("Could not read \(day).md to back it up.") }
        do {
            try fm.createDirectory(at: dayDir, withIntermediateDirectories: true)
            var stamp = clock()
            if let newest = backupNames(in: dayDir).first.flatMap({ BackupName.date(fromFile: $0) }) {
                if throttled, minBackupInterval > 0, stamp.timeIntervalSince(newest) < minBackupInterval { return .notNeeded }
                if BackupName.stem(stamp) <= BackupName.stem(newest) { stamp = newest.addingTimeInterval(1) }   // keep names increasing
            }
            try AtomicFile.write(bytes, to: dayDir.appendingPathComponent(BackupName.stem(stamp) + BackupName.ext), tempDir: root)
        } catch {
            return .failed("Could not write a backup of \(day).md: \(error.localizedDescription)")
        }
        var warning: String?
        for old in backupNames(in: dayDir).dropFirst(Self.maxBackupsPerDay) {
            do { try fm.removeItem(at: dayDir.appendingPathComponent(old)) }
            catch { warning = "Could not remove old backups of \(day).md: \(error.localizedDescription)" }
        }
        return .saved(warning)
    }

    /// Safety copies of one day, newest first. [] when backups are off, the day is malformed or nothing was saved yet.
    func listBackups(day: String) -> [BackupInfo] {
        guard let d = backupDayDir(day) else { return [] }
        return backupNames(in: d).compactMap { name in
            guard let date = BackupName.date(fromFile: name) else { return nil }
            let u = d.appendingPathComponent(name)
            // the size of the version, jots included (the "logged" rule is not what a restore list is about)
            let words = (try? String(contentsOf: u, encoding: .utf8)).map { raw -> Int in
                let page = MarkdownFormat.parse(day: day, raw: raw)
                return page.isSkipped ? 0 : MarkdownBody.words(in: page.body)
            } ?? 0
            return BackupInfo(date: date, url: u, words: words)
        }
    }

    /// Writes a backup back as the day file. The current file is backed up first (so a restore can itself be undone);
    /// if that is impossible nothing is changed and .backupFailed is thrown. Only the file NAME of `backup.url` is used,
    /// resolved inside this store's backup folder for `day`.
    @discardableResult
    func restoreBackup(day: String, backup: BackupInfo) throws -> DayDocument {
        try validate(day)
        guard backupDir != nil, let dayDir = backupDayDir(day) else { throw LogError.backupsDisabled }
        let name = backup.url.lastPathComponent
        guard BackupName.date(fromFile: name) != nil else { throw LogError.io("Not a backup file: \(name)") }
        let text: String
        do { text = try String(contentsOf: dayDir.appendingPathComponent(name), encoding: .utf8) }
        catch { throw LogError.io("Backup \(name) could not be read.") }
        try checkFolder()
        try requireLocal(day)
        let file = url(for: day)
        let exists = fm.fileExists(atPath: file.path)
        let curData: Data? = exists ? (try? Data(contentsOf: file)) : nil
        let curText = curData.flatMap { String(data: $0, encoding: .utf8) }
        if curText != text {
            if exists {
                switch backUpCurrent(day: day, data: curData, text: curText, throttled: false) {
                case .failed(let message): lastBackupWarning = message; throw LogError.backupFailed(message)
                case .saved(let warning): lastBackupWarning = warning
                case .notNeeded: break
                }
            }
            try write(text, day: day)
        }
        return MarkdownFormat.parse(day: day, raw: text)
    }

    func skip(_ day: String, reason: String = "") throws {
        try validate(day)
        if let e = try load(day), !e.isSkipped, e.hasContent { throw LogError.hasContent(day) }
        try write(MarkdownFormat.serializeSkip(day: day, reason: reason), day: day)
    }

    /// Skips every scheduled day (per `weekdays`) in from...to inclusive. Returns the days written.
    @discardableResult
    func skipRange(from: String, to: String, reason: String = "", weekdays: Set<Int>, calendar: Calendar) throws -> [String] {
        try validate(from); try validate(to)
        var written = [String](), d = from
        var guardCount = 0
        while d <= to && guardCount < 3700 {
            if weekdays.contains(DayKey.weekday(d, calendar)) {
                do { try skip(d, reason: reason); written.append(d) } catch LogError.hasContent { }
            }
            guard let n = DayKey.adding(d, 1, calendar) else { break }
            d = n; guardCount += 1
        }
        return written
    }

    @discardableResult
    func unskip(_ day: String) throws -> Bool {
        try validate(day)
        guard let e = try load(day), e.isSkipped else { return false }
        do { try fm.removeItem(at: url(for: day)) } catch { throw LogError.io(error.localizedDescription) }
        return true
    }

    // MARK: search (Search.swift). Pages are parsed once per run and re-read only when their file changed: SearchIndex.swift.
    let searchIndex = SearchIndex()
    func search(_ query: String, heading: String? = nil) throws -> [SearchHit] {
        try searchIndex.pages(of: self, load: loadIfLocal).flatMap { Search.hits(day: $0.day, sections: $0.sections, query: query, heading: heading) }
    }
    /// Fills the search cache ahead of the first search; the app calls it off the main thread after launch and when the folder changes.
    func warmSearch() throws { _ = try searchIndex.pages(of: self, load: loadIfLocal) }
}
