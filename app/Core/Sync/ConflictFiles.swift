// ConflictFiles.swift - what happens to a conflict copy once the user has chosen: it is MOVED into the safety copies
// (<backups>/conflicts/<yyyyMMdd-HHmmss>-<its name>), never deleted. A move is done in two steps so that a failure can never lose
// text: `keep` copies the file and checks the copy byte for byte; only after the chosen text has been saved does `release` take
// the original out of the shared folder, and only if it still matches what was kept.
import Foundation

enum ConflictFiles {
    static func folder(in backupDir: URL) -> URL { backupDir.appendingPathComponent("conflicts", isDirectory: true) }

    private static func stamp(_ d: Date) -> String {
        var cal = Calendar(identifier: .gregorian); cal.timeZone = TimeZone(identifier: "GMT")!
        let c = cal.dateComponents([.year, .month, .day, .hour, .minute, .second], from: d)
        return String(format: "%04ld%02ld%02ld-%02ld%02ld%02ld", c.year ?? 0, c.month ?? 0, c.day ?? 0, c.hour ?? 0, c.minute ?? 0, c.second ?? 0)
    }

    /// Writes the file's bytes into the conflicts folder and verifies them. The original is not touched. An earlier safety copy is never
    /// overwritten (an identical one is reused, so retrying is harmless). Throws when the safety copy cannot be made or does not match.
    /// `name` is what the copy is called after the time stamp (default: the file's own name).
    static func keep(_ copy: URL, in backupDir: URL, now: Date, name: String? = nil) throws -> URL {
        let fm = FileManager.default
        let data = try Data(contentsOf: copy)
        let dir = folder(in: backupDir)
        let label = name ?? copy.lastPathComponent
        try fm.createDirectory(at: dir, withIntermediateDirectories: true)
        // the same bytes are already kept under this name (an earlier try or scan): reuse that copy instead of piling up duplicates
        for existing in (try? fm.contentsOfDirectory(atPath: dir.path)) ?? [] where existing.hasSuffix("-\(label)") {
            let url = dir.appendingPathComponent(existing)
            if (try? Data(contentsOf: url)) == data { return url }
        }
        var dest = dir.appendingPathComponent("\(stamp(now))-\(label)")
        var n = 2
        while fm.fileExists(atPath: dest.path) {
            if (try? Data(contentsOf: dest)) == data { return dest }
            dest = dir.appendingPathComponent("\(stamp(now))-\(n)-\(label)"); n += 1
            if n > 99 { throw CocoaError(.fileWriteFileExists) }
        }
        try AtomicFile.write(data, to: dest, tempDir: dir)
        guard (try? Data(contentsOf: dest)) == data else { try? fm.removeItem(at: dest); throw CocoaError(.fileWriteUnknown) }
        return dest
    }

    /// Takes the original out of the shared folder, but only if it still holds exactly what `kept` holds. Anything else (the provider
    /// wrote to it again meanwhile) leaves it where it is, and it is offered again.
    static func release(_ copy: URL, keptAt kept: URL) throws {
        guard let a = try? Data(contentsOf: copy), let b = try? Data(contentsOf: kept), a == b else { throw CocoaError(.fileWriteUnknown) }
        try FileManager.default.removeItem(at: copy)
    }
}
