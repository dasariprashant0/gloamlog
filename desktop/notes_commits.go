package main

// Commits on a list page (writing redesign spec, section 4, revised 2026-10-19): a day's commits in one project are one line,
// "- 21:30 48 commits in daily-log · latest: Add CSV export `1a2b3c4`", timed at the latest commit, through the capture writer
// as a timed note (journal first; an open page takes it in its editor) in the list the project's folder belongs to (Work or
// Personal, Settings > Page). A newer count replaces the project's line (core.WithoutCommitsLines). A commit whose hash is on
// the page outside a commits line (the suggestions strip put it there) is not counted; a line the person deleted is not put
// back this session until the count changes.
//
// Today's commits arrive by themselves while "Add commits for me" (sources.autoAdd) is on: when today's page opens, every
// commitTick, and on gloamlog://refresh. A past day has a button (AddCommits); history is never rewritten by itself. Classic
// pages get nothing here: they keep the suggestions strip.

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
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

// AddCommits puts day's commits on day's list page, one line per project at its latest commit, each in its folder's list, and
// returns how many lines it wrote. A classic page gets none. The past-day button calls it; today's commits come by themselves (autoAddCommits).
func (n *Notes) AddCommits(day string) (int, error) {
	n.mu.Lock()
	s, cal, home, now := n.settings, n.cal(), n.home, n.now()
	n.mu.Unlock()
	today := core.DayKeyString(now, appZone())
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

	// The page and the notes waiting for it: all, to see a commits line already up to date; and without the commits lines,
	// to see commits the strip wrote one by one (their hashes).
	all, others := "", ""
	doc, err := c.store.Load(day)
	if err != nil {
		return 0, err // an unreadable page: its hashes can't be checked, so nothing is added (the next look tries again)
	}
	texts := []string{}
	if doc != nil {
		texts = strings.Split(doc.Body, "\n")
	}
	for _, it := range c.journal.Pending() {
		if it.Day == day {
			texts = append(texts, it.Text)
		}
	}
	for _, t := range texts {
		all += t + "\n"
		if !isCommitsText(t) {
			others += t + "\n"
		}
	}
	type project struct {
		root, name   string
		count        int
		latest       time.Time
		latestLine   string // the newest commit as it reads: its subject and hash
		latestCommit string
	}
	byRoot := map[string]*project{}
	var order []string
	for _, b := range result.Bullets {
		line, ok := sources.CommitLine(b)
		if !ok || b.Time.IsZero() || strings.Contains(others, b.Sha) {
			continue
		}
		k := strings.ToLower(b.Root)
		p := byRoot[k]
		if p == nil {
			name := b.Project
			if name == "" {
				name = filepath.Base(b.Root)
			}
			p = &project{root: b.Root, name: name}
			byRoot[k] = p
			order = append(order, k)
		}
		p.count++
		if b.Time.After(p.latest) {
			p.latest, p.latestLine, p.latestCommit = b.Time, line, b.Sha
		}
	}
	sort.Strings(order)
	added := 0
	for _, k := range order {
		p := byRoot[k]
		line := core.CommitsLine(p.count, p.name, p.latestLine)
		if strings.Contains(all, line) {
			continue // this count is on the page (or on its way) already
		}
		key := day + "|" + k + "|" + strconv.Itoa(p.count) + "|" + p.latestCommit
		c.mu.Lock()
		seen := c.commits[key]
		c.commits[key] = true
		c.mu.Unlock()
		if seen {
			continue // written this session: a line the person deleted stays deleted until the count changes
		}
		if _, ok := c.writer.CaptureTimed(line, "git", s.Lists.ListFor(p.root), p.latest); ok {
			added++
		}
	}
	if added > 0 {
		c.notify()
	}
	return added, nil
}

// isCommitsText is true for a commits line as a page holds it ("- 21:30 48 commits in …") or as a waiting note ("48 commits in …").
func isCommitsText(t string) bool {
	t = strings.TrimPrefix(strings.TrimSpace(t), "- ")
	if len(t) > 6 && t[2] == ':' && t[5] == ' ' {
		t = t[6:]
	}
	return core.CommitsLineProject(t) != ""
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
	added, _ := n.AddCommits(core.DayKeyString(n.now(), appZone()))
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
