// searchindex.go - the in-memory cache behind LogStore.Search. Port of app/Core/SearchIndex.swift.
//
// Every page as search sees it (SearchSection: the plain text of each heading-delimited section), parsed once per run and re-read
// only when the file's "mtime size" signature changed: the app writes files itself, and folder sync and outside editors change them
// underneath it. Which days exist is still ListDays()'s call, so a deleted file drops out of the results (and of the cache). A day
// that is not on this Mac yet (LogStore.loadIfLocal returns nil: an iCloud placeholder or a dataless file) is left out and never
// cached as empty, exactly as the uncached search skipped it. Memory: the plain text plus the titled haystack of every section and
// the folded copy of each, about 4x the text.
// Thread-safe: one lock around each call (the app searches and warms from different goroutines).
// ponytail: one global lock, so a search typed while the launch warm-up is still reading waits for it (once per run, the cost of one
// cold search); every search stats every file (~30 ms per 3,650 on APFS) and then runs a substring search over all cached text.
// Ceiling: a cold start over tens of thousands of files, or a folder whose stat sweep is slow (network volume). Upgrade path then,
// in order: one opendir/fstatat sweep instead of per-file stats (as FolderStamp does); a persistent SQLite FTS5 index.
package core

import (
	"path/filepath"
	"sync"
)

type searchPage struct {
	day      string
	sections []SearchSection
}

type searchEntry struct {
	signature string
	sections  []SearchSection
}

// SearchIndex caches the searchable form of every page of one store.
type SearchIndex struct {
	mu        sync.Mutex
	entries   map[string]searchEntry
	fileReads int // files read and parsed so far (tests: a file is parsed once, and again only after it changed)
}

func newSearchIndex() *SearchIndex { return &SearchIndex{entries: map[string]searchEntry{}} }

// FileReads is how many files have been read and parsed so far.
func (x *SearchIndex) FileReads() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.fileReads
}

// pages returns the pages of the store's days, newest first, from the cache wherever the file is unchanged. load is
// LogStore.loadIfLocal: nil = not here yet, left out and not cached; an error fails the whole call, as it did for the uncached search.
func (x *SearchIndex) pages(store *LogStore, load func(day string) (*DayDocument, error)) ([]searchPage, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	days, err := store.ListDays()
	if err != nil {
		return nil, err
	}
	out := make([]searchPage, 0, len(days))
	seen := map[string]bool{}
	for _, day := range days {
		// Signature before read, so a cached text is never older than its signature.
		path := filepath.Join(store.Dir, day+".md")
		sig, ok := fileSignature(path)
		if !ok { // gone since the listing: Load says nil too
			continue
		}
		// A hit is served only while the page is here: the same two checks as LogStore.IsNotLocal (a miss goes through load).
		if e, hit := x.entries[day]; hit && e.signature == sig &&
			!pathExists(filepath.Join(store.Dir, placeholderName(day))) && !pathIsDataless(path) {
			out = append(out, searchPage{day, e.sections})
			seen[day] = true
			continue
		}
		doc, err := load(day)
		if err != nil {
			return nil, err
		}
		if doc == nil { // not on this Mac yet: skipped, never cached
			delete(x.entries, day)
			continue
		}
		x.fileReads++
		sections := searchSections(*doc)
		x.entries[day] = searchEntry{sig, sections}
		out = append(out, searchPage{day, sections})
		seen[day] = true
	}
	if len(x.entries) > len(seen) { // deleted days leave the cache
		for d := range x.entries {
			if !seen[d] {
				delete(x.entries, d)
			}
		}
	}
	return out, nil
}
