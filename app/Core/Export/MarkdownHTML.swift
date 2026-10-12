// MarkdownHTML.swift - the small markdown this app produces, turned into HTML (rich-text copy, PDF) and into plain text. Pure, Foundation only.
//
// API:
//   MarkdownHTML.body(md, images:) -> String          HTML fragment
//   MarkdownHTML.fragment(md) -> String               a whole HTML page for the pasteboard (inline font only: Mail and Notes ignore style sheets)
//   MarkdownHTML.document(title:, md:, images:)       a whole HTML page for printing: strict CSP (no network, no script), print style sheet
//   MarkdownHTML.plain(md) -> String                  no "#", "**" or backticks; headings become "Title:" lines, lists keep "- ", links "text (url)"
//   MarkdownHTML.nodes(md) -> [MDNode]                the block parse (headings, paragraphs, flat list items with a level, quotes, code, rules, tables)
//   MDSpan.parse(line) -> [MDSpan]                    the inline parse (bold, italic, strike, code, links, images; no regular expressions)
//   MarkdownHTML.imageTargets(in: md) -> [String]     every image path in the text outside code, each once, in order
// Rules: every text node is HTML-escaped (raw HTML in a page is shown as text); a link keeps its target only for http, https and mailto;
// an image is its alt text ("[Image: alt]") unless `images` answers a data: URI for its path (the PDF does that, nothing else is loaded);
// input is cut at 2 MB, a line longer than 5,000 characters is shown as written, nesting stops at 8 levels. Task boxes print as ☐ and ☑.
import Foundation

enum MDSpan: Equatable {
    case text(String), code(String)
    case bold([MDSpan]), italic([MDSpan]), strike([MDSpan])
    case link(text: [MDSpan], url: String)
    case image(alt: String, url: String)
}

struct MDItem: Equatable {
    var level: Int              // 0 = top level, nested by indentation
    var ordered: Bool
    var number: String
    var checked: Bool?          // nil = not a task
    var lines: [String]         // the item's text, then its continuation lines
}

enum MDNode: Equatable {
    case heading(level: Int, text: String)
    case paragraph([String])
    case item(MDItem)           // consecutive items form a list
    case quote([String])
    case code(String)
    case rule
    case table(header: [String], rows: [[String]])
}

// MARK: inline
extension MDSpan {
    static let maxLineChars = 5_000
    static let maxDepth = 8
    private static let escapable = Set("\\`*_{}[]()#+-.!~|<>")

    static func parse(_ s: String) -> [MDSpan] {
        guard s.count <= maxLineChars else { return [.text(s)] }
        return parse(Array(s), 0)
    }

    private static func runLength(_ c: [Character], _ i: Int) -> Int {
        var n = 0
        while i + n < c.count, c[i + n] == c[i] { n += 1 }
        return n
    }

    /// The start of the next run of exactly `length` copies of `ch` at or after `from`.
    private static func closingRun(_ c: [Character], _ ch: Character, _ length: Int, from: Int) -> Int? {
        var j = from
        while j < c.count {
            if c[j] == ch { let r = runLength(c, j); if r == length { return j }; j += r } else { j += 1 }
        }
        return nil
    }

    /// "[label](dest)" starting at c[i] == "[": the label, the destination without an optional "title", and the index after ")".
    private static func link(_ c: [Character], from i: Int) -> (label: String, url: String, next: Int)? {
        let n = c.count
        var j = i + 1, depth = 1
        while j < n {
            if c[j] == "\\" { j += 2; continue }
            if c[j] == "[" { depth += 1 } else if c[j] == "]" { depth -= 1; if depth == 0 { break } }
            j += 1
        }
        guard j + 1 < n, c[j] == "]", c[j + 1] == "(" else { return nil }
        var k = j + 2, paren = 1
        while k < n {
            if c[k] == "\\" { k += 2; continue }
            if c[k] == "(" { paren += 1 } else if c[k] == ")" { paren -= 1; if paren == 0 { break } }
            k += 1
        }
        guard k < n, c[k] == ")" else { return nil }
        var dest = String(c[(j + 2)..<k]).trimmingCharacters(in: .whitespaces)
        if dest.hasPrefix("<"), let e = dest.firstIndex(of: ">") { dest = String(dest[dest.index(after: dest.startIndex)..<e]) }
        else if let sp = dest.firstIndex(where: { $0 == " " || $0 == "\t" }) { dest = String(dest[..<sp]) }
        return (String(c[(i + 1)..<j]), dest, k + 1)
    }

    /// The closing delimiter of an emphasis run: exactly `len` copies of `marker`, not after a space, skipping code spans and escapes.
    private static func closing(_ c: [Character], marker: Character, len: Int, from: Int) -> Int? {
        var j = from
        while j < c.count {
            if c[j] == "\\" { j += 2; continue }
            if c[j] == "`" {
                let r = runLength(c, j)
                if let e = closingRun(c, "`", r, from: j + r) { j = e + r } else { j += r }
                continue
            }
            if c[j] == marker {
                let r = runLength(c, j)
                if r == len, j > from, !c[j - 1].isWhitespace {
                    let after: Character = j + r < c.count ? c[j + r] : " "
                    if marker != "_" || !(after.isLetter || after.isNumber) { return j }
                }
                j += r
                continue
            }
            j += 1
        }
        return nil
    }

    private static func parse(_ c: [Character], _ depth: Int) -> [MDSpan] {
        var out = [MDSpan](), buf = ""
        func flush() { if !buf.isEmpty { out.append(.text(buf)); buf = "" } }
        let n = c.count
        var i = 0
        while i < n {
            let ch = c[i]
            if ch == "\\", i + 1 < n, escapable.contains(c[i + 1]) { buf.append(c[i + 1]); i += 2; continue }
            if ch == "`" {
                let r = runLength(c, i)
                if let e = closingRun(c, "`", r, from: i + r) {
                    flush(); out.append(.code(String(c[(i + r)..<e]).trimmingCharacters(in: .whitespaces))); i = e + r; continue
                }
                buf += String(repeating: "`", count: r); i += r; continue
            }
            if ch == "!", i + 1 < n, c[i + 1] == "[", let l = link(c, from: i + 1) {
                flush(); out.append(.image(alt: l.label, url: l.url)); i = l.next; continue
            }
            if ch == "[", let l = link(c, from: i) {
                flush(); out.append(.link(text: depth < maxDepth ? parse(Array(l.label), depth + 1) : [.text(l.label)], url: l.url)); i = l.next; continue
            }
            if depth < maxDepth, ch == "*" || ch == "_" || ch == "~" {
                let r = runLength(c, i)
                let before: Character = i > 0 ? c[i - 1] : " "
                let len = ch == "~" ? (r == 2 ? 2 : 0) : (r <= 3 ? r : 0)
                let opens = len > 0 && i + len < n && !c[i + len].isWhitespace && (ch != "_" || !(before.isLetter || before.isNumber))
                if opens, let j = closing(c, marker: ch, len: len, from: i + len) {
                    let inner = parse(Array(c[(i + len)..<j]), depth + 1)
                    flush()
                    if ch == "~" { out.append(.strike(inner)) }
                    else if len == 1 { out.append(.italic(inner)) }
                    else if len == 2 { out.append(.bold(inner)) }
                    else { out.append(.bold([.italic(inner)])) }
                    i = j + len; continue
                }
                buf += String(repeating: ch, count: r); i += r; continue
            }
            buf.append(ch); i += 1
        }
        flush()
        return out
    }

    // MARK: renderers
    static func escape(_ s: String) -> String {
        var o = ""
        o.reserveCapacity(s.utf8.count)
        for ch in s {
            switch ch {
            case "&": o += "&amp;"
            case "<": o += "&lt;"
            case ">": o += "&gt;"
            case "\"": o += "&quot;"
            case "'": o += "&#39;"
            default: o.append(ch)
            }
        }
        return o
    }

    /// The target when it is http, https or mailto; nil for anything else (javascript:, file:, relative paths).
    static func safeURL(_ url: String) -> String? {
        let u = url.trimmingCharacters(in: .whitespacesAndNewlines), l = u.lowercased()
        return l.hasPrefix("http://") || l.hasPrefix("https://") || l.hasPrefix("mailto:") ? u : nil
    }

    static func imageText(_ alt: String) -> String { alt.dlTrimmed.isEmpty ? "[Image]" : "[Image: \(alt.dlTrimmed)]" }

    static func html(_ spans: [MDSpan], images: ((String) -> String?)?) -> String {
        spans.map { s -> String in
            switch s {
            case .text(let t): return escape(t)
            case .code(let t): return "<code>\(escape(t))</code>"
            case .bold(let x): return "<strong>\(html(x, images: images))</strong>"
            case .italic(let x): return "<em>\(html(x, images: images))</em>"
            case .strike(let x): return "<s>\(html(x, images: images))</s>"
            case .link(let x, let url):
                guard let u = safeURL(url) else { return html(x, images: images) }
                return "<a href=\"\(escape(u))\">\(html(x, images: images))</a>"
            case .image(let alt, let url):
                if let data = images?(url) { return "<img src=\"\(escape(data))\" alt=\"\(escape(alt))\">" }
                return "<span class=\"img\">\(escape(imageText(alt)))</span>"
            }
        }.joined()
    }

    static func plain(_ spans: [MDSpan]) -> String {
        spans.map { s -> String in
            switch s {
            case .text(let t), .code(let t): return t
            case .bold(let x), .italic(let x), .strike(let x): return plain(x)
            case .link(let x, let url):
                let t = plain(x)
                guard let u = safeURL(url) else { return t }
                return t.isEmpty || t == u ? u : "\(t) (\(u))"
            case .image(let alt, _): return imageText(alt)
            }
        }.joined()
    }

    /// Every image destination inside the spans.
    static func imageURLs(_ spans: [MDSpan]) -> [String] {
        spans.flatMap { s -> [String] in
            switch s {
            case .image(_, let url): return [url]
            case .bold(let x), .italic(let x), .strike(let x), .link(let x, _): return imageURLs(x)
            case .text, .code: return []
            }
        }
    }
}

// MARK: blocks
enum MarkdownHTML {
    static let maxInput = 2_000_000
    static let maxListLevel = 7

    private static func isRule(_ s: String) -> Bool {
        let b = s.filter { $0 != " " }
        guard b.count >= 3, let f = b.first, "-*_".contains(f) else { return false }
        return b.allSatisfy { $0 == f }
    }
    private static func isDelimiterRow(_ s: String) -> Bool {
        let t = s.trimmingCharacters(in: .whitespaces)
        return t.contains("-") && t.contains("|") && t.allSatisfy { "|-: ".contains($0) }
    }
    private static func cells(_ line: String) -> [String] {
        var c = Array(line.trimmingCharacters(in: .whitespaces))
        if c.first == "|" { c.removeFirst() }
        if c.last == "|", c.count < 2 || c[c.count - 2] != "\\" { c.removeLast() }
        var out = [String](), cur = "", i = 0
        while i < c.count {
            if c[i] == "\\", i + 1 < c.count, c[i + 1] == "|" { cur.append("|"); i += 2; continue }
            if c[i] == "|" { out.append(cur.trimmingCharacters(in: .whitespaces)); cur = ""; i += 1; continue }
            cur.append(c[i]); i += 1
        }
        out.append(cur.trimmingCharacters(in: .whitespaces))
        return out
    }

    static func nodes(_ md: String) -> [MDNode] {
        let lines = MarkdownBody.scan(md.count > maxInput ? String(md.prefix(maxInput)) : md)
        var out = [MDNode](), para = [String](), quote = [String](), code = [String](), inCode = false
        var indents = [Int]()                                       // indentation of the open list levels
        func flush() {
            if !para.isEmpty { out.append(.paragraph(para)); para = [] }
            if !quote.isEmpty { out.append(.quote(quote)); quote = [] }
        }
        var i = 0
        while i < lines.count {
            let l = lines[i]
            i += 1
            switch l.kind {
            case .fence:
                flush(); indents = []
                if inCode { out.append(.code(code.joined(separator: "\n"))); code = []; inCode = false } else { inCode = true }
            case .code: code.append(l.text)
            case .blank: flush()
            case .heading(let level, let title): flush(); indents = []; out.append(.heading(level: level, text: title))
            case .text:
                let t = l.text, trimmed = t.trimmingCharacters(in: .whitespaces)
                if trimmed.contains("|"), i < lines.count, lines[i].kind == .text, isDelimiterRow(lines[i].text) {
                    flush(); indents = []
                    let header = cells(trimmed)
                    i += 1
                    var rows = [[String]]()
                    while i < lines.count, lines[i].kind == .text, lines[i].text.contains("|") { rows.append(cells(lines[i].text)); i += 1 }
                    out.append(.table(header: header, rows: rows))
                } else if isRule(trimmed) {
                    flush(); indents = []; out.append(.rule)
                } else if trimmed.hasPrefix(">") {
                    if !para.isEmpty { flush() }
                    indents = []
                    quote.append(String(trimmed.drop(while: { $0 == ">" || $0 == " " })))
                } else if let indent = MarkdownBody.listItemIndent(t) {
                    flush()
                    while let top = indents.last, indent < top { indents.removeLast() }
                    if indents.last.map({ indent > $0 }) ?? true { indents.append(indent) }
                    let m = MarkdownBody.stripMarkers(t)
                    let digits = trimmed.prefix(while: { $0.isASCII && $0.isNumber })
                    out.append(.item(MDItem(level: min(indents.count - 1, maxListLevel), ordered: !digits.isEmpty, number: String(digits),
                                            checked: m.checked, lines: [m.text])))
                } else if case .item(var it)? = out.last, para.isEmpty, quote.isEmpty, t.prefix(while: { $0 == " " }).count >= 2 {
                    it.lines.append(trimmed); out[out.count - 1] = .item(it)     // a wrapped line of the item above
                } else {
                    if !quote.isEmpty { flush() }
                    indents = []
                    para.append(trimmed)
                }
            }
        }
        if inCode { out.append(.code(code.joined(separator: "\n"))) }
        flush()
        return out
    }

    /// Every image path the text uses (outside code), each once, in order of appearance.
    static func imageTargets(in md: String) -> [String] {
        var seen = Set<String>(), out = [String]()
        for l in MarkdownBody.scan(md) {
            let line: String
            switch l.kind {
            case .text: line = l.text
            case .heading(_, let title): line = title
            default: continue
            }
            for u in MDSpan.imageURLs(MDSpan.parse(line)) where seen.insert(u).inserted { out.append(u) }
        }
        return out
    }

    // MARK: HTML
    static func body(_ md: String, images: ((String) -> String?)? = nil) -> String {
        var h = ""
        var open = [String]()                                       // the open lists, outermost first: "ul" or "ol"
        func inl(_ s: String) -> String { MDSpan.html(MDSpan.parse(s), images: images) }
        func close(to n: Int) { while open.count > n { h += "</li></\(open.removeLast())>" } }
        for node in nodes(md) {
            if case .item(let it) = node {
                let tag = it.ordered ? "ol" : "ul", depth = it.level + 1
                let attrs = it.checked == nil ? "" : " class=\"task\""
                if open.count < depth {
                    while open.count < depth { open.append(tag); h += "<\(tag)><li\(open.count == depth ? attrs : "")>" }
                } else {
                    close(to: depth)
                    if open[open.count - 1] != tag { h += "</li></\(open[open.count - 1])><\(tag)><li\(attrs)>"; open[open.count - 1] = tag }
                    else { h += "</li><li\(attrs)>" }
                }
                if let c = it.checked { h += c ? "☑ " : "☐ " }
                h += it.lines.map(inl).joined(separator: "<br>")
                continue
            }
            close(to: 0)
            switch node {
            case .heading(let level, let text): h += "<h\(level)>\(inl(text))</h\(level)>\n"
            case .paragraph(let ls): h += "<p>\(ls.map(inl).joined(separator: "<br>\n"))</p>\n"
            case .quote(let ls): h += "<blockquote><p>\(ls.map(inl).joined(separator: "<br>\n"))</p></blockquote>\n"
            case .code(let t): h += "<pre><code>\(MDSpan.escape(t))</code></pre>\n"
            case .rule: h += "<hr>\n"
            case .table(let header, let rows):
                h += "<table><thead><tr>" + header.map { "<th>\(inl($0))</th>" }.joined() + "</tr></thead><tbody>"
                for r in rows { h += "<tr>" + r.map { "<td>\(inl($0))</td>" }.joined() + "</tr>" }
                h += "</tbody></table>\n"
            case .item: break
            }
        }
        close(to: 0)
        return h
    }

    static let css = """
    body { font: 11pt/1.45 -apple-system, "Helvetica Neue", Helvetica, Arial, sans-serif; color: #1b201e; margin: 0; }
    h1 { font-size: 20pt; margin: 0 0 6pt; } h2 { font-size: 14pt; margin: 18pt 0 4pt; } h3 { font-size: 12pt; margin: 12pt 0 3pt; }
    h4, h5, h6 { font-size: 11pt; margin: 10pt 0 2pt; }
    h1, h2, h3, h4, h5, h6 { break-after: avoid; break-inside: avoid; }
    p { margin: 0 0 6pt; } ul, ol { margin: 0 0 6pt; padding-left: 18pt; } li { margin: 2pt 0; break-inside: avoid; }
    li.task { list-style: none; margin-left: -14pt; }
    blockquote { margin: 6pt 0; padding-left: 10pt; border-left: 2pt solid #bcc5bf; color: #566059; }
    pre { background: #f0f2f0; padding: 6pt; font: 9.5pt/1.4 Menlo, monospace; white-space: pre-wrap; break-inside: avoid; }
    code { font-family: Menlo, monospace; font-size: 0.92em; background: #f0f2f0; padding: 0 2pt; }
    table { border-collapse: collapse; margin: 6pt 0; } th, td { border: 0.5pt solid #bcc5bf; padding: 3pt 6pt; text-align: left; }
    img { max-width: 100%; max-height: 280pt; vertical-align: top; break-inside: avoid; } .img { color: #566059; } hr { border: 0; border-top: 0.5pt solid #bcc5bf; }
    """

    /// A whole page for printing: no script, no network (the CSP allows only inline styles and data: images).
    static func document(title: String, md: String, images: ((String) -> String?)? = nil) -> String {
        """
        <!DOCTYPE html>
        <html lang="en"><head><meta charset="utf-8">
        <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'">
        <title>\(MDSpan.escape(title))</title>
        <style>\(css)</style></head>
        <body>\(body(md, images: images))</body></html>
        """
    }

    /// A whole page for the pasteboard: structure plus one inline font, which is what Mail, Notes and Pages keep.
    static func fragment(_ md: String) -> String {
        "<html><head><meta charset=\"utf-8\"></head><body style=\"font-family: -apple-system, Helvetica, Arial, sans-serif; font-size: 13px;\">"
            + body(md) + "</body></html>"
    }

    // MARK: plain text
    static func plain(_ md: String) -> String {
        var out = [String]()
        func gap() { if let l = out.last, !l.isEmpty { out.append("") } }
        func p(_ s: String) -> String { MDSpan.plain(MDSpan.parse(s)) }
        var inList = false
        for node in nodes(md) {
            if case .item(let it) = node {
                if !inList { gap() }
                inList = true
                let pad = String(repeating: "  ", count: it.level)
                let box = it.checked.map { $0 ? "[x] " : "[ ] " } ?? ""
                out.append(pad + (it.ordered ? "\(it.number)." : "-") + " " + box + p(it.lines.first ?? ""))
                for l in it.lines.dropFirst() { out.append(pad + "  " + p(l)) }
                continue
            }
            inList = false
            gap()
            switch node {
            case .heading(let level, let text): out.append(level == 1 ? p(text) : p(text) + ":")
            case .paragraph(let ls), .quote(let ls): out += ls.map(p)
            case .code(let t): out += t.components(separatedBy: "\n")
            case .rule: out.append("----")
            case .table(let header, let rows): out.append(header.map(p).joined(separator: " | ")); out += rows.map { $0.map(p).joined(separator: " | ") }
            case .item: break
            }
        }
        while let l = out.last, l.isEmpty { out.removeLast() }
        return out.joined(separator: "\n") + "\n"
    }
}
