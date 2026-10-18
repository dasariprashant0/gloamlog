// markdownhtml.go - the small markdown this app produces, turned into HTML (rich-text copy, PDF) and into plain text. Pure.
// Port of app/Core/Export/MarkdownHTML.swift.
//
//	MDBody(md, images) string            HTML fragment
//	MDFragment(md) string                a whole HTML page for the pasteboard (inline font only: Mail and Notes ignore style sheets)
//	MDDocument(title, md, images)        a whole HTML page for printing: strict CSP (no network, no script), print style sheet
//	MDPlain(md) string                   no "#", "**" or backticks; headings become "Title:" lines, lists keep "- ", links "text (url)"
//	MDNodes(md) []MDNode                 the block parse (headings, paragraphs, flat list items with a level, quotes, code, rules, tables)
//	MDParseSpans(line) []MDSpan          the inline parse (bold, italic, strike, code, links, images; no regular expressions)
//	MDImageTargets(md) []string          every image path in the text outside code, each once, in order
//
// Rules: every text node is HTML-escaped (raw HTML in a page is shown as text); a link keeps its target only for http, https and
// mailto; an image is its alt text ("[Image: alt]") unless `images` answers a data: URI for its path (the PDF does that, nothing else
// is loaded); input is cut at 2 MB, a line longer than 5,000 characters is shown as written, nesting stops at 8 levels. Task boxes
// print as a ballot box and a checked ballot box.
//
// Go vs Swift: Swift walked the line as Characters (grapheme clusters); this walks runes. The two differ only when a marker character
// (* _ ~ ` [ ] ( ) \ !) is followed by a combining mark, and for the 5,000 / 2,000,000 limits, which count runes here.
package core

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MDSpanKind says what an MDSpan is.
type MDSpanKind int

const (
	MDText MDSpanKind = iota
	MDCode
	MDBold
	MDItalic
	MDStrike
	MDLink
	MDImage
)

// MDSpan is one inline element. Text is the text, the code or the image's alt text; Children are the content of bold, italic,
// strike and link spans; URL is the target of a link or an image.
type MDSpan struct {
	Kind     MDSpanKind
	Text     string
	Children []MDSpan
	URL      string
}

// MDItem is one list item: consecutive items form a list.
type MDItem struct {
	Level   int // 0 = top level, nested by indentation
	Ordered bool
	Number  string
	Task    TaskState // NoTaskBox unless the item has a task box (Swift: checked Bool?)
	Lines   []string  // the item's text, then its continuation lines
}

// MDNodeKind says what an MDNode is.
type MDNodeKind int

const (
	MDHeading MDNodeKind = iota
	MDParagraph
	MDListItem
	MDQuote
	MDCodeBlock
	MDRule
	MDTable
)

// MDNode is one block.
type MDNode struct {
	Kind   MDNodeKind
	Level  int      // heading level
	Text   string   // heading text, or the code of a code block
	Lines  []string // paragraph or quote lines
	Item   MDItem   // MDListItem
	Header []string // MDTable
	Rows   [][]string
}

const (
	mdMaxLineChars = 5_000
	mdMaxDepth     = 8
	mdMaxInput     = 2_000_000
	mdMaxListLevel = 7
)

const mdEscapable = "\\`*_{}[]()#+-.!~|<>"

// isHSpace is Foundation's CharacterSet.whitespaces: a tab, the Unicode space separators and U+200B (no line breaks).
func isHSpace(r rune) bool { return r == '\t' || r == 0x200B || unicode.Is(unicode.Zs, r) }

func trimHSpace(s string) string { return strings.TrimFunc(s, isHSpace) }

// ---- inline ----

// MDParseSpans is the inline parse of one line.
func MDParseSpans(s string) []MDSpan {
	if utf8.RuneCountInString(s) > mdMaxLineChars {
		return []MDSpan{{Kind: MDText, Text: s}}
	}
	return parseSpans([]rune(s), 0)
}

func runLength(c []rune, i int) int {
	n := 0
	for i+n < len(c) && c[i+n] == c[i] {
		n++
	}
	return n
}

// closingRun is the start of the next run of exactly length copies of ch at or after from.
func closingRun(c []rune, ch rune, length, from int) (int, bool) {
	j := from
	for j < len(c) {
		if c[j] == ch {
			r := runLength(c, j)
			if r == length {
				return j, true
			}
			j += r
		} else {
			j++
		}
	}
	return 0, false
}

// mdLink reads "[label](dest)" starting at c[i] == '[': the label, the destination without an optional "title", and the index after ")".
func mdLink(c []rune, i int) (label, url string, next int, ok bool) {
	n := len(c)
	j, depth := i+1, 1
	for j < n {
		if c[j] == '\\' {
			j += 2
			continue
		}
		if c[j] == '[' {
			depth++
		} else if c[j] == ']' {
			depth--
			if depth == 0 {
				break
			}
		}
		j++
	}
	if !(j+1 < n && c[j] == ']' && c[j+1] == '(') {
		return "", "", 0, false
	}
	k, paren := j+2, 1
	for k < n {
		if c[k] == '\\' {
			k += 2
			continue
		}
		if c[k] == '(' {
			paren++
		} else if c[k] == ')' {
			paren--
			if paren == 0 {
				break
			}
		}
		k++
	}
	if !(k < n && c[k] == ')') {
		return "", "", 0, false
	}
	dest := trimHSpace(string(c[j+2 : k]))
	if e := strings.IndexByte(dest, '>'); strings.HasPrefix(dest, "<") && e >= 0 {
		dest = dest[1:e]
	} else if sp := strings.IndexAny(dest, " \t"); sp >= 0 {
		dest = dest[:sp]
	}
	return string(c[i+1 : j]), dest, k + 1, true
}

// closingEmphasis finds the closing delimiter of an emphasis run: exactly length copies of marker, not after a space, skipping code
// spans and escapes.
func closingEmphasis(c []rune, marker rune, length, from int) (int, bool) {
	j := from
	for j < len(c) {
		if c[j] == '\\' {
			j += 2
			continue
		}
		if c[j] == '`' {
			r := runLength(c, j)
			if e, ok := closingRun(c, '`', r, j+r); ok {
				j = e + r
			} else {
				j += r
			}
			continue
		}
		if c[j] == marker {
			r := runLength(c, j)
			if r == length && j > from && !unicode.IsSpace(c[j-1]) {
				after := ' '
				if j+r < len(c) {
					after = c[j+r]
				}
				if marker != '_' || !(unicode.IsLetter(after) || unicode.IsNumber(after)) {
					return j, true
				}
			}
			j += r
			continue
		}
		j++
	}
	return 0, false
}

func parseSpans(c []rune, depth int) []MDSpan {
	var out []MDSpan
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			out = append(out, MDSpan{Kind: MDText, Text: string(buf)})
			buf = nil
		}
	}
	n := len(c)
	i := 0
	for i < n {
		ch := c[i]
		if ch == '\\' && i+1 < n && c[i+1] < 128 && strings.ContainsRune(mdEscapable, c[i+1]) {
			buf = append(buf, c[i+1])
			i += 2
			continue
		}
		if ch == '`' {
			r := runLength(c, i)
			if e, ok := closingRun(c, '`', r, i+r); ok {
				flush()
				out = append(out, MDSpan{Kind: MDCode, Text: trimHSpace(string(c[i+r : e]))})
				i = e + r
				continue
			}
			for k := 0; k < r; k++ {
				buf = append(buf, '`')
			}
			i += r
			continue
		}
		if ch == '!' && i+1 < n && c[i+1] == '[' {
			if label, url, next, ok := mdLink(c, i+1); ok {
				flush()
				out = append(out, MDSpan{Kind: MDImage, Text: label, URL: url})
				i = next
				continue
			}
		}
		if ch == '[' {
			if label, url, next, ok := mdLink(c, i); ok {
				flush()
				var kids []MDSpan
				if depth < mdMaxDepth {
					kids = parseSpans([]rune(label), depth+1)
				} else {
					kids = []MDSpan{{Kind: MDText, Text: label}}
				}
				out = append(out, MDSpan{Kind: MDLink, Children: kids, URL: url})
				i = next
				continue
			}
		}
		if depth < mdMaxDepth && (ch == '*' || ch == '_' || ch == '~') {
			r := runLength(c, i)
			before := ' '
			if i > 0 {
				before = c[i-1]
			}
			length := 0
			if ch == '~' {
				if r == 2 {
					length = 2
				}
			} else if r <= 3 {
				length = r
			}
			opens := length > 0 && i+length < n && !unicode.IsSpace(c[i+length]) &&
				(ch != '_' || !(unicode.IsLetter(before) || unicode.IsNumber(before)))
			if opens {
				if j, ok := closingEmphasis(c, ch, length, i+length); ok {
					inner := parseSpans(c[i+length:j], depth+1)
					flush()
					switch {
					case ch == '~':
						out = append(out, MDSpan{Kind: MDStrike, Children: inner})
					case length == 1:
						out = append(out, MDSpan{Kind: MDItalic, Children: inner})
					case length == 2:
						out = append(out, MDSpan{Kind: MDBold, Children: inner})
					default:
						out = append(out, MDSpan{Kind: MDBold, Children: []MDSpan{{Kind: MDItalic, Children: inner}}})
					}
					i = j + length
					continue
				}
			}
			for k := 0; k < r; k++ {
				buf = append(buf, ch)
			}
			i += r
			continue
		}
		buf = append(buf, ch)
		i++
	}
	flush()
	return out
}

// ---- inline renderers ----

// MDEscape escapes text for HTML.
func MDEscape(s string) string {
	var o strings.Builder
	o.Grow(len(s))
	for _, ch := range s {
		switch ch {
		case '&':
			o.WriteString("&amp;")
		case '<':
			o.WriteString("&lt;")
		case '>':
			o.WriteString("&gt;")
		case '"':
			o.WriteString("&quot;")
		case '\'':
			o.WriteString("&#39;")
		default:
			o.WriteRune(ch)
		}
	}
	return o.String()
}

// MDSafeURL is the target when it is http, https or mailto; ok is false for anything else (javascript:, file:, relative paths).
func MDSafeURL(url string) (string, bool) {
	u := trimmed(url)
	l := strings.ToLower(u)
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:") {
		return u, true
	}
	return "", false
}

func mdImageText(alt string) string {
	if trimmed(alt) == "" {
		return "[Image]"
	}
	return "[Image: " + trimmed(alt) + "]"
}

// MDImages answers a data: URI for an image path, or ok=false to show the alt text instead.
type MDImages func(path string) (dataURI string, ok bool)

func spansHTML(spans []MDSpan, images MDImages) string {
	var b strings.Builder
	for _, s := range spans {
		switch s.Kind {
		case MDText:
			b.WriteString(MDEscape(s.Text))
		case MDCode:
			b.WriteString("<code>" + MDEscape(s.Text) + "</code>")
		case MDBold:
			b.WriteString("<strong>" + spansHTML(s.Children, images) + "</strong>")
		case MDItalic:
			b.WriteString("<em>" + spansHTML(s.Children, images) + "</em>")
		case MDStrike:
			b.WriteString("<s>" + spansHTML(s.Children, images) + "</s>")
		case MDLink:
			u, ok := MDSafeURL(s.URL)
			if !ok {
				b.WriteString(spansHTML(s.Children, images))
			} else {
				b.WriteString("<a href=\"" + MDEscape(u) + "\">" + spansHTML(s.Children, images) + "</a>")
			}
		case MDImage:
			if images != nil {
				if data, ok := images(s.URL); ok {
					b.WriteString("<img src=\"" + MDEscape(data) + "\" alt=\"" + MDEscape(s.Text) + "\">")
					continue
				}
			}
			b.WriteString("<span class=\"img\">" + MDEscape(mdImageText(s.Text)) + "</span>")
		}
	}
	return b.String()
}

func spansPlain(spans []MDSpan) string {
	var b strings.Builder
	for _, s := range spans {
		switch s.Kind {
		case MDText, MDCode:
			b.WriteString(s.Text)
		case MDBold, MDItalic, MDStrike:
			b.WriteString(spansPlain(s.Children))
		case MDLink:
			t := spansPlain(s.Children)
			u, ok := MDSafeURL(s.URL)
			switch {
			case !ok:
				b.WriteString(t)
			case t == "" || t == u:
				b.WriteString(u)
			default:
				b.WriteString(t + " (" + u + ")")
			}
		case MDImage:
			b.WriteString(mdImageText(s.Text))
		}
	}
	return b.String()
}

func spansImageURLs(spans []MDSpan) []string {
	var out []string
	for _, s := range spans {
		switch s.Kind {
		case MDImage:
			out = append(out, s.URL)
		case MDBold, MDItalic, MDStrike, MDLink:
			out = append(out, spansImageURLs(s.Children)...)
		}
	}
	return out
}

// ---- blocks ----

func mdIsRule(s string) bool {
	b := strings.ReplaceAll(s, " ", "")
	if utf8.RuneCountInString(b) < 3 || strings.IndexByte("-*_", b[0]) < 0 {
		return false
	}
	return allBytes(b, b[0])
}

func mdIsDelimiterRow(s string) bool {
	t := trimHSpace(s)
	if !strings.Contains(t, "-") || !strings.Contains(t, "|") {
		return false
	}
	for _, r := range t {
		if !strings.ContainsRune("|-: ", r) {
			return false
		}
	}
	return true
}

func mdCells(line string) []string {
	c := []rune(trimHSpace(line))
	if len(c) > 0 && c[0] == '|' {
		c = c[1:]
	}
	if len(c) > 0 && c[len(c)-1] == '|' && (len(c) < 2 || c[len(c)-2] != '\\') {
		c = c[:len(c)-1]
	}
	var out []string
	var cur []rune
	for i := 0; i < len(c); {
		if c[i] == '\\' && i+1 < len(c) && c[i+1] == '|' {
			cur = append(cur, '|')
			i += 2
			continue
		}
		if c[i] == '|' {
			out = append(out, trimHSpace(string(cur)))
			cur = nil
			i++
			continue
		}
		cur = append(cur, c[i])
		i++
	}
	return append(out, trimHSpace(string(cur)))
}

// MDNodes is the block parse of md.
func MDNodes(md string) []MDNode {
	if utf8.RuneCountInString(md) > mdMaxInput {
		md = string([]rune(md)[:mdMaxInput])
	}
	lines := ScanBody(md)
	var out []MDNode
	var para, quote, code []string
	inCode := false
	var indents []int // indentation of the open list levels
	flush := func() {
		if len(para) > 0 {
			out = append(out, MDNode{Kind: MDParagraph, Lines: para})
			para = nil
		}
		if len(quote) > 0 {
			out = append(out, MDNode{Kind: MDQuote, Lines: quote})
			quote = nil
		}
	}
	for i := 0; i < len(lines); {
		l := lines[i]
		i++
		switch l.Kind {
		case LineFence:
			flush()
			indents = nil
			if inCode {
				out = append(out, MDNode{Kind: MDCodeBlock, Text: strings.Join(code, "\n")})
				code, inCode = nil, false
			} else {
				inCode = true
			}
		case LineCode:
			code = append(code, l.Text)
		case LineBlank:
			flush()
		case LineHeading:
			flush()
			indents = nil
			out = append(out, MDNode{Kind: MDHeading, Level: l.Level, Text: l.Title})
		case LineText:
			t := l.Text
			tr := trimHSpace(t)
			indent, isItem := listItemIndent(t)
			switch {
			case strings.Contains(tr, "|") && i < len(lines) && lines[i].Kind == LineText && mdIsDelimiterRow(lines[i].Text):
				flush()
				indents = nil
				header := mdCells(tr)
				i++
				var rows [][]string
				for i < len(lines) && lines[i].Kind == LineText && strings.Contains(lines[i].Text, "|") {
					rows = append(rows, mdCells(lines[i].Text))
					i++
				}
				out = append(out, MDNode{Kind: MDTable, Header: header, Rows: rows})
			case mdIsRule(tr):
				flush()
				indents = nil
				out = append(out, MDNode{Kind: MDRule})
			case strings.HasPrefix(tr, ">"):
				if len(para) > 0 {
					flush()
				}
				indents = nil
				quote = append(quote, strings.TrimLeft(tr, "> "))
			case isItem:
				flush()
				for len(indents) > 0 && indent < indents[len(indents)-1] {
					indents = indents[:len(indents)-1]
				}
				if len(indents) == 0 || indent > indents[len(indents)-1] {
					indents = append(indents, indent)
				}
				text, task := StripMarkers(t)
				d := 0
				for d < len(tr) && tr[d] >= '0' && tr[d] <= '9' {
					d++
				}
				out = append(out, MDNode{Kind: MDListItem, Item: MDItem{
					Level: min(len(indents)-1, mdMaxListLevel), Ordered: d > 0, Number: tr[:d], Task: task, Lines: []string{text}}})
			case len(out) > 0 && out[len(out)-1].Kind == MDListItem && len(para) == 0 && len(quote) == 0 && leadingSpaces(t) >= 2:
				last := &out[len(out)-1]
				last.Item.Lines = append(last.Item.Lines, tr) // a wrapped line of the item above
			default:
				if len(quote) > 0 {
					flush()
				}
				indents = nil
				para = append(para, tr)
			}
		}
	}
	if inCode {
		out = append(out, MDNode{Kind: MDCodeBlock, Text: strings.Join(code, "\n")})
	}
	flush()
	return out
}

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// MDImageTargets is every image path the text uses (outside code), each once, in order of appearance.
func MDImageTargets(md string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range ScanBody(md) {
		var line string
		switch l.Kind {
		case LineText:
			line = l.Text
		case LineHeading:
			line = l.Title
		default:
			continue
		}
		for _, u := range spansImageURLs(MDParseSpans(line)) {
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	return out
}

// ---- HTML ----

// MDBody renders md as an HTML fragment. images may be nil.
func MDBody(md string, images MDImages) string {
	var h strings.Builder
	var open []string // the open lists, outermost first: "ul" or "ol"
	inl := func(s string) string { return spansHTML(MDParseSpans(s), images) }
	closeTo := func(n int) {
		for len(open) > n {
			h.WriteString("</li></" + open[len(open)-1] + ">")
			open = open[:len(open)-1]
		}
	}
	for _, node := range MDNodes(md) {
		if node.Kind == MDListItem {
			it := node.Item
			tag := "ul"
			if it.Ordered {
				tag = "ol"
			}
			depth := it.Level + 1
			attrs := ""
			if it.Task != NoTaskBox {
				attrs = " class=\"task\""
			}
			if len(open) < depth {
				for len(open) < depth {
					open = append(open, tag)
					a := ""
					if len(open) == depth {
						a = attrs
					}
					h.WriteString("<" + tag + "><li" + a + ">")
				}
			} else {
				closeTo(depth)
				if open[len(open)-1] != tag {
					h.WriteString("</li></" + open[len(open)-1] + "><" + tag + "><li" + attrs + ">")
					open[len(open)-1] = tag
				} else {
					h.WriteString("</li><li" + attrs + ">")
				}
			}
			switch it.Task {
			case TaskDone:
				h.WriteString("☑ ")
			case TaskOpen:
				h.WriteString("☐ ")
			}
			var parts []string
			for _, l := range it.Lines {
				parts = append(parts, inl(l))
			}
			h.WriteString(strings.Join(parts, "<br>"))
			continue
		}
		closeTo(0)
		switch node.Kind {
		case MDHeading:
			lv := itoa(node.Level)
			h.WriteString("<h" + lv + ">" + inl(node.Text) + "</h" + lv + ">\n")
		case MDParagraph:
			h.WriteString("<p>" + joinInline(node.Lines, inl, "<br>\n") + "</p>\n")
		case MDQuote:
			h.WriteString("<blockquote><p>" + joinInline(node.Lines, inl, "<br>\n") + "</p></blockquote>\n")
		case MDCodeBlock:
			h.WriteString("<pre><code>" + MDEscape(node.Text) + "</code></pre>\n")
		case MDRule:
			h.WriteString("<hr>\n")
		case MDTable:
			h.WriteString("<table><thead><tr>")
			for _, c := range node.Header {
				h.WriteString("<th>" + inl(c) + "</th>")
			}
			h.WriteString("</tr></thead><tbody>")
			for _, r := range node.Rows {
				h.WriteString("<tr>")
				for _, c := range r {
					h.WriteString("<td>" + inl(c) + "</td>")
				}
				h.WriteString("</tr>")
			}
			h.WriteString("</tbody></table>\n")
		}
	}
	closeTo(0)
	return h.String()
}

func joinInline(lines []string, inl func(string) string, sep string) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = inl(l)
	}
	return strings.Join(parts, sep)
}

// MDCSS is the print style sheet.
const MDCSS = `body { font: 11pt/1.45 -apple-system, "Helvetica Neue", Helvetica, Arial, sans-serif; color: #1b201e; margin: 0; }
h1 { font-size: 20pt; margin: 0 0 6pt; } h2 { font-size: 14pt; margin: 18pt 0 4pt; } h3 { font-size: 12pt; margin: 12pt 0 3pt; }
h4, h5, h6 { font-size: 11pt; margin: 10pt 0 2pt; }
h1, h2, h3, h4, h5, h6 { break-after: avoid; break-inside: avoid; }
p { margin: 0 0 6pt; } ul, ol { margin: 0 0 6pt; padding-left: 18pt; } li { margin: 2pt 0; break-inside: avoid; }
li.task { list-style: none; margin-left: -14pt; }
blockquote { margin: 6pt 0; padding-left: 10pt; border-left: 2pt solid #bcc5bf; color: #566059; }
pre { background: #f0f2f0; padding: 6pt; font: 9.5pt/1.4 Menlo, monospace; white-space: pre-wrap; break-inside: avoid; }
code { font-family: Menlo, monospace; font-size: 0.92em; background: #f0f2f0; padding: 0 2pt; }
table { border-collapse: collapse; margin: 6pt 0; } th, td { border: 0.5pt solid #bcc5bf; padding: 3pt 6pt; text-align: left; }
img { max-width: 100%; max-height: 280pt; vertical-align: top; break-inside: avoid; } .img { color: #566059; } hr { border: 0; border-top: 0.5pt solid #bcc5bf; }`

// MDDocument is a whole page for printing: no script, no network (the CSP allows only inline styles and data: images).
func MDDocument(title, md string, images MDImages) string {
	return "<!DOCTYPE html>\n" +
		"<html lang=\"en\"><head><meta charset=\"utf-8\">\n" +
		"<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src data:; style-src 'unsafe-inline'\">\n" +
		"<title>" + MDEscape(title) + "</title>\n" +
		"<style>" + MDCSS + "</style></head>\n" +
		"<body>" + MDBody(md, images) + "</body></html>"
}

// MDFragment is a whole page for the pasteboard: structure plus one inline font, which is what Mail, Notes and Pages keep.
func MDFragment(md string) string {
	return "<html><head><meta charset=\"utf-8\"></head><body style=\"font-family: -apple-system, Helvetica, Arial, sans-serif; font-size: 13px;\">" +
		MDBody(md, nil) + "</body></html>"
}

// ---- plain text ----

// MDPlain renders md as plain text for chat boxes.
func MDPlain(md string) string {
	var out []string
	gap := func() {
		if n := len(out); n > 0 && out[n-1] != "" {
			out = append(out, "")
		}
	}
	p := func(s string) string { return spansPlain(MDParseSpans(s)) }
	inList := false
	for _, node := range MDNodes(md) {
		if node.Kind == MDListItem {
			it := node.Item
			if !inList {
				gap()
			}
			inList = true
			pad := strings.Repeat("  ", it.Level)
			box := ""
			switch it.Task {
			case TaskDone:
				box = "[x] "
			case TaskOpen:
				box = "[ ] "
			}
			mark := "-"
			if it.Ordered {
				mark = it.Number + "."
			}
			first := ""
			if len(it.Lines) > 0 {
				first = it.Lines[0]
			}
			out = append(out, pad+mark+" "+box+p(first))
			for _, l := range it.Lines[min(1, len(it.Lines)):] {
				out = append(out, pad+"  "+p(l))
			}
			continue
		}
		inList = false
		gap()
		switch node.Kind {
		case MDHeading:
			if node.Level == 1 {
				out = append(out, p(node.Text))
			} else {
				out = append(out, p(node.Text)+":")
			}
		case MDParagraph, MDQuote:
			for _, l := range node.Lines {
				out = append(out, p(l))
			}
		case MDCodeBlock:
			out = append(out, strings.Split(node.Text, "\n")...)
		case MDRule:
			out = append(out, "----")
		case MDTable:
			out = append(out, joinInline(node.Header, p, " | "))
			for _, r := range node.Rows {
				out = append(out, joinInline(r, p, " | "))
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}
