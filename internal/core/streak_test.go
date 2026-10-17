package core

// Ported from tests/main.swift: "streak + heatmap", "DST", "log start date (M1)", "fresh install".

import "testing"

func TestStreakAcrossWeekendSkipAndMiss(t *testing.T) {
	mon12 := mk(2026, 10, 12, 9, 0) // Monday, nothing logged yet today
	l := StatusLogged
	s := states("2026-10-05", l, "2026-10-06", l, "2026-10-07", l, "2026-10-08", l, "2026-10-09", l)
	cur := func(wd WeekdaySet) StreakResult { return ComputeStreak(s, mon12, ny, wd, "") }
	expect(t, cur(workdays) == StreakResult{5, 5}, "weekend neutral, today not yet: %+v", cur(workdays))
	s["2026-10-12"] = l
	expect(t, cur(workdays).Current == 6, "today logged +1")
	delete(s, "2026-10-12")
	s["2026-10-07"] = StatusSkipped
	expect(t, cur(workdays).Current == 4, "skip neutral")
	delete(s, "2026-10-07")
	expect(t, cur(workdays) == StreakResult{2, 2}, "miss breaks: %+v", cur(workdays))
	s["2026-10-07"] = StatusPartial
	expect(t, cur(workdays).Current == 2, "partial past day breaks")
	s["2026-10-07"], s["2026-10-10"] = l, l // Saturday extra effort
	expect(t, cur(workdays).Current == 6, "non-scheduled log +1")
	// today is Tuesday unlogged, Monday missed -> broken
	delete(s, "2026-10-12")
	expect(t, ComputeStreak(s, mk(2026, 10, 13, 9, 0), ny, workdays, "").Current == 0, "yesterday missed")
	expect(t, ComputeStreak(map[string]DayStatus{}, mon12, ny, workdays, "") == StreakResult{}, "no files")
	// schedule change re-evaluates: Friday off
	noFri := WeekdaySet{2, 3, 4, 5}
	tt := states("2026-10-05", l, "2026-10-06", l, "2026-10-07", l, "2026-10-08", l)
	expect(t, ComputeStreak(tt, mon12, ny, noFri, "").Current == 4, "Friday off")
	expect(t, ComputeStreak(tt, mon12, ny, workdays, "").Current == 0, "Fri 9 missed under Mon-Fri")
	// best survives a later break
	b := states("2026-10-05", l, "2026-10-06", l, "2026-10-07", l, "2026-10-09", l)
	expect(t, ComputeStreak(b, mon12, ny, workdays, "") == StreakResult{1, 3}, "best survives a later break")
}

func TestHeatmap12WeeksStatusesTodayRing(t *testing.T) {
	now := mk(2026, 10, 7, 10, 0) // Wed
	st := states("2026-10-05", StatusLogged, "2026-10-06", StatusSkipped, "2026-09-30", StatusPartial)
	hm := HeatmapWeeks(st, now, ny, workdays, 12, "")
	expect(t, len(hm) == 12, "12 weeks")
	for _, c := range hm {
		expect(t, len(c) == 7, "7 days a week")
	}
	last := hm[len(hm)-1]
	var days []string
	var sts []DayStatus
	var today []bool
	for _, c := range last {
		days = append(days, c.Day)
		sts = append(sts, c.Status)
		today = append(today, c.IsToday)
	}
	expect(t, equal(days, []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11"}), "week starts Monday: %v", days)
	expect(t, equal(sts, []DayStatus{StatusLogged, StatusSkipped, StatusMissed, StatusFuture, StatusFuture, StatusFuture, StatusFuture}), "statuses %v", sts)
	expect(t, equal(today, []bool{false, false, true, false, false, false, false}), "today ring")
	expect(t, hm[10][2].Status == StatusPartial && hm[10][5].Status == StatusOff, "Saturday off")
	expect(t, hm[0][0].Status == StatusOff, "before first entry is not 'missed'")
	expect(t, hm[10][3].Status == StatusMissed, "after first entry, empty workday is missed")
}

func TestDSTDayKeysHeatmapAndStreakAcrossFallBackAndSpringForward(t *testing.T) {
	// Fall back 2026-11-01 (25h day), spring forward 2026-03-08 (23h day) in New York.
	expect(t, DayKeyString(mk(2026, 11, 1, 23, 30), nyLoc) == "2026-11-01", "late on the long day")
	add := func(k string, n int) string { r, _ := DayKeyAdding(k, n, nyLoc); return r }
	expect(t, add("2026-11-01", 1) == "2026-11-02" && add("2026-03-08", 1) == "2026-03-09", "forward")
	expect(t, add("2026-11-02", -1) == "2026-11-01" && add("2026-03-09", -1) == "2026-03-08", "backward")
	_, bad := DayKeyDate("2026-02-30", nyLoc)
	_, good := DayKeyDate("2026-03-08", nyLoc)
	expect(t, !bad && good, "impossible and real dates")
	for _, now := range []struct{ y, m, d int }{{2026, 11, 3}, {2026, 3, 10}} {
		hm := HeatmapWeeks(states("2026-10-01", StatusLogged), mk(now.y, now.m, now.d, 10, 0), ny, workdays, 12, "")
		var flat []HeatCell
		for _, c := range hm {
			flat = append(flat, c...)
		}
		seen := map[string]bool{}
		for _, c := range flat {
			seen[c.Day] = true
		}
		expect(t, len(flat) == 84 && len(seen) == 84, "unique days")
		ok := true
		for i := 1; i < len(flat); i++ {
			if add(flat[i-1].Day, 1) != flat[i].Day {
				ok = false
			}
		}
		expect(t, ok, "consecutive across DST")
	}
	every := WeekdaySet{1, 2, 3, 4, 5, 6, 7}
	s := states("2026-10-30", StatusLogged, "2026-10-31", StatusLogged, "2026-11-01", StatusLogged, "2026-11-02", StatusLogged)
	expect(t, ComputeStreak(s, mk(2026, 11, 2, 18, 0), ny, every, "").Current == 4, "streak over the 25h day")
	s2 := states("2026-03-07", StatusLogged, "2026-03-08", StatusLogged, "2026-03-09", StatusLogged)
	expect(t, ComputeStreak(s2, mk(2026, 3, 9, 18, 0), ny, every, "").Current == 3, "streak over the 23h day")
	// reminder uses wall-clock minutes on the DST day
	st := DefaultSettings("/home/x")
	st.Weekdays = every
	r, _ := ReminderDecide(mk(2026, 11, 1, 17, 0), ny, st, StatusMissed, PlannerState{Day: "2026-11-01"}, true)
	expect(t, r == ActionBringToFront(true), "17:00 wall clock on fall-back day")
}

func TestLogStartBackfillingCreatesNoWallOfMissedDays(t *testing.T) {
	now := mk(2026, 10, 7, 12, 0) // Wednesday
	st := states("2026-10-05", StatusLogged, "2026-10-06", StatusLogged)
	before := ComputeStreak(st, now, ny, workdays, "2026-10-01")
	expect(t, before == StreakResult{2, 2}, "%+v", before)
	st["2026-09-14"] = StatusLogged // backfill a Monday weeks earlier
	// without a pinned start the earliest file moves back and 15 Sep - 2 Oct become "missed"
	unpinned := EffectiveSince("", st)
	expect(t, unpinned == "2026-09-14", "earliest file")
	expect(t, ResolveStatus("2026-09-15", "", now, ny, workdays, unpinned) == StatusMissed, "the wall of missed days")
	// with the start pinned they are off, and the streak is exactly what it was
	since := EffectiveSince("2026-10-01", st)
	expect(t, since == "2026-10-01", "pinned")
	expect(t, ResolveStatus("2026-09-15", "", now, ny, workdays, since) == StatusOff, "off")
	expect(t, ResolveStatus("2026-09-14", StatusLogged, now, ny, workdays, since) == StatusLogged, "the backfilled page itself still shows as logged")
	expect(t, ComputeStreak(st, now, ny, workdays, since) == before, "streak unchanged")
	var flat []HeatCell
	for _, c := range HeatmapWeeks(st, now, ny, workdays, 12, "2026-10-01") {
		flat = append(flat, c...)
	}
	find := func(day string) DayStatus {
		for _, c := range flat {
			if c.Day == day {
				return c.Status
			}
		}
		return ""
	}
	expect(t, find("2026-09-15") == StatusOff && find("2026-09-14") == StatusLogged, "heatmap")
}

func TestLogStartNilKeepsTheOldRuleAStartAfterTodayCountsNothing(t *testing.T) {
	now := mk(2026, 10, 7, 12, 0)
	st := states("2026-10-05", StatusLogged, "2026-10-06", StatusLogged)
	expect(t, ComputeStreak(st, now, ny, workdays, "") == ComputeStreak(st, now, ny, workdays, ""), "same")
	expect(t, ComputeStreak(st, now, ny, workdays, "2026-10-20") == StreakResult{}, "a start after today counts nothing")
	expect(t, EffectiveSince("", st) == "2026-10-05" && EffectiveSince("", map[string]DayStatus{}) == "", "effective since")
	expect(t, ComputeStreak(st, now, ny, workdays, "").Current == 2, "old rule: from the earliest file")
}

func TestFreshInstallPastDaysAreNotMissed(t *testing.T) {
	now := mk(2026, 10, 7, 12, 0) // Wednesday
	for _, day := range []string{"2026-09-30", "2026-10-05", "2026-10-06"} {
		expect(t, ResolveStatus(day, "", now, ny, workdays, "") == StatusOff, "%s", day)
	}
	expect(t, ResolveStatus("2026-10-07", "", now, ny, workdays, "") == StatusMissed, "today is still due")
	expect(t, ResolveStatus("2026-10-08", "", now, ny, workdays, "") == StatusFuture, "future")
	var missed []string
	for _, col := range HeatmapWeeks(map[string]DayStatus{}, now, ny, workdays, 12, "") {
		for _, c := range col {
			if c.Status == StatusMissed {
				missed = append(missed, c.Day)
			}
		}
	}
	expect(t, equal(missed, []string{"2026-10-07"}), "heatmap %v", missed)
	var grid []string
	for _, row := range MonthGridRows(2026, 10, map[string]DayStatus{}, now, ny, workdays, "") {
		for _, c := range row {
			if c.Status == StatusMissed {
				grid = append(grid, c.Day)
			}
		}
	}
	expect(t, equal(grid, []string{"2026-10-07"}), "grid %v", grid)
}
