package core

// Ported from tests/main.swift: "C4 CatchUp and batch skip" (missing, window, log start, DST, next, skipDays/unskipDays, e2e, speed).

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

var (
	wed7 = noon(2026, 10, 7) // Wednesday 7 Oct 2026, noon, New York
	thu8 = noon(2026, 10, 8) // Thursday 8 Oct 2026
)

// missing is CatchUpMissing with the Swift test defaults: now = wed7, Mon-Fri, window 30.
func missing(st map[string]DayStatus, logStart string) []string {
	return CatchUpMissing(st, wed7, ny, workdays, logStart, 30)
}

// folderSnapshot is every entry of a folder (hidden files too) with its bytes: a byte-for-byte comparison of two states of the folder.
func folderSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, n := range dirNames(t, dir) {
		fi, err := os.Stat(filepath.Join(dir, n))
		must(t, err)
		if fi.IsDir() {
			out[n] = "<dir>"
		} else {
			b, err := os.ReadFile(filepath.Join(dir, n))
			must(t, err)
			out[n] = string(b)
		}
	}
	return out
}

func hasDay(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestCatchUpListsScheduledMissedAndPartialDaysOldestFirst(t *testing.T) {
	list := missing(states("2026-10-05", StatusLogged), "2026-09-28")
	expect(t, equal(list, []string{"2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02", "2026-10-06"}), "%v", list)
	expect(t, !hasDay(list, "2026-10-07"), "never today")
	expect(t, !hasDay(list, "2026-10-03") && !hasDay(list, "2026-10-04"), "weekend is not scheduled")
	expect(t, sort.StringsAreSorted(list), "oldest first")
	st := states("2026-09-29", StatusPartial, "2026-09-30", StatusSkipped, "2026-10-01", StatusLogged, "2026-10-05", StatusSkipped, "2026-10-06", StatusSkipped)
	expect(t, equal(missing(st, "2026-09-28"), []string{"2026-09-28", "2026-09-29", "2026-10-02"}), "%v", missing(st, "2026-09-28"))
	off := states("2026-10-03", StatusPartial, "2026-10-04", StatusMissed, "2026-09-26", StatusLogged, "2026-09-27", StatusPartial)
	expect(t, equal(missing(off, "2026-09-21"), []string{"2026-09-21", "2026-09-22", "2026-09-23", "2026-09-24", "2026-09-25", "2026-09-28", "2026-09-29",
		"2026-09-30", "2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"}), "%v", missing(off, "2026-09-21"))
	expect(t, !hasDay(missing(states("2026-10-07", StatusPartial, "2026-10-09", StatusLogged, "2026-10-08", StatusPartial), "2026-10-05"), "2026-10-07"), "today")
	expect(t, equal(missing(states("2026-10-07", StatusPartial, "2026-10-08", StatusPartial), "2026-10-05"), []string{"2026-10-05", "2026-10-06"}), "today and the future")
	expect(t, len(missing(states("2026-10-05", StatusLogged, "2026-10-06", StatusLogged, "2026-10-02", StatusLogged), "2026-10-02")) == 0, "all written")
	expect(t, equal(CatchUpMissing(nil, wed7, ny, WeekdaySet{1, 7}, "2026-09-28", 30), []string{"2026-10-03", "2026-10-04"}), "weekend schedule")
	every := WeekdaySet{1, 2, 3, 4, 5, 6, 7}
	expect(t, equal(CatchUpMissing(nil, wed7, ny, every, "2026-10-03", 30), []string{"2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06"}), "every day")
	expect(t, equal(CatchUpMissing(nil, mk(2026, 10, 5, 9, 0), ny, workdays, "2026-09-28", 30), []string{"2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02"}), "Monday")
}

func TestCatchUpWindowEdgesAreExactAndClamped(t *testing.T) {
	l30 := CatchUpMissing(nil, thu8, ny, workdays, "2026-01-01", 30)
	expect(t, l30[0] == "2026-09-08" && !hasDay(l30, "2026-09-07") && l30[len(l30)-1] == "2026-10-07", "%s ... %s", l30[0], l30[len(l30)-1])
	expect(t, len(l30) == 22, "22 workdays between 8 Sep and 7 Oct, got %d", len(l30))
	expect(t, equal(CatchUpMissing(nil, wed7, ny, workdays, "2026-01-01", 7), []string{"2026-09-30", "2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"}), "7 days")
	for _, w := range []int{6, 3, 1, 0, -5, -1 << 62} {
		expect(t, equal(CatchUpMissing(nil, wed7, ny, workdays, "2026-01-01", w), CatchUpMissing(nil, wed7, ny, workdays, "2026-01-01", 7)), "window %d acts as 7", w)
	}
	l365 := CatchUpMissing(nil, thu8, ny, workdays, "2025-01-01", 365)
	expect(t, l365[0] == "2025-10-08" && !hasDay(l365, "2025-10-07") && l365[len(l365)-1] == "2026-10-07", "%s", l365[0])
	for _, w := range []int{366, 1000, 100_000, 1 << 62} {
		expect(t, equal(CatchUpMissing(nil, thu8, ny, workdays, "2025-01-01", w), l365), "window %d acts as 365", w)
	}
	expect(t, CatchUpMissing(nil, thu8, ny, workdays, "2025-01-01", 364)[0] == "2025-10-09", "364")
}

func TestCatchUpLogStartCutsTheListNoStartAndNoPagesMeansNothing(t *testing.T) {
	expect(t, equal(missing(nil, "2026-10-02"), []string{"2026-10-02", "2026-10-05", "2026-10-06"}), "start day itself is listed")
	expect(t, equal(missing(states("2026-09-30", StatusPartial), "2026-10-02"), []string{"2026-10-02", "2026-10-05", "2026-10-06"}), "a partial page before the start is not listed")
	expect(t, len(missing(nil, "2026-10-20")) == 0 && len(missing(nil, "2026-10-07")) == 0, "a start today or later: nothing before it")
	expect(t, equal(missing(states("2026-09-30", StatusLogged), ""), []string{"2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"}), "earliest page")
	expect(t, equal(missing(states("2026-09-30", StatusSkipped), ""), []string{"2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"}), "a skip marker also starts the log")
	expect(t, len(missing(nil, "")) == 0, "a new user has nothing to catch up on")
	expect(t, len(CatchUpMissing(nil, wed7, ny, workdays, "", 365)) == 0, "365 days, still nothing")
	m := missing(states("2026-09-12", StatusLogged, "2026-10-05", StatusLogged), "2026-10-01")
	expect(t, equal(m, []string{"2026-10-01", "2026-10-02", "2026-10-06"}), "%v", m)
	wall := missing(states("2026-09-12", StatusLogged, "2026-10-05", StatusLogged), "")
	expect(t, wall[0] == "2026-09-14" && len(wall) == 16, "%d days without a pinned start", len(wall))
	expect(t, equal(missing(states("2026-10-06", StatusLogged), "2026-10-05"), []string{"2026-10-05"}), "explicit start")
}

func TestCatchUpWindowAcrossDSTHasNoDuplicateOrMissingDay(t *testing.T) {
	every := WeekdaySet{1, 2, 3, 4, 5, 6, 7}
	for _, now := range []time.Time{mk(2026, 11, 4, 12, 0), mk(2026, 3, 10, 12, 0), mk(2026, 11, 2, 0, 30), mk(2026, 3, 9, 23, 59)} {
		for _, w := range []int{7, 30, 365} {
			l := CatchUpMissing(nil, now, ny, every, "2000-01-01", w)
			today := DayKeyString(now, nyLoc)
			uniq := map[string]bool{}
			for _, d := range l {
				uniq[d] = true
			}
			expect(t, len(l) == w && len(uniq) == w, "%s window %d: %d unique days", today, w, len(l))
			yesterday, _ := DayKeyAdding(today, -1, nyLoc)
			first, _ := DayKeyAdding(today, -w, nyLoc)
			expect(t, l[len(l)-1] == yesterday && l[0] == first, "%s window %d edges", today, w)
			consecutive := true
			for i := 1; i < len(l); i++ {
				if n, _ := DayKeyAdding(l[i-1], 1, nyLoc); n != l[i] {
					consecutive = false
				}
			}
			expect(t, consecutive, "%s window %d consecutive", today, w)
		}
	}
}

func TestCatchUpAFullYearWindowIsFast(t *testing.T) {
	t0 := time.Now()
	total := 0
	for i := 0; i < 5; i++ {
		total += len(CatchUpMissing(nil, thu8, ny, workdays, "2000-01-01", 365))
	}
	per := time.Since(t0) / 5
	expect(t, total > 0 && per < 50*time.Millisecond, "365-day window took %v", per)
}

func TestCatchUpNextIsTheFirstEntryStrictlyAfter(t *testing.T) {
	l := []string{"2026-09-28", "2026-09-29", "2026-10-02", "2026-10-05"}
	next := func(after string, list []string) string { s, _ := CatchUpNext(after, list); return s }
	expect(t, next("", l) == "2026-09-28", "nil -> first")
	expect(t, next("2026-09-28", l) == "2026-09-29" && next("2026-09-29", l) == "2026-10-02", "walks on")
	expect(t, next("2026-09-30", l) == "2026-10-02", "a day that is not in the list")
	_, ok1 := CatchUpNext("2026-10-05", l)
	_, ok2 := CatchUpNext("2026-12-01", l)
	expect(t, !ok1 && !ok2, "none after the last")
	expect(t, next("2000-01-01", l) == "2026-09-28", "long ago")
	_, ok3 := CatchUpNext("", nil)
	_, ok4 := CatchUpNext("2026-10-01", nil)
	expect(t, !ok3 && !ok4, "empty list")
	one := []string{"2026-10-02"}
	expect(t, next("", one) == "2026-10-02" && next("2026-10-01", one) == "2026-10-02", "one entry")
	_, ok5 := CatchUpNext("2026-10-02", one)
	_, ok6 := CatchUpNext("2026-10-03", one)
	expect(t, !ok5 && !ok6, "strictly after")
}

func TestSkipDaysOneMarkerPerDayReturnsOnlyWhatItChanged(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	save(t, store, "2026-10-05", lorem(30, "w"))
	save(t, store, "2026-10-02", "## What I did\n\n## Finished") // headings only: no writing
	save(t, store, "2026-10-01", "![](assets/x.png)")            // an image alone is content
	before := folderSnapshot(t, dir)
	r := val(store.SkipDays([]string{"2026-09-29", "2026-09-30", "2026-10-05", "2026-10-02", "2026-10-01"}, "Leave"))
	expect(t, equal(r, []string{"2026-09-29", "2026-09-30", "2026-10-02"}), "%v", r)
	for _, d := range r {
		doc := load(t, store, d)
		expect(t, doc != nil && doc.IsSkipped && doc.SkipReason == "Leave", d)
	}
	expect(t, fileText(t, dir, "2026-09-29") == "# 2026-09-29\n\n> Skipped: Leave\n", "the marker text written by Skip")
	after := folderSnapshot(t, dir)
	expect(t, after["2026-10-05.md"] == before["2026-10-05.md"] && after["2026-10-01.md"] == before["2026-10-01.md"], "pages with writing are byte-identical")
	want := map[string]bool{"2026-09-29.md": true, "2026-09-30.md": true}
	for k := range before {
		want[k] = true
	}
	got := map[string]bool{}
	for k := range after {
		got[k] = true
	}
	expect(t, reflect.DeepEqual(got, want), "only markers were added: %v", after)
	expect(t, len(val(store.SkipDays(nil, "Leave"))) == 0, "empty list")
	r2 := val(store.SkipDays([]string{"2026-10-08", "2026-10-07", "2026-10-08"}, ""))
	expect(t, equal(r2, []string{"2026-10-08", "2026-10-07"}), "input order, duplicates once: %v", r2)
	expect(t, fileText(t, dir, "2026-10-07") == "# 2026-10-07\n\n> Skipped\n", "no reason = plain marker")
	r3 := val(store.SkipDays([]string{"2026-10-09"}, "Sick\nday  "))
	expect(t, equal(r3, []string{"2026-10-09"}) && load(t, store, "2026-10-09").SkipReason == "Sick day", "the reason stays on one line")
	r4 := val(store.SkipDays([]string{"2026-09-29", "2026-10-07"}, "Holiday"))
	expect(t, len(r4) == 0, "%v", r4)
	expect(t, load(t, store, "2026-09-29").SkipReason == "Leave" && load(t, store, "2026-10-07").SkipReason == "", "reasons are kept")
	st := val(store.FileStates(20))
	expect(t, st["2026-09-29"] == StatusSkipped && st["2026-10-05"] == StatusLogged && st["2026-10-02"] == StatusSkipped, "states")
	snap := folderSnapshot(t, dir)
	_, err := store.SkipDays([]string{"2026-11-02", "../evil", "2026-11-03"}, "x")
	expectErr(t, err, ErrBadDay, "../evil")
	expect(t, reflect.DeepEqual(folderSnapshot(t, dir), snap), "a malformed entry writes nothing at all")
	_, err = store.SkipDays([]string{"2026-11-04", "x"}, "x")
	expectErr(t, err, ErrBadDay, "x")
	missingFolder := NewLogStore(filepath.Join(t.TempDir(), "nope-skip"), "", 0, nil)
	_, err = missingFolder.SkipDays([]string{"2026-10-05"}, "x")
	expectErr(t, err, ErrFolderMissing, missingFolder.Dir)
}

func TestSkipDaysAFailingWriteTakesBackWhatTheCallWrote(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	save(t, store, "2026-10-05", lorem(30, "w"))
	chmod(t, dir, 0o555)
	_, err := store.SkipDays([]string{"2026-10-01", "2026-10-02"}, "Leave")
	chmod(t, dir, 0o755)
	expectErr(t, err, ErrFolderNotWritable, dir)
	expect(t, equal(val(store.ListDays()), []string{"2026-10-05"}), "no markers were left behind")
	// a failure in the MIDDLE of a batch (a directory squats on the day file name): the marker written before it is taken back
	mid := newStore(t)
	must(t, os.Mkdir(filepath.Join(mid.Dir, "2026-10-02.md"), 0o755))
	_, err = mid.SkipDays([]string{"2026-10-01", "2026-10-02", "2026-10-05"}, "Leave")
	var le *LogError
	expect(t, errors.As(err, &le), "typed error, got %v", err)
	expect(t, equal(dirNames(t, mid.Dir), []string{"2026-10-02.md"}), "no marker left behind: %v", dirNames(t, mid.Dir))
}

func TestUnskipDaysRestoresPureMarkersOnly(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	save(t, store, "2026-10-05", lorem(30, "w"))
	must(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "assets/2026-10-05-1a2b3c4d.png"), pngBytes, 0o644))
	save(t, store, "2026-10-06", lorem(5, "w")) // a partial day stays untouched by the batch
	before := folderSnapshot(t, dir)
	skipped := val(store.SkipDays([]string{"2026-09-29", "2026-09-30", "2026-10-01", "2026-10-05", "2026-10-06"}, "Leave"))
	expect(t, equal(skipped, []string{"2026-09-29", "2026-09-30", "2026-10-01"}), "%v", skipped)
	expect(t, equal(val(store.UnskipDays(skipped)), skipped), "restored")
	expect(t, reflect.DeepEqual(folderSnapshot(t, dir), before), "folder is byte-identical after skip then undo")
	expect(t, len(val(store.UnskipDays(skipped))) == 0, "undo twice does nothing")
	again := val(store.SkipDays([]string{"2026-09-29", "2026-09-30", "2026-10-01"}, "Leave"))
	save(t, store, "2026-09-30", "I worked after all today, wrote this by hand.")
	r := val(store.UnskipDays(again))
	expect(t, equal(r, []string{"2026-09-29", "2026-10-01"}), "%v", r)
	expect(t, load(t, store, "2026-09-30").Body == "I worked after all today, wrote this by hand." && !load(t, store, "2026-09-30").IsSkipped, "the typed page is kept")
	expect(t, load(t, store, "2026-09-29") == nil && load(t, store, "2026-10-01") == nil, "markers gone")
	writeRaw(t, dir, "2026-10-02", "# 2026-10-02\n\n> Skipped: Leave\n\nand some notes I added\n")
	writeRaw(t, dir, "2026-10-07", "# 2026-10-07\n\n> Skipped\n")
	got := val(store.UnskipDays([]string{"2026-10-02", "2026-10-07", "2026-10-05", "2026-10-06", "2026-10-30"}))
	expect(t, equal(got, []string{"2026-10-07"}), "logs, partial pages, notes and missing files are not touched: %v", got)
	expect(t, strings.Contains(fileText(t, dir, "2026-10-02"), "and some notes I added"), "notes kept")
	expect(t, load(t, store, "2026-10-05").Words() == 30 && load(t, store, "2026-10-06").Words() == 5, "pages untouched")
	expect(t, len(val(store.UnskipDays(nil))) == 0, "empty")
	two := val(store.SkipDays([]string{"2026-10-12", "2026-10-13"}, "Sick"))
	expect(t, equal(val(store.UnskipDays([]string{"2026-10-13", "2026-10-12", "2026-10-13"})), []string{"2026-10-13", "2026-10-12"}) && len(two) == 2, "input order, duplicates once")
	val(store.SkipDays([]string{"2026-10-14"}, "x"))
	_, err := store.UnskipDays([]string{"2026-10-14", "nope"})
	expectErr(t, err, ErrBadDay, "nope")
	expect(t, load(t, store, "2026-10-14").IsSkipped, "a malformed entry stops the call before anything is deleted")
	save(t, store, "2026-10-19", "## What I did\n\n## Finished")
	expect(t, equal(val(store.SkipDays([]string{"2026-10-19"}, "Leave")), []string{"2026-10-19"}) && equal(val(store.UnskipDays([]string{"2026-10-19"})), []string{"2026-10-19"}), "skip and undo")
	expect(t, load(t, store, "2026-10-19") == nil, "a headings-only page is gone after undo (documented)")
}

func TestSkipThenUndoTheStreakNeverFallsAndCatchUpFollows(t *testing.T) {
	store := newStore(t)
	now := mk(2026, 10, 8, 12, 0) // Thu 8 Oct, nothing written yet today
	for _, d := range []string{"2026-09-28", "2026-09-29", "2026-09-30", "2026-10-06", "2026-10-07"} {
		save(t, store, d, lorem(25, "w"))
	}
	state := func() (StreakResult, []string) {
		st := val(store.FileStates(20))
		return ComputeStreak(st, now, ny, workdays, "2026-09-28"), CatchUpMissing(st, now, ny, workdays, "2026-09-28", 30)
	}
	s0, l0 := state()
	expect(t, equal(l0, []string{"2026-10-01", "2026-10-02", "2026-10-05"}) && s0 == StreakResult{2, 3}, "before: %+v %v", s0, l0)
	skipped := val(store.SkipDays(l0, "Leave"))
	s1, l1 := state()
	expect(t, equal(skipped, l0) && len(l1) == 0, "the days leave the list")
	expect(t, s1.Current >= s0.Current && s1.Best >= s0.Best, "skipping never lowers the streak: %+v -> %+v", s0, s1)
	expect(t, s1 == StreakResult{5, 5}, "skipped days are neutral, so the two runs join: %+v", s1)
	val(store.UnskipDays(skipped))
	s2, l2 := state()
	expect(t, s2 == s0 && equal(l2, l0), "undo brings the streak and the list back exactly: %+v", s2)
}

func TestCatchUpEndToEndThroughRealFiles(t *testing.T) {
	store := newStore(t)
	for _, d := range []string{"2026-09-28", "2026-09-29", "2026-09-30", "2026-10-06", "2026-10-07"} {
		save(t, store, d, lorem(25, "w"))
	}
	now := mk(2026, 10, 8, 12, 0)
	list := func() []string { return CatchUpMissing(val(store.FileStates(20)), now, ny, workdays, "", 30) }
	l := list()
	expect(t, equal(l, []string{"2026-10-01", "2026-10-02", "2026-10-05"}), "exactly the 3 unwritten workdays: %v", l)
	cur, _ := CatchUpNext("", l)
	expect(t, cur == "2026-10-01", "first")
	save(t, store, "2026-10-01", lorem(19, "w"))
	expect(t, equal(list(), []string{"2026-10-01", "2026-10-02", "2026-10-05"}), "19 of 20 words stays on the list (partial)")
	save(t, store, "2026-10-01", lorem(20, "w"))
	l = list()
	expect(t, equal(l, []string{"2026-10-02", "2026-10-05"}), "20 words closes the day: %v", l)
	cur, _ = CatchUpNext(cur, l)
	expect(t, cur == "2026-10-02", "next")
	save(t, store, "2026-10-02", lorem(20, "w"))
	cur, _ = CatchUpNext(cur, list())
	expect(t, cur == "2026-10-05", "next again")
	save(t, store, "2026-10-05", lorem(20, "w"))
	_, ok := CatchUpNext(cur, list())
	expect(t, len(list()) == 0 && !ok, "All caught up")
	save(t, store, "2026-10-05", DefaultTemplate)
	expect(t, equal(list(), []string{"2026-10-05"}), "an emptied day returns to the list")
}

func TestSkipDays30DaysAreFast(t *testing.T) {
	store := newStore(t)
	var days []string
	for i := 0; i < 30; i++ {
		d, _ := DayKeyAdding("2026-09-01", i, nyLoc)
		days = append(days, d)
	}
	t0 := time.Now()
	r := val(store.SkipDays(days, "Leave"))
	skipT := time.Since(t0)
	expect(t, equal(r, days) && skipT < 250*time.Millisecond, "SkipDays(30) took %v", skipT)
	t1 := time.Now()
	u := val(store.UnskipDays(days))
	unT := time.Since(t1)
	expect(t, equal(u, days) && unT < 250*time.Millisecond, "UnskipDays(30) took %v", unT)
}
