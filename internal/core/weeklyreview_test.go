package core

// Ported from tests/main.swift: "weekly review" and "C5 week review completeness".

import (
	"strings"
	"testing"
	"time"
)

func weekFixture(t *testing.T, minWords int, template *string) (WeekSummary, *LogStore) {
	t.Helper()
	store := newStore(t)
	mon := "## What I did\nPlanned the week and reviewed pricing page copy with Ana.\nLong standup then deep work on the editor bridge.\n\n## Finished\nShipped A\nand B\n\n## Started\nDocs\n\n## Pending / blocked\n\n## To do next\n- [ ] Plan Q4"
	tue := "## Standup notes\nQuick sync about launch dates and owners for the week ahead plans.\n\n## Finished\nShipped C\n### Details\nten words here to describe what was shipped for the customer demo\n\n## Pending / blocked\nWaiting on legal"
	thu := "Spent the whole day on the editor bridge and fixed three bugs in the autosave debounce logic, then wrote notes for tomorrow."
	save(t, store, "2026-10-05", mon)
	save(t, store, "2026-10-06", tue)
	must(t, store.Skip("2026-10-07", "Holiday"))
	save(t, store, "2026-10-08", thu)
	save(t, store, "2026-10-09", "Short note only")
	save(t, store, "2026-09-30", lorem(30, "outside"))
	s := defaults()
	s.MinWords = minWords
	s.Template = ClassicTemplate
	if template != nil {
		s.Template = *template
	}
	w := SummarizeWeek(noon(2026, 10, 7), val(store.Documents("2026-10-05", "2026-10-11")), val(store.FileStates(minWords)), s, ny, mk(2026, 10, 11, 20, 0))
	return w, store
}

func sectionTitles(w WeekSummary) []string {
	var out []string
	for _, s := range w.Sections {
		out = append(out, s.Title)
	}
	return out
}

func itemTexts(s *WeekSection) []string {
	var out []string
	for _, it := range s.Items {
		out = append(out, it.Text)
	}
	return out
}

func itemDays(s *WeekSection) []string {
	var out []string
	for _, it := range s.Items {
		out = append(out, it.Day)
	}
	return out
}

func TestWeeklyReviewAggregateByTemplateHeadingPlusOtherNotes(t *testing.T) {
	w, _ := weekFixture(t, 20, nil)
	expect(t, w.WeekStart == "2026-10-05" && w.WeekEnd == "2026-10-11" && len(w.Days) == 7, "week range")
	expect(t, w.LoggedCount == 3 && w.WorkdayCount == 5 && w.SkippedCount == 1, "counts %d %d %d", w.LoggedCount, w.WorkdayCount, w.SkippedCount)
	var sts []DayStatus
	for _, d := range w.Days {
		sts = append(sts, d.Status)
	}
	expect(t, equal(sts, []DayStatus{StatusLogged, StatusLogged, StatusSkipped, StatusLogged, StatusPartial, StatusOff, StatusOff}), "%v", sts)
	expect(t, equal(sectionTitles(w), []string{"What I did", "Finished", "Started", "Pending / blocked", "To do next", "Other notes"}), "%v", sectionTitles(w))
	var others []bool
	for _, s := range w.Sections {
		others = append(others, s.IsOther)
	}
	expect(t, equal(others, []bool{false, false, false, false, false, true}), "isOther flags")
	fin := w.Section("finished")
	expect(t, fin != nil && equal(itemDays(fin), []string{"2026-10-05", "2026-10-06"}) && fin.Items[0].Text == "Shipped A\nand B", "finished")
	expect(t, fin.Items[1].Text == "Shipped C\n\n**Details**\nten words here to describe what was shipped for the customer demo", "nested heading stays with its owner")
	expect(t, equal(itemTexts(w.Section("⏳ Pending / blocked")), []string{"Waiting on legal"}), "empty Monday section is left out")
	expect(t, equal(itemTexts(w.Section("to do next")), []string{"- [ ] Plan Q4"}) && w.Section("Started").Items[0].Text == "Docs", "other sections")
	other := w.Section("Other notes")
	expect(t, other.IsOther && equal(itemDays(other), []string{"2026-10-06", "2026-10-08", "2026-10-09"}), "other notes days")
	expect(t, other.Items[0].Text == "**Standup notes**\nQuick sync about launch dates and owners for the week ahead plans.", "bold label for a heading not in the template")
	expect(t, strings.HasPrefix(other.Items[1].Text, "Spent the whole day") && other.Items[2].Text == "Short note only", "other texts")
	expect(t, w.Section("Nope") == nil, "no such section")
	// a smaller template: everything else goes to Other notes (with its heading kept as a bold label)
	tf := "## Finished"
	w2, _ := weekFixture(t, 20, &tf)
	expect(t, equal(sectionTitles(w2), []string{"Finished", "Other notes"}), "%v", sectionTitles(w2))
	expect(t, strings.HasPrefix(w2.Section("other notes").Items[0].Text, "**What I did**\nPlanned the week"), "label kept")
	empty := ""
	w3, _ := weekFixture(t, 20, &empty)
	expect(t, equal(sectionTitles(w3), []string{"Other notes"}) && len(w3.Sections[0].Items) == 4, "no template: one group, one item per page with content")
	w4, _ := weekFixture(t, 3, nil)
	expect(t, w4.LoggedCount == 4, "minWords drives logged: Friday's 3 words now count")
	expect(t, ShiftWeek(noon(2026, 10, 7), -1, ny).Equal(noon(2026, 9, 30)), "shift keeps the time of day")
}

func TestWeeklyReviewMarkdownDayAndSectionGrouping(t *testing.T) {
	w, _ := weekFixture(t, 20, nil)
	day := WeekMarkdown(w, WeekByDay, ny)
	expect(t, strings.HasPrefix(day, "# Week of 5 Oct – 11 Oct 2026\n\nLogged 3 of 5 workdays · 1 skipped\n"), "%q", day)
	expect(t, strings.Contains(day, "## Mon 5 Oct\n\n### What I did\nPlanned the week"), "Monday heading")
	monBlock := day[strings.Index(day, "## Mon 5 Oct"):strings.Index(day, "## Tue 6 Oct")]
	expect(t, strings.Contains(monBlock, "### Started\nDocs\n\n### To do next\n- [ ] Plan Q4") && !strings.Contains(monBlock, "Pending / blocked"), "empty sections are pruned")
	expect(t, strings.Contains(day, "## Tue 6 Oct\n\n### Standup notes\nQuick sync") && strings.Contains(day, "#### Details\nten words here"), "Tuesday")
	expect(t, strings.Contains(day, "## Wed 7 Oct\n_Skipped: Holiday_"), "skipped day")
	expect(t, strings.Contains(day, "## Thu 8 Oct\n\nSpent the whole day") && strings.Contains(day, "## Fri 9 Oct\n\nShort note only"), "Thu and Fri")
	expect(t, !strings.Contains(day, "Sat 10 Oct") && !strings.Contains(day, "outside") && !strings.Contains(day, "30 Sep"), "nothing outside the week")
	sec := WeekMarkdown(w, WeekBySection, ny)
	expect(t, strings.Contains(sec, "## Finished\n\n- **Mon 5 Oct**: Shipped A\n  and B\n- **Tue 6 Oct**: Shipped C\n\n  **Details**\n  ten words here"), "%q", sec)
	expect(t, strings.Contains(sec, "## To do next\n\n- **Mon 5 Oct**\n  - [ ] Plan Q4"), "list items get the label on its own line")
	expect(t, strings.Contains(sec, "## Other notes\n\n- **Tue 6 Oct**\n  **Standup notes**\n  Quick sync"), "other notes")
	expect(t, strings.Contains(sec, "- **Thu 8 Oct**: Spent the whole day") && strings.Contains(sec, "- **Fri 9 Oct**: Short note only"), "plain entries")
	expect(t, !strings.Contains(sec, "Skipped") && strings.HasSuffix(sec, "\n") && !strings.HasSuffix(sec, "\n\n"), "no skipped days, one trailing newline")
	emptyW := SummarizeWeek(noon(2026, 10, 7), map[string]DayDocument{}, map[string]DayStatus{}, defaults(), ny, noon(2026, 10, 7))
	for _, g := range []WeekGrouping{WeekByDay, WeekBySection} {
		em := WeekMarkdown(emptyW, g, ny)
		expect(t, em == "# Week of 5 Oct – 11 Oct 2026\n\nLogged 0 of 3 workdays\n", "%q", em)
	}
}

// weekSummary is the Swift weekSummary() helper of the C5 tests.
func weekSummary(date time.Time, st map[string]DayStatus, now time.Time, cal Calendar, logStart string) WeekSummary {
	s := defaults()
	s.LogStartDate = logStart
	s.Weekdays = workdays
	return SummarizeWeek(date, map[string]DayDocument{}, st, s, cal, now)
}

func weekStatuses(w WeekSummary) []DayStatus {
	var out []DayStatus
	for _, d := range w.Days {
		out = append(out, d.Status)
	}
	return out
}

func TestWeekWithNoFilesListsEveryScheduledDayMissing(t *testing.T) {
	history := states("2026-09-28", StatusLogged, "2026-09-29", StatusLogged) // pages exist, but none in the week under review
	w := weekSummary(noon(2026, 10, 7), history, noon(2026, 10, 14), ny, "")
	expect(t, w.WeekStart == "2026-10-05" && w.WeekEnd == "2026-10-11" && len(w.Days) == 7, "week range")
	for _, d := range w.Days {
		if workdays.Has(DayKeyWeekday(d.Day, nyLoc)) {
			expect(t, d.Status == StatusMissed && d.Doc == nil, "%s should be missed with no page", d.Day)
		} else {
			expect(t, d.Status == StatusOff, "%s weekend", d.Day)
		}
	}
	expect(t, equal(w.MissingDays, []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"}), "%v", w.MissingDays)
	expect(t, w.LoggedCount == 0 && w.WorkdayCount == 5 && w.SkippedCount == 0 && len(w.Sections) == 0, "counts")
	// with no page anywhere and no log start nothing is "missing" (a new user), although the statuses keep ResolveStatus's rule
	fresh := weekSummary(noon(2026, 10, 7), map[string]DayStatus{}, noon(2026, 10, 14), ny, "")
	expect(t, len(fresh.MissingDays) == 0 && len(fresh.Days) == 7, "no history, no start: nothing to catch up")
	// a week before the first page: nothing is missed, nothing is missing
	early := weekSummary(noon(2026, 9, 9), history, noon(2026, 10, 14), ny, "")
	allOff := true
	for _, s := range weekStatuses(early) {
		allOff = allOff && s == StatusOff
	}
	expect(t, len(early.Days) == 7 && allOff && len(early.MissingDays) == 0, "%v", weekStatuses(early))
	// the log start date from Settings mutes the days before it
	cut := weekSummary(noon(2026, 10, 7), history, noon(2026, 10, 14), ny, "2026-10-07")
	expect(t, equal(weekStatuses(cut), []DayStatus{StatusOff, StatusOff, StatusMissed, StatusMissed, StatusMissed, StatusOff, StatusOff}), "%v", weekStatuses(cut))
	expect(t, equal(cut.MissingDays, []string{"2026-10-07", "2026-10-08", "2026-10-09"}), "%v", cut.MissingDays)
	// a written page and a skip marker are not missing; a started page is
	mixed := states("2026-09-28", StatusLogged, "2026-09-29", StatusLogged, "2026-10-05", StatusLogged, "2026-10-06", StatusSkipped, "2026-10-07", StatusPartial)
	mw := weekSummary(noon(2026, 10, 7), mixed, noon(2026, 10, 14), ny, "")
	expect(t, equal(mw.MissingDays, []string{"2026-10-07", "2026-10-08", "2026-10-09"}) && mw.LoggedCount == 1 && mw.SkippedCount == 1, "%v", mw.MissingDays)
}

func TestWeeksStartOnCalendarFirstWeekday(t *testing.T) {
	sun := calIn("America/New_York", 1, "")
	w := weekSummary(noon(2026, 10, 7), states("2026-09-28", StatusLogged), noon(2026, 10, 14), sun, "")
	expect(t, w.WeekStart == "2026-10-04" && w.WeekEnd == "2026-10-10" && len(w.Days) == 7, "Sunday-first: %s...%s", w.WeekStart, w.WeekEnd)
	expect(t, equal(weekStatuses(w), []DayStatus{StatusOff, StatusMissed, StatusMissed, StatusMissed, StatusMissed, StatusMissed, StatusOff}), "%v", weekStatuses(w))
	expect(t, equal(w.MissingDays, []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"}) && w.WorkdayCount == 5, "missing days")
	// a Sunday belongs to the week before it on Monday-first calendars and starts its own week on Sunday-first ones
	monday := weekSummary(noon(2026, 10, 11), map[string]DayStatus{}, noon(2026, 10, 14), ny, "")
	sunday := weekSummary(noon(2026, 10, 11), map[string]DayStatus{}, noon(2026, 10, 14), sun, "")
	expect(t, monday.WeekStart == "2026-10-05" && monday.WeekEnd == "2026-10-11", "%s", monday.WeekStart)
	expect(t, sunday.WeekStart == "2026-10-11" && sunday.WeekEnd == "2026-10-17", "%s", sunday.WeekStart)
	// Saturday-first, to show it is the calendar and not a Monday/Sunday special case
	sat := weekSummary(noon(2026, 10, 7), map[string]DayStatus{}, noon(2026, 10, 14), calIn("America/New_York", 7, ""), "")
	expect(t, sat.WeekStart == "2026-10-03" && sat.WeekEnd == "2026-10-09", "%s", sat.WeekStart)
}

func TestCurrentWeekListsFutureDaysAndMissingDaysNeverHoldTodayOrTheFuture(t *testing.T) {
	st := states("2026-09-30", StatusLogged, "2026-10-05", StatusLogged)
	wed7 := noon(2026, 10, 7)
	w := weekSummary(wed7, st, wed7, ny, "") // Wed 7 Oct is today, nothing written yet
	expect(t, equal(weekStatuses(w), []DayStatus{StatusLogged, StatusMissed, StatusMissed, StatusFuture, StatusFuture, StatusFuture, StatusFuture}), "%v", weekStatuses(w))
	expect(t, len(w.Days) == 7, "every day of the week is present")
	expect(t, equal(w.MissingDays, []string{"2026-10-06"}), "Tuesday only: %v", w.MissingDays)
	expect(t, w.WorkdayCount == 3 && w.LoggedCount == 1, "scheduled days not in the future: %d", w.WorkdayCount)
	// once today has a page it is logged, still not missing; a started page today is not listed either
	st2 := states("2026-09-30", StatusLogged, "2026-10-05", StatusLogged, "2026-10-07", StatusPartial)
	w2 := weekSummary(wed7, st2, wed7, ny, "")
	expect(t, equal(w2.MissingDays, []string{"2026-10-06"}) && w2.Days[2].Status == StatusPartial, "partial today")
	// next week: all future
	next := weekSummary(noon(2026, 10, 14), st, wed7, ny, "")
	allFuture := true
	for _, s := range weekStatuses(next) {
		allFuture = allFuture && s == StatusFuture
	}
	expect(t, len(next.Days) == 7 && allFuture && len(next.MissingDays) == 0 && next.WorkdayCount == 0, "next week")
	// Monday morning: the week is only today
	mon := weekSummary(noon(2026, 10, 5), states("2026-09-30", StatusLogged), mk(2026, 10, 5, 9, 0), ny, "")
	expect(t, equal(weekStatuses(mon), []DayStatus{StatusMissed, StatusFuture, StatusFuture, StatusFuture, StatusFuture, StatusFuture, StatusFuture}) && len(mon.MissingDays) == 0, "%v", weekStatuses(mon))
}

func TestWeekMissingDaysIsCatchUpMissingRestrictedToTheWeek(t *testing.T) {
	now := noon(2026, 10, 21) // Wed 21 Oct
	st := states("2026-09-21", StatusLogged, "2026-09-22", StatusPartial, "2026-09-23", StatusSkipped, "2026-09-30", StatusLogged, "2026-10-01", StatusPartial,
		"2026-10-02", StatusSkipped, "2026-10-06", StatusLogged, "2026-10-07", StatusLogged, "2026-10-12", StatusPartial, "2026-10-17", StatusPartial,
		"2026-10-20", StatusSkipped)
	for _, logStart := range []string{"", "2026-09-21", "2026-09-30", "2026-10-07", "2026-10-15"} {
		all := CatchUpMissing(st, now, ny, workdays, logStart, 365)
		for weekOffset := -5; weekOffset <= 0; weekOffset++ {
			date := ShiftWeek(now, weekOffset, ny)
			w := weekSummary(date, st, now, ny, logStart)
			in := map[string]bool{}
			for _, d := range w.Days {
				in[d.Day] = true
			}
			var want []string
			for _, d := range all {
				if in[d] {
					want = append(want, d)
				}
			}
			expect(t, strs(w.MissingDays, want), "logStart %q week %s: %v vs %v", logStart, w.WeekStart, w.MissingDays, want)
		}
	}
}

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
