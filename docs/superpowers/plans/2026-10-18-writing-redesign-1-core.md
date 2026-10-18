# Writing redesign, piece 1: core rules — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Teach the Go core the list page from the spec (`## Log`, timed bullets, `#next` / `#blocked` marks) so jots, counting, carry-over and the review all work on it, with every existing page behaving exactly as before.

**Architecture:** One new pure file, `internal/core/loglist.go`, parses and writes the Log section. The existing rules (jot placement, word counts, carry-over, review) ask it "is this a list page?" and branch; the classic path is untouched, so the Swift differential test stays green. New days become list pages by a default-template change plus a one-time settings migration.

**Tech Stack:** Go standard library only (`internal/core` imports nothing else), the repo's `expect`/`equal` test helpers (`internal/core/helpers_test.go`), React/TS constants in `desktop/frontend/src/screens/settings/shared.tsx`.

**Spec:** [docs/superpowers/specs/2026-10-18-writing-redesign-design.md](../specs/2026-10-18-writing-redesign-design.md). Pieces 2-4 (the line editor and page, standup, commits) get their own plans once this lands.

**Commands** (the sandbox needs these on every Go command):

```bash
export GOCACHE=/tmp/claude-501/gocache GOMODCACHE=/tmp/claude-501/gomodcache GOFLAGS=-mod=mod GOTELEMETRY=off
```

Run from the repo root (`~/Projects/personal/daily-log`) unless a step says otherwise.

**Spec refinements found while planning** (folded into the spec in Task 9):
- A delivered jot goes at the **end** of `## Log` (the existing `ApplyJot` rule; jots arrive within seconds). Only commits (piece 4) use `InsertTimed`.
- Times and marks are not words; a commit's backticked hash still counts as one word (not worth a special rule).
- On a list page every Log line counts toward "logged", whatever "count jots toward logged" says: the person's lines and delivered notes look the same there.
- The Done / Next / Blocked review groups apply when the template is a list template; with a classic template the review is the Swift one, so the differential test keeps checking it.

---

## File structure

| File | Change | Responsibility |
|---|---|---|
| `internal/core/loglist.go` | create | `LogHeading`, `LogMark`, `LogLine`, `LogPage`, `IsLogPage`, `ParseLog`, `Markdown`, `InsertTimed`, `OpenLines`, `Plain` |
| `internal/core/loglist_test.go` | create | Tests for all of the above, and for counting on list pages |
| `internal/core/markdownbodyjots.go` | modify | `JotsRules.NewDaysLog`, `JotsRulesFor`, `JotsHeadingFor` |
| `internal/core/markdownbody.go` | modify | `wordCount` drops a trailing mark on jot-block list items |
| `internal/core/models.go` | modify | `Words`, `JotCount`, `Status` use `JotsRulesFor(d.Body)` |
| `internal/core/daywriter.go` | modify | Notes go to `JotsHeadingFor(body, …)` per page |
| `internal/core/settings.go` | modify | `ClassicTemplate`, new `DefaultTemplate`, `LogTemplateSeen` migration |
| `internal/core/carryover.go` | modify | `CarryCard.Marks`; a list-page source carries its open lines |
| `internal/core/periodreview.go` | modify | Done / Next / Blocked groups when the template is a list template |
| `internal/core/*_test.go` | modify | Tests that meant "the five headings" use `ClassicTemplate` |
| `desktop/notes.go` | modify | `apply` calls `core.SetJotsRules` (existing port bug: never called) |
| `desktop/capture.go` | modify | Live-page and Jot-panel heading per page |
| `desktop/frontend/src/screens/settings/shared.tsx`, `PagePane.tsx`, `Onboarding.tsx` | modify | `DEFAULT_TEMPLATE = '## Log'`, `CLASSIC_TEMPLATE` for "Daily review" |
| `docs/superpowers/specs/2026-10-18-writing-redesign-design.md`, `docs/port/STATUS.md` | modify | The refinements; status |

---

### Task 1: Log lines — parse and write one bullet

**Files:**
- Create: `internal/core/loglist.go`
- Test: `internal/core/loglist_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func logLine(t *testing.T, raw string) LogLine {
	t.Helper()
	l, ok := parseLogRows(splitLines(raw))
	if !ok {
		t.Fatalf("not a log line: %q", raw)
	}
	return l
}

func TestLogLineReadsTimeTextAndMark(t *testing.T) {
	cases := []struct {
		raw        string
		time, text string
		mark       LogMark
		task       bool
	}{
		{"- 09:40 Fixed the login loop", "09:40", "Fixed the login loop", MarkDone, false},
		{"- 9:05 Waiting on keys #blocked", "9:05", "Waiting on keys", MarkBlocked, false},
		{"- Write the test #NEXT", "", "Write the test", MarkNext, false},
		{"- tagged #next in the middle", "", "tagged #next in the middle", MarkDone, false},
		{"- [ ] call Sam", "", "call Sam", MarkNext, true},
		{"- [x] called Sam", "", "called Sam", MarkDone, true},
		{"- [ ] stuck on review #blocked", "", "stuck on review", MarkBlocked, true},
		{"* 14:30 star bullet", "14:30", "star bullet", MarkDone, false},
		{"- 24:00 not a time", "", "24:00 not a time", MarkDone, false},
		{"- 17:45 Write the migration test #next\n  Needs staging first.", "17:45", "Write the migration test\nNeeds staging first.", MarkNext, false},
		{"- #next", "", "", MarkNext, false},
	}
	for _, c := range cases {
		l := logLine(t, c.raw)
		expect(t, l.Time == c.time && l.Text == c.text && l.Mark == c.mark && l.Task == c.task && !l.Other,
			"%q -> %+v", c.raw, l)
	}
	_, ok := parseLogRows(splitLines("1. ordered"))
	expect(t, !ok, "an ordered item is not a log line")
	_, ok = parseLogRows(splitLines("  - nested"))
	expect(t, !ok, "an indented bullet is not a top-level log line")
}

func TestLogLineWritesBackUntouchedBytesAndRendersEdits(t *testing.T) {
	raw := "*   09:40   Fixed it   #blocked"
	l := logLine(t, raw)
	expect(t, l.Markdown() == raw, "untouched line keeps its bytes: %q", l.Markdown())

	l.Mark = MarkNext
	expect(t, l.Markdown() == "- 09:40 Fixed it #next", "an edited line is rendered: %q", l.Markdown())
	l.Mark = MarkDone
	expect(t, l.Markdown() == "- 09:40 Fixed it", "done has no mark: %q", l.Markdown())

	task := logLine(t, "- [ ] call Sam")
	task.Mark = MarkDone
	expect(t, task.Markdown() == "- [x] call Sam", "a task line is ticked, not tagged: %q", task.Markdown())

	fresh := LogLine{Time: "18:02", Text: "# not a heading\nsecond row", Mark: MarkBlocked}
	expect(t, fresh.Markdown() == "- 18:02 \\# not a heading #blocked\n  second row", "new line: %q", fresh.Markdown())
	expect(t, (LogLine{}).Markdown() == "-", "an empty line is a bare bullet")
}

func TestLogLinePlainDropsTimeAndMark(t *testing.T) {
	l := logLine(t, "- 17:45 Write the test #next\n  Needs staging.")
	expect(t, l.Plain() == "- Write the test\n  Needs staging.", "%q", l.Plain())
	other := LogLine{Other: true, Text: "A paragraph."}
	expect(t, other.Plain() == "A paragraph.", "%q", other.Plain())
}
```

- [ ] **Step 2: Run the test to see it fail**

Run: `go test ./internal/core/ -run 'LogLine' 2>&1 | grep -v telemetry | head`
Expected: FAIL, `undefined: parseLogRows` (and `LogLine`, `splitLines`, `MarkDone`…).

- [ ] **Step 3: Write the implementation**

```go
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
	Time  string  `json:"time"`  // "09:40" as written; "" for none
	Text  string  `json:"text"`  // the rows joined by "\n", without bullet, task box, time or mark
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

// timeKey is minutes since midnight, ok=false for no time.
func timeKey(tm string) (int, bool) {
	if t, _ := splitTime(tm); t == "" {
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
	i := strings.LastIndexAny(t, " \t")
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

// Markdown is the line as it goes in the file.
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
	return "- " + strings.ReplaceAll(l.Text, "\n", "\n  ")
}
```

- [ ] **Step 4: Run the tests to see them pass**

Run: `go test ./internal/core/ -run 'LogLine' 2>&1 | grep -v telemetry | tail -3`
Expected: `ok  github.com/dasariprashant0/gloamlog/internal/core`. If the `\#` escape case fails, print `escapeJotLine("# not a heading")` and match the expected string to what it returns (it is the existing jot escape rule; do not change it).

- [ ] **Step 5: Commit**

```bash
git add internal/core/loglist.go internal/core/loglist_test.go
git commit -m "core: read and write one log line (time, text, #next / #blocked)"
```

---

### Task 2: The Log section — parse a page, write it back byte for byte

**Files:**
- Modify: `internal/core/loglist.go` (append)
- Test: `internal/core/loglist_test.go` (append)

- [ ] **Step 1: Write the failing test**

```go
func TestParseLogFindsTheSectionAndKeepsTheRest(t *testing.T) {
	body := "intro words\n\n## Log\n\n- 09:40 a\n- 10:00 b #next\n  more of b\n\nA paragraph\nstill it\n\n```\ncode\n\nblock\n```\n\n## Notes\nx\n"
	p, ok := ParseLog(body)
	expect(t, ok, "has a Log heading")
	expect(t, p.Before == "intro words\n\n## Log", "before: %q", p.Before)
	expect(t, p.After == "## Notes\nx\n", "after: %q", p.After)
	expect(t, len(p.Lines) == 4, "two bullets, a paragraph, a code block: %+v", p.Lines)
	expect(t, p.Lines[0].Blank == 1 && p.Lines[0].Text == "a", "%+v", p.Lines[0])
	expect(t, p.Lines[1].Mark == MarkNext && p.Lines[1].Text == "b\nmore of b", "%+v", p.Lines[1])
	expect(t, p.Lines[2].Other && p.Lines[2].Text == "A paragraph\nstill it" && p.Lines[2].Blank == 1, "%+v", p.Lines[2])
	expect(t, p.Lines[3].Other && p.Lines[3].Text == "```\ncode\n\nblock\n```", "a fence keeps its blank line: %+v", p.Lines[3])
	expect(t, p.Trail == 1, "one blank line before ## Notes: %d", p.Trail)
	expect(t, p.Markdown() == body, "untouched page comes back byte for byte:\n%q", p.Markdown())
}

func TestParseLogRoundTripsAwkwardPages(t *testing.T) {
	for _, body := range []string{
		"## Log",
		"## Log\n",
		"# Log\n- a\n- b",
		"### 📝 Log\n\n\n- a\n\n\n- b\n\n",
		"## Log\n- a\nlazy continuation\n- [ ] t\n#### sub\n- c",
		"## Done\n- x\n## Log\n- y\n## Later\n- z",
	} {
		p, ok := ParseLog(body)
		expect(t, ok && p.Markdown() == body, "%q -> %q", body, p.Markdown())
	}
	_, ok := ParseLog("## What I did\n- x")
	expect(t, !ok, "a classic page is not a list page")
	_, ok = ParseLog("```\n## Log\n```")
	expect(t, !ok, "a heading inside code does not count")
	expect(t, IsLogPage("## LOG\n") && !IsLogPage("## Logbook"), "IsLogPage matches the whole normalised heading")
}

func TestParseLogEditsRenderOnlyTheChangedLine(t *testing.T) {
	body := "## Log\n\n*  09:40  a\n- 10:00 b\n"
	p, _ := ParseLog(body)
	p.Lines[1].Mark = MarkBlocked
	expect(t, p.Markdown() == "## Log\n\n*  09:40  a\n- 10:00 b #blocked\n", "%q", p.Markdown())
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/core/ -run 'ParseLog' 2>&1 | grep -v telemetry | head -5`
Expected: FAIL, `undefined: ParseLog`.

- [ ] **Step 3: Write the implementation** (append to `loglist.go`)

```go
// LogPage is a list page split around its first Log section.
type LogPage struct {
	Before string    `json:"before"` // the page up to and including the Log heading line
	Lines  []LogLine `json:"lines"`
	Trail  int       `json:"trail"` // blank lines after the last line, before After
	After  string    `json:"after"` // the page from the heading that ends the section; "" when it runs to the end
}

// IsLogPage is true when body has a Log heading outside code.
func IsLogPage(body string) bool {
	return len(jotsBlocks(ScanBody(strings.ReplaceAll(body, "\r\n", "\n")), LogHeading)) > 0
}

// ParseLog splits body around its first Log section (see the header). ok=false when there is none.
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
			l, _ = parseLogRows(rows)
		}
		l.Blank = startBlank
		p.Lines = append(p.Lines, l)
		rows = nil
	}
	for i := b.heading + 1; i < b.end; i++ {
		l := scanned[i]
		if l.Kind == LineBlank {
			flush()
			blank++
			continue
		}
		indent, isItem := listItemIndent(lines[i])
		top := l.Kind == LineText && isItem && indent < 2 && strings.IndexByte("-*+", strings.TrimLeft(lines[i], " \t")[0]) >= 0
		switch {
		case top:
			flush()
			rows, bullet, startBlank, blank = []string{lines[i]}, true, blank, 0
		case rows != nil && (!bullet || l.Kind == LineText):
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
```

Inside a code fence a blank line is `LineCode`, not `LineBlank`, so the fence stays one Other block. A sub-heading (`#### sub`) inside the section is not `LineText`, so it ends the bullet before it and starts its own Other block.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/core/ -run 'ParseLog|LogLine' 2>&1 | grep -v telemetry | tail -3`
Expected: `ok`. If the lazy-continuation case fails, check `ScanBody` gives that line `LineText`: it is then appended to the bullet's rows, and Raw (the whole thing) round-trips.

- [ ] **Step 5: Commit**

```bash
git add internal/core/loglist.go internal/core/loglist_test.go
git commit -m "core: parse the Log section and write it back byte for byte"
```

---

### Task 3: Time order and open lines

**Files:**
- Modify: `internal/core/loglist.go` (append)
- Test: `internal/core/loglist_test.go` (append)

- [ ] **Step 1: Write the failing test**

```go
func logTexts(ls []LogLine) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

func TestInsertTimedKeepsTimeOrder(t *testing.T) {
	p, _ := ParseLog("## Log\n\n- 09:00 a\n- note without time\n- 11:00 c")
	got := InsertTimed(p.Lines, LogLine{Time: "10:30", Text: "b"})
	expect(t, equal(logTexts(got), []string{"a", "note without time", "b", "c"}), "before the first later time: %v", logTexts(got))
	got = InsertTimed(p.Lines, LogLine{Time: "8:15", Text: "first"})
	expect(t, equal(logTexts(got), []string{"first", "a", "note without time", "c"}), "%v", logTexts(got))
	expect(t, got[0].Blank == 1 && got[1].Blank == 0, "the new first line takes the blank line after the heading")
	expect(t, p.Lines[0].Blank == 1, "the input is not changed")
	got = InsertTimed(p.Lines, LogLine{Text: "untimed"})
	expect(t, logTexts(got)[3] == "untimed", "an untimed line goes at the end")
	got = InsertTimed(nil, LogLine{Time: "09:00", Text: "only"})
	expect(t, len(got) == 1 && got[0].Blank == 1, "the first line of an empty Log gets a blank line before it")
	p2, _ := ParseLog("## Log")
	p2.Lines = InsertTimed(p2.Lines, LogLine{Time: "09:00", Text: "x"})
	expect(t, p2.Markdown() == "## Log\n\n- 09:00 x", "%q", p2.Markdown())
}

func TestOpenLinesAreNextBlockedAndOpenTasks(t *testing.T) {
	body := "## Log\n- 09:00 done thing\n- 10:00 later #next\n- [ ] a task\n- [x] ticked\n- stuck #blocked\n\nParagraph"
	var got []string
	for _, l := range OpenLines(body) {
		got = append(got, string(l.Mark)+":"+l.Text)
	}
	expect(t, equal(got, []string{"next:later", "next:a task", "blocked:stuck"}), "%v", got)
	expect(t, len(OpenLines("## To do next\n- [ ] x")) == 0, "a classic page has no log lines")
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/core/ -run 'InsertTimed|OpenLines' 2>&1 | grep -v telemetry | head -5`
Expected: FAIL, `undefined: InsertTimed`.

- [ ] **Step 3: Write the implementation** (append to `loglist.go`)

```go
// InsertTimed returns lines with l placed before the first line whose time is later than l's; untimed lines are passed
// over, and an untimed l goes at the end. lines is not changed.
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
	case len(out) == 0 && l.Blank == 0:
		l.Blank = 1 // a blank line between the heading and the first line
	case at == 0:
		l.Blank, out[0].Blank = out[0].Blank, 0
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
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/core/ -run 'InsertTimed|OpenLines|ParseLog|LogLine' 2>&1 | grep -v telemetry | tail -3`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/core/loglist.go internal/core/loglist_test.go
git commit -m "core: insert log lines in time order; list the open ones"
```

---

### Task 4: Counting on list pages

Every line of `## Log` counts toward "logged", and a mark is not a word. Classic pages keep `JotsCountTowardLogged`.

**Files:**
- Modify: `internal/core/markdownbodyjots.go:36-59` (JotsRules), add `JotsRulesFor`, `JotsHeadingFor`
- Modify: `internal/core/markdownbody.go:417-433` (wordCount)
- Modify: `internal/core/models.go:193-233` (Words, JotCount, Status)
- Test: `internal/core/loglist_test.go` (append)

- [ ] **Step 1: Write the failing test**

```go
func TestListPagesCountEveryLogLineAndNoMarks(t *testing.T) {
	SetJotsRules(JotsRules{Heading: "Jots"})
	defer SetJotsRules(JotsRules{Heading: "Jots"})
	page := DayDocument{Body: "## Log\n\n- 09:40 one two three #next\n- 10:00 four five #blocked"}
	expect(t, page.Words() == 5, "times and marks are not words; Log lines count although jots don't: %d", page.Words())
	expect(t, page.Status(5) == StatusLogged && page.Status(6) == StatusPartial, "status follows the same count")
	classic := DayDocument{Body: "## What I did\n\none two\n\n## Jots\n\n- 09:40 three four"}
	expect(t, classic.Words() == 2, "a classic page still leaves jots out: %d", classic.Words())
	expect(t, JotsHeadingFor(page.Body, "Jots") == LogHeading && JotsHeadingFor(classic.Body, "Jots") == "Jots", "heading per page")
	expect(t, JotsHeadingFor("", "Jots") == "Jots", "a new day follows NewDaysLog, off here")
	SetJotsRules(JotsRules{Heading: "Jots", NewDaysLog: true})
	expect(t, JotsHeadingFor("", "Jots") == LogHeading && JotsHeadingFor("  \n", "Jots") == LogHeading, "new days are list pages")
	expect(t, JotsHeadingFor("words", "Jots") == "Jots", "a page with writing but no Log stays classic")
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/core/ -run 'ListPagesCount' 2>&1 | grep -v telemetry | head -5`
Expected: FAIL, `unknown field NewDaysLog` / `undefined: JotsHeadingFor`.

- [ ] **Step 3: Implement**

In `internal/core/markdownbodyjots.go`, replace the `JotsRules` struct:

```go
type JotsRules struct {
	Heading           string
	CountTowardLogged bool
	NewDaysLog        bool // the template is a list page: a note to a day without writing starts "## Log" (the app sets it)
}
```

and add after `CurrentJotsRules`:

```go
// JotsRulesFor is the rule for one page: on a list page notes go to Log and every Log line counts toward "logged" (the
// person's lines and delivered notes look the same there). Any other page follows the configured rule.
func JotsRulesFor(body string) JotsRules {
	r := CurrentJotsRules()
	if IsLogPage(body) {
		r.Heading, r.CountTowardLogged = LogHeading, true
	}
	return r
}

// JotsHeadingFor is the section a note to this page goes under: Log on a list page, and on a day with no writing yet when new
// days are list pages; else the configured Jots heading.
func JotsHeadingFor(body, configured string) string {
	if IsLogPage(body) || (trimmed(body) == "" && CurrentJotsRules().NewDaysLog) {
		return LogHeading
	}
	return configured
}
```

In `internal/core/markdownbody.go`, `wordCount`, change the jot branch:

```go
		case dropJotTimes && l.ListItem:
			text, _ := cutMark(dropLeadingTime(l.Text)) // a jot's time stamp and a trailing #next / #blocked are not words
			n += CountTokens(RemoveNoise(text))
```

and change the comment above `wordCount` to: `// wordCount is the one counting formula. dropJotTimes skips a leading H:mm / HH:mm token and a trailing #next / #blocked on list items (neither is a word).`

In `internal/core/models.go`, use the per-page rule:

```go
func (d DayDocument) Words() int {
	if d.IsSkipped {
		return 0
	}
	rules := JotsRulesFor(d.Body)
	return LoggedWords(d.Body, rules.Heading, rules.CountTowardLogged)
}

func (d DayDocument) JotCount() int {
	if d.IsSkipped {
		return 0
	}
	return BodyCounts(d.Body, JotsRulesFor(d.Body).Heading).JotCount
}
```

and in `Status` replace `rules := CurrentJotsRules()` with `rules := JotsRulesFor(d.Body)`. Add to the `Words` doc comment: `On a list page every Log line counts (JotsRulesFor).`

- [ ] **Step 4: Run the new test and the whole package**

Run: `go test ./internal/core/ 2>&1 | grep -v telemetry | tail -5`
Expected: `ok`, including `TestChunkBMatchesTheSwiftCore`. If the Swift diff test fails, read its first differing case: if it is a jot ending in `#next` or `#blocked`, move the `cutMark` call out of `wordCount` into `BodyCounts`, only when `NormalizeHeading(jotsHeading) == NormalizeHeading(LogHeading)`; any other difference is a bug in this task.

- [ ] **Step 5: Commit**

```bash
git add internal/core/markdownbodyjots.go internal/core/markdownbody.go internal/core/models.go internal/core/loglist_test.go
git commit -m "core: on a list page every Log line counts; marks are not words"
```

---

### Task 5: Notes land in `## Log` on list pages

**Files:**
- Modify: `internal/core/daywriter.go` (lines 222, 381 and 401, 444, 479, 484-501)
- Modify: `desktop/capture.go:124`, `desktop/capture.go:231`
- Modify: `desktop/notes.go:70-73` (`apply`)
- Test: `internal/core/daywriter_test.go` (append)

- [ ] **Step 1: Write the failing test** (append to `daywriter_test.go`; it uses that file's `newCapCore`, `newCapLookup` and `mk` from `helpers_test.go`)

```go
func TestNotesGoUnderLogOnListPages(t *testing.T) {
	defer SetJotsRules(JotsRules{Heading: "Jots"})
	SetJotsRules(JotsRules{Heading: "Jots", NewDaysLog: true})
	c := newCapCore(t)
	w := c.writer(newCapLookup(), func() time.Time { return mk(2026, 10, 14, 14, 32) })
	day := "2026-10-14"

	w.Capture("first note from Claude", "claude")
	got := c.body(day)
	expect(t, strings.HasPrefix(got, "## Log\n\n- ") && strings.HasSuffix(got, " first note from Claude"), "a new day starts as a list page: %q", got)

	w.Capture("second note", "claude")
	got = c.body(day)
	expect(t, strings.Count(got, "## Log") == 1 && !strings.Contains(got, "## Jots") && strings.HasSuffix(got, " second note"), "%q", got)

	save(t, c.store, "2026-10-13", "## What I did\n\nwords")
	expect(t, JotsHeadingFor(load(t, c.store, "2026-10-13").Body, "Jots") == "Jots", "a classic page keeps the Jots heading")
}
```

If `c.body` returns the file with its `# yyyy-MM-dd` title line, compare against what follows it (read `capCore.body` at `daywriter_test.go:105` and match its shape).

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/core/ -run 'NotesGoUnderLog' 2>&1 | grep -v telemetry | head -8`
Expected: FAIL: the body has `## Jots`.

- [ ] **Step 3: Implement**

In `internal/core/daywriter.go`:

1. Line 222 (acknowledging a saved page): `keys := JotKeys(body, JotsHeadingFor(body, w.prefs().JotsHeading))`
2. `routePass`, line 381: delete `heading := w.prefs().JotsHeading`; at line 401 call `w.deliverToFile(day, items)`.
3. Line 444 (the editor's answer): `if k, ok := LastJotKey(r.NormalizedMarkdown, JotsHeadingFor(r.NormalizedMarkdown, w.prefs().JotsHeading)); ok {`
4. `fileHolds`, line 479: `return w.holds(item, JotKeys(doc.Body, JotsHeadingFor(doc.Body, w.prefs().JotsHeading)))`
5. `deliverToFile`: drop the `heading string` parameter and choose it from the body once, before the loop:

```go
func (w *DayWriter) deliverToFile(day string, items []CaptureItem) {
	doc, err := w.store.Load(day) // nil = no file yet
	if err != nil {
		return // stays in the journal: ReplayJournal delivers it when the folder (or the page) is back
	}
	body := ""
	if doc != nil {
		body = doc.Body // a skip marker has an empty body: it is written over
	}
	heading := JotsHeadingFor(body, w.prefs().JotsHeading) // Log on a list page, or on a new day when new days are list pages
	changed := false
	// ... the rest of the function unchanged
```

Run `go build ./internal/... && go vet ./internal/core/` and fix any other caller of `deliverToFile` the compiler names, the same way.

In `desktop/capture.go`, both places that read `c.n.settings.Capture.JotsHeading` (in `AppendJot` and in `jot`) choose per page from the day's file. In `AppendJot`:

```go
	c.n.mu.Lock()
	heading := c.n.settings.Capture.JotsHeading
	store := c.n.store
	c.n.mu.Unlock()
	if doc, err := store.Load(p.day); err == nil {
		body := ""
		if doc != nil {
			body = doc.Body
		}
		heading = core.JotsHeadingFor(body, heading) // a list page takes notes under Log
	}
```

In `jot`, the same block with `day` in place of `p.day`, moved below the line that computes `day` (it decides the "Added to today's page under …" wording).

In `desktop/notes.go`, `apply` sets the process-wide jot rules, as the Swift app did on every settings change (`AppModel+Capture.swift`). The port never did, so a custom Jots heading and "count jots toward logged" were ignored when counting:

```go
func (n *Notes) apply(s core.Settings) {
	n.settings = s
	n.store = core.NewLogStore(s.StorageFolder, n.backupDir, 10*time.Minute, n.now)
	core.SetJotsRules(core.JotsRules{Heading: s.Capture.JotsHeading, CountTowardLogged: s.Capture.JotsCountTowardLogged,
		NewDaysLog: core.IsLogPage(s.Template)})
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/... 2>&1 | grep -v telemetry | tail -4` then `(cd desktop && go vet . && go test . 2>&1 | grep -v "ld: warning\|telemetry" | tail -3)`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/core/daywriter.go internal/core/daywriter_test.go desktop/capture.go desktop/notes.go
git commit -m "capture: notes land in ## Log on list pages; the app sets the jot rules (never set before)"
```

---

### Task 6: New days are list pages

**Files:**
- Modify: `internal/core/settings.go:69` (constants), the `Settings` struct, `DefaultSettings`, `decodeOver`
- Modify: the `internal/core/*_test.go` files that use `DefaultTemplate` to mean the five headings
- Modify: `desktop/frontend/src/screens/settings/shared.tsx:40`, `PagePane.tsx`, `Onboarding.tsx`
- Test: `internal/core/settings_test.go` (append)

- [ ] **Step 1: Point the old tests at the old template**

```bash
cd internal/core && for f in catchup_test.go carryover_test.go markdownbodyjots_test.go logstore_test.go captureplacement_test.go markdownbody_test.go; do perl -pi -e 's/\bDefaultTemplate\b/ClassicTemplate/g' "$f"; done; cd ../..
```

Leave `settings_test.go` alone: its uses mean "the default".

- [ ] **Step 2: Write the failing test** (append to `settings_test.go`)

```go
func TestNewDaysAreListPagesAndOldDefaultsMoveOnce(t *testing.T) {
	home := "/Users/someone"
	expect(t, DefaultTemplate == "## Log" && DefaultSettings(home).LogTemplateSeen, "fresh installs start on the list page")

	old := DecodeSettings([]byte(`{"template":`+jsonString(ClassicTemplate)+`}`), home)
	expect(t, old.Template == DefaultTemplate && old.LogTemplateSeen, "an untouched five-heading template moves to the list page")

	custom := DecodeSettings([]byte(`{"template":"## Wins\n\n## Next"}`), home)
	expect(t, custom.Template == "## Wins\n\n## Next" && custom.LogTemplateSeen, "a custom template is never touched")

	chosen := DecodeSettings([]byte(`{"template":`+jsonString(ClassicTemplate)+`,"logTemplateSeen":true}`), home)
	expect(t, chosen.Template == ClassicTemplate, "choosing the five headings again after the move sticks")
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }
```

Add `"encoding/json"` to the file's imports if it is not there.

- [ ] **Step 3: Run it to see it fail**

Run: `go test ./internal/core/ -run 'NewDaysAreListPages' 2>&1 | grep -v telemetry | head -5`
Expected: FAIL, `undefined: ClassicTemplate` / `LogTemplateSeen`.

- [ ] **Step 4: Implement** in `internal/core/settings.go`

Replace the `DefaultTemplate` constant:

```go
	// ClassicTemplate is the five-heading page every day used before the list page (still offered in Settings > Page).
	ClassicTemplate = "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next"
	DefaultTemplate = "## " + LogHeading // new days are list pages
```

Add the field to `Settings`, after `Template`:

```go
	LogTemplateSeen    bool          `json:"logTemplateSeen"`        // the one-time move of an untouched ClassicTemplate to the list page has run
```

In `DefaultSettings`, add `LogTemplateSeen: true,` after `Template: DefaultTemplate,`.

In `decodeOver`, add `{"logTemplateSeen", &s.LogTemplateSeen},` to the `steps` table. After the `template` / `legacySectionsTemplate` block, before the function returns `nil`:

```go
	// An old settings file has no logTemplateSeen; decoding starts from DefaultSettings (true), so mark it unseen first.
	if _, ok := o["logTemplateSeen"]; !ok {
		s.LogTemplateSeen = false
	}
	if !s.LogTemplateSeen {
		if s.Template == ClassicTemplate { // untouched: move it once; a custom template stays
			s.Template = DefaultTemplate
		}
		s.LogTemplateSeen = true
	}
```

Add to the header comment line for `Template` (line 19): `New days are list pages ("## Log"); an untouched old default moves once (logTemplateSeen).`

- [ ] **Step 5: Run the Go tests**

Run: `go test ./internal/... 2>&1 | grep -v telemetry | tail -4`
Expected: `ok`. A failure such as `markdownbody_test.go:144` (`TemplateHeadings(...)`) means a `DefaultTemplate` the perl step missed: change it to `ClassicTemplate`.

- [ ] **Step 6: Update the frontend constants**

In `desktop/frontend/src/screens/settings/shared.tsx`, replace line 40:

```ts
/** The five-heading page every day used before the list page. Still offered as "Daily review". */
export const CLASSIC_TEMPLATE = '## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next';
/** New days: the list page (internal/core DefaultTemplate). */
export const DEFAULT_TEMPLATE = '## Log';
```

In `PagePane.tsx` and `Onboarding.tsx`, import `CLASSIC_TEMPLATE` beside `DEFAULT_TEMPLATE`; the "Daily review" preset's `markdown` becomes `CLASSIC_TEMPLATE`; add a first preset for the list page. `PagePane.tsx` presets (line 11 area):

```ts
    {title: 'Log list', markdown: DEFAULT_TEMPLATE, carry: DEFAULT_CARRY},
```

`Onboarding.tsx` presets (line 15 area):

```ts
    {id: 'log', title: 'Log list', detail: 'One list through the day. Mark lines next or blocked.', markdown: DEFAULT_TEMPLATE, carry: DEFAULT_CARRY,
        preview: ['09:40 Fixed the login loop', '17:45 Write the test #next']},
```

Both match their file's `Preset` shape (`Onboarding.tsx` line 13: `preview: string[]`). Onboarding preselects `'review'` in two places, `useState<string | null>('review')` (line 48) and `setPreset('review')` (line 83): change both to `'log'`.

Run: `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: `tsc ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/core desktop/frontend/src/screens
git commit -m "settings: new days are list pages; an untouched old template moves once"
```

---

### Task 7: Carry-over from a list page

**Files:**
- Modify: `internal/core/carryover.go` (`CarryCard`, `CarryCardBefore`)
- Test: `internal/core/carryover_test.go` (append)

- [ ] **Step 1: Write the failing test**

```go
func TestCarryCardFromAListPageCarriesItsOpenLines(t *testing.T) {
	cal := NewCalendar(time.UTC, 2)
	s := DefaultSettings("/Users/someone")
	s.MinWords = 3
	pages := map[string]*DayDocument{
		"2026-10-16": {Day: "2026-10-16", Body: "## Log\n\n- 09:00 shipped the export today\n- 10:00 write the test #next\n- 11:00 keys from ops #blocked\n- [ ] call Sam"},
	}
	load := func(d string) *DayDocument { return pages[d] }
	card := CarryCardBefore("2026-10-17", cal, s, load)
	expect(t, card != nil && equal(card.Items, []string{"write the test", "keys from ops", "call Sam"}), "%+v", card)
	expect(t, equal(card.Marks, []LogMark{MarkNext, MarkBlocked, MarkNext}), "marks travel with the items: %+v", card.Marks)

	pages["2026-10-16"].Body = "## Log\n\n- 09:00 shipped the export today, all done"
	expect(t, CarryCardBefore("2026-10-17", cal, s, load) == nil, "nothing open, no card")
}
```

Add `"time"` to the file's imports if missing.

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/core/ -run 'CarryCardFromAListPage' 2>&1 | grep -v telemetry | head -5`
Expected: FAIL, `card.Marks undefined`.

- [ ] **Step 3: Implement** in `internal/core/carryover.go`

Add to `CarryCard`:

```go
	Marks       []LogMark `json:"marks"` // list-page source only: Items[i] is Marks[i]; nil for a classic source
```

In `CarryCardBefore`, replace `items := CarryItems(doc.Body, settings.CarryOverHeadings)` through the final `return &CarryCard{...}` of the loop body with:

```go
		var items []string
		var marks []LogMark
		if IsLogPage(doc.Body) {
			for _, l := range OpenLines(doc.Body) {
				items, marks = append(items, Collapse(l.Text)), append(marks, l.Mark)
			}
		} else {
			items = CarryItems(doc.Body, settings.CarryOverHeadings)
		}
		if len(items) == 0 {
			return nil
		}
		return &CarryCard{SourceDay: d, SourceLabel: DayKeyFormat(d, "EEE d MMM", cal.location()), Items: items, Marks: marks}
```

Add to the file's header, under `CarryCardBefore`: `On a list page the items are its open lines (OpenLines) and Marks says which is next or blocked.`

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/core/ 2>&1 | grep -v telemetry | tail -3`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/core/carryover.go internal/core/carryover_test.go
git commit -m "core: a list page carries its open lines, with their marks"
```

---

### Task 8: Review groups — Done, Next, Blocked

Only when the template is a list template (`IsLogPage(settings.Template)`): list pages split by mark and classic pages map their headings. With a classic template the review is exactly the Swift one, which the differential test checks.

**Files:**
- Modify: `internal/core/periodreview.go:229-327` (`BuildReview`)
- Test: `internal/core/weeklyreview_test.go` (append)

- [ ] **Step 1: Write the failing test**

```go
func TestReviewGroupsListAndClassicPagesIntoDoneNextBlocked(t *testing.T) {
	cal := NewCalendar(time.UTC, 2)
	s := DefaultSettings("/Users/someone") // list template
	s.MinWords = 1
	docs := map[string]DayDocument{
		"2026-10-12": {Day: "2026-10-12", Body: "## What I did\n\nold work\n\n## To do next\n\n- [ ] old next\n\n## Pending / blocked\n\nold block\n\n## Ideas\n\nan idea"},
		"2026-10-13": {Day: "2026-10-13", Body: "## Log\n\n- 09:00 new work\n- 10:00 new next #next\n- 11:00 new block #blocked\n\n## Aside\n\nnote outside"},
	}
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	p := ReviewPeriodContaining(ScaleWeek, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC), cal)
	sum := BuildReview(p, docs, nil, s, ReviewOptions{Grouping: WeekBySection}, cal, now)
	var titles []string
	byTitle := map[string][]ReviewItem{}
	for _, g := range sum.Groups {
		titles = append(titles, g.Title)
		byTitle[g.Title] = g.Items
	}
	expect(t, equal(titles, []string{"Done", "Next", "Blocked", WeekOtherTitle}), "groups: %v", titles)
	expect(t, len(byTitle["Done"]) == 2 && byTitle["Done"][0].Text == "old work" && byTitle["Done"][1].Text == "- new work", "%+v", byTitle["Done"])
	expect(t, byTitle["Next"][1].Text == "- new next" && byTitle["Blocked"][1].Text == "- new block", "marks dropped: %+v %+v", byTitle["Next"], byTitle["Blocked"])
	other := byTitle[WeekOtherTitle]
	expect(t, len(other) == 2 && strings.Contains(other[0].Text, "an idea") && strings.Contains(other[1].Text, "note outside"), "%+v", other)

	s.Template = ClassicTemplate
	sum = BuildReview(p, docs, nil, s, ReviewOptions{Grouping: WeekBySection}, cal, now)
	expect(t, sum.Groups[0].Title == "What I did", "a classic template keeps the Swift groups: %v", sum.Groups[0].Title)
}
```

Add `"strings"` and `"time"` to the file's imports if missing.

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/core/ -run 'ReviewGroupsList' 2>&1 | grep -v telemetry | head -5`
Expected: FAIL: the groups are `[Log Other notes]` or similar.

- [ ] **Step 3: Implement** in `internal/core/periodreview.go`

Add above `BuildReview`:

```go
// The list page's review groups, and where the classic default headings go when the template is a list template.
var (
	listGroupTitles = []string{"Done", "Next", "Blocked"}
	classicGroupOf  = map[string]int{"what i did": 0, "finished": 0, "started": 0, "to do next": 1, "pending blocked": 2}
)

func markGroup(m LogMark) int {
	switch m {
	case MarkNext:
		return 1
	case MarkBlocked:
		return 2
	}
	return 0
}

// outsideLog is a list page without its Log section: what Before holds above the heading, and After.
func outsideLog(p LogPage) string {
	before := ""
	if i := strings.LastIndexByte(p.Before, '\n'); i >= 0 {
		before = p.Before[:i]
	}
	return TrimBody(strings.TrimSpace(before + "\n\n" + p.After))
}
```

In `BuildReview`, replace the block from `template := TemplateHeadings(settings.Template)` through `buckets := make([][]ReviewItem, len(template))` with:

```go
	listMode := IsLogPage(settings.Template)
	template := TemplateHeadings(settings.Template)
	if listMode {
		// The five classic headings are always recognised (old pages), then any other heading of the template.
		template = append(TemplateHeadings(ClassicTemplate), template...)
	}
	norm := make([]string, len(template))
	for i, t := range template {
		norm[i] = NormalizeHeading(t)
	}
	// slot is where a template heading's text goes: its own index; in list mode a Done / Next / Blocked group (0-2) for a
	// classic heading and 3+ for any other heading of the template.
	slot := func(i int) int { return i }
	nSlots := len(template)
	var slotTitles []string
	if listMode {
		slotTitles = append([]string{}, listGroupTitles...)
		extra := map[string]int{}
		for _, t := range template {
			n := NormalizeHeading(t)
			if _, classic := classicGroupOf[n]; !classic && n != NormalizeHeading(LogHeading) && extra[n] == 0 {
				slotTitles = append(slotTitles, t)
				extra[n] = len(slotTitles) - 1 // 3 or more, so 0 means "not seen"
			}
		}
		slot = func(i int) int {
			if g, ok := classicGroupOf[norm[i]]; ok {
				return g
			}
			if s, ok := extra[norm[i]]; ok {
				return s
			}
			return -1 // the Log heading itself: never matched on a classic page
		}
		nSlots = len(slotTitles)
	}
	buckets := make([][]ReviewItem, nSlots)
```

Inside the per-day loop, replace the block from `matched, otherText := PartitionBody(rest, norm, SubheadingBoldLabel)` through the end of the `if otherText != ""` block with:

```go
		var otherText string
		per := map[int][]string{}
		if page, ok := ParseLog(rest); ok && listMode {
			for _, l := range page.Lines {
				g := markGroup(l.Mark)
				per[g] = append(per[g], l.Plain())
			}
			for g, texts := range per {
				per[g] = []string{strings.Join(texts, "\n")} // one list per group per day
			}
			otherText = outsideLog(page)
		} else {
			var matched []PartitionMatch
			matched, otherText = PartitionBody(rest, norm, SubheadingBoldLabel)
			for _, m := range matched {
				if s := slot(m.Index); s >= 0 {
					per[s] = append(per[s], m.Text)
				}
			}
		}
		var idx []int
		for i := range per {
			idx = append(idx, i)
		}
		sort.Ints(idx)
		for _, i := range idx {
			buckets[i] = append(buckets[i], ReviewItem{Day: k, Text: strings.Join(per[i], "\n\n")})
		}
		if otherText != "" {
			other = append(other, ReviewItem{Day: k, Text: otherText})
		}
```

At the end, replace the `for i := range template { … }` loop that adds `GroupHeading` groups with:

```go
	for i, items := range buckets {
		if len(items) == 0 {
			continue
		}
		title := ""
		if listMode {
			title = slotTitles[i]
		} else {
			title = template[i]
		}
		summary.Groups = append(summary.Groups, ReviewGroup{Title: title, Kind: GroupHeading, Items: items})
	}
```

When `listMode` is false nothing changes: `slot` is the identity and `nSlots == len(template)`.

- [ ] **Step 4: Run the tests, the Swift diff test included**

Run: `go test ./internal/core/ 2>&1 | grep -v telemetry | tail -3`
Expected: `ok`. `TestChunkBMatchesTheSwiftCore` must stay green: its cases set a classic template, so `listMode` is false. If a case builds settings without a template (and now gets `## Log`), set `s.Template = ClassicTemplate` in the review case builder (near `swiftdiff_test.go:150`) and say so in the commit message.

- [ ] **Step 5: Commit**

```bash
git add internal/core/periodreview.go internal/core/weeklyreview_test.go
git commit -m "review: Done / Next / Blocked groups on the list template; old headings map onto them"
```

---

### Task 9: Full check, docs, status

**Files:**
- Modify: `docs/superpowers/specs/2026-10-18-writing-redesign-design.md`
- Modify: `docs/port/STATUS.md`

- [ ] **Step 1: Run every check**

```bash
gofmt -l internal desktop/*.go
go vet ./internal/... && go test ./internal/... 2>&1 | grep -v telemetry | tail -4
(cd desktop && go vet . && go test . 2>&1 | grep -v "ld: warning\|telemetry" | tail -3)
(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)
```

Expected: no gofmt output; every package `ok`; `tsc ok`.

- [ ] **Step 2: Check it in the browser mock**

Start the `gloamlog-mock` preview (in loadout's `.claude/launch.json`), open Settings > Page: "Log list" is the first preset; "Daily review" shows the five headings. The day page still uses the classic editor in this piece; that is expected (piece 2 replaces it).

- [ ] **Step 3: Fold the planning refinements into the spec**

In the spec's section 1, the "Logged" bullet becomes: `**Logged** keeps its rule: words ≥ MinWords (default 20). Times and marks are not words. On a list page every Log line counts, whatever "count jots toward logged" says (the person's lines and delivered notes look the same).` In section 4, the Jot bullet says the line goes at the **end** of `## Log`, and the Review bullet adds: `These groups apply when the template is a list template; with a classic template the review is unchanged.`

- [ ] **Step 4: Status**

In `docs/port/STATUS.md`, under "Built", add: ``- **Writing redesign, piece 1 (core):** list pages (`## Log`, #next / #blocked) in counting, capture, carry-over and the review; new days are list pages; the app now sets the jot rules on every settings change (the port never did).``

- [ ] **Step 5: Commit**

```bash
git add docs
git commit -m "docs: writing redesign piece 1 status; fold planning refinements into the spec"
```

---

## After this plan

Piece 2 (the line editor and list page), piece 3 (standup) and piece 4 (commits) each get a plan written against the API above. Until piece 2 lands, list pages open in the classic editor, which handles `## Log` as ordinary markdown, so nothing breaks in between.
