// monthgrid.go - the month calendar's data. Pure. Port of app/Core/Calendar/MonthGrid.swift.
// CONTRACT:
//
//	MonthGridRows(...) -> 6 rows x 7 columns, columns start at cal.FirstWeekday; cells outside the month have InMonth == false.
//	status comes from ResolveStatus with since = EffectiveSince(logStart, states), so:
//	  future days are future, days before the effective log start are off (a page written there still shows its own state),
//	  unscheduled days with no page are off, scheduled past days with no page are missed.
//	Cells outside the month are real days and get the same status, scheduled and today flags as the others.
//	Always 6 rows, even for a month that needs 4 or 5, so the calendar never changes height. A month outside 1...12 gives nil.
//
// Days are stepped by calendar date from one anchor, never by adding 86,400 s, so DST days (23 h / 25 h, and
// zones where DST changes at midnight) neither repeat nor skip a day.
package core

import "time"

// MonthCell is one square of the month grid.
type MonthCell struct {
	Day         string    `json:"day"` // "yyyy-MM-dd"
	InMonth     bool      `json:"inMonth"`
	Status      DayStatus `json:"status"`
	IsToday     bool      `json:"isToday"`
	IsScheduled bool      `json:"isScheduled"` // the day's weekday is one of the user's working weekdays
}

const (
	MonthGridRowCount    = 6
	MonthGridColumnCount = 7
)

// MonthGridRows builds the 6x7 grid of a month. logStart "" = none.
func MonthGridRows(year, month int, states map[string]DayStatus, now time.Time, cal Calendar, weekdays WeekdaySet, logStart string) [][]MonthCell {
	if month < 1 || month > 12 {
		return nil
	}
	loc := cal.location()
	first := startOfDay(year, time.Month(month), 1, loc)
	lead := (weekdayOf(first, loc) - cal.firstWeekday() + 7) % 7
	anchor := addDays(first, -lead, loc)
	today := DayKeyString(now, loc)
	since := EffectiveSince(logStart, states)
	rows := make([][]MonthCell, 0, MonthGridRowCount)
	for r := 0; r < MonthGridRowCount; r++ {
		row := make([]MonthCell, 0, MonthGridColumnCount)
		for c := 0; c < MonthGridColumnCount; c++ {
			d := addDays(anchor, r*MonthGridColumnCount+c, loc)
			key := DayKeyString(d, loc)
			y, m, _ := d.In(loc).Date()
			row = append(row, MonthCell{
				Day:         key,
				InMonth:     y == year && int(m) == month,
				Status:      ResolveStatus(key, states[key], now, cal, weekdays, since),
				IsToday:     key == today,
				IsScheduled: weekdays.Has(weekdayOf(d, loc)),
			})
		}
		rows = append(rows, row)
	}
	return rows
}
