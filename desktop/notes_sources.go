package main

// "Suggested from your work": the day page's strip and Settings > Sources call these. The rules live in internal/sources;
// this file adds the Swift SourcesController's service behaviour: a newer scan cancels the one before it, and a result is
// reused for 300 s while the settings that shaped it are unchanged.

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/dasariprashant0/gloamlog/internal/sources"
)

// Suggestions is the strip's state for one day. Off means the strip is not shown at all (no source on, or no consent yet).
type Suggestions struct {
	Off     bool               `json:"off"`
	NoneOn  bool               `json:"noneOn"` // sources are on but no project is ticked, so nothing was read
	Heading string             `json:"heading"`
	Result  sources.ScanResult `json:"result"`
}

type scanCache struct {
	mu      sync.Mutex
	scanner *sources.Scanner
	cancel  context.CancelFunc
	key     string
	at      time.Time
	result  sources.ScanResult
}

var scans scanCache

func (c *scanCache) get(home string) *sources.Scanner {
	if c.scanner == nil {
		c.scanner = sources.NewScanner(home, os.Getenv)
	}
	return c.scanner
}

// Suggestions scans day for work to suggest. again forces a fresh look ("Look again").
func (n *Notes) Suggestions(day string, again bool) (Suggestions, error) {
	n.mu.Lock()
	s, cal, home, now, store := n.settings, n.cal(), n.home, n.now(), n.store
	n.mu.Unlock()
	prefs := s.Sources
	out := Suggestions{Heading: pageHeading(store, day, prefs.HeadingText())} // a list page takes them under Log
	if !prefs.AnyOn() || !prefs.Consented {
		out.Off = true
		return out, nil
	}
	if !prefs.AllProjects && len(prefs.Allowed) == 0 {
		out.NoneOn = true
		return out, nil
	}
	settingsKey, _ := json.Marshal(prefs) // a plain struct of bools and strings: cannot fail
	key := day + "\x00" + string(settingsKey)

	scans.mu.Lock()
	if !again && scans.key == key && now.Sub(scans.at) < 300*time.Second {
		out.Result = scans.result
		scans.mu.Unlock()
		return out, nil
	}
	if scans.cancel != nil {
		scans.cancel() // the newer scan wins
	}
	ctx, cancel := context.WithCancel(context.Background())
	scans.cancel = cancel
	scanner := scans.get(home)
	scans.mu.Unlock()

	result := scanner.Scan(ctx, day, core.DayKeyString(now, time.Local), cal, prefs, nil)
	if ctx.Err() != nil {
		return out, context.Canceled // a newer scan replaced this one: the caller drops it
	}
	scans.mu.Lock()
	scans.key, scans.at, scans.result = key, now, result
	scans.mu.Unlock()
	out.Result = result
	return out, nil
}

// SourceProjects lists Claude Code's projects and the repositories under the added Git folders, for Settings > Sources.
func (n *Notes) SourceProjects() []sources.SourceProject {
	n.mu.Lock()
	prefs, home := n.settings.Sources, n.home
	n.mu.Unlock()
	scans.mu.Lock()
	scanner := scans.get(home)
	scans.mu.Unlock()
	return scanner.Projects(context.Background(), prefs, nil)
}

// SuggestionLines is the markdown the strip inserts for the ticked bullets: one "- " line each, escaped.
func (n *Notes) SuggestionLines(bullets []sources.SuggestedBullet) string {
	return sources.BulletsMarkdown(bullets)
}

// AlreadyAdded lists the bullets whose evidence is already in the page, so the strip never offers them twice.
func (n *Notes) AlreadyAdded(bullets []sources.SuggestedBullet, pageMarkdown string) []string {
	plain := sources.PlainText(pageMarkdown)
	ids := []string{}
	for _, b := range bullets {
		if sources.IsAdded(b, plain) {
			ids = append(ids, b.ID)
		}
	}
	return ids
}
