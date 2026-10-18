// models.go - core value types: day keys, day status, the day document. Standard library only. Port of app/Core/Models.swift.
// See docs/EDITOR_CONTRACT.md.
//
// API:
//
//	DayKey*           "yyyy-MM-dd" day strings: DayKeyString(t, loc) / DayKeyDate(key, loc) / DayKeyAdding(key, n, loc) /
//	                  DayKeyWeekday(key, loc) / DayKeyFormat(key, pattern, loc) / DayKeyIsWellFormed(key)
//	DayStatus         logged | partial | skipped | missed | off | future
//	DayDocument       one day = ONE free-form markdown page:
//	                    {Day, Body, IsSkipped, SkipReason}  +  Words(), HasContent(), Status(minWords)
//	                  Body never contains the "# yyyy-MM-dd" title line (LogStore adds/strips it).
//	                  Logged rule: words >= minWords. partial = 1...minWords-1 words. 0 words -> missed (= no log).
//	                  Task lines under a "Carried over from ..." heading are not words (a Carry over click alone never logs a day).
//	                  Jots: the words of the Jots block (JotsRules.Heading) are not words unless JotsRules.CountTowardLogged is on,
//	                  and a leading HH:mm of a jot is never a word. A page with no words but at least one jot is "partial" ("started"),
//	                  whatever the switch says, so a jots-only day stays in Catch up and the reminder still asks for the write-up.
//
// All dates are passed in with a time zone; nothing here reads the clock.
package core

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// isFoundationSpace is Foundation's .whitespacesAndNewlines: unicode.IsSpace plus U+200B (zero width space, which Foundation counts).
func isFoundationSpace(r rune) bool { return unicode.IsSpace(r) || r == 0x200B }

// trimmed removes leading and trailing whitespace and newlines (Foundation's .whitespacesAndNewlines; Swift name: dlTrimmed).
func trimmed(s string) string { return strings.TrimFunc(s, isFoundationSpace) }

// DayKeyIsWellFormed is a shape check only ("2026-10-06"); it guards file names against path tricks.
func DayKeyIsWellFormed(s string) bool {
	if len(s) != 10 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if i == 4 || i == 7 {
			if b != '-' {
				return false
			}
		} else if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

// DayKeyString is the "yyyy-MM-dd" key of the day that contains t in loc.
func DayKeyString(t time.Time, loc *time.Location) string {
	y, m, d := t.In(loc).Date()
	return fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
}

// DayKeyDate is the start of that day in loc; ok is false for malformed or impossible dates (2026-02-30).
func DayKeyDate(key string, loc *time.Location) (time.Time, bool) {
	if !DayKeyIsWellFormed(key) {
		return time.Time{}, false
	}
	var y, m, d int
	fmt.Sscanf(key, "%d-%d-%d", &y, &m, &d)
	if y < 1 { // Foundation's calendar has no year 0
		return time.Time{}, false
	}
	t := startOfDay(y, time.Month(m), d, loc)
	if DayKeyString(t, loc) != key { // an impossible date rolls over into the next month, so the round trip catches it
		return time.Time{}, false
	}
	return t, true
}

// DayKeyAdding does calendar-based day arithmetic (DST safe: never adds 86400 seconds).
func DayKeyAdding(key string, days int, loc *time.Location) (string, bool) {
	d, ok := DayKeyDate(key, loc)
	if !ok {
		return "", false
	}
	return DayKeyString(addDays(d, days, loc), loc), true
}

// DayKeyWeekday is the calendar weekday number, 1 = Sunday ... 7 = Saturday; 0 for a bad key.
func DayKeyWeekday(key string, loc *time.Location) int {
	d, ok := DayKeyDate(key, loc)
	if !ok {
		return 0
	}
	return weekdayOf(d, loc)
}

// DayKeyFormat formats a day with an ICU/CLDR date pattern, English names, in loc. Swift used DateFormatter(en_US_POSIX); Go has no
// such formatter, so only the pattern letters the app uses are supported: y/yy/yyyy, M/MM/MMM/MMMM, d/dd, E..EEEE ("EEE d MMM",
// "EEEE, d MMMM yyyy", "MMMM"). 'quoted text' is literal; any other character is copied. A bad key comes back unchanged.
func DayKeyFormat(key, pattern string, loc *time.Location) string {
	d, ok := DayKeyDate(key, loc)
	if !ok {
		return key
	}
	return formatPattern(d, pattern)
}

func formatPattern(t time.Time, pattern string) string {
	var out strings.Builder
	rs := []rune(pattern)
	for i := 0; i < len(rs); {
		c := rs[i]
		if c == '\'' { // 'quoted' literal; '' is one quote
			if i+1 < len(rs) && rs[i+1] == '\'' {
				out.WriteRune('\'')
				i += 2
				continue
			}
			i++
			for i < len(rs) && rs[i] != '\'' {
				out.WriteRune(rs[i])
				i++
			}
			i++
			continue
		}
		if !unicode.IsLetter(c) || c > unicode.MaxASCII {
			out.WriteRune(c)
			i++
			continue
		}
		n := 1
		for i+n < len(rs) && rs[i+n] == c {
			n++
		}
		i += n
		switch c {
		case 'y':
			if n == 2 {
				out.WriteString(fmt.Sprintf("%02d", t.Year()%100))
			} else {
				out.WriteString(fmt.Sprintf("%0*d", n, t.Year()))
			}
		case 'M', 'L':
			name := t.Month().String()
			switch {
			case n <= 2:
				out.WriteString(fmt.Sprintf("%0*d", n, int(t.Month())))
			case n == 3:
				out.WriteString(name[:3])
			case n == 4:
				out.WriteString(name)
			default:
				out.WriteString(name[:1])
			}
		case 'd':
			out.WriteString(fmt.Sprintf("%0*d", n, t.Day()))
		case 'E':
			name := t.Weekday().String()
			switch {
			case n <= 3:
				out.WriteString(name[:3])
			case n == 4:
				out.WriteString(name)
			case n == 5:
				out.WriteString(name[:1])
			default:
				out.WriteString(name[:2])
			}
		default:
			out.WriteString(strings.Repeat(string(c), n)) // not supported: copied as is
		}
	}
	return out.String()
}

// DayStatus is what a day looks like in the calendar and the streak.
type DayStatus string

const (
	StatusLogged  DayStatus = "logged"  // words >= minWords
	StatusPartial DayStatus = "partial" // 1...minWords-1 words
	StatusSkipped DayStatus = "skipped" // skip marker file
	StatusMissed  DayStatus = "missed"  // scheduled day, no log (today unlogged is also missed; UI draws today's ring)
	StatusOff     DayStatus = "off"     // not a scheduled day, no log (or before the first ever entry)
	StatusFuture  DayStatus = "future"  // after today
)

// DayDocument is one day's page. Body is plain markdown without the "# yyyy-MM-dd" title line.
type DayDocument struct {
	Day        string `json:"day"`
	Body       string `json:"body"`
	IsSkipped  bool   `json:"isSkipped"`
	SkipReason string `json:"skipReason"`
}

// Words counts per the contract rule (headings, empty list/task markers, code-fence lines, images, HTML comments ignored; task lines
// under a "Carried over from ..." heading never count, see markdownbody.go). Jots do not count unless JotsRules.CountTowardLogged is
// on (LoggedWords): this is what "logged" is measured in. On a list page every Log line counts (JotsRulesFor).
func (d DayDocument) Words() int {
	if d.IsSkipped {
		return 0
	}
	rules := JotsRulesFor(d.Body)
	return LoggedWords(d.Body, rules.Heading, rules.CountTowardLogged)
}

// JotCount is how many notes the page's Jots block holds (top-level list items), whatever the rule counts as words.
func (d DayDocument) JotCount() int {
	if d.IsSkipped {
		return 0
	}
	return BodyCounts(d.Body, JotsRulesFor(d.Body).Heading).JotCount
}

// HasContent is true if the page holds anything beyond headings / empty markers (an image or a carried-over block alone counts).
func (d DayDocument) HasContent() bool { return !d.IsSkipped && HasContent(d.Body) }

// Status is skipped / logged (words >= minWords) / partial (1...minWords-1, or no words but at least one jot) / missed (nothing
// written: treat as "no log"). One scan of the page: the words and the jot count come from the same BodyCounts.
func (d DayDocument) Status(minWords int) DayStatus {
	if d.IsSkipped {
		return StatusSkipped
	}
	rules := JotsRulesFor(d.Body)
	c := BodyCounts(d.Body, rules.Heading)
	w := c.Words
	if rules.CountTowardLogged {
		w += c.JotWords
	}
	if w >= max(1, minWords) {
		return StatusLogged
	}
	if w > 0 || c.JotCount > 0 {
		return StatusPartial
	}
	return StatusMissed
}

// WeekdaySet is a set of Calendar weekday numbers (1 = Sunday ... 7 = Saturday), kept sorted without duplicates by Settings.Normalized.
// Swift used Set<Int>; a sorted slice marshals to a plain JSON array.
type WeekdaySet []int

// Has reports whether weekday (1 = Sunday ... 7 = Saturday) is in the set.
func (w WeekdaySet) Has(weekday int) bool {
	for _, d := range w {
		if d == weekday {
			return true
		}
	}
	return false
}

// utf8BOM is the UTF-8 byte-order mark (U+FEFF). Files that came through a Windows editor start with one; it is dropped on load.
const utf8BOM = "\xef\xbb\xbf"
