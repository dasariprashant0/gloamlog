// CloudFile.swift - where the log folder lives (FolderKind) and the few facts about cloud files that sync needs: is this day's
// page on this Mac yet, and how to ask the provider for it. Foundation + POSIX only.
//
// A day whose page is not on this Mac yet is "not local": an iCloud placeholder (".2026-10-08.md.icloud", macOS 13) or a
// dataless file (the name is there, the bytes are not: iCloud Drive since macOS 14 and other providers' on-demand files). Such a
// day is never read, written, skipped or deleted by Gloamlog (LogStore refuses with LogError.notLocal): treating it as "no log" and
// creating a page over it is exactly how a second Mac gets a conflict copy.
import Foundation

enum FolderKind: Equatable {
    case local
    case iCloudDrive
    case cloud(String)          // another provider that keeps the folder in the cloud: "Dropbox", "OneDrive", "GoogleDrive"
    case network                // a network or removable volume: file events are not reliable there, so it is polled

    var label: String {
        switch self {
        case .local: return "Local"
        case .iCloudDrive: return "iCloud Drive"
        case .cloud(let p): return p
        case .network: return "Network or external drive"
        }
    }
    var isICloud: Bool { self == .iCloudDrive }

    /// From the folder's real path (symlinks resolved) and its volume. A folder that does not exist yet is `.local`.
    static func detect(_ dir: URL) -> FolderKind {
        let real = dir.resolvingSymlinksInPath()
        let path = real.path
        if path.contains("/Library/Mobile Documents/com~apple~CloudDocs") { return .iCloudDrive }
        if (try? real.resourceValues(forKeys: [.isUbiquitousItemKey]))?.isUbiquitousItem == true { return .iCloudDrive }   // Desktop and Documents in iCloud
        let parts = path.split(separator: "/").map(String.init)
        if let i = parts.firstIndex(of: "CloudStorage"), i + 1 < parts.count {
            return .cloud(parts[i + 1].split(separator: "-").first.map(String.init) ?? parts[i + 1])    // "OneDrive-Personal" -> "OneDrive"
        }
        if parts.contains(where: { $0 == "Dropbox" || $0.hasPrefix("Dropbox (") }) { return .cloud("Dropbox") }
        if (try? real.resourceValues(forKeys: [.volumeIsLocalKey]))?.volumeIsLocal == false { return .network }
        return .local
    }
}

enum CloudFile {
    /// SF_DATALESS from <sys/stat.h>: the file's bytes live in the cloud.
    static let datalessFlag: UInt32 = 0x40000000

    static func placeholderName(for day: String) -> String { ".\(day).md.icloud" }

    static func exists(atPath path: String) -> Bool { var st = stat(); return lstat(path, &st) == 0 }
    static func isDataless(atPath path: String) -> Bool { var st = stat(); return lstat(path, &st) == 0 && (st.st_flags & datalessFlag) != 0 }

    /// True when the day's page exists in the folder but is not (fully) on this Mac yet. Two lstat calls, nothing is read.
    static func isNotLocal(dir: URL, day: String) -> Bool {
        exists(atPath: dir.appendingPathComponent(placeholderName(for: day)).path)
            || isDataless(atPath: dir.appendingPathComponent(day + ".md").path)
    }

    /// iCloud's own verdict on a file (nil when it has none): .current, .downloaded (a local copy exists, a newer one is coming), .notDownloaded.
    static func downloadStatus(_ url: URL) -> URLUbiquitousItemDownloadingStatus? {
        (try? url.resourceValues(forKeys: [.ubiquitousItemDownloadingStatusKey]))?.ubiquitousItemDownloadingStatus
    }

    /// Asks the provider for the file's bytes. `url` is the real file's URL, never the placeholder's. Call it off the main thread: for
    /// providers other than iCloud the request is a plain read of the file, which blocks until the bytes are here.
    static func requestDownload(_ url: URL, iCloud: Bool) {
        if iCloud, (try? FileManager.default.startDownloadingUbiquitousItem(at: url)) != nil { return }
        if let h = FileHandle(forReadingAtPath: url.path) { _ = try? h.read(upToCount: 1); try? h.close() }
    }

    /// The path with every symlink resolved, as file events report it (/tmp is /private/tmp). The path itself when it cannot be resolved.
    static func canonicalPath(_ path: String) -> String {
        var buffer = [CChar](repeating: 0, count: Int(PATH_MAX))
        return realpath(path, &buffer) != nil ? String(cString: buffer) : path
    }

    /// "mtime.nanoseconds size" of the file now, nil when it cannot be stat'ed. Equal signatures mean the file was not touched in between.
    static func signature(atPath path: String) -> String? {
        var st = stat()
        guard stat(path, &st) == 0 else { return nil }
        return "\(st.st_mtimespec.tv_sec).\(st.st_mtimespec.tv_nsec) \(st.st_size)"
    }
}

/// Our own writes, so that the folder watcher can tell them from somebody else's. AtomicFile notes the signature of every file it
/// writes; an event for a path whose signature still matches is our own echo and wakes nobody.
enum OwnWrites {
    private static let lock = NSLock()
    private static var signatures: [String: String] = [:]

    static func note(_ url: URL) {
        let path = CloudFile.canonicalPath(url.path)
        guard let sig = CloudFile.signature(atPath: path) else { return }
        lock.lock(); defer { lock.unlock() }
        if signatures.count > 512 { signatures.removeAll() }
        signatures[path] = sig
    }

    /// True when `path` is a file we wrote and nobody has touched since.
    static func isOwn(_ path: String) -> Bool {
        lock.lock(); let known = signatures[path]; lock.unlock()
        guard let known = known else { return false }
        return CloudFile.signature(atPath: path) == known
    }
}

extension LogError: LocalizedError {
    /// Only the sync case has a sentence of its own; every other case keeps the system's wording.
    var errorDescription: String? {
        if case .notLocal(let day) = self { return "\(day) is still downloading from your cloud folder, so it can't be changed yet." }
        return nil
    }
}
