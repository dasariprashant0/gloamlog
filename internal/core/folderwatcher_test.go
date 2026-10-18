package core

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func newTestWatcher(dir string) *FolderWatcher {
	return NewFolderWatcher(func() string { return dir }, time.Hour, func() {})
}

func TestFolderWatcherDetectsChangesAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "2026-10-08.md", "one")
	w := newTestWatcher(dir)

	if w.poll() {
		t.Error("the first look is only the baseline")
	}
	if w.poll() {
		t.Error("no event without a change")
	}

	touch(t, dir, "2026-10-09.md", "new day")
	if !w.poll() {
		t.Error("a new day file must be noticed")
	}
	if w.poll() {
		t.Error("the change was already reported")
	}

	touch(t, dir, "2026-10-09.md", "an edit with a different size")
	if !w.poll() {
		t.Error("an edited day file must be noticed")
	}

	touch(t, dir, "2026-10-09 2.md", "a conflict copy")
	if !w.poll() {
		t.Error("a conflict copy must be noticed")
	}

	touch(t, dir, ".2026-10-10.md.icloud", "")
	if !w.poll() {
		t.Error("an iCloud placeholder must be noticed")
	}

	must(t, os.Remove(filepath.Join(dir, "2026-10-09.md")))
	if !w.poll() {
		t.Error("a removed day file must be noticed")
	}
}

func TestFolderWatcherIgnoresWhatIsNotOurs(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(dir)
	w.poll()
	touch(t, dir, ".DS_Store", "x")
	touch(t, dir, ".gloamlog-tmp-1-X", "x")
	touch(t, dir, "notes.md", "x")
	touch(t, dir, "2026-10-08 standup.md", "x")
	must(t, os.Mkdir(filepath.Join(dir, "assets"), 0o755))
	touch(t, filepath.Join(dir, "assets"), "pic.png", "x")
	if w.poll() {
		t.Error("hidden files, assets/ and foreign files must not wake anybody")
	}
}

func TestFolderWatcherIgnoresOurOwnWrites(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(dir)
	w.poll()
	// AtomicWrite notes the file's signature through OwnWriteHook, so the echo of our own save wakes nobody
	must(t, AtomicWrite([]byte("our save"), filepath.Join(dir, "2026-10-08.md"), ""))
	if w.poll() {
		t.Error("our own write must not count as a change")
	}
	must(t, AtomicWrite([]byte("our second, longer save"), filepath.Join(dir, "2026-10-08.md"), ""))
	if w.poll() {
		t.Error("our own overwrite must not count as a change")
	}
	// somebody else touches the file afterwards: that is a change
	touch(t, dir, "2026-10-08.md", "written by the sync provider")
	if !w.poll() {
		t.Error("somebody else's write after ours must be noticed")
	}
}

func TestFolderWatcherFollowsAChangedFolder(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	cur := a
	w := NewFolderWatcher(func() string { return cur }, time.Hour, func() {})
	w.poll()
	cur = b
	if !w.poll() {
		t.Error("a different folder counts as a change")
	}
	if w.poll() {
		t.Error("and only once")
	}
}

func TestFolderWatcherCallsBackAndStops(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	w := NewFolderWatcher(func() string { return dir }, 5*time.Millisecond, func() { calls.Add(1) })
	w.Start()
	w.Start() // starting twice does nothing
	time.Sleep(40 * time.Millisecond)
	if n := calls.Load(); n != 0 {
		t.Fatalf("%d calls without a change", n)
	}
	touch(t, dir, "2026-10-08.md", "hello")
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("onChange was not called after a change")
	}
	w.Stop()
	w.Stop() // stopping twice does nothing
	n := calls.Load()
	touch(t, dir, "2026-10-09.md", "after stop")
	time.Sleep(40 * time.Millisecond)
	if calls.Load() != n {
		t.Error("onChange was called after Stop")
	}
}

func TestFolderWatcherEmptyAndMissingFolders(t *testing.T) {
	// an empty folder is a baseline like any other: its first day file is a change
	dir := t.TempDir()
	w := newTestWatcher(dir)
	if w.poll() || w.poll() {
		t.Error("an empty folder has nothing to report")
	}
	touch(t, dir, "2026-10-08.md", "first")
	if !w.poll() {
		t.Error("the first file in an empty folder must be noticed")
	}
	// a folder that is not there looks empty; its coming back with files is a change
	gone := filepath.Join(t.TempDir(), "later")
	w = newTestWatcher(gone)
	if w.poll() || w.poll() {
		t.Error("a missing folder has nothing to report")
	}
	must(t, os.Mkdir(gone, 0o755))
	touch(t, gone, "2026-10-08.md", "back")
	if !w.poll() {
		t.Error("a folder coming back with a day file must be noticed")
	}
	// an unset folder ("") is the empty case too
	w = NewFolderWatcher(func() string { return "" }, time.Hour, func() {})
	if w.poll() || w.poll() {
		t.Error("an empty folder path has nothing to report")
	}
}
