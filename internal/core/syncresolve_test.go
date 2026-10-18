package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// conflictEnv is a store with backups on and one conflict copy for 2026-10-08.
type conflictEnv struct {
	store *LogStore
	c     ConflictCopy
	now   time.Time
}

func newConflictEnv(t *testing.T, mine, theirs string) conflictEnv {
	t.Helper()
	now := time.Date(2026, 10, 9, 10, 15, 30, 0, time.UTC)
	store := NewLogStore(t.TempDir(), t.TempDir(), time.Hour, func() time.Time { return now })
	if mine != "" {
		writeRaw(t, store.Dir, "2026-10-08", SerializeDay("2026-10-08", mine))
	}
	touch(t, store.Dir, "2026-10-08 2.md", SerializeDay("2026-10-08", theirs))
	return conflictEnv{store, ConflictCopy{Day: "2026-10-08", Name: "2026-10-08 2.md", Family: FamilyICloudNumbered}, now}
}

func (e conflictEnv) copyExists() bool { return pathExists(filepath.Join(e.store.Dir, e.c.Name)) }

// keptTexts is the text of every file under <backups>/conflicts.
func (e conflictEnv) keptTexts(t *testing.T) []string {
	t.Helper()
	var out []string
	entries, _ := os.ReadDir(ConflictsFolder(e.store.BackupDir))
	for _, f := range entries {
		b, err := os.ReadFile(filepath.Join(ConflictsFolder(e.store.BackupDir), f.Name()))
		must(t, err)
		out = append(out, string(b))
	}
	return out
}

func hasKept(texts []string, sub string) bool {
	for _, s := range texts {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestResolveKeepMine(t *testing.T) {
	e := newConflictEnv(t, "mine line", "their line")
	must(t, ResolveConflict(e.store, e.c, KeepMine, e.now))
	if got := load(t, e.store, "2026-10-08").Body; got != "mine line" {
		t.Errorf("page = %q, want mine untouched", got)
	}
	if e.copyExists() {
		t.Error("the conflict copy should have left the shared folder")
	}
	if !hasKept(e.keptTexts(t), "their line") {
		t.Error("their version must be kept under backups/conflicts")
	}
}

func TestResolveKeepTheirs(t *testing.T) {
	e := newConflictEnv(t, "mine line", "their line")
	must(t, ResolveConflict(e.store, e.c, KeepTheirs, e.now))
	if got := load(t, e.store, "2026-10-08").Body; got != "their line" {
		t.Errorf("page = %q, want theirs", got)
	}
	if e.copyExists() {
		t.Error("the conflict copy should have left the shared folder")
	}
	kept := e.keptTexts(t)
	if !hasKept(kept, "their line") || !hasKept(kept, "mine line") {
		t.Errorf("both versions must be kept, got %q", kept)
	}
	if len(e.store.ListBackups("2026-10-08")) == 0 {
		t.Error("the replaced page must be in the day's version history (forced backup)")
	}
}

func TestResolveKeepBoth(t *testing.T) {
	e := newConflictEnv(t, "shared\nmine only", "shared\ntheirs only")
	must(t, ResolveConflict(e.store, e.c, KeepBoth, e.now))
	want := "shared\nmine only\n\n## From another copy\n\ntheirs only"
	if got := load(t, e.store, "2026-10-08").Body; got != want {
		t.Errorf("page = %q, want %q", got, want)
	}
	if e.copyExists() {
		t.Error("the conflict copy should have left the shared folder")
	}
	kept := e.keptTexts(t)
	if !hasKept(kept, "theirs only") || !hasKept(kept, "mine only") {
		t.Errorf("both versions must be kept, got %q", kept)
	}
}

func TestResolveKeepBothWithNothingNewLeavesThePageAlone(t *testing.T) {
	e := newConflictEnv(t, "a\nb", "b")
	before := fileText(t, e.store.Dir, "2026-10-08")
	must(t, ResolveConflict(e.store, e.c, KeepBoth, e.now))
	if fileText(t, e.store.Dir, "2026-10-08") != before {
		t.Error("the page file changed although the other copy had nothing new")
	}
	if e.copyExists() || !hasKept(e.keptTexts(t), "b") {
		t.Error("the copy should be kept and released")
	}
}

func TestResolveWhenThisMacHasNoPage(t *testing.T) {
	e := newConflictEnv(t, "", "their line")
	err := ResolveConflict(e.store, e.c, KeepMine, e.now)
	var p *SyncProblem
	if !errors.As(err, &p) || !strings.Contains(p.Message, "no page for this day yet") {
		t.Fatalf("keep mine without a page = %v", err)
	}
	if !e.copyExists() {
		t.Error("a refused choice must leave the copy where it is")
	}
	must(t, ResolveConflict(e.store, e.c, KeepBoth, e.now))
	if got := load(t, e.store, "2026-10-08").Body; got != "their line" {
		t.Errorf("page = %q", got)
	}
}

func TestResolveRefusesAnEmptyOtherCopy(t *testing.T) {
	e := newConflictEnv(t, "mine", "")
	if err := ResolveConflict(e.store, e.c, KeepTheirs, e.now); err == nil || !strings.Contains(err.Error(), "no text to take") {
		t.Errorf("take theirs of nothing = %v", err)
	}
	if !e.copyExists() {
		t.Error("the copy must stay")
	}
}

func TestResolveWithoutBackupsChangesNothing(t *testing.T) {
	e := newConflictEnv(t, "mine", "theirs")
	e.store.BackupDir = ""
	if err := ResolveConflict(e.store, e.c, KeepTheirs, e.now); err == nil {
		t.Fatal("without a safety-copy folder nothing may change")
	}
	if got := load(t, e.store, "2026-10-08").Body; got != "mine" || !e.copyExists() {
		t.Errorf("page %q, copy exists %v", got, e.copyExists())
	}
}

func TestResolveRestoresTheBackupInterval(t *testing.T) {
	e := newConflictEnv(t, "mine", "theirs")
	must(t, ResolveConflict(e.store, e.c, KeepTheirs, e.now))
	if e.store.MinBackupInterval != time.Hour {
		t.Errorf("MinBackupInterval = %v, want it restored", e.store.MinBackupInterval)
	}
}

func TestSettleIdenticalConflicts(t *testing.T) {
	e := newConflictEnv(t, "same text", "same text")
	touch(t, e.store.Dir, "2026-10-08 3.md", SerializeDay("2026-10-08", "different text"))
	scan := ScanSyncFolder(e.store.Dir)
	if len(scan.Conflicts) != 2 {
		t.Fatalf("scan found %d conflicts", len(scan.Conflicts))
	}
	left := SettleIdenticalConflicts(e.store, scan, e.now)
	if len(left.Conflicts) != 1 || left.Conflicts[0].Name != "2026-10-08 3.md" {
		t.Errorf("left = %+v, want only the differing copy", left.Conflicts)
	}
	if e.copyExists() {
		t.Error("the identical copy should have moved to the safety copies")
	}
	if !hasKept(e.keptTexts(t), "same text") {
		t.Error("the identical copy must be kept, not deleted")
	}
	if !pathExists(filepath.Join(e.store.Dir, "2026-10-08 3.md")) {
		t.Error("a differing copy must stay in the folder")
	}
}

func TestReadConflictSides(t *testing.T) {
	e := newConflictEnv(t, "mine", "theirs")
	sides := val(ReadConflictSides(e.store, e.c))
	if sides.MineBody() != "mine" || sides.Theirs.Body != "theirs" || sides.Mine == nil {
		t.Errorf("sides = %+v", sides)
	}
	if sides.MineDate.IsZero() || sides.TheirsDate.IsZero() {
		t.Error("modification dates should be known")
	}
	must(t, os.Remove(filepath.Join(e.store.Dir, "2026-10-08 2.md")))
	var p *SyncProblem
	if _, err := ReadConflictSides(e.store, e.c); !errors.As(err, &p) {
		t.Errorf("a missing copy should give a SyncProblem, got %v", err)
	}
}
