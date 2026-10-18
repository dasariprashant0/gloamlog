// inbox.go - entries other programs leave for Gloamlog (today: the `log_work` tool of the Claude Code helper in
// Resources/claude/gloamlog-mcp.py). Port of app/Core/Capture/Inbox.swift.
//
// One small JSON file per entry in <home>/Library/Application Support/Gloamlog/inbox:
//
//	{"text": "Fixed the card-order bug by sorting before the merge.", "ts": "2026-10-14T14:35:00Z"}
//
// Writers create the file under a hidden name and rename it, so a half-written file is never seen. The app turns each entry into a jot
// (DayWriter: durable journal first, then the page) and deletes the file only after that (DayWriter.DrainInbox).
//
// Reader rules: "*.json" (not hidden), oldest modification time first (name breaks ties), at most 50 files per look (junk counts), at
// most 64 KiB each, a JSON object with a non-empty trimmed string "text". A file that is not valid is skipped and never deleted here.
//
// The inbox "ts" (UTC, written when the helper ran): the Swift app IGNORED it and stamped every entry with the moment it drained the
// inbox, so an entry written near midnight, or while the app was closed, could land on the wrong day. This port reads it and offers a
// choice (InboxTimeMode): the default uses ts when it parses, else the drain time; InboxUseDrainTime is Swift's behaviour.
package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	InboxMaxFileBytes      = 64 * 1024
	InboxMaxEntriesPerLook = 50
)

// InboxTimeMode says which moment an inbox entry is dated with (its day and its HH:mm stamp).
type InboxTimeMode int

const (
	// InboxUseFileTime (the default) dates the entry with the file's "ts" when it parses and is not in the future, else with the
	// drain time.
	InboxUseFileTime InboxTimeMode = iota
	// InboxUseDrainTime is what the Swift app did: always the moment of draining.
	InboxUseDrainTime
)

// InboxEntry is one entry waiting in the inbox.
type InboxEntry struct {
	Path string    // the file; DayWriter.DrainInbox deletes it after the note is journaled
	Text string    // trimmed
	Ts   time.Time // the file's "ts"; zero when missing or unparseable
}

// DefaultInboxDir is <home>/Library/Application Support/Gloamlog/inbox.
func DefaultInboxDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Gloamlog", "inbox")
}

// CaptureTime is the moment the entry is dated with. drain is the time of draining.
func (e InboxEntry) CaptureTime(drain time.Time, mode InboxTimeMode) time.Time {
	if mode == InboxUseFileTime && !e.Ts.IsZero() && !e.Ts.After(drain) { // a ts from the future (clock skew) is not believed
		return e.Ts
	}
	return drain
}

// InboxPending lists the entries waiting in dir, oldest first. A missing or unreadable dir has none.
func InboxPending(dir string) []InboxEntry {
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type file struct {
		name string
		mod  time.Time // zero (long ago) when unknown
	}
	var files []file
	for _, e := range names {
		n := e.Name()
		if strings.HasPrefix(n, ".") || filepath.Ext(n) != ".json" {
			continue
		}
		var mod time.Time
		if fi, err := e.Info(); err == nil {
			mod = fi.ModTime()
		}
		files = append(files, file{n, mod})
	}
	sort.Slice(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.mod.Equal(b.mod) {
			return a.name < b.name
		}
		return a.mod.Before(b.mod)
	})
	if len(files) > InboxMaxEntriesPerLook {
		files = files[:InboxMaxEntriesPerLook]
	}
	var out []InboxEntry
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		fi, err := os.Stat(path)
		if err != nil || fi.Size() > InboxMaxFileBytes {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var o map[string]any
		if json.Unmarshal(data, &o) != nil {
			continue
		}
		text, _ := o["text"].(string)
		if text = trimmed(text); text == "" {
			continue
		}
		var ts time.Time
		if s, ok := o["ts"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				ts = t
			}
		}
		out = append(out, InboxEntry{Path: path, Text: text, Ts: ts})
	}
	return out
}
