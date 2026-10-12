// ActivityItem.swift - what a source (Claude Code, Git) found for one day, and the small contract a source implements (M6).
// Foundation only. Sources are READ-ONLY: they never write, never touch the network, and look only at projects the owner ticked
// (ProjectPolicy). Items may hold raw text and are NEVER shown as they are: ActivityDigest runs the Redactor over everything
// that reaches the screen or a page.
//
// To add a source: implement `ActivitySource` (an id, the projects it can see, and the items for one day), make sure it asks
// the policy before it reads anything, and hand its items to ActivityDigest like the two below do.
import Foundation

enum ActivityKind: String, Codable, Equatable { case session, prompt, file, commit, pullRequest }

struct ActivityItem: Equatable, Identifiable {
    var kind: ActivityKind
    var root: String                // the project's root folder: the policy key, never shown
    var project: String             // what the project is called (normally the folder name)
    var title: String               // prompt first line / commit subject / file path (project-relative) / pull-request link
    var detail = ""                 // secondary text, e.g. a short hash
    var time: Date?                 // when it happened (a session: its first record that day)
    var evidence = ""               // the short reference written with the bullet (a short hash, a link): "is it already on the page?"
    var count = 1                   // a file: how often it was edited; a session item: how many sessions that day
    var seconds = 0                 // a session item: active time
    var id: String { "\(kind.rawValue)|\(root)|\(evidence.isEmpty ? title : evidence)" }
}

/// What a scan saw, for the "3 records skipped" line and the format check.
struct ScanStats: Equatable {
    var files = 0                   // transcript files read, or repositories asked
    var lines = 0
    var skipped = 0                 // lines that were not valid JSON or too long to read
    var records = 0                 // user / assistant records seen ...
    var usable = 0                  // ... that had a time and a message: if far fewer than all, the format has probably changed
    mutating func add(_ o: ScanStats) { files += o.files; lines += o.lines; skipped += o.skipped; records += o.records; usable += o.usable }
    var formatLooksChanged: Bool { records >= 30 && usable * 5 < records * 4 }
}

struct SourceResult {
    var items: [ActivityItem] = []
    var stats = ScanStats()
    var notices: [String] = []
}

/// A project (or repository) a source can see: one row of the project list in Settings > Sources.
struct SourceProject: Equatable, Identifiable {
    var root: String
    var name: String
    var sources: Set<String>        // ActivitySource.id values that know it: "claude", "git"
    var lastActive: Date?
    var id: String { root.lowercased() }
}

/// Cancel and progress for a scan that runs off the main thread. `isCancelled` may be read from any thread.
final class ScanControl {
    private let lock = NSLock()
    private var flag = false
    var onProgress: ((Int, Int) -> Void)?
    func cancel() { lock.lock(); flag = true; lock.unlock() }
    var isCancelled: Bool { lock.lock(); defer { lock.unlock() }; return flag }
    func progress(_ done: Int, _ total: Int) { onProgress?(done, total) }
}

protocol ActivitySource {
    var id: String { get }
    /// The projects this source can see (names only). Reads as little as it can.
    func projects(control: ScanControl) -> [SourceProject]
    /// The items for `day` ("yyyy-MM-dd" in `calendar`'s time zone), only for projects `policy` allows.
    func activity(day: String, calendar: Calendar, policy: ProjectPolicy, control: ScanControl) -> SourceResult
}
