package core

// Differential test for chunk B: the SWIFT Core's output on 2,400+ cases (testdata/chunkb_swift.json, written by
// testdata/swiftdiff/main.swift from testdata/chunkb_cases.json) against this port. It covers MarkdownHTML (HTML and plain text),
// CapturePlacement (render, apply, keys), PeriodReview (periods, shifts, ranges, DST zones, whole reviews), ReviewFormats (titles, file
// names, counts, markdown, plain, HTML) and ReminderScheduler (plan, diff, copy). To add a case: edit the cases file (or its generator),
// run testdata/swiftdiff/run.sh on a Mac with the Xcode command line tools, review the diff of chunkb_swift.json, commit both.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func sdCal(c map[string]any) Calendar {
	tz, _ := c["tz"].(string)
	if tz == "" {
		tz = "America/New_York"
	}
	fw := 2
	if f, ok := c["fw"].(float64); ok {
		fw = int(f)
	}
	return Calendar{Loc: mustLoc(tz), FirstWeekday: fw}
}

// sdLocal reads "2026-10-07T14:32" in the case's zone.
func sdLocal(s string, cal Calendar) time.Time {
	var y, mo, d, h, mi int
	h = 12
	fmt.Sscanf(strings.NewReplacer("T", " ", ":", " ").Replace(s), "%d-%d-%d %d %d", &y, &mo, &d, &h, &mi) // a date alone leaves the 12:00 default
	t := time.Date(y, time.Month(mo), d, h, mi, 0, 0, cal.location())
	if _, _, td := t.Date(); td != d { // a wall time that DST skips (Havana 00:30): Foundation reads it as the same instant an hour later
		t = t.Add(time.Hour)
	}
	return t
}

func sdStr(c map[string]any, k string) string { s, _ := c[k].(string); return s }
func sdBool(c map[string]any, k string, def bool) bool {
	if b, ok := c[k].(bool); ok {
		return b
	}
	return def
}
func sdInt(c map[string]any, k string, def int) int {
	if f, ok := c[k].(float64); ok {
		return int(f)
	}
	return def
}
func sdInts(v any) []int {
	var out []int
	for _, x := range v.([]any) {
		out = append(out, int(x.(float64)))
	}
	return out
}

func sdEsc(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "\n", "\\n")
}

func sdPeriod(c map[string]any) ReviewPeriod {
	return ReviewPeriod{Scale: ReviewScale(sdStr(c, "scale")), Start: sdStr(c, "start"), End: sdStr(c, "end")}
}

func sdPS(p ReviewPeriod, ok bool) string {
	if !ok {
		return "<nil>"
	}
	return p.Start + " " + p.End
}

func sdItem(c map[string]any) CaptureItem {
	it := CaptureItem{ID: "x", Day: "2026-10-07", Text: sdStr(c, "text"), Todo: sdBool(c, "todo", false), Source: "hotkey"}
	it.Stamp = sdStr(c, "stamp") // null reads as ""
	return it
}

func sdHeading(c map[string]any) string {
	if h, ok := c["heading"].(string); ok {
		return h
	}
	return "Jots"
}

func sdImages(c map[string]any) MDImages {
	if !sdBool(c, "images", false) {
		return nil
	}
	return func(p string) (string, bool) {
		if strings.HasSuffix(p, ".png") {
			return "data:image/png;base64,AAA", true
		}
		return "", false
	}
}

func sdDump(s ReviewSummary) string {
	o := []string{fmt.Sprintf("counts %d %d %d %d", s.Counts.Logged, s.Counts.Skipped, s.Counts.Started, s.Counts.Unlogged)}
	for _, d := range s.Days {
		o = append(o, fmt.Sprintf("day|%s|%s|%d|%s|%s", d.Day, d.Status, d.Words, sdEsc(d.SkipReason), sdEsc(d.Body)))
	}
	for _, g := range s.Groups {
		o = append(o, fmt.Sprintf("group|%s|%s", g.Kind, sdEsc(g.Title)))
		for _, i := range g.Items {
			o = append(o, fmt.Sprintf("  item|%s|%s", i.Day, sdEsc(i.Text)))
		}
	}
	for _, t := range s.Tasks.Completed {
		o = append(o, fmt.Sprintf("done|%s|%s", t.Day, sdEsc(t.Text)))
	}
	for _, t := range s.Tasks.OpenAtEnd {
		o = append(o, "open|"+sdEsc(t))
	}
	for _, m := range s.Months {
		o = append(o, fmt.Sprintf("month|%s|%d %d %d %d|%v", m.Month, m.Counts.Logged, m.Counts.Skipped, m.Counts.Started, m.Counts.Unlogged, m.IsFuture))
	}
	o = append(o, fmt.Sprintf("empty|%v", s.IsEmpty()))
	return strings.Join(o, "\n")
}

func sdReview(t *testing.T, c map[string]any) string {
	cal := sdCal(c)
	s := DefaultSettings("/home/x")
	st := c["settings"].(map[string]any)
	if v, ok := st["template"].(string); ok {
		s.Template = v
	}
	if v, ok := st["weekdays"]; ok {
		s.Weekdays = WeekdaySet(sdInts(v))
	}
	s.MinWords = sdInt(st, "minWords", 20)
	if v, ok := st["carry"]; ok {
		s.CarryOverHeadings = nil
		for _, x := range v.([]any) {
			s.CarryOverHeadings = append(s.CarryOverHeadings, x.(string))
		}
	}
	s.LogStartDate = sdStr(st, "logStart")
	s.Capture.JotsHeading = "Jots"
	if v, ok := st["jotsHeading"].(string); ok {
		s.Capture.JotsHeading = v
	}
	s.Capture.JotsCountTowardLogged = sdBool(st, "jotsCount", false)
	SetJotsRules(JotsRules{Heading: s.Capture.JotsHeading, CountTowardLogged: s.Capture.JotsCountTowardLogged})
	defer SetJotsRules(JotsRules{Heading: "Jots"})
	docs := map[string]DayDocument{}
	for _, x := range c["docs"].([]any) {
		d := x.(map[string]any)
		day := sdStr(d, "day")
		docs[day] = DayDocument{Day: day, Body: sdStr(d, "body"), IsSkipped: sdBool(d, "skipped", false), SkipReason: sdStr(d, "reason")}
	}
	states := map[string]DayStatus{}
	if m, ok := c["states"].(map[string]any); ok {
		for k, v := range m {
			states[k] = DayStatus(v.(string))
		}
	}
	if sdBool(c, "autostates", false) {
		for k, d := range docs {
			if d.IsSkipped {
				states[k] = StatusSkipped
			} else {
				states[k] = d.Status(s.MinWords)
			}
		}
	}
	o, _ := c["options"].(map[string]any)
	opts := ReviewOptions{Grouping: WeekBySection, IncludeSkipped: sdBool(o, "skipped", true), IncludeJots: sdBool(o, "jots", false)}
	if sdStr(o, "grouping") == "day" {
		opts.Grouping = WeekByDay
	}
	now := sdLocal(sdStr(c, "now"), cal)
	var p ReviewPeriod
	if a := sdStr(c, "anchor"); a != "" {
		p = ReviewPeriodContaining(ReviewScale(sdStr(c, "scale")), sdLocal(a, cal), cal)
	} else {
		p = sdPeriod(c)
	}
	sum := BuildReview(p, docs, states, s, opts, cal, now)
	return "period " + p.Start + " " + p.End + "\n" + sdDump(sum) + "\n--- markdown\n" + ReviewMarkdown(sum, cal) + "--- plain\n" + ReviewPlain(sum, cal) +
		"--- html\n" + ReviewHTML(sum, cal, nil) + "\n--- rich\n" + ReviewRichHTML(sum, cal)
}

func sdFire(a []int) ReminderFire {
	return ReminderFire{Year: a[0], Month: a[1], Day: a[2], Hour: a[3], Minute: a[4]}
}

func sdRun(t *testing.T, c map[string]any) string {
	kind := sdStr(c, "kind")
	switch kind {
	case "mdbody":
		return MDBody(sdStr(c, "md"), sdImages(c))
	case "mdplain":
		return MDPlain(sdStr(c, "md"))
	case "mdfragment":
		return MDFragment(sdStr(c, "md"))
	case "mddoc":
		return MDDocument(sdStr(c, "title"), sdStr(c, "md"), sdImages(c))
	case "imagetargets":
		return strings.Join(MDImageTargets(sdStr(c, "md")), "\n")
	case "render":
		return RenderJot(sdItem(c))
	case "apply":
		return ApplyJot(sdItem(c), sdStr(c, "body"), sdHeading(c))
	case "jotkeys":
		var keys []string
		for k := range JotKeys(sdStr(c, "body"), sdHeading(c)) {
			keys = append(keys, k)
		}
		sort.Strings(keys) // Swift sorted by Unicode scalar order of its Strings; byte order of UTF-8 is the same order
		return strings.Join(keys, "\n")
	case "lastjotkey":
		if k, ok := LastJotKey(sdStr(c, "body"), sdHeading(c)); ok {
			return k
		}
		return "<nil>"
	case "key":
		return jotKey(sdStr(c, "md"))
	case "period":
		cal := sdCal(c)
		return sdPS(ReviewPeriodContaining(ReviewScale(sdStr(c, "scale")), sdLocal(sdStr(c, "anchor"), cal), cal), true)
	case "shift":
		return sdPS(ShiftReview(sdPeriod(c), sdInt(c, "n", 0), sdCal(c)))
	case "range":
		cal := sdCal(c)
		return sdPS(ReviewRange(sdLocal(sdStr(c, "from"), cal), sdLocal(sdStr(c, "to"), cal), cal), true)
	case "daycount":
		return itoa(ReviewDayCount(sdPeriod(c), sdCal(c)))
	case "title":
		return ReviewTitle(sdPeriod(c), sdCal(c), sdBool(c, "dash", false))
	case "filename":
		return ReviewFileName(sdPeriod(c), sdCal(c))
	case "countsline":
		n := sdInts(c["counts"])
		if s, ok := ReviewCountsLine(ReviewCounts{n[0], n[1], n[2], n[3]}); ok {
			return s
		}
		return "<nil>"
	case "tasksline":
		ts := TaskStats{}
		for i := 0; i < sdInt(c, "done", 0); i++ {
			ts.Completed = append(ts.Completed, ReviewItem{Day: "2026-10-05", Text: fmt.Sprintf("t%d", i)})
		}
		for i := 0; i < sdInt(c, "open", 0); i++ {
			ts.OpenAtEnd = append(ts.OpenAtEnd, fmt.Sprintf("o%d", i))
		}
		if s, ok := ReviewTasksLine(ts); ok {
			return s
		}
		return "<nil>"
	case "rowline":
		n := sdInts(c["counts"])
		return ReviewRowLine(MonthRow{Month: "2026-09", Counts: ReviewCounts{n[0], n[1], n[2], n[3]}, IsFuture: sdBool(c, "future", false)})
	case "reminder":
		cal := sdCal(c)
		s := DefaultSettings("/home/x")
		s.Weekdays = WeekdaySet(sdInts(c["weekdays"]))
		s.ReminderMinutes = sdInt(c, "minutes", 0)
		s.LogStartDate = sdStr(c, "logStart")
		s.ClosedAppReminders = sdBool(c, "closedApp", true)
		st := map[string]DayStatus{}
		if m, ok := c["states"].(map[string]any); ok {
			for k, v := range m {
				st[k] = DayStatus(v.(string))
			}
		}
		now := sdLocal(sdStr(c, "now"), cal)
		use24 := sdStr(c, "locale") == "en_GB"
		plan := ReminderPlan(now, cal, s, st, sdInt(c, "horizon", 14), use24)
		var o []string
		for _, p := range plan {
			o = append(o, fmt.Sprintf("%s|%s|%s|%s|%s", p.ID, p.Fire.key(), p.Title, p.Body, p.Category))
		}
		if n, ok := ReminderNext(plan, now, cal); ok {
			o = append(o, "next "+ReminderLabel(n, cal, use24))
		} else {
			o = append(o, "next <nil>")
		}
		return strings.Join(o, "\n")
	case "reminderdiff":
		var planned []PlannedReminder
		for _, x := range c["planned"].([]any) {
			m := x.(map[string]any)
			planned = append(planned, PlannedReminder{ID: sdStr(m, "id"), Fire: sdFire(sdInts(m["fire"])), Title: sdStr(m, "title"), Body: sdStr(m, "body"), Category: ReminderCategory})
		}
		var pending []PendingReminder
		for _, x := range c["pending"].([]any) {
			m := x.(map[string]any)
			p := PendingReminder{ID: sdStr(m, "id"), Title: sdStr(m, "title"), Body: sdStr(m, "body"), Category: ReminderCategory}
			if cat, ok := m["category"].(string); ok {
				p.Category = cat
			}
			if m["fire"] != nil {
				f := sdFire(sdInts(m["fire"]))
				p.Fire = &f
			}
			pending = append(pending, p)
		}
		add, remove := ReminderDiff(planned, pending)
		var ids []string
		for _, a := range add {
			ids = append(ids, a.ID)
		}
		return "add " + strings.Join(ids, ",") + "\nremove " + strings.Join(remove, ",")
	case "reminderlabel":
		cal := sdCal(c)
		d := sdLocal(sdStr(c, "at"), cal)
		use24 := sdStr(c, "locale") == "en_GB"
		title, body := ReminderCopy(d, cal, use24)
		return ReminderLabel(d, cal, use24) + "\n" + title + "\n" + body
	case "review":
		return sdReview(t, c)
	case "journalline":
		ts := c["ts"].(float64)
		sec := int64(ts)
		if float64(sec) > ts { // floor, so a negative time keeps a positive nanosecond part
			sec--
		}
		it := CaptureItem{ID: sdStr(c, "id"), Ts: time.Unix(sec, int64((ts-float64(sec))*1e9)), Day: sdStr(c, "day"), Text: sdStr(c, "text"),
			Stamp: sdStr(c, "stamp"), Todo: sdBool(c, "todo", false), Source: sdStr(c, "source")}
		return journalLine(it) + "\n" + journalAckLine(it.ID)
	case "normalise":
		text, todo, ok := normaliseCapture(sdStr(c, "raw"), sdBool(c, "shortcut", true))
		if !ok {
			return "<nil>"
		}
		return fmt.Sprintf("%v|%s", todo, sdEsc(text))
	case "splitjots":
		rest, jots := SplitJots(sdStr(c, "body"), sdHeading(c))
		return "rest:" + sdEsc(rest) + "\njots:" + sdEsc(jots)
	}
	return "<unknown kind " + kind + ">"
}

func TestChunkBMatchesTheSwiftCore(t *testing.T) {
	var cases []map[string]any
	raw, err := os.ReadFile("testdata/chunkb_cases.json")
	must(t, err)
	must(t, json.Unmarshal(raw, &cases))
	var want map[string]string
	raw, err = os.ReadFile("testdata/chunkb_swift.json")
	must(t, err)
	must(t, json.Unmarshal(raw, &want))
	if len(want) != len(cases) {
		t.Fatalf("cases (%d) and Swift outputs (%d) differ: regenerate with testdata/swiftdiff/run.sh", len(cases), len(want))
	}
	bad := 0
	for _, c := range cases {
		id := c["id"].(string)
		exp := want[id]
		// macOS 14+ writes a narrow no-break space before am/pm; the Go port writes a plain space (see reminderscheduler.go)
		if strings.HasPrefix(id, "reminder") {
			exp = strings.ReplaceAll(exp, " ", " ")
		}
		got := sdRun(t, c)
		if got == exp {
			continue
		}
		bad++
		if bad <= 12 {
			t.Errorf("%s differs\ncase: %v\n--- swift\n%s\n--- go\n%s", id, trimCase(c), clip(exp), clip(got))
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d cases differ from the Swift output", bad, len(cases))
	}
}

func trimCase(c map[string]any) string {
	b, _ := json.Marshal(c)
	return clip(string(b))
}

func clip(s string) string {
	if len(s) > 1500 {
		return s[:1500] + "..."
	}
	return s
}
