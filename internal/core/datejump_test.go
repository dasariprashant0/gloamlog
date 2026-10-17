package core

// Ported from tests/main.swift: "C3 DateJump". An empty "want" means the parser must return ok == false.

import (
	"strings"
	"testing"
	"time"
)

func jump(in string, now time.Time, cal Calendar) string {
	k, _ := ParseDateJump(in, now, cal)
	return k
}

type jumpRow struct{ in, want string }

func jumpTable(t *testing.T, rows []jumpRow, now time.Time, cal Calendar) {
	t.Helper()
	for _, r := range rows {
		if got := jump(r.in, now, cal); got != r.want {
			t.Errorf("%q -> %q, want %q", r.in, got, r.want)
		}
	}
}

func TestDateJumpKeywordsAndRelativeDays(t *testing.T) {
	jumpTable(t, []jumpRow{{"yesterday", "2026-10-06"}, {"today", "2026-10-07"}, {"-1", "2026-10-06"}, {"-3", "2026-10-04"}, {"-0", "2026-10-07"},
		{"3 days ago", "2026-10-04"}, {"1 day ago", "2026-10-06"}, {"0 days ago", "2026-10-07"},
		{"1 week ago", "2026-09-30"}, {"2 weeks ago", "2026-09-23"}, {"-30", "2026-09-07"},
		{"-279", "2026-01-01"}, {"-280", "2025-12-31"}}, wed7, ny)
}

func TestDateJumpWeekdayNames(t *testing.T) {
	jumpTable(t, []jumpRow{{"last fri", "2026-10-02"}, {"fri", "2026-10-02"}, {"friday", "2026-10-02"}, {"last friday", "2026-10-02"},
		{"wed", "2026-10-07"}, {"wednesday", "2026-10-07"}, {"weds", "2026-10-07"}, {"last wed", "2026-09-30"}, {"last wednesday", "2026-09-30"},
		{"thu", "2026-10-01"}, {"thur", "2026-10-01"}, {"thurs", "2026-10-01"}, {"last thu", "2026-10-01"},
		{"tue", "2026-10-06"}, {"tues", "2026-10-06"}, {"tuesday", "2026-10-06"}, {"last tue", "2026-10-06"},
		{"mon", "2026-10-05"}, {"last mon", "2026-10-05"}, {"sat", "2026-10-03"}, {"sun", "2026-10-04"}, {"last sun", "2026-10-04"},
		{"saturday", "2026-10-03"}, {"last saturday", "2026-10-03"}, {"sunday", "2026-10-04"}, {"monday", "2026-10-05"}}, wed7, ny)
}

func TestDateJumpMonthNamesNeverAFutureDay(t *testing.T) {
	jumpTable(t, []jumpRow{{"2 oct", "2026-10-02"}, {"oct 2", "2026-10-02"}, {"2 October 2026", "2026-10-02"}, {"2026-10-02", "2026-10-02"},
		{"2nd oct", "2026-10-02"}, {"oct 2nd", "2026-10-02"}, {"october 2, 2026", "2026-10-02"}, {"Oct. 2", "2026-10-02"},
		{"2 oct 2026", "2026-10-02"}, {"oct 2 2026", "2026-10-02"}, {"3rd oct", "2026-10-03"}, {"1st oct", "2026-10-01"},
		{"7 oct", "2026-10-07"}, {"1 sep", "2026-09-01"}, {"1 sept", "2026-09-01"}, {"1 september", "2026-09-01"},
		{"15 jan", "2026-01-15"}, {"31 dec", "2025-12-31"}, {"2 dec", "2025-12-02"}, {"8 oct", "2025-10-08"},
		{"29 feb", "2024-02-29"}, {"29 feb 2024", "2024-02-29"}, {"1 jan 2000", "2000-01-01"}, {"mar 3", "2026-03-03"},
		{"3 may", "2026-05-03"}, {"8 oct 2025", "2025-10-08"}, {"8 oct 2026", ""}}, wed7, ny)
}

func TestDateJumpNumericDatesFollowTheLocaleIsoNever(t *testing.T) {
	// No locale on the calendar behaves like en_US (deterministic in tests).
	jumpTable(t, []jumpRow{{"10/2", "2026-10-02"}, {"10/7", "2026-10-07"}, {"10/8", "2025-10-08"}, {"10/2/2026", "2026-10-02"}, {"10/2/26", "2026-10-02"},
		{"1/5", "2026-01-05"}, {"12/31", "2025-12-31"}, {"10.2", "2026-10-02"}, {"10-2", "2026-10-02"},
		{"2026/10/02", "2026-10-02"}, {"2026-10-2", "2026-10-02"}, {"2026-1-5", "2026-01-05"}, {"2026.10.02", "2026-10-02"},
		{"13/2", ""}, {"2/30", ""}, {"0/5", ""}, {"1/1/00", "2000-01-01"}, {"10/2/27", ""}}, wed7, ny)
	us, gb, de, ja, in := calIn("America/New_York", 2, "en_US"), calIn("America/New_York", 2, "en_GB"), calIn("America/New_York", 2, "de_DE"),
		calIn("America/New_York", 2, "ja_JP"), calIn("America/New_York", 2, "en_IN")
	jumpTable(t, []jumpRow{{"10/2", "2026-10-02"}, {"10/2/2026", "2026-10-02"}, {"2/10", "2026-02-10"}, {"13/2", ""}}, wed7, us)
	jumpTable(t, []jumpRow{{"10/2", "2026-02-10"}, {"2/10", "2026-10-02"}, {"10/2/2026", "2026-02-10"}, {"2/10/26", "2026-10-02"},
		{"31/12", "2025-12-31"}, {"12/31", ""}, {"2026-10-02", "2026-10-02"}, {"8/10", "2025-10-08"}}, wed7, gb)
	jumpTable(t, []jumpRow{{"10/2", "2026-02-10"}, {"2/10", "2026-10-02"}, {"10/2/2026", "2026-02-10"}, {"2026-10-02", "2026-10-02"}}, wed7, in)
	expect(t, ny.Locale == "", "fixture: the harness calendar has no locale, so it must read like en_US")
	jumpTable(t, []jumpRow{{"10.2", "2026-02-10"}, {"10.2.", "2026-02-10"}, {"2.10.2026", "2026-10-02"}, {"2026-10-02", "2026-10-02"}}, wed7, de)
	jumpTable(t, []jumpRow{{"10/2", "2026-10-02"}, {"26/10/2", "2026-10-02"}, {"2026/10/2", "2026-10-02"}, {"2026-10-02", "2026-10-02"}}, wed7, ja)
	jumpTable(t, []jumpRow{{"2 oct", "2026-10-02"}, {"oct 2", "2026-10-02"}, {"yesterday", "2026-10-06"}, {"last fri", "2026-10-02"}}, wed7, gb)
}

func TestDateJumpUnreadableImpossibleFutureAndPre2000GiveNothing(t *testing.T) {
	var rows []jumpRow
	for _, s := range []string{"tomorrow", "2027-01-01", "2026-10-08", "garbage", "", "   ", "\n\t",
		"31 feb", "30 feb", "29 feb 2026", "30 feb 2024", "2026-02-30", "2026-02-29",
		"2026-13-01", "2026-00-10", "2026-10-00", "2026-10-32", "32 oct", "0 oct",
		"31 sep", "31 apr", "oct", "2", "3", "last", "last week", "last garbage",
		"fri fri", "last last fri", "2 oct oct", "oct oct 2", "2 3 oct", "2 oct 3", "2 oct 26",
		"oct 2026", "2026 oct 2", "+3", "-", "--3", "3 days", "days ago",
		"-3 days ago", "3 days from now", "in 3 days", "next fri", "this fri", "now",
		"1999-12-31", "31 dec 1999", "1 jan 1999", "1900-01-01", "0000-01-01", "-99999",
		"12/31/99", "99999999999999999999 days ago", "-99999999999999999999", "9999999 weeks ago",
		"10000-01-01", "2026-10-02-1", "1/2/3/4", "//", "1//2", "10/2/", "/10/2",
		"10/2-2026", "2026-10", "٣", "١٢/٣", "🎉", "2 🎉"} {
		rows = append(rows, jumpRow{s, ""})
	}
	jumpTable(t, rows, wed7, ny)
}

func TestDateJumpCaseWhitespaceAndPunctuationAreIgnored(t *testing.T) {
	jumpTable(t, []jumpRow{{"  YESTERDAY  ", "2026-10-06"}, {"LAST FRI", "2026-10-02"}, {"Last   Fri", "2026-10-02"}, {"\t2 Oct\n", "2026-10-02"},
		{"OCT 2", "2026-10-02"}, {"2 OCTOBER 2026", "2026-10-02"}, {"  2026-10-02  ", "2026-10-02"}, {"2 oct", "2026-10-02"},
		{"Oct 2,", "2026-10-02"}, {"2 oct,", "2026-10-02"}, {"OCTOBER 2ND, 2026", "2026-10-02"}, {" -3 ", "2026-10-04"},
		{"3  DAYS   AGO", "2026-10-04"}, {"Fri.", "2026-10-02"}, {"last fri.", "2026-10-02"}}, wed7, ny)
}

func TestDateJumpAYearRoundTripsAndWeekdayNamesMatchAnOracle(t *testing.T) {
	for _, now := range []time.Time{wed7, mk(2028, 3, 10, 9, 0), mk(2026, 1, 2, 22, 0)} {
		today := DayKeyString(now, nyLoc)
		var failures []string
		checked := 0
		for n := 0; n < 360; n++ {
			day, _ := DayKeyAdding(today, -n, nyLoc)
			oracle := utcKey(utcDate(atoi(today[:4]), atoi(today[5:7]), atoi(today[8:])).AddDate(0, 0, -n))
			if day != oracle {
				failures = append(failures, "oracle "+day+" vs "+oracle)
			}
			for _, in := range []string{day, DayKeyFormat(day, "d MMM yyyy", nyLoc), DayKeyFormat(day, "MMMM d, yyyy", nyLoc), DayKeyFormat(day, "d MMM", nyLoc),
				DayKeyFormat(day, "MMM d", nyLoc), "-" + itoa(n), itoa(n) + " days ago"} {
				checked++
				if got := jump(in, now, ny); got != day {
					failures = append(failures, "'"+in+"' -> "+got+", want "+day)
				}
			}
		}
		expect(t, len(failures) == 0 && checked == 360*7, "%s: %v checked %d", today, failures[:min(4, len(failures))], checked)
	}
	names := []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
	var bad []string
	checked := 0
	for dom := 1; dom <= 31; dom++ {
		now := mk(2026, 10, dom, 12, 0)
		todayUTC := utcDate(2026, 10, dom)
		for i, name := range names {
			for _, c := range []struct {
				in     string
				strict bool
			}{{name, false}, {"last " + name, true}} {
				k := 0
				if c.strict {
					k = 1
				}
				for int(todayUTC.AddDate(0, 0, -k).Weekday()) != i {
					k++
				}
				want := utcKey(todayUTC.AddDate(0, 0, -k))
				checked++
				if jump(c.in, now, ny) != want {
					bad = append(bad, c.in+" on 2026-10-"+itoa(dom))
				}
			}
		}
	}
	expect(t, len(bad) == 0 && checked == 31*14, "%v checked %d", bad[:min(4, len(bad))], checked)
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func TestDateJumpBoundaries(t *testing.T) {
	jumpTable(t, []jumpRow{{"yesterday", "2000-01-02"}, {"-2", "2000-01-01"}, {"-3", ""}, {"last mon", ""}, {"mon", "2000-01-03"}, {"1 jan", "2000-01-01"},
		{"31 dec", ""}, {"sun", "2000-01-02"}, {"2000-01-01", "2000-01-01"}, {"1999-12-31", ""}}, mk(2000, 1, 3, 12, 0), ny)
	jumpTable(t, []jumpRow{{"2 dec", "2025-12-02"}, {"last fri", "2025-12-26"}, {"-3", "2025-12-30"}, {"1 jan", "2026-01-01"}, {"3 jan", "2025-01-03"},
		{"12/31", "2025-12-31"}}, mk(2026, 1, 2, 12, 0), ny)
	jumpTable(t, []jumpRow{{"-1", "2026-11-01"}, {"-2", "2026-10-31"}, {"last sun", "2026-11-01"}, {"3 days ago", "2026-10-30"}, {"1 week ago", "2026-10-26"},
		{"sun", "2026-11-01"}}, mk(2026, 11, 2, 12, 0), ny) // day after the 25 h fall-back day
	jumpTable(t, []jumpRow{{"yesterday", "2026-03-08"}, {"last sun", "2026-03-08"}, {"-2", "2026-03-07"}, {"-7", "2026-03-02"}}, mk(2026, 3, 9, 12, 0), ny) // after the 23 h day
	jumpTable(t, []jumpRow{{"yesterday", "2028-02-29"}, {"29 feb", "2028-02-29"}, {"-366", "2027-03-01"}, {"2028-02-29", "2028-02-29"}, {"29 feb 2027", ""},
		{"1 mar", "2028-03-01"}, {"2 mar", "2027-03-02"}}, mk(2028, 3, 1, 12, 0), ny)
	jumpTable(t, []jumpRow{{"29 feb", "2000-02-29"}, {"29 feb 2000", "2000-02-29"}, {"29 feb 2001", ""}}, mk(2002, 1, 5, 12, 0), ny) // most recent leap day, two years back
	jumpTable(t, []jumpRow{{"today", "2026-10-07"}, {"yesterday", "2026-10-06"}, {"-1", "2026-10-06"}, {"tomorrow", ""}}, mk(2026, 10, 7, 23, 59), ny)
	jumpTable(t, []jumpRow{{"today", "2026-10-07"}, {"yesterday", "2026-10-06"}, {"wed", "2026-10-07"}}, mk(2026, 10, 7, 0, 1), ny)
	// the same instant is a different day in another zone: `today` follows the calendar's time zone
	instant := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	nyc, akl := calIn("America/New_York", 2, ""), calIn("Pacific/Auckland", 2, "")
	expect(t, jump("today", instant, nyc) == "2026-10-07", "20:00 UTC is still the 7th in New York")
	expect(t, jump("today", instant, akl) == "2026-10-08", "and already the 8th in Auckland")
	expect(t, jump("yesterday", instant, akl) == "2026-10-07" && jump("2026-10-08", instant, akl) == "2026-10-08", "Auckland yesterday and ISO")
	expect(t, jump("2026-10-08", instant, nyc) == "", "the 8th is the future in New York")
}

func TestDateJumpJunkAndExtremeInputNeverCrash(t *testing.T) {
	junk := []string{strings.Repeat("9", 5000), strings.Repeat("a ", 3000), strings.Repeat("1/", 2000), "-" + strings.Repeat("9", 40),
		"\u0000", "oct\u00002", "2‮oct", "é 2", "1e3", "0x10", "1_000", "٢٠٢٦-١٠-٠٢", "２０２６-１０-０２", "２ oct", "ⅹ", "½",
		"last fri fri fri fri", "-9223372036854775808", "9223372036854775807 days ago", "-0 days ago", "00000000000000000002 oct"}
	for _, s := range junk {
		r := jump(s, wed7, ny)
		_, valid := DayKeyDate(r, nyLoc)
		expect(t, r == "" || (r >= "2000-01-01" && r <= "2026-10-07" && valid), "junk %.20q -> %q", s, r)
	}
	r := jump("00000000000000000002 oct", wed7, ny)
	expect(t, r == "" || r == "2026-10-02", "leading zeros")
}
