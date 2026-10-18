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
	expect(t, (LogLine{}).Plain() == "-", "an empty line is a bare bullet: %q", (LogLine{}).Plain())
}

func TestTimeKeyReadsOnlyABareTime(t *testing.T) {
	for tm, want := range map[string]int{"09:40": 580, "9:05": 545, "23:59": 1439} {
		got, ok := timeKey(tm)
		expect(t, ok && got == want, "%q -> %d, %v", tm, got, ok)
	}
	for _, tm := range []string{"09:40 x", "24:00", ""} {
		_, ok := timeKey(tm)
		expect(t, !ok, "%q is not a bare time", tm)
	}
}

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
