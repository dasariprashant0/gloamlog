package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func touch(t *testing.T, dir, name, content string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func TestScanSyncFolderFindsPlaceholdersAndConflicts(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "2026-10-08.md", "# 2026-10-08\n\ntext")
	touch(t, dir, "2026-10-08 2.md", "other")
	touch(t, dir, "2026-10-07 (conflicted copy).md", "other")
	touch(t, dir, ".2026-10-05.md.icloud", "")             // a day still in the cloud
	touch(t, dir, ".2026-10-04 2.md.icloud", "")           // a conflict copy still in the cloud
	touch(t, dir, ".2026-10-08 2.md.icloud", "")           // the same conflict copy is also a real file: not here yet
	touch(t, dir, "notes.md", "not ours")                  // ignored
	touch(t, dir, ".DS_Store", "")                         // ignored
	touch(t, dir, ".gloamlog-tmp-1-X", "")                 // ignored
	must(t, os.Mkdir(filepath.Join(dir, "assets"), 0o755)) // ignored

	scan := ScanSyncFolder(dir)

	wantRemote := []RemoteFile{
		{Name: "2026-10-04 2.md", Day: "2026-10-04", IsConflictCopy: true},
		{Name: "2026-10-05.md", Day: "2026-10-05"},
		{Name: "2026-10-08 2.md", Day: "2026-10-08", IsConflictCopy: true},
	}
	if !reflect.DeepEqual(scan.Remote, wantRemote) {
		t.Errorf("Remote = %+v\nwant %+v", scan.Remote, wantRemote)
	}
	wantConflicts := []ConflictCopy{{Day: "2026-10-07", Name: "2026-10-07 (conflicted copy).md", Family: FamilyDropbox}}
	if !reflect.DeepEqual(scan.Conflicts, wantConflicts) {
		t.Errorf("Conflicts = %+v\nwant %+v", scan.Conflicts, wantConflicts)
	}
	if got := scan.PendingDays(); !reflect.DeepEqual(got, []string{"2026-10-04", "2026-10-05", "2026-10-08"}) {
		t.Errorf("PendingDays = %v", got)
	}
	// only a day's own page blocks it; a waiting conflict copy does not
	if got := scan.BlockedDays(); !reflect.DeepEqual(got, map[string]bool{"2026-10-05": true}) {
		t.Errorf("BlockedDays = %v", got)
	}
}

func TestScanSyncFolderCleanAndMissing(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "2026-10-08.md", "text")
	scan := ScanSyncFolder(dir)
	if len(scan.Remote) != 0 || len(scan.Conflicts) != 0 || len(scan.PendingDays()) != 0 {
		t.Errorf("a clean folder found %+v", scan)
	}
	if scan := ScanSyncFolder(filepath.Join(dir, "gone")); len(scan.Remote)+len(scan.Conflicts) != 0 {
		t.Errorf("a missing folder found %+v", scan)
	}
}
