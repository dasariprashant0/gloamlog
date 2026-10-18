package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFolderKindForPath(t *testing.T) {
	cases := []struct {
		path  string
		local bool
		want  FolderKind
		label string
	}{
		{"/Users/a/Library/Mobile Documents/com~apple~CloudDocs/Gloamlog", true, FolderKind{Kind: "icloud"}, "iCloud Drive"},
		{"/Users/a/Library/CloudStorage/OneDrive-Personal/Log", true, FolderKind{Kind: "cloud", Provider: "OneDrive"}, "OneDrive"},
		{"/Users/a/Library/CloudStorage/GoogleDrive-me@x.com/My Drive/Log", true, FolderKind{Kind: "cloud", Provider: "GoogleDrive"}, "GoogleDrive"},
		{"/Users/a/Dropbox/Log", true, FolderKind{Kind: "cloud", Provider: "Dropbox"}, "Dropbox"},
		{"/Users/a/Dropbox (Personal)/Log", true, FolderKind{Kind: "cloud", Provider: "Dropbox"}, "Dropbox"},
		{"/Volumes/NAS/Log", false, FolderKind{Kind: "network"}, "Network or external drive"},
		{"/Users/a/Documents/Log", true, FolderKind{Kind: "local"}, "Local"},
		{"", true, FolderKind{Kind: "local"}, "Local"},
	}
	for _, c := range cases {
		got := folderKindForPath(c.path, c.local)
		if got != c.want || got.Label() != c.label {
			t.Errorf("folderKindForPath(%q) = %+v (%s), want %+v (%s)", c.path, got, got.Label(), c.want, c.label)
		}
	}
	if got := DetectFolderKind(filepath.Join(t.TempDir(), "not yet created")); got.Kind != "local" {
		t.Errorf("a folder that does not exist yet is local, got %+v", got)
	}
}

func TestFolderKindSourceName(t *testing.T) {
	for kind, want := range map[FolderKind]string{
		{Kind: "icloud"}:                     "iCloud",
		{Kind: "cloud", Provider: "Dropbox"}: "Dropbox",
		{Kind: "local"}:                      "your cloud folder",
		{Kind: "network"}:                    "your cloud folder",
	} {
		if got := kind.SourceName(); got != want {
			t.Errorf("%+v SourceName = %q, want %q", kind, got, want)
		}
	}
}

func TestOwnWritesKnowTheirFileUntilSomebodyTouchesIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-10-08.md")
	must(t, AtomicWrite([]byte("ours"), path, ""))
	real := CanonicalPath(path)
	if !IsOwnWrite(real) {
		t.Error("AtomicWrite must note the file through OwnWriteHook")
	}
	must(t, os.WriteFile(path, []byte("somebody else, other size"), 0o644))
	if IsOwnWrite(real) {
		t.Error("a touched file is no longer our own echo")
	}
	if IsOwnWrite(filepath.Join(dir, "never written.md")) {
		t.Error("an unknown file is not ours")
	}
}

func TestRequestDownloadAsksICloudFirstThenReads(t *testing.T) {
	old := ubiquityDownload
	defer func() { ubiquityDownload = old }()
	path := filepath.Join(t.TempDir(), "2026-10-08.md")
	must(t, os.WriteFile(path, []byte("x"), 0o644))

	var asked []string
	ubiquityDownload = func(p string) error { asked = append(asked, p); return nil }
	RequestDownload(path, true)
	if len(asked) != 1 || asked[0] != path {
		t.Errorf("iCloud was asked for %v", asked)
	}
	RequestDownload(path, false)
	if len(asked) != 1 {
		t.Error("other providers are not asked through iCloud")
	}
	ubiquityDownload = func(string) error { return errors.New("no brctl") }
	RequestDownload(path, true) // falls back to the 1-byte read
	// best effort: a missing file or an empty path is simply asked for again on the next scan, and nothing is created
	RequestDownload(path+".gone", true)
	RequestDownload("", true)
	if pathExists(path + ".gone") {
		t.Error("a download request must never create the file")
	}
	if IsOwnWrite("") {
		t.Error("the empty path is nobody's file")
	}
}
