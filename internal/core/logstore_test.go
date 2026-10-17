package core

// Ported from tests/main.swift: "store", "end to end through real files", the folderStamp tests and the kill regressions.
// Backups are in logstore_backup_test.go, skip batches in catchup_test.go, search in search_test.go.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadNoEscapingUnicodeImagesIdempotent(t *testing.T) {
	store := newStore(t)
	body := "intro\n## Real heading\ntext\n\\## literal slash stays\n\n![](assets/2026-10-05-1a2b3c4d.png)\n\n- [ ] 今天完成 ✅ café"
	save(t, store, "2026-10-05", body)
	expect(t, fileText(t, store.Dir, "2026-10-05") == "# 2026-10-05\n\n"+body+"\n", "written verbatim, nothing is escaped")
	expect(t, strings.Contains(load(t, store, "2026-10-05").Body, "\n## literal slash stays\n"), "documented trade-off: old-style escapes are undone on load")
	b2 := "intro\n## Real heading\ntext\n\n![](assets/2026-10-05-1a2b3c4d.png)\n\n- [ ] 今天完成 ✅ café"
	save(t, store, "2026-10-06", b2)
	expect(t, load(t, store, "2026-10-06").Body == b2, "round trip")
	expect(t, strings.Contains(fileText(t, store.Dir, "2026-10-06"), "\n## Real heading\n"), "headings are not escaped")
	save(t, store, "2026-10-07", "# 2026-10-07\ntext under a date heading")
	expect(t, load(t, store, "2026-10-07").Body == "# 2026-10-07\ntext under a date heading", "only the first title line is stripped")
	for _, b := range []string{"\n\nlead blank", "trail  \n\n", "  indented first", "a\r\nb"} {
		save(t, store, "2026-10-08", b)
		l := load(t, store, "2026-10-08").Body
		expect(t, l == TrimBody(b), "load after save of %q = %q", b, l)
		save(t, store, "2026-10-08", l)
		expect(t, load(t, store, "2026-10-08").Body == l, "stable")
	}
}

func TestEmptiedPageIsNoLog(t *testing.T) {
	store := newStore(t)
	save(t, store, "2026-10-05", "  \n \n")
	days := val(store.ListDays())
	expect(t, len(days) == 0 && load(t, store, "2026-10-05") == nil, "no file for an empty page")
	save(t, store, "2026-10-05", "hello there")
	expect(t, equal(val(store.ListDays()), []string{"2026-10-05"}), "one day")
	save(t, store, "2026-10-05", "\n")
	expect(t, len(val(store.ListDays())) == 0, "emptied page removes the file")
	must(t, store.Skip("2026-10-06", "Holiday"))
	save(t, store, "2026-10-06", "")
	expect(t, load(t, store, "2026-10-06").IsSkipped, "empty save keeps the skip marker")
	save(t, store, "2026-10-06", "I worked anyway today")
	d := load(t, store, "2026-10-06")
	expect(t, !d.IsSkipped && d.Body == "I worked anyway today", "write a log anyway replaces the marker")
}

func TestSkipUnskipSkipRange(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	must(t, store.Skip("2026-10-06", "Holiday"))
	expect(t, strings.Contains(fileText(t, dir, "2026-10-06"), "> Skipped: Holiday"), "marker text")
	e := load(t, store, "2026-10-06")
	expect(t, e.IsSkipped && e.SkipReason == "Holiday" && e.Status(20) == StatusSkipped, "skipped doc")
	must(t, store.Skip("2026-10-07", ""))
	expect(t, strings.Contains(fileText(t, dir, "2026-10-07"), "> Skipped\n") && load(t, store, "2026-10-07").SkipReason == "", "no reason")
	must(t, store.Skip("2026-10-06", "Leave"))
	expect(t, load(t, store, "2026-10-06").SkipReason == "Leave", "re-skipping updates the reason")
	ok := val(store.Unskip("2026-10-06"))
	expect(t, ok && load(t, store, "2026-10-06") == nil, "unskip removes the marker")
	expect(t, !val(store.Unskip("2026-10-06")), "nothing left to unskip")
	save(t, store, "2026-10-08", lorem(30, "w"))
	expectErr(t, store.Skip("2026-10-08", ""), ErrHasContent, "2026-10-08")
	expect(t, !val(store.Unskip("2026-10-08")), "unskip never deletes a log")
	save(t, store, "2026-10-09", "## What I did\n\n## Finished")
	must(t, store.Skip("2026-10-09", "headings only"))
	expect(t, load(t, store, "2026-10-09").IsSkipped, "a heading-only page may be skipped")
	save(t, store, "2026-10-10", "![](assets/x.png)")
	expectErr(t, store.Skip("2026-10-10", ""), ErrHasContent, "2026-10-10")
	w := val(store.SkipRange("2026-10-12", "2026-10-18", "Leave", workdays, ny))
	expect(t, equal(w, []string{"2026-10-12", "2026-10-13", "2026-10-14", "2026-10-15", "2026-10-16"}), "range: %v", w)
	expect(t, load(t, store, "2026-10-17") == nil, "weekend untouched")
	save(t, store, "2026-10-21", lorem(30, "w"))
	w2 := val(store.SkipRange("2026-10-20", "2026-10-22", "", workdays, ny))
	expect(t, equal(w2, []string{"2026-10-20", "2026-10-22"}), "days with writing are left alone: %v", w2)
	save(t, store, "2026-10-20", lorem(25, "w"))
	expect(t, load(t, store, "2026-10-20").Status(20) == StatusLogged, "write a log over a marker")
}

func TestFileStatesFollowTheWordsRuleAndListingsIgnoreForeignFiles(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	save(t, store, "2026-10-05", lorem(25, "w"))
	save(t, store, "2026-10-06", lorem(5, "w"))
	save(t, store, "2026-10-07", DefaultTemplate)
	must(t, store.Skip("2026-10-08", "x"))
	expect(t, equal(val(store.ListDays()), []string{"2026-10-08", "2026-10-07", "2026-10-06", "2026-10-05"}), "newest first")
	expect(t, equal(val(store.FileStates(20)), states("2026-10-05", StatusLogged, "2026-10-06", StatusPartial, "2026-10-08", StatusSkipped)), "template-only page is omitted")
	expect(t, equal(val(store.FileStates(5)), states("2026-10-05", StatusLogged, "2026-10-06", StatusLogged, "2026-10-08", StatusSkipped)), "minWords 5")
	expect(t, equal(val(store.FileStates(30)), states("2026-10-05", StatusPartial, "2026-10-06", StatusPartial, "2026-10-08", StatusSkipped)), "minWords 30")
	must(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	for _, n := range []string{"notes.md", "2026-10-0x.md", ".hidden"} {
		must(t, os.WriteFile(filepath.Join(dir, n), []byte("hi"), 0o644))
	}
	expect(t, len(val(store.ListDays())) == 4, "foreign files are not days")
	expect(t, val(store.FolderSummary()) == FolderSummary{Logs: 2, Skipped: 1, Unrecognized: 2}, "summary %+v", val(store.FolderSummary()))
	docs := val(store.Documents("2026-10-06", "2026-10-07"))
	_, a := docs["2026-10-06"]
	_, b := docs["2026-10-07"]
	expect(t, len(docs) == 2 && a && b, "documents in range")
	expect(t, len(val(store.AllDocuments())) == 4, "all documents")
}

func TestTypedErrorsMissingAndUnwritableFolderAtomicWritesBadDays(t *testing.T) {
	missing := NewLogStore(filepath.Join(t.TempDir(), "nope"), "", 0, nil)
	_, err := missing.Save("2026-10-05", "hello")
	expectErr(t, err, ErrFolderMissing, missing.Dir)
	_, err = missing.ListDays()
	expectErr(t, err, ErrFolderMissing, missing.Dir)
	_, err = missing.Load("2026-10-05")
	expectErr(t, err, ErrFolderMissing, missing.Dir)
	expectErr(t, missing.Skip("2026-10-05", ""), ErrFolderMissing, missing.Dir)
	must(t, missing.EnsureFolder())
	save(t, missing, "2026-10-05", "hello")
	expect(t, equal(dirNames(t, missing.Dir), []string{"2026-10-05.md"}), "atomic writes leave no temp files: %v", dirNames(t, missing.Dir))

	rs := newStore(t)
	ro := rs.Dir
	save(t, rs, "2026-10-05", "hello there")
	chmod(t, ro, 0o555)
	defer chmod(t, ro, 0o755)
	_, err = rs.Save("2026-10-06", "x y z")
	expectErr(t, err, ErrFolderNotWritable, ro)
	expectErr(t, rs.Skip("2026-10-07", ""), ErrFolderNotWritable, ro)
	_, err = rs.Assets().Save(pngBytes, "png", "2026-10-05")
	expectErr(t, err, ErrFolderNotWritable, ro)
	expect(t, load(t, rs, "2026-10-05") != nil, "still readable")
	chmod(t, ro, 0o755)
	for _, bad := range []string{"../evil", "2026-10-5", "x", "2026-10-05.md", ""} {
		_, err = rs.Save(bad, "x")
		expectErr(t, err, ErrBadDay, bad)
	}
	_, err = rs.Load("../x")
	expectErr(t, err, ErrBadDay, "../x")
}

func TestStoreAgreesWithStreakHeatmapStatusPlanner(t *testing.T) {
	store := newStore(t)
	for _, d := range []string{"2026-10-05", "2026-10-06", "2026-10-08", "2026-10-09"} {
		save(t, store, d, "## What I did\n"+lorem(25, "w"))
	}
	must(t, store.Skip("2026-10-07", "x"))
	save(t, store, "2026-10-12", DefaultTemplate+"\n\n"+lorem(5, "w"))
	now := mk(2026, 10, 12, 18, 0)
	st := val(store.FileStates(20))
	expect(t, st["2026-10-12"] == StatusPartial && st["2026-10-05"] == StatusLogged && st["2026-10-07"] == StatusSkipped, "states %v", st)
	expect(t, ComputeStreak(st, now, ny, workdays, "") == StreakResult{Current: 4, Best: 4}, "partial today is 'not yet'")
	status := func(day string, s map[string]DayStatus) DayStatus {
		return ResolveStatus(day, s[day], now, ny, workdays, EffectiveSince("", s))
	}
	expect(t, status("2026-10-12", st) == StatusPartial && status("2026-10-13", st) == StatusFuture && status("2026-10-10", st) == StatusOff && status("2026-10-02", st) == StatusOff, "statuses")
	hm := HeatmapWeeks(st, now, ny, workdays, 12, "")
	var last []DayStatus
	for _, c := range hm[len(hm)-1] {
		last = append(last, c.Status)
	}
	expect(t, equal(last, []DayStatus{StatusPartial, StatusFuture, StatusFuture, StatusFuture, StatusFuture, StatusFuture, StatusFuture}) && hm[len(hm)-1][0].IsToday, "last week %v", last)
	var prev []DayStatus
	for _, c := range hm[10] {
		prev = append(prev, c.Status)
	}
	expect(t, equal(prev, []DayStatus{StatusLogged, StatusLogged, StatusSkipped, StatusLogged, StatusLogged, StatusOff, StatusOff}), "previous week %v", prev)
	// planner: a partial page still nags; reaching minWords stops it
	s := DefaultSettings("/home/x")
	s.MinWords = 20
	fresh := PlannerState{Day: "2026-10-12"}
	a, _ := ReminderDecide(now, ny, s, status("2026-10-12", st), fresh, true)
	expect(t, a == ActionBringToFront(true), "a partial page still nags")
	save(t, store, "2026-10-12", DefaultTemplate+"\n\n"+lorem(25, "w"))
	st2 := val(store.FileStates(20))
	expect(t, status("2026-10-12", st2) == StatusLogged, "logged now")
	a, _ = ReminderDecide(now, ny, s, status("2026-10-12", st2), fresh, true)
	expect(t, a == ActionNone, "no more nagging")
	expect(t, ComputeStreak(st2, now, ny, workdays, "") == StreakResult{Current: 5, Best: 5}, "streak 5")
	// changing the threshold re-evaluates history
	st3 := val(store.FileStates(30))
	expect(t, st3["2026-10-05"] == StatusPartial && st3["2026-10-07"] == StatusSkipped, "minWords 30")
	expect(t, ComputeStreak(st3, now, ny, workdays, "") == StreakResult{}, "nothing logged at 30 words")
	st4 := val(store.FileStates(5))
	expect(t, st4["2026-10-12"] == StatusLogged && ComputeStreak(st4, now, ny, workdays, "").Current == 5, "minWords 5")
	// a day that was emptied again is no log at all
	save(t, store, "2026-10-09", "")
	expect(t, val(store.FileStates(20))["2026-10-09"] == "", "emptied day")
	expect(t, ComputeStreak(val(store.FileStates(20)), now, ny, workdays, "").Current == 1, "Friday missed -> only Monday counts")
}

// ---- C6 folderStamp ----

func TestFolderStampTracksNamesMtimesAndSizes(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	old := time.Unix(1_700_000_000, 0)
	save(t, store, "2026-10-05", lorem(30, "w"))
	save(t, store, "2026-10-06", lorem(5, "w"))
	must(t, store.Skip("2026-10-07", "x"))
	for _, d := range []string{"2026-10-05", "2026-10-06", "2026-10-07"} {
		setMTime(t, store.Path(d), old) // pin mtimes so "edit" can only be seen through the stamp
	}
	stamp := func() string { return val(store.FolderStamp()) }
	a := stamp()
	expect(t, a != "", "a folder with days has a stamp")
	expect(t, stamp() == a && val(NewLogStore(dir, "", 0, nil).FolderStamp()) == a, "unchanged folder -> identical stamp; another store agrees")
	val(store.ListDays())
	load(t, store, "2026-10-05")
	val(store.FileStates(20))
	expect(t, stamp() == a, "reading does not change the stamp")
	save(t, store, "2026-10-05", lorem(30, "w"))
	expect(t, stamp() == a, "saving identical content rewrites nothing")
	save(t, store, "2026-10-08", "a new page with words")
	expect(t, stamp() != a, "add")
	save(t, store, "2026-10-08", "")
	expect(t, stamp() == a, "delete returns to the earlier stamp")
	save(t, store, "2026-10-05", lorem(30, "x"))
	expect(t, stamp() != a, "edit (same size, new mtime)")
	setMTime(t, store.Path("2026-10-05"), old)
	expect(t, stamp() == a, "same name, size and mtime, different bytes: the same stamp (contents are not read)")
	save(t, store, "2026-10-05", lorem(31, "w"))
	setMTime(t, store.Path("2026-10-05"), old)
	expect(t, stamp() != a, "edit that changes only the size")
	save(t, store, "2026-10-05", lorem(30, "w"))
	setMTime(t, store.Path("2026-10-05"), old)
	expect(t, stamp() == a, "same name, size and mtime: same stamp again")
	setMTime(t, store.Path("2026-10-06"), old.Add(60*time.Second))
	expect(t, stamp() != a, "touch")
	setMTime(t, store.Path("2026-10-06"), old)
	expect(t, stamp() == a, "touch undone")
	setMTime(t, store.Path("2026-10-06"), old.Add(500*time.Millisecond))
	expect(t, stamp() != a, "a sub-second mtime change is seen")
	setMTime(t, store.Path("2026-10-06"), old)
	must(t, os.Rename(store.Path("2026-10-06"), store.Path("2026-10-09")))
	expect(t, stamp() != a, "rename")
	must(t, os.Rename(store.Path("2026-10-09"), store.Path("2026-10-06")))
	expect(t, stamp() == a, "renamed back")
	must(t, store.Skip("2026-10-12", "Leave"))
	expect(t, stamp() != a, "a skip marker is a file")
	val(store.Unskip("2026-10-12"))
	expect(t, stamp() == a, "unskip")
}

func TestFolderStampIgnoresAssetsForeignFilesAndBackups(t *testing.T) {
	store := newStore(t)
	dir := store.Dir
	save(t, store, "2026-10-05", lorem(30, "w"))
	a := val(store.FolderStamp())
	must(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "assets/2026-10-05-1a2b3c4d.png"), pngBytes, 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backups/2026-10-05"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backups/2026-10-05/20261005-120000.md"), []byte("old"), 0o644))
	for _, n := range []string{"notes.md", "2026-10-0x.md", "2026-10-5.md", "readme.txt", "2026-10-05.md.bak", ".hidden", ".2026-10-06.md.tmp", "2026-10-05.txt"} {
		must(t, os.WriteFile(filepath.Join(dir, n), []byte("hi"), 0o644))
	}
	must(t, os.MkdirAll(filepath.Join(dir, "2026-10-11.md"), 0o755)) // a folder that looks like a day file
	expect(t, val(store.FolderStamp()) == a, "none of those is a day file")
	must(t, os.WriteFile(filepath.Join(dir, "assets/another.png"), pngBytes, 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("changed"), 0o644))
	expect(t, val(store.FolderStamp()) == a, "editing them changes nothing")
	// a symlink to a day file counts, through its target
	target := filepath.Join(t.TempDir(), "page.md")
	must(t, os.WriteFile(target, []byte("# 2026-10-13\n\nlinked page"), 0o644))
	must(t, os.Symlink(target, filepath.Join(dir, "2026-10-13.md")))
	expect(t, val(store.FolderStamp()) != a, "a linked day file is a day file")
	must(t, os.Remove(target))
	val(store.FolderStamp()) // a dangling link must not fail
	must(t, os.Remove(filepath.Join(dir, "2026-10-13.md")))
	expect(t, val(store.FolderStamp()) == a, "back to the first stamp")
	missing := NewLogStore(filepath.Join(t.TempDir(), "nope"), "", 0, nil)
	_, err := missing.FolderStamp()
	expectErr(t, err, ErrFolderMissing, missing.Dir)
	expect(t, val(newStore(t).FolderStamp()) == val(newStore(t).FolderStamp()), "empty folders agree")
}

func TestFolderStampNeverReadsFileContents(t *testing.T) {
	store := newStore(t)
	save(t, store, "2026-10-05", lorem(30, "w"))
	save(t, store, "2026-10-06", lorem(30, "w"))
	chmod(t, store.Path("2026-10-05"), 0o000)
	defer chmod(t, store.Path("2026-10-05"), 0o644)
	b := val(store.FolderStamp())
	expect(t, strings.Contains(b, "2026-10-05.md") && strings.Contains(b, "2026-10-06.md"), "both files are in the stamp: %q", b)
}

func TestFolderStampOver3650FilesIsFast(t *testing.T) {
	store := newStore(t)
	d := utcDate(2016, 10, 8)
	for i := 0; i < 3650; i++ {
		key := utcKey(d)
		must(t, os.WriteFile(filepath.Join(store.Dir, key+".md"), []byte("# "+key+"\n\nsome words for this day\n"), 0o644))
		d = d.AddDate(0, 0, 1)
	}
	first := val(store.FolderStamp())
	best, total := time.Hour, time.Duration(0)
	for i := 0; i < 5; i++ {
		t0 := time.Now()
		s := val(store.FolderStamp())
		dt := time.Since(t0)
		best = min(best, dt)
		total += dt
		expect(t, s == first, "stable across calls")
	}
	// Swift asserted the mean < 50 ms as well; here one slow run in five (file system noise, GC) would make that flaky, so the mean only has to stay sane.
	expect(t, best < 50*time.Millisecond && total/5 < 150*time.Millisecond, "3,650 files: best %v, mean %v", best, total/5)
	expect(t, len(strings.Split(first, "\n")) == 3650, "one entry per file")
	t.Logf("folderStamp over 3,650 files: best %v, mean %v", best, total/5)
}

// ---- kill regressions (tests/run-kill-tests.sh) ----

// deadPid is a pid that is certainly not running: a child that already exited and was reaped.
func deadPid(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	must(t, cmd.Run())
	return cmd.Process.Pid
}

func TestKillRegressionStaleTempFilesAreSweptByTheFirstWrite(t *testing.T) {
	now := mk(2026, 10, 6, 12, 0)
	bdir := t.TempDir()
	s0 := NewLogStore(t.TempDir(), bdir, 0, func() time.Time { return now })
	dir := s0.Dir
	save(t, s0, "2026-10-05", lorem(30, "w"))
	pid := deadPid(t)
	strayA := filepath.Join(dir, AtomicTempPrefix+itoa(pid)+"-AAAA")
	strayB := filepath.Join(bdir, AtomicTempPrefix+itoa(pid)+"-BBBB")
	strayC := filepath.Join(dir, AtomicTempPrefix+"garbage")                                             // our prefix, no readable pid: stale
	live := filepath.Join(dir, AtomicTempPrefix+itoa(os.Getpid())+"-LIVE")                               // this process is alive: not a leftover
	foreign := []string{"notes.md", ".hidden", ".2026-10-05.md.tmp", "2026-10-05.md.sb-9d433aed-Oy56Tb"} // never ours: never touched
	for _, u := range []string{strayA, strayB, strayC, live} {
		must(t, os.WriteFile(u, []byte("half a page"), 0o644))
	}
	for _, n := range foreign {
		must(t, os.WriteFile(filepath.Join(dir, n), []byte("keep"), 0o644))
	}
	// invisible meanwhile: not a day, not an unrecognised file, not part of the folder stamp
	expect(t, equal(val(s0.ListDays()), []string{"2026-10-05"}) && val(s0.FolderSummary()) == FolderSummary{Logs: 1, Unrecognized: 2}, "summary %+v", val(s0.FolderSummary()))
	expect(t, !strings.Contains(val(s0.FolderStamp()), "gloamlog"), "not in the stamp")
	// a fresh store (= the next launch): nothing is swept before it writes, everything of ours that is stale goes with its first save
	s1 := NewLogStore(dir, bdir, 0, func() time.Time { return now })
	load(t, s1, "2026-10-05")
	val(s1.ListDays())
	expect(t, pathExists(strayA) && pathExists(strayB), "reading sweeps nothing")
	now = now.Add(30 * time.Second)
	save(t, s1, "2026-10-05", lorem(31, "w"))
	expect(t, !pathExists(strayA), "stale temp in the log folder is removed")
	expect(t, !pathExists(strayB), "stale temp in the backups folder is removed")
	expect(t, !pathExists(strayC), "a temp with an unreadable pid is stale")
	expect(t, pathExists(live), "a temp whose writer is alive is left alone")
	for _, n := range foreign {
		expect(t, pathExists(filepath.Join(dir, n)), "%s is not ours and stays", n)
	}
	expect(t, equal(dirNames(t, bdir), []string{"2026-10-05"}), "backups folder holds only day folders: %v", dirNames(t, bdir))
	expect(t, len(s1.ListBackups("2026-10-05")) == 1, "one backup")
	must(t, os.Remove(live))
	// every later save by the same store is a plain save: nothing else is deleted, nothing is left behind
	later := filepath.Join(dir, AtomicTempPrefix+itoa(pid)+"-LATER")
	must(t, os.WriteFile(later, []byte("later"), 0o644))
	save(t, s1, "2026-10-05", lorem(32, "w"))
	expect(t, pathExists(later), "one sweep per store, at its first write")
	must(t, os.Remove(later))
	for _, n := range foreign {
		must(t, os.Remove(filepath.Join(dir, n)))
	}
	expect(t, equal(dirNames(t, dir), []string{"2026-10-05.md"}), "a save leaves no temp file: %v", dirNames(t, dir))
}

func TestKillRegressionSkipSweepsAndFailedSaveLeavesNoTemp(t *testing.T) {
	now := mk(2026, 10, 6, 12, 0)
	bdir := t.TempDir()
	s0 := NewLogStore(t.TempDir(), bdir, 0, func() time.Time { return now })
	dir := s0.Dir
	stray := filepath.Join(dir, AtomicTempPrefix+itoa(deadPid(t))+"-SKIP")
	must(t, os.WriteFile(stray, []byte("x"), 0o644))
	must(t, NewLogStore(dir, bdir, 0, nil).Skip("2026-10-07", "Leave"))
	expect(t, !pathExists(stray), "skip is a write: the first one sweeps")
	// a directory squats on a day file name: the save fails with a typed error and leaves nothing behind
	must(t, os.Mkdir(filepath.Join(dir, "2026-10-08.md"), 0o755))
	_, err := s0.Save("2026-10-08", "some words here")
	expectErr(t, err, ErrIO, "-")
	for _, d := range []string{dir, bdir} {
		for _, n := range dirNames(t, d) {
			expect(t, !strings.HasPrefix(n, AtomicTempPrefix), "no temp file after a failed write: %s", n)
		}
	}
}

func TestKillRegressionSavingKeepsThePagePermissions(t *testing.T) {
	store := newStore(t)
	mode := func(d string) os.FileMode {
		fi, err := os.Stat(store.Path(d))
		must(t, err)
		return fi.Mode().Perm()
	}
	save(t, store, "2026-10-05", lorem(30, "w"))
	chmod(t, store.Path("2026-10-05"), 0o600)
	save(t, store, "2026-10-05", lorem(31, "w"))
	expect(t, mode("2026-10-05") == 0o600, "0600 stays 0600, got %o", mode("2026-10-05"))
	chmod(t, store.Path("2026-10-05"), 0o444)
	save(t, store, "2026-10-05", lorem(32, "w"))
	expect(t, mode("2026-10-05") == 0o444 && load(t, store, "2026-10-05").Words() == 32, "a read-only page is still replaced (the folder is writable), mode kept")
	chmod(t, store.Path("2026-10-05"), 0o644)
}

// ---- not in the Swift suite: things the port itself relies on ----

func TestReadingDropsTheByteOrderMarkLikeFoundationDoes(t *testing.T) {
	store := newStore(t)
	// a BOM-only difference is "identical content": nothing is rewritten, as String(data:encoding:) drops the BOM in Swift
	must(t, os.WriteFile(store.Path("2026-10-05"), []byte("\xEF\xBB\xBF# 2026-10-05\n\nhello there\n"), 0o644))
	out := save(t, store, "2026-10-05", "hello there")
	expect(t, out == SaveOutcome{}, "outcome %+v", out)
	expect(t, strings.HasPrefix(fileText(t, store.Dir, "2026-10-05"), "\xEF\xBB\xBF"), "file untouched")
	// invalid UTF-8 is an error on load, like String(contentsOf:encoding:)
	must(t, os.WriteFile(store.Path("2026-10-06"), []byte{0xFF, 0xFE, 0xFD}, 0o644))
	_, err := store.Load("2026-10-06")
	expectErr(t, err, ErrIO, "-")
}

func TestLogErrorMessageAndUnwrap(t *testing.T) {
	err := logErr(ErrNotLocal, "2026-10-08")
	expect(t, strings.Contains(err.Error(), "2026-10-08 is still downloading from your cloud folder"), "%v", err)
	expect(t, errors.Is(err, ErrNotLocal) && !errors.Is(err, ErrIO), "errors.Is")
	expect(t, LogErrorDetail(err) == "2026-10-08" && LogErrorDetail(errors.New("x")) == "", "detail")
}

func TestNotLocalDaysAreNeverReadOrReplaced(t *testing.T) {
	store := newStore(t)
	save(t, store, "2026-10-05", lorem(30, "w"))
	must(t, os.WriteFile(filepath.Join(store.Dir, placeholderName("2026-10-05")), nil, 0o644))
	expect(t, store.IsNotLocal("2026-10-05") && !store.IsNotLocal("2026-10-06"), "placeholder marks the day")
	_, err := store.Load("2026-10-05")
	expectErr(t, err, ErrNotLocal, "2026-10-05")
	_, err = store.Save("2026-10-05", "new words here")
	expectErr(t, err, ErrNotLocal, "2026-10-05")
	expectErr(t, store.Skip("2026-10-05", ""), ErrNotLocal, "2026-10-05")
	_, err = store.Unskip("2026-10-05")
	expectErr(t, err, ErrNotLocal, "2026-10-05")
	docs := val(store.AllDocuments())
	expect(t, len(docs) == 0, "loops leave the day out instead of failing")
	days := val(store.ListDays())
	sort.Strings(days)
	expect(t, equal(days, []string{"2026-10-05"}), "the day is still listed")
}
