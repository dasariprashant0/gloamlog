// SyncScan.swift - one look at the log folder for sync: which files are still in the cloud and which conflict copies are here.
// Names, lstat and (inside iCloud Drive) the download status of day files: no file is opened or read. Run it off the main thread.
import Foundation

/// A second version of a day that a sync service saved next to the real file ("2026-10-08 2.md").
struct ConflictCopy: Equatable, Identifiable {
    let day: String
    let name: String                  // the file's name in the folder
    let family: ConflictFamily
    var id: String { name }
}

struct SyncScan: Equatable {
    /// A file whose bytes are not (all) on this Mac yet. `hasLocalText` is true for an iCloud file that has a local copy but
    /// is waiting for a newer one: it can still be read, but it is not final.
    struct Remote: Equatable {
        let name: String              // the real file's name, never the placeholder's
        let day: String
        let isConflictCopy: Bool
        let hasLocalText: Bool
    }

    var remote: [Remote] = []
    /// Conflict copies that are fully on this Mac, by day then name.
    var conflicts: [ConflictCopy] = []

    /// Days with something still on its way, oldest first ("N days still downloading").
    var pendingDays: [String] { Array(Set(remote.map { $0.day })).sorted() }
    /// Days whose page has no local text at all: they cannot be opened for writing and are not "unlogged".
    var blockedDays: Set<String> { Set(remote.filter { !$0.isConflictCopy && !$0.hasLocalText }.map { $0.day }) }
}

/// Scans one folder. Keep one per folder and call `scan()` from one queue at a time: it remembers which iCloud files it has
/// already seen up to date (by name, mtime and size), so a rescan costs one lstat per day file and no iCloud query.
final class FolderScanner {
    let dir: URL
    let iCloud: Bool
    private var current: [String: String] = [:]

    init(dir: URL, iCloud: Bool) { self.dir = dir; self.iCloud = iCloud }

    func scan() -> SyncScan {
        var out = SyncScan()
        guard let names = try? FileManager.default.contentsOfDirectory(atPath: dir.path) else { return out }
        var remote = [String: SyncScan.Remote]()
        func add(_ r: SyncScan.Remote) {
            // a file with both a placeholder and a real entry is not here until the placeholder is gone
            if let old = remote[r.name] { remote[r.name] = SyncScan.Remote(name: r.name, day: r.day, isConflictCopy: r.isConflictCopy,
                                                                             hasLocalText: old.hasLocalText && r.hasLocalText) }
            else { remote[r.name] = r }
        }
        for name in names {
            switch FolderEntryClassifier.classify(name) {
            case .ignored: continue
            case .placeholder(let real, let day, let isCopy):
                add(SyncScan.Remote(name: real, day: day, isConflictCopy: isCopy, hasLocalText: false))
            case .day(let day):
                if let hasText = notHere(name) { add(SyncScan.Remote(name: name, day: day, isConflictCopy: false, hasLocalText: hasText)) }
            case .conflict(let day, let family):
                if let hasText = notHere(name) { add(SyncScan.Remote(name: name, day: day, isConflictCopy: true, hasLocalText: hasText)) }
                else { out.conflicts.append(ConflictCopy(day: day, name: name, family: family)) }
            }
        }
        // a conflict copy that is also still a placeholder is not a conflict to review yet
        out.conflicts.removeAll { remote[$0.name].map { !$0.hasLocalText } ?? false }
        out.remote = remote.values.sorted { ($0.day, $0.name) < ($1.day, $1.name) }
        out.conflicts.sort { ($0.day, $0.name) < ($1.day, $1.name) }
        return out
    }

    /// nil when the file is fully here; otherwise whether any usable text of it is local.
    private func notHere(_ name: String) -> Bool? {
        let url = dir.appendingPathComponent(name)
        var st = stat()
        guard lstat(url.path, &st) == 0 else { return nil }                       // gone since the listing
        if (st.st_flags & CloudFile.datalessFlag) != 0 { return false }
        guard iCloud else { return nil }
        let sig = "\(st.st_mtimespec.tv_sec).\(st.st_mtimespec.tv_nsec) \(st.st_size)"
        if current[name] == sig { return nil }
        let status = CloudFile.downloadStatus(url)
        if status == .notDownloaded { return false }
        if status == .downloaded { return true }                                  // a stale local copy: readable, a newer one is coming
        current[name] = sig
        return nil
    }
}
