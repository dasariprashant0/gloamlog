package core

import "testing"

func TestClassifyFolderEntry(t *testing.T) {
	day := func(d string) FolderEntry { return FolderEntry{Kind: EntryDay, Day: d} }
	conflict := func(d string, f ConflictFamily) FolderEntry {
		return FolderEntry{Kind: EntryConflict, Day: d, Family: f}
	}
	cases := []struct {
		name string
		want FolderEntry
	}{
		{"2026-10-08.md", day("2026-10-08")},
		{"2026-10-08 2.md", conflict("2026-10-08", FamilyICloudNumbered)},
		{"2026-10-08 12.md", conflict("2026-10-08", FamilyICloudNumbered)},
		{"2026-10-08 999.md", conflict("2026-10-08", FamilyICloudNumbered)},
		{"2026-10-08 1.md", FolderEntry{}},    // numbering starts at 2
		{"2026-10-08 1000.md", FolderEntry{}}, // at most 3 digits
		{"2026-10-08 (conflicted copy).md", conflict("2026-10-08", FamilyDropbox)},
		{"2026-10-08 (Alex's MacBook's conflicted copy 2026-10-09).md", conflict("2026-10-08", FamilyDropbox)},
		{"2026-10-08 (Conflicted Copy).md", conflict("2026-10-08", FamilyDropbox)},
		{"2026-10-08.sync-conflict-20261009-101530-ABCDEFG.md", conflict("2026-10-08", FamilySyncthing)},
		{"2026-10-08.sync-conflict-.md", FolderEntry{}},
		{"2026-10-08 standup.md", FolderEntry{}},
		{"2026-10-08 (draft).md", FolderEntry{}},
		{"", FolderEntry{}},
		{"notes.md", FolderEntry{}},
		{"2026-13-40.md", FolderEntry{}},
		{"2026-00-10.md", FolderEntry{}},
		{"2026-10-08.txt", FolderEntry{}},
		{".DS_Store", FolderEntry{}},
		{".gloamlog-tmp-123-ABC", FolderEntry{}},
		{"assets", FolderEntry{}},
		{".2026-10-08.md.icloud", FolderEntry{Kind: EntryPlaceholder, Day: "2026-10-08", Name: "2026-10-08.md"}},
		{".2026-10-08 2.md.icloud", FolderEntry{Kind: EntryPlaceholder, Day: "2026-10-08", Name: "2026-10-08 2.md", Family: FamilyICloudNumbered, IsConflictCopy: true}},
		{".notes.md.icloud", FolderEntry{}},
		{".icloud", FolderEntry{}},
	}
	for _, c := range cases {
		if got := ClassifyFolderEntry(c.name); got != c.want {
			t.Errorf("ClassifyFolderEntry(%q) = %+v, want %+v", c.name, got, c.want)
		}
	}
}
