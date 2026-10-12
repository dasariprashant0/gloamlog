// AtomicFile.swift - crash-safe replace of a file, used for day files and backups. Foundation + POSIX only.
//
// Why not Data.write(.atomic) / String.write(atomically:)? Those write a temp file named "<name>.sb-xxxxxxxx-xxxxxx" next to the
// target; a SIGKILL or crash mid-write leaves it there for good: visible in Finder and sync clients, counted as an "unrecognised
// file" in Settings, never cleaned (found by tests/run-kill-tests.sh in 36 of 40 rounds). Here the temp file is named by us:
//   ".gloamlog-tmp-<pid of the writer>-<uuid>"   hidden (dot file), so it is never listed as a day, a backup or an unrecognised file.
// write(_:to:)  1. the bytes go to the temp file (in `tempDir`, default the target's folder; must be on the target's volume),
//               2. fsync, 3. the target's permissions are copied onto it (as Foundation's atomic write did), 4. rename(2) over the
//               target. The target is therefore always either its old complete content or the new complete content. If anything
//               fails the temp file is removed again and the error is thrown. Like Foundation's, a symlink at the target is replaced.
// sweep(in:)    removes ".gloamlog-tmp-*" files whose writer process is no longer running (the leftovers of a kill). A temp file of a
//               live process, including this one, is never touched; anything not starting with the prefix is never touched.
import Foundation

enum AtomicFile {
    static let tempPrefix = ".gloamlog-tmp-"

    static func write(_ data: Data, to dest: URL, tempDir: URL? = nil) throws {
        let fm = FileManager.default
        let tmp = (tempDir ?? dest.deletingLastPathComponent()).appendingPathComponent("\(tempPrefix)\(getpid())-\(UUID().uuidString)")
        do {
            try data.write(to: tmp, options: .withoutOverwriting)
            let fd = open(tmp.path, O_RDONLY)
            if fd >= 0 { _ = fsync(fd); close(fd) }               // best effort: not every file system supports it
            if let attrs = try? fm.attributesOfItem(atPath: dest.path), attrs[.type] as? FileAttributeType == .typeRegular,
               let permissions = attrs[.posixPermissions] {
                try? fm.setAttributes([.posixPermissions: permissions], ofItemAtPath: tmp.path)
            }
            guard rename(tmp.path, dest.path) == 0 else { throw POSIXError(POSIXErrorCode(rawValue: errno) ?? .EIO) }
            OwnWrites.note(dest)                                  // the folder watcher ignores the echo of this write (Core/Sync/CloudFile.swift)
        } catch {
            try? fm.removeItem(at: tmp)
            throw error
        }
    }

    /// Removes the temp files of writers that no longer exist. Never throws; a folder that cannot be listed is skipped.
    static func sweep(in dir: URL) {
        let fm = FileManager.default
        for name in (try? fm.contentsOfDirectory(atPath: dir.path)) ?? [] where name.hasPrefix(tempPrefix) && isStale(name) {
            try? fm.removeItem(at: dir.appendingPathComponent(name))
        }
    }

    /// ".gloamlog-tmp-<pid>-...": stale when no process with that pid is running. A name without a readable pid is stale too.
    private static func isStale(_ name: String) -> Bool {
        let digits = name.dropFirst(tempPrefix.count).prefix { $0.isASCII && $0.isNumber }
        guard let pid = Int32(digits) else { return true }
        if pid == getpid() { return false }
        return kill(pid, 0) != 0 && errno == ESRCH                // EPERM means the process exists
    }
}
