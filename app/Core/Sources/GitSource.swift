// GitSource.swift - your own commits on a day, from the repositories you ticked (M6). Foundation only.
//
// READ-ONLY and local: it runs `git log` (and `git config --get user.email`), which never writes and never touches the network,
// through Process with a minimal environment, no terminal and no pager, and a timeout. It only runs in repositories the owner
// ticked, and only shows commits whose author email is the owner's own git `user.email`; merge commits are left out.
import Foundation

final class GitSource: ActivitySource {
    let id = "git"
    static let timeout: TimeInterval = 10
    static let maxRepos = 400
    static let maxCommitsPerRepo = 300

    private let roots: [String]
    private let git: String?

    /// `roots` are folders the owner added: a repository itself, or a folder that holds repositories (looked at 3 levels deep).
    init(roots: [String], gitPath: String? = GitSource.locateGit()) {
        self.roots = roots.map(SourcePath.normalize)
        self.git = gitPath
    }

    /// A real git binary. `/usr/bin/git` is not used: without the developer tools it is a stub that opens an install dialog.
    static func locateGit() -> String? {
        ["/Library/Developer/CommandLineTools/usr/bin/git", "/Applications/Xcode.app/Contents/Developer/usr/bin/git",
         "/opt/homebrew/bin/git", "/usr/local/bin/git"].first { FileManager.default.isExecutableFile(atPath: $0) }
    }

    // MARK: repositories
    /// Repositories under the added folders (a folder with `.git` is one; what is inside it belongs to it). Symbolic links, hidden
    /// folders and the usual build folders are skipped, and the walk is bounded, so adding a huge folder cannot hang the app.
    func repositories(control: ScanControl) -> [String] {
        let fm = FileManager.default
        let skip: Set<String> = ["node_modules", "Library", "Pods", "venv", "target", "build", "dist", "vendor"]
        var found = [String](), visited = 0
        func walk(_ dir: String, _ depth: Int) {
            guard visited < 5000, found.count < GitSource.maxRepos, !control.isCancelled else { return }
            visited += 1
            if fm.fileExists(atPath: dir + "/.git") { found.append(dir); return }
            guard depth < 3, let kids = try? fm.contentsOfDirectory(atPath: dir) else { return }
            for k in kids.sorted() where !k.hasPrefix(".") && !skip.contains(k) {
                let p = dir + "/" + k
                var isDir: ObjCBool = false
                guard fm.fileExists(atPath: p, isDirectory: &isDir), isDir.boolValue, (try? fm.destinationOfSymbolicLink(atPath: p)) == nil else { continue }
                walk(p, depth + 1)
            }
        }
        for r in roots { walk(r, 0) }
        var seen = Set<String>()
        return found.filter { seen.insert($0.lowercased()).inserted }
    }

    func projects(control: ScanControl) -> [SourceProject] {
        repositories(control: control).map { SourceProject(root: $0, name: SourcePath.name(of: $0), sources: ["git"], lastActive: nil) }
    }

    // MARK: one day
    func activity(day: String, calendar: Calendar, policy: ProjectPolicy, control: ScanControl) -> SourceResult {
        var res = SourceResult()
        guard !policy.isEmpty, !roots.isEmpty, let start = DayKey.date(day, calendar), let end = calendar.date(byAdding: .day, value: 1, to: start) else { return res }
        guard let git = git else { res.notices.append("Git isn't installed on this Mac, so commits can't be listed."); return res }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX"); f.calendar = calendar; f.timeZone = calendar.timeZone
        f.dateFormat = "yyyy-MM-dd'T'HH:mm:ssZZZ"
        // git stops walking at the first commit older than --since, so a history whose dates are not strictly in order (a clock that
        // was off, a rebase) would hide later commits: ask for a wider window and cut to the day here, by commit time.
        let since = f.string(from: calendar.date(byAdding: .day, value: -2, to: start) ?? start)
        let until = f.string(from: calendar.date(byAdding: .day, value: 1, to: end) ?? end)
        let lo = start.timeIntervalSince1970, hi = end.timeIntervalSince1970

        let repos = repositories(control: control).compactMap { r in policy.root(for: r).map { (path: r, root: $0) } }
        var noEmail = 0
        for (i, repo) in repos.enumerated() {
            if control.isCancelled { break }
            control.progress(i, repos.count)
            guard let email = GitSource.run(git, ["-C", repo.path, "config", "--get", "user.email"])?.dlTrimmed, !email.isEmpty else { noEmail += 1; continue }
            let args = ["-C", repo.path, "-c", "core.fsmonitor=false", "log", "--branches", "--no-merges", "-i", "--fixed-strings", "--author=\(email)",
                        "--since=\(since)", "--until=\(until)", "--max-count=\(GitSource.maxCommitsPerRepo)", "--format=%H%x1f%ae%x1f%ct%x1f%s"]
            guard let out = GitSource.run(git, args) else { res.stats.skipped += 1; continue }
            res.stats.files += 1
            let name = SourcePath.name(of: repo.root)
            for line in out.split(separator: "\n") {
                let p = line.components(separatedBy: "\u{1f}")
                guard p.count >= 4, p[1].lowercased() == email.lowercased(), let ct = TimeInterval(p[2]), ct >= lo, ct < hi else { continue }
                let sha = String(p[0].prefix(7))
                res.items.append(ActivityItem(kind: .commit, root: repo.root, project: name, title: p[3...].joined(separator: "\u{1f}"), detail: sha,
                                              time: Date(timeIntervalSince1970: ct), evidence: sha))
            }
        }
        if noEmail > 0 { res.notices.append("Set your email with `git config --global user.email` to see your commits.") }
        return res
    }

    // MARK: running git
    /// Runs git and returns its output (at most 1 MB), or nil when it failed or took longer than the timeout. No shell, no
    /// terminal, no pager, no prompts, no repository locks; the environment is only what git needs to find its own configuration.
    static func run(_ git: String, _ args: [String], timeout: TimeInterval = GitSource.timeout) -> String? {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: git)
        p.arguments = args
        p.environment = ["PATH": "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin", "HOME": NSHomeDirectory(), "GIT_TERMINAL_PROMPT": "0",
                         "GIT_OPTIONAL_LOCKS": "0", "GIT_PAGER": "cat", "GIT_CONFIG_NOSYSTEM": "1"]
        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = FileHandle.nullDevice
        p.standardInput = FileHandle.nullDevice
        do { try p.run() } catch { return nil }
        let killer = DispatchWorkItem { if p.isRunning { p.terminate() } }
        DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + timeout, execute: killer)
        let data = pipe.fileHandleForReading.readDataToEndOfFile()            // ends when git exits, or is terminated by the timeout
        p.waitUntilExit()
        killer.cancel()
        guard p.terminationStatus == 0 else { return nil }
        return String(decoding: data.prefix(1_000_000), as: UTF8.self)
    }
}
