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
