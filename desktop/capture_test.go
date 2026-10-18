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

// captureNotes is a Notes over a made-up home in a temp folder, with a fixed clock (14 Oct 2026, 10:32) and a recording emit.
func captureNotes(t *testing.T, folderExists bool) (n *Notes, c *captureState, folder string, events *recorder) {
	t.Helper()
	home := t.TempDir()
	folder = filepath.Join(home, "Gloamlog")
	if folderExists {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	n = &Notes{home: home, settingsPath: filepath.Join(home, "settings.json"), backupDir: filepath.Join(home, "backups"),
		now: func() time.Time { return time.Date(2026, 10, 14, 10, 32, 0, 0, time.Local) }}
	s := core.DefaultSettings(home)
	s.StorageFolder = folder
	n.apply(s)
	c = captureFor(n)
	events = &recorder{}
	c.emit = events.add
	return n, c, folder, events
}

type recorder struct {
	mu     sync.Mutex
	events []recorded
}
type recorded struct {
	name string
	data any
}

func (r *recorder) add(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, recorded{name, data})
	r.mu.Unlock()
}
func (r *recorder) named(name string) []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []any
	for _, e := range r.events {
		if e.name == name {
			out = append(out, e.data)
		}
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

const today = "2026-10-14"

func TestJotWithNoOpenPageWritesTheFile(t *testing.T) {
	n, c, folder, _ := captureNotes(t, true)
	res, err := n.Jot("call Sam")
	if err != nil || res.Outcome != "added" || res.Message != "Added to today's page under Jots." || res.Day != today {
		t.Fatalf("Jot = %+v, %v", res, err)
	}
	file := readFile(t, filepath.Join(folder, today+".md"))
	if !strings.Contains(file, "## Jots") || !strings.Contains(file, "- 10:32 call Sam") {
		t.Errorf("file = %q", file)
	}
	if n.JotsWaiting() != 0 || len(c.journal.Pending()) != 0 {
		t.Errorf("waiting %d, pending %d: the note should be acknowledged", n.JotsWaiting(), len(c.journal.Pending()))
	}
	if _, err := os.Stat(c.journal.Path()); err != nil {
		t.Errorf("journal missing: %v", err)
	}
}

func TestEmptyJotDoesNothing(t *testing.T) {
	n, _, folder, _ := captureNotes(t, true)
	for _, text := range []string{"", "   \n", "[]", "[ ]  "} {
		if res, _ := n.Jot(text); res.Outcome != "empty" {
			t.Errorf("Jot(%q) = %+v", text, res)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, today+".md")); err == nil {
		t.Error("an empty jot wrote a page")
	}
}

func TestJotIntoAnOpenPageGoesThroughTheEditor(t *testing.T) {
	n, _, folder, events := captureNotes(t, true)
	n.EditorOpened(today)
	res, _ := n.Jot("ship it")
	if res.Outcome != "added" {
		t.Fatalf("Jot = %+v", res)
	}
	got := events.named("jot-into-page")
	if len(got) != 1 {
		t.Fatalf("jot-into-page events = %v", got)
	}
	ev := got[0].(map[string]any)
	if ev["day"] != today || ev["heading"] != "Jots" || ev["markdown"] != "- 10:32 ship it" || ev["id"] == "" {
		t.Errorf("event = %v", ev)
	}
	if _, err := os.Stat(filepath.Join(folder, today+".md")); err == nil {
		t.Fatal("the file was written under an open page")
	}
	if n.JotsWaiting() != 1 {
		t.Errorf("waiting = %d, want 1 until the page saves", n.JotsWaiting())
	}
	n.JotPlaced(ev["id"].(string), "appended", "## Jots\n\n- 10:32 ship it\n")
	n.EditorSaved(today, "## Jots\n\n- 10:32 ship it\n")
	if n.JotsWaiting() != 0 {
		t.Errorf("waiting = %d after the page saved", n.JotsWaiting())
	}
}

func TestAPageThatNeverAnswersFallsBackToTheFile(t *testing.T) {
	n, c, folder, _ := captureNotes(t, true)
	c.answerTimeout = 30 * time.Millisecond
	n.EditorOpened(today)
	if res, _ := n.Jot("late note"); res.Outcome != "added" {
		t.Fatalf("Jot = %+v", res)
	}
	waitFor(t, "the file fallback", func() bool { return n.JotsWaiting() == 0 })
	if file := readFile(t, filepath.Join(folder, today+".md")); !strings.Contains(file, "late note") {
		t.Errorf("file = %q", file)
	}
	if c.pageOpen(today) {
		t.Error("the silent page is still registered")
	}
}

func TestAPageThatIsNotLoadedKeepsTheNoteForARetry(t *testing.T) {
	n, c, folder, events := captureNotes(t, true)
	c.writer.ScheduleRetry = func(time.Duration, func()) {} // no timer: the test drives the retry
	n.EditorOpened(today)
	n.Jot("early")
	first := events.named("jot-into-page")[0].(map[string]any)["id"].(string)
	n.JotPlaced(first, "notLoaded", "")
	if n.JotsWaiting() != 1 {
		t.Fatalf("waiting = %d", n.JotsWaiting())
	}
	c.replay()
	if got := len(events.named("jot-into-page")); got != 2 {
		t.Fatalf("the note was not offered to the page again (%d events)", got)
	}
	if _, err := os.Stat(filepath.Join(folder, today+".md")); err == nil {
		t.Error("the file was written while the page was open")
	}
}

func TestJotWhileTheFolderIsMissingIsKeptThenDelivered(t *testing.T) {
	n, c, folder, _ := captureNotes(t, false)
	res, _ := n.Jot("remember the milk")
	if res.Outcome != "kept" || res.Message != keptMessage || res.Waiting != 1 {
		t.Fatalf("Jot = %+v", res)
	}
	if got := c.journal.Pending(); len(got) != 1 || got[0].Text != "remember the milk" {
		t.Fatalf("journal pending = %+v", got)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if left := n.RetryJots(); left != 0 {
		t.Fatalf("still %d waiting", left)
	}
	if file := readFile(t, filepath.Join(folder, today+".md")); !strings.Contains(file, "remember the milk") {
		t.Errorf("file = %q", file)
	}
}

func TestACapturedNoteKeepsItsDayAcrossAReplay(t *testing.T) {
	n, c, folder, _ := captureNotes(t, false)
	n.Jot("written on the 14th")
	n.now = func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local) } // the folder comes back the next day
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	c.replay()
	if _, err := os.Stat(filepath.Join(folder, "2026-10-15.md")); err == nil {
		t.Error("the note moved to the day it was delivered")
	}
	if file := readFile(t, filepath.Join(folder, today+".md")); !strings.Contains(file, "written on the 14th") {
		t.Errorf("file = %q", file)
	}
}

func TestFolderChangeIsFollowed(t *testing.T) {
	n, c, _, _ := captureNotes(t, true)
	other := filepath.Join(t.TempDir(), "Elsewhere")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	s := n.Settings()
	s.StorageFolder = other
	n.apply(s)
	if !c.syncFolder() || c.syncFolder() {
		t.Fatal("syncFolder should report the change once")
	}
	n.Jot("new place")
	if file := readFile(t, filepath.Join(other, today+".md")); !strings.Contains(file, "new place") {
		t.Errorf("file = %q", file)
	}
}

func TestDrainInboxDatesEntriesByTheirOwnTimestamp(t *testing.T) {
	n, c, folder, events := captureNotes(t, true)
	inbox := core.DefaultInboxDir(n.home)
	if err := os.MkdirAll(inbox, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(inbox, "1760000000000-ab12cd34.json")
	if err := os.WriteFile(entry, []byte(`{"text":"Fixed the card-order bug.","ts":"2026-10-12T12:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c.drain()
	if file := readFile(t, filepath.Join(folder, "2026-10-12.md")); !strings.Contains(file, "Fixed the card-order bug.") {
		t.Errorf("file = %q", file)
	}
	if _, err := os.Stat(entry); err == nil {
		t.Error("the inbox file was not removed")
	}
	if len(events.named("capture-changed")) == 0 {
		t.Error("no capture-changed event")
	}
}

func TestHandleURLRefreshDrainsAndAsksSourcesToLookAgain(t *testing.T) {
	n, c, folder, events := captureNotes(t, true)
	inbox := core.DefaultInboxDir(n.home)
	os.MkdirAll(inbox, 0o700)
	os.WriteFile(filepath.Join(inbox, "1-aaaaaaaa.json"), []byte(`{"text":"from the hook"}`), 0o600)
	scans.mu.Lock()
	scans.key = "cached"
	scans.mu.Unlock()

	handleURL(n, "gloamlog://elsewhere")
	if len(events.named("refresh-sources")) != 0 {
		t.Fatal("an unknown URL was handled")
	}
	handleURL(n, "gloamlog://refresh")
	if file := readFile(t, filepath.Join(folder, today+".md")); !strings.Contains(file, "from the hook") {
		t.Errorf("file = %q", file)
	}
	scans.mu.Lock()
	key := scans.key
	scans.mu.Unlock()
	if key != "" || len(events.named("refresh-sources")) != 1 {
		t.Errorf("sources not refreshed: key %q, events %d", key, len(events.named("refresh-sources")))
	}
	_ = c
}

func TestOpenPagesAreCounted(t *testing.T) {
	n, c, _, _ := captureNotes(t, true)
	n.EditorOpened(today)
	n.EditorOpened(today)
	n.EditorClosed(today)
	if !c.pageOpen(today) {
		t.Fatal("one editor is still open")
	}
	n.EditorClosed(today)
	if c.pageOpen(today) || c.Page(today) != nil {
		t.Fatal("the page should be gone")
	}
}
