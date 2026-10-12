// ClaudeCodeSource.swift - what you did with Claude Code on a day, read from its local session files (M6). Foundation only.
//
// READ-ONLY. It opens `<config>/projects/<project>/<session>.jsonl` for reading and nothing else: no writes, no network, no
// credentials, and no file outside those folders. The config folder is `~/.claude`, plus `$CLAUDE_CONFIG_DIR` when the app sees it.
//
// Privacy: a session file is read in full only when its project (the working folder recorded in it) is ticked. The first
// records of any other file are only asked for the working folder name, from the raw bytes, without decoding the record.
// Records of a ticked file whose own folder is not ticked are dropped before any text is taken from them.
//
// The format is Claude Code's private one and changes between versions, so the parser asks for almost nothing: a line is a JSON
// object with a `type`; user and assistant records need a `timestamp` and a `message`; everything else, and every field that is
// missing or has another type, is ignored. A line that is not JSON is counted, never fatal. Assumptions (all unverified against
// real files, see docs/research/CLAUDE_INTEGRATION.md 2.2):
//   record      {type: "user"|"assistant", timestamp: "2026-10-06T09:12:03.120Z", sessionId, cwd, isSidechain, isMeta,
//                isCompactSummary, uuid, message: {role, content: String | [block]}}
//   blocks      {type:"text", text} · {type:"tool_use", id, name, input} · {type:"tool_result", tool_use_id, content: String | [{text}]}
//   tools       Write / Edit / MultiEdit {input.file_path} · NotebookEdit {input.notebook_path} · Bash {input.command}
//   commit      a Bash command with `git commit`; its result holds "[branch 1a2b3c4] subject"
//   wrappers    system and command text arrives wrapped in tags such as <system-reminder> or <command-name>
import Foundation

/// "2026-10-06T09:12:03.120Z" -> seconds since 1970. A hand parser (no formatter) because it runs on every line of 100 MB files.
enum ClaudeTime {
    static func parse(_ s: String) -> TimeInterval? {
        if let t = fast(s) { return t }
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = f.date(from: s) { return d.timeIntervalSince1970 }
        f.formatOptions = [.withInternetDateTime]
        return f.date(from: s)?.timeIntervalSince1970
    }

    private static func fast(_ s: String) -> TimeInterval? {
        let u = Array(s.utf8)
        guard u.count >= 20, u[4] == 45, u[7] == 45, u[10] == 84, u[13] == 58, u[16] == 58 else { return nil }
        func n(_ a: Int, _ len: Int) -> Int? {
            var v = 0
            for k in a..<(a + len) { guard u[k] >= 48, u[k] <= 57 else { return nil }; v = v * 10 + Int(u[k] - 48) }
            return v
        }
        guard let y = n(0, 4), let mo = n(5, 2), let d = n(8, 2), let h = n(11, 2), let mi = n(14, 2), let se = n(17, 2),
              (1...12).contains(mo), (1...31).contains(d), h < 24, mi < 60, se < 61 else { return nil }
        var frac = 0.0, idx = 19
        if u[idx] == 46 {
            idx += 1
            var scale = 0.1
            while idx < u.count, u[idx] >= 48, u[idx] <= 57 { frac += Double(u[idx] - 48) * scale; scale /= 10; idx += 1 }
        }
        guard idx < u.count, u[idx] == 90 else { return nil }          // must end in Z (UTC)
        return Double(daysFromCivil(y, mo, d) * 86400 + h * 3600 + mi * 60 + se) + frac
    }

    private static func daysFromCivil(_ y0: Int, _ m: Int, _ d: Int) -> Int {          // Hinnant's algorithm
        let y = m <= 2 ? y0 - 1 : y0
        let era = (y >= 0 ? y : y - 399) / 400
        let yoe = y - era * 400
        let doy = (153 * (m + (m > 2 ? -3 : 9)) + 2) / 5 + d - 1
        return era * 146097 + yoe * 365 + yoe / 4 - yoe / 100 + doy - 719468
    }

    /// Seconds of activity: sort the record times; a gap longer than `gap` ends a period; add up the periods.
    static func activeSeconds(_ times: [TimeInterval], gap: TimeInterval) -> Int {
        let t = times.sorted()
        guard let first = t.first else { return 0 }
        var start = first, prev = first, total = 0.0
        for x in t.dropFirst() { if x - prev > gap { total += prev - start; start = x }; prev = x }
        return Int((total + prev - start).rounded())
    }
}

/// Text rules for transcript content: what counts as a prompt, how to read a commit and a pull request out of a command.
enum ClaudeText {
    private static func rx(_ p: String, _ o: NSRegularExpression.Options = []) -> NSRegularExpression? { try? NSRegularExpression(pattern: p, options: o) }

    // Tag names whose content is system or command noise, never something the owner wrote.
    private static let wrapped = rx(#"<(system-reminder|local-command-caveat|local-command-stdout|local-command-stderr|command-name|command-message|command-args|command-contents|bash-input|bash-stdout|bash-stderr|ide_opened_file|ide_selection|ide_diagnostics|user-prompt-submit-hook|task-notification|function_results|persisted-output)\b[^>]*>[\s\S]*?</\1>"#, [.caseInsensitive])
    private static let unclosed = rx(#"<(?:system-reminder|local-command-caveat|ide_opened_file|ide_selection|user-prompt-submit-hook)\b[^>]*>[\s\S]*$"#, [.caseInsensitive])
    private static let placeholders = rx(#"\[(?:Pasted text|Pasted image|Image|Attachment)[^\]]*\]"#, [.caseInsensitive])
    private static let noisePrefixes = ["Caveat:", "[Request interrupted", "This session is being continued", "API Error"]

    private static func strip(_ re: NSRegularExpression?, _ s: String) -> String {
        guard let re = re else { return s }
        return re.stringByReplacingMatches(in: s, range: NSRange(location: 0, length: (s as NSString).length), withTemplate: "")
    }
    static func collapse(_ s: String) -> String { s.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ") }

    /// The first line of something the owner typed, with system, command and tool wrappers removed; nil when there is none
    /// (a slash command, an interrupt marker, a compaction summary, "yes").
    static func promptLine(_ raw: String) -> String? {
        let s = strip(placeholders, strip(unclosed, strip(wrapped, raw))).dlTrimmed
        guard !s.isEmpty, !noisePrefixes.contains(where: { s.hasPrefix($0) }) else { return nil }
        if s.hasPrefix("/"), !s.contains(" ") { return nil }                            // a bare slash command
        guard let first = s.components(separatedBy: .newlines).map({ $0.dlTrimmed }).first(where: { !$0.isEmpty }) else { return nil }
        let line = collapse(first)
        guard line.split(separator: " ").count >= 3 else { return nil }                // "ok", "yes do it": not worth a bullet
        return String(line.prefix(400))
    }

    // MARK: commits
    private static let gitCommit = rx(#"\bgit\s+(?:(?:-C|-c)\s+\S+\s+|--[a-z-]+\s+)*commit(?![\w-])"#)
    private static let heredoc = rx(#"<<-?\s*['"]?(\w+)['"]?[^\n]*\n([\s\S]*?)\n\s*\1\b"#)
    private static let messageArg = rx(#"(?:^|\s)(?:-[A-Za-z]*m|--message)(?:\s+|=)(?:"((?:[^"\\]|\\.)*)"|'([^']*)')"#)
    private static let commitLine = rx(#"^\[[^\]\n]+? ([0-9a-f]{7,40})\] (.+)$"#, [.anchorsMatchLines])

    static func isGitCommit(_ command: String) -> Bool {
        gitCommit?.firstMatch(in: command, range: NSRange(location: 0, length: (command as NSString).length)) != nil
    }

    /// The commit message's first line, from `-m "..."`, `-m '...'`, `-am` or a heredoc; nil when the command does not carry one.
    static func commitSubject(_ command: String) -> String? {
        let ns = command as NSString, all = NSRange(location: 0, length: ns.length)
        func firstLine(_ s: String) -> String? { s.components(separatedBy: .newlines).map { $0.dlTrimmed }.first(where: { !$0.isEmpty }).map(collapse) }
        if let m = heredoc?.firstMatch(in: command, range: all), m.numberOfRanges > 2, let l = firstLine(ns.substring(with: m.range(at: 2))) { return l }
        guard let m = messageArg?.firstMatch(in: command, range: all) else { return nil }
        if m.range(at: 1).location != NSNotFound {
            let raw = ns.substring(with: m.range(at: 1))
            if raw.hasPrefix("$(") || raw.hasPrefix("`") { return nil }                 // a shell substitution we could not read
            return firstLine(raw.replacingOccurrences(of: "\\\"", with: "\"").replacingOccurrences(of: "\\\\", with: "\\"))
        }
        if m.range(at: 2).location != NSNotFound { return firstLine(ns.substring(with: m.range(at: 2))) }
        return nil
    }

    /// "[feature/pricing 1a2b3c4] Add pricing table" in a command's output: the short hash and the subject.
    static func commitResult(_ text: String) -> (sha: String, subject: String)? {
        let ns = text as NSString
        guard let m = commitLine?.firstMatch(in: text, range: NSRange(location: 0, length: ns.length)), m.numberOfRanges > 2 else { return nil }
        return (String(ns.substring(with: m.range(at: 1)).prefix(7)), collapse(ns.substring(with: m.range(at: 2))))
    }

    // MARK: pull requests
    private static let prCreate = rx(#"\b(?:gh\s+pr\s+create|glab\s+mr\s+create)\b"#)
    private static let prLink = rx(#"https?://(?:github\.com/[\w.-]+/[\w.-]+/pull/\d+|gitlab\.com/[\w./-]+?/-/merge_requests/\d+|bitbucket\.org/[\w.-]+/[\w.-]+/pull-requests/\d+)"#)

    static func isPRCreate(_ command: String) -> Bool {
        prCreate?.firstMatch(in: command, range: NSRange(location: 0, length: (command as NSString).length)) != nil
    }
    static func pullRequestURLs(_ text: String) -> [String] {
        guard text.contains("/pull/") || text.contains("/merge_requests/") || text.contains("/pull-requests/") else { return [] }
        let ns = text as NSString
        var seen = Set<String>(), out = [String]()
        for m in prLink?.matches(in: text, range: NSRange(location: 0, length: ns.length)) ?? [] {
            let u = ns.substring(with: m.range)
            if seen.insert(u).inserted { out.append(u) }
        }
        return out
    }

    /// A tool result's text: the string, or the `text` of its blocks.
    static func resultText(_ content: Any?) -> String {
        if let s = content as? String { return String(s.prefix(40_000)) }
        guard let blocks = content as? [Any] else { return "" }
        var parts = [String]()
        for case let b as [String: Any] in blocks { if let t = b["text"] as? String { parts.append(t) } }
        return String(parts.joined(separator: "\n").prefix(40_000))
    }
}

/// What one session file holds for the ticked projects, across all days (the cache keeps it; a day is cut from it afterwards).
struct FileDigest {
    struct TickKey: Hashable { let root: String; let session: String }
    struct Prompt { var t: TimeInterval; var root: String; var session: String; var text: String }
    struct Edit { var t: TimeInterval; var root: String; var path: String; var id: String }
    struct Commit { var t: TimeInterval; var root: String; var id: String; var subject: String; var sha: String; var failed: Bool }
    struct PR { var t: TimeInterval; var root: String; var url: String }

    var ticks = [TickKey: [TimeInterval]]()      // the time of every record, per project and session: the active-time estimate
    var prompts = [Prompt]()
    var edits = [Edit]()
    var commits = [Commit]()
    var prs = [PR]()
    var stats = ScanStats()
}

/// Reads one transcript line by line. A value type with one entry point: `handle` never throws and never traps.
private struct TranscriptParser {
    let policy: ProjectPolicy
    let anchor: String?                                // the ticked project this file started in: a `cd` into one of its folders stays in it
    let fallbackSession: String
    var digest = FileDigest()
    private var cwd: String?
    private var seenTools = Set<String>()
    private var pendingCommit = [String: Int]()        // tool_use id -> index in digest.commits, until its result arrives
    private var pendingPR = Set<String>()              // tool_use ids of `gh pr create`

    init(policy: ProjectPolicy, anchor: String?, fallbackSession: String) { self.policy = policy; self.anchor = anchor; self.fallbackSession = fallbackSession }

    private static let userMark = Array("\"user\"".utf8), assistantMark = Array("\"assistant\"".utf8)

    mutating func handle(_ line: Data) {
        digest.stats.lines += 1
        // Cheap superset check: a user or assistant record always carries one of these words, whatever the spacing.
        guard line.containsBytes(Self.userMark) || line.containsBytes(Self.assistantMark) else { return }
        guard let obj = try? JSONSerialization.jsonObject(with: line), let rec = obj as? [String: Any] else { digest.stats.skipped += 1; return }
        guard let type = rec["type"] as? String, type == "user" || type == "assistant" else { return }     // titles, snapshots, newer kinds
        digest.stats.records += 1
        if let c = rec["cwd"] as? String, !c.isEmpty { cwd = c }
        guard let t = (rec["timestamp"] as? String).flatMap(ClaudeTime.parse), let msg = rec["message"] as? [String: Any] else { return }
        digest.stats.usable += 1
        // The gate: nothing is taken from a record whose project is not ticked.
        guard let c = cwd, let root = policy.root(for: c) ?? anchor.flatMap({ SourcePath.contains(root: $0, path: c) ? $0 : nil }) else { return }
        let session = (rec["sessionId"] as? String).flatMap { $0.isEmpty ? nil : $0 } ?? fallbackSession
        digest.ticks[FileDigest.TickKey(root: root, session: session), default: []].append(t)
        if type == "user" { user(rec, msg["content"], t: t, root: root, session: session) } else { assistant(msg["content"], t: t, root: root) }
    }

    private mutating func user(_ rec: [String: Any], _ content: Any?, t: TimeInterval, root: String, session: String) {
        let synthetic = (rec["isMeta"] as? Bool ?? false) || (rec["isCompactSummary"] as? Bool ?? false)
            || (rec["isVisibleInTranscriptOnly"] as? Bool ?? false)
        let sidechain = rec["isSidechain"] as? Bool ?? false           // a subagent's prompt is Claude's, not the owner's
        var texts = [String]()
        if let s = content as? String { texts.append(s) }
        else if let blocks = content as? [Any] {
            for case let b as [String: Any] in blocks {
                switch b["type"] as? String {
                case "text": if let s = b["text"] as? String { texts.append(s) }
                case "tool_result": toolResult(b, t: t, root: root)
                default: break
                }
            }
        }
        for s in texts {
            if let a = s.range(of: "<bash-input>"), let z = s.range(of: "</bash-input>"), a.upperBound <= z.lowerBound {   // a "!" command
                let cmd = String(s[a.upperBound..<z.lowerBound])
                if ClaudeText.isGitCommit(cmd) {
                    digest.commits.append(.init(t: t, root: root, id: "", subject: ClaudeText.commitSubject(cmd) ?? "", sha: "", failed: false))
                }
                continue
            }
            guard !synthetic, !sidechain, let line = ClaudeText.promptLine(s) else { continue }
            digest.prompts.append(.init(t: t, root: root, session: session, text: line))
        }
    }

    private mutating func assistant(_ content: Any?, t: TimeInterval, root: String) {
        guard let blocks = content as? [Any] else { return }
        for case let b as [String: Any] in blocks {
            guard (b["type"] as? String) == "tool_use", let name = b["name"] as? String else { continue }
            let id = b["id"] as? String ?? ""
            if !id.isEmpty, !seenTools.insert(id).inserted { continue }       // a response streamed as several lines repeats its tool calls
            let input = b["input"] as? [String: Any] ?? [:]
            if ClaudeCodeSource.editTools.contains(name) {
                if let p = (input["file_path"] as? String) ?? (input["notebook_path"] as? String), !p.isEmpty {
                    digest.edits.append(.init(t: t, root: root, path: SourcePath.relative(p, to: root), id: id))
                }
            } else if name == "Bash", let cmd = input["command"] as? String {
                if ClaudeText.isGitCommit(cmd) {
                    if !id.isEmpty { pendingCommit[id] = digest.commits.count }
                    digest.commits.append(.init(t: t, root: root, id: id, subject: ClaudeText.commitSubject(cmd) ?? "", sha: "", failed: false))
                }
                if !id.isEmpty, ClaudeText.isPRCreate(cmd) { pendingPR.insert(id) }
            }
        }
    }

    /// Only the results of a commit or `gh pr create` call are looked at; every other tool output is ignored unread.
    private mutating func toolResult(_ b: [String: Any], t: TimeInterval, root: String) {
        guard let id = b["tool_use_id"] as? String else { return }
        let commit = pendingCommit[id], pr = pendingPR.contains(id)
        guard commit != nil || pr else { return }
        let text = ClaudeText.resultText(b["content"])
        if let i = commit {
            pendingCommit[id] = nil
            if let r = ClaudeText.commitResult(text) {
                digest.commits[i].sha = r.sha
                if !r.subject.isEmpty { digest.commits[i].subject = r.subject }
            } else { digest.commits[i].failed = true }                       // the commit did not happen (hook failed, nothing to commit)
        }
        if pr {
            pendingPR.remove(id)
            for u in ClaudeText.pullRequestURLs(text) { digest.prs.append(.init(t: t, root: root, url: u)) }
        }
    }
}

final class ClaudeCodeSource: ActivitySource {
    let id = "claude"
    /// Tools whose input names a file they changed. A table, so a renamed tool is a one-word fix.
    static let editTools: Set<String> = ["Write", "Edit", "MultiEdit", "NotebookEdit"]
    /// A pause longer than this ends a period of activity (15 minutes).
    static let idleGap: TimeInterval = 15 * 60
    static let maxPrompts = 3

    private let roots: [URL]
    private let cache = FileCache()

    /// `roots` are Claude config folders (each holds `projects/`); nil = `$CLAUDE_CONFIG_DIR` when set, and `~/.claude`.
    init(roots: [URL]? = nil) { self.roots = roots ?? ClaudeCodeSource.defaultRoots() }

    static func defaultRoots(environment: [String: String] = ProcessInfo.processInfo.environment, home: String = NSHomeDirectory()) -> [URL] {
        var paths = [String]()
        if let e = environment["CLAUDE_CONFIG_DIR"], !e.dlTrimmed.isEmpty { paths.append(SourcePath.normalize(e)) }
        paths.append(SourcePath.normalize(home + "/.claude"))
        var seen = Set<String>()
        return paths.filter { seen.insert($0.lowercased()).inserted }.map { URL(fileURLWithPath: $0, isDirectory: true) }
    }

    // MARK: files
    private struct SessionFile { var url: URL; var mtime: Date; var size: Int64 }
    private enum Entry { case skipped, digest(FileDigest) }

    /// Every `*.jsonl` directly inside a project folder. Symbolic links are never followed, and the files Claude Code sets
    /// aside (`.orphaned-`, `.superseded-`) are ignored, or their copies would be counted twice.
    private func sessionFiles() -> [SessionFile] {
        let fm = FileManager.default
        var out = [SessionFile]()
        for root in roots {
            let projects = root.appendingPathComponent("projects", isDirectory: true)
            guard let dirs = try? fm.contentsOfDirectory(at: projects, includingPropertiesForKeys: [.isDirectoryKey, .isSymbolicLinkKey], options: [.skipsHiddenFiles]) else { continue }
            for d in dirs {
                guard let dv = try? d.resourceValues(forKeys: [.isDirectoryKey, .isSymbolicLinkKey]), dv.isDirectory == true, dv.isSymbolicLink != true,
                      let files = try? fm.contentsOfDirectory(at: d, includingPropertiesForKeys: [.isRegularFileKey, .contentModificationDateKey, .fileSizeKey], options: [.skipsHiddenFiles])
                else { continue }
                for f in files where f.pathExtension == "jsonl" {
                    let name = f.lastPathComponent
                    if name.contains(".orphaned-") || name.contains(".superseded-") { continue }
                    guard let v = try? f.resourceValues(forKeys: [.isRegularFileKey, .contentModificationDateKey, .fileSizeKey]),
                          v.isRegularFile == true, let m = v.contentModificationDate else { continue }
                    out.append(SessionFile(url: f, mtime: m, size: Int64(v.fileSize ?? 0)))
                }
            }
        }
        return out
    }

    // MARK: which project a file belongs to
    private static let cwdKeys = [Array("\"cwd\":\"".utf8), Array("\"cwd\": \"".utf8)]

    /// The working folder named by a line, read from its raw bytes: nothing else in the record is decoded.
    private static func cwd(in line: Data) -> String? {
        for key in cwdKeys {
            guard let r = line.range(of: Data(key)) else { continue }
            var bytes = [UInt8](), i = r.upperBound
            while i < line.endIndex {
                let b = line[i]
                if b == 0x22 { return String(decoding: bytes, as: UTF8.self) }
                if b == 0x5C {                                                        // an escape in the path: let JSON decode that one
                    return (try? JSONSerialization.jsonObject(with: line)).flatMap { ($0 as? [String: Any])?["cwd"] as? String }
                }
                bytes.append(b); i += 1
            }
        }
        return nil
    }

    /// The working folder of the first record that names one, looking at no more than the first 512 KB.
    private func firstCwd(_ url: URL) -> String? {
        var found: String?
        _ = try? JSONLReader(url: url).forEachLine(limit: 512 << 10) { line in
            if let c = ClaudeCodeSource.cwd(in: line), !c.isEmpty { found = c; return false }
            return true
        }
        return found
    }

    // MARK: the projects (Settings)
    func projects(control: ScanControl) -> [SourceProject] {
        var byRoot = [String: SourceProject]()
        let groups = Dictionary(grouping: sessionFiles(), by: { $0.url.deletingLastPathComponent().path })
        for (i, files) in groups.values.enumerated() {
            if control.isCancelled { break }
            control.progress(i, groups.count)
            let newest = files.sorted { $0.mtime > $1.mtime }
            for f in newest.prefix(4) {
                guard let c = firstCwd(f.url), !c.isEmpty else { continue }
                let root = SourcePath.collapseWorktree(c)
                var p = byRoot[root.lowercased()] ?? SourceProject(root: root, name: SourcePath.name(of: root), sources: ["claude"], lastActive: nil)
                if let m = newest.first?.mtime, m > (p.lastActive ?? .distantPast) { p.lastActive = m }
                byRoot[root.lowercased()] = p
                break
            }
        }
        return Array(byRoot.values)
    }

    // MARK: one day
    func activity(day: String, calendar: Calendar, policy: ProjectPolicy, control: ScanControl) -> SourceResult {
        var res = SourceResult()
        guard !policy.isEmpty, let start = DayKey.date(day, calendar), let end = calendar.date(byAdding: .day, value: 1, to: start) else { return res }
        let lo = start.timeIntervalSince1970, hi = end.timeIntervalSince1970
        // A file's modified time is its newest record, so an older file cannot hold anything from this day.
        let files = sessionFiles().filter { $0.mtime >= start }.sorted { $0.mtime < $1.mtime }

        var ms = [String: [String: Set<Int64>]]()                     // project -> session -> record times (ms): copies of a record collapse
        var firstPrompt = [String: [String: FileDigest.Prompt]]()     // project -> session -> that session's first prompt of the day
        var edits = [String: [String: Int]](), editSeen = Set<String>()
        var commits = [String: [FileDigest.Commit]](), commitSeen = Set<String>()
        var prs = [String: [String]](), prSeen = Set<String>()
        var unreadable = 0

        for (i, f) in files.enumerated() {
            if control.isCancelled { break }
            control.progress(i, files.count)
            guard let entry = entry(for: f, policy: policy, control: control) else { if !control.isCancelled { unreadable += 1 }; continue }
            guard case .digest(let d) = entry else { continue }
            res.stats.add(d.stats)
            for (k, ts) in d.ticks { for t in ts where t >= lo && t < hi { ms[k.root, default: [:]][k.session, default: []].insert(Int64(t * 1000)) } }
            for p in d.prompts where p.t >= lo && p.t < hi {
                if let old = firstPrompt[p.root]?[p.session], old.t <= p.t { continue }
                firstPrompt[p.root, default: [:]][p.session] = p
            }
            for e in d.edits where e.t >= lo && e.t < hi && (e.id.isEmpty || editSeen.insert(e.id).inserted) { edits[e.root, default: [:]][e.path, default: 0] += 1 }
            for c in d.commits where c.t >= lo && c.t < hi && !c.failed {
                if commitSeen.insert(c.id.isEmpty ? "\(c.root)|\(c.t)" : c.id).inserted { commits[c.root, default: []].append(c) }
            }
            for p in d.prs where p.t >= lo && p.t < hi && prSeen.insert(p.url).inserted { prs[p.root, default: []].append(p.url) }
        }

        for (root, sessions) in ms {
            let name = SourcePath.name(of: root)
            let times = sessions.values.flatMap { $0 }.map { Double($0) / 1000 }
            res.items.append(ActivityItem(kind: .session, root: root, project: name, title: "Claude Code", time: Date(timeIntervalSince1970: times.min() ?? lo),
                                          count: sessions.count, seconds: ClaudeTime.activeSeconds(times, gap: ClaudeCodeSource.idleGap)))
            let chosen = (firstPrompt[root] ?? [:]).values.sorted { $0.t < $1.t }.prefix(ClaudeCodeSource.maxPrompts)
            for p in chosen {
                res.items.append(ActivityItem(kind: .prompt, root: root, project: name, title: p.text, time: Date(timeIntervalSince1970: p.t)))
            }
            for (path, n) in (edits[root] ?? [:]).sorted(by: { $0.value != $1.value ? $0.value > $1.value : $0.key < $1.key }).prefix(60) {
                res.items.append(ActivityItem(kind: .file, root: root, project: name, title: path, count: n))
            }
            for c in (commits[root] ?? []).sorted(by: { $0.t < $1.t }).prefix(12) where !c.subject.isEmpty {
                res.items.append(ActivityItem(kind: .commit, root: root, project: name, title: c.subject, detail: c.sha, time: Date(timeIntervalSince1970: c.t),
                                              evidence: c.sha))
            }
            for u in (prs[root] ?? []).prefix(5) {
                res.items.append(ActivityItem(kind: .pullRequest, root: root, project: name, title: u, evidence: u))
            }
        }
        if unreadable > 0 && res.stats.files == 0 {
            res.notices.append("Gloamlog can't read Claude Code's files. Allow access in System Settings > Privacy & Security > Files & Folders.")
        }
        return res
    }

    // MARK: cache (path + modified time + size + the ticked projects)
    private func entry(for f: SessionFile, policy: ProjectPolicy, control: ScanControl) -> Entry? {
        let stamp = FileCache.Stamp(mtime: f.mtime.timeIntervalSince1970, size: f.size, policy: policy.key)
        if let hit = cache.get(f.url.path, stamp) { return hit }
        let made: Entry
        let first = firstCwd(f.url)                                   // a file whose first working folder is not ticked is never read further
        if let c = first, policy.root(for: c) == nil { made = .skipped }
        else {
            var parser = TranscriptParser(policy: policy, anchor: first.flatMap { policy.root(for: $0) }, fallbackSession: f.url.deletingPathExtension().lastPathComponent)
            guard let out = try? JSONLReader(url: f.url).forEachLine({ line in autoreleasepool { parser.handle(line) }; return !control.isCancelled }), !control.isCancelled else { return nil }
            parser.digest.stats.skipped += out.skippedLong
            parser.digest.stats.files = 1
            made = .digest(parser.digest)
        }
        cache.put(f.url.path, stamp, made)
        return made
    }

    func clearCache() { cache.clear() }

    private final class FileCache {
        struct Stamp: Equatable { var mtime: TimeInterval; var size: Int64; var policy: String }
        private var map = [String: (stamp: Stamp, entry: Entry)]()
        private let lock = NSLock()
        func get(_ path: String, _ s: Stamp) -> Entry? { lock.lock(); defer { lock.unlock() }; if let e = map[path], e.stamp == s { return e.entry }; return nil }
        func put(_ path: String, _ s: Stamp, _ e: Entry) { lock.lock(); defer { lock.unlock() }; if map.count > 600 { map.removeAll() }; map[path] = (s, e) }
        func clear() { lock.lock(); map.removeAll(); lock.unlock() }
    }
}
