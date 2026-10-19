package core

// DayWriter had no unit tests in Swift (only editor-check scenarios: app/Tools/editor-check/ScenarioCapture.swift, sections g and i).
// These port those scenarios over a real folder and add the pieces the Go version made explicit: journal first, then page; crash
// replay idempotency; the retry seam; the inbox drain (including the failed-delete case); concurrent captures.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var capNow = mk(2026, 10, 7, 14, 32) // Wed 7 Oct 2026, 14:32: the writer's injected clock

const capToday = "2026-10-07"

// capPage is a PageHandle that answers as the test says: at once, never (hold), or with another kind.
type capPage struct {
	day      string
	loaded   bool
	received []CaptureItem
	reply    string // the whole page as the editor answers once it has taken the note
	kind     JotAppendKind
	hold     bool // never answers until release()
	held     []func()
	check    func(CaptureItem) // runs when the note arrives
}

func newCapPage(day string) *capPage { return &capPage{day: day, loaded: true} }

func (p *capPage) Day() string    { return p.day }
func (p *capPage) IsLoaded() bool { return p.loaded }
func (p *capPage) AppendJot(item CaptureItem, done func(JotAppendResult)) {
	p.received = append(p.received, item)
	if p.check != nil {
		p.check(item)
	}
	answer := func() { done(JotAppendResult{Kind: p.kind, NormalizedMarkdown: p.reply}) }
	if p.hold {
		p.held = append(p.held, answer)
		return
	}
	answer()
}
func (p *capPage) release() {
	h := p.held
	p.held = nil
	for _, f := range h {
		f()
	}
}

type capLookup struct{ pages map[string]PageHandle }

func newCapLookup() *capLookup { return &capLookup{pages: map[string]PageHandle{}} }
func (l *capLookup) Page(day string) PageHandle {
	if p, ok := l.pages[day]; ok {
		return p
	}
	return nil // a true nil interface
}

// capCore is a log folder, a backup folder and a journal folder in a temp dir.
type capCore struct {
	t                         *testing.T
	logs, backups, journalDir string
	store                     *LogStore
}

func newCapCore(t *testing.T) *capCore {
	t.Helper()
	dir := t.TempDir()
	c := &capCore{t: t, logs: filepath.Join(dir, "logs"), backups: filepath.Join(dir, "backups"), journalDir: filepath.Join(dir, "capture")}
	must(t, os.MkdirAll(c.logs, 0o755))
	must(t, os.MkdirAll(c.backups, 0o755))
	c.store = NewLogStore(c.logs, c.backups, 0, nil)
	return c
}

func (c *capCore) writer(pages EditorLookup, now func() time.Time) *DayWriter {
	if pages == nil {
		pages = newCapLookup()
	}
	if now == nil {
		now = func() time.Time { return capNow }
	}
	return NewDayWriter(c.store, NewCaptureJournal(c.journalDir), pages, DefaultCapturePrefs, now, func() Calendar { return ny })
}

func (c *capCore) pending() []CaptureItem { return NewCaptureJournal(c.journalDir).Pending() }

// file is the day's file text, "" when there is none.
func (c *capCore) file(day string) string {
	b, err := os.ReadFile(filepath.Join(c.logs, day+".md"))
	if err != nil {
		return ""
	}
	return string(b)
}

func (c *capCore) body(day string) string {
	doc, err := c.store.Load(day)
	must(c.t, err)
	if doc == nil {
		return ""
	}
	return doc.Body
}

func (c *capCore) exists(day string) bool {
	_, err := os.Stat(filepath.Join(c.logs, day+".md"))
	return err == nil
}

func countOf(haystack, needle string) int { return strings.Count(haystack, needle) }

func lock(t *testing.T, path string) {
	t.Helper()
	chmod(t, path, 0o555)
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) }) // so the temp folder can be removed whatever happens
}

// ---- normalising ----

func TestNormaliseCapture(t *testing.T) {
	long := strings.Repeat("a", 5000)
	cases := []struct {
		in       string
		shortcut bool
		text     string
		todo     bool
		ok       bool
	}{
		{"  hello  ", true, "hello", false, true},
		{"a\r\nb", true, "a\nb", false, true},
		{"a\rb", true, "a\nb", false, true},
		{"a\r\n\r\nb", true, "a\n\nb", false, true},
		{"a b c\u0085d", true, "a\nb\nc\nd", false, true},
		{"a\tb\u000bc\u000cd", true, "a b c d", false, true},
		{"a\x00b\x07c\x1bd\x7fe", true, "abcde", false, true},
		{"[] call Sam", true, "call Sam", true, true},
		{"[ ] call Sam", true, "call Sam", true, true},
		{"[]call Sam", true, "call Sam", true, true},
		{"[]", true, "", false, false},
		{"[ ]  \n ", true, "", false, false},
		{"[] x", false, "[] x", false, true},
		{"x []", true, "x []", false, true},
		{"   \n\t ", true, "", false, false},
		{"\x00\x01", true, "", false, false},
		{long, true, long[:CaptureMaxChars], false, true},
		{strings.Repeat("a", 3999) + " " + strings.Repeat("b", 10), true, strings.Repeat("a", 3999), false, true},
		{"café \U0001F389", true, "café \U0001F389", false, true},
	}
	for _, c := range cases {
		text, todo, ok := normaliseCapture(c.in, c.shortcut)
		expect(t, ok == c.ok && text == c.text && todo == c.todo, "%q: got (%q, %v, %v) want (%q, %v, %v)", c.in, text, todo, ok, c.text, c.todo, c.ok)
	}
}

func TestCaptureItemFields(t *testing.T) {
	c := newCapCore(t)
	prefs := DefaultCapturePrefs()
	w := NewDayWriter(c.store, NewCaptureJournal(c.journalDir), newCapLookup(), func() CapturePrefs { return prefs },
		func() time.Time { return capNow }, func() Calendar { return ny })
	r, ok := w.Capture("call Sam", "hotkey")
	expect(t, ok && r.ID != "" && r.Waiting == 0, "delivered at once into the file")
	_, ok = w.Capture("[]", "hotkey")
	expect(t, !ok, "nothing left: no note")
	// the fields as the journal saw them (delivered notes are acked, so read the raw journal)
	var rec map[string]any
	lines := jlines(jfile(c.journalDir))
	must(t, json.Unmarshal([]byte(lines[0]), &rec))
	expect(t, rec["day"] == capToday && rec["stamp"] == "14:32" && rec["source"] == "hotkey" && rec["todo"] == false && rec["id"] == r.ID, lines[0])
	expect(t, rec["ts"] == "2026-10-07T18:32:00.000Z", "%v", rec["ts"])
	expect(t, len(lines) == 2 && lines[1] == `{"ack":"`+r.ID+`"}`, "journal first, ack after delivery: %v", lines)

	prefs.Timestamps = false
	w.Capture("no stamp", "menubar")
	prefs.Timestamps = true
	w.Capture("[] to do", "menu")
	body := c.body(capToday)
	expect(t, body == "## Jots\n\n- 14:32 call Sam\n- no stamp\n- [ ] to do", "%q", body)
	prefs.TodoShorthand = false
	w.Capture("[] literal", "menu")
	expect(t, strings.HasSuffix(c.body(capToday), "\n- 14:32 \\[\\] literal") || strings.HasSuffix(c.body(capToday), "\n- 14:32 [] literal"), "%q", c.body(capToday))
}

func TestCaptureAtDatesTheNoteWithItsOwnTime(t *testing.T) {
	c := newCapCore(t)
	w := c.writer(nil, nil)
	_, ok := w.CaptureAt("from Claude", "claude", mk(2026, 10, 6, 23, 59))
	expect(t, ok)
	expect(t, c.exists("2026-10-06") && !c.exists(capToday), "the note's own day, not the clock's")
	expect(t, c.body("2026-10-06") == "## Jots\n\n- 23:59 from Claude", "%q", c.body("2026-10-06"))
}

// ---- journal first, then the page ----

func TestJournalFirstThenPageEditorRoute(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	lookup.pages[capToday] = page
	sawJournaled := false
	page.check = func(it CaptureItem) {
		for _, p := range c.pending() {
			if p.ID == it.ID {
				sawJournaled = true
			}
		}
	}
	w := c.writer(lookup, nil)
	r, _ := w.Capture("call with Sam about pricing", "hotkey")
	expect(t, sawJournaled, "the note was durable in the journal before the page ever saw it")
	expect(t, r.Waiting == 1 && len(page.received) == 1 && page.received[0].Text == "call with Sam about pricing", "waiting until the page has saved it")
	expect(t, !c.exists(capToday), "the writer never writes a day's file while an editor exists for it")
}

func TestJournalFirstThenFileRoute(t *testing.T) {
	c := newCapCore(t)
	sawJournaled := false
	var w *DayWriter
	lookup := &spyLookup{onPage: func() {
		if len(c.pending()) == 1 {
			sawJournaled = true
		}
	}}
	w = c.writer(lookup, nil)
	r, _ := w.Capture("note", "hotkey")
	expect(t, sawJournaled, "the journal held the note when the writer went to route it")
	expect(t, r.Waiting == 0 && c.exists(capToday) && len(c.pending()) == 0, "delivered and acknowledged")
}

type spyLookup struct{ onPage func() }

func (s *spyLookup) Page(string) PageHandle { s.onPage(); return nil }

// ---- crash and replay ----

func TestCrashAfterPageTookTheNoteReplayWritesItOnce(t *testing.T) {
	c := newCapCore(t)
	line := "- 14:32 call with Sam about pricing"
	expected := "# 2026-10-07\n\n## Jots\n\n" + line + "\n"
	lookup := newCapLookup()
	page := newCapPage(capToday)
	lookup.pages[capToday] = page
	w1 := c.writer(lookup, nil)
	r, _ := w1.Capture("call with Sam about pricing", "hotkey")
	expect(t, r.Waiting == 1 && len(page.received) == 1, "a note handed to an open page is waiting (1) until that page has saved it")
	expect(t, len(c.pending()) == 1 && c.pending()[0].Text == "call with Sam about pricing", "durable in the journal")
	w1 = nil // the app is gone
	expect(t, !c.exists(capToday), "nothing was written to the log folder")
	w2 := c.writer(nil, nil)
	expect(t, w2.Waiting() == 1, "the next run knows the note is waiting")
	w2.ReplayJournal()
	expect(t, c.file(capToday) == expected && w2.Waiting() == 0 && len(c.pending()) == 0, "replay writes exactly one line: %q", c.file(capToday))
	w2.ReplayJournal()
	c.writer(nil, nil).ReplayJournal()
	expect(t, countOf(c.file(capToday), line) == 1 && c.file(capToday) == expected, "a second replay (and a third writer's) adds no duplicate")
}

func TestCrashAfterSaveBeforeAckAddsNoSecondCopy(t *testing.T) {
	c := newCapCore(t)
	item := CaptureItem{ID: newUUID(), Ts: capNow, Day: capToday, Text: "call with Sam about pricing", Stamp: "14:32", Source: "hotkey"}
	must(t, NewCaptureJournal(c.journalDir).Append(item))
	save(t, c.store, capToday, ApplyJot(item, "", "Jots"))
	before := c.file(capToday)
	w := c.writer(nil, nil)
	expect(t, w.Waiting() == 1, "the note still counts as waiting for the new run")
	w.ReplayJournal()
	expect(t, c.file(capToday) == before && len(c.pending()) == 0 && w.Waiting() == 0, "the file is byte-identical and the note is acknowledged")
}

func TestReplayMakesOneBackupForABacklogNotOnePerNote(t *testing.T) {
	c := newCapCore(t)
	save(t, c.store, capToday, "## What I did\n\nsome earlier writing here")
	w := c.writer(nil, nil)
	lock(t, c.logs)
	for _, s := range []string{"one", "two", "three", "four"} {
		w.Capture(s, "hotkey")
	}
	chmod(t, c.logs, 0o755)
	w.ReplayJournal()
	expect(t, w.Waiting() == 0 && countOf(c.body(capToday), "\n- 14:32 ") == 4 && strings.Contains(c.body(capToday), "## Jots\n\n- 14:32 one\n- 14:32 two"), "%q", c.body(capToday))
	entries, _ := os.ReadDir(filepath.Join(c.backups, capToday))
	expect(t, len(entries) == 1, "one load + one save per day: %d backups", len(entries))
}

func TestEditorSpellingBareLinkAcknowledgedAndNeverAddedTwice(t *testing.T) {
	link, spelled := "https://example.com/pricing for Sam", "- 14:32 <https://example.com/pricing> for Sam"
	// the page saved: acknowledged through AcknowledgeSaved
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.reply = "## Jots\n\n" + spelled
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	w.Capture(link, "hotkey")
	save(t, c.store, capToday, "## Jots\n\n"+spelled) // what the page's autosave writes
	w.AcknowledgeSaved(capToday, "## Jots\n\n"+spelled)
	expect(t, w.Waiting() == 0 && len(c.pending()) == 0, "waiting=%d pending=%v", w.Waiting(), c.pending())

	// saved but never acknowledged, then delivered through the file
	d := newCapCore(t)
	lookup2 := newCapLookup()
	page2 := newCapPage(capToday)
	page2.reply = "## Jots\n\n" + spelled
	lookup2.pages[capToday] = page2
	w2 := d.writer(lookup2, nil)
	w2.Capture(link, "hotkey")
	save(t, d.store, capToday, "## Jots\n\n"+spelled)
	before := d.file(capToday)
	delete(lookup2.pages, capToday) // the page is gone: the next delivery goes through the file
	w2.ReplayJournal()
	expect(t, d.file(capToday) == before && countOf(d.file(capToday), "pricing") == 1 && w2.Waiting() == 0 && len(d.pending()) == 0, "%q", d.file(capToday))
}

func TestAcknowledgeSavedOnlyAcksNotesTheBodyHolds(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	lookup.pages[capToday] = newCapPage(capToday)
	w := c.writer(lookup, nil)
	w.Capture("first", "hotkey")
	w.Capture("second", "hotkey")
	expect(t, w.Waiting() == 2)
	w.AcknowledgeSaved(capToday, "## Jots\n\n- 14:32 first")
	expect(t, w.Waiting() == 1 && c.pending()[0].Text == "second", "only the first is in the written body")
	w.AcknowledgeSaved("2026-10-08", "## Jots\n\n- 14:32 second")
	expect(t, w.Waiting() == 1, "another day's save acks nothing")
	w.AcknowledgeSaved(capToday, "## Jots\n\n- 14:32 first\n- 14:32 second")
	expect(t, w.Waiting() == 0 && len(c.pending()) == 0)
}

func TestTornJournalFragmentNeverGluesOntoALaterNote(t *testing.T) {
	c := newCapCore(t)
	w1 := c.writer(nil, nil)
	w1.Capture("first note", "hotkey")
	lock(t, c.logs)
	w1.Capture("second note while the folder is read-only", "hotkey")
	chmod(t, c.logs, 0o755)
	appendRaw(t, jfile(c.journalDir), `{"v":1,"id":"torn`)
	w2 := c.writer(nil, nil)
	expect(t, w2.Waiting() == 1 && len(c.pending()) == 1 && c.pending()[0].Text == "second note while the folder is read-only", "the notes before the torn line are still pending: %v", c.pending())
	w2.Capture("third note after the tear", "hotkey")
	b := c.body(capToday)
	expect(t, w2.Waiting() == 0 && len(c.pending()) == 0 && strings.Contains(b, "second note while the folder is read-only\n- 14:32 third note after the tear") && countOf(b, "first note") == 1, "%q", b)
}

func TestFolderAwayKeepsTheNoteAndDeliversItOnceWhenBack(t *testing.T) {
	c := newCapCore(t)
	line := "- 14:32 call with Sam about pricing"
	expected := "# 2026-10-07\n\n## Jots\n\n" + line + "\n"
	w := c.writer(nil, nil)
	lock(t, c.logs)
	r, _ := w.Capture("call with Sam about pricing", "menubar")
	expect(t, r.Waiting == 1 && !c.exists(capToday) && len(c.pending()) == 1, "the note is kept (waiting 1), nothing is written, the journal holds it")
	w.ReplayJournal()
	expect(t, w.Waiting() == 1 && !c.exists(capToday) && len(c.pending()) == 1, "replaying while it is still away changes nothing and loses nothing")
	chmod(t, c.logs, 0o755)
	w.ReplayJournal()
	expect(t, c.file(capToday) == expected && w.Waiting() == 0 && len(c.pending()) == 0, "folder back: delivered exactly once: %q", c.file(capToday))
	w.ReplayJournal()
	c.writer(nil, nil).ReplayJournal()
	expect(t, countOf(c.file(capToday), line) == 1, "more replays add nothing")
}

func TestFolderMissingEntirelyKeepsTheNote(t *testing.T) {
	c := newCapCore(t)
	must(t, os.RemoveAll(c.logs))
	w := c.writer(nil, nil)
	r, ok := w.Capture("note", "hotkey")
	expect(t, ok && r.Waiting == 1 && len(c.pending()) == 1, "kept while the folder is gone")
	must(t, os.MkdirAll(c.logs, 0o755))
	w.ReplayJournal()
	expect(t, w.Waiting() == 0 && c.body(capToday) == "## Jots\n\n- 14:32 note", "%q", c.body(capToday))
}

func TestNoteTakenAt2359BelongsToThatDayWheneverItIsDelivered(t *testing.T) {
	c := newCapCore(t)
	now := mk(2026, 10, 7, 23, 59)
	w := c.writer(nil, func() time.Time { return now })
	lock(t, c.logs)
	w.Capture("last thought of the day", "hotkey")
	now = mk(2026, 10, 8, 0, 4)
	chmod(t, c.logs, 0o755)
	w.ReplayJournal()
	expect(t, c.body(capToday) == "## Jots\n\n- 23:59 last thought of the day" && !c.exists("2026-10-08"), "%q", c.body(capToday))
}

func TestSkippedDayIsWrittenOver(t *testing.T) {
	c := newCapCore(t)
	must(t, c.store.Skip(capToday, "off sick"))
	w := c.writer(nil, nil)
	w.Capture("note anyway", "hotkey")
	doc, err := c.store.Load(capToday)
	must(t, err)
	expect(t, !doc.IsSkipped && doc.Body == "## Jots\n\n- 14:32 note anyway", "like \"write a log anyway\": %+v", doc)
}

func TestJournalWriteFailureStillRoutesAndPersistsLater(t *testing.T) {
	c := newCapCore(t)
	// the journal's folder cannot be made: its parent is a file
	blocker := filepath.Join(t.TempDir(), "file")
	must(t, os.WriteFile(blocker, []byte("x"), 0o644))
	c.journalDir = filepath.Join(blocker, "capture")
	w := c.writer(nil, nil)
	r, ok := w.Capture("saved without a journal", "hotkey")
	expect(t, ok && r.Waiting == 0 && c.body(capToday) == "## Jots\n\n- 14:32 saved without a journal", "still routed and delivered: %+v %q", r, c.body(capToday))

	// folder away AND journal broken: the note is waiting in memory, then reaches the journal and the file when both are back
	c2 := newCapCore(t)
	good := c2.journalDir
	blocker2 := filepath.Join(t.TempDir(), "file")
	must(t, os.WriteFile(blocker2, []byte("x"), 0o644))
	w2 := NewDayWriter(c2.store, &CaptureJournal{Dir: filepath.Join(blocker2, "capture")}, newCapLookup(), DefaultCapturePrefs,
		func() time.Time { return capNow }, func() Calendar { return ny })
	lock(t, c2.logs)
	r2, _ := w2.Capture("kept in memory", "hotkey")
	expect(t, r2.Waiting == 1 && w2.Waiting() == 1 && len(NewCaptureJournal(good).Pending()) == 0, "waiting counts the unjournaled note")
	chmod(t, c2.logs, 0o755)
	w2.journal = NewCaptureJournal(good) // the disk is back
	w2.ReplayJournal()
	expect(t, w2.Waiting() == 0 && c2.body(capToday) == "## Jots\n\n- 14:32 kept in memory", "%q", c2.body(capToday))
	expect(t, len(NewCaptureJournal(good).Pending()) == 0, "journaled, then delivered and acknowledged")
}

// ---- markdown-looking notes never change the page (the file route) ----

func TestMarkdownLookingNotesNeverChangeThePage(t *testing.T) {
	nasty := []string{"# x", "## Heading two", "- x", "+ x", "* x", "1. x", "12) x", "> quote", "---", "***", "===", "- - -", "```", "~~~x",
		"<!-- hidden", "see <!-- x", "a\n# b\n- c\n> d\n1. e", "#hashtag", "| a | b |", "  - indented"}
	c := newCapCore(t)
	seed := "## What I did\n\nSome real writing under the first heading, enough words to read as a proper page for the day.\n\n## Finished\n\n- Existing item\n\n## To do next\n\n- [ ] existing task"
	save(t, c.store, capToday, seed)
	w := c.writer(nil, nil)
	for _, text := range nasty {
		w.Capture(text, "menubar")
	}
	w.Capture("a note after the open comment", "menubar")
	body := c.body(capToday)
	var heads []string
	for _, s := range BodySections(body) {
		if s.Level > 0 {
			heads = append(heads, s.NormalizedTitle)
		}
	}
	expect(t, equal(heads, []string{"what i did", "finished", "to do next", "jots"}), "no heading appears but the page's own and Jots: %v", heads)
	expect(t, strings.HasPrefix(body, seed), "everything above the Jots block is untouched")
	expect(t, BodyCounts(body, "Jots").JotCount == len(nasty)+1, "each one is exactly one list item: %d of %d", BodyCounts(body, "Jots").JotCount, len(nasty)+1)
	other, paragraphs := 0, 0
	for _, n := range MDNodes(body) {
		switch n.Kind {
		case MDHeading, MDListItem:
		case MDParagraph:
			paragraphs++
		default:
			other++ // a quote, rule, code block or table made by a note
		}
	}
	expect(t, other == 0 && paragraphs == 1, "no quote, rule, code block or table, and no extra paragraph: other=%d paragraphs=%d", other, paragraphs)
	expect(t, c.file(capToday) == SerializeDay(capToday, body), "the file round-trips unchanged")
	w.ReplayJournal()
	expect(t, w.Waiting() == 0 && BodyCounts(c.body(capToday), "Jots").JotCount == len(nasty)+1, "all acknowledged, a replay adds nothing")
}

// ---- the editor route ----

func TestEditorRouteConfirmedNotesWaitForTheirAutosaveAndResendOnReplay(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	w.Capture("first", "hotkey")
	expect(t, len(page.received) == 1 && w.IsInOpenPage(w.queueIDs()[0]), "confirmed: it sits in the live page")
	w.Capture("second", "hotkey")
	expect(t, len(page.received) == 2 && page.received[1].Text == "second", "a capture sends only what the page has not been handed")
	w.ReplayJournal()
	expect(t, len(page.received) == 4, "a replay resends everything still waiting: %d", len(page.received))
	delete(lookup.pages, capToday)
	expect(t, !w.IsInOpenPage(w.queueIDs()[0]), "no page any more: no longer in an open page")
	expect(t, !w.IsInOpenPage("unknown-id"))
}

func (w *DayWriter) queueIDs() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ids(w.queue)
}

func TestEditorRouteUnansweredCallIsSentAgainAfterFiveSeconds(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.hold = true
	lookup.pages[capToday] = page
	now := capNow
	w := c.writer(lookup, func() time.Time { return now })
	w.Capture("first", "hotkey")
	now = now.Add(2 * time.Second)
	w.Capture("second", "hotkey")
	expect(t, len(page.received) == 2 && page.received[1].Text == "second", "still being answered: not sent again")
	now = now.Add(4 * time.Second)
	w.Capture("third", "hotkey")
	texts := []string{}
	for _, it := range page.received {
		texts = append(texts, it.Text)
	}
	expect(t, equal(texts, []string{"first", "second", "first", "third"}), "only the call silent for 5 s is sent again: %v", texts)
	page.hold = false
	page.release() // late answers for notes that are still pending are accepted
	expect(t, w.Waiting() == 3)
}

func TestEditorRouteTransientAnswersAreRetriedByABoundedTimer(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.kind = JotNotLoaded
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	var scheduled []func()
	w.ScheduleRetry = func(_ time.Duration, work func()) { scheduled = append(scheduled, work) }
	w.Capture("note", "hotkey")
	runs := 0
	for len(scheduled) > 0 {
		f := scheduled[0]
		scheduled = scheduled[1:]
		runs++
		f()
	}
	expect(t, runs == w.MaxAutoRetries, "the retry budget is %d, ran %d", w.MaxAutoRetries, runs)
	expect(t, w.Waiting() == 1 && len(c.pending()) == 1, "the note is still safe in the journal")
	// the page becomes ready; a replay starts the budget again
	page.kind = JotAppended
	w.ReplayJournal()
	expect(t, w.Waiting() == 1 && w.IsInOpenPage(w.queueIDs()[0]), "accepted by the page now")
	w.AcknowledgeSaved(capToday, "## Jots\n\n- 14:32 note")
	expect(t, w.Waiting() == 0)
}

func TestEditorRoutePageNotLoadedWaitsThenDelivers(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.loaded = false
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	var scheduled []func()
	w.ScheduleRetry = func(_ time.Duration, work func()) { scheduled = append(scheduled, work) }
	w.Capture("note", "hotkey")
	expect(t, len(page.received) == 0 && len(scheduled) == 1 && !c.exists(capToday), "nothing is sent to a page that is not loaded, a retry is scheduled, the file is not touched")
	page.loaded = true
	scheduled[0]()
	expect(t, len(page.received) == 1, "sent once the page has loaded")
}

func TestEditorRouteStaleIsReroutedOnceThenLeftToTheRetry(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.kind = JotStale
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	var scheduled []func()
	w.ScheduleRetry = func(_ time.Duration, work func()) { scheduled = append(scheduled, work) }
	w.Capture("note", "hotkey")
	expect(t, len(page.received) == 2, "re-routed once: %d calls", len(page.received))
	expect(t, len(scheduled) == 1, "then left to the retry timer")
	scheduled[0]()
	expect(t, len(page.received) == 3, "the retry asks again")
}

func TestEditorRouteAlreadyPresentAcksWhenTheFileHoldsTheNote(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.kind = JotAlreadyPresent
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	r, _ := w.Capture("note", "hotkey")
	expect(t, r.Waiting == 1, "the file does not hold it yet: waiting for the autosave")
	save(t, c.store, capToday, "## Jots\n\n- 14:32 note")
	w.ReplayJournal() // resend: the page says it is already there, and now the file agrees
	expect(t, w.Waiting() == 0 && len(c.pending()) == 0, "acknowledged")
}

// ---- the inbox ----

func writeInbox(t *testing.T, dir, name, content string, mod time.Time) {
	t.Helper()
	must(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, name)
	must(t, os.WriteFile(p, []byte(content), 0o644))
	setMTime(t, p, mod)
}

func TestDrainInboxTurnsEntriesIntoClaudeJotsAndDeletesTheFiles(t *testing.T) {
	c := newCapCore(t)
	inbox := filepath.Join(t.TempDir(), "inbox")
	writeInbox(t, inbox, "1.json", `{"text":"  Fixed the cards.  ","ts":"2026-10-07T18:20:00Z"}`, capNow.Add(-2*time.Minute))
	writeInbox(t, inbox, "2.json", `{"text":"second","ts":"2026-10-07T18:25:00Z"}`, capNow.Add(-1*time.Minute))
	writeInbox(t, inbox, "3.json", "not json", capNow)
	w := c.writer(nil, nil)
	n := w.DrainInbox(inbox, InboxUseFileTime)
	expect(t, n == 2, "two entries taken, junk left alone: %d", n)
	expect(t, c.body(capToday) == "## Jots\n\n- 14:20 Fixed the cards.\n- 14:25 second", "dated with the file's own ts: %q", c.body(capToday))
	left := dirNames(t, inbox)
	expect(t, equal(left, []string{"3.json"}), "the taken files are deleted, the junk file is not: %v", left)
	var rec map[string]any
	must(t, json.Unmarshal([]byte(jlines(jfile(c.journalDir))[0]), &rec))
	expect(t, rec["source"] == "claude", "source claude")
	expect(t, w.DrainInbox(inbox, InboxUseFileTime) == 0, "a second look finds nothing new")
}

func TestDrainInboxFileTimeVsDrainTime(t *testing.T) {
	inbox := filepath.Join(t.TempDir(), "inbox")
	writeInbox(t, inbox, "a.json", `{"text":"written yesterday night","ts":"2026-10-07T03:50:00Z"}`, capNow) // 23:50 on the 6th in New York
	c := newCapCore(t)
	c.writer(nil, nil).DrainInbox(inbox, InboxUseFileTime)
	expect(t, c.body("2026-10-06") == "## Jots\n\n- 23:50 written yesterday night" && !c.exists(capToday), "the entry's own day: %q", c.body("2026-10-06"))

	writeInbox(t, inbox, "a.json", `{"text":"written yesterday night","ts":"2026-10-07T03:50:00Z"}`, capNow)
	d := newCapCore(t)
	d.writer(nil, nil).DrainInbox(inbox, InboxUseDrainTime)
	expect(t, d.body(capToday) == "## Jots\n\n- 14:32 written yesterday night", "Swift's behaviour: the moment of draining: %q", d.body(capToday))

	writeInbox(t, inbox, "a.json", `{"text":"clock skew","ts":"2026-10-09T10:00:00Z"}`, capNow)
	e := newCapCore(t)
	e.writer(nil, nil).DrainInbox(inbox, InboxUseFileTime)
	expect(t, e.body(capToday) == "## Jots\n\n- 14:32 clock skew", "a ts in the future is not believed: %q", e.body(capToday))

	writeInbox(t, inbox, "a.json", `{"text":"no ts"}`, capNow)
	f := newCapCore(t)
	f.writer(nil, nil).DrainInbox(inbox, InboxUseFileTime)
	expect(t, f.body(capToday) == "## Jots\n\n- 14:32 no ts", "no ts: the drain time")
}

func TestDrainInboxDeletesAnEntryWhoseTextIsEmptyAfterNormalising(t *testing.T) {
	c := newCapCore(t)
	inbox := filepath.Join(t.TempDir(), "inbox")
	writeInbox(t, inbox, "a.json", `{"text":"\u0001\u0002"}`, capNow)
	n := c.writer(nil, nil).DrainInbox(inbox, InboxUseFileTime)
	expect(t, n == 1 && len(dirNames(t, inbox)) == 0 && !c.exists(capToday), "taken and deleted, nothing written")
}

func TestDrainInboxFailedDeleteNeverAddsTheNoteTwice(t *testing.T) {
	c := newCapCore(t)
	inbox := filepath.Join(t.TempDir(), "inbox")
	writeInbox(t, inbox, "a.json", `{"text":"only once please"}`, capNow)
	w := c.writer(nil, nil)
	chmod(t, inbox, 0o500) // the file cannot be removed
	t.Cleanup(func() { _ = os.Chmod(inbox, 0o755) })
	expect(t, w.DrainInbox(inbox, InboxUseDrainTime) == 1, "taken")
	expect(t, len(dirNames(t, inbox)) == 1, "but the delete failed")
	expect(t, w.DrainInbox(inbox, InboxUseDrainTime) == 0 && w.DrainInbox(inbox, InboxUseDrainTime) == 0, "later looks take nothing")
	expect(t, countOf(c.body(capToday), "only once please") == 1 && BodyCounts(c.body(capToday), "Jots").JotCount == 1, "one note, not three: %q", c.body(capToday))
	chmod(t, inbox, 0o755)
	expect(t, w.DrainInbox(inbox, InboxUseDrainTime) == 0, "nothing new")
	expect(t, len(dirNames(t, inbox)) == 0, "the delete is retried once it can work, and no note is added")
	expect(t, countOf(c.body(capToday), "only once please") == 1, "still one note")
}

// ---- concurrency ----

func TestConcurrentCapturesAllArriveExactlyOnce(t *testing.T) {
	c := newCapCore(t)
	w := c.writer(nil, nil)
	var wg sync.WaitGroup
	const goroutines, each = 8, 5
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				w.Capture("g"+itoa(g)+"n"+itoa(i), "hotkey")
			}
		}(g)
	}
	wg.Wait()
	w.ReplayJournal() // a capture that arrived during another goroutine's pass may still be waiting: a replay delivers it
	body := c.body(capToday)
	expect(t, w.Waiting() == 0 && len(c.pending()) == 0, "nothing left waiting: %d", w.Waiting())
	expect(t, BodyCounts(body, "Jots").JotCount == goroutines*each, "every note once: %d of %d", BodyCounts(body, "Jots").JotCount, goroutines*each)
	for g := 0; g < goroutines; g++ {
		for i := 0; i < each; i++ {
			expect(t, countOf(body+"\n", " g"+itoa(g)+"n"+itoa(i)+"\n") == 1, "g%dn%d once", g, i)
		}
	}
}

func TestNotesGoUnderLogOnListPages(t *testing.T) {
	defer SetJotsRules(JotsRules{Heading: "Jots"})
	SetJotsRules(JotsRules{Heading: "Jots", NewDaysLog: true})
	c := newCapCore(t)
	w := c.writer(newCapLookup(), func() time.Time { return mk(2026, 10, 14, 14, 32) })
	day := "2026-10-14"

	w.Capture("first note from Claude", "claude")
	got := c.body(day)
	expect(t, strings.HasPrefix(got, "## Log\n\n- ") && strings.HasSuffix(got, " first note from Claude"), "a new day starts as a list page: %q", got)

	w.Capture("second note", "claude")
	got = c.body(day)
	expect(t, strings.Count(got, "## Log") == 1 && !strings.Contains(got, "## Jots") && strings.HasSuffix(got, " second note"), "%q", got)

	save(t, c.store, "2026-10-13", "## What I did\n\nwords")
	expect(t, JotsHeadingFor(load(t, c.store, "2026-10-13").Body, "Jots") == "Jots", "a classic page keeps the Jots heading")
}

func TestNotesGoToTheirList(t *testing.T) {
	defer SetJotsRules(JotsRules{Heading: "Jots"})
	SetJotsRules(JotsRules{Heading: "Jots", NewDaysLog: true, NewDaysSplit: true})
	c := newCapCore(t)
	w := c.writer(newCapLookup(), func() time.Time { return mk(2026, 10, 14, 14, 32) })
	w.SetLists(func() ListsPrefs {
		return ListsPrefs{Work: []string{"/p/zuddl"}, Personal: []string{"/p/personal"}, Jots: ListWork}
	})
	day := "2026-10-14"
	save(t, c.store, day, "## Work\n\n- 09:00 standup\n\n## Personal\n\n- 08:00 run")

	w.Capture("hotkey note", "hotkey")
	got := c.body(day)
	expect(t, strings.Index(got, " hotkey note") > 0 && strings.Index(got, " hotkey note") < strings.Index(got, "## Personal"),
		"a note without a folder goes to the jots list (Work): %q", got)

	inbox := t.TempDir()
	if err := os.WriteFile(filepath.Join(inbox, "1-a.json"), []byte(`{"text":"gloamlog standup view","cwd":"/p/personal/daily-log"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w.DrainInbox(inbox, InboxUseFileTime)
	got = c.body(day)
	expect(t, strings.Index(got, " gloamlog standup view") > strings.Index(got, "## Personal"), "a note from a personal folder goes to Personal: %q", got)

	other := "2026-10-15"
	w2 := c.writer(newCapLookup(), func() time.Time { return mk(2026, 10, 15, 9, 0) })
	w2.SetLists(func() ListsPrefs { return ListsPrefs{Jots: ListPersonal} })
	w2.Capture("first of the day", "hotkey")
	expect(t, strings.HasPrefix(c.body(other), "## Personal\n\n- "), "a new split day starts with the note's list: %q", c.body(other))

	expect(t, JotsHeadingIn("## Log\n- a", "Jots", ListPersonal) == LogHeading, "a one-list page takes everything")
	expect(t, JotsHeadingIn("## Work\n- a", "Jots", ListPersonal) == ListPersonal, "the missing partner")
	expect(t, JotsHeadingIn("", "Jots", ListPersonal) == ListPersonal, "a new split day")
}

func TestCaptureTimedPlacesTheLineByItsTimeAlwaysStamped(t *testing.T) {
	c := newCapCore(t)
	save(t, c.store, capToday, "## Log\n\n- 09:00 a\n- 15:00 c")
	prefs := DefaultCapturePrefs()
	prefs.Timestamps = false // a commit is stamped anyway: its time is where it goes
	w := NewDayWriter(c.store, NewCaptureJournal(c.journalDir), newCapLookup(), func() CapturePrefs { return prefs },
		func() time.Time { return capNow }, func() Calendar { return ny })
	_, ok := w.CaptureTimed("[] Add CSV export `1a2b3c4`", "git", ListWork, mk(2026, 10, 7, 13, 2))
	expect(t, ok)
	expect(t, c.body(capToday) == "## Log\n\n- 09:00 a\n- 13:02 [] Add CSV export `1a2b3c4`\n- 15:00 c", "by time, never a to-do: %q", c.body(capToday))
}

func TestTimedNoteInAnOpenPageIsKnownByItsOwnLine(t *testing.T) {
	c := newCapCore(t)
	lookup := newCapLookup()
	page := newCapPage(capToday)
	page.kind = JotAppended
	page.reply = "## Log\n\n- 13:02 x `5d6e7f8`\n- 15:00 c" // the editor put it by time: the last line is another one
	lookup.pages[capToday] = page
	w := c.writer(lookup, nil)
	r, _ := w.CaptureTimed("x `5d6e7f8`", "git", ListWork, mk(2026, 10, 7, 13, 2))
	expect(t, r.Waiting == 1 && len(page.received) == 1 && page.received[0].Timed, "handed to the page as a timed note")
	w.AcknowledgeSaved(capToday, "## Log\n\n- 15:00 c")
	expect(t, w.Waiting() == 1, "a save without the commit's line does not acknowledge it")
	w.AcknowledgeSaved(capToday, page.reply)
	expect(t, w.Waiting() == 0, "the save that holds it does")
}
