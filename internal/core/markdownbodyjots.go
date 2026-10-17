// markdownbodyjots.go - the Jots block in the words rule and in the editor's initial/written text. Pure.
// Port of app/Core/Capture/MarkdownBody+Jots.swift. That file belongs to the Capture chunk, but the logged rule (DayDocument.Status,
// WordsRule) cannot work without it, so it is ported here. Left out for the Capture chunk: unclosedFenceCloser and jotsHeadingText,
// which only CapturePlacement uses.
//
// A "Jots block" = a heading whose NormalizeHeading equals the normalised jots heading, at any level, running to the next heading of
// the same or higher level (the rule isCarriedHeading already uses).
//
//	BodyCounts(body, jotsHeading)             words outside the block, words inside it, number of jot list items
//	IsJotsOnly(body, jotsHeading)             true when the page is nothing but a Jots block (blank lines allowed), false for an empty page
//	StripUntouchedTemplate(md, tmpl, heading) if md starts with the trimmed template and the rest is a Jots block only (or nothing),
//	                                          return just that rest; otherwise return md unchanged. (A jots-only page is shown WITH the
//	                                          template above it in the editor, but never writes the template back to disk.)
//	LoggedWords(body, jotsHeading, jotsCount) what the "logged" rule counts: Words, plus JotWords only when jotsCount is true
//
// Details:
//   - Every Jots block of the page counts as jots (the first one is where CapturePlacement appends). A configured heading that normalises
//     to nothing (empty, emoji only) means "Jots". Fences and HTML comments are respected (a "## Jots" inside one is not a heading).
//   - JotWords: all words of the block, except a LEADING H:mm / HH:mm token on a list item (the time stamp is not a word); carried-over
//     task lines never count, code counts like everywhere else. JotCount: list items (bullet, ordered, task) indented less than two
//     spaces, i.e. the top-level notes; the nested items and continuation lines of a multi-line note are words but not extra notes.
//   - A page with no Jots block: Words == CountWords, JotWords == JotCount == 0.
//   - StripUntouchedTemplate compares the template and the page line by line, ignoring blank lines and surrounding spaces, because the
//     editor re-serialises the template's blank lines; it never touches a page where the user wrote anything outside the Jots block.
package core

import (
	"strings"
	"sync"
)

// DefaultJotsHeading is the heading jots are written under unless the user changes it.
const DefaultJotsHeading = "Jots"

// JotsRules is the Jots rule behind "logged": which heading holds the jots, and whether their words count toward a day being logged.
type JotsRules struct {
	Heading           string
	CountTowardLogged bool
}

var (
	jotsRulesMu     sync.RWMutex
	storedJotsRules = JotsRules{Heading: DefaultJotsHeading}
)

// SetJotsRules sets the rule DayDocument.Words/Status, FileStates and everything built on them follow. The app sets it from
// Settings.Capture. Guarded by a lock: background readers never see a half-written value. (Swift: LogStore.jotsRules.)
func SetJotsRules(r JotsRules) {
	jotsRulesMu.Lock()
	defer jotsRulesMu.Unlock()
	storedJotsRules = r
}

// CurrentJotsRules returns the rule set by SetJotsRules (heading "Jots", jots not counted, until the app sets it).
func CurrentJotsRules() JotsRules {
	jotsRulesMu.RLock()
	defer jotsRulesMu.RUnlock()
	return storedJotsRules
}

// BodyCountsResult is what BodyCounts returns. (Swift: MarkdownBody.Counts.)
type BodyCountsResult struct {
	Words    int // words outside every Jots block
	JotWords int // words inside the Jots blocks (a leading time stamp is not a word)
	JotCount int // top-level list items inside the Jots blocks
}

// jotsBlock is one Jots block in a scan: heading is the line of the heading, the content lines are heading+1 ..< end.
type jotsBlock struct{ heading, level, end int }

// jotsTarget is what a heading is matched against: its normalised text; "jots" when the configured heading normalises to nothing.
func jotsTarget(jotsHeading string) string {
	if n := NormalizeHeading(jotsHeading); n != "" {
		return n
	}
	return "jots"
}

// jotsBlocks lists every Jots block, in document order. A deeper heading stays inside the block; a heading of the same or a higher
// level ends it.
func jotsBlocks(lines []ScannedLine, jotsHeading string) []jotsBlock {
	target := jotsTarget(jotsHeading)
	var out []jotsBlock
	open := false
	var cur jotsBlock
	for i, l := range lines {
		if l.Kind != LineHeading {
			continue
		}
		if open {
			if l.Level > cur.level {
				continue
			}
			cur.end = i
			out = append(out, cur)
			open = false
		}
		if NormalizeHeading(l.Title) == target {
			open = true
			cur = jotsBlock{heading: i, level: l.Level}
		}
	}
	if open {
		cur.end = len(lines)
		out = append(out, cur)
	}
	return out
}

// listItemIndent returns the indentation (a tab counts 4) when line starts a list item: "-", "*", "+" or "1." / "1)" followed by a
// space or the end of the line. A thematic break ("---", "* * *") is not one. ok is false for anything else.
func listItemIndent(line string) (indent int, ok bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		if line[i] == '\t' {
			indent += 4
		} else {
			indent++
		}
		i++
	}
	s := line[i:]
	if s == "" {
		return 0, false
	}
	var rest string
	if strings.IndexByte("-*+", s[0]) >= 0 {
		rest = s[1:]
	} else {
		n := 0
		for n < len(s) && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n == 0 || n > 9 || n >= len(s) || (s[n] != '.' && s[n] != ')') {
			return 0, false
		}
		rest = s[n+1:]
	}
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, false
	}
	bare := strings.NewReplacer(" ", "", "\t", "").Replace(line)
	if len(bare) >= 3 && strings.IndexByte("-*_", bare[0]) >= 0 && strings.Count(bare, bare[:1]) == len(bare) {
		return 0, false
	}
	return indent, true
}

// dropLeadingTime: "14:32 call Sam" -> "call Sam". Drops a leading H:mm / HH:mm token (real times only: 00:00 to 23:59, two-digit
// minutes, then a space or the end). Anything else is returned unchanged.
func dropLeadingTime(text string) string {
	digits := func(from, maxDigits int) (value, count, end int) {
		i := from
		for i < len(text) && count < maxDigits && text[i] >= '0' && text[i] <= '9' {
			value = value*10 + int(text[i]-'0')
			count++
			i++
		}
		return value, count, i
	}
	hv, hn, hend := digits(0, 2)
	if hn < 1 || hv > 23 || hend >= len(text) || text[hend] != ':' {
		return text
	}
	mv, mn, mend := digits(hend+1, 2)
	if mn != 2 || mv > 59 || !(mend == len(text) || text[mend] == ' ' || text[mend] == '\t') {
		return text
	}
	return text[mend:]
}

// BodyCounts splits the page at its Jots blocks. A page without one counts exactly like CountWords.
func BodyCounts(body, jotsHeading string) BodyCountsResult {
	lines := ScanBody(body)
	blocks := jotsBlocks(lines, jotsHeading)
	if len(blocks) == 0 {
		return BodyCountsResult{Words: wordCount(contentLinesOf(lines), false)}
	}
	inBlock := make([]bool, len(lines))
	var c BodyCountsResult
	for _, b := range blocks {
		for i := b.heading; i < b.end; i++ {
			inBlock[i] = true
		}
		content := lines[b.heading+1 : b.end]
		c.JotWords += wordCount(contentLinesOf(content), true)
		for _, l := range content {
			if indent, ok := listItemIndent(l.Text); l.Kind == LineText && ok && indent < 2 {
				c.JotCount++
			}
		}
	}
	var outside []ScannedLine
	for i, l := range lines {
		if !inBlock[i] {
			outside = append(outside, l)
		}
	}
	c.Words = wordCount(contentLinesOf(outside), false)
	return c
}

// IsJotsOnly is true when the page is nothing but Jots blocks and blank lines (false for an empty page).
func IsJotsOnly(body, jotsHeading string) bool {
	lines := ScanBody(body)
	blocks := jotsBlocks(lines, jotsHeading)
	if len(blocks) == 0 {
		return false
	}
	covered := make([]bool, len(lines))
	for _, b := range blocks {
		for i := b.heading; i < b.end; i++ {
			covered[i] = true
		}
	}
	for i, l := range lines {
		if l.Kind != LineBlank && !covered[i] {
			return false
		}
	}
	return true
}

// StripUntouchedTemplate: see the header. Returns md unchanged unless it is the template followed by a Jots block only (or nothing).
func StripUntouchedTemplate(md, template, jotsHeading string) string {
	split := func(s string) []string { return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") }
	var want []string
	for _, l := range split(template) {
		if t := trimmed(l); t != "" {
			want = append(want, t)
		}
	}
	if len(want) == 0 {
		return md
	}
	lines := split(md)
	i, matched := 0, 0
	for i < len(lines) && matched < len(want) {
		l := trimmed(lines[i])
		if l != "" {
			if l != want[matched] {
				return md
			}
			matched++
		}
		i++
	}
	if matched != len(want) {
		return md
	}
	rest := TrimBody(strings.Join(lines[i:], "\n"))
	if rest == "" {
		return ""
	}
	if IsJotsOnly(rest, jotsHeading) {
		return rest
	}
	return md
}

// LoggedWords is what the "logged" rule counts: the words outside the Jots blocks, plus the jot words only when jotsCount is true.
func LoggedWords(body, jotsHeading string, jotsCount bool) int {
	c := BodyCounts(body, jotsHeading)
	if jotsCount {
		return c.Words + c.JotWords
	}
	return c.Words
}
