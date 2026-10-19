// loglist.go - the list page: lists of timed bullets, each done unless it ends in #next or #blocked. A page has one list
// ("## Log") or two ("## Work" and "## Personal"). Pure.
// Specs: docs/superpowers/specs/2026-10-18-writing-redesign-design.md, section 1; 2026-10-19-work-personal-split-design.md.
//
//	IsLogPage(body)        the body has a list heading: Log, Work or Personal (any level, matched with NormalizeHeading)
//	ParseLog(body)         LogPage{Before, Lines, Sections, After}; ok=false when there is no list heading
//	(p LogPage) Target(l)  the section a line routed to list l goes in
//	(p LogPage) Markdown() the page back; an untouched page comes back byte for byte
//	(l LogLine) Markdown() one bullet: its bytes as read while nothing changed, else rendered
//	(l LogLine) Plain()    the bullet without time and mark, for reviews and standups
//	InsertTimed(lines, l)  l before the first line with a later time; untimed lines are passed over
//	OpenLines(body)        the #next / #blocked / "- [ ]" lines of a list page
//
// A line: "- " or "* " or "+ ", an optional task box ("[ ]" = next, "[x]" = done), an optional H:MM / HH:MM time, the text,
// an optional trailing mark (the last word of the first row, case-insensitive). Continuation rows follow the bullet without a
// blank line. Anything else inside the section (a paragraph, a code block, a sub-heading) is an Other line, kept as written.
package core

import (
	"sort"
	"strconv"
	"strings"
)

// LogHeading is the section a one-list page keeps its lines under; ListWork and ListPersonal are the two lists of a split page.
// Constants, not settings.
const (
	LogHeading   = "Log"
	ListWork     = "Work"
	ListPersonal = "Personal"
)

// ListHeadings are the headings that make a list section.
var ListHeadings = []string{LogHeading, ListWork, ListPersonal}

// LogMark is what a line is: done (no mark), next or blocked.
type LogMark string

const (
	MarkDone    LogMark = ""
	MarkNext    LogMark = "next"
	MarkBlocked LogMark = "blocked"
)

// LogLine is one bullet of the Log section, or one Other block.
type LogLine struct {
	Time  string  `json:"time"` // "09:40" as written; "" for none
	Text  string  `json:"text"` // the rows joined by "\n", without bullet, task box, time or mark
	Mark  LogMark `json:"mark"`
	Task  bool    `json:"task"`  // written as a task line ("- [ ]" / "- [x]")
	Other bool    `json:"other"` // not a bullet: Text is the block as written
	Blank int     `json:"blank"` // blank lines before it in the file
	Raw   string  `json:"raw"`   // the rows as read; "" for a new line. Used only while the fields still match it.
	// Section is the index into LogPage.Sections of the list it belongs to (0 on a one-list page).
	Section int `json:"section"`
}

func splitLines(s string) []string { return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") }

// splitTime takes a leading H:MM / HH:MM (00:00-23:59) followed by a space or the end.
func splitTime(s string) (tm, rest string) {
	colon := strings.IndexByte(s, ':')
	if colon < 1 || colon > 2 || len(s) < colon+3 {
		return "", s
	}
	for _, c := range s[:colon] + s[colon+1:colon+3] {
		if c < '0' || c > '9' {
			return "", s
		}
	}
	h, _ := strconv.Atoi(s[:colon])
	m, _ := strconv.Atoi(s[colon+1 : colon+3])
	end := colon + 3
	if h > 23 || m > 59 || (end < len(s) && s[end] != ' ' && s[end] != '\t') {
		return "", s
	}
	return s[:end], strings.TrimLeft(s[end:], " \t")
}

// timeKey is minutes since midnight, ok=false for no time or anything but a bare time.
func timeKey(tm string) (int, bool) {
	if t, _ := splitTime(tm); t == "" || t != tm {
		return 0, false
	}
	colon := strings.IndexByte(tm, ':')
	h, _ := strconv.Atoi(tm[:colon])
	m, _ := strconv.Atoi(tm[colon+1:])
	return h*60 + m, true
}

// cutMark takes a trailing #next / #blocked off the first row.
func cutMark(s string) (string, LogMark) {
	t := strings.TrimRight(s, " \t")
	i := strings.LastIndexAny(t, " \t") // -1 (one word) makes t[i+1:] the whole string
	switch strings.ToLower(t[i+1:]) {
	case "#next":
		return strings.TrimRight(t[:i+1], " \t"), MarkNext
	case "#blocked":
		return strings.TrimRight(t[:i+1], " \t"), MarkBlocked
	}
	return s, MarkDone
}

// parseLogRows reads a bullet and its continuation rows. ok=false when the first row is not a top-level "-", "*" or "+" bullet.
func parseLogRows(rows []string) (LogLine, bool) {
	if len(rows) == 0 {
		return LogLine{}, false
	}
	if indent, ok := listItemIndent(rows[0]); !ok || indent >= 2 {
		return LogLine{}, false
	}
	s := strings.TrimLeft(rows[0], " \t")
	if strings.IndexByte("-*+", s[0]) < 0 {
		return LogLine{}, false
	}
	s = strings.TrimLeft(s[1:], " \t")
	l := LogLine{Raw: strings.Join(rows, "\n")}
	switch {
	case strings.HasPrefix(s, "[ ]"):
		l.Task, l.Mark, s = true, MarkNext, strings.TrimLeft(s[3:], " \t")
	case strings.HasPrefix(s, "[x]"), strings.HasPrefix(s, "[X]"):
		l.Task, s = true, strings.TrimLeft(s[3:], " \t")
	}
	l.Time, s = splitTime(s)
	if text, mark := cutMark(s); mark != MarkDone {
		s, l.Mark = text, mark
	}
	parts := []string{strings.TrimRight(s, " \t")}
	for _, r := range rows[1:] {
		parts = append(parts, strings.TrimRight(strings.TrimPrefix(r, "  "), " \t"))
	}
	l.Text = strings.Join(parts, "\n")
	return l, true
}

func (l LogLine) same(o LogLine) bool {
	return l.Time == o.Time && l.Text == o.Text && l.Mark == o.Mark && l.Task == o.Task
}

// Markdown is the line as it goes in the file. A trailing #next / #blocked in Text marks the line when it is read back: that is
// how typing a mark works (spec section 1).
func (l LogLine) Markdown() string {
	if l.Other {
		return l.Text
	}
	if l.Raw != "" {
		if was, ok := parseLogRows(splitLines(l.Raw)); ok && was.same(l) {
			return l.Raw
		}
	}
	rows := strings.Split(l.Text, "\n")
	head := "- "
	if l.Task {
		if l.Mark == MarkDone {
			head += "[x] "
		} else {
			head += "[ ] "
		}
	}
	if l.Time != "" {
		head += l.Time + " "
	}
	first := escapeJotLine(trimmed(rows[0]))
	if l.Mark == MarkBlocked || (l.Mark == MarkNext && !l.Task) {
		first = strings.TrimLeft(first+" #"+string(l.Mark), " ")
	}
	out := []string{strings.TrimRight(head+first, " ")}
	for _, r := range rows[1:] {
		if r = trimmed(r); r != "" {
			out = append(out, "  "+escapeJotLine(r))
		}
	}
	return strings.Join(out, "\n")
}

// Plain is the line for a review or standup: "- text", continuation rows indented, no time or mark.
func (l LogLine) Plain() string {
	if l.Other {
		return l.Text
	}
	return strings.TrimRight("- "+strings.ReplaceAll(l.Text, "\n", "\n  "), " ")
}

// LogSection is one list of a page, in file order.
type LogSection struct {
	Name  string `json:"name"`  // LogHeading, ListWork or ListPersonal, whatever case the heading is written in
	Join  string `json:"join"`  // after the first section: the page from the end of the previous list up to and including this heading line
	Trail int    `json:"trail"` // blank lines after its last line (a trailing newline counts as one)
	// Virtual is the missing partner of a split page (Work without Personal, or the reverse): it writes its heading line (Join),
	// after the last list, only once it has lines.
	Virtual bool `json:"virtual"`
}

// LogPage is a list page split around its lists.
type LogPage struct {
	Before   string       `json:"before"` // the page up to and including the first list heading line
	Lines    []LogLine    `json:"lines"`  // every list's lines; LogLine.Section says which
	Sections []LogSection `json:"sections"`
	Trail    int          `json:"trail"` // the last list's trailing blank lines; only read when Sections is empty (one list)
	After    string       `json:"after"` // the page from the heading that ends the last list; "" when it runs to the end
}

// listBlock is one list section in a scan.
type listBlock struct {
	jotsBlock
	name string
}

// listBlocks lists the list sections of a scan in file order. A list heading inside an earlier list is not a section: it stays
// an Other line of that list.
func listBlocks(scanned []ScannedLine) []listBlock {
	var all []listBlock
	for _, name := range ListHeadings {
		for _, b := range jotsBlocks(scanned, name) {
			all = append(all, listBlock{b, name})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].heading < all[j].heading })
	var out []listBlock
	for _, b := range all {
		if len(out) > 0 && b.heading < out[len(out)-1].end {
			continue
		}
		out = append(out, b)
	}
	return out
}

// IsLogPage is true when body has a list heading outside code.
func IsLogPage(body string) bool {
	return len(listBlocks(ScanBody(strings.ReplaceAll(body, "\r\n", "\n")))) > 0
}

// ParseLog splits body around its lists (see the header). ok=false when there is none.
// A whitespace-only line reads as blank and is written back as "": a deliberate exception to byte for byte (markdown ignores it,
// and a page is only rewritten after a real edit). A comment line is not blank: ScanBody blanks it, so the raw text decides.
func ParseLog(body string) (LogPage, bool) {
	text := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	scanned := ScanBody(text) // splits exactly like lines, so the indexes match
	blocks := listBlocks(scanned)
	if len(blocks) == 0 {
		return LogPage{}, false
	}
	p := LogPage{Before: strings.Join(lines[:blocks[0].heading+1], "\n"), Lines: []LogLine{}}
	has := map[string]bool{}
	for i, b := range blocks {
		s := LogSection{Name: b.name}
		if i > 0 {
			s.Join = strings.Join(lines[blocks[i-1].end:b.heading+1], "\n")
		}
		var ls []LogLine
		ls, s.Trail = parseListLines(lines, scanned, b.heading+1, b.end)
		for _, l := range ls {
			l.Section = i
			p.Lines = append(p.Lines, l)
		}
		p.Sections = append(p.Sections, s)
		has[b.name] = true
	}
	last := blocks[len(blocks)-1]
	if last.end < len(lines) {
		p.After = strings.Join(lines[last.end:], "\n")
	}
	p.Trail = p.Sections[len(p.Sections)-1].Trail
	if has[ListWork] != has[ListPersonal] { // a split page with one list: the other is there to be written into
		missing, level := ListWork, 2
		if has[ListWork] {
			missing = ListPersonal
		}
		for _, b := range blocks {
			if b.name != LogHeading {
				level = b.level
				break
			}
		}
		p.Sections = append(p.Sections, LogSection{Name: missing, Join: strings.Repeat("#", level) + " " + missing, Virtual: true})
	}
	return p, true
}

// parseListLines reads the lines from..to-1 of one list: its bullets and Other blocks, and the blank lines after the last.
func parseListLines(lines []string, scanned []ScannedLine, from, to int) ([]LogLine, int) {
	var out []LogLine
	var rows []string
	bullet, blank, startBlank := false, 0, 0
	flush := func() {
		if rows == nil {
			return
		}
		l := LogLine{Other: true, Text: strings.Join(rows, "\n")}
		if bullet {
			l, _ = parseLogRows(rows) // rows[0] was already checked as a bullet
		}
		l.Blank = startBlank
		out = append(out, l)
		rows = nil
	}
	for i := from; i < to; i++ {
		l := scanned[i]
		if l.Kind != LineCode && strings.TrimSpace(lines[i]) == "" { // a blank inside a fence is code
			flush()
			blank++
			continue
		}
		_, isBullet := parseLogRows([]string{lines[i]})
		switch {
		case l.Kind == LineText && isBullet:
			flush()
			rows, bullet, startBlank, blank = []string{lines[i]}, true, blank, 0
		case rows != nil && !bullet, rows != nil && l.Kind == LineText:
			rows = append(rows, lines[i]) // a continuation row, or more of an Other block
		default:
			flush()
			rows, bullet, startBlank, blank = []string{lines[i]}, false, blank, 0
		}
	}
	flush()
	return out, blank
}

// Target is the section a line routed to list (ListWork / ListPersonal) goes in: the list of that name, else the first list
// (a one-list page takes everything).
func (p LogPage) Target(list string) int {
	for i, s := range p.Sections {
		if s.Name == list {
			return i
		}
	}
	return 0
}

// Markdown is the page with its lists written back. A line whose Section is out of range is written into the first list, never
// dropped.
func (p LogPage) Markdown() string {
	out := []string{p.Before}
	write := func(ls []LogLine) {
		for _, l := range ls {
			for i := 0; i < l.Blank; i++ {
				out = append(out, "")
			}
			out = append(out, l.Markdown())
		}
	}
	blanks := func(n int) {
		for i := 0; i < n; i++ {
			out = append(out, "")
		}
	}
	if len(p.Sections) == 0 { // one list, from a caller that built the page by hand
		write(p.Lines)
		blanks(p.Trail)
		if p.After != "" {
			out = append(out, p.After)
		}
		return strings.Join(out, "\n")
	}
	of := make([][]LogLine, len(p.Sections))
	for _, l := range p.Lines {
		s := l.Section
		if s < 0 || s >= len(p.Sections) {
			s = 0
		}
		of[s] = append(of[s], l)
	}
	wroteVirtual := false
	for i, s := range p.Sections {
		switch {
		case s.Virtual && len(of[i]) == 0:
			continue
		case s.Virtual:
			if out[len(out)-1] != "" {
				out = append(out, "")
			}
			out = append(out, s.Join)
			wroteVirtual = true
		case i > 0:
			out = append(out, s.Join)
		}
		write(of[i])
		blanks(s.Trail)
	}
	if p.After != "" {
		if wroteVirtual && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, p.After)
	}
	return strings.Join(out, "\n")
}

// InsertTimed returns lines with l placed before the first line whose time is later than l's; untimed lines are passed
// over, and an untimed l goes at the end. lines is not changed.
// At the top, l takes the blank lines that were before the old first line (its own Blank is replaced).
func InsertTimed(lines []LogLine, l LogLine) []LogLine {
	out := append([]LogLine{}, lines...)
	at := len(out)
	if k, ok := timeKey(l.Time); ok {
		for i, x := range out {
			if xk, ok := timeKey(x.Time); ok && xk > k {
				at = i
				break
			}
		}
	}
	switch {
	case len(out) == 0:
		if l.Blank == 0 {
			l.Blank = 1 // a blank line between the heading and the first line
		}
	case at == 0:
		l.Blank, out[0].Blank = out[0].Blank, 0 // the new first line takes the blank lines after the heading
	}
	out = append(out, LogLine{})
	copy(out[at+1:], out[at:])
	out[at] = l
	return out
}

// OpenLines are the lines still to do on a list page: #next, #blocked and unticked tasks, in page order.
func OpenLines(body string) []LogLine {
	p, ok := ParseLog(body)
	if !ok {
		return nil
	}
	var out []LogLine
	for _, l := range p.Lines {
		if !l.Other && l.Mark != MarkDone {
			out = append(out, l)
		}
	}
	return out
}
