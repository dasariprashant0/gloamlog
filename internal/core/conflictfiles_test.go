package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeepConflictCopyNeverOverwritesAndReusesIdentical(t *testing.T) {
	dir, backups := t.TempDir(), t.TempDir()
	copyPath := filepath.Join(dir, "2026-10-08 2.md")
	must(t, os.WriteFile(copyPath, []byte("other"), 0o644))
	at := time.Date(2026, 10, 9, 10, 15, 30, 0, time.UTC)

	kept := val(KeepConflictCopy(copyPath, backups, at, ""))
	want := filepath.Join(backups, "conflicts", "20261009-101530-2026-10-08 2.md")
	if kept != want {
		t.Fatalf("kept at %s, want %s", kept, want)
	}
	if b, _ := os.ReadFile(kept); string(b) != "other" {
		t.Errorf("kept text = %q", b)
	}
	if b, _ := os.ReadFile(copyPath); string(b) != "other" {
		t.Error("the original must not be touched by keep")
	}
	// a retry (same bytes) reuses the copy, even a second later
	if again := val(KeepConflictCopy(copyPath, backups, at.Add(time.Second), "")); again != kept {
		t.Errorf("identical retry made %s, want the reused %s", again, kept)
	}
	// the same name with other bytes in the same second gets -2-; the first copy stays
	must(t, os.WriteFile(copyPath, []byte("changed"), 0o644))
	second := val(KeepConflictCopy(copyPath, backups, at, ""))
	if !strings.HasSuffix(second, "20261009-101530-2-2026-10-08 2.md") {
		t.Errorf("second copy = %s", second)
	}
	if b, _ := os.ReadFile(kept); string(b) != "other" {
		t.Error("an earlier safety copy was overwritten")
	}
	// a custom label
	mac := val(KeepConflictCopy(copyPath, backups, at, "2026-10-08 (this Mac).md"))
	if !strings.HasSuffix(mac, "-2026-10-08 (this Mac).md") {
		t.Errorf("labelled copy = %s", mac)
	}
}

func TestKeepConflictCopyFailsWhenTheOriginalCannotBeRead(t *testing.T) {
	if _, err := KeepConflictCopy(filepath.Join(t.TempDir(), "nope.md"), t.TempDir(), time.Now(), ""); err == nil {
		t.Error("keeping a missing file must fail")
	}
}

func TestReleaseConflictCopyOnlyWhenUnchanged(t *testing.T) {
	dir, backups := t.TempDir(), t.TempDir()
	copyPath := filepath.Join(dir, "2026-10-08 2.md")
	must(t, os.WriteFile(copyPath, []byte("v1"), 0o644))
	kept := val(KeepConflictCopy(copyPath, backups, time.Now(), ""))

	must(t, os.WriteFile(copyPath, []byte("v2 written by the provider meanwhile"), 0o644))
	if err := ReleaseConflictCopy(copyPath, kept); err == nil {
		t.Error("a changed copy must not be released")
	}
	if !pathExists(copyPath) {
		t.Error("the changed copy was deleted")
	}
	must(t, os.WriteFile(copyPath, []byte("v1"), 0o644))
	must(t, ReleaseConflictCopy(copyPath, kept))
	if pathExists(copyPath) {
		t.Error("the unchanged copy should have left the folder")
	}
	if !pathExists(kept) {
		t.Error("the safety copy must stay")
	}
}
