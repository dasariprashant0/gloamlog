// MarkdownBody.swift - utilities for the free-form markdown page that is a day's body. Pure functions, no disk.
//
// API (enum MarkdownBody):
//   words(in:) -> Int                    the logged-rule word count (see "Word counting" below); also fine for a live "n/20" label
//   hasContent(_:) -> Bool               anything besides headings / empty list+task markers / fence lines / comments (an image counts)
//   sections(of:) -> [BodySection]       flat, document order: level (0 = before first heading), title, normalizedTitle, text
//   normalizeHeading(_:) -> String       lowercase, emoji/punctuation stripped, spaces collapsed: "📌 To do next" -> "to do next"
//   headings(inTemplate:) -> [String]    raw heading titles of a template, de-duplicated
//   plainText(_:) -> String              one line of text: markers/images/tags removed (used for snippets)
//   insertCarryOver(into:heading:items:) -> String   puts "## <heading>" + "- [ ] item" lines at the TOP; idempotent; returns `body` unchanged if nothing to add
//   partition(_:headings:subheadings:)   splits a body by owning template heading (nested sub-headings stay with their owner)
//   shiftHeadings(in:minLevel:) / pruneEmptySections(_:) / trimBody(_:)   helpers for the weekly markdown and for saving
// Word counting (decision log): a "word" is a whitespace-delimited token containing at least one letter or number
// (emoji, "-", "***", "|" are not words). Ignored entirely: heading lines, empty list/task markers, code-FENCE lines
// (the ``` lines themselves; the code between them still counts), image syntax ![..](..) and <img>/other HTML tags,
// HTML comments (also multi-line). List markers (-, *, +, 1., 1)), blockquote > and task boxes [ ] [x] are stripped
// before counting. Chinese/Japanese runs (Han, Hiragana, Katakana) have no spaces, so each contiguous run counts as
// ceil(length/2) words. Scripts without spaces that are not CJK (Thai, Lao, Khmer) count one word per space-delimited chunk.
// Carried-over tasks never count: task lines ("- [ ] x", "- [x] x", any marker, nested or not) under a heading that starts with
// "Carried over from" (case-insensitive after normalizeHeading; any level; the block runs to the next heading of the same or
// a higher level, deeper sub-headings stay inside) are skipped by words(in:), ticked or rewritten alike, so one "Carry over"
// click can never log a day. Everything else there still counts (a paragraph or plain bullet typed under the block is the
// user's own writing, and a headingless page that gets a block inserted above it keeps its words). hasContent(_:) still
// sees the block, so a page holding only carried tasks is not "empty" (never deleted, skip refuses it).
// ATX headings only ("# ", "## " ...; up to 3 leading spaces; "#tag" is not a heading). Fenced code (``` or ~~~) is respected.
import Foundation

struct BodySection: Equatable {
    var level: Int            // 0 = text before the first heading, else 1...6
    var title: String         // raw heading text ("📝 What I did"); "" for level 0
    var normalizedTitle: String
    var text: String         // lines up to the next heading (any level); HTML comments removed; outer blank lines trimmed
}

enum SubheadingStyle { case boldLabel, drop }

enum MarkdownBody {
    // MARK: line scanner
    enum LineKind: Equatable { case blank, heading(level: Int, title: String), fence, code, text }
    struct ScannedLine { var kind: LineKind; var text: String }

    static func openingFence(_ line: String) -> (ch: Character, len: Int)? {
        var idx = line.startIndex, spaces = 0
        while idx < line.endIndex, line[idx] == " " { idx = line.index(after: idx); spaces += 1; if spaces > 3 { return nil } }
        guard idx < line.endIndex else { return nil }
        let ch = line[idx]
        guard ch == "`" || ch == "~" else { return nil }
        var n = 0, j = idx
        while j < line.endIndex, line[j] == ch { n += 1; j = line.index(after: j) }
        guard n >= 3 else { return nil }
        if ch == "`", line[j...].contains("`") { return nil }
        return (ch, n)
    }
    static func isClosingFence(_ line: String, _ f: (ch: Character, len: Int)) -> Bool {
        var idx = line.startIndex, spaces = 0
        while idx < line.endIndex, line[idx] == " " { idx = line.index(after: idx); spaces += 1; if spaces > 3 { return false } }
        var n = 0
        while idx < line.endIndex, line[idx] == f.ch { n += 1; idx = line.index(after: idx) }
        return n >= f.len && line[idx...].allSatisfy { $0 == " " || $0 == "\t" }
    }

    static func parseHeading(_ line: String) -> (level: Int, title: String)? {
        var idx = line.startIndex, spaces = 0
        while idx < line.endIndex, line[idx] == " " { idx = line.index(after: idx); spaces += 1; if spaces > 3 { return nil } }
        var n = 0
        while idx < line.endIndex, line[idx] == "#" { n += 1; idx = line.index(after: idx) }
        guard n >= 1, n <= 6 else { return nil }
        if idx < line.endIndex, line[idx] != " ", line[idx] != "\t" { return nil }
        var title = String(line[idx...]).dlTrimmed
        if title.hasSuffix("#") {
            var end = title.endIndex
            while end > title.startIndex, title[title.index(before: end)] == "#" { end = title.index(before: end) }
            if end == title.startIndex { title = "" }
            else if title[title.index(before: end)] == " " || title[title.index(before: end)] == "\t" { title = String(title[..<end]).dlTrimmed }
        }
        return (n, title)
    }

    private static func stripComments(_ line: String, _ inComment: inout Bool) -> String {
        var out = "", s = Substring(line)
        while !s.isEmpty {
            if inComment {
                if let r = s.range(of: "-->") { s = s[r.upperBound...]; inComment = false } else { s = "" }
            } else if let r = s.range(of: "<!--") {
                out += s[..<r.lowerBound]; s = s[r.upperBound...]; inComment = true
            } else { out += s; s = "" }
        }
        return out
    }

    /// Classifies every line. HTML comments are removed from text/heading lines (not from fenced code).
    static func scan(_ body: String) -> [ScannedLine] {
        var out = [ScannedLine]()
        var fence: (ch: Character, len: Int)?
        var inComment = false
        for raw in body.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n") {
            if let f = fence {
                if isClosingFence(raw, f) { fence = nil; out.append(ScannedLine(kind: .fence, text: raw)) }
                else { out.append(ScannedLine(kind: .code, text: raw)) }
                continue
            }
            let line = stripComments(raw, &inComment)
            if let f = openingFence(line) { fence = f; out.append(ScannedLine(kind: .fence, text: line)); continue }
            if line.dlTrimmed.isEmpty { out.append(ScannedLine(kind: .blank, text: "")); continue }
            if let h = parseHeading(line) { out.append(ScannedLine(kind: .heading(level: h.level, title: h.title), text: line)) }
            else { out.append(ScannedLine(kind: .text, text: line)) }
        }
        return out
    }

    // MARK: markers, noise, tokens
    /// Strips blockquote prefixes, one list marker and a task box. `checked` is non-nil when a task box was present.
    static func stripMarkers(_ line: String) -> (text: String, checked: Bool?) {
        var s = Substring(line)
        while true {
            let t = s.drop(while: { $0 == " " })
            guard t.hasPrefix(">") else { break }
            s = t.dropFirst()
            if s.hasPrefix(" ") { s = s.dropFirst() }
        }
        var t = s.drop(while: { $0 == " " || $0 == "\t" })
        func endsOrSpace(_ r: Substring) -> Bool { r.first.map { $0 == " " || $0 == "\t" } ?? true }
        if let f = t.first, "-*+".contains(f), endsOrSpace(t.dropFirst()) {
            t = t.dropFirst()
        } else {
            let digits = t.prefix(while: { ("0"..."9").contains($0) })
            if !digits.isEmpty, digits.count <= 9 {
                let rest = t.dropFirst(digits.count)
                if let d = rest.first, d == "." || d == ")", endsOrSpace(rest.dropFirst()) { t = rest.dropFirst() }
            }
        }
        t = t.drop(while: { $0 == " " || $0 == "\t" })
        var checked: Bool?
        if t.hasPrefix("["), t.count >= 3 {
            let mark = t[t.index(after: t.startIndex)], close = t[t.index(t.startIndex, offsetBy: 2)]
            if close == "]", mark == " " || mark == "x" || mark == "X", endsOrSpace(t.dropFirst(3)) {
                checked = mark != " "; t = t.dropFirst(3)
            }
        }
        return (String(t).dlTrimmed, checked)
    }

    private static let imageRE = try! NSRegularExpression(pattern: "!\\[[^\\]]*\\](\\([^)]*\\)|\\[[^\\]]*\\])")
    private static let tagRE = try! NSRegularExpression(pattern: "</?[A-Za-z][A-Za-z0-9-]*(\\s[^<>]*)?/?>")

    /// Removes markdown images and HTML tags (replaced by a space).
    static func removeNoise(_ s: String) -> String {
        var r = s
        for re in [imageRE, tagRE] {
            r = re.stringByReplacingMatches(in: r, options: [], range: NSRange(r.startIndex..., in: r), withTemplate: " ")
        }
        return r
    }

    private static func isLetterOrNumber(_ u: Unicode.Scalar) -> Bool {
        switch u.properties.generalCategory {
        case .uppercaseLetter, .lowercaseLetter, .titlecaseLetter, .modifierLetter, .otherLetter,
             .decimalNumber, .letterNumber, .otherNumber: return true
        default: return false
        }
    }
    private static func isCJK(_ u: Unicode.Scalar) -> Bool {
        switch u.value {
        case 0x3400...0x4DBF, 0x4E00...0x9FFF, 0xF900...0xFAFF, 0x20000...0x2FA1F,
             0x3040...0x309F, 0x30A0...0x30FF, 0x31F0...0x31FF, 0xFF66...0xFF9F: return true
        default: return false
        }
    }
    private static func tokenWords(_ tok: Substring) -> Int {
        var count = 0, alnum = false, cjk = 0
        for u in tok.unicodeScalars {
            if isCJK(u) {
                if alnum { count += 1; alnum = false }
                cjk += 1
            } else {
                if cjk > 0 { count += (cjk + 1) / 2; cjk = 0 }
                if isLetterOrNumber(u) { alnum = true }
            }
        }
        if alnum { count += 1 }
        if cjk > 0 { count += (cjk + 1) / 2 }
        return count
    }
    static func countTokens(_ s: String) -> Int {
        s.split(whereSeparator: { $0.isWhitespace }).reduce(0) { $0 + tokenWords($1) }
    }

    // MARK: content, words
    struct ContentLine { var text: String; var isCode: Bool; var checked: Bool?; var inCarried: Bool; var listItem = false }

    /// True for "Carried over from ..." (what CarryOver inserts), at any level, however the heading is decorated.
    static func isCarriedHeading(_ title: String) -> Bool {
        let n = normalizeHeading(title)
        return n == "carried over from" || n.hasPrefix("carried over from ")
    }

    /// Lines that carry writing: not blank, not headings, not fence lines; list/quote/task markers stripped.
    /// `inCarried` marks lines inside a "Carried over from" block (see the header); `checked` is non-nil for task lines.
    static func contentLines(_ body: String) -> [ContentLine] { contentLines(of: scan(body)) }
    static func contentLines(of lines: [ScannedLine]) -> [ContentLine] {
        var out = [ContentLine]()
        var carried: Int?   // level of the open "Carried over from" heading
        for l in lines {
            switch l.kind {
            case .blank, .fence: continue
            case .heading(let level, let title):
                if let c = carried, level > c { continue }   // deeper sub-heading: still inside the block
                carried = isCarriedHeading(title) ? level : nil
            case .code:
                if !l.text.dlTrimmed.isEmpty { out.append(ContentLine(text: l.text, isCode: true, checked: nil, inCarried: carried != nil)) }
            case .text:
                let m = stripMarkers(l.text)
                if !m.text.isEmpty {
                    out.append(ContentLine(text: m.text, isCode: false, checked: m.checked, inCarried: carried != nil, listItem: listItemIndent(l.text) != nil))
                }
            }
        }
        return out
    }
    static func words(in body: String) -> Int { wordCount(contentLines(body)) }
    /// The one counting formula. `dropJotTimes` skips a leading H:mm / HH:mm token on list items (a jot's time stamp is not a word).
    static func wordCount(_ lines: [ContentLine], dropJotTimes: Bool = false) -> Int {
        lines.reduce(0) { n, l in
            if l.inCarried && l.checked != nil { return n }   // carried-over task lines never count
            if l.isCode { return n + countTokens(l.text) }
            return n + countTokens(removeNoise(dropJotTimes && l.listItem ? dropLeadingTime(l.text) : l.text))
        }
    }
    static func hasContent(_ body: String) -> Bool { !contentLines(body).isEmpty }

    static func collapse(_ s: String) -> String { s.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ") }

    /// One-line plain text (markers, images, tags removed) for search snippets.
    static func plainText(_ body: String) -> String {
        collapse(contentLines(body).map { $0.isCode ? $0.text : removeNoise($0.text) }.joined(separator: " "))
    }

    // MARK: headings and sections
    static func normalizeHeading(_ s: String) -> String {
        var out = "", pendingSpace = false
        for u in s.unicodeScalars {
            if isLetterOrNumber(u) || isKeptMark(u) {
                if pendingSpace, !out.isEmpty { out += " " }
                pendingSpace = false
                out.unicodeScalars.append(u)
            } else { pendingSpace = true }
        }
        return out.lowercased()
    }
    private static func isKeptMark(_ u: Unicode.Scalar) -> Bool {
        switch u.properties.generalCategory {
        case .nonspacingMark, .spacingMark: return !(0xFE00...0xFE0F).contains(u.value) && !(0xE0100...0xE01EF).contains(u.value)
        default: return false
        }
    }

    static func sections(of body: String) -> [BodySection] {
        var out = [BodySection]()
        var level = 0, title = "", lines = [String]()
        func flush() {
            let text = lines.joined(separator: "\n").dlTrimmed
            if level > 0 || !text.isEmpty {
                out.append(BodySection(level: level, title: title, normalizedTitle: normalizeHeading(title), text: text))
            }
        }
        for l in scan(body) {
            if case .heading(let lv, let t) = l.kind { flush(); level = lv; title = t; lines = [] }
            else { lines.append(l.kind == .blank ? "" : l.text) }
        }
        flush()
        return out
    }

    /// Raw heading titles of a template (any level), de-duplicated by normalised title.
    static func headings(inTemplate template: String) -> [String] {
        var seen = Set<String>(), out = [String]()
        for s in sections(of: template) where s.level > 0 && !s.normalizedTitle.isEmpty {
            if seen.insert(s.normalizedTitle).inserted { out.append(s.title) }
        }
        return out
    }

    // MARK: save / trim
    /// Leading blank lines and trailing whitespace removed; CRLF -> LF. Indentation of the first text line is kept.
    static func trimBody(_ s: String) -> String {
        var lines = s.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n")
        while let f = lines.first, f.dlTrimmed.isEmpty { lines.removeFirst() }
        var out = lines.joined(separator: "\n")
        while let last = out.last, last.isWhitespace { out.removeLast() }
        return out
    }

    // MARK: carry-over
    /// Normalised key used to compare an item with existing lines ("- [ ] Call  Ana" == "call ana").
    static func itemKey(_ text: String) -> String { collapse(stripMarkers(text).text).lowercased() }

    /// Inserts "## <heading>", a blank line and "- [ ] item" lines at the very TOP of `body`.
    /// Idempotent: returns `body` unchanged when a heading with the same normalised title already exists (any level),
    /// or when every item is already present as a line of the body.
    static func insertCarryOver(into body: String, heading: String, items: [String]) -> String {
        let norm = normalizeHeading(heading)
        if sections(of: body).contains(where: { $0.level > 0 && $0.normalizedTitle == norm }) { return body }
        var have = Set(contentLines(body).map { itemKey($0.text) })
        var fresh = [String]()
        for raw in items {
            let item = collapse(raw)
            let key = itemKey(item)
            if key.isEmpty || have.contains(key) { continue }
            have.insert(key); fresh.append(item)
        }
        if fresh.isEmpty { return body }
        let block = "## \(heading)\n\n" + fresh.map { "- [ ] \($0)" }.joined(separator: "\n")
        let rest = trimBody(body)
        return rest.isEmpty ? block : block + "\n\n" + rest
    }

    // MARK: weekly review helpers
    private struct Segment { var label: String?; var lines: [String] }
    private struct Owned { var index: Int; var segs: [Segment] }

    /// Splits `body` by owning heading. `headings` are NORMALISED titles. A matched heading owns everything up to the next
    /// heading of the same or a higher level (deeper sub-headings stay inside, as bold labels or dropped).
    /// Result: matched segments in document order (index into `headings`) and the remaining text ("Other notes").
    static func partition(_ body: String, headings: [String], subheadings: SubheadingStyle = .boldLabel)
        -> (matched: [(index: Int, text: String)], other: String) {
        var owned = [Owned]()
        var other = [Segment(label: nil, lines: [])]
        var owner: Int?, ownerLevel = 0
        for l in scan(body) {
            if case .heading(let level, let title) = l.kind {
                let norm = normalizeHeading(title)
                if !norm.isEmpty, let hi = headings.firstIndex(of: norm) {
                    owned.append(Owned(index: hi, segs: [Segment(label: nil, lines: [])])); owner = owned.count - 1; ownerLevel = level
                } else if let o = owner, level > ownerLevel {
                    owned[o].segs.append(Segment(label: title, lines: []))
                } else {
                    owner = nil; other.append(Segment(label: title, lines: []))
                }
            } else {
                let line = l.kind == .blank ? "" : l.text
                if let o = owner { owned[o].segs[owned[o].segs.count - 1].lines.append(line) }
                else { other[other.count - 1].lines.append(line) }
            }
        }
        func render(_ segs: [Segment]) -> String {
            var parts = [String]()
            for s in segs {
                let text = s.lines.joined(separator: "\n").dlTrimmed
                guard hasContent(text) else { continue }
                if let label = s.label, !label.isEmpty, subheadings == .boldLabel { parts.append("**\(label)**\n" + text) }
                else { parts.append(text) }
            }
            return parts.joined(separator: "\n\n")
        }
        let matched = owned.compactMap { o -> (index: Int, text: String)? in
            let t = render(o.segs); return t.isEmpty ? nil : (o.index, t)
        }
        return (matched, render(other))
    }

    private static func lineHasContent(_ l: ScannedLine) -> Bool {
        switch l.kind {
        case .code: return !l.text.dlTrimmed.isEmpty
        case .text: return !stripMarkers(l.text).text.isEmpty
        default: return false
        }
    }
    private static func rebuild(_ lines: [ScannedLine]) -> String {
        var out = [String](), lastBlank = true
        for l in lines {
            if l.kind == .blank { if !lastBlank { out.append("") }; lastBlank = true; continue }
            lastBlank = false
            if case .heading(let lv, let t) = l.kind { out.append(String(repeating: "#", count: lv) + (t.isEmpty ? "" : " " + t)) }
            else { out.append(l.text) }
        }
        return trimBody(out.joined(separator: "\n"))
    }

    /// Removes headings whose section has no content (and no deeper sub-heading with content), with their empty lines.
    /// Also drops HTML comments and collapses blank-line runs. Used for "Copy week as markdown".
    static func pruneEmptySections(_ body: String) -> String {
        var lines = scan(body)
        var changed = true
        while changed {
            changed = false
            var keep = [Bool](repeating: true, count: lines.count)
            var i = 0
            while i < lines.count {
                guard case .heading(let level, _) = lines[i].kind else { i += 1; continue }
                var j = i + 1, hasText = false
                while j < lines.count {
                    if case .heading = lines[j].kind { break }
                    if lineHasContent(lines[j]) { hasText = true }
                    j += 1
                }
                var deeperNext = false
                if j < lines.count, case .heading(let nl, _) = lines[j].kind { deeperNext = nl > level }
                if !hasText && !deeperNext { for k in i..<j { keep[k] = false }; changed = true }
                i = j
            }
            if changed { lines = zip(lines, keep).filter { $0.1 }.map { $0.0 } }
        }
        return rebuild(lines)
    }

    /// Re-levels headings so the shallowest one becomes `minLevel` (max 6). Used to nest a body under a "## Mon 5 Oct" heading.
    static func shiftHeadings(in body: String, minLevel: Int) -> String {
        let lines = scan(body)
        var lowest = 7
        for l in lines { if case .heading(let lv, _) = l.kind { lowest = min(lowest, lv) } }
        let offset = lowest == 7 ? 0 : max(0, minLevel - lowest)
        let shifted = lines.map { l -> ScannedLine in
            guard case .heading(let lv, let t) = l.kind else { return l }
            return ScannedLine(kind: .heading(level: min(6, lv + offset), title: t), text: l.text)
        }
        return rebuild(shifted)
    }
}
