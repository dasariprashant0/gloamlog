// folderwatcher.go - "something in the log folder changed", delivered once per look instead of on a 30 s tick.
// Port of app/Core/Sync/FolderWatcher.swift, polling half only (PollingSource): the Swift app listened to FSEvents for local and cloud
// folders and polled network volumes every 5 s. Go's standard library has no FSEvents and the repo takes no new dependency (fsnotify
// would be one; FSEvents through cgo would break the cgo-free build), so every folder is polled: one readdir plus one stat per day
// file, a few milliseconds for thousands of files, every few seconds.
//
// Like the Swift sources it ignores what is not ours to care about: hidden files (our own temp files, .DS_Store), assets/ and anything
// that is not a day file, an iCloud placeholder or a conflict copy, and the echo of our own writes (NoteOwnWrite / IsOwnWrite).
// Nothing is trusted but the stat: a change only means "look again", and the caller's own checks decide what changed.
package core

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FolderWatcher polls one folder and calls onChange (from its own goroutine) after a look in which something other than our own
// writes changed. The folder comes from a function, so a changed storage folder is followed: that counts as a change too.
type FolderWatcher struct {
	dir      func() string
	interval time.Duration
	onChange func()

	mu      sync.Mutex // guards stop/done (Start, Stop)
	stop    chan struct{}
	done    chan struct{}
	lastDir string            // only touched by the polling goroutine (and tests calling poll directly)
	last    map[string]string // name -> signature; nil until the first look, which is only the baseline
}

// NewFolderWatcher makes a stopped watcher. dir is asked on every look.
func NewFolderWatcher(dir func() string, interval time.Duration, onChange func()) *FolderWatcher {
	return &FolderWatcher{dir: dir, interval: interval, onChange: onChange}
}

// Start takes the baseline look and begins polling. Starting a started watcher does nothing.
func (w *FolderWatcher) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop != nil {
		return
	}
	w.stop, w.done = make(chan struct{}), make(chan struct{})
	w.poll() // the first look is only the baseline
	go func(stop <-chan struct{}, done chan<- struct{}) {
		defer close(done)
		t := time.NewTicker(w.interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if w.poll() {
					w.onChange()
				}
			}
		}
	}(w.stop, w.done)
}

// Stop ends polling; a look that is already running finishes first, and onChange is not called after Stop returns.
func (w *FolderWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop == nil {
		return
	}
	close(w.stop)
	<-w.done
	w.stop, w.done = nil, nil
}

// poll takes one look and says whether something changed since the one before.
func (w *FolderWatcher) poll() bool {
	dir := w.dir()
	now := watchSnapshot(dir)
	defer func() { w.last, w.lastDir = now, dir }()
	if w.last == nil {
		return false
	}
	if dir != w.lastDir {
		return true
	}
	changed := false
	for name, sig := range now {
		if w.last[name] != sig && !IsOwnWrite(CanonicalPath(filepath.Join(dir, name))) {
			changed = true
		}
	}
	for name := range w.last {
		if _, still := now[name]; !still {
			changed = true
		}
	}
	return changed
}

// watchSnapshot is name -> "mtime.ns size [dataless]" of every day file, placeholder and conflict copy in dir.
func watchSnapshot(dir string) map[string]string {
	out := map[string]string{}
	entries, _ := os.ReadDir(dir) // a folder that cannot be listed looks empty; its coming back is then a change
	for _, e := range entries {
		if ClassifyFolderEntry(e.Name()).Kind == EntryIgnored {
			continue
		}
		path := filepath.Join(dir, e.Name())
		sig, ok := fileSignature(path)
		if !ok {
			sig = "-"
		}
		if pathIsDataless(path) {
			sig += " dataless"
		}
		out[e.Name()] = sig
	}
	return out
}
