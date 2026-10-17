// datejump.go - "Go to date" text parser. Pure. Port of app/Core/Calendar/DateJump.swift.
// CONTRACT:
//
//	ParseDateJump("yesterday" | "today" | "-3" | "3 days ago" | "last fri" | "fri" | "2 oct" | "oct 2" | "2 October 2026" |
//	              "2026-10-02" | "10/2", now, cal)
//	-> a day key "yyyy-MM-dd" that is not after today, or ok == false when the text cannot be read or names a future day.
//	"last fri" = the most recent Friday strictly before today; "fri" = the most recent Friday on or before today.
//
// Grammar (case, surrounding and repeated whitespace, commas and trailing periods are ignored; English words only):
//
//	today | yesterday                      -n  (n days back, n >= 0)            n day(s) ago | n week(s) ago
//	<weekday> | last <weekday>             sun mon tue(s) wed(s) thu(r)(s) fri sat, or the full name
//	<d> <month> | <month> <d>              d = 1...31 with an optional st/nd/rd/th; month = jan..dec, sept, full names
//	<d> <month> <yyyy> | <month> <d> <yyyy>
//	yyyy-M-d (also yyyy/M/d, yyyy.M.d)     ISO order, never locale dependent
//	a/b, a/b/yy, a/b/yyyy (also - and .)   field ORDER comes from cal.Locale (en_US 10/2 = 2 Oct, en_GB 10/2 = 10 Feb,
//	                                       ja_JP y/M/d); an empty Locale behaves like en_US so results never depend on the machine.
//
// A date without a year is the most recent occurrence on or before today (2 dec read in October = last December; 29 feb = the last
// leap day), so the result is never in the future. An explicit year in the future gives ok == false. "tomorrow" gives false. Days
// before 2000-01-01 and impossible dates (31 feb, month 13, 29 feb 2026) give false. No guessing: 13/2 is false in en_US, never
// swapped to 13 Feb. Month and weekday names are not localised, so the result is the same on every machine.
//
// Go vs Swift: Swift asked DateFormatter for the locale's numeric field order. Go has no locale database, so numericFieldOrder below
// is a small table of the common locales (month-first: en_US and a few more; year-first: ja, zh, ko, hu, lt, sv, en_CA ...;
// everything else day-first). Add a locale there if one is wrong for a user.
package core

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// DateJumpEarliestDay is the earliest day "go to date" accepts.
const DateJumpEarliestDay = "2000-01-01"

const dateJumpMaxDaysBack = 60_000 // far beyond 2000 for any real "now"; keeps the arithmetic far from overflow

// ParseDateJump reads the text of the "Go to date" box. ok is false when it cannot be read or names a future or too early day.
func ParseDateJump(input string, now time.Time, cal Calendar) (string, bool) {
	loc := cal.location()
	today := DayKeyString(now, loc)
	key, ok := resolveJump(jumpWords(input), today, cal)
	if !ok || key < DateJumpEarliestDay || key > today {
		return "", false
	}
	if _, valid := DayKeyDate(key, loc); !valid {
		return "", false
	}
	return key, true
}

// ---- grammar ----

var jumpWeekdayNames = map[string]int{
	"sun": 1, "sunday": 1, "mon": 2, "monday": 2, "tue": 3, "tues": 3, "tuesday": 3, "wed": 4, "weds": 4, "wednesday": 4,
	"thu": 5, "thur": 5, "thurs": 5, "thursday": 5, "fri": 6, "friday": 6, "sat": 7, "saturday": 7}

var jumpMonthNames = map[string]int{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3, "apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6,
	"jul": 7, "july": 7, "aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9, "oct": 10, "october": 10,
	"nov": 11, "november": 11, "dec": 12, "december": 12}

func jumpWords(input string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ReplaceAll(strings.ToLower(input), ",", " "), unicode.IsSpace) {
		w = strings.TrimRight(w, ".") // "oct.", "fri.", "10.2."
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func resolveJump(t []string, today string, cal Calendar) (string, bool) {
	loc := cal.location()
	back := func(days int) (string, bool) {
		if days < 0 || days > dateJumpMaxDaysBack {
			return "", false
		}
		return DayKeyAdding(today, -days, loc)
	}
	weekday := func(name string, strictlyBefore bool) (string, bool) {
		wd, ok := jumpWeekdayNames[name]
		if !ok {
			return "", false
		}
		delta := (DayKeyWeekday(today, loc) - wd + 7) % 7
		if strictlyBefore && delta == 0 {
			delta = 7
		}
		return back(delta)
	}
	switch len(t) {
	case 1:
		w := t[0]
		switch {
		case w == "today":
			return today, true
		case w == "yesterday":
			return back(1)
		case strings.HasPrefix(w, "-"): // "-3"
			n, ok := jumpNumber(w[1:])
			if !ok {
				return "", false
			}
			return back(n)
		}
		if k, ok := weekday(w, false); ok {
			return k, true
		}
		return numericDate(w, today, cal)
	case 2:
		if t[0] == "last" {
			return weekday(t[1], true)
		}
		return monthDay(t, today, cal)
	case 3:
		if n, ok := jumpNumber(t[0]); t[2] == "ago" && ok {
			switch t[1] {
			case "day", "days":
				return back(n)
			case "week", "weeks":
				return back(n * 7) // n <= 999,999,999, so n * 7 cannot overflow
			}
			return "", false
		}
		return monthDay(t, today, cal)
	}
	return "", false
}

// monthDay reads "2 oct", "oct 2", "2nd october", "2 oct 2026", "oct 2, 2026": one month name, one day, an optional 4-digit year last.
func monthDay(t []string, today string, cal Calendar) (string, bool) {
	month, day, year := 0, 0, 0
	hasYear := false
	for i, w := range t {
		if m, ok := jumpMonthNames[w]; ok {
			if month != 0 {
				return "", false
			}
			month = m
			continue
		}
		if y, ok := jumpNumber(w); len(t) == 3 && i == 2 && utf8.RuneCountInString(w) == 4 && ok {
			year, hasYear = y, true
			continue
		}
		d, ok := jumpDayNumber(w)
		if !ok || day != 0 {
			return "", false
		}
		day = d
	}
	if month == 0 || day == 0 {
		return "", false
	}
	if hasYear {
		return jumpDateKey(year, month, day, cal)
	}
	return latestOccurrence(month, day, today, cal)
}

// numericDate reads "10/2", "10/2/26", "10/2/2026", "2026-10-02" (and the "." and "-" separators).
func numericDate(w, today string, cal Calendar) (string, bool) {
	var sep rune
	found := false
	for _, r := range w {
		if !jumpIsDigit(r) {
			sep, found = r, true
			break
		}
	}
	if !found || !strings.ContainsRune("/.-", sep) {
		return "", false
	}
	parts := strings.Split(w, string(sep))
	if len(parts) < 2 || len(parts) > 3 {
		return "", false
	}
	n := make([]int, len(parts))
	for i, p := range parts { // each part 1-4 digits
		v, ok := jumpNumber(p)
		if utf8.RuneCountInString(p) > 4 || !ok {
			return "", false
		}
		n[i] = v
	}
	if len(parts) == 3 && len(parts[0]) == 4 { // ISO order, not locale dependent
		return jumpDateKey(n[0], n[1], n[2], cal)
	}
	order := numericFieldOrder(cal.Locale)
	if len(parts) == 2 {
		order = strings.ReplaceAll(order, "y", "")
	}
	month, day, year := 0, 0, 0
	for i, field := range order {
		switch field {
		case 'y':
			if len(parts[i]) != 2 && len(parts[i]) != 4 {
				return "", false
			}
			year = n[i]
			if len(parts[i]) == 2 {
				year = 2000 + n[i]
			}
		case 'M':
			if len(parts[i]) > 2 {
				return "", false
			}
			month = n[i]
		default:
			if len(parts[i]) > 2 {
				return "", false
			}
			day = n[i]
		}
	}
	if len(parts) == 3 {
		return jumpDateKey(year, month, day, cal)
	}
	return latestOccurrence(month, day, today, cal)
}

// numericFieldOrder is the order of the numeric fields in the locale's short date as letters: "Mdy" (en_US), "dMy" (en_GB),
// "yMd" (ja_JP). "" (the root locale) reads like en_US.
func numericFieldOrder(locale string) string {
	id := locale
	if i := strings.IndexAny(id, "@."); i >= 0 {
		id = id[:i]
	}
	id = strings.ReplaceAll(id, "-", "_")
	lang, region, _ := strings.Cut(id, "_")
	lang = strings.ToLower(lang)
	region = strings.ToUpper(region)
	switch lang {
	case "":
		return "Mdy"
	case "ja", "zh", "ko", "hu", "lt", "sv", "mn", "eu":
		return "yMd"
	case "fil", "tl":
		return "Mdy"
	case "en":
		switch region {
		case "", "US", "PH", "UM", "VI", "GU", "AS", "MP", "PR":
			return "Mdy"
		case "CA", "ZA":
			return "yMd"
		}
	}
	return "dMy"
}

// ---- helpers ----

func jumpIsDigit(r rune) bool { return r >= '0' && r <= '9' } // ASCII only

// jumpNumber reads 1...9 ASCII digits (nine digits cannot overflow); anything else is not a number.
func jumpNumber(s string) (int, bool) {
	if s == "" || len(s) > 9 {
		return 0, false
	}
	v := 0
	for _, r := range s {
		if !jumpIsDigit(r) {
			return 0, false
		}
		v = v*10 + int(r-'0')
	}
	return v, true
}

// jumpDayNumber reads "2", "02", "2nd", "31st" as 2, 2, 2, 31 (1...31 only).
func jumpDayNumber(w string) (int, bool) {
	s := w
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(s, suffix) {
			s = s[:len(s)-2]
			break
		}
	}
	d, ok := jumpNumber(s)
	if !ok || len(s) > 2 || d < 1 || d > 31 {
		return 0, false
	}
	return d, true
}

// jumpDateKey is the key for y-m-d, or false when that date does not exist (31 feb, 29 feb 2026).
func jumpDateKey(y, m, d int, cal Calendar) (string, bool) {
	key := fmt.Sprintf("%04d-%02d-%02d", y, m, d)
	if _, ok := DayKeyDate(key, cal.location()); !ok {
		return "", false
	}
	return key, true
}

// latestOccurrence is month and day without a year: the most recent occurrence on or before today (29 feb walks back to the last leap day).
func latestOccurrence(month, day int, today string, cal Calendar) (string, bool) {
	var year int
	if _, err := fmt.Sscanf(today[:4], "%d", &year); err != nil {
		return "", false
	}
	for y := year; y >= year-8; y-- {
		if k, ok := jumpDateKey(y, month, day, cal); ok && k <= today {
			return k, true
		}
	}
	return "", false
}
