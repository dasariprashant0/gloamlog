package main

// The Catch up screen and the Restore dialog need a little more than Overview and Backups give: the unlogged days with their
// word counts and week, a skip over a date range, and a short preview of each safety copy. The rules stay in internal/core.

import (
	"os"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// CatchUpDay is one unlogged working day for the Catch up list.
type CatchUpDay struct {
	Day       string `json:"day"`
	WeekStart string `json:"weekStart"` // the first day of its week, for the "Week of 5 Oct" groups
	Started   bool   `json:"started"`   // partly written: shown as "N of 20 words" with Continue
	Words     int    `json:"words"`     // words so far when started, else 0
}

type CatchUpData struct {
	Days  []CatchUpDay `json:"days"`
	Since string       `json:"since"` // the day the log starts ("" = no pages yet)
}

// CatchUp lists the unlogged working days in the last windowDays days (0 = the Settings value), oldest first.
func (n *Notes) CatchUp(windowDays int) (CatchUpData, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if windowDays <= 0 {
		windowDays = n.settings.CatchUpWindowDays
	}
	states, since, err := n.states()
	if err != nil {
		return CatchUpData{}, err
	}
	out := CatchUpData{Days: []CatchUpDay{}, Since: since}
	cal := n.cal()
	for _, d := range core.CatchUpMissing(states, n.now(), cal, n.settings.Weekdays, since, windowDays) {
		item := CatchUpDay{Day: d, WeekStart: d, Started: states[d] == core.StatusPartial}
		if t, ok := core.DayKeyDate(d, time.Local); ok {
			item.WeekStart = core.DayKeyString(cal.StartOfWeek(t), time.Local)
		}
		if item.Started {
			if doc, _ := n.store.Load(d); doc != nil { // a word count only: an unreadable day shows none
				item.Words = doc.Words()
			}
		}
		out.Days = append(out.Days, item)
	}
	return out, nil
}

// SkipRange writes one skip marker for every scheduled day from..to (both included) and returns the days it skipped.
// Days with writing are left alone.
func (n *Notes) SkipRange(from, to, reason string) ([]string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.SkipRange(from, to, reason, n.settings.Weekdays, n.cal())
}

// BackupVersion is a safety copy with the first words of what it holds. Pass Date, Path and Words back to Restore.
type BackupVersion struct {
	Date    time.Time `json:"date"`
	Path    string    `json:"path"`
	Words   int       `json:"words"`
	Preview string    `json:"preview"` // up to 120 characters of plain text
}

// BackupVersions lists the safety copies of a day, newest first, each with a short preview.
func (n *Notes) BackupVersions(day string) []BackupVersion {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := []BackupVersion{}
	for _, b := range n.store.ListBackups(day) {
		v := BackupVersion{Date: b.Date, Path: b.Path, Words: b.Words}
		if raw, err := os.ReadFile(b.Path); err == nil {
			if page := core.ParseDay(day, string(raw)); !page.IsSkipped {
				if r := []rune(core.PlainText(page.Body)); len(r) > 120 {
					v.Preview = string(r[:120])
				} else {
					v.Preview = string(r)
				}
			}
		}
		out = append(out, v)
	}
	return out
}
