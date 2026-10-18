// syncresolve.go - settling a conflict copy without ever losing a line. Port of the resolving half of app/UI/SyncController.swift
// (readSides, perform, settleIdentical); the UI half (sheet, banner, announcements) lives in the desktop app.
//
// The other copy is first written into <backups>/conflicts (checked byte for byte), then the chosen text goes through the normal
// save path (which backs the old page up, forced, not throttled), and only then does the other copy leave the folder. A copy that
// holds exactly the same page as the real file is a plain duplicate: it moves to the safety copies and nobody is asked.
//
// What stays with the caller (it needs the open editor): saving the editor's page before the call, and refusing while the page has
// unsaved edits. The caller must also serialise saves of this store: ResolveConflict sets MinBackupInterval to 0 for the one save.
package core

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ConflictChoice is what the user picked for a conflict.
type ConflictChoice string

const (
	KeepMine   ConflictChoice = "keep-mine"   // this Mac's page stays as it is
	KeepTheirs ConflictChoice = "keep-theirs" // the other copy replaces this Mac's page
	KeepBoth   ConflictChoice = "keep-both"   // this Mac's page plus the other copy's extra lines (MergeBoth)
)

// SyncProblem is a sentence for the user. Nothing was lost when one is returned: the folder is as it was, or has an extra safety copy.
type SyncProblem struct{ Message string }

func (p *SyncProblem) Error() string { return p.Message }

func problem(msg string) error { return &SyncProblem{msg} }

// ConflictSides is both versions of a conflicted day, read from disk. Mine is nil when this Mac has no page for the day; the dates are
// zero when unknown.
type ConflictSides struct {
	Mine       *DayDocument
	MineDate   time.Time
	Theirs     DayDocument
	TheirsDate time.Time
}

// MineBody is this Mac's text ("" when there is no page or the day is skipped).
func (s ConflictSides) MineBody() string {
	if s.Mine == nil || s.Mine.IsSkipped {
		return ""
	}
	return s.Mine.Body
}

func modTime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

// ReadConflictSides reads both versions as they are on disk now. The error is a *SyncProblem when either cannot be read yet.
func ReadConflictSides(store *LogStore, c ConflictCopy) (ConflictSides, error) {
	copyPath := filepath.Join(store.Dir, c.Name)
	mine, err := store.Load(c.Day)
	if errors.Is(err, ErrNotLocal) {
		return ConflictSides{}, problem("This day is still downloading. Try again in a moment.")
	}
	if err != nil {
		return ConflictSides{}, problem("Couldn't read this day. " + err.Error())
	}
	if pathIsDataless(copyPath) {
		return ConflictSides{}, problem("The other copy can't be read yet. Try again in a moment.")
	}
	raw, err := readText(copyPath)
	if err != nil {
		return ConflictSides{}, problem("The other copy can't be read yet. Try again in a moment.")
	}
	return ConflictSides{Mine: mine, MineDate: modTime(store.Path(c.Day)), Theirs: ParseDay(c.Day, raw), TheirsDate: modTime(copyPath)}, nil
}

// ResolveConflict applies the choice. A failure is a *SyncProblem whose message is the sentence to show. now stamps the safety copies.
func ResolveConflict(store *LogStore, c ConflictCopy, choice ConflictChoice, now time.Time) error {
	sides, err := ReadConflictSides(store, c)
	if err != nil {
		return err
	}
	// the text the page will have; nil leaves the page file alone
	var newBody *string
	switch choice {
	case KeepMine:
	case KeepTheirs:
		if !HasContent(sides.Theirs.Body) {
			return problem("The other copy has no text to take.")
		}
		newBody = &sides.Theirs.Body
	case KeepBoth:
		merged := MergeBoth(sides.MineBody(), sides.Theirs.Body)
		if merged != sides.MineBody() {
			newBody = &merged
		}
	default:
		return problem("That isn't a choice Gloamlog knows.")
	}
	if choice == KeepMine && sides.Mine == nil && HasContent(sides.Theirs.Body) {
		return problem("This Mac has no page for this day yet. Choose Take theirs or Keep both.")
	}

	copyPath := filepath.Join(store.Dir, c.Name)
	// 1. both versions go into the safety copies first (copied and checked; nothing is touched yet). The page's own backup before a
	//    save is "best effort" in the store, so when the page is about to change this makes sure this Mac's version is kept too.
	if store.BackupDir == "" {
		return problem("Couldn't keep a safety copy of the versions, so nothing was changed.")
	}
	kept, err := KeepConflictCopy(copyPath, store.BackupDir, now, "")
	if err == nil && newBody != nil && sides.Mine != nil {
		_, err = KeepConflictCopy(store.Path(c.Day), store.BackupDir, now, c.Day+" (this Mac).md")
	}
	if err != nil {
		return problem("Couldn't keep a safety copy of the versions, so nothing was changed.")
	}
	// 2. the chosen text goes through the normal save path, which backs the page up first (forced, not throttled)
	if newBody != nil {
		interval := store.MinBackupInterval
		store.MinBackupInterval = 0
		_, err = store.Save(c.Day, *newBody)
		store.MinBackupInterval = interval
		if err != nil {
			return problem("Couldn't save the page. Nothing was removed. " + err.Error())
		}
	}
	// 3. only now does the other copy leave the shared folder; if that fails it stays and is offered again, harmlessly
	_ = ReleaseConflictCopy(copyPath, kept)
	return nil
}

// SettleIdenticalConflicts moves every conflict copy that holds exactly the same page as the real file into the safety copies, and
// returns the scan without them. A copy that cannot be settled (the day is still downloading, the pages differ, the safety copy fails)
// stays in the list.
func SettleIdenticalConflicts(store *LogStore, scan SyncScan, now time.Time) SyncScan {
	if len(scan.Conflicts) == 0 || store.BackupDir == "" {
		return scan
	}
	blocked := scan.BlockedDays()
	var left []ConflictCopy
	for _, c := range scan.Conflicts {
		copyPath, mainPath := filepath.Join(store.Dir, c.Name), store.Path(c.Day)
		if !settleIfIdentical(c, copyPath, mainPath, store.BackupDir, blocked, now) {
			left = append(left, c)
		}
	}
	scan.Conflicts = left
	return scan
}

func settleIfIdentical(c ConflictCopy, copyPath, mainPath, backupDir string, blocked map[string]bool, now time.Time) bool {
	if blocked[c.Day] || pathIsDataless(mainPath) {
		return false
	}
	a, errA := readText(mainPath)
	b, errB := readText(copyPath)
	if errA != nil || errB != nil || ParseDay(c.Day, a) != ParseDay(c.Day, b) {
		return false
	}
	kept, err := KeepConflictCopy(copyPath, backupDir, now, "")
	return err == nil && ReleaseConflictCopy(copyPath, kept) == nil
}
