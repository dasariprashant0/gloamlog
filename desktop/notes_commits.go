package main

// Commits on a list page (writing redesign spec, section 4): the day's commits from the ticked projects become lines at their
// commit time, "- 13:02 Add CSV export `1a2b3c4`", through the capture writer as timed notes (journal first; an open page takes
// them in its editor; core.InsertTimed puts them in time order) in the list their repository's folder belongs to (Work or
// Personal, Settings > Page). The short hash is the de-duplication key: a commit whose hash is in the page, waits in the journal,
// or was added this session is not added again.
//
// Today's commits arrive by themselves while "Add commits for me" (sources.autoAdd) is on: when today's page opens, every
// commitTick, and on gloamlog://refresh. A past day has a button (AddCommits); history is never rewritten by itself. Classic
// pages get nothing here: they keep the suggestions strip.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/dasariprashant0/gloamlog/internal/sources"
)

const commitTick = 5 * time.Minute

var errSourcesOff = errors.New("Turn on Git or Claude Code and tick a project in Settings > Sources first.")

// scanCommits looks at day's work with the shared scanner (its per-file cache keeps a repeat cheap). Its own context, so it never
// cancels the strip's look on a classic page, nor the strip this one. Tests replace it.
var scanCommits = func(home, day, today string, cal core.Calendar, prefs core.SourcesPrefs) sources.ScanResult {
	scans.mu.Lock()
	scanner := scans.get(home)
	scans.mu.Unlock()
	return scanner.Scan(context.Background(), day, today, cal, prefs, nil)
}

// isListHeading is true for the heading a list page takes notes under (Log, Work, Personal).
func isListHeading(h string) bool {
	return h == core.LogHeading || h == core.ListWork || h == core.ListPersonal
}

// AddCommits puts day's commits on day's list page at their commit times, each in its folder's list, and returns how many it
// added. A classic page gets none. The past-day button calls it; today's commits come by themselves (autoAddCommits).
func (n *Notes) AddCommits(day string) (int, error) {
	n.mu.Lock()
	s, cal, home, now := n.settings, n.cal(), n.home, n.now()
	n.mu.Unlock()
	today := core.DayKeyString(now, time.Local)
	if !core.DayKeyIsWellFormed(day) || day > today {
		return 0, errors.New("not a day with commits: " + day)
	}
	prefs := s.Sources
	if !prefs.AnyOn() || !prefs.Consented || (!prefs.AllProjects && len(prefs.Allowed) == 0) {
		return 0, errSourcesOff
	}
	c := captureFor(n)
	c.syncFolder()
	if !isListHeading(c.headingFor(day, s.Capture.JotsHeading, core.ListWork)) {
		return 0, nil // a classic page (or an unreadable one): the suggestions strip covers it
	}
	result := scanCommits(home, day, today, cal, prefs)

	have := "" // the page and the notes waiting for it: where a commit already written shows its hash
	doc, err := c.store.Load(day)
	if err != nil {
		return 0, err // an unreadable page: its hashes can't be checked, so nothing is added (the next look tries again)
	}
	if doc != nil {
		have = doc.Body
	}
	for _, it := range c.journal.Pending() {
		if it.Day == day {
			have += "\n" + it.Text
		}
	}
	added := 0
	for _, b := range result.Bullets {
		line, ok := sources.CommitLine(b)
		if !ok || b.Time.IsZero() || strings.Contains(have, b.Sha) {
			continue
		}
		key := day + "|" + b.Sha
		c.mu.Lock()
		seen := c.commits[key]
		c.commits[key] = true
		c.mu.Unlock()
		if seen {
			continue // added this session: a line the person deleted stays deleted
		}
		if _, ok := c.writer.CaptureTimed(line, "git", s.Lists.ListFor(b.Root), b.Time); ok {
			added++
		}
	}
	if added > 0 {
		c.notify()
	}
	return added, nil
}

// autoAddCommits adds today's commits while "Add commits for me" is on. Off, or nothing new: it tries again next time.
func (n *Notes) autoAddCommits() int {
	n.mu.Lock()
	on := n.settings.Sources.AutoAdd
	n.mu.Unlock()
	if !on {
		return 0
	}
	// No one to tell in the background: sources off, a classic page or an unreadable one all mean "nothing this time", and the
	// next look (page open, the timer, gloamlog://refresh) tries again. The past-day button shows AddCommits' errors.
	added, _ := n.AddCommits(core.DayKeyString(n.now(), time.Local))
	return added
}

// startCommits looks at today's commits every commitTick while the app runs. main.go calls it once; today's page opening and
// gloamlog://refresh look too (capture.go).
func startCommits(n *Notes) {
	go func() {
		for range time.Tick(commitTick) {
			n.autoAddCommits()
		}
	}()
}
