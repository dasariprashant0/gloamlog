// MarkdownBody+Jots.swift - the Jots block in the words rule and in the editor's initial/written text. Pure. Owned by the core stream (task K1).
// CONTRACT (signatures frozen). A "Jots block" = a heading whose MarkdownBody.normalizeHeading equals the
// normalised jotsHeading, at any level, running to the next heading of the same or higher level (the rule isCarriedHeading already uses).
//   counts(in:jotsHeading:)            words outside the block, words inside it, number of jot list items
//   isJotsOnly(_:jotsHeading:)         true when the page is nothing but a Jots block (blank lines allowed), false for an empty page
//   stripUntouchedTemplate(_:template:jotsHeading:)   if md starts with the trimmed template and the rest is a Jots block only (or nothing),
//                                       return just that rest; otherwise return md unchanged. (A jots-only page is shown WITH the template
//                                       above it in the editor, but never writes the template back to disk.)
//   loggedWords(in:jotsHeading:jotsCount:)   what the "logged" rule counts: counts.words, plus jotWords only when jotsCount is true
// Details (all tested in tests/main.swift):
//   * Every Jots block of the page counts as jots (the first one is where CapturePlacement appends). A configured heading that normalises to
//     nothing (empty, emoji only) means "Jots". Fences and HTML comments are respected (a "## Jots" inside one is not a heading).
//   * jotWords: all words of the block, except a LEADING H:mm / HH:mm token on a list item (the time stamp is not a word); carried-over task
//     lines never count, code counts like everywhere else. jotCount: list items (bullet, ordered, task) indented less than two spaces, i.e.
//     the top-level notes; the nested items and continuation lines of a multi-line note are words but not extra notes.
//   * A page with no Jots block: words == words(in:), jotWords == jotCount == 0.
//   * stripUntouchedTemplate compares the template and the page line by line, ignoring blank lines and surrounding spaces, because the
//     editor re-serialises the template's blank lines; it never touches a page where the user wrote anything outside the Jots block.
import Foundation

extension MarkdownBody {
    struct Counts: Equatable { var words: Int; var jotWords: Int; var jotCount: Int }

    // MARK: the block
    /// One Jots block in a scan: `heading` is the line of the heading, the content lines are heading+1 ..< end.
    struct JotsBlock: Equatable { var heading: Int; var level: Int; var end: Int }

    /// What a heading is matched against: its normalised text; "jots" when the configured heading normalises to nothing.
    static func jotsTarget(_ jotsHeading: String) -> String {
        let n = normalizeHeading(jotsHeading)
        return n.isEmpty ? "jots" : n
    }
    /// The text a NEW Jots heading is written with: the configured text with whitespace collapsed, or "Jots" when it is unusable.
    static func jotsHeadingText(_ jotsHeading: String) -> String {
        let c = collapse(jotsHeading)
        return normalizeHeading(c).isEmpty ? "Jots" : c
    }

    /// Every Jots block, in document order. A deeper heading stays inside the block; a heading of the same or a higher level ends it.
    static func jotsBlocks(in lines: [ScannedLine], jotsHeading: String) -> [JotsBlock] {
        let target = jotsTarget(jotsHeading)
        var out = [JotsBlock]()
        var open: (heading: Int, level: Int)?
        for (i, l) in lines.enumerated() {
            guard case .heading(let level, let title) = l.kind else { continue }
            if let o = open {
                if level > o.level { continue }
                out.append(JotsBlock(heading: o.heading, level: o.level, end: i)); open = nil
            }
            if normalizeHeading(title) == target { open = (i, level) }
        }
        if let o = open { out.append(JotsBlock(heading: o.heading, level: o.level, end: lines.count)) }
        return out
    }

    /// The closing fence line ("```", "~~~~") when the page ends inside an open code fence, else nil.
    static func unclosedFenceCloser(_ lines: [ScannedLine]) -> String? {
        let fences = lines.indices.filter { lines[$0].kind == .fence }
        guard fences.count % 2 == 1, let last = fences.last, let f = openingFence(lines[last].text) else { return nil }
        return String(repeating: f.ch, count: f.len)
    }

    // MARK: list items and times
    /// The indentation (a tab counts 4) when `line` starts a list item: "-", "*", "+" or "1." / "1)" followed by a space or the end of the
    /// line. A thematic break ("---", "* * *") is not one. nil for anything else.
    static func listItemIndent(_ line: String) -> Int? {
        var indent = 0
        var s = Substring(line)
        while let c = s.first, c == " " || c == "\t" { indent += c == "\t" ? 4 : 1; s = s.dropFirst() }
        guard let first = s.first else { return nil }
        let rest: Substring
        if "-*+".contains(first) {
            rest = s.dropFirst()
        } else {
            let digits = s.prefix(while: { ("0"..."9").contains($0) })
            guard !digits.isEmpty, digits.count <= 9, let d = s.dropFirst(digits.count).first, d == "." || d == ")" else { return nil }
            rest = s.dropFirst(digits.count + 1)
        }
        guard rest.isEmpty || rest.first == " " || rest.first == "\t" else { return nil }
        let bare = line.filter { $0 != " " && $0 != "\t" }
        if bare.count >= 3, let f = bare.first, "-*_".contains(f), bare.allSatisfy({ $0 == f }) { return nil }
        return indent
    }

    /// "14:32 call Sam" -> "call Sam". Drops a leading H:mm / HH:mm token (real times only: 00:00 to 23:59, two-digit minutes, then a
    /// space or the end). Anything else is returned unchanged.
    static func dropLeadingTime(_ text: String) -> String {
        let t = Substring(text)
        func digits(_ from: Substring.Index, max: Int) -> (value: Int, count: Int, end: Substring.Index) {
            var i = from, v = 0, n = 0
            while i < t.endIndex, n < max, let d = t[i].wholeNumberValue, ("0"..."9").contains(t[i]) { v = v * 10 + d; n += 1; i = t.index(after: i) }
            return (v, n, i)
        }
        let h = digits(t.startIndex, max: 2)
        guard h.count >= 1, h.value <= 23, h.end < t.endIndex, t[h.end] == ":" else { return text }
        let m = digits(t.index(after: h.end), max: 2)
        guard m.count == 2, m.value <= 59, m.end == t.endIndex || t[m.end] == " " || t[m.end] == "\t" else { return text }
        return String(t[m.end...])
    }

    // MARK: counts
    static func counts(in body: String, jotsHeading: String = "Jots") -> Counts {
        let lines = scan(body)
        let blocks = jotsBlocks(in: lines, jotsHeading: jotsHeading)
        guard !blocks.isEmpty else { return Counts(words: wordCount(contentLines(of: lines)), jotWords: 0, jotCount: 0) }
        var inBlock = [Bool](repeating: false, count: lines.count)
        var jotWords = 0, jotCount = 0
        for b in blocks {
            for i in b.heading..<b.end { inBlock[i] = true }
            let content = Array(lines[(b.heading + 1)..<b.end])
            jotWords += wordCount(contentLines(of: content), dropJotTimes: true)
            jotCount += content.filter { $0.kind == .text && (listItemIndent($0.text) ?? Int.max) < 2 }.count
        }
        let outside = zip(lines, inBlock).filter { !$0.1 }.map { $0.0 }
        return Counts(words: wordCount(contentLines(of: outside)), jotWords: jotWords, jotCount: jotCount)
    }

    static func isJotsOnly(_ body: String, jotsHeading: String = "Jots") -> Bool {
        let lines = scan(body)
        let blocks = jotsBlocks(in: lines, jotsHeading: jotsHeading)
        guard !blocks.isEmpty else { return false }
        var covered = [Bool](repeating: false, count: lines.count)
        for b in blocks { for i in b.heading..<b.end { covered[i] = true } }
        for (i, l) in lines.enumerated() where l.kind != .blank && !covered[i] { return false }
        return true
    }

    static func stripUntouchedTemplate(_ md: String, template: String, jotsHeading: String = "Jots") -> String {
        func split(_ s: String) -> [String] { s.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n") }
        let want = split(template).map { $0.dlTrimmed }.filter { !$0.isEmpty }
        guard !want.isEmpty else { return md }
        let lines = split(md)
        var i = 0, matched = 0
        while i < lines.count, matched < want.count {
            let l = lines[i].dlTrimmed
            if !l.isEmpty { guard l == want[matched] else { return md }; matched += 1 }
            i += 1
        }
        guard matched == want.count else { return md }
        let rest = trimBody(lines[i...].joined(separator: "\n"))
        if rest.isEmpty { return "" }
        return isJotsOnly(rest, jotsHeading: jotsHeading) ? rest : md
    }

    static func loggedWords(in body: String, jotsHeading: String = "Jots", jotsCount: Bool = false) -> Int {
        let c = counts(in: body, jotsHeading: jotsHeading)
        return c.words + (jotsCount ? c.jotWords : 0)
    }
}
