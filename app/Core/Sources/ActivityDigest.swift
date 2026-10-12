// ActivityDigest.swift - turns what the sources found into the short bullets the owner can insert (M6). Foundation only.
//
// This is the ONE place text leaves the sources: every string that can reach the screen or a page is redacted here
// (Redactor), cut to a short length AFTER redaction (so a secret is never cut in half and left half-visible), and put in one line.
// Typical output for a day:
//   Worked in acme-site with Claude Code for about 2h 55m (3 sessions, 6 files edited)
//   Worked on acme-site: Add a pricing table to the landing page
//   Committed: Add pricing table (acme-site 3f2a9c1)
//   Pull request: https://github.com/acme/site/pull/42 (acme-site)
import Foundation

struct SuggestedBullet: Equatable, Identifiable {
    var id: String
    var kind: ActivityKind
    var project: String
    var text: String                // the bullet as it reads on the page, redacted, one line
    var detail: String              // a second line for the strip only (the files touched)
    var evidence: String            // the part of `text` that shows the bullet is already on the page
    var time: Date?
}

struct DigestResult: Equatable {
    var bullets: [SuggestedBullet]
    var hidden: [Redactor.Kind: Int]        // what the redactor hid, for "4 values hidden (2 emails, 2 tokens)"
}

enum ActivityDigest {
    static let maxPrompt = 140, maxSubject = 120, maxName = 60

    static func bullets(from raw: [ActivityItem], redactor: Redactor = .standard) -> DigestResult {
        var hidden = [Redactor.Kind: Int]()
        func clean(_ s: String, _ limit: Int) -> String {
            let r = redactor.apply(s)
            for (k, n) in r.hidden { hidden[k, default: 0] += n }
            return truncate(ClaudeText.collapse(r.text), limit)
        }

        // The same item twice (two sources, or a session and its copy) is one item; a commit without a hash that another
        // item already shows with one is that commit.
        var seen = Set<String>()
        var items = raw.filter { seen.insert($0.id).inserted }
        let known = Set(items.filter { $0.kind == .commit && !$0.evidence.isEmpty }.map { "\($0.root)|\($0.title.lowercased())" })
        items.removeAll { $0.kind == .commit && $0.evidence.isEmpty && known.contains("\($0.root)|\($0.title.lowercased())") }

        let groups = Dictionary(grouping: items, by: { $0.root })
        func rank(_ g: [ActivityItem]) -> (Int, Int, String) {
            (-(g.first { $0.kind == .session }?.seconds ?? 0), -g.filter { $0.kind == .commit }.count, g.first?.project.lowercased() ?? "")
        }
        var out = [SuggestedBullet]()
        for g in groups.values.sorted(by: { rank($0) < rank($1) }) {
            guard let first = g.first else { continue }
            let name = clean(first.project, maxName)
            if let s = g.first(where: { $0.kind == .session }) {
                let files = g.filter { $0.kind == .file }
                var parts = ["\(s.count) session\(s.count == 1 ? "" : "s")"]
                if !files.isEmpty { parts.append("\(files.count) file\(files.count == 1 ? "" : "s") edited") }
                let lead = "Worked in \(name) with Claude Code"
                let span = s.seconds >= 60 ? " for about \(duration(s.seconds))" : ""
                var detail = files.prefix(3).map { clean($0.title, 60) + ($0.count > 1 ? " (\($0.count))" : "") }.joined(separator: ", ")
                if files.count > 3 { detail += ", +\(files.count - 3) more" }
                out.append(SuggestedBullet(id: "session|\(first.root)", kind: .session, project: name, text: "\(lead)\(span) (\(parts.joined(separator: ", ")))",
                                           detail: detail, evidence: lead, time: s.time))
            }
            for p in g where p.kind == .prompt {
                let line = clean(p.title, maxPrompt)
                guard line.replacingOccurrences(of: "[redacted]", with: "").dlTrimmed.count >= 8 else { continue }
                out.append(SuggestedBullet(id: "prompt|\(g.first?.root ?? "")|\(line)", kind: .prompt, project: name, text: "Worked on \(name): \(line)", detail: "", evidence: line, time: p.time))
            }
            for c in g.filter({ $0.kind == .commit }).sorted(by: { ($0.time ?? .distantPast) < ($1.time ?? .distantPast) }) {
                let subject = clean(c.title, maxSubject)
                guard !subject.isEmpty else { continue }
                let sha = clean(c.evidence, 12)
                out.append(SuggestedBullet(id: "commit|\(first.root)|\(sha.isEmpty ? subject : sha)", kind: .commit, project: name, text: "Committed: \(subject) (\(name)\(sha.isEmpty ? "" : " " + sha))",
                                           detail: "", evidence: sha.isEmpty ? subject : "\(name) \(sha)", time: c.time))
            }
            for p in g where p.kind == .pullRequest {
                let url = clean(p.title, 200)
                out.append(SuggestedBullet(id: "pr|\(first.root)|\(url)", kind: .pullRequest, project: name, text: "Pull request: \(url) (\(name))", detail: "", evidence: url, time: p.time))
            }
        }
        return DigestResult(bullets: out, hidden: hidden)
    }

    // MARK: markdown
    /// The bullets as markdown list items. Everything that could mean something to markdown is backslash-escaped, so what was
    /// typed in a commit message or a prompt can never become a heading, a link, a table or raw HTML on the page.
    static func markdown(_ bullets: [SuggestedBullet]) -> String {
        bullets.map { "- " + escape($0.text) }.joined(separator: "\n")
    }
    static func escape(_ s: String) -> String {
        var out = ""
        for ch in s {
            if "\\`*_[]<>~|&#".contains(ch) { out.append("\\") }
            out.append(ch)
        }
        return out
    }

    // MARK: "already on the page"
    private static let escapes = try? NSRegularExpression(pattern: #"\\([!-/:-@\[-`{-~])"#)
    /// A page's markdown with escapes undone and whitespace collapsed: what a reader sees, for comparing with `evidence`.
    static func plainText(_ markdown: String) -> String {
        let ns = markdown as NSString
        let undone = escapes?.stringByReplacingMatches(in: markdown, range: NSRange(location: 0, length: ns.length), withTemplate: "$1") ?? markdown
        return ClaudeText.collapse(undone)
    }
    static func isAdded(_ b: SuggestedBullet, inPlainPage page: String) -> Bool {
        let e = ClaudeText.collapse(b.evidence)
        return !e.isEmpty && page.contains(e)
    }

    // MARK: small helpers
    /// "about" times: under a minute, minutes, then hours and minutes; rounded to 5 minutes from 10 minutes up.
    static func duration(_ seconds: Int) -> String {
        if seconds < 60 { return "under a minute" }
        var m = seconds / 60
        if m >= 10 { m = Int((Double(m) / 5).rounded()) * 5 }
        if m < 60 { return "\(m) min" }
        return m % 60 == 0 ? "\(m / 60)h" : "\(m / 60)h \(m % 60)m"
    }

    /// At most `limit` characters, cut at a word when one is near the end, with an ellipsis.
    static func truncate(_ s: String, _ limit: Int) -> String {
        guard s.count > limit else { return s }
        var cut = String(s.prefix(limit))
        if let sp = cut.lastIndex(of: " "), cut.distance(from: cut.startIndex, to: sp) > limit / 2 { cut = String(cut[..<sp]) }
        while let l = cut.last, " ,;:.-–—".contains(l) { cut.removeLast() }
        return cut + "…"
    }
}
