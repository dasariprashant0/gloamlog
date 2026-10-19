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
		"## Log\n- a\n<!-- c\nd -->\n- b",
		"## Log\n<!-- one -->\n- a",
		"## Log\n<!-- unclosed\n- a",
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

func TestParseLogReadsWhitespaceOnlyLinesAsBlank(t *testing.T) {
	p, _ := ParseLog("## Log\n  \n- a")
	expect(t, p.Markdown() == "## Log\n\n- a", "%q", p.Markdown())
}

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
	got = InsertTimed(nil, LogLine{Time: "09:00", Text: "x", Blank: 2})
	expect(t, len(got) == 1 && got[0].Blank == 2, "an empty list keeps a Blank the caller set")
	got = InsertTimed([]LogLine{}, LogLine{Time: "09:00", Text: "x", Blank: 2})
	expect(t, len(got) == 1 && got[0].Blank == 2, "an empty list keeps a Blank the caller set")
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

func TestSplitPageRoundTripsAndKnowsSections(t *testing.T) {
	body := "# d\n\n## Work\n\n- 09:40 fixed it\n- 15:30 keys #blocked\n\n## Notes\nfree text\n\n## Personal\n\n- 21:10 standup view\n\n## After\nx"
	p, ok := ParseLog(body)
	if !ok || len(p.Sections) != 2 || p.Sections[0].Name != "Work" || p.Sections[1].Name != "Personal" {
		t.Fatalf("%v %+v", ok, p.Sections)
	}
	if got := []int{p.Lines[0].Section, p.Lines[1].Section, p.Lines[2].Section}; got[0] != 0 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("sections %v", got)
	}
	if p.Markdown() != body {
		t.Fatalf("not byte for byte:\n%q", p.Markdown())
	}
	if p.Target("Personal") != 1 || p.Target("Work") != 0 {
		t.Fatal("target")
	}
}

func TestLogPageStaysOneListAndNestedHeadingIsOther(t *testing.T) {
	body := "## Log\n\n- a\n\n### Personal\n- b\n"
	p, _ := ParseLog(body)
	if len(p.Sections) != 1 || p.Target("Personal") != 0 || p.Markdown() != body {
		t.Fatalf("%+v", p.Sections)
	}
	if !p.Lines[1].Other {
		t.Fatal("a nested list heading is an Other line")
	}
}

func TestMissingPartnerWritesOnlyWithLines(t *testing.T) {
	body := "## Work\n\n- a\n\n## Notes\nx"
	p, _ := ParseLog(body)
	if len(p.Sections) != 2 || !p.Sections[1].Virtual || p.Markdown() != body {
		t.Fatalf("%+v\n%q", p.Sections, p.Markdown())
	}
	p.Lines = append(p.Lines, LogLine{Time: "21:00", Text: "home", Section: 1, Blank: 1})
	if got := p.Markdown(); got != "## Work\n\n- a\n\n## Personal\n\n- 21:00 home\n\n## Notes\nx" {
		t.Fatalf("%q", got)
	}
	// the partner of a page that ends with its list
	q, _ := ParseLog("## Personal\n- b\n")
	q.Lines = append(q.Lines, LogLine{Text: "c", Section: 1, Blank: 1})
	if got := q.Markdown(); got != "## Personal\n- b\n\n## Work\n\n- c" {
		t.Fatalf("%q", got)
	}
}

func TestOpenLinesCoverEveryList(t *testing.T) {
	got := OpenLines("## Work\n- a #next\n## Personal\n- b #blocked\n")
	if len(got) != 2 || got[1].Section != 1 {
		t.Fatalf("%+v", got)
	}
}
