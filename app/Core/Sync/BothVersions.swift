// BothVersions.swift - "Keep both": two versions of one day's page become one without losing a line. Pure and deterministic.
//
// The base text (this Mac's, or the open page's) is kept exactly; the lines that exist only in the other copy go under a
// "From another copy" heading at the end. A line is compared by its trimmed text against the trimmed lines of the base; the extra
// lines keep their order and their own indentation, and a blank line stays between two extra lines only where the other copy had
// one. When the other copy has nothing new (the same text, a subset, or empty) the base comes back unchanged.
//
// The rule is deliberately dumb: a sentence edited on both Macs shows up once from each side and a ticked task sits next to its
// unticked twin. Nothing is ever lost, and every version also stays in the safety copies. Jots from two Macs are separate stamped
// lines, so they simply add up.
import Foundation

enum BothVersions {
    static let heading = "From another copy"

    private static func lines(_ text: String) -> [String] {
        text.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n")
    }
    private static func keys(_ lines: [String]) -> Set<String> {
        Set(lines.map { $0.dlTrimmed }.filter { !$0.isEmpty })
    }

    /// The lines of `theirs` that `mine` does not have, as they will be appended (blank lines only between two of them).
    static func extraLines(mine: String, theirs: String) -> [String] {
        let have = keys(lines(mine))
        var out = [String]()
        var previousWasExtra = false, blanks = 0
        for line in lines(theirs) {
            let t = line.dlTrimmed
            if t.isEmpty { if previousWasExtra { blanks += 1 }; continue }
            if have.contains(t) { previousWasExtra = false; blanks = 0; continue }
            if previousWasExtra && blanks > 0 { out.append("") }
            out.append(line)
            previousWasExtra = true; blanks = 0
        }
        return out
    }

    static func merge(mine: String, theirs: String) -> String {
        let extra = extraLines(mine: mine, theirs: theirs)
        if extra.isEmpty { return mine }
        var base = mine
        while let last = base.last, last.isNewline || last == " " || last == "\t" { base.removeLast() }
        if base.isEmpty { return extra.joined(separator: "\n") }                  // nothing of ours to keep apart from
        return base + "\n\n## " + heading + "\n\n" + extra.joined(separator: "\n")
    }

    /// For the conflict sheet: every line of `text`, and whether `other` lacks it (blank lines never differ).
    static func marked(_ text: String, against other: String) -> [(text: String, differs: Bool)] {
        let have = keys(lines(other))
        return lines(text).map { line in
            let k = line.dlTrimmed
            return (text: line, differs: !k.isEmpty && !have.contains(k))
        }
    }
}
