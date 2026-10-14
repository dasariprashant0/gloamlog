// ClaudeMd.swift - the short instruction Gloamlog adds to ~/.claude/CLAUDE.md so Claude Code knows when to call `log_work`.
// Pure text work, Foundation only. The section is wrapped in markers so it can be replaced or removed without touching anything else
// in the file. A file that already tells Claude to use `log_work` (written by hand) counts as set up and is left alone.
import Foundation

enum ClaudeMd {
    static let begin = "<!-- gloamlog:begin -->"
    static let end = "<!-- gloamlog:end -->"

    static let instruction = """
    ## Gloamlog work log
    When a meaningful piece of work is finished in a session (a problem solved, a deliverable done, a decision made, or I say I'm done), call the gloamlog `log_work` tool once with 1-3 plain sentences: the problem I was solving, what was done, and what is still open. Not after every step. No secrets, customer data or code.
    """

    static var block: String { begin + "\n" + instruction + "\n" + end }

    /// True when the text already carries our block, or an older hand-written section that mentions `log_work`.
    static func isPresent(in text: String) -> Bool { text.contains(begin) || text.contains("log_work") }

    /// `text` with the block added at the end, or replaced when ours is already there. A hand-written section is left as it is.
    static func apply(to text: String) -> String {
        if let r = markedRange(in: text) { return text.replacingCharacters(in: r, with: block) }
        if text.contains("log_work") { return text }
        let trimmed = text.replacingOccurrences(of: "\\s+$", with: "", options: .regularExpression)
        return (trimmed.isEmpty ? "" : trimmed + "\n\n") + block + "\n"
    }

    /// `text` without our block (and the blank line before it). Anything else, including a hand-written section, stays.
    static func remove(from text: String) -> String {
        guard let r = markedRange(in: text) else { return text }
        var out = text.replacingCharacters(in: r, with: "")
        out = out.replacingOccurrences(of: "\n{3,}", with: "\n\n", options: .regularExpression)
        let trimmed = out.replacingOccurrences(of: "\\s+$", with: "", options: .regularExpression)
        return trimmed.isEmpty ? "" : trimmed + "\n"
    }

    private static func markedRange(in text: String) -> Range<String.Index>? {
        guard let b = text.range(of: begin), let e = text.range(of: end, range: b.upperBound..<text.endIndex) else { return nil }
        return b.lowerBound..<e.upperBound
    }
}
