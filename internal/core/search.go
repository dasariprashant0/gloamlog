// search.go - search over saved day pages. Port of app/Core/Search.swift.
//
// API: LogStore.Search(query, heading) []SearchHit   (newest day first, then document order; cached, see searchindex.go)
//
//	SearchDocument(doc, query, heading)           pure per-day matcher = searchSections(doc) + searchHits(day, sections, query, heading)
//
// A page is cut at its headings; every heading-delimited section that matches yields ONE hit. Matching is case- and
// diacritic-insensitive (searchfold.go), whitespace-separated words are AND-ed within a section (the heading text counts too, so a
// heading-only match is found; text before the first heading has Heading == nil). `heading` restricts to sections whose
// normalised title equals the normalised given title (e.g. "To do next" also finds "📌 To do next").
// SearchHit.Heading is the RAW title of the nearest preceding heading. Snippet = ~120 chars of plain text around the first
// word, with "…" at cut edges; MatchStart/MatchEnd are BYTE offsets into Snippet (Swift had a String.Index range; in Go
// Snippet[MatchStart:MatchEnd] is the match, in its original case; MatchUTF16 gives the offsets a JavaScript string wants).
// Skipped days and drafts are not searched.
//
// Go vs Swift: the snippet window is 40 characters before and 80 after the match. Swift counted grapheme clusters; Go has no
// grapheme segmentation in the standard library, so clusterStarts below approximates it (combining marks, zero-width joiners,
// skin-tone modifiers, regional-indicator pairs, CRLF) and never cuts inside one of those.
package core

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// SearchHit is one matching section of one day.
type SearchHit struct {
	Day        string  `json:"day"`
	Heading    *string `json:"heading"` // nearest preceding heading (raw text), nil before the first heading
	Snippet    string  `json:"snippet"`
	MatchStart int     `json:"matchStart"` // byte offsets into Snippet
	MatchEnd   int     `json:"matchEnd"`
}

// MatchText is the matched text of the snippet.
func (h SearchHit) MatchText() string { return h.Snippet[h.MatchStart:h.MatchEnd] }

// MatchUTF16 is the match as UTF-16 code unit offsets into Snippet, the indexing JavaScript strings use.
func (h SearchHit) MatchUTF16() (start, end int) {
	return len(utf16.Encode([]rune(h.Snippet[:h.MatchStart]))), len(utf16.Encode([]rune(h.Snippet[:h.MatchEnd])))
}

// SearchSection is a page as search sees it: one record per heading-delimited section with its two haystacks ready (what the index
// caches): the plain text, and for a titled section the heading joined to it. The folded copies are what is actually searched.
type SearchSection struct {
	Level           int
	Title           string
	NormalizedTitle string
	Text            string
	Titled          string // heading + text; only when HasTitled
	HasTitled       bool

	foldedText   string
	foldedTitled string
}

func searchWords(q string) []string { return strings.FieldsFunc(q, unicode.IsSpace) }

// searchSections cuts a page into its searchable sections. A skipped day has none.
func searchSections(doc DayDocument) []SearchSection {
	if doc.IsSkipped {
		return nil
	}
	var out []SearchSection
	for _, s := range BodySections(doc.Body) {
		text := PlainText(s.Text)
		sec := SearchSection{Level: s.Level, Title: s.Title, NormalizedTitle: s.NormalizedTitle, Text: text, foldedText: foldString(text)}
		if s.Level > 0 && s.Title != "" {
			sec.HasTitled = true
			sec.Titled = Collapse(s.Title + " " + text)
			sec.foldedTitled = foldString(sec.Titled)
		}
		out = append(out, sec)
	}
	return out
}

// SearchDocument finds the hits of one day. heading nil = every section.
func SearchDocument(doc DayDocument, query string, heading *string) []SearchHit {
	return searchHits(doc.Day, searchSections(doc), query, heading)
}

func searchHits(day string, sections []SearchSection, query string, heading *string) []SearchHit {
	words := searchWords(query)
	if len(words) == 0 {
		return nil
	}
	folded := make([]string, len(words))
	for i, w := range words {
		folded[i] = foldString(w)
	}
	wanted := ""
	if heading != nil {
		wanted = NormalizeHeading(*heading)
	}
	var out []SearchHit
	for _, s := range sections {
		if heading != nil && s.NormalizedTitle != wanted {
			continue
		}
		hays := []struct{ orig, folded string }{{s.Text, s.foldedText}}
		if s.HasTitled {
			hays = append(hays, struct{ orig, folded string }{s.Titled, s.foldedTitled})
		}
		for _, h := range hays {
			idx, n, ok := firstMatch(folded, h.folded)
			if !ok {
				continue
			}
			lo, hi := foldText(h.orig).originalRange(idx, n)
			snip, ms, me := snippet(h.orig, lo, hi)
			hit := SearchHit{Day: day, Snippet: snip, MatchStart: ms, MatchEnd: me}
			if s.Level > 0 {
				title := s.Title
				hit.Heading = &title
			}
			out = append(out, hit)
			break
		}
	}
	return out
}

// firstMatch finds the earliest-starting word when ALL words occur in the (folded) haystack: its folded byte offset and length.
func firstMatch(words []string, hay string) (idx, n int, ok bool) {
	best := -1
	for _, w := range words {
		i := strings.Index(hay, w)
		if w == "" || i < 0 {
			return 0, 0, false
		}
		if best < 0 || i < best {
			best, n = i, len(w)
		}
	}
	return best, n, best >= 0
}

// snippet cuts a window of 40 characters before and 80 after the match [lo, hi) out of s, with "…" where it was cut. It returns the
// snippet and the match as byte offsets into it.
func snippet(s string, lo, hi int) (string, int, int) {
	starts := clusterStarts(s)
	loC := sort.SearchInts(starts, lo+1) - 1 // the character that contains lo
	hiC := sort.SearchInts(starts, hi)       // the first character boundary at or after hi
	startC := max(0, loC-40)
	endC := min(len(starts)-1, hiC+80)
	a, b := starts[startC], starts[endC]
	pre, post := "", ""
	if a > 0 {
		pre = "…"
	}
	if b < len(s) {
		post = "…"
	}
	return pre + s[a:b] + post, len(pre) + lo - a, len(pre) + hi - a
}

// clusterStarts returns the byte offset where each (approximate) grapheme cluster of s starts, followed by len(s).
func clusterStarts(s string) []int {
	starts := make([]int, 0, len(s)+1)
	for i := 0; i < len(s); {
		starts = append(starts, i)
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r == '\r' && i < len(s) && s[i] == '\n' {
			i++
			continue
		}
		if r >= 0x1F1E6 && r <= 0x1F1FF && i < len(s) { // a flag is two regional indicators
			if r2, sz := utf8.DecodeRuneInString(s[i:]); r2 >= 0x1F1E6 && r2 <= 0x1F1FF {
				i += sz
			}
		}
		for i < len(s) {
			r2, sz := utf8.DecodeRuneInString(s[i:])
			if !isClusterExtender(r2) {
				break
			}
			i += sz
			if r2 == 0x200D && i < len(s) { // zero-width joiner: the next character belongs to the same cluster
				_, sz2 := utf8.DecodeRuneInString(s[i:])
				i += sz2
			}
		}
	}
	return append(starts, len(s))
}

func isClusterExtender(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc) || r == 0x200D || (r >= 0x1F3FB && r <= 0x1F3FF) || (r >= 0xE0020 && r <= 0xE007F)
}
