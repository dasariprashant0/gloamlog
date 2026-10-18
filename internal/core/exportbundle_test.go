package core

import (
	"os"
	"path/filepath"
	"testing"
)

// Review fixes: an export never deletes or replaces a file the person already had.

func TestExportNeverReplacesADifferentImageOfTheSameName(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	pic := filepath.Join(src, "a.png")
	os.WriteFile(pic, []byte("ours"), 0o644)
	os.MkdirAll(filepath.Join(dst, AssetFolderName), 0o755)
	theirs := filepath.Join(dst, AssetFolderName, "a.png")
	os.WriteFile(theirs, []byte("theirs"), 0o644)

	plan := ExportPlan{Name: "R", Markdown: "x", Assets: []ExportAsset{{Path: AssetFolderName + "/a.png", Source: pic}}}
	if _, err := WriteExport(plan, ExportLayout{Kind: ExportFile, Path: filepath.Join(dst, "R.md")}); err == nil {
		t.Fatal("want an error when a different picture has the same name")
	}
	if b, _ := os.ReadFile(theirs); string(b) != "theirs" {
		t.Fatalf("their image was changed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dst, "R.md")); err == nil {
		t.Fatal("the markdown was written although the export stopped")
	}
}

func TestZipExportReplacesAnOldFileAndKeepsADirectory(t *testing.T) {
	if _, err := os.Stat("/usr/bin/ditto"); err != nil {
		t.Skip("needs /usr/bin/ditto (macOS)")
	}
	dst := t.TempDir()
	plan := ExportPlan{Name: "R", Markdown: "x"}

	old := filepath.Join(dst, "R.zip")
	os.WriteFile(old, []byte("old"), 0o644)
	if _, err := WriteExport(plan, ExportLayout{Kind: ExportZip, Path: old}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(old); string(b) == "old" {
		t.Fatal("the old zip was not replaced")
	}

	// a folder at the chosen path is never wiped (Swift removed it recursively)
	dir := filepath.Join(dst, "D.zip")
	os.MkdirAll(dir, 0o755)
	keep := filepath.Join(dir, "keep.txt")
	os.WriteFile(keep, []byte("k"), 0o644)
	WriteExport(plan, ExportLayout{Kind: ExportZip, Path: dir})
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("a folder's contents were deleted by the export")
	}
}
