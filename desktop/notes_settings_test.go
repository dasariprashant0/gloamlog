package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// Moving the log folder never loses or overwrites a day: a day already in the new folder stays as it is and its original
// stays behind; everything else arrives byte for byte and only then leaves the old folder.
func TestMoveLogFolderKeepsEveryDay(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(from, "2026-10-12.md"), "# 2026-10-12\n\nold folder\n")
	writeFile(t, filepath.Join(from, "2026-10-13.md"), "# 2026-10-13\n\nonly in the old folder\n")
	writeFile(t, filepath.Join(from, "notes.txt"), "not a day")
	writeFile(t, filepath.Join(from, "assets", "2026-10-13-1a2b3c4d.png"), "png bytes")
	writeFile(t, filepath.Join(to, "2026-10-12.md"), "# 2026-10-12\n\nnew folder wins\n")

	res := transfer(from, to, true)

	if got := readFile(t, filepath.Join(to, "2026-10-12.md")); !strings.Contains(got, "new folder wins") {
		t.Errorf("existing day in the new folder was overwritten: %q", got)
	}
	if got := readFile(t, filepath.Join(from, "2026-10-12.md")); !strings.Contains(got, "old folder") {
		t.Errorf("the conflicting original must stay behind, got %q", got)
	}
	if got := readFile(t, filepath.Join(to, "2026-10-13.md")); !strings.Contains(got, "only in the old folder") {
		t.Errorf("moved day: %q", got)
	}
	if _, err := os.Stat(filepath.Join(from, "2026-10-13.md")); !os.IsNotExist(err) {
		t.Errorf("a moved day should leave the old folder, stat err = %v", err)
	}
	if got := readFile(t, filepath.Join(to, "assets", "2026-10-13-1a2b3c4d.png")); got != "png bytes" {
		t.Errorf("image: %q", got)
	}
	if _, err := os.Stat(filepath.Join(from, "notes.txt")); err != nil {
		t.Errorf("non-day files are never touched: %v", err)
	}
	if len(res.Conflicts) != 1 || !strings.Contains(res.Text, "Moved 1 log and 1 image.") || !strings.Contains(res.Text, "1 day already existed") {
		t.Errorf("result: %+v", res)
	}
}

// Copying leaves the old folder exactly as it was.
func TestCopyLogFolderLeavesTheOriginal(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(from, "2026-10-14.md"), "# 2026-10-14\n\ncopied\n")
	transfer(from, to, false)
	if readFile(t, filepath.Join(from, "2026-10-14.md")) != readFile(t, filepath.Join(to, "2026-10-14.md")) {
		t.Error("copy differs from the original")
	}
}

// Edge cases: an empty old folder copies nothing; a zero-byte day still arrives (it is the user's file); copy mode never
// overwrites a day that already exists in the new folder.
func TestTransferEdgeCases(t *testing.T) {
	empty, to := t.TempDir(), t.TempDir()
	if res := transfer(empty, to, true); len(res.Conflicts) != 0 || !strings.HasPrefix(res.Text, "Moved 0 logs") {
		t.Errorf("empty folder: %+v", res)
	}

	from, to := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(from, "2026-10-15.md"), "")
	writeFile(t, filepath.Join(from, "2026-10-16.md"), "# 2026-10-16\n\nold\n")
	writeFile(t, filepath.Join(to, "2026-10-16.md"), "# 2026-10-16\n\nkeep me\n")
	res := transfer(from, to, false)
	if got := readFile(t, filepath.Join(to, "2026-10-15.md")); got != "" {
		t.Errorf("zero-byte day: %q", got)
	}
	if got := readFile(t, filepath.Join(to, "2026-10-16.md")); !strings.Contains(got, "keep me") {
		t.Errorf("copy overwrote an existing day: %q", got)
	}
	if got := readFile(t, filepath.Join(from, "2026-10-16.md")); !strings.Contains(got, "old") {
		t.Errorf("copy touched the original: %q", got)
	}
	if len(res.Conflicts) != 1 {
		t.Errorf("conflicts: %+v", res.Conflicts)
	}
}
