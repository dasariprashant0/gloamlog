// Inbox.swift - entries other programs leave for Gloamlog (today: the `log_work` tool of the Claude Code helper in
// Resources/claude/gloamlog-mcp.py). One small JSON file per entry in ~/Library/Application Support/Gloamlog/inbox:
//   {"text": "Fixed the card-order bug by sorting before the merge.", "ts": "2026-10-14T14:35:00Z"}
// Writers create the file under a hidden name and rename it, so a half-written file is never seen. The app turns each entry into
// a jot (DayWriter: durable journal first, then the page) and deletes the file only after that. Foundation only.
import Foundation

enum Inbox {
    struct Entry: Equatable { var url: URL; var text: String }

    static let maxFileBytes = 64 * 1024
    static let maxEntriesPerLook = 50

    static var defaultDir: URL {
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/Gloamlog/inbox", isDirectory: true)
    }

    /// The entries waiting in `dir`, oldest first. A file that is not JSON with a non-empty "text" is skipped, never deleted here.
    static func pending(in dir: URL = defaultDir) -> [Entry] {
        let fm = FileManager.default
        guard let names = try? fm.contentsOfDirectory(at: dir, includingPropertiesForKeys: [.contentModificationDateKey, .fileSizeKey], options: [.skipsHiddenFiles]) else { return [] }
        let files = names.filter { $0.pathExtension == "json" }.sorted {
            let a = (try? $0.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            let b = (try? $1.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            return a == b ? $0.lastPathComponent < $1.lastPathComponent : a < b
        }
        var out = [Entry]()
        for f in files.prefix(maxEntriesPerLook) {
            guard let size = try? f.resourceValues(forKeys: [.fileSizeKey]).fileSize, size <= maxFileBytes,
                  let data = try? Data(contentsOf: f),
                  let o = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let text = (o["text"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines), !text.isEmpty else { continue }
            out.append(Entry(url: f, text: text))
        }
        return out
    }
}
