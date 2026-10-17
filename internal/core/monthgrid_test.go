package core

// Ported from tests/main.swift: "C2 MonthGrid".

import (
	"testing"
	"time"
)

func grid(y, m int, st map[string]DayStatus) [][]MonthCell {
	return MonthGridRows(y, m, st, noon(2026, 10, 7), ny, workdays, "")
}

func flatCells(g [][]MonthCell) []MonthCell {
	var out []MonthCell
	for _, r := range g {
		out = append(out, r...)
	}
	return out
}

func gridCell(g [][]MonthCell, day string) *MonthCell {
	for _, c := range flatCells(g) {
		if c.Day == day {
			c := c
			return &c
		}
	}
	return nil
}

func cellStatus(g [][]MonthCell, day string) DayStatus {
	if c := gridCell(g, day); c != nil {
		return c.Status
	}
	return "?"
}

func TestMonthGridShapeAndPositionMondayFirst(t *testing.T) {
	g := grid(2026, 10, nil)
	ok := len(g) == 6
	for _, r := range g {
		ok = ok && len(r) == 7
	}
	expect(t, ok, "6 rows x 7 columns")
	expect(t, g[0][0].Day == "2026-09-28" && !g[0][0].InMonth, "first cell is the Monday on or before the 1st")
	expect(t, g[0][3].Day == "2026-10-01" && g[0][3].InMonth, "1 Oct is a Thursday: row 0, column 3")
	expect(t, g[5][6].Day == "2026-11-08" && !g[5][6].InMonth, "last cell, 41 days after the first")
	var in []string
	for _, c := range flatCells(g) {
		if c.InMonth {
			in = append(in, c.Day)
		}
	}
	expect(t, len(in) == 31 && in[0] == "2026-10-01" && in[30] == "2026-10-31", "%d in-month cells", len(in))
	for _, c := range flatCells(g) {
		if !c.InMonth {
			expect(t, c.Day < "2026-10-01" || c.Day > "2026-10-31", "%s is outside", c.Day)
		}
	}
	for _, row := range g {
		for c, x := range row {
			expect(t, DayKeyWeekday(x.Day, nyLoc) == (ny.FirstWeekday-1+c)%7+1, "column %d of %s has the wrong weekday", c, x.Day)
		}
	}
}

func TestMonthGridSundayFirstYearBoundariesShortMonths(t *testing.T) {
	sun := calIn("America/New_York", 1, "")
	g := MonthGridRows(2026, 10, nil, noon(2026, 10, 7), sun, workdays, "")
	expect(t, g[0][0].Day == "2026-09-27" && !g[0][0].InMonth, "Sunday-first: first cell is Sunday 27 Sep")
	expect(t, g[0][4].Day == "2026-10-01", "1 Oct is column 4 when weeks start on Sunday")
	for _, row := range g {
		for c, x := range row {
			expect(t, DayKeyWeekday(x.Day, nyLoc) == c+1, "Sunday-first column %d = %s", c, x.Day)
		}
	}
	feb := grid(2027, 2, nil)
	expect(t, len(feb) == 6 && feb[0][0].Day == "2027-02-01" && feb[0][0].InMonth, "Feb 2027 starts in column 0")
	expect(t, feb[4][0].Day == "2027-03-01" && !feb[4][0].InMonth && feb[5][6].Day == "2027-03-14", "rows 5 and 6 are March")
	dec := grid(2026, 12, nil)
	expect(t, dec[0][0].Day == "2026-11-30" && dec[5][6].Day == "2027-01-10", "December 2026 runs into January 2027")
	jan := grid(2027, 1, nil)
	expect(t, jan[0][0].Day == "2026-12-28" && jan[0][4].Day == "2027-01-01" && jan[0][4].InMonth, "January 2027 starts in December 2026")
}

func TestMonthGridLeapFebruary(t *testing.T) {
	count := func(y int, c Calendar) int {
		n := 0
		for _, x := range flatCells(MonthGridRows(y, 2, nil, noon(2026, 10, 7), c, workdays, "")) {
			if x.InMonth {
				n++
			}
		}
		return n
	}
	for _, y := range []int{2028, 2024, 2000} {
		expect(t, count(y, ny) == 29, "leap year %d", y)
	}
	for _, y := range []int{2026, 2027, 2100} {
		expect(t, count(y, ny) == 28, "ordinary year %d (2100 is not a leap year)", y)
	}
	expect(t, count(2028, calIn("America/New_York", 1, "")) == 29, "Sunday-first")
	g := grid(2028, 2, nil)
	expect(t, gridCell(g, "2028-02-29").InMonth && !gridCell(g, "2028-03-01").InMonth, "29 Feb is in, 1 Mar is out")
	g26 := grid(2026, 2, nil)
	expect(t, gridCell(g26, "2026-02-28").InMonth && !gridCell(g26, "2026-03-01").InMonth, "2026")
}

func TestMonthGrid42UniqueConsecutiveDaysInDSTMonthsAndZones(t *testing.T) {
	for _, ym := range [][2]int{{2026, 3}, {2026, 11}, {2027, 3}, {2027, 11}, {2028, 3}} {
		var keys []string
		for _, c := range flatCells(grid(ym[0], ym[1], nil)) {
			keys = append(keys, c.Day)
		}
		uniq := map[string]bool{}
		for _, k := range keys {
			uniq[k] = true
		}
		expect(t, len(keys) == 42 && len(uniq) == 42, "%v: 42 unique keys, got %d", ym, len(uniq))
		for i := 1; i < len(keys); i++ {
			n, _ := DayKeyAdding(keys[i-1], 1, nyLoc)
			expect(t, n == keys[i], "%v: consecutive at %d", ym, i)
		}
	}
	cases := []struct {
		tz   string
		year int
	}{{"America/New_York", 2026}, {"America/Sao_Paulo", 2018}, {"Europe/London", 2026}, {"Asia/Kolkata", 2026}, {"Pacific/Auckland", 2026},
		{"Australia/Lord_Howe", 2026}, {"America/Santiago", 2026}, {"Asia/Beirut", 2026}, {"America/Havana", 2026}}
	var bad []string
	checked := 0
	for _, c := range cases {
		for _, fw := range []int{1, 2} {
			cal := calIn(c.tz, fw, "")
			for m := 1; m <= 12; m++ {
				checked++
				g := MonthGridRows(c.year, m, nil, mk(c.year, 6, 15, 12, 0), cal, workdays, "")
				first := utcDate(c.year, m, 1)
				lead := (int(first.Weekday()) + 1 - fw + 7) % 7
				cells := flatCells(g)
				if len(g) != 6 || len(cells) != 42 {
					bad = append(bad, c.tz+" shape")
					continue
				}
				inMonth := 0
				for i, x := range cells {
					if x.Day != utcKey(first.AddDate(0, 0, i-lead)) {
						bad = append(bad, c.tz+" "+x.Day)
						break
					}
					if x.InMonth {
						inMonth++
					}
				}
				if want := time.Date(c.year, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day(); inMonth != want {
					bad = append(bad, c.tz+" in-month")
				}
			}
		}
	}
	expect(t, len(bad) == 0 && checked == len(cases)*2*12, "wrong grids: %v, checked %d", bad, checked)
}

func TestMonthGridStatusesComeFromResolveStatus(t *testing.T) {
	st := states("2026-09-30", StatusLogged, "2026-10-02", StatusSkipped, "2026-10-05", StatusLogged, "2026-10-06", StatusPartial)
	g := grid(2026, 10, st) // now = Wed 7 Oct 2026
	s := func(d string) DayStatus { return cellStatus(g, d) }
	expect(t, s("2026-10-05") == StatusLogged && s("2026-10-06") == StatusPartial && s("2026-10-02") == StatusSkipped, "file states map through")
	expect(t, s("2026-09-30") == StatusLogged, "a leading out-of-month cell keeps its state")
	expect(t, s("2026-10-01") == StatusMissed, "scheduled, unwritten, after the first page = missed")
	expect(t, s("2026-10-07") == StatusMissed && gridCell(g, "2026-10-07").IsToday, "today unlogged is missed with the today flag")
	expect(t, s("2026-10-08") == StatusFuture && s("2026-10-31") == StatusFuture && s("2026-11-08") == StatusFuture, "after today is future")
	expect(t, s("2026-10-10") == StatusFuture, "a future Saturday is future, not off")
	expect(t, s("2026-10-03") == StatusOff && s("2026-10-04") == StatusOff, "past weekend, nothing written = off")
	todays, skipped, partial := 0, 0, 0
	for _, c := range flatCells(g) {
		if c.IsToday {
			todays++
		}
		if c.Status == StatusSkipped {
			skipped++
		}
		if c.Status == StatusPartial {
			partial++
		}
	}
	expect(t, todays == 1 && skipped == 1 && partial == 1, "one today, one skipped, one partial")
	sep := MonthGridRows(2026, 9, nil, mk(2026, 10, 1, 9, 0), ny, workdays, "")
	expect(t, gridCell(sep, "2026-10-01").IsToday && !gridCell(sep, "2026-10-01").InMonth, "today as a trailing cell")
	later := MonthGridRows(2026, 11, nil, noon(2026, 10, 7), ny, workdays, "")
	for _, c := range flatCells(later) {
		expect(t, c.Status == StatusFuture && !c.IsToday, "a month entirely after today: %s", c.Day)
	}
	earlier := MonthGridRows(2026, 10, st, noon(2026, 11, 20), ny, workdays, "")
	for _, c := range flatCells(earlier) {
		expect(t, c.Status != StatusFuture && !c.IsToday, "a month entirely before today: %s", c.Day)
	}
	expect(t, cellStatus(earlier, "2026-10-30") == StatusMissed && cellStatus(earlier, "2026-10-31") == StatusOff, "end of the month")
}

func TestMonthGridIsScheduledFollowsWorkingWeekdays(t *testing.T) {
	g := grid(2026, 10, states("2026-09-30", StatusLogged, "2026-10-03", StatusLogged))
	expect(t, gridCell(g, "2026-10-05").IsScheduled && gridCell(g, "2026-10-09").IsScheduled, "Mon and Fri are scheduled")
	expect(t, !gridCell(g, "2026-10-03").IsScheduled && !gridCell(g, "2026-10-04").IsScheduled, "Sat and Sun are not")
	expect(t, cellStatus(g, "2026-10-03") == StatusLogged, "a log on a day off still shows as logged")
	expect(t, cellStatus(g, "2026-10-04") == StatusOff, "an unwritten day off is off, never missed")
	expect(t, !gridCell(g, "2026-10-31").IsScheduled, "isScheduled does not depend on the future")
	weekend := MonthGridRows(2026, 10, states("2026-09-30", StatusLogged), noon(2026, 10, 7), ny, WeekdaySet{1, 7}, "")
	expect(t, gridCell(weekend, "2026-10-03").IsScheduled && !gridCell(weekend, "2026-10-05").IsScheduled, "weekend schedule")
	expect(t, cellStatus(weekend, "2026-10-03") == StatusMissed && cellStatus(weekend, "2026-10-05") == StatusOff, "a weekend schedule flips missed and off")
}

func TestMonthGridLogStartCutsMissedDaysToOff(t *testing.T) {
	st := states("2026-09-14", StatusLogged, "2026-10-05", StatusLogged)
	g := MonthGridRows(2026, 9, st, noon(2026, 10, 7), ny, workdays, "2026-10-01")
	expect(t, cellStatus(g, "2026-09-14") == StatusLogged, "the backfilled page is logged")
	expect(t, cellStatus(g, "2026-09-15") == StatusOff && cellStatus(g, "2026-09-30") == StatusOff, "13 to 30 Sep are off, not missed")
	expect(t, gridCell(g, "2026-09-15").IsScheduled, "off because of the start date, not because it is a day off")
	expect(t, cellStatus(g, "2026-10-01") == StatusMissed, "the log start day itself counts again")
	wall := MonthGridRows(2026, 9, st, noon(2026, 10, 7), ny, workdays, "")
	expect(t, cellStatus(wall, "2026-09-15") == StatusMissed && cellStatus(wall, "2026-09-14") == StatusLogged, "nil start = earliest file")
	late := MonthGridRows(2026, 10, states("2026-10-05", StatusLogged), noon(2026, 10, 7), ny, workdays, "2026-10-07")
	expect(t, cellStatus(late, "2026-10-06") == StatusOff && cellStatus(late, "2026-10-05") == StatusLogged && cellStatus(late, "2026-10-07") == StatusMissed, "late start")
	fut := MonthGridRows(2026, 10, nil, noon(2026, 10, 7), ny, workdays, "2026-10-20")
	for _, c := range flatCells(fut) {
		if c.Day <= "2026-10-07" {
			expect(t, c.Status == StatusOff, "start after today: %s is off", c.Day)
		}
	}
	expect(t, cellStatus(grid(2026, 10, nil), "2026-10-06") == StatusOff, "brand-new install")
}

func TestMonthGridInvalidMonthGivesNoRows(t *testing.T) {
	expect(t, len(grid(2026, 0, nil)) == 0 && len(grid(2026, 13, nil)) == 0 && len(grid(2026, -1, nil)) == 0, "invalid months")
	expect(t, len(grid(2026, 1, nil)) > 0 && len(grid(2026, 12, nil)) > 0, "valid months")
}
