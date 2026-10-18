// loglist.go - the list page: one "## Log" section of timed bullets, each done unless it ends in #next or #blocked. Pure.
// Spec: docs/superpowers/specs/2026-10-18-writing-redesign-design.md, section 1.
//
//	IsLogPage(body)        the body has a Log heading (any level, matched with NormalizeHeading)
//	ParseLog(body)         LogPage{Before, Lines, Trail, After}; ok=false when there is no Log heading
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
	"strconv"
	"strings"
)

// LogHeading is the section a list page keeps its lines under. A constant, not a setting.
const LogHeading = "Log"

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

// LogPage is a list page split around its first Log section.
type LogPage struct {
	Before string    `json:"before"` // the page up to and including the Log heading line
	Lines  []LogLine `json:"lines"`
	Trail  int       `json:"trail"` // blank lines after the last line (a trailing newline counts as one), before After
	After  string    `json:"after"` // the page from the heading that ends the section; "" when it runs to the end
}

// IsLogPage is true when body has a Log heading outside code.
func IsLogPage(body string) bool {
	return len(jotsBlocks(ScanBody(strings.ReplaceAll(body, "\r\n", "\n")), LogHeading)) > 0
}

// ParseLog splits body around its first Log section (see the header). ok=false when there is none.
// A whitespace-only line reads as blank and is written back as "": a deliberate exception to byte for byte (markdown ignores it,
// and a page is only rewritten after a real edit). A comment line is not blank: ScanBody blanks it, so the raw text decides.
func ParseLog(body string) (LogPage, bool) {
	text := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	scanned := ScanBody(text) // splits exactly like lines, so the indexes match
	blocks := jotsBlocks(scanned, LogHeading)
	if len(blocks) == 0 {
		return LogPage{}, false
	}
	b := blocks[0]
	p := LogPage{Before: strings.Join(lines[:b.heading+1], "\n"), Lines: []LogLine{}}
	if b.end < len(lines) {
		p.After = strings.Join(lines[b.end:], "\n")
	}
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
		p.Lines = append(p.Lines, l)
		rows = nil
	}
	for i := b.heading + 1; i < b.end; i++ {
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
	p.Trail = blank
	return p, true
}

// Markdown is the page with its Log section written back.
func (p LogPage) Markdown() string {
	out := []string{p.Before}
	for _, l := range p.Lines {
		for i := 0; i < l.Blank; i++ {
			out = append(out, "")
		}
		out = append(out, l.Markdown())
	}
	for i := 0; i < p.Trail; i++ {
		out = append(out, "")
	}
	if p.After != "" {
		out = append(out, p.After)
	}
	return strings.Join(out, "\n")
}
