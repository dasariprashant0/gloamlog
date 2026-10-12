// CapturePlacement.swift - where and how a note appears in a page. Pure. Owned by the core stream (task K1).
// CONTRACT (signatures frozen; K1 replaces the stub bodies):
//   render   "- 14:32 text", or "- [ ] text" for a to-do; continuation lines indented 2 spaces; a leading # > - + * or "1." in the text is
//            backslash-escaped so it can never become a heading or nested list.
//   apply    puts the rendered line at the END of the page's Jots block (a heading whose normalised text equals jotsHeading, any level, running
//            to the next heading of the same or higher level), joined to the list without a blank line when the last content line is a list
//            item, else after one blank line; with no Jots block: the trimmed page + blank line + "## Jots" + blank line + the line.
//   contains true when the rendered line (ignoring list-marker style and escape differences) is already in the Jots block (replay safety).
// Details (all tested in tests/main.swift):
//   * render: every line of the note is trimmed, blank lines inside a note are dropped, then the START of each line is escaped when it would
//     rewrite the page: an ATX heading (#, up to six, then a space or the end), ">", a bullet (- + * then a space or the end), an ordered item
//     ("1." / "1)" gets the backslash before the dot), a setext underline or thematic break (---, ===, ***, - - -) and a code fence
//     (``` or ~~~). "<!--" anywhere becomes "<\!--": an open comment would hide the rest of the page from the scanners. Text that only
//     looks like a marker (#tag, -5, *bold*, 1.5x) is left alone. A to-do never carries a time stamp.
//   * apply: the first Jots block is the target (the same rule as the editor's appendToSection). The result is trimBody'd (no trailing
//     newline). Lines after the insertion keep their spacing, except that a line directly after the new note (a heading, or text hidden by an
//     open comment) gets one blank line, so it can never become a continuation of the note.
//   * contains: the note's identity is `key`: list marker, task box, backslash escapes and line breaks (soft break, hard break "\") are flattened
//     and whitespace collapsed, so the editor re-serialising the line ("* " for "- ", "\*" for "*", a ticked box) never defeats it. It looks
//     in every Jots block, at top-level list items only (a multi-line note is one item), never in code.
import Foundation

enum CapturePlacement {
    // MARK: render
    static func render(_ item: CaptureItem) -> String {
        let lines = item.text.replacingOccurrences(of: "\r\n", with: "\n").replacingOccurrences(of: "\r", with: "\n")
            .components(separatedBy: "\n").map { $0.dlTrimmed }.filter { !$0.isEmpty }.map(escapeLine)
        var head = "- "
        if item.todo { head += "[ ] " } else if let s = item.stamp, !s.isEmpty { head += s + " " }
        let first = lines.first ?? ""
        let firstLine = first.isEmpty ? String(head.dropLast()) : head + first
        return ([firstLine] + lines.dropFirst().map { "  " + $0 }).joined(separator: "\n")
    }

    /// Backslash-escapes what would turn this line (the first line of the note or a continuation line) into page structure.
    private static func escapeLine(_ line: String) -> String {
        let s = line.replacingOccurrences(of: "<!--", with: "<\\!--")
        guard let f = s.first else { return s }
        let bare = s.filter { $0 != " " && $0 != "\t" }
        if bare.allSatisfy({ $0 == "=" }) || bare.allSatisfy({ $0 == "-" }) { return "\\" + s }                 // setext underline, "-", "---"
        if bare.count >= 3, let b = bare.first, "*_-".contains(b), bare.allSatisfy({ $0 == b }) { return "\\" + s }   // "***", "- - -"
        if s.hasPrefix("```") || s.hasPrefix("~~~") { return "\\" + s }
        func marker(_ after: Substring) -> Bool { after.first.map { $0 == " " || $0 == "\t" } ?? true }
        if f == "#" {
            let hashes = s.prefix(while: { $0 == "#" })
            if hashes.count <= 6, marker(s.dropFirst(hashes.count)) { return "\\" + s }
        } else if f == ">" {
            return "\\" + s
        } else if "-+*".contains(f), marker(s.dropFirst()) {
            return "\\" + s
        } else {
            let digits = s.prefix(while: { ("0"..."9").contains($0) })
            if !digits.isEmpty, digits.count <= 9, let d = s.dropFirst(digits.count).first, d == "." || d == ")", marker(s.dropFirst(digits.count + 1)) {
                return String(digits) + "\\" + s.dropFirst(digits.count)
            }
        }
        return s
    }

    // MARK: apply
    static func apply(_ item: CaptureItem, to body: String, jotsHeading: String) -> String {
        let line = render(item)
        let text = body.replacingOccurrences(of: "\r\n", with: "\n")
        let lines = text.components(separatedBy: "\n")
        let scanned = MarkdownBody.scan(text)                        // splits exactly like `lines`, so the indexes match
        // A page that ends inside an open code fence or HTML comment would swallow the note (it would be saved but never seen as a note,
        // so a replay would add it again): in that case the fence / comment is closed first.
        var attempts: [[String]] = [[]]
        let fence = MarkdownBody.unclosedFenceCloser(scanned)
        if let f = fence { attempts.append([f]) }
        attempts.append(["-->"])
        if let f = fence { attempts.append([f, "-->"]) }
        var first: String?
        for closers in attempts {
            let result = place(line, closers: closers, text: text, lines: lines, scanned: scanned, jotsHeading: jotsHeading)
            if first == nil { first = result }
            if contains(item, in: result, jotsHeading: jotsHeading) { return result }
        }
        return first ?? line
    }

    private static func place(_ line: String, closers: [String], text: String, lines: [String], scanned: [MarkdownBody.ScannedLine],
                              jotsHeading: String) -> String {
        guard let block = MarkdownBody.jotsBlocks(in: scanned, jotsHeading: jotsHeading).first else {
            let page = MarkdownBody.trimBody(text)
            let head = ([page] + closers).filter { !$0.isEmpty }.joined(separator: "\n")
            return (head.isEmpty ? "" : head + "\n\n") + "## " + MarkdownBody.jotsHeadingText(jotsHeading) + "\n\n" + line
        }
        var insertAt: Int, blankFirst: Bool
        if let last = (block.heading + 1..<block.end).last(where: { scanned[$0].kind != .blank }) {
            insertAt = last + 1
            blankFirst = !closers.isEmpty || !endsInList(scanned, last, floor: block.heading)
        } else if block.heading + 1 < block.end {                    // an empty block: the blank line after the heading is the separator
            insertAt = block.heading + 2; blankFirst = false
        } else {
            insertAt = block.heading + 1; blankFirst = true
        }
        var out = Array(lines[..<insertAt])
        out += closers
        if blankFirst { out.append("") }
        out += line.components(separatedBy: "\n")
        if insertAt < lines.count, !lines[insertAt].dlTrimmed.isEmpty { out.append("") }    // never glue the next heading or line onto the note
        out += lines[insertAt...]
        return MarkdownBody.trimBody(out.joined(separator: "\n"))
    }

    /// True when the content line `last` belongs to a list: some line of the run of text lines ending there is a list item (the lines
    /// after an item without a blank line are its continuation). Code, headings and blank lines end the run.
    private static func endsInList(_ s: [MarkdownBody.ScannedLine], _ last: Int, floor: Int) -> Bool {
        var i = last
        while i > floor, s[i].kind == .text {
            if MarkdownBody.listItemIndent(s[i].text) != nil { return true }
            i -= 1
        }
        return false
    }

    // MARK: contains
    static func contains(_ item: CaptureItem, in body: String, jotsHeading: String) -> Bool {
        jotKeys(in: body, jotsHeading: jotsHeading).contains(key(of: render(item)))
    }

    /// The keys of every top-level list item in every Jots block of `body` (one scan; the writer checks many notes against one page).
    static func jotKeys(in body: String, jotsHeading: String) -> Set<String> {
        let scanned = MarkdownBody.scan(body)
        var keys = Set<String>()
        for b in MarkdownBody.jotsBlocks(in: scanned, jotsHeading: jotsHeading) {
            var item: [String]?
            func flush() { if let lines = item { keys.insert(key(of: lines.joined(separator: "\n"))) }; item = nil }
            for i in (b.heading + 1)..<b.end {
                let l = scanned[i]
                guard l.kind == .text else { flush(); continue }
                if let indent = MarkdownBody.listItemIndent(l.text), indent < 2 { flush(); item = [l.text] }
                else if item != nil { item?.append(l.text) }
            }
            flush()
        }
        return keys
    }

    /// A note's identity: first line without its list marker and task box, every line without backslash escapes, line breaks turned into
    /// spaces ("\" at the end of a line is a hard break), whitespace collapsed. Case and time are part of it.
    static func key(of markdown: String) -> String {
        var lines = markdown.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n")
        guard !lines.isEmpty else { return "" }
        lines[0] = MarkdownBody.stripMarkers(lines[0]).text
        for i in lines.indices.dropLast() {
            let trailing = lines[i].reversed().prefix(while: { $0 == "\\" }).count
            if trailing % 2 == 1 { lines[i].removeLast() }
        }
        return MarkdownBody.collapse(unescape(lines.joined(separator: " ")))
    }

    private static let escapable = Set("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")
    private static func unescape(_ s: String) -> String {
        var out = "", skipNext = false
        let chars = Array(s)
        for (i, c) in chars.enumerated() {
            if skipNext { skipNext = false; out.append(c); continue }
            if c == "\\", i + 1 < chars.count, escapable.contains(chars[i + 1]) { skipNext = true; continue }
            out.append(c)
        }
        return out
    }
}
