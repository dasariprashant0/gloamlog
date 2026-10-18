// syncscan.go - one look at the log folder for sync: which files are still in the cloud and which conflict copies are here.
// Port of app/Core/Sync/SyncScan.swift. Names and lstat only: no file is opened or read. Run it off the UI thread.
//
// Not ported: inside iCloud Drive the Swift scan also asked iCloud for each day file's download status (needs Objective-C), which
// found the "stale local copy, a newer one is coming" case (RemoteFile.HasLocalText there). Here a file is "not here" when it is
// dataless or a placeholder; a stale but readable copy counts as here until the new bytes replace it (the folder watcher then
// reloads the page).
package core

import (
	"os"
	"path/filepath"
	"sort"
)

// ConflictCopy is a second version of a day that a sync service saved next to the real file ("2026-10-08 2.md").
type ConflictCopy struct {
	Day    string         `json:"day"`
	Name   string         `json:"name"` // the file's name in the folder
	Family ConflictFamily `json:"family"`
}

// RemoteFile is a file whose bytes are not on this Mac yet.
type RemoteFile struct {
	Name           string `json:"name"` // the real file's name, never the placeholder's
	Day            string `json:"day"`
	IsConflictCopy bool   `json:"isConflictCopy"`
}

// SyncScan is what one look at the folder found.
type SyncScan struct {
	Remote []RemoteFile `json:"remote"`
	// Conflicts are the conflict copies that are fully on this Mac, by day then name.
	Conflicts []ConflictCopy `json:"conflicts"`
}

// PendingDays are the days with something still on its way, oldest first ("N days still downloading").
func (s SyncScan) PendingDays() []string {
	seen := map[string]bool{}
	days := []string{}
	for _, r := range s.Remote {
		if !seen[r.Day] {
			seen[r.Day] = true
			days = append(days, r.Day)
		}
	}
	sort.Strings(days)
	return days
}

// BlockedDays are the days whose page has no local text at all: they cannot be opened for writing and are not "unlogged".
func (s SyncScan) BlockedDays() map[string]bool {
	out := map[string]bool{}
	for _, r := range s.Remote {
		if !r.IsConflictCopy {
			out[r.Day] = true
		}
	}
	return out
}

// ScanSyncFolder looks at dir once. A folder that cannot be listed gives an empty scan.
func ScanSyncFolder(dir string) SyncScan {
	var out SyncScan
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	remote := map[string]RemoteFile{} // by name: a file with both a placeholder and a real entry is listed once
	for _, e := range entries {
		name := e.Name()
		switch f := ClassifyFolderEntry(name); f.Kind {
		case EntryPlaceholder:
			remote[f.Name] = RemoteFile{Name: f.Name, Day: f.Day, IsConflictCopy: f.IsConflictCopy}
		case EntryDay:
			if pathIsDataless(filepath.Join(dir, name)) {
				remote[name] = RemoteFile{Name: name, Day: f.Day}
			}
		case EntryConflict:
			if pathIsDataless(filepath.Join(dir, name)) {
				remote[name] = RemoteFile{Name: name, Day: f.Day, IsConflictCopy: true}
			} else {
				out.Conflicts = append(out.Conflicts, ConflictCopy{Day: f.Day, Name: name, Family: f.Family})
			}
		}
	}
	// a conflict copy that is also still a placeholder is not a conflict to review yet
	kept := out.Conflicts[:0]
	for _, c := range out.Conflicts {
		if _, still := remote[c.Name]; still {
			continue
		}
		kept = append(kept, c)
	}
	out.Conflicts = kept
	out.Remote = make([]RemoteFile, 0, len(remote))
	for _, r := range remote {
		out.Remote = append(out.Remote, r)
	}
	sort.Slice(out.Remote, func(i, j int) bool {
		a, b := out.Remote[i], out.Remote[j]
		return a.Day < b.Day || (a.Day == b.Day && a.Name < b.Name)
	})
	sort.Slice(out.Conflicts, func(i, j int) bool {
		a, b := out.Conflicts[i], out.Conflicts[j]
		return a.Day < b.Day || (a.Day == b.Day && a.Name < b.Name)
	})
	return out
}
