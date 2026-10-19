// daywriter.go - the single place a captured note is routed. Port of app/Core/Capture/DayWriter.swift.
//
//	Capture(text, source)   normalise (trim, strip control chars except newlines, cap 4,000, "[]" to-do shorthand, timestamp per prefs),
//	                        return ok=false when nothing is left; journal.Append (durable) FIRST; then route:
//	                          - page open for that day and loaded -> page.AppendJot (live editor transaction); on JotAppended the editor's
//	                            own autosave writes the file, and AcknowledgeSaved(day, body) acks the note once the file holds it;
//	                            JotNotLoaded -> keep waiting (retried by ReplayJournal); JotStale -> re-route once;
//	                            JotAlreadyPresent -> ack when saved
//	                          - no open page for that day -> load the file (or ""), ApplyJot, LogStore.Save, ack
//	                          - the folder is unavailable or the save fails -> the note stays in the journal (Waiting > 0), nothing is lost
//	CaptureAt(text, source, at)   the same, dated with `at` (the inbox's own timestamp); Capture is CaptureAt(clock())
//	DrainInbox(dir, mode)   every entry of the inbox becomes a jot (source "claude"), then its file is deleted (see inbox.go for mode)
//	ReplayJournal()         re-route every pending note (launch, folder returns, wake, app activation); idempotent via JotPresent
//	Waiting()               journal notes not yet acknowledged
//	AcknowledgeSaved        the UI calls this after every successful page save; acks pending notes whose line is now in the written body
//
// Details (what the contract leaves open):
//   - Normalising: CR, CRLF, NEL, U+2028 and U+2029 become "\n"; tab, VT and FF become a space; every other control character is
//     dropped; the text is trimmed and capped at 4,000 characters (runes here; Swift counted grapheme clusters). "[]" or "[ ]" at the
//     start makes a to-do when todoShorthand is on (no time stamp, the marker is removed; a note that is only the marker is empty).
//     The time stamp is HH:mm of the capture time in the calendar's zone, the day is the day of that time and never changes: a note
//     captured at 23:59 and delivered after midnight still belongs to the day it was captured on.
//   - CaptureReceipt.Waiting and Waiting() are literal: every note that is not acknowledged yet counts, so right after a capture into
//     an OPEN page it is 1 until that page's autosave has been reported through AcknowledgeSaved. IsInOpenPage(id) tells that case
//     apart from a note that waits for the folder.
//   - The writer never writes a day's file while an editor exists for that day (loaded or not, orphans and hidden windows included):
//     the live page owns the text. Only a day with no page at all goes through LogStore, one load + one save per day for all its pending
//     notes in capture order, so a replay of a backlog makes one backup, not one per note.
//   - A capture routes ALL pending notes in order, not only the new one: notes that waited for the folder arrive before newer ones.
//   - Idempotent delivery: the file path checks JotPresent (or the note's line as the page editor spelled it when it took the note,
//     editorKeys: a bare link comes back as <https://...>) before it adds a line, and acknowledges after a successful save even when
//     the page could not show the line, so a replay can never add a note twice. The editor path sends every note with the editor's own
//     "unless present" rule. A skipped day (marker file) is written over, exactly like "write a log anyway".
//   - The journal is the durability point. If its write fails (disk full), the note is still routed, kept in memory as "not
//     journaled", and appended to the journal at the next capture or replay; Waiting counts it.
//   - Transient answers (JotNotLoaded, JotStale twice) are retried by a bounded timer (RetryDelay, MaxAutoRetries; ScheduleRetry is the
//     seam tests replace); every ReplayJournal starts the budget again. A call to the editor that never answers is sent again after 5 s.
//   - The first ReplayJournal of a writer compacts the journal (acknowledged notes are dropped).
//
// Go vs Swift: the Swift writer was main-thread only. This one has a mutex, so any goroutine may call it. The lock is NOT held while a
// PageHandle is called (a callback may answer at once, or later from another goroutine). Only one routing pass runs at a time: a call
// that arrives during a pass (or a completion that needs a new pass) only asks the running pass to go round again, so a Capture made
// while another goroutine is routing may return before its own note has been delivered (its Waiting is then still 1). The note is
// safe in the journal either way. Callers that need the exact receipt should capture from one goroutine, as the Swift app did.
package core

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unicode"
)

const (
	// CaptureMaxChars is the longest note kept.
	CaptureMaxChars = 4000
	// editorAnswerTimeout: a page that never answers an append is sent the note again after this long.
	editorAnswerTimeout = 5 * time.Second
)

// sentNote is a note handed to a live page: confirmed once the page answered JotAppended (or JotAlreadyPresent and the file does not
// hold it yet).
type sentNote struct {
	page      PageHandle
	at        time.Time
	confirmed bool
}

type pendingSend struct {
	page PageHandle
	item CaptureItem
}

// DayWriter routes captured notes. Make one with NewDayWriter.
type DayWriter struct {
	store   *LogStore
	journal *CaptureJournal
	pages   EditorLookup
	prefs   func() CapturePrefs
	lists   func() ListsPrefs // nil: everything goes to Work (SetLists)
	clock   func() time.Time
	cal     func() Calendar

	// RetryDelay and MaxAutoRetries bound the retry of transient answers. ScheduleRetry runs work after delay (default: time.AfterFunc).
	RetryDelay     time.Duration
	MaxAutoRetries int
	ScheduleRetry  func(delay time.Duration, work func())

	mu          sync.Mutex
	undeletable map[string]bool // inbox files taken once whose delete failed, so the next drain does not add them a second time
	queue       []CaptureItem   // not acknowledged yet, in capture order: the journal's pending notes plus unjournaled ones
	loaded      bool
	unjournaled map[string]bool
	sent        map[string]sentNote
	editorKeys  map[string]string // note id -> its line as the page editor wrote it (it may spell it differently from RenderJot)
	staleTried  map[string]bool
	routing     bool
	again       bool
	needsRetry  bool
	retrySet    bool
	compacted   bool
	autoRetries int
}

// NewDayWriter makes a writer. prefs, clock and calendar are read on every call, so settings changes apply at once.
func NewDayWriter(store *LogStore, journal *CaptureJournal, pages EditorLookup, prefs func() CapturePrefs,
	clock func() time.Time, calendar func() Calendar) *DayWriter {
	return &DayWriter{
		store: store, journal: journal, pages: pages, prefs: prefs, clock: clock, cal: calendar,
		RetryDelay: 500 * time.Millisecond, MaxAutoRetries: 12,
		ScheduleRetry: func(d time.Duration, work func()) { time.AfterFunc(d, work) },
		unjournaled:   map[string]bool{}, sent: map[string]sentNote{}, editorKeys: map[string]string{}, staleTried: map[string]bool{},
	}
}

// ---- public ----

// Capture adds a note dated now (the writer's clock). ok is false when nothing is left of the text (empty, only to-do brackets).
func (w *DayWriter) Capture(text, source string) (CaptureReceipt, bool) {
	return w.CaptureAt(text, source, w.clock())
}

// CaptureAt is Capture dated with at: the note's day and time stamp come from at, not from the clock. It goes to the list for
// notes without a folder (ListsPrefs.Jots).
func (w *DayWriter) CaptureAt(text, source string, at time.Time) (CaptureReceipt, bool) {
	return w.captureIn(text, source, "", at)
}

// captureIn is CaptureAt for a note routed to list; "" means the list for notes without a folder.
func (w *DayWriter) captureIn(text, source, list string, at time.Time) (CaptureReceipt, bool) {
	if list == "" {
		list = w.listsPrefs().Jots
	}
	p := w.prefs()
	norm, todo, ok := normaliseCapture(text, p.TodoShorthand)
	if !ok {
		return CaptureReceipt{}, false
	}
	loc := w.cal().location()
	local := at.In(loc)
	item := CaptureItem{ID: newUUID(), Ts: at, Day: DayKeyString(at, loc), Text: norm, Todo: todo, Source: source, List: list}
	if p.Timestamps && !todo {
		item.Stamp = fmt.Sprintf("%02d:%02d", local.Hour(), local.Minute())
	}
	w.mu.Lock()
	w.loadLocked()
	if err := w.journal.Append(item); err != nil { // durable first; if the disk refuses, keep it in memory
		w.unjournaled[item.ID] = true
	}
	w.queue = append(w.queue, item)
	w.autoRetries = 0
	w.mu.Unlock()
	w.routeAll(false)
	w.mu.Lock()
	defer w.mu.Unlock()
	return CaptureReceipt{ID: item.ID, Waiting: len(w.queue)}, true
}

// DrainInbox turns every entry waiting in dir into a jot (source "claude") and then deletes its file, as the Swift app did: the note
// is durable (journal first) before the file goes, so an entry is never lost between the two. The file is deleted even when nothing
// is left of its text. A file that could not be deleted is remembered: the next drain only retries the delete, so the note is never
// added twice (the Swift app added it again on every look). Returns how many entries were taken. mode: see InboxTimeMode.
func (w *DayWriter) DrainInbox(dir string, mode InboxTimeMode) int {
	n := 0
	for _, e := range InboxPending(dir) {
		w.mu.Lock()
		taken := w.undeletable[e.Path]
		w.mu.Unlock()
		if taken { // the note is in already; only the delete is left, so try that again and add nothing
			if err := os.Remove(e.Path); err == nil || errors.Is(err, os.ErrNotExist) {
				w.mu.Lock()
				delete(w.undeletable, e.Path)
				w.mu.Unlock()
			}
			continue
		}
		list := "" // no folder: the list for notes without one
		if e.Cwd != "" {
			list = w.listsPrefs().ListFor(e.Cwd)
		}
		w.captureIn(e.Text, "claude", list, e.CaptureTime(w.clock(), mode))
		if err := os.Remove(e.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			w.mu.Lock()
			if w.undeletable == nil {
				w.undeletable = map[string]bool{}
			}
			w.undeletable[e.Path] = true // remembered: taking the entry again would add the note a second time
			w.mu.Unlock()
		}
		n++
	}
	return n
}

// ReplayJournal re-routes every pending note (idempotent).
func (w *DayWriter) ReplayJournal() {
	w.mu.Lock()
	w.loadLocked()
	w.autoRetries = 0
	w.staleTried = map[string]bool{}
	w.mu.Unlock()
	w.routeAll(true)
	w.mu.Lock()
	first := !w.compacted
	w.compacted = true
	w.mu.Unlock()
	if first {
		_ = w.journal.Compact() // at launch: drop what is acknowledged; best effort, the next launch tries again
	}
}

// Waiting is how many notes are not acknowledged yet.
func (w *DayWriter) Waiting() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.loadLocked()
	return len(w.queue)
}

// AcknowledgeSaved acks the pending notes of day whose line is now in body, the text just written to that day's file.
func (w *DayWriter) AcknowledgeSaved(day, body string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.loadLocked()
	var mine []CaptureItem
	for _, it := range w.queue {
		if it.Day == day {
			mine = append(mine, it)
		}
	}
	if len(mine) == 0 {
		return
	}
	keys := jotKeysOf(body, w.prefs().JotsHeading)
	for _, it := range mine {
		if w.holds(it, keys) {
			w.ack(it)
		}
	}
}

// holds is true when keys (the jot keys of a page or a file) hold the note: as RenderJot wrote it, or as the page editor spelled it
// when it took the note (a link it writes as <https://...>, for one, which RenderJot never produces). Without the second form such a
// note would never be acknowledged, stay "waiting" for ever and be added a second time when it is next delivered through the file.
func (w *DayWriter) holds(item CaptureItem, keys map[string]bool) bool {
	if keys[jotKey(RenderJot(item))] {
		return true
	}
	k, ok := w.editorKeys[item.ID]
	return ok && keys[k]
}

// IsInOpenPage is true while the note sits in a live editor page that has accepted it and will save it: it is not in the file yet, but
// it is not waiting for the folder either. A panel should say "kept on this Mac" only for a note where this is false and Waiting() > 0.
func (w *DayWriter) IsInOpenPage(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.sent[id]
	if !ok || !s.confirmed || s.page == nil {
		return false
	}
	for _, it := range w.queue {
		if it.ID == id {
			return w.pages.Page(it.Day) == s.page
		}
	}
	return false
}

// ---- normalising ----

// normaliseCapture: ok is false when nothing is left.
func normaliseCapture(raw string, todoShorthand bool) (text string, todo, ok bool) {
	var b []rune
	afterCR := false
	for _, u := range raw {
		switch u {
		case 0x0A:
			if !afterCR { // the LF of a CRLF is not a second break
				b = append(b, '\n')
			}
		case 0x0D, 0x85, 0x2028, 0x2029:
			b = append(b, '\n')
		case 0x09, 0x0B, 0x0C:
			b = append(b, ' ')
		default:
			if !unicode.IsControl(u) {
				b = append(b, u)
			}
		}
		afterCR = u == 0x0D
	}
	text = trimmed(string(b))
	if todoShorthand {
		if len(text) >= 2 && text[:2] == "[]" {
			text, todo = trimmed(text[2:]), true
		} else if len(text) >= 3 && text[:3] == "[ ]" {
			text, todo = trimmed(text[3:]), true
		}
	}
	if rs := []rune(text); len(rs) > CaptureMaxChars {
		text = trimmed(string(rs[:CaptureMaxChars]))
	}
	if text == "" {
		return "", false, false
	}
	return text, todo, true
}

// ---- state (callers hold w.mu) ----

func (w *DayWriter) loadLocked() {
	if w.loaded {
		return
	}
	w.loaded = true
	w.queue = w.journal.Pending()
}

func (w *DayWriter) isPending(id string) bool {
	for _, it := range w.queue {
		if it.ID == id {
			return true
		}
	}
	return false
}

// ack: acknowledged = gone from the queue and, when it was ever journaled, from the journal.
func (w *DayWriter) ack(item CaptureItem) {
	kept := w.queue[:0:0]
	for _, it := range w.queue {
		if it.ID != item.ID {
			kept = append(kept, it)
		}
	}
	w.queue = kept
	delete(w.sent, item.ID)
	delete(w.editorKeys, item.ID)
	delete(w.staleTried, item.ID)
	if w.unjournaled[item.ID] {
		delete(w.unjournaled, item.ID)
	} else {
		_ = w.journal.Ack(item.ID) // best effort: a lost ack only means a replay, which JotPresent makes harmless
	}
}

func (w *DayWriter) persistUnjournaled() {
	for _, it := range w.queue {
		if w.unjournaled[it.ID] && w.journal.Append(it) == nil {
			delete(w.unjournaled, it.ID)
		}
	}
}

// ---- routing ----

// routeAll runs routing passes, one at a time: a completion that asks for another pass while one is running only sets `again`.
func (w *DayWriter) routeAll(resend bool) {
	w.mu.Lock()
	if w.routing {
		w.again = true
		w.mu.Unlock()
		return
	}
	w.routing = true
	w.needsRetry = false
	passes := 0
	doResend := resend
	for {
		w.again = false
		passes++
		sends := w.routePass(doResend)
		doResend = false
		w.mu.Unlock()
		for _, s := range sends { // outside the lock: the page may answer at once
			s := s
			s.page.AppendJot(s.item, func(r JotAppendResult) { w.completed(s.item, s.page, r) })
		}
		w.mu.Lock()
		if !w.again || passes >= 3 {
			break
		}
	}
	w.routing = false
	w.mu.Unlock()
	w.scheduleRetryIfNeeded()
}

// routePass delivers what it can through the files and returns the notes to hand to live pages.
func (w *DayWriter) routePass(resend bool) []pendingSend {
	w.persistUnjournaled()
	var days []string
	seen := map[string]bool{}
	for _, it := range w.queue {
		if !seen[it.Day] {
			seen[it.Day] = true
			days = append(days, it.Day)
		}
	}
	var sends []pendingSend
	for _, day := range days {
		var items []CaptureItem
		for _, it := range w.queue {
			if it.Day == day {
				items = append(items, it)
			}
		}
		if page := w.pages.Page(day); page != nil {
			sends = append(sends, w.prepareSends(items, page, resend)...)
		} else {
			w.deliverToFile(day, items)
		}
	}
	return sends
}

// prepareSends is the editor path: pick each pending note of the day to hand to the live page, in order.
func (w *DayWriter) prepareSends(items []CaptureItem, page PageHandle, resend bool) []pendingSend {
	if !page.IsLoaded() {
		w.needsRetry = true
		return nil
	}
	var out []pendingSend
	for _, it := range items {
		if !w.isPending(it.ID) {
			continue
		}
		if s, ok := w.sent[it.ID]; ok && s.page == page {
			if s.confirmed {
				if !resend {
					continue // already in the page, waiting for its autosave
				}
			} else if w.clock().Sub(s.at) < editorAnswerTimeout {
				continue // still being answered
			}
		}
		w.sent[it.ID] = sentNote{page: page, at: w.clock()}
		out = append(out, pendingSend{page, it})
	}
	return out
}

// completed is the page's answer to AppendJot.
func (w *DayWriter) completed(item CaptureItem, page PageHandle, r JotAppendResult) {
	w.mu.Lock()
	if !w.isPending(item.ID) { // acknowledged while the call was in flight
		delete(w.sent, item.ID)
		w.mu.Unlock()
		return
	}
	reroute := false
	switch r.Kind {
	case JotAppended:
		if k, ok := LastJotKey(r.NormalizedMarkdown, JotsHeadingIn(r.NormalizedMarkdown, w.prefs().JotsHeading, item.List)); ok { // the note is the last item of its section
			w.editorKeys[item.ID] = k
		}
		w.sent[item.ID] = sentNote{page: page, at: w.clock(), confirmed: true} // the editor's autosave writes it; AcknowledgeSaved acks it
	case JotAlreadyPresent:
		if w.fileHolds(item) {
			w.ack(item)
		} else {
			w.sent[item.ID] = sentNote{page: page, at: w.clock(), confirmed: true}
		}
	case JotNotLoaded:
		delete(w.sent, item.ID)
		w.needsRetry = true
	case JotStale:
		delete(w.sent, item.ID)
		if !w.staleTried[item.ID] {
			w.staleTried[item.ID] = true
			reroute = true // re-route once, then leave it to the retry
		} else {
			w.needsRetry = true
		}
	}
	w.mu.Unlock()
	if reroute {
		w.routeAll(false)
	}
	w.scheduleRetryIfNeeded()
}

// fileHolds is true when the day's file already holds the note's line (never for a skipped day or an unreadable file).
func (w *DayWriter) fileHolds(item CaptureItem) bool {
	doc, err := w.store.Load(item.Day)
	if err != nil || doc == nil || doc.IsSkipped {
		return false
	}
	return w.holds(item, jotKeysOf(doc.Body, w.prefs().JotsHeading))
}

// jotKeysOf are the jot keys of a page: every list of a list page (a note may sit in either), else the Jots heading's.
func jotKeysOf(body, configured string) map[string]bool {
	p, ok := ParseLog(body)
	if !ok {
		return JotKeys(body, JotsHeadingFor(body, configured))
	}
	keys := map[string]bool{}
	for _, s := range p.Sections {
		if !s.Virtual {
			for k := range JotKeys(body, s.Name) {
				keys[k] = true
			}
		}
	}
	return keys
}

// listsPrefs are the routing settings; Work for everything until the app sets them.
func (w *DayWriter) listsPrefs() ListsPrefs {
	if w.lists == nil {
		return ListsPrefs{Jots: ListWork}
	}
	return w.lists()
}

// SetLists gives the writer the Work / Personal routing (read on every note, so a settings change applies at once).
func (w *DayWriter) SetLists(lists func() ListsPrefs) { w.lists = lists }

// deliverToFile is the file path: nobody has this day open. Load once, add every pending note that is not in the page yet, save
// once, acknowledge. Any failure (folder missing or read-only, unreadable page, failed write) leaves every note pending.
func (w *DayWriter) deliverToFile(day string, items []CaptureItem) {
	doc, err := w.store.Load(day) // nil = no file yet
	if err != nil {
		return // stays in the journal: ReplayJournal delivers it when the folder (or the page) is back
	}
	body := ""
	if doc != nil {
		body = doc.Body // a skip marker has an empty body: it is written over
	}
	changed := false
	for _, it := range items {
		if !w.isPending(it.ID) {
			continue
		}
		if w.holds(it, jotKeysOf(body, w.prefs().JotsHeading)) {
			continue
		}
		// its list on a list page (or on a new day when new days are list pages), else the Jots heading
		body = ApplyJot(it, body, JotsHeadingIn(body, w.prefs().JotsHeading, it.List))
		changed = true
	}
	if changed {
		if _, err := w.store.Save(day, body); err != nil {
			return
		}
	}
	for _, it := range items {
		if w.isPending(it.ID) {
			w.ack(it)
		}
	}
}

func (w *DayWriter) scheduleRetryIfNeeded() {
	w.mu.Lock()
	if !w.needsRetry || w.retrySet || w.autoRetries >= w.MaxAutoRetries || len(w.queue) == 0 {
		w.mu.Unlock()
		return
	}
	w.needsRetry, w.retrySet = false, true
	w.autoRetries++
	delay, schedule := w.RetryDelay, w.ScheduleRetry
	w.mu.Unlock()
	schedule(delay, func() {
		w.mu.Lock()
		w.retrySet = false
		w.mu.Unlock()
		w.routeAll(false)
	})
}
