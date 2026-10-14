// Search.swift - search over saved day pages.
//
// API: LogStore.search(query, heading:) -> [SearchHit]   (newest day first, then document order; cached, see SearchIndex.swift)
//      Search.hits(in: doc, query:, heading:)             pure per-day matcher = sections(of: doc) + hits(day:sections:query:heading:)
// A page is cut at its headings; every heading-delimited section that matches yields ONE hit. Matching is case- and
// diacritic-insensitive, whitespace-separated words are AND-ed within a section (the heading text counts too, so a
// heading-only match is found; text before the first heading has heading == nil). `heading:` restricts to sections whose
// normalised title equals the normalised given title (e.g. "To do next" also finds "📌 To do next").
// SearchHit.heading is the RAW title of the nearest preceding heading. snippet = ~120 chars of plain text around the first
// word, with "…" at cut edges; matchRange indexes into snippet. Skipped days and drafts are not searched.
import Foundation

struct SearchHit: Equatable {
    var day: String
    var heading: String?         // nearest preceding heading (raw text), nil before the first heading
    var snippet: String
    var matchRange: Range<String.Index>?
}

enum Search {
    private static let opts: String.CompareOptions = [.caseInsensitive, .diacriticInsensitive]

    /// A page as search sees it: one record per heading-delimited section with its two haystacks ready (what SearchIndex caches):
    /// the plain text, and for a titled section the heading joined to it. Both are NSString-backed on purpose: range(of:options:) is the
    /// same NSString call either way, but on a native Swift string it bridges per call, 15x slower on non-ASCII text (emoji headings).
    struct Section: Equatable { var level: Int; var title: String; var normalizedTitle: String; var text: String; var titled: String? }

    static func words(_ q: String) -> [String] { q.split(whereSeparator: { $0.isWhitespace }).map(String.init) }

    static func sections(of doc: DayDocument) -> [Section] {
        doc.isSkipped ? [] : MarkdownBody.sections(of: doc.body).map {
            let text = MarkdownBody.plainText($0.text)
            let titled = $0.level > 0 && !$0.title.isEmpty ? NSString(string: MarkdownBody.collapse($0.title + " " + text)) as String : nil
            return Section(level: $0.level, title: $0.title, normalizedTitle: $0.normalizedTitle, text: NSString(string: text) as String, titled: titled)
        }
    }

    static func hits(in doc: DayDocument, query: String, heading: String? = nil) -> [SearchHit] {
        hits(day: doc.day, sections: sections(of: doc), query: query, heading: heading)
    }

    static func hits(day: String, sections: [Section], query: String, heading: String? = nil) -> [SearchHit] {
        let ws = words(query)
        guard !ws.isEmpty else { return [] }
        let wanted = heading.map { MarkdownBody.normalizeHeading($0) }
        var out = [SearchHit]()
        for s in sections {
            if let w = wanted, s.normalizedTitle != w { continue }
            for h in [s.text, s.titled].compactMap({ $0 }) {
                guard let r = firstMatch(ws, in: h) else { continue }
                let (snip, range) = snippet(h, r)
                out.append(SearchHit(day: day, heading: s.level > 0 ? s.title : nil, snippet: snip, matchRange: range))
                break
            }
        }
        return out
    }

    /// Range of the earliest-starting word when ALL words occur; nil otherwise.
    private static func firstMatch(_ ws: [String], in s: String) -> Range<String.Index>? {
        var first: Range<String.Index>?
        for w in ws {
            guard let r = s.range(of: w, options: opts) else { return nil }
            if first == nil || r.lowerBound < first!.lowerBound { first = r }
        }
        return first
    }

    private static func snippet(_ s: String, _ r: Range<String.Index>) -> (String, Range<String.Index>?) {
        let start = s.index(r.lowerBound, offsetBy: -40, limitedBy: s.startIndex) ?? s.startIndex
        let end = s.index(r.upperBound, offsetBy: 80, limitedBy: s.endIndex) ?? s.endIndex
        let pre = start > s.startIndex ? "…" : "", post = end < s.endIndex ? "…" : ""
        let out = pre + String(s[start..<end]) + post
        // UTF-16 offsets survive concatenation even when "…" merges with a following combining mark.
        let lo = pre.utf16.count + s.utf16.distance(from: start, to: r.lowerBound)
        let hi = pre.utf16.count + s.utf16.distance(from: start, to: r.upperBound)
        return (out, String.Index(utf16Offset: lo, in: out)..<String.Index(utf16Offset: hi, in: out))
    }
}
