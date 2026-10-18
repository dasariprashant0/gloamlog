// folderentry.go - what a name in the log folder is, as far as sync is concerned: a day file, an iCloud placeholder, a
// conflict copy, or nothing of ours. Pure (names in, a verdict out, no disk). Port of app/Core/Sync/FolderEntry.swift.
// Precision over recall: only the known providers' conflict names count; a name that merely starts like a day
// ("2026-10-08 standup.md") is left alone.
//
//	2026-10-08.md                                                day
//	.2026-10-08.md.icloud   .2026-10-08 2.md.icloud              placeholder (iCloud has the file, not its bytes yet; Name is the real file's)
//	2026-10-08 2.md   2026-10-08 3.md                            conflict, iCloud / Finder numbering (2 or more)
//	2026-10-08 (conflicted copy).md                              conflict, Dropbox
//	2026-10-08 (Alex's MacBook's conflicted copy 2026-10-09).md  conflict, Dropbox
//	2026-10-08.sync-conflict-20261009-101530-ABCDEFG.md          conflict, Syncthing
//	notes.md, 2026-13-40.md, .DS_Store, .gloamlog-tmp-*, assets  ignored
package core

import (
	"strconv"
	"strings"
)

// ConflictFamily is which sync service names a conflict copy this way.
type ConflictFamily string

const (
	FamilyICloudNumbered ConflictFamily = "iCloudNumbered"
	FamilyDropbox        ConflictFamily = "dropbox"
	FamilySyncthing      ConflictFamily = "syncthing"
)

// EntryKind is what a name in the folder is.
type EntryKind int

const (
	EntryIgnored     EntryKind = iota // nothing of ours
	EntryDay                          // 2026-10-08.md
	EntryConflict                     // a second version of a day that a sync service saved
	EntryPlaceholder                  // an iCloud placeholder for a day file or a conflict copy
)

// FolderEntry is the verdict of ClassifyFolderEntry. Day is set for everything but EntryIgnored; Family for EntryConflict, and for an
// EntryPlaceholder of a conflict copy; Name (the real file's name, never the placeholder's) for EntryPlaceholder.
type FolderEntry struct {
	Kind           EntryKind
	Day            string
	Family         ConflictFamily
	Name           string
	IsConflictCopy bool // EntryPlaceholder: the placeholder stands for a conflict copy
}

// ClassifyFolderEntry sorts one file name of the log folder.
func ClassifyFolderEntry(name string) FolderEntry {
	if strings.HasPrefix(name, ".") {
		const ext = ".icloud"
		if !strings.HasSuffix(name, ext) || len(name) <= 8 {
			return FolderEntry{}
		}
		real := name[1 : len(name)-len(ext)]
		switch v := classifyVisible(real); v.Kind {
		case EntryDay:
			return FolderEntry{Kind: EntryPlaceholder, Name: real, Day: v.Day}
		case EntryConflict:
			return FolderEntry{Kind: EntryPlaceholder, Name: real, Day: v.Day, Family: v.Family, IsConflictCopy: true}
		}
		return FolderEntry{}
	}
	return classifyVisible(name)
}

func classifyVisible(name string) FolderEntry {
	if !strings.HasSuffix(name, ".md") || len(name) < 13 {
		return FolderEntry{}
	}
	stem := name[:len(name)-3]
	day, rest := stem[:10], stem[10:]
	if !isPlausibleDay(day) {
		return FolderEntry{}
	}
	if rest == "" {
		return FolderEntry{Kind: EntryDay, Day: day}
	}
	if family, ok := conflictFamily(rest); ok {
		return FolderEntry{Kind: EntryConflict, Day: day, Family: family}
	}
	return FolderEntry{}
}

// isPlausibleDay is a well-formed key with a month 01...12 and a day 01...31 (the calendar itself is not consulted).
func isPlausibleDay(day string) bool {
	if !DayKeyIsWellFormed(day) {
		return false
	}
	m, _ := strconv.Atoi(day[5:7])
	d, _ := strconv.Atoi(day[8:10])
	return m >= 1 && m <= 12 && d >= 1 && d <= 31
}

// conflictFamily reads what follows the 10-character day in the name (without ".md").
func conflictFamily(rest string) (ConflictFamily, bool) {
	const syncthing = ".sync-conflict-"
	if strings.HasPrefix(rest, syncthing) {
		return FamilySyncthing, len(rest) > len(syncthing)
	}
	if !strings.HasPrefix(rest, " ") {
		return "", false
	}
	t := rest[1:]
	if t != "" && len(t) <= 3 && strings.Trim(t, "0123456789") == "" {
		if n, err := strconv.Atoi(t); err == nil && n >= 2 {
			return FamilyICloudNumbered, true
		}
	}
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") && strings.Contains(strings.ToLower(t), "conflicted copy") {
		return FamilyDropbox, true
	}
	return "", false
}
