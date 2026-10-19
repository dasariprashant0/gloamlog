package main

// Quick capture: the Jot panel, the Claude inbox and the journal replay. The rules (journal first, placement under the
// Jots heading, idempotent replay) live in internal/core's DayWriter; this file only wires it to the app:
//
//   - the journal is ~/Library/Application Support/Gloamlog/capture/journal.jsonl, the inbox the sibling "inbox" folder;
//   - a note for a day whose page is OPEN in the main window goes through the editor, never the file underneath it. The
//     page tells us it is open (EditorOpened / EditorClosed), we emit the "jot-into-page" event and the page answers
//     through JotPlaced. A page that does not answer within jotAnswerTimeout is treated as gone and the note goes to
//     the file (core would otherwise keep resending to it);
//   - everything else is written to the file by the writer, one save per day.
//
// Events emitted:  "jot-into-page" {id, day, heading, markdown}   a page must add the line (see JotPlaced)
//                  "capture-changed" {waiting}                      something was delivered or journaled; refresh the sidebar
//                  "refresh-menubar" {}                             the menu bar item re-reads today's state
//                  "refresh-sources" {}                             gloamlog://refresh: the suggestions strip should look again
// Calls the pages make: EditorOpened, EditorClosed, JotPlaced, EditorSaved (all on Notes).
// main.go calls startCapture once; handleURL is for gloamlog://refresh.

import (
	"strings"
	"sync"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	jotAnswerTimeout = 4 * time.Second // shorter than the writer's own 5 s "send it again"
	captureTick      = 15 * time.Second
	keptMessage      = "Kept on this Mac. It will be added when your log folder is back."
)

// JotResult is what the Jot panel shows after Return.
type JotResult struct {
	Outcome string `json:"outcome"` // "added" | "kept" | "empty" (nothing left of the text)
	Message string `json:"message"`
	Waiting int    `json:"waiting"` // notes journaled but not in a page yet
	Day     string `json:"day"`
	Heading string `json:"heading"` // the heading the note goes under
}

// captureState is the capture machinery of one Notes. Made lazily by captureFor.
type captureState struct {
	n       *Notes
	journal *core.CaptureJournal
	store   *core.LogStore // same folder as n.store; its Dir follows it (syncFolder)
	writer  *core.DayWriter

	// emit sends an event to every window; the app sets it, tests record.
	emit          func(name string, data any)
	answerTimeout time.Duration

	mu      sync.Mutex
	handles map[string]*pageHandle // day -> its open page (nil entry = none)
	counts  map[string]int
	replies map[string]func(core.JotAppendResult)
	commits map[string]bool // "day|sha" of the commits handed to the writer this session (notes_commits.go)
}

var captures sync.Map // *Notes -> *captureState

// captureFor returns the capture state of n, making it on first use.
func captureFor(n *Notes) *captureState {
	if v, ok := captures.Load(n); ok {
		return v.(*captureState)
	}
	n.mu.Lock()
	dir := n.store.Dir
	n.mu.Unlock()
	c := &captureState{
		n:             n,
		journal:       core.NewCaptureJournal(core.DefaultCaptureDir(n.home)),
		store:         core.NewLogStore(dir, n.backupDir, 10*time.Minute, n.now),
		emit:          func(string, any) {},
		answerTimeout: jotAnswerTimeout,
		handles:       map[string]*pageHandle{},
		counts:        map[string]int{},
		replies:       map[string]func(core.JotAppendResult){},
		commits:       map[string]bool{},
	}
	c.writer = core.NewDayWriter(c.store, c.journal, c,
		func() core.CapturePrefs { n.mu.Lock(); defer n.mu.Unlock(); return n.settings.Capture },
		n.now,
		func() core.Calendar { n.mu.Lock(); defer n.mu.Unlock(); return n.cal() })
	c.writer.SetLists(func() core.ListsPrefs { n.mu.Lock(); defer n.mu.Unlock(); return n.settings.Lists })
	v, _ := captures.LoadOrStore(n, c)
	return v.(*captureState)
}

// syncFolder points the writer's store at the folder the settings name now. It reports whether the folder changed.
// ponytail: a plain field write; a folder change while a delivery runs on another goroutine is rare enough to ignore.
func (c *captureState) syncFolder() bool {
	c.n.mu.Lock()
	dir := c.n.store.Dir
	c.n.mu.Unlock()
	if dir == c.store.Dir {
		return false
	}
	c.store.Dir = dir
	return true
}

// headingFor is the heading a note routed to list goes under on `day`: its list on a list page (or a new day that starts as one),
// else the configured one. It reads the day's file through the writer's own store, outside n.mu, like the other store reads here.
func (c *captureState) headingFor(day, configured, list string) string {
	return pageHeadingIn(c.store, day, configured, list)
}

// pageHeading is pageHeadingIn for Work: the section suggestions go under on a classic page is the configured heading.
func pageHeading(store *core.LogStore, day, configured string) string {
	return pageHeadingIn(store, day, configured, core.ListWork)
}

// pageHeadingIn is the section new lines routed to list go under on day's page (core.JotsHeadingIn). An unreadable file falls
// back to configured. Used by capture and by the suggestions strip.
func pageHeadingIn(store *core.LogStore, day, configured, list string) string {
	doc, err := store.Load(day)
	if err != nil {
		return configured
	}
	body := ""
	if doc != nil {
		body = doc.Body
	}
	return core.JotsHeadingIn(body, configured, list)
}

// ---- the open pages (core.EditorLookup / core.PageHandle) ----

type pageHandle struct {
	c   *captureState
	day string
}

func (p *pageHandle) Day() string { return p.day }

// IsLoaded is part of core.EditorLookup's page interface. The handle can't see the editor, so it says yes and the page
// itself answers "notLoaded" (through JotPlaced) while its editor is still starting.
func (p *pageHandle) IsLoaded() bool { return true }

// AppendJot hands the line to the page through the "jot-into-page" event. The page's answer comes back as JotPlaced; no answer
// within the timeout means the page is gone: forget it and let the writer re-route (to the file).
func (p *pageHandle) AppendJot(item core.CaptureItem, done func(core.JotAppendResult)) {
	c := p.c
	c.mu.Lock()
	c.replies[item.ID] = done
	c.mu.Unlock()
	c.n.mu.Lock()
	heading := c.n.settings.Capture.JotsHeading
	c.n.mu.Unlock()
	list := item.List
	if list == "" {
		list = core.ListWork // a note from an older journal
	}
	heading = c.headingFor(p.day, heading, list)
	time.AfterFunc(c.answerTimeout, func() {
		c.mu.Lock()
		cb := c.replies[item.ID]
		delete(c.replies, item.ID)
		if cb != nil {
			delete(c.counts, p.day)
			delete(c.handles, p.day)
		}
		c.mu.Unlock()
		if cb != nil {
			cb(core.JotAppendResult{Kind: core.JotStale})
		}
	})
	c.emit("jot-into-page", map[string]any{"id": item.ID, "day": item.Day, "heading": heading, "list": list, "markdown": core.RenderJot(item), "timed": item.Timed})
}

// Page implements core.EditorLookup: a true nil interface when the day has no open page.
func (c *captureState) Page(day string) core.PageHandle {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h := c.handles[day]; h != nil {
		return h
	}
	return nil
}

func (c *captureState) pageOpen(day string) bool { return c.Page(day) != nil }

// EditorOpened: the main window's page for `day` exists (even while it loads) and owns that day's text until EditorClosed.
// Notes that were waiting for it go in now.
func (n *Notes) EditorOpened(day string) {
	c := captureFor(n)
	c.mu.Lock()
	c.counts[day]++
	if c.handles[day] == nil {
		c.handles[day] = &pageHandle{c, day}
	}
	c.mu.Unlock()
	go c.replay()
	if day == core.DayKeyString(n.now(), time.Local) {
		go n.autoAddCommits() // today's commits, while "Add commits for me" is on (notes_commits.go)
	}
}

// EditorClosed: the page for `day` is gone and everything it held is saved (call it after the last write).
func (n *Notes) EditorClosed(day string) {
	c := captureFor(n)
	c.mu.Lock()
	if c.counts[day]--; c.counts[day] <= 0 {
		delete(c.counts, day)
		delete(c.handles, day)
	}
	c.mu.Unlock()
	go c.replay() // notes the page could not take can go to the file now
}

// JotPlaced is the page's answer to "jot-into-page": result is the editor's appendResult ("appended", "alreadyPresent",
// "stale") or "notLoaded"; markdown is the editor's markdown after an "appended". An answer that comes after the timeout is ignored.
func (n *Notes) JotPlaced(id, result, markdown string) {
	c := captureFor(n)
	c.mu.Lock()
	done := c.replies[id]
	delete(c.replies, id)
	c.mu.Unlock()
	if done == nil {
		return
	}
	r := core.JotAppendResult{Kind: core.JotStale}
	switch result {
	case "appended":
		r = core.JotAppendResult{Kind: core.JotAppended, NormalizedMarkdown: markdown}
		// The page saves by itself; look at the file shortly after, in case nobody calls EditorSaved.
		for _, d := range []time.Duration{2 * time.Second, 6 * time.Second} {
			time.AfterFunc(d, c.ackFromFiles)
		}
	case "alreadyPresent":
		r.Kind = core.JotAlreadyPresent
	case "notLoaded":
		r.Kind = core.JotNotLoaded
	}
	done(r)
}

// EditorSaved: the page just wrote `day` with this body. Notes whose line is in it are done.
func (n *Notes) EditorSaved(day, body string) {
	c := captureFor(n)
	c.writer.AcknowledgeSaved(day, body)
	c.emit("refresh-menubar", nil)
}

// ---- jotting ----

// Jot adds a note to today's page: durable in the journal first, then placed (see the file header).
func (n *Notes) Jot(text string) (JotResult, error) { return captureFor(n).jot(text, "hotkey"), nil }

// JotsWaiting is how many notes are journaled but not in a page yet.
func (n *Notes) JotsWaiting() int { return captureFor(n).writer.Waiting() }

// RetryJots delivers what is waiting now ("Try now").
func (n *Notes) RetryJots() int {
	c := captureFor(n)
	c.replay()
	return c.writer.Waiting()
}

func (c *captureState) jot(text, source string) JotResult {
	c.syncFolder()
	c.n.mu.Lock()
	heading, list := c.n.settings.Capture.JotsHeading, c.n.settings.Lists.Jots
	c.n.mu.Unlock()
	day := core.DayKeyString(c.n.now(), time.Local)
	heading = c.headingFor(day, heading, list)
	res := JotResult{Outcome: "empty", Day: day, Heading: heading}
	rec, ok := c.writer.Capture(text, source)
	if !ok {
		return res
	}
	res.Waiting = rec.Waiting
	res.Outcome, res.Message = "added", "Added to today's page under "+heading+"."
	// Waiting is normal while an open page autosaves. It is "kept" only when the folder is unavailable, or no page is open
	// to take the note and the file did not (core's DayWriter header; Swift jotOutcome).
	if rec.Waiting > 0 && (c.store.CheckFolder(true) != nil || !c.pageOpen(day)) {
		res.Outcome, res.Message = "kept", keptMessage
	}
	c.notify()
	return res
}

func (c *captureState) notify() {
	c.emit("capture-changed", map[string]any{"waiting": c.writer.Waiting()})
	c.emit("refresh-menubar", nil)
}

// ---- retrying ----

// ackFromFiles acknowledges the pending notes whose line a day's file already holds (a page saved, or the file was written).
func (c *captureState) ackFromFiles() {
	seen := map[string]bool{}
	for _, it := range c.journal.Pending() {
		if seen[it.Day] {
			continue
		}
		seen[it.Day] = true
		if doc, err := c.store.Load(it.Day); err == nil && doc != nil && !doc.IsSkipped {
			c.writer.AcknowledgeSaved(it.Day, doc.Body)
		}
	}
}

// replay delivers whatever waits: at launch, on activation, when a page opens or closes, after a folder change.
func (c *captureState) replay() {
	c.syncFolder()
	c.ackFromFiles()
	c.writer.ReplayJournal()
	c.notify()
}

// drain turns the entries Claude Code left in the inbox into jots (dated by the file's own time stamp).
func (c *captureState) drain() {
	c.syncFolder()
	if c.writer.DrainInbox(core.DefaultInboxDir(c.n.home), core.InboxUseFileTime) > 0 {
		c.notify()
	}
}

// startCapture is called once from main after the app exists: replay the journal, drain the inbox now, on every activation
// and every 15 s.
func startCapture(app *application.App, n *Notes) {
	c := captureFor(n)
	c.emit = func(name string, data any) { app.Event.Emit(name, data) }
	both := func() { c.replay(); c.drain() }
	go both()
	app.Event.OnApplicationEvent(events.Mac.ApplicationDidBecomeActive, func(*application.ApplicationEvent) { go both() })
	app.Event.OnApplicationEvent(events.Mac.ApplicationDidWake, func(*application.ApplicationEvent) {
		time.AfterFunc(5*time.Second, both) // let the disk or network volume settle
	})
	go func() {
		for range time.Tick(captureTick) {
			if c.syncFolder() || c.writer.Waiting() > 0 {
				c.replay()
			}
			c.drain()
		}
	}()
}

// handleURL answers gloamlog://refresh (the Claude Code Stop hook): take what is in the inbox and re-read the sources.
// Hook it to the URL event: app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, ...) with e.Context().URL().
func handleURL(n *Notes, url string) {
	if !strings.HasPrefix(strings.ToLower(url), "gloamlog://refresh") {
		return
	}
	c := captureFor(n)
	c.drain()
	c.ackFromFiles()
	scans.mu.Lock()
	scans.key = "" // the next Suggestions call looks again
	scans.mu.Unlock()
	c.emit("refresh-sources", nil)
	go n.autoAddCommits() // a Claude Code session just ended: its commits too
}
