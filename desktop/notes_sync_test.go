package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// syncNotes is a Notes over temp folders: no real home, no real iCloud.
func syncNotes(t *testing.T) (*Notes, string, string) {
	t.Helper()
	dir, backups := t.TempDir(), t.TempDir()
	now := func() time.Time { return time.Date(2026, 10, 9, 10, 15, 30, 0, time.UTC) }
	n := &Notes{backupDir: backups, now: now, store: core.NewLogStore(dir, backups, time.Hour, now)}
	return n, dir, backups
}

func put(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncStateCountsConflictsAndDownloads(t *testing.T) {
	n, dir, _ := syncNotes(t)
	var mu sync.Mutex
	var asked []string
	old := requestDownload
	requestDownload = func(path string, iCloud bool) { mu.Lock(); asked = append(asked, filepath.Base(path)); mu.Unlock() }
	defer func() { requestDownload = old }()

	put(t, dir, "2026-10-08.md", core.SerializeDay("2026-10-08", "mine"))
	put(t, dir, "2026-10-08 2.md", core.SerializeDay("2026-10-08", "theirs"))
	put(t, dir, ".2026-10-05.md.icloud", "")
	put(t, dir, ".2026-10-04.md.icloud", "")

	st := n.SyncState()
	if st.Conflicts != 1 || len(st.ConflictDays) != 1 || st.ConflictDays[0] != "2026-10-08" {
		t.Errorf("conflicts = %d %v", st.Conflicts, st.ConflictDays)
	}
	if got := strings.Join(st.PendingDays, ","); got != "2026-10-04,2026-10-05" {
		t.Errorf("pending days = %q", got)
	}
	if got := strings.Join(st.BlockedDays, ","); got != "2026-10-04,2026-10-05" {
		t.Errorf("blocked days = %q", got)
	}
	if st.Kind != "local" || st.KindLabel != "Local" {
		t.Errorf("kind = %q %q", st.Kind, st.KindLabel)
	}
	// the downloads were asked for (on goroutines), and not again within 2 minutes
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(asked)
		mu.Unlock()
		if got == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	n.SyncState()
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(asked) != 2 {
		t.Errorf("download requests = %v, want one per file", asked)
	}
}

func TestSyncStateWhenNothingIsWaiting(t *testing.T) {
	n, dir, _ := syncNotes(t)
	put(t, dir, "2026-10-08.md", core.SerializeDay("2026-10-08", "mine"))
	st := n.SyncState()
	if st.Conflicts != 0 || len(st.PendingDays) != 0 || st.ConflictDays == nil || st.PendingDays == nil || st.BlockedDays == nil {
		t.Errorf("a quiet folder gave %+v (slices must be [] not null)", st)
	}
}

func TestSyncConflictsHaveBothVersionsAndResolve(t *testing.T) {
	n, dir, backups := syncNotes(t)
	put(t, dir, "2026-10-08.md", core.SerializeDay("2026-10-08", "shared\nmine only"))
	put(t, dir, "2026-10-08 2.md", core.SerializeDay("2026-10-08", "shared\ntheirs only"))

	items := n.SyncConflicts()
	if len(items) != 1 {
		t.Fatalf("%d conflicts", len(items))
	}
	it := items[0]
	if it.Error != "" || it.Mine == nil || it.Theirs == nil || !it.MineExists || !it.TheirsHasText || it.Family != "iCloudNumbered" {
		t.Fatalf("item = %+v", it)
	}
	if !strings.HasPrefix(it.Mine.Title, "This Mac, ") || !strings.HasSuffix(it.Mine.Title, ", 3 words") {
		t.Errorf("mine title = %q", it.Mine.Title)
	}
	if !strings.HasPrefix(it.Theirs.Title, "Other copy, ") {
		t.Errorf("theirs title = %q", it.Theirs.Title)
	}
	var minesOnly, theirsOnly bool
	for _, l := range it.Mine.Lines {
		minesOnly = minesOnly || (l.Text == "mine only" && l.Differs)
	}
	for _, l := range it.Theirs.Lines {
		theirsOnly = theirsOnly || (l.Text == "theirs only" && l.Differs)
	}
	if !minesOnly || !theirsOnly {
		t.Errorf("differing lines not marked: %+v / %+v", it.Mine.Lines, it.Theirs.Lines)
	}

	if err := n.ResolveConflict("../../etc/passwd", "keep-mine"); err == nil {
		t.Error("a name that is not a conflict copy of this folder must be refused")
	}
	if err := n.ResolveConflict(it.Name, "keep-both"); err != nil {
		t.Fatal(err)
	}
	if got := syncRead(t, filepath.Join(dir, "2026-10-08.md")); !strings.Contains(got, "## From another copy") || !strings.Contains(got, "theirs only") || !strings.Contains(got, "mine only") {
		t.Errorf("page after keep both = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, it.Name)); err == nil {
		t.Error("the conflict copy should have left the folder")
	}
	kept, _ := os.ReadDir(filepath.Join(backups, "conflicts"))
	if len(kept) == 0 {
		t.Error("the conflict copy must be kept under backups/conflicts")
	}
	if len(n.SyncConflicts()) != 0 || n.SyncState().Conflicts != 0 {
		t.Error("no conflicts should be left")
	}
	if err := n.ResolveConflict(it.Name, "keep-both"); err == nil || !strings.Contains(err.Error(), "already settled") {
		t.Errorf("resolving twice = %v", err)
	}
}

func syncRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDownloadDayAndAll(t *testing.T) {
	n, dir, _ := syncNotes(t)
	var mu sync.Mutex
	asked := map[string]int{}
	old := requestDownload
	requestDownload = func(path string, _ bool) { mu.Lock(); asked[filepath.Base(path)]++; mu.Unlock() }
	defer func() { requestDownload = old }()
	put(t, dir, ".2026-10-05.md.icloud", "")

	if err := n.DownloadDay("nope"); err == nil {
		t.Error("a bad day must be refused")
	}
	if err := n.DownloadDay("2026-10-07"); err != nil {
		t.Fatal(err)
	}
	n.DownloadAll()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := asked["2026-10-07.md"] >= 1 && asked["2026-10-05.md"] >= 1
		mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Errorf("download requests = %v", asked)
}

func TestStartSyncNotifiesOnAChangeAndStops(t *testing.T) {
	n, dir, _ := syncNotes(t)
	var mu sync.Mutex
	var events []string
	stop := startSync(n, func(e string) { mu.Lock(); events = append(events, e); mu.Unlock() })
	defer stop()
	mu.Lock()
	if len(events) != 0 {
		t.Errorf("events before any change: %v", events)
	}
	mu.Unlock()
	put(t, dir, "2026-10-08.md", core.SerializeDay("2026-10-08", "written by the provider"))
	deadline := time.Now().Add(8 * time.Second) // the poll runs every 3 s
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(events)
		mu.Unlock()
		if got > 0 {
			mu.Lock()
			defer mu.Unlock()
			if events[0] != "folder-changed" {
				t.Errorf("event = %q", events[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("no folder-changed event after a change")
}
