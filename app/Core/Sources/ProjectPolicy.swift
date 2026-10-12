// ProjectPolicy.swift - which projects Sources may read (M6), and the Settings that remember it. Foundation only.
//
// The rule that matters: NOTHING is read, counted or shown for a project until the owner ticks it. A project is its root folder
// (a Claude Code session's working folder with git worktrees folded into their main folder, or a Git repository's top folder);
// matching is exact on that folder, case-insensitive (the default APFS volume is), never a prefix, so ticking a repository
// can never switch on whatever happens to sit inside it, and ticking nothing reads nothing.
import Foundation

/// Path helpers shared by the sources. Pure string work: nothing here touches the disk.
enum SourcePath {
    /// Absolute, `~` expanded, no `.` / `..` and no trailing slash.
    static func normalize(_ p: String) -> String {
        let s = NSString(string: (p as NSString).expandingTildeInPath).standardizingPath
        return s.count > 1 && s.hasSuffix("/") ? String(s.dropLast()) : s
    }
    /// `<root>/.claude/worktrees/<name>[/...]` is the same project as `<root>`.
    static func collapseWorktree(_ p: String) -> String {
        let s = normalize(p)
        guard let r = s.range(of: "/.claude/worktrees/") else { return s }
        return String(s[s.startIndex..<r.lowerBound])
    }
    static func name(of root: String) -> String {
        if normalize(root).lowercased() == normalize(NSHomeDirectory()).lowercased() { return "Home" }      // not the account name
        let n = (root as NSString).lastPathComponent
        return n.isEmpty ? root : n
    }
    /// True when `path` is `root` itself or inside it (whole path components, case-insensitive).
    static func contains(root: String, path: String) -> Bool {
        let r = normalize(root).lowercased(), p = normalize(path).lowercased()
        return p == r || p.hasPrefix(r == "/" ? "/" : r + "/")
    }
    /// `path` as seen from the project: relative to the root (or to its worktree), else just the file name.
    static func relative(_ path: String, to root: String) -> String {
        let p = normalize(path), base = normalize(root)
        let lower = p.lowercased()
        if let r = p.range(of: "/.claude/worktrees/"), lower.hasPrefix(base.lowercased() + "/") {
            let rest = p[r.upperBound...]
            if let slash = rest.firstIndex(of: "/") { return String(rest[rest.index(after: slash)...]) }
        }
        if lower.hasPrefix(base.lowercased() + "/") { return String(p.dropFirst(base.count + 1)) }
        return (p as NSString).lastPathComponent
    }
}

struct ProjectPolicy: Equatable {
    /// The ticked project roots, as the owner's Settings hold them.
    let allowed: [String]
    private let table: [String: String]            // lower-cased normalised root -> normalised root

    init(allowed: [String]) {
        self.allowed = allowed
        var t = [String: String]()
        for a in allowed { let n = SourcePath.normalize(a); t[n.lowercased()] = n }
        table = t
    }

    var isEmpty: Bool { table.isEmpty }
    /// A short stable text for "which projects were ticked", used to key caches.
    var key: String { table.keys.sorted().joined(separator: "\n") }

    /// The ticked root that is this project (worktrees folded), or nil: then nothing about it may be read or shown.
    func root(for path: String) -> String? { table[SourcePath.collapseWorktree(path).lowercased()] }
}

/// Settings > Sources. Everything is off until the owner turns it on; decoding is tolerant so an older or newer save never
/// loses the rest of the settings.
struct SourcesPrefs: Codable, Equatable {
    static let defaultHeading = "What I did"      // exists in the default template, so a click adds bullets there instead of a new section

    var claude = false               // read Claude Code's session files (only for ticked projects)
    var git = false                  // run `git log` in ticked repositories
    var consented = false            // the first-enable sheet was accepted
    var allowed: [String] = []       // ticked project roots (absolute paths)
    var gitRoots: [String] = []      // folders added with "Add folder…": a repository, or a folder that holds repositories
    var heading = SourcesPrefs.defaultHeading

    init() {}
    enum CodingKeys: String, CodingKey { case claude, git, consented, allowed, gitRoots, heading }
    init(from d: Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        claude = (try? c.decodeIfPresent(Bool.self, forKey: .claude)) ?? claude
        git = (try? c.decodeIfPresent(Bool.self, forKey: .git)) ?? git
        consented = (try? c.decodeIfPresent(Bool.self, forKey: .consented)) ?? consented
        allowed = (try? c.decodeIfPresent([String].self, forKey: .allowed)) ?? allowed
        gitRoots = (try? c.decodeIfPresent([String].self, forKey: .gitRoots)) ?? gitRoots
        heading = (try? c.decodeIfPresent(String.self, forKey: .heading)) ?? heading
    }

    var anyOn: Bool { claude || git }
    var headingText: String { let h = heading.dlTrimmed; return h.isEmpty ? SourcesPrefs.defaultHeading : h }
    var policy: ProjectPolicy { ProjectPolicy(allowed: allowed) }
    func isAllowed(_ root: String) -> Bool { policy.root(for: root) != nil }

    mutating func setAllowed(_ root: String, _ on: Bool) {
        let n = SourcePath.normalize(root)
        allowed.removeAll { SourcePath.normalize($0).lowercased() == n.lowercased() }
        if on { allowed.append(n) }
    }
}
