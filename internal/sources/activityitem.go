// Package sources is "Suggested from your work": what Claude Code and Git saw for one day, turned into short, redacted bullets.
//
// Port of app/Core/Sources/*.swift. Sources are READ-ONLY: they never write, never touch the network, and look only at projects
// the owner ticked (ProjectPolicy). Items may hold raw text and are NEVER shown as they are: Bullets runs the Redactor over
// everything that reaches the screen or a page.
//
// Cancellation is a context.Context; progress is an optional func(done, total int). Nothing reads the clock or the real
// home folder on its own: "now", locations, roots and the git runner are all parameters.
package sources

import (
	"context"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// ActivityKind is what an ActivityItem describes.
type ActivityKind string

const (
	KindSession     ActivityKind = "session"
	KindPrompt      ActivityKind = "prompt"
	KindFile        ActivityKind = "file"
	KindCommit      ActivityKind = "commit"
	KindPullRequest ActivityKind = "pullRequest"
)

// ActivityItem is what a source found for one day.
type ActivityItem struct {
	Kind     ActivityKind
	Root     string    // the project's root folder: the policy key, never shown
	Project  string    // what the project is called (normally the folder name)
	Title    string    // prompt first line / commit subject / file path (project-relative) / pull-request link
	Detail   string    // secondary text, e.g. a short hash
	Time     time.Time // when it happened (a session: its first record that day); zero = unknown
	Evidence string    // the short reference written with the bullet (a short hash, a link): "is it already on the page?"
	Count    int       // a file: how often it was edited; a session item: how many sessions that day
	Seconds  int       // a session item: active time
}

// ID is the identity used to dedupe items: kind, project root and the evidence (or the title when there is none).
func (a ActivityItem) ID() string {
	e := a.Evidence
	if e == "" {
		e = a.Title
	}
	return string(a.Kind) + "|" + a.Root + "|" + e
}

// ScanStats is what a scan saw, for the "3 records skipped" line and the format check.
type ScanStats struct {
	Files   int `json:"files"`   // transcript files read, or repositories asked
	Lines   int `json:"lines"`   //
	Skipped int `json:"skipped"` // lines that were not valid JSON or too long to read
	Records int `json:"records"` // user / assistant records seen ...
	Usable  int `json:"usable"`  // ... that had a time and a message: if far fewer than all, the format has probably changed
}

// Add folds o into s.
func (s *ScanStats) Add(o ScanStats) {
	s.Files += o.Files
	s.Lines += o.Lines
	s.Skipped += o.Skipped
	s.Records += o.Records
	s.Usable += o.Usable
}

// FormatLooksChanged is true when far fewer records than expected were usable.
func (s ScanStats) FormatLooksChanged() bool { return s.Records >= 30 && s.Usable*5 < s.Records*4 }

// SourceResult is what one source found for one day.
type SourceResult struct {
	Items   []ActivityItem
	Stats   ScanStats
	Notices []string
}

// SourceProject is one row of the project list in Settings > Sources.
type SourceProject struct {
	Root       string    `json:"root"`
	Name       string    `json:"name"`
	Sources    []string  `json:"sources"`    // "claude", "git"
	LastActive time.Time `json:"lastActive"` // zero = unknown
}

// Progress is called with (done, total) while a scan runs. A nil Progress is fine.
type Progress func(done, total int)

func (p Progress) report(done, total int) {
	if p != nil {
		p(done, total)
	}
}

// dayBounds is the start of the day key and the start of the next one, in the calendar's zone.
func dayBounds(day string, cal core.Calendar) (start, end time.Time, ok bool) {
	loc := cal.Loc
	if loc == nil {
		loc = time.UTC
	}
	start, ok = core.DayKeyDate(day, loc)
	if !ok {
		return start, end, false
	}
	return start, start.In(loc).AddDate(0, 0, 1), true
}

func cancelled(ctx context.Context) bool { return ctx.Err() != nil }
