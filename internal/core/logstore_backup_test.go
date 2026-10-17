package core

// Ported from tests/main.swift: "backups" (the safety copies LogStore.Save keeps of a page before it overwrites or deletes it).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// bkEnv is a store with backups on and a clock the test moves by hand (the Swift tests had a global fakeNow).
type bkEnv struct {
	store *LogStore
	bdir  string
	now   *time.Time
}

func backupStore(t *testing.T, interval time.Duration) bkEnv {
	t.Helper()
	now := mk(2026, 10, 6, 12, 0) // 16:00:00 UTC (New York is on EDT)
	bdir := t.TempDir()
	return bkEnv{store: NewLogStore(t.TempDir(), bdir, interval, func() time.Time { return now }), bdir: bdir, now: &now}
}

func (e bkEnv) advance(d time.Duration) { *e.now = e.now.Add(d) }

func bkNames(t *testing.T, bdir, day string) []string {
	names, _ := os.ReadDir(filepath.Join(bdir, day))
	var out []string
	for _, n := range names {
		out = append(out, n.Name())
	}
	sort.Strings(out)
	return out
}

func bkText(t *testing.T, b BackupInfo) string {
	t.Helper()
	data, err := os.ReadFile(b.Path)
	must(t, err)
	return string(data)
}

// bk is a safe accessor: a missing backup becomes a placeholder that makes the next call fail instead of crashing the test run.
func bk(l []BackupInfo, i int) BackupInfo {
	if i >= 0 && i < len(l) {
		return l[i]
	}
	return fakeBackup("00000000-000000.md")
}

func fakeBackup(name string) BackupInfo {
	return BackupInfo{Date: time.Now(), Path: filepath.Join("/tmp", name)}
}

func wordsList(l []BackupInfo) []int {
	var w []int
	for _, b := range l {
		w = append(w, b.Words)
	}
	return w
}

func TestBackupOverwriteCopiesThePreviousFileFirst(t *testing.T) {
	e := backupStore(t, 0)
	store, day := e.store, "2026-10-06"
	o := save(t, store, day, "first version "+lorem(25, "w"))
	expect(t, o == SaveOutcome{} && len(store.ListBackups(day)) == 0, "nothing existed, nothing to back up")
	expect(t, !pathExists(filepath.Join(e.bdir, day)), "no backup folder before the first backup")
	v1 := fileText(t, store.Dir, day)
	e.advance(30 * time.Second)
	o = save(t, store, day, "second version "+lorem(30, "w"))
	expect(t, o.BackedUp && o.BackupWarning == "" && store.LastBackupWarning() == "", "outcome %+v", o)
	list := store.ListBackups(day)
	expect(t, len(list) == 1 && bk(list, 0).Words == 27 && bk(list, 0).Date.Equal(*e.now), "list %+v", list)
	expect(t, bkText(t, bk(list, 0)) == v1, "byte-for-byte copy of the replaced file")
	expect(t, equal(bkNames(t, e.bdir, day), []string{"20261006-160030.md"}), "UTC time stamp: %v", bkNames(t, e.bdir, day))
	expect(t, strings.Contains(fileText(t, store.Dir, day), "second version"), "new text is in place")
	e.advance(60 * time.Second)
	v2 := fileText(t, store.Dir, day)
	save(t, store, day, "third "+lorem(5, "w"))
	list = store.ListBackups(day)
	expect(t, equal(wordsList(list), []int{32, 27}) && bkText(t, bk(list, 0)) == v2 && bk(list, 0).Date.After(bk(list, 1).Date), "newest first: %v", wordsList(list))
	save(t, store, "2026-10-07", "a b c")
	save(t, store, "2026-10-07", "a b c d")
	expect(t, len(store.ListBackups("2026-10-07")) == 1 && len(store.ListBackups(day)) == 2, "one folder per day")
	names := dirNames(t, e.bdir)
	sort.Strings(names)
	expect(t, equal(names, []string{"2026-10-06", "2026-10-07"}), "backup folders %v", names)
}

func TestBackupDeletingAPageKeepsACopy(t *testing.T) {
	e := backupStore(t, 0)
	store, day := e.store, "2026-10-06"
	save(t, store, day, "keep me "+lorem(25, "w"))
	before := fileText(t, store.Dir, day)
	o := save(t, store, day, " \n\n ")
	expect(t, o.BackedUp && o.BackupWarning == "" && load(t, store, day) == nil, "file deleted, outcome %+v", o)
	list := store.ListBackups(day)
	expect(t, len(list) == 1 && bkText(t, bk(list, 0)) == before && bk(list, 0).Words == 27, "copy of the deleted page")
	writeRaw(t, store.Dir, "2026-10-08", "# 2026-10-08\n")
	o = save(t, store, "2026-10-08", "")
	expect(t, !o.BackedUp && load(t, store, "2026-10-08") == nil && len(store.ListBackups("2026-10-08")) == 0, "title-only file: nothing to keep")
	must(t, store.Skip("2026-10-09", "x"))
	o = save(t, store, "2026-10-09", "")
	expect(t, !o.BackedUp && load(t, store, "2026-10-09").IsSkipped && len(store.ListBackups("2026-10-09")) == 0, "skip marker is neither copied nor deleted")
	o = save(t, store, "2026-10-10", "")
	expect(t, !o.BackedUp && len(store.ListBackups("2026-10-10")) == 0, "no file at all")
	expect(t, equal(dirNames(t, e.bdir), []string{"2026-10-06"}), "only the one day has backups: %v", dirNames(t, e.bdir))
}

func TestBackupOnlyTheNewest10PerDayAreKept(t *testing.T) {
	e := backupStore(t, 0)
	store, day := e.store, "2026-10-06"
	expect(t, MaxBackupsPerDay == 10, "constant")
	save(t, store, "2026-10-07", "other day v1")
	save(t, store, "2026-10-07", "other day v2")
	for i := 1; i <= 15; i++ { // the clock never moves
		save(t, store, day, "version "+itoa(i)+" "+lorem(25, "w"))
	}
	list := store.ListBackups(day)
	expect(t, len(list) == 10 && len(bkNames(t, e.bdir, day)) == 10, "%d backups", len(list))
	expect(t, strings.Contains(bkText(t, bk(list, 0)), "version 14 ") && strings.Contains(bkText(t, bk(list, len(list)-1)), "version 5 "), "versions 5...14 survive")
	for i := 1; i < len(list); i++ {
		expect(t, list[i-1].Date.After(list[i].Date), "strictly newest first")
	}
	expect(t, strings.Contains(fileText(t, store.Dir, day), "version 15 "), "latest is in place")
	expect(t, len(store.ListBackups("2026-10-07")) == 1, "pruning is per day")
}

func TestBackupIdenticalContentIsANoOp(t *testing.T) {
	e := backupStore(t, 0)
	store, day := e.store, "2026-10-06"
	save(t, store, day, "same text "+lorem(25, "w"))
	mtime := func() time.Time {
		fi, err := os.Stat(store.Path(day))
		must(t, err)
		return fi.ModTime()
	}
	m0 := mtime()
	time.Sleep(50 * time.Millisecond)
	for _, b := range []string{"same text " + lorem(25, "w"), "same text " + lorem(25, "w") + "\n\n  ", "\n\nsame text " + lorem(25, "w")} {
		o := save(t, store, day, b)
		expect(t, o == SaveOutcome{}, "identical after normalisation: %+v", o)
	}
	expect(t, len(store.ListBackups(day)) == 0 && mtime().Equal(m0), "no copy and the file was not rewritten")
	save(t, store, day, "changed "+lorem(25, "w"))
	expect(t, len(store.ListBackups(day)) == 1, "one copy")
	save(t, store, day, "changed "+lorem(25, "w"))
	expect(t, len(store.ListBackups(day)) == 1, "saving the same thing again adds nothing")
	must(t, store.Skip("2026-10-07", "Holiday"))
	o := save(t, store, "2026-10-07", "worked anyway "+lorem(25, "w"))
	expect(t, !o.BackedUp && len(store.ListBackups("2026-10-07")) == 0 && !load(t, store, "2026-10-07").IsSkipped, "a skip marker is not worth a copy")
	writeRaw(t, store.Dir, "2026-10-08", "# 2026-10-08\n\n")
	o = save(t, store, "2026-10-08", "now text")
	expect(t, !o.BackedUp && len(store.ListBackups("2026-10-08")) == 0, "title-only file")
	legacy := "# 2026-10-09\n\n## 📝 What I did\nWrote code\n\n## ✅ Finished\nShipped it\n\n"
	writeRaw(t, store.Dir, "2026-10-09", legacy)
	o2 := save(t, store, "2026-10-09", load(t, store, "2026-10-09").Body)
	expect(t, o2.BackedUp && bkText(t, bk(store.ListBackups("2026-10-09"), 0)) == legacy, "re-normalising an old file keeps its original bytes")
}

func TestBackupFailuresNeverFailTheSave(t *testing.T) {
	now := mk(2026, 10, 6, 12, 0)
	clock := func() time.Time { return now }
	day := "2026-10-06"
	blocker := filepath.Join(t.TempDir(), "blocker")
	must(t, os.WriteFile(blocker, []byte("i am a file"), 0o644))
	a := NewLogStore(t.TempDir(), blocker, 0, clock)
	save(t, a, day, "v1 "+lorem(25, "w"))
	oa := save(t, a, day, "v2 "+lorem(25, "w"))
	expect(t, !oa.BackedUp && oa.BackupWarning != "" && a.LastBackupWarning() == oa.BackupWarning, "outcome %+v", oa)
	expect(t, strings.Contains(fileText(t, a.Dir, day), "v2 ") && len(a.ListBackups(day)) == 0, "backup dir that is a file: the save still happened")

	e := backupStore(t, 0)
	b, bdir := e.store, e.bdir
	save(t, b, day, "v1 "+lorem(25, "w"))
	chmod(t, bdir, 0o555)
	defer chmod(t, bdir, 0o755)
	o2 := save(t, b, day, "v2 "+lorem(25, "w"))
	expect(t, !o2.BackedUp && o2.BackupWarning != "" && b.LastBackupWarning() == o2.BackupWarning, "outcome %+v", o2)
	expect(t, strings.Contains(fileText(t, b.Dir, day), "v2 "), "unwritable backup dir: the save itself went through")
	o3 := save(t, b, day, "")
	expect(t, !o3.BackedUp && o3.BackupWarning != "" && load(t, b, day) == nil, "deleting is also never blocked by a failed backup")
	chmod(t, bdir, 0o755)
	save(t, b, day, "v3 "+lorem(25, "w"))
	o4 := save(t, b, day, "v4 "+lorem(25, "w"))
	expect(t, o4.BackedUp && o4.BackupWarning == "" && b.LastBackupWarning() == "" && len(b.ListBackups(day)) == 1, "recovers on its own and clears the warning: %+v", o4)
	v4 := fileText(t, b.Dir, day)
	old := bk(b.ListBackups(day), 0)
	dayDir := filepath.Join(bdir, day)
	chmod(t, dayDir, 0o555)
	_, err := b.RestoreBackup(day, old)
	expectErr(t, err, ErrBackupFailed, "-")
	expect(t, fileText(t, b.Dir, day) == v4 && b.LastBackupWarning() != "", "nothing was changed")
	chmod(t, dayDir, 0o755)
	_, err = b.RestoreBackup(day, old)
	must(t, err)
	expect(t, strings.Contains(fileText(t, b.Dir, day), "v3 ") && b.LastBackupWarning() == "", "works again once the folder is writable")
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	e := backupStore(t, 0)
	store, day := e.store, "2026-10-06"
	var raws []string
	for i := 1; i <= 3; i++ {
		e.advance(10 * time.Second)
		save(t, store, day, "v"+itoa(i)+" "+lorem(25, "w"))
		raws = append(raws, fileText(t, store.Dir, day))
	}
	list := store.ListBackups(day) // [v2, v1]
	expect(t, len(list) == 2 && bkText(t, bk(list, 0)) == raws[1] && bkText(t, bk(list, 1)) == raws[0], "two backups")
	e.advance(10 * time.Second)
	doc := val(store.RestoreBackup(day, bk(list, 1))) // back to v1
	expect(t, doc.Day == day && strings.HasPrefix(doc.Body, "v1 ") && fileText(t, store.Dir, day) == raws[0] && equal(load(t, store, day), &doc), "restored v1")
	list = store.ListBackups(day) // [v3 (the safety copy), v2, v1]
	expect(t, len(list) == 3 && bkText(t, bk(list, 0)) == raws[2], "the current file was copied first")
	e.advance(10 * time.Second)
	val(store.RestoreBackup(day, bk(list, 0))) // undo the restore
	expect(t, fileText(t, store.Dir, day) == raws[2], "undone")
	list = store.ListBackups(day) // [v1 (safety copy), v3, v2, v1]
	expect(t, len(list) == 4 && bkText(t, bk(list, 0)) == raws[0], "four backups")
	val(store.RestoreBackup(day, bk(list, 1))) // identical to the current file
	expect(t, len(store.ListBackups(day)) == 4 && fileText(t, store.Dir, day) == raws[2], "identical: no write, no copy")
	// over a deleted page and over a skip marker
	save(t, store, day, "")
	expect(t, load(t, store, day) == nil, "deleted")
	list = store.ListBackups(day)
	val(store.RestoreBackup(day, bk(list, 0)))
	expect(t, fileText(t, store.Dir, day) == raws[2], "a deleted page comes back")
	save(t, store, "2026-10-07", "s1 "+lorem(25, "w"))
	save(t, store, "2026-10-07", "s2 "+lorem(25, "w"))
	s2 := fileText(t, store.Dir, "2026-10-07")
	save(t, store, "2026-10-07", "")
	must(t, store.Skip("2026-10-07", "x"))
	sl := store.ListBackups("2026-10-07") // [s2, s1]
	val(store.RestoreBackup("2026-10-07", bk(sl, 0)))
	expect(t, fileText(t, store.Dir, "2026-10-07") == s2 && len(store.ListBackups("2026-10-07")) == 2, "a skip marker is replaced without a copy")
	// forged or foreign backups
	for _, bad := range []BackupInfo{fakeBackup("passwd"), fakeBackup("20260101-000000.md"), fakeBackup("../x.md"),
		{Date: time.Now(), Path: filepath.Dir(bk(list, 0).Path)}} {
		_, err := store.RestoreBackup(day, bad)
		expectErr(t, err, ErrIO, "-")
	}
	_, err := store.RestoreBackup("2026-10-09", bk(list, 0))
	expectErr(t, err, ErrIO, "-") // a name is only looked up inside the given day's folder
	_, err = store.RestoreBackup("../x", bk(list, 0))
	expectErr(t, err, ErrBadDay, "../x")
	plain := newStore(t)
	_, err = plain.RestoreBackup(day, bk(list, 0))
	expectErr(t, err, ErrBackupsDisabled, "")
}

func TestBackupsAreNeverListedSearchedOrCounted(t *testing.T) {
	for _, nested := range []bool{false, true} {
		now := mk(2026, 10, 6, 12, 0)
		dir := t.TempDir()
		bdir := t.TempDir()
		if nested {
			bdir = filepath.Join(dir, "backups")
		}
		store := NewLogStore(dir, bdir, 0, func() time.Time { return now })
		save(t, store, "2026-10-06", "zebra "+lorem(25, "w"))
		save(t, store, "2026-10-06", "giraffe "+lorem(25, "w"))
		save(t, store, "2026-10-07", "zebra again "+lorem(25, "w"))
		save(t, store, "2026-10-07", "okapi "+lorem(25, "w"))
		expect(t, len(store.ListBackups("2026-10-06")) == 1 && pathExists(bdir), "backup written")
		expect(t, equal(val(store.ListDays()), []string{"2026-10-07", "2026-10-06"}), "nested=%v", nested)
		expect(t, equal(val(store.FileStates(20)), states("2026-10-06", StatusLogged, "2026-10-07", StatusLogged)), "states")
		zebra := val(store.Search("zebra", nil))
		giraffe := val(store.Search("giraffe", nil))
		okapi := val(store.Search("okapi", nil))
		expect(t, len(zebra) == 0 && len(giraffe) == 1 && giraffe[0].Day == "2026-10-06" && len(okapi) == 1, "old text lives only in backups")
		docs := val(store.Documents("2000-01-01", "2100-01-01"))
		expect(t, len(val(store.AllDocuments())) == 2 && len(docs) == 2, "documents")
		expect(t, val(store.FolderSummary()) == FolderSummary{Logs: 2}, "nested=%v summary %+v", nested, val(store.FolderSummary()))
		expect(t, strings.HasPrefix(load(t, store, "2026-10-06").Body, "giraffe"), "latest text")
	}
}

func TestBackupUTCNamesJunkClockSetBackThrottleOffByDefault(t *testing.T) {
	// DST fall-back: 01:30 happens twice in New York; the UTC names stay distinct and ordered
	t1 := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	expect(t, t1.In(nyLoc).Hour() == 1 && t2.In(nyLoc).Hour() == 1 && t2.In(nyLoc).Minute() == 30, "same wall-clock time twice")
	e := backupStore(t, 0)
	store, bdir, day := e.store, e.bdir, "2026-11-01"
	*e.now = t1.Add(-time.Minute)
	save(t, store, day, "a "+lorem(25, "w"))
	*e.now = t1
	save(t, store, day, "b "+lorem(25, "w"))
	*e.now = t2
	save(t, store, day, "c "+lorem(25, "w"))
	expect(t, equal(bkNames(t, bdir, day), []string{"20261101-053000.md", "20261101-063000.md"}), "%v", bkNames(t, bdir, day))
	l := store.ListBackups(day)
	expect(t, len(l) == 2 && l[0].Date.Equal(t2) && l[1].Date.Equal(t1), "dates")
	// junk next to real backups; a valid name holding non-UTF-8 bytes lists with 0 words
	dayDir := filepath.Join(bdir, day)
	for _, junk := range []string{"notes.txt", ".DS_Store", "20261101-053000.md.tmp", "bad-name.md", "20261301-160000.md", "2026110-0530000.md", "20261101_053000.md"} {
		must(t, os.WriteFile(filepath.Join(dayDir, junk), []byte("x"), 0o644))
	}
	must(t, os.WriteFile(filepath.Join(dayDir, "20260101-000000.md"), []byte{0xFF, 0xFE, 0xFD}, 0o644))
	l0 := store.ListBackups(day)
	expect(t, len(l0) == 3 && bk(l0, 2).Words == 0 && bk(l0, 2).Date.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), "%d backups listed", len(l0))
	expect(t, len(store.ListBackups("../x")) == 0 && len(store.ListBackups("nope")) == 0 && len(store.ListBackups("2026-12-31")) == 0, "bad or unknown days")
	_, err := store.RestoreBackup(day, bk(l0, 2))
	expectErr(t, err, ErrIO, "-") // non-UTF-8 backup
	// a clock set back never makes a new copy sort older (and so prunable)
	*e.now = t2.Add(-72 * time.Hour)
	save(t, store, day, "d "+lorem(25, "w"))
	l1 := store.ListBackups(day)
	expect(t, len(l1) == 4 && bk(l1, 0).Date.Equal(t2.Add(time.Second)) && strings.Contains(bkText(t, bk(l1, 0)), "c "), "newest copy is still first")
	// throttle: overwrites skip the copy inside the interval; deletes and restores always copy
	te := backupStore(t, 60*time.Second)
	ts, d2 := te.store, "2026-10-06"
	save(t, ts, d2, "v1 "+lorem(25, "w"))
	te.advance(5 * time.Second)
	save(t, ts, d2, "v2 "+lorem(25, "w"))
	te.advance(5 * time.Second)
	skipped := save(t, ts, d2, "v3 "+lorem(25, "w"))
	expect(t, !skipped.BackedUp && skipped.BackupWarning == "" && len(ts.ListBackups(d2)) == 1 && strings.Contains(bkText(t, bk(ts.ListBackups(d2), 0)), "v1 "), "inside the interval")
	te.advance(120 * time.Second)
	save(t, ts, d2, "v4 "+lorem(25, "w"))
	expect(t, len(ts.ListBackups(d2)) == 2 && strings.Contains(bkText(t, bk(ts.ListBackups(d2), 0)), "v3 "), "interval over: copy of v3")
	te.advance(time.Second)
	save(t, ts, d2, "")
	expect(t, len(ts.ListBackups(d2)) == 3 && strings.Contains(bkText(t, bk(ts.ListBackups(d2), 0)), "v4 "), "delete ignores the interval")
	te.advance(time.Second)
	val(ts.RestoreBackup(d2, bk(ts.ListBackups(d2), 0)))
	te.advance(time.Second)
	val(ts.RestoreBackup(d2, bk(ts.ListBackups(d2), 2)))
	expect(t, len(ts.ListBackups(d2)) == 4, "restore ignores the interval: the current page was copied before the oldest backup came back")
	// off by default: nothing is created, nothing breaks
	off := newStore(t)
	save(t, off, "2026-10-06", "a "+lorem(25, "w"))
	expect(t, save(t, off, "2026-10-06", "b "+lorem(25, "w")) == SaveOutcome{} && len(off.ListBackups("2026-10-06")) == 0 && off.LastBackupWarning() == "" && off.BackupDir == "", "backups off")
	save(t, off, "2026-10-06", "")
	expect(t, load(t, off, "2026-10-06") == nil && len(dirNames(t, off.Dir)) == 0, "deleted, nothing left")
	_, err = off.RestoreBackup("2026-10-06", fakeBackup("20260101-000000.md"))
	expectErr(t, err, ErrBackupsDisabled, "")
}
