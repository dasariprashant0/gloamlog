// markdownbody.go - utilities for the free-form markdown page that is a day's body. Pure functions, no disk.
// Port of app/Core/MarkdownBody.swift.
//
// API:
//
//	CountWords(body) int                  the logged-rule word count (see "Word counting" below); also fine for a live "n/20" label
//	HasContent(body) bool                 anything besides headings / empty list+task markers / fence lines / comments (an image counts)
//	BodySections(body) []BodySection      flat, document order: Level (0 = before first heading), Title, NormalizedTitle, Text
//	NormalizeHeading(s) string            lowercase, emoji/punctuation stripped, spaces collapsed: "📌 To do next" -> "to do next"
//	TemplateHeadings(template) []string   raw heading titles of a template, de-duplicated
//	PlainText(body) string                one line of text: markers/images/tags removed (used for snippets)
//	InsertCarryOver(body, heading, items) puts "## <heading>" + "- [ ] item" lines at the TOP; idempotent; returns body unchanged if nothing to add
//	PartitionBody(body, headings, style)  splits a body by owning template heading (nested sub-headings stay with their owner)
//	ShiftHeadings / PruneEmptySections / TrimBody   helpers for the weekly markdown and for saving
//
// The Jots block rules (BodyCounts, IsJotsOnly, StripUntouchedTemplate, LoggedWords) are in markdownbodyjots.go.
//
// Word counting (decision log): a "word" is a whitespace-delimited token containing at least one letter or number
// (emoji, "-", "***", "|" are not words). Ignored entirely: heading lines, empty list/task markers, code-FENCE lines
// (the ``` lines themselves; the code between them still counts), image syntax ![..](..) and <img>/other HTML tags,
// HTML comments (also multi-line). List markers (-, *, +, 1., 1)), blockquote > and task boxes [ ] [x] are stripped
// before counting. Chinese/Japanese runs (Han, Hiragana, Katakana) have no spaces, so each contiguous run counts as
// ceil(length/2) words. Scripts without spaces that are not CJK (Thai, Lao, Khmer) count one word per space-delimited chunk.
// Carried-over tasks never count: task lines ("- [ ] x", "- [x] x", any marker, nested or not) under a heading that starts with
// "Carried over from" (case-insensitive after NormalizeHeading; any level; the block runs to the next heading of the same or
// a higher level, deeper sub-headings stay inside) are skipped by CountWords, ticked or rewritten alike, so one "Carry over"
// click can never log a day. Everything else there still counts (a paragraph or plain bullet typed under the block is the
// user's own writing, and a headingless page that gets a block inserted above it keeps its words). HasContent still
// sees the block, so a page holding only carried tasks is not "empty" (never deleted, skip refuses it).
// ATX headings only ("# ", "## " ...; up to 3 leading spaces; "#tag" is not a heading). Fenced code (``` or ~~~) is respected.
//
// Go vs Swift: Swift walks Characters (grapheme clusters); here the scanners walk bytes, because every character they test for
// ("#", "`", "~", ">", "-", " ") is ASCII and never occurs inside a multi-byte UTF-8 sequence. Only the places that need real
// Unicode (words, headings) walk runes.
package core

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// BodySection is one heading-delimited piece of a page.
type BodySection struct {
	Level           int    // 0 = text before the first heading, else 1...6
	Title           string // raw heading text ("📝 What I did"); "" for level 0
	NormalizedTitle string
	Text            string // lines up to the next heading (any level); HTML comments removed; outer blank lines trimmed
}

// SubheadingStyle says what PartitionBody does with a deeper heading inside a matched section.
type SubheadingStyle int

const (
	SubheadingBoldLabel SubheadingStyle = iota // keep the sub-heading as a **bold label**
	SubheadingDrop                             // drop it
)

// ---- line scanner ----

// LineKind classifies one line of a page.
type LineKind int

const (
	LineBlank LineKind = iota
	LineHeading
	LineFence // a ``` or ~~~ line itself
	LineCode  // a line inside a fenced block
	LineText
)

// ScannedLine is one classified line. Level and Title are only set for headings.
type ScannedLine struct {
	Kind  LineKind
	Level int
	Title string
	Text  string
}

type fence struct {
	ch byte // '`' or '~'
	n  int  // how many of them opened the fence
}

// openingFence: up to 3 spaces, then 3 or more backticks/tildes. A backtick fence may not have another backtick in its info string.
func openingFence(line string) (fence, bool) {
	i, spaces := 0, 0
	for i < len(line) && line[i] == ' ' {
		i++
		spaces++
		if spaces > 3 {
			return fence{}, false
		}
	}
	if i >= len(line) {
		return fence{}, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return fence{}, false
	}
	n, j := 0, i
	for j < len(line) && line[j] == ch {
		n++
		j++
	}
	if n < 3 {
		return fence{}, false
	}
	if ch == '`' && strings.Contains(line[j:], "`") {
		return fence{}, false
	}
	return fence{ch, n}, true
}

// isClosingFence: up to 3 spaces, at least as many fence characters as opened it, then only spaces or tabs.
func isClosingFence(line string, f fence) bool {
	i, spaces := 0, 0
	for i < len(line) && line[i] == ' ' {
		i++
		spaces++
		if spaces > 3 {
			return false
		}
	}
	n := 0
	for i < len(line) && line[i] == f.ch {
		n++
		i++
	}
	if n < f.n {
		return false
	}
	for ; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return false
		}
	}
	return true
}

// parseHeading reads an ATX heading: 1-6 "#", then a space/tab or the end of the line; optional closing "#"s are dropped.
func parseHeading(line string) (level int, title string, ok bool) {
	i, spaces := 0, 0
	for i < len(line) && line[i] == ' ' {
		i++
		spaces++
		if spaces > 3 {
			return 0, "", false
		}
	}
	n := 0
	for i < len(line) && line[i] == '#' {
		n++
		i++
	}
	if n < 1 || n > 6 {
		return 0, "", false
	}
	if i < len(line) && line[i] != ' ' && line[i] != '\t' {
		return 0, "", false
	}
	title = trimmed(line[i:])
	if strings.HasSuffix(title, "#") {
		end := len(title)
		for end > 0 && title[end-1] == '#' {
			end--
		}
		if end == 0 {
			title = ""
		} else if title[end-1] == ' ' || title[end-1] == '\t' {
			title = trimmed(title[:end])
		}
	}
	return n, title, true
}

// stripComments removes <!-- ... --> from one line; inComment carries an open comment over to the next line.
func stripComments(line string, inComment *bool) string {
	var out strings.Builder
	s := line
	for s != "" {
		if *inComment {
			if i := strings.Index(s, "-->"); i >= 0 {
				s = s[i+3:]
				*inComment = false
			} else {
				s = ""
			}
		} else if i := strings.Index(s, "<!--"); i >= 0 {
			// A backslash before the "<" makes it plain text, and that is how the page editor writes a typed "<!--" ("\<!--"): it
			// opens no comment. Reading it as one hid the rest of the page (later jots, words, headings) from every scanner.
			pos := len(line) - len(s) + i // where this "<!--" sits in the whole line
			backslashes := 0
			for k := pos - 1; k >= 0 && line[k] == '\\'; k-- {
				backslashes++
			}
			if backslashes%2 == 1 {
				out.WriteString(s[:i+4])
				s = s[i+4:]
			} else {
				out.WriteString(s[:i])
				s = s[i+4:]
				*inComment = true
			}
		} else {
			out.WriteString(s)
			s = ""
		}
	}
	return out.String()
}

// ScanBody classifies every line. HTML comments are removed from text/heading lines (not from fenced code).
func ScanBody(body string) []ScannedLine {
	var out []ScannedLine
	var open fence
	inFence := false
	inComment := false
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if inFence {
			if isClosingFence(raw, open) {
				inFence = false
				out = append(out, ScannedLine{Kind: LineFence, Text: raw})
			} else {
				out = append(out, ScannedLine{Kind: LineCode, Text: raw})
			}
			continue
		}
		line := stripComments(raw, &inComment)
		if f, ok := openingFence(line); ok {
			open, inFence = f, true
			out = append(out, ScannedLine{Kind: LineFence, Text: line})
			continue
		}
		if trimmed(line) == "" {
			out = append(out, ScannedLine{Kind: LineBlank})
			continue
		}
		if level, title, ok := parseHeading(line); ok {
			out = append(out, ScannedLine{Kind: LineHeading, Level: level, Title: title, Text: line})
		} else {
			out = append(out, ScannedLine{Kind: LineText, Text: line})
		}
	}
	return out
}

// ---- markers, noise, tokens ----

// TaskState says whether a line had a task box: none, "[ ]" or "[x]".
type TaskState int

const (
	NoTaskBox TaskState = iota
	TaskOpen
	TaskDone
)

func endsOrSpace(rest string) bool { return rest == "" || rest[0] == ' ' || rest[0] == '\t' }

// StripMarkers strips blockquote prefixes, one list marker and a task box.
func StripMarkers(line string) (text string, task TaskState) {
	s := line
	for {
		t := strings.TrimLeft(s, " ")
		if !strings.HasPrefix(t, ">") {
			break
		}
		s = t[1:]
		s = strings.TrimPrefix(s, " ")
	}
	t := strings.TrimLeft(s, " \t")
	if len(t) > 0 && strings.IndexByte("-*+", t[0]) >= 0 && endsOrSpace(t[1:]) {
		t = t[1:]
	} else {
		n := 0
		for n < len(t) && t[n] >= '0' && t[n] <= '9' {
			n++
		}
		if n > 0 && n <= 9 {
			rest := t[n:]
			if len(rest) > 0 && (rest[0] == '.' || rest[0] == ')') && endsOrSpace(rest[1:]) {
				t = rest[1:]
			}
		}
	}
	t = strings.TrimLeft(t, " \t")
	if len(t) >= 3 && t[0] == '[' && t[2] == ']' && (t[1] == ' ' || t[1] == 'x' || t[1] == 'X') && endsOrSpace(t[3:]) {
		task = TaskOpen
		if t[1] != ' ' {
			task = TaskDone
		}
		t = t[3:]
	}
	return trimmed(t), task
}

var (
	imageRE = regexp.MustCompile(`!\[[^\]]*\](\([^)]*\)|\[[^\]]*\])`)
	// \s in ICU (Swift's NSRegularExpression) is [\t\n\f\r\p{Z}]; Go's \s is ASCII only.
	tagRE = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9-]*([\t\n\f\r\p{Z}][^<>]*)?/?>`)
)

// RemoveNoise removes markdown images and HTML tags (each replaced by a space).
func RemoveNoise(s string) string {
	s = imageRE.ReplaceAllLiteralString(s, " ")
	return tagRE.ReplaceAllLiteralString(s, " ")
}

func isLetterOrNumber(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

func isCJK(r rune) bool {
	switch {
	case r >= 0x3400 && r <= 0x4DBF, r >= 0x4E00 && r <= 0x9FFF, r >= 0xF900 && r <= 0xFAFF, r >= 0x20000 && r <= 0x2FA1F,
		r >= 0x3040 && r <= 0x309F, r >= 0x30A0 && r <= 0x30FF, r >= 0x31F0 && r <= 0x31FF, r >= 0xFF66 && r <= 0xFF9F:
		return true
	}
	return false
}

// tokenWords counts the words in one whitespace-free token: a run of letters/numbers is one word, a CJK run is ceil(len/2).
func tokenWords(tok string) int {
	count, cjk := 0, 0
	alnum := false
	for _, r := range tok {
		if isCJK(r) {
			if alnum {
				count++
				alnum = false
			}
			cjk++
		} else {
			if cjk > 0 {
				count += (cjk + 1) / 2
				cjk = 0
			}
			if isLetterOrNumber(r) {
				alnum = true
			}
		}
	}
	if alnum {
		count++
	}
	if cjk > 0 {
		count += (cjk + 1) / 2
	}
	return count
}

// CountTokens counts the words of already-cleaned text.
func CountTokens(s string) int {
	n := 0
	for _, tok := range strings.FieldsFunc(s, unicode.IsSpace) {
		n += tokenWords(tok)
	}
	return n
}

// ---- content, words ----

// ContentLine is one line that carries writing, with its markers stripped.
type ContentLine struct {
	Text      string
	IsCode    bool
	Task      TaskState // NoTaskBox unless the line was a task line
	InCarried bool      // inside a "Carried over from" block
	ListItem  bool
}

// isCarriedHeading is true for "Carried over from ..." (what CarryOver inserts), at any level, however the heading is decorated.
func isCarriedHeading(title string) bool {
	n := NormalizeHeading(title)
	return n == "carried over from" || strings.HasPrefix(n, "carried over from ")
}

// ContentLines returns the lines that carry writing: not blank, not headings, not fence lines; list/quote/task markers stripped.
// InCarried marks lines inside a "Carried over from" block (see the header); Task is set for task lines.
func ContentLines(body string) []ContentLine { return contentLinesOf(ScanBody(body)) }

func contentLinesOf(lines []ScannedLine) []ContentLine {
	var out []ContentLine
	carried := 0 // level of the open "Carried over from" heading, 0 = none
	for _, l := range lines {
		switch l.Kind {
		case LineBlank, LineFence:
			continue
		case LineHeading:
			if carried > 0 && l.Level > carried {
				continue // deeper sub-heading: still inside the block
			}
			if isCarriedHeading(l.Title) {
				carried = l.Level
			} else {
				carried = 0
			}
		case LineCode:
			if trimmed(l.Text) != "" {
				out = append(out, ContentLine{Text: l.Text, IsCode: true, InCarried: carried > 0})
			}
		case LineText:
			text, task := StripMarkers(l.Text)
			if text != "" {
				_, isItem := listItemIndent(l.Text)
				out = append(out, ContentLine{Text: text, Task: task, InCarried: carried > 0, ListItem: isItem})
			}
		}
	}
	return out
}

// CountWords is the logged-rule word count of a page body.
func CountWords(body string) int { return wordCount(ContentLines(body), false) }

// wordCount is the one counting formula. dropJotTimes skips a leading H:mm / HH:mm token on list items (a jot's time stamp is not a word).
func wordCount(lines []ContentLine, dropJotTimes bool) int {
	n := 0
	for _, l := range lines {
		switch {
		case l.InCarried && l.Task != NoTaskBox:
			// carried-over task lines never count
		case l.IsCode:
			n += CountTokens(l.Text)
		case dropJotTimes && l.ListItem:
			n += CountTokens(RemoveNoise(dropLeadingTime(l.Text)))
		default:
			n += CountTokens(RemoveNoise(l.Text))
		}
	}
	return n
}

// HasContent is true when the body holds anything besides headings, empty markers, fence lines and comments.
func HasContent(body string) bool { return len(ContentLines(body)) > 0 }

// Collapse joins the whitespace-separated words of s with single spaces.
func Collapse(s string) string { return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ") }

// PlainText is one line of text (markers, images, tags removed) for search snippets.
func PlainText(body string) string {
	var parts []string
	for _, l := range ContentLines(body) {
		if l.IsCode {
			parts = append(parts, l.Text)
		} else {
			parts = append(parts, RemoveNoise(l.Text))
		}
	}
	return Collapse(strings.Join(parts, " "))
}

// ---- headings and sections ----

// NormalizeHeading lowercases, drops emoji and punctuation and collapses spaces: "📌 To do next" -> "to do next".
func NormalizeHeading(s string) string {
	var out strings.Builder
	pendingSpace := false
	for _, r := range s {
		if isLetterOrNumber(r) || isKeptMark(r) {
			if pendingSpace && out.Len() > 0 {
				out.WriteByte(' ')
			}
			pendingSpace = false
			out.WriteRune(r)
		} else {
			pendingSpace = true
		}
	}
	return strings.ToLower(out.String())
}

// isKeptMark keeps combining marks (accents, Devanagari vowel signs) but not variation selectors, which belong to emoji.
func isKeptMark(r rune) bool {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) {
		return !(r >= 0xFE00 && r <= 0xFE0F) && !(r >= 0xE0100 && r <= 0xE01EF)
	}
	return false
}

// BodySections cuts a body at its headings.
func BodySections(body string) []BodySection {
	var out []BodySection
	level, title := 0, ""
	var lines []string
	flush := func() {
		text := trimmed(strings.Join(lines, "\n"))
		if level > 0 || text != "" {
			out = append(out, BodySection{Level: level, Title: title, NormalizedTitle: NormalizeHeading(title), Text: text})
		}
	}
	for _, l := range ScanBody(body) {
		if l.Kind == LineHeading {
			flush()
			level, title, lines = l.Level, l.Title, nil
		} else if l.Kind == LineBlank {
			lines = append(lines, "")
		} else {
			lines = append(lines, l.Text)
		}
	}
	flush()
	return out
}

// TemplateHeadings returns the raw heading titles of a template (any level), de-duplicated by normalised title.
func TemplateHeadings(template string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range BodySections(template) {
		if s.Level > 0 && s.NormalizedTitle != "" && !seen[s.NormalizedTitle] {
			seen[s.NormalizedTitle] = true
			out = append(out, s.Title)
		}
	}
	return out
}

// ---- save / trim ----

// TrimBody removes leading blank lines and trailing whitespace; CRLF -> LF. Indentation of the first text line is kept.
func TrimBody(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for len(lines) > 0 && trimmed(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.TrimRightFunc(strings.Join(lines, "\n"), unicode.IsSpace)
}

// ---- carry-over ----

// ItemKey is the normalised key used to compare an item with existing lines ("- [ ] Call  Ana" == "call ana").
func ItemKey(text string) string {
	t, _ := StripMarkers(text)
	return strings.ToLower(Collapse(t))
}

// InsertCarryOver inserts "## <heading>", a blank line and "- [ ] item" lines at the very TOP of body.
// Idempotent: returns body unchanged when a heading with the same normalised title already exists (any level),
// or when every item is already present as a line of the body.
func InsertCarryOver(body, heading string, items []string) string {
	norm := NormalizeHeading(heading)
	for _, s := range BodySections(body) {
		if s.Level > 0 && s.NormalizedTitle == norm {
			return body
		}
	}
	have := map[string]bool{}
	for _, l := range ContentLines(body) {
		have[ItemKey(l.Text)] = true
	}
	var fresh []string
	for _, raw := range items {
		item := Collapse(raw)
		key := ItemKey(item)
		if key == "" || have[key] {
			continue
		}
		have[key] = true
		fresh = append(fresh, item)
	}
	if len(fresh) == 0 {
		return body
	}
	block := "## " + heading + "\n\n"
	for i, it := range fresh {
		if i > 0 {
			block += "\n"
		}
		block += "- [ ] " + it
	}
	rest := TrimBody(body)
	if rest == "" {
		return block
	}
	return block + "\n\n" + rest
}

// ---- weekly review helpers ----

type segment struct {
	label string // "" = none
	lines []string
}

type owned struct {
	index int
	segs  []segment
}

// PartitionMatch is the text that one template heading owns in a body.
type PartitionMatch struct {
	Index int // index into the headings passed to PartitionBody
	Text  string
}

// PartitionBody splits body by owning heading. headings are NORMALISED titles. A matched heading owns everything up to the next
// heading of the same or a higher level (deeper sub-headings stay inside, as bold labels or dropped).
// Result: matched segments in document order and the remaining text ("Other notes").
func PartitionBody(body string, headings []string, style SubheadingStyle) (matched []PartitionMatch, other string) {
	var own []owned
	others := []segment{{}}
	owner, ownerLevel := -1, 0
	for _, l := range ScanBody(body) {
		if l.Kind == LineHeading {
			norm := NormalizeHeading(l.Title)
			hi := -1
			if norm != "" {
				for i, h := range headings {
					if h == norm {
						hi = i
						break
					}
				}
			}
			switch {
			case hi >= 0:
				own = append(own, owned{index: hi, segs: []segment{{}}})
				owner, ownerLevel = len(own)-1, l.Level
			case owner >= 0 && l.Level > ownerLevel:
				own[owner].segs = append(own[owner].segs, segment{label: l.Title})
			default:
				owner = -1
				others = append(others, segment{label: l.Title})
			}
			continue
		}
		line := l.Text
		if l.Kind == LineBlank {
			line = ""
		}
		if owner >= 0 {
			segs := own[owner].segs
			segs[len(segs)-1].lines = append(segs[len(segs)-1].lines, line)
		} else {
			others[len(others)-1].lines = append(others[len(others)-1].lines, line)
		}
	}
	render := func(segs []segment) string {
		var parts []string
		for _, s := range segs {
			text := trimmed(strings.Join(s.lines, "\n"))
			if !HasContent(text) {
				continue
			}
			if s.label != "" && style == SubheadingBoldLabel {
				parts = append(parts, "**"+s.label+"**\n"+text)
			} else {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	for _, o := range own {
		if t := render(o.segs); t != "" {
			matched = append(matched, PartitionMatch{Index: o.index, Text: t})
		}
	}
	return matched, render(others)
}

func lineHasContent(l ScannedLine) bool {
	switch l.Kind {
	case LineCode:
		return trimmed(l.Text) != ""
	case LineText:
		t, _ := StripMarkers(l.Text)
		return t != ""
	}
	return false
}

func rebuild(lines []ScannedLine) string {
	var out []string
	lastBlank := true
	for _, l := range lines {
		if l.Kind == LineBlank {
			if !lastBlank {
				out = append(out, "")
			}
			lastBlank = true
			continue
		}
		lastBlank = false
		if l.Kind == LineHeading {
			h := strings.Repeat("#", l.Level)
			if l.Title != "" {
				h += " " + l.Title
			}
			out = append(out, h)
		} else {
			out = append(out, l.Text)
		}
	}
	return TrimBody(strings.Join(out, "\n"))
}

// PruneEmptySections removes headings whose section has no content (and no deeper sub-heading with content), with their empty lines.
// Also drops HTML comments and collapses blank-line runs. Used for "Copy week as markdown".
func PruneEmptySections(body string) string {
	lines := ScanBody(body)
	for changed := true; changed; {
		changed = false
		keep := make([]bool, len(lines))
		for i := range keep {
			keep[i] = true
		}
		for i := 0; i < len(lines); {
			if lines[i].Kind != LineHeading {
				i++
				continue
			}
			level := lines[i].Level
			j, hasText := i+1, false
			for j < len(lines) && lines[j].Kind != LineHeading {
				if lineHasContent(lines[j]) {
					hasText = true
				}
				j++
			}
			deeperNext := j < len(lines) && lines[j].Level > level
			if !hasText && !deeperNext {
				for k := i; k < j; k++ {
					keep[k] = false
				}
				changed = true
			}
			i = j
		}
		if changed {
			var kept []ScannedLine
			for i, l := range lines {
				if keep[i] {
					kept = append(kept, l)
				}
			}
			lines = kept
		}
	}
	return rebuild(lines)
}

// ShiftHeadings re-levels headings so the shallowest one becomes minLevel (max 6). Used to nest a body under a "## Mon 5 Oct" heading.
func ShiftHeadings(body string, minLevel int) string {
	lines := ScanBody(body)
	lowest := 7
	for _, l := range lines {
		if l.Kind == LineHeading {
			lowest = min(lowest, l.Level)
		}
	}
	offset := 0
	if lowest != 7 {
		offset = max(0, minLevel-lowest)
	}
	for i := range lines {
		if lines[i].Kind == LineHeading {
			lines[i].Level = min(6, lines[i].Level+offset)
		}
	}
	return rebuild(lines)
}

// utf8Len is the number of characters (runes) in s; Swift's String.count counted grapheme clusters, which only differs for
// combining marks and emoji sequences.
func utf8Len(s string) int { return utf8.RuneCountInString(s) }
