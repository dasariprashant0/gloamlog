package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// RetentionDays: Claude Code keeps sessions about this long.
const RetentionDays = 30

// Scanner is what the desktop service keeps: the Claude Code reader (it caches per file), the git settings and the redactor.
// Port of SourcesController.scan/finish (Swift UI/SourcesController.swift), minus the UI state.
type Scanner struct {
	Claude   *ClaudeCodeSource
	GitPath  string    // "" = not installed
	GitRun   GitRunner // nil = DefaultGitRunner(Home)
	Home     string
	Redactor Redactor
}

// NewScanner is the production wiring: roots from $CLAUDE_CONFIG_DIR and <home>/.claude, a located git, the real redactor.
func NewScanner(home string, getenv func(string) string) *Scanner {
	return &Scanner{Claude: NewClaudeCodeSource(DefaultClaudeRoots(getenv, home), home), GitPath: LocateGit(), Home: home, Redactor: NewRedactor(home)}
}

// ScanResult is everything the strip shows for one day.
type ScanResult struct {
	Day          string             `json:"day"`
	Bullets      []SuggestedBullet  `json:"bullets"`
	HiddenLine   string             `json:"hiddenLine"`   // "4 values hidden (2 emails, 2 tokens)", or ""
	Notes        []string           `json:"notes"`        // notices, "N records skipped", format-changed warning, retention note
	LastScanLine string             `json:"lastScanLine"` // "Last look: Thu 8 Oct, 12 files read, 0 records skipped."
	Stats        ScanStats          `json:"stats"`
	Hidden       map[RedactKind]int `json:"hidden"`
}

// Scan looks at day: Git first (when a commit is seen by both sources, the one with the right time and subject wins), then
// Claude Code, only for projects the prefs allow and sources they switch on. today is the current day key (for the retention
// note). A cancelled ctx stops early and returns what was gathered so far; the caller should drop that result.
func (s *Scanner) Scan(ctx context.Context, day, today string, cal core.Calendar, prefs core.SourcesPrefs, progress Progress) ScanResult {
	policy := PolicyFromPrefs(prefs)
	var items []ActivityItem
	var stats ScanStats
	var notices []string
	absorb := func(r SourceResult) {
		items = append(items, r.Items...)
		stats.Add(r.Stats)
		notices = append(notices, r.Notices...)
	}
	if prefs.Git && !cancelled(ctx) {
		g := NewGitSource(s.gitRoots(ctx, prefs), s.GitPath, s.Home, s.GitRun)
		absorb(g.Activity(ctx, day, cal, policy, progress))
	}
	if prefs.Claude && !cancelled(ctx) {
		absorb(s.Claude.Activity(ctx, day, cal, policy, progress))
	}
	digest := Bullets(items, s.Redactor)

	notes := append([]string(nil), notices...)
	if stats.Skipped > 0 {
		notes = append(notes, fmt.Sprintf("%s skipped", pluralWord(stats.Skipped, "record")))
	}
	if stats.FormatLooksChanged() {
		notes = append(notes, "Claude Code's file format may have changed, so this list may be incomplete.")
	}
	if len(digest.Bullets) == 0 && prefs.Claude {
		if ago, ok := daysBetween(day, today, cal); ok && ago > RetentionDays {
			notes = append(notes, "Claude Code no longer keeps sessions this old.")
		}
	}
	loc := cal.Loc
	if loc == nil {
		loc = time.UTC
	}
	return ScanResult{
		Day: day, Bullets: digest.Bullets, HiddenLine: RedactSummary(digest.Hidden), Notes: notes, Stats: stats, Hidden: digest.Hidden,
		LastScanLine: fmt.Sprintf("Last look: %s, %s read, %s skipped.", core.DayKeyFormat(day, "EEE d MMM", loc),
			pluralWord(stats.Files, "file"), pluralWord(stats.Skipped, "record")),
	}
}

// gitRoots are where Git looks for repositories: the added folders, the ticked projects, and (while Claude Code is read) the
// folders Claude Code worked in. A repository Claude worked in has its commits read by Git, with their exact hash and time,
// even when its folder was never added: Claude's own record of a commit often has no hash (`git commit -q` prints nothing).
func (s *Scanner) gitRoots(ctx context.Context, prefs core.SourcesPrefs) []string {
	roots := append(append([]string(nil), prefs.GitRoots...), prefs.Allowed...)
	if prefs.Claude && s.Claude != nil {
		for _, p := range s.Claude.Projects(ctx, nil) {
			roots = append(roots, p.Root)
		}
	}
	return roots
}

// Projects is the project list for Settings > Sources: Claude Code's projects and the repositories under the added folders.
// A project both know shows both sources.
func (s *Scanner) Projects(ctx context.Context, prefs core.SourcesPrefs, progress Progress) []SourceProject {
	byRoot := map[string]*SourceProject{}
	var order []string
	add := func(ps []SourceProject) {
		for _, p := range ps {
			k := strings.ToLower(p.Root)
			if e, ok := byRoot[k]; ok {
				e.Sources = append(e.Sources, p.Sources...)
				if p.LastActive.After(e.LastActive) {
					e.LastActive = p.LastActive
				}
				continue
			}
			c := p
			byRoot[k] = &c
			order = append(order, k)
		}
	}
	add(s.Claude.Projects(ctx, progress))
	add(NewGitSource(prefs.GitRoots, s.GitPath, s.Home, s.GitRun).Projects(ctx))
	out := make([]SourceProject, 0, len(order))
	for _, k := range order {
		out = append(out, *byRoot[k])
	}
	return out
}

func pluralWord(n int, word string) string {
	es := false
	for _, suf := range []string{"ch", "sh", "s", "x", "z"} {
		if len(word) >= len(suf) && word[len(word)-len(suf):] == suf {
			es = true
		}
	}
	switch {
	case n == 1:
		return fmt.Sprintf("%d %s", n, word)
	case es:
		return fmt.Sprintf("%d %ses", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func daysBetween(from, to string, cal core.Calendar) (int, bool) {
	loc := cal.Loc
	if loc == nil {
		loc = time.UTC
	}
	a, ok1 := core.DayKeyDate(from, loc)
	b, ok2 := core.DayKeyDate(to, loc)
	if !ok1 || !ok2 {
		return 0, false
	}
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return int(time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC).Sub(time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)).Hours() / 24), true
}
