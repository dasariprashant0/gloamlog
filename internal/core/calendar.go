// calendar.go - the little bit of Foundation's Calendar that the Swift core leaned on, as plain Go.
//
// Swift passed a `Calendar` (time zone + first weekday + locale) into every date function. Here that is the Calendar struct below.
// Nothing in this package reads the clock: "now" is always a parameter, so tests are deterministic.
//
// Foundation semantics kept on purpose:
//   - weekdays are numbered 1 = Sunday ... 7 = Saturday (Go's time.Weekday is 0 = Sunday, so add 1);
//   - a "day" is a civil date in the calendar's time zone, and its start is local midnight, or the first instant that exists
//     when DST skips midnight (Beirut, Havana, Santiago, Sao Paulo before 2019);
//   - days are stepped by calendar date, never by adding 86,400 seconds, so a 23 h or 25 h day neither repeats nor skips.
package core

import "time"

// Calendar is what Foundation's Calendar carried: a time zone, the weekday a week starts on, and a locale.
type Calendar struct {
	Loc          *time.Location // the zone every day is read in (nil = UTC)
	FirstWeekday int            // 1 = Sunday ... 7 = Saturday; 0 behaves like 1 (Foundation's gregorian default)
	Locale       string         // "en_GB", "ja_JP", ...; "" = the root locale, which reads numeric dates like en_US (see DateJump)
}

// NewCalendar returns a Calendar for loc whose weeks start on firstWeekday (1 = Sunday ... 7 = Saturday).
func NewCalendar(loc *time.Location, firstWeekday int) Calendar {
	return Calendar{Loc: loc, FirstWeekday: firstWeekday}
}

func (c Calendar) location() *time.Location {
	if c.Loc == nil {
		return time.UTC
	}
	return c.Loc
}

func (c Calendar) firstWeekday() int {
	if c.FirstWeekday < 1 || c.FirstWeekday > 7 {
		return 1
	}
	return c.FirstWeekday
}

// startOfDay is the first instant of the civil date y-m-d in loc. Day numbers may overflow (d = 32 is the 1st of next month).
// If midnight does not exist that day (DST starts at 00:00), Go would hand back 23:00 of the day before, so walk forward to the
// first instant that is really on the requested date, like Foundation does.
func startOfDay(y int, m time.Month, d int, loc *time.Location) time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, loc)
	want := time.Date(y, m, d, 12, 0, 0, 0, time.UTC) // normalises the overflowing day/month once, in a zone without DST
	wy, wm, wd := want.Date()
	for i := 0; i < 8; i++ {
		ty, tm, td := t.Date()
		if ty == wy && tm == wm && td == wd {
			break
		}
		t = t.Add(30 * time.Minute)
	}
	return t
}

// dayStart is the start of the day that contains t, in loc.
func dayStart(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return startOfDay(y, m, d, loc)
}

// addDays steps a START-OF-DAY instant by n calendar days (Foundation's date(byAdding: .day)) and returns the start of that day.
func addDays(t time.Time, n int, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return startOfDay(y, m, d+n, loc)
}

// weekdayOf is the Calendar weekday number of t in loc: 1 = Sunday ... 7 = Saturday.
func weekdayOf(t time.Time, loc *time.Location) int {
	return int(t.In(loc).Weekday()) + 1
}

// StartOfWeek is the first instant of the week that contains t, the week starting on c.FirstWeekday
// (Foundation's dateInterval(of: .weekOfYear, for:).start).
func (c Calendar) StartOfWeek(t time.Time) time.Time {
	loc := c.location()
	lead := (weekdayOf(t, loc) - c.firstWeekday() + 7) % 7
	y, m, d := t.In(loc).Date()
	return startOfDay(y, m, d-lead, loc)
}

// ShiftWeeks moves t by n weeks keeping its time of day (Foundation's date(byAdding: .weekOfYear, value:, to:)).
func (c Calendar) ShiftWeeks(t time.Time, n int) time.Time {
	return t.In(c.location()).AddDate(0, 0, 7*n)
}
