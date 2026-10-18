# Writing redesign, piece 2: the list page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A list page opens in a line editor (one row per line, an add field, keys for marks, From yesterday) instead of the rich-text editor, with saving, jots and suggestions as safe as today.

**Architecture:** Go stays the only writer of the file: three small `Notes` methods parse a page, write it out (markdown, word count, safe HTML per line) and append jots. The screen holds the lines in a pure TypeScript model (`src/log/model.ts`, tested with Node's own test runner) and shows them in a `LogEditor` component. `DayPage` keeps its save pipeline (debounce, orphans, flush on leave) and only asks the list editor for the markdown instead of Milkdown. "Edit whole page" switches to the existing Milkdown editor.

**Tech Stack:** Go (desktop package, Wails v3 beta.30 bindings), React 19 + TypeScript 5.6, `node --test` (Node 26 runs `.ts` directly), `@personal/ui`.

**Spec:** [docs/superpowers/specs/2026-10-18-writing-redesign-design.md](../specs/2026-10-18-writing-redesign-design.md), sections 2 and 3. Piece 1 (core rules) is done: `internal/core/loglist.go` (`ParseLog`, `LogPage`, `LogLine`, `IsLogPage`, `OpenLines`), `OpenItems`, `CarryCard.Marks`.

**Decided while planning (owner, 2026-10-18):** ⌘1 / ⌘2 / ⌘3 mark a line done / next / blocked while typing in it or in the add field; everywhere else they keep their app meaning (Today / Catch up / Review). The key handler stops the event so the window shortcut doesn't also fire.

**Not in this piece:** folding the App-level sync / conflict banners into one status line (the reminder notice is already one line and stays); the List | Standup switch (piece 3); commits added by themselves (piece 4). The suggestions strip stays on list pages until piece 4 replaces it; it adds into the list.

**Commands** (the sandbox needs this on every Go command):

```bash
export GOCACHE=/tmp/claude-501/gocache GOMODCACHE=/tmp/claude-501/gomodcache GOFLAGS=-mod=mod GOTELEMETRY=off
```

- Go tests: `(cd desktop && go test . -count=1 2>&1 | grep -v "ld: warning\|telemetry\|upload")`
- Bindings, after any change to a `Notes` method signature: `(cd desktop && /tmp/claude-501/gobin/wails3 generate bindings -d frontend/bindings -ts .)`
- TS model tests: `(cd desktop/frontend && npm test)` (added in Task 3)
- Type check: `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)` (never `npx`: it needs the network)
- Browser check: the `gloamlog-mock` preview (loadout's `.claude/launch.json`, port 9246)

---

## File structure

| File | Change | Responsibility |
|---|---|---|
| `desktop/notes_log.go` | create | `LogParse`, `LogRender`, `LogAppendLines`: the list editor's Go side |
| `desktop/notes_log_test.go` | create | Tests for them |
| `desktop/notes_review.go` | modify | Extract `assetImages()` (shared with `LogRender`) |
| `desktop/notes_carry.go` | create | `CarryDismiss`, `carry-dismissed.json`, `offerFor` (filters a list page's card) |
| `desktop/notes_carry_test.go` | create | Tests |
| `desktop/notes.go` | modify | `Open` filters the card through `offerFor` |
| `desktop/frontend/bindings/…` | regenerate | New methods and types |
| `desktop/frontend/src/log/model.ts` | create | The list editor's state, pure functions |
| `desktop/frontend/src/log/model.test.ts` | create | `node --test` tests |
| `desktop/frontend/src/log/LogEditor.tsx` | create | Rows, the add field, keys, image paste |
| `desktop/frontend/src/log/CarryBox.tsx` | create | From yesterday: Done / Keep / Drop |
| `desktop/frontend/src/dev/mock-log.ts` | create | Browser-mock stand-ins for the new Go methods |
| `desktop/frontend/src/dev/mock.ts` | modify | Use them; list-page demo data |
| `desktop/frontend/src/screens/DayPage.tsx` | modify | List or classic; save pipeline; jots; suggestions; Edit whole page; title |
| `desktop/frontend/src/app.css` | modify | `.log*` and `.carry-box` styles, tokens only |
| `desktop/frontend/package.json`, `tsconfig.json` | modify | `npm test`; keep `*.test.ts` out of `tsc` |
| `docs/EDITOR_CONTRACT.md`, `docs/port/STATUS.md`, the spec | modify | Which editor runs when; status |

---

### Task 1: The list editor's Go side

**Files:**
- Create: `desktop/notes_log.go`, `desktop/notes_log_test.go`
- Modify: `desktop/notes_review.go` (the `images` closure in `reviewBuild`, around line 187)

- [ ] **Step 1: Write the failing test** (`desktop/notes_log_test.go`; `captureNotes` is the existing helper in `capture_test.go`)

```go
package main

import (
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

func TestLogParseAndRenderRoundTrip(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	body := "## Log\n\n*  09:40  Fixed it\n- 10:00 next one #next\n"
	p := n.LogParse(body)
	if !p.OK || len(p.Page.Lines) != 2 {
		t.Fatalf("parse: %+v", p)
	}
	r := n.LogRender(p.Page)
	if r.Markdown != body {
		t.Fatalf("an untouched page comes back byte for byte: %q", r.Markdown)
	}
	if r.Words != 4 || len(r.HTML) != 2 || !strings.Contains(r.HTML[0], "Fixed it") {
		t.Fatalf("words %d, html %q", r.Words, r.HTML)
	}
	if n.LogParse("## What I did\n- x").OK {
		t.Fatal("a classic page is not a list page")
	}
}

func TestLogRenderEscapesHTML(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	r := n.LogRender(core.LogPage{Before: "## Log", Lines: []core.LogLine{{Text: "<script>x</script> **bold**", Blank: 1}}})
	if strings.Contains(r.HTML[0], "<script>") || !strings.Contains(r.HTML[0], "<strong>bold</strong>") {
		t.Fatalf("%q", r.HTML[0])
	}
}

func TestLogAppendLinesAddsOnceAtTheEnd(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	p := n.LogParse("## Log").Page
	a := n.LogAppendLines(p, "- 14:32 note from Claude")
	if a.Result != "appended" || a.Rendered.Markdown != "## Log\n\n- 14:32 note from Claude" {
		t.Fatalf("%+v", a)
	}
	p.Lines = a.Lines
	again := n.LogAppendLines(p, "- 14:40 note from Claude")
	if again.Result != "alreadyPresent" || len(again.Lines) != 1 {
		t.Fatalf("the same text twice is added once: %+v", again)
	}
	two := n.LogAppendLines(p, "- one\n- two")
	if len(two.Lines) != 3 || two.Lines[2].Text != "two" || two.Lines[1].Blank != 0 {
		t.Fatalf("%+v", two.Lines)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `(cd desktop && go test . -run 'LogParse|LogRender|LogAppend' -count=1 2>&1 | grep -v "ld: warning" | head -5)`
Expected: FAIL, `n.LogParse undefined`.

- [ ] **Step 3: Share the image resolver.** In `desktop/notes_review.go`, `reviewBuild`, replace

```go
	assets := n.store.Assets()
	images := func(path string) (string, bool) {
		if assets.Resolve(path) == "" {
			return "", false
		}
		parts := strings.Split(path, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		return "/dlasset/" + strings.Join(parts, "/"), true
	}
	b.html = func(md string) string { return core.MDBody(md, images) }
```

with

```go
	images := n.assetImages()
	b.html = func(md string) string { return core.MDBody(md, images) }
```

and add after `reviewBuild`:

```go
// assetImages links an image path on a page to /dlasset/ (the app's asset route) when the file is there. Caller holds n.mu.
func (n *Notes) assetImages() core.MDImages {
	assets := n.store.Assets()
	return func(path string) (string, bool) {
		if assets.Resolve(path) == "" {
			return "", false
		}
		parts := strings.Split(path, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		return "/dlasset/" + strings.Join(parts, "/"), true
	}
}
```

- [ ] **Step 4: Write `desktop/notes_log.go`**

```go
package main

// The list page's editor (frontend/src/log) asks Go for every rule about the file (internal/core/loglist.go): a page is
// parsed and written here, never in TypeScript, so a line on disk is always what core writes. The screen holds the lines
// while they are edited and sends them back to be written out.

import "github.com/dasariprashant0/gloamlog/internal/core"

// LogParsed is a body split around its Log section; OK is false for a classic page.
type LogParsed struct {
	Page core.LogPage `json:"page"`
	OK   bool         `json:"ok"`
}

// LogParse splits body for the list editor.
func (n *Notes) LogParse(body string) LogParsed {
	p, ok := core.ParseLog(body)
	return LogParsed{Page: p, OK: ok}
}

// LogRendered is a list page written out: the markdown to save, its words (what "logged" counts), and each line's text as
// HTML for display (escaped; images through /dlasset/, like the review).
type LogRendered struct {
	Markdown string   `json:"markdown"`
	Words    int      `json:"words"`
	HTML     []string `json:"html"`
}

// LogRender writes page out.
func (n *Notes) LogRender(page core.LogPage) LogRendered {
	n.mu.Lock()
	images := n.assetImages()
	n.mu.Unlock()
	md := page.Markdown()
	out := LogRendered{Markdown: md, Words: core.DayDocument{Body: md}.Words(), HTML: make([]string, len(page.Lines))}
	for i, l := range page.Lines {
		out.HTML[i] = core.MDBody(l.Text, images)
	}
	return out
}

// LogAppended is the page after LogAppendLines: its lines, the page written out, and "appended", or "alreadyPresent" when
// every line was there already (the same words the Milkdown editor answers with).
type LogAppended struct {
	Lines    []core.LogLine `json:"lines"`
	Rendered LogRendered    `json:"rendered"`
	Result   string         `json:"result"`
}

// LogAppendLines adds the bullets of markdown (a jot as capture renders it, or the suggestions strip's lines) at the end
// of the Log, skipping any whose text is already a line. Used while the list editor has the page open.
func (n *Notes) LogAppendLines(page core.LogPage, markdown string) LogAppended {
	add, _ := core.ParseLog("## " + core.LogHeading + "\n" + markdown)
	have := map[string]bool{}
	for _, l := range page.Lines {
		have[core.ItemKey(l.Text)] = true
	}
	lines, added := append([]core.LogLine{}, page.Lines...), 0
	for _, l := range add.Lines {
		key := core.ItemKey(l.Text)
		if l.Other || key == "" || have[key] {
			continue
		}
		l.Blank = 0
		if len(lines) == 0 {
			l.Blank = 1 // a blank line between the heading and the first line
		}
		lines = append(lines, l)
		have[key] = true
		added++
	}
	page.Lines = lines
	res := LogAppended{Lines: lines, Rendered: n.LogRender(page), Result: "appended"}
	if added == 0 {
		res.Result = "alreadyPresent"
	}
	return res
}
```

- [ ] **Step 5: Run the tests**

Run: `(cd desktop && go vet . 2>&1 | grep -v "ld: warning"; go test . -count=1 2>&1 | grep -v "ld: warning\|telemetry\|upload" | tail -3)`
Expected: `ok`. If `TestLogRenderEscapesHTML` fails on `<strong>`, print `core.MDBody("**bold**", nil)` and match the tag it uses (do not change MDBody).

- [ ] **Step 6: Regenerate the bindings and type-check**

Run the bindings command, then tsc. Expected: `notes.ts` gains `LogParse`, `LogRender`, `LogAppendLines`; `desktop/models.ts` gains `LogParsed`, `LogRendered`, `LogAppended`; `internal/core/models.ts` gains `LogPage`, `LogLine`, `LogMark`; `tsc ok`.

- [ ] **Step 7: Commit**

```bash
git add desktop/notes_log.go desktop/notes_log_test.go desktop/notes_review.go desktop/frontend/bindings
git commit -m "desktop: Go side of the list editor (parse, render, append jots)"
```

---

### Task 2: From yesterday on a list page — dismissals and filtering

**Files:**
- Create: `desktop/notes_carry.go`, `desktop/notes_carry_test.go`
- Modify: `desktop/notes.go` (`Open`, the carry block near line 254)

- [ ] **Step 1: Write the failing test** (`desktop/notes_carry_test.go`)

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

func TestCarryOfferOnAListPageDropsDismissedAndPresentLines(t *testing.T) {
	n, _, _, _ := captureNotes(t, true) // today is 2026-10-14
	n.mu.Lock()
	n.settings.MinWords = 1
	n.mu.Unlock()
	save := func(day, body string) {
		if _, err := n.store.Save(day, body); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-10-13", "## Log\n\n- 09:00 shipped it\n- 10:00 write the test #next\n- 11:00 keys from ops #blocked\n- 12:00 call Sam #next")
	save("2026-10-14", "## Log\n\n- 09:00 call Sam")
	p, err := n.Open("2026-10-14")
	if err != nil || p.Carry == nil || strings.Join(p.Carry.Items, "|") != "write the test|keys from ops" {
		t.Fatalf("a line already on today's page is not offered: %+v %v", p.Carry, err)
	}
	if err := n.CarryDismiss("2026-10-14", "keys from ops"); err != nil {
		t.Fatal(err)
	}
	p, _ = n.Open("2026-10-14")
	if p.Carry == nil || len(p.Carry.Items) != 1 || p.Carry.Items[0] != "write the test" || p.Carry.Marks[0] != core.MarkNext {
		t.Fatalf("a dropped line is not offered again: %+v", p.Carry)
	}
	if err := n.CarryDismiss("2026-10-14", "write the test"); err != nil {
		t.Fatal(err)
	}
	if p, _ = n.Open("2026-10-14"); p.Carry != nil {
		t.Fatalf("nothing left, no card: %+v", p.Carry)
	}
}

func TestCarryDismissedFileIsPrunedAndSurvivesDamage(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	path := n.carryDismissedPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"2026-09-01":["old"],"2026-10-13":["kept"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := n.CarryDismiss("2026-10-14", "x"); err != nil {
		t.Fatal(err)
	}
	d := n.loadCarryDismissed()
	if _, old := d["2026-09-01"]; old || len(d["2026-10-13"]) != 1 || len(d["2026-10-14"]) != 1 {
		t.Fatalf("older than 30 days goes, the rest stays: %v", d)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(n.loadCarryDismissed()) != 0 {
		t.Fatal("a damaged file offers everything again")
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `(cd desktop && go test . -run 'Carry' -count=1 2>&1 | grep -v "ld: warning" | head -5)`
Expected: FAIL, `n.CarryDismiss undefined`.

- [ ] **Step 3: Write `desktop/notes_carry.go`**

```go
package main

// "From yesterday" on a list page: Keep and Done add the line to the page (the screen does that, like any edit); Drop is
// remembered here so the line is not offered again that day. The file holds item keys (core.ItemKey), never whole pages,
// and forgets days older than 30. A damaged file means everything is offered again: nothing on any page is affected.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

const carryDismissedFile = "carry-dismissed.json"

// carryDismissed is day -> the keys dropped from that day's offer.
type carryDismissed map[string][]string

func (n *Notes) carryDismissedPath() string {
	return filepath.Join(filepath.Dir(n.settingsPath), carryDismissedFile)
}

func (n *Notes) loadCarryDismissed() carryDismissed {
	data, err := os.ReadFile(n.carryDismissedPath())
	if err != nil {
		return carryDismissed{} // none dropped yet, or unreadable: everything is offered
	}
	var d carryDismissed
	if json.Unmarshal(data, &d) != nil || d == nil {
		return carryDismissed{}
	}
	return d
}

// CarryDismiss drops one item from day's "From yesterday" offer.
func (n *Notes) CarryDismiss(day, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	d := n.loadCarryDismissed()
	key := core.ItemKey(text)
	for _, k := range d[day] {
		if k == key {
			return nil
		}
	}
	d[day] = append(d[day], key)
	cutoff := core.DayKeyString(n.now().AddDate(0, 0, -30), time.Local)
	for k := range d {
		if k < cutoff {
			delete(d, k)
		}
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	path := n.carryDismissedPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return core.AtomicWrite(append(data, '\n'), path, "")
}

// offerFor takes out of a list page's card what was dropped today or is already a line on body. A classic card (no marks)
// comes back as it is. nil when nothing is left.
func (n *Notes) offerFor(day, body string, c *core.CarryCard) *core.CarryCard {
	if c == nil || c.Marks == nil {
		return c
	}
	gone := map[string]bool{}
	for _, k := range n.loadCarryDismissed()[day] {
		gone[k] = true
	}
	if p, ok := core.ParseLog(body); ok {
		for _, l := range p.Lines {
			gone[core.ItemKey(l.Text)] = true
		}
	}
	out := *c
	out.Items, out.Marks = nil, nil
	for i, it := range c.Items {
		if !gone[core.ItemKey(it)] {
			out.Items, out.Marks = append(out.Items, it), append(out.Marks, c.Marks[i])
		}
	}
	if len(out.Items) == 0 {
		return nil
	}
	return &out
}
```

Check `core.AtomicWrite`'s signature (`grep -n "func AtomicWrite" internal/core/*.go`) and match it.

- [ ] **Step 4: Use it in `Open`** (`desktop/notes.go`): replace

```go
		if c := core.CarryCardBefore(day, cal, s, load); c != nil {
			p.Carry, p.CarryApplied = c, core.IsCarryApplied(*c, p.Body)
		}
```

with

```go
		if c := n.offerFor(day, p.Body, core.CarryCardBefore(day, cal, s, load)); c != nil {
			p.Carry, p.CarryApplied = c, core.IsCarryApplied(*c, p.Body)
		}
```

- [ ] **Step 5: Run the tests**, then **regenerate the bindings and type-check** (as in Task 1). Expected: all `ok`; `notes.ts` gains `CarryDismiss`; `tsc ok`.

- [ ] **Step 6: Commit**

```bash
git add desktop/notes_carry.go desktop/notes_carry_test.go desktop/notes.go desktop/frontend/bindings
git commit -m "desktop: From yesterday on a list page leaves out dropped and present lines"
```

---

### Task 3: The list editor's model (pure TypeScript, tested)

**Files:**
- Create: `desktop/frontend/src/log/model.ts`, `desktop/frontend/src/log/model.test.ts`
- Modify: `desktop/frontend/package.json` (scripts), `desktop/frontend/tsconfig.json` (exclude tests)

- [ ] **Step 1: Make tests runnable.** In `package.json` `scripts`, add `"test": "node --test src/log/model.test.ts"`. In `tsconfig.json`, after `"include"`, add `"exclude": ["src/**/*.test.ts"]` (the test imports `node:test`, whose types aren't installed; Node strips the types when it runs the file).

- [ ] **Step 2: Write the failing test** (`src/log/model.test.ts`)

```ts
import {test} from 'node:test';
import assert from 'node:assert/strict';
import * as M from './model.ts';

const L = (text: string, more: Partial<M.Line> = {}): M.Line => ({time: '', text, mark: '', task: false, other: false, blank: 0, raw: `- ${text}`, ...more});
const texts = (s: M.State) => s.lines.map(l => l.text);

test('cutMark takes only a trailing #next / #blocked, any case', () => {
    assert.deepEqual(M.cutMark('write the test #NEXT'), ['write the test', 'next']);
    assert.deepEqual(M.cutMark('#blocked'), ['', 'blocked']);
    assert.deepEqual(M.cutMark('tagged #next in the middle'), ['tagged #next in the middle', null]);
});

test('clock is HH:MM', () => {
    assert.equal(M.clock(new Date(2026, 9, 18, 9, 5)), '09:05');
});

test('commit writes the draft, cuts a mark, and keeps no history when nothing changed', () => {
    let s = M.focus(M.fresh([L('a')]), 0);
    assert.equal(M.commit(s).past.length, 0);
    s = M.setDraft(s, 'a better #next');
    const c = M.commit(s);
    assert.deepEqual([c.lines[0].text, c.lines[0].mark, c.edit, c.past.length], ['a better', 'next', -1, 1]);
    const plain = M.commit(M.setDraft(M.focus(c, 0), 'a better'));
    assert.equal(plain.lines[0].mark, '', 'deleting the word makes it done');
});

test('a task line keeps its box mark unless the draft says otherwise', () => {
    const task = L('call Sam', {task: true, mark: 'next'});
    let s = M.focus(M.fresh([task]), 0);
    assert.equal(s.draft, 'call Sam');
    assert.equal(M.commit(M.setDraft(s, 'call Sam today')).lines[0].mark, 'next');
    s = M.setMark(s, 0, '');
    assert.equal(M.commit(s).lines[0].mark, '', '⌘1 ticks it');
});

test('Enter starts a timed line after the current one; Enter on an empty new line does nothing', () => {
    let s = M.newLineAfter(M.focus(M.fresh([L('a'), L('b')]), 0), 0, '10:30');
    assert.deepEqual([texts(s), s.edit, s.lines[1].time], [['a', '', 'b'], 1, '10:30']);
    assert.equal(M.newLineAfter(s, 1, '10:31'), s);
    s = M.focus(s, 2);
    assert.deepEqual(texts(s), ['a', 'b'], 'leaving an empty new line removes it');
    assert.equal(s.edit, 1, 'and the target index follows');
});

test('the first line of an empty list gets a blank line after the heading', () => {
    const s = M.append(M.fresh([]), 'first', '09:00');
    assert.equal(s.lines[0].blank, 1);
    assert.equal(M.append(s, 'second', '09:01').lines[1].blank, 0);
});

test('append reads a trailing mark from the text, or takes one', () => {
    assert.equal(M.append(M.fresh([]), 'x #blocked', '09:00').lines[0].mark, 'blocked');
    assert.equal(M.append(M.fresh([]), 'x', '09:00', 'next').lines[0].mark, 'next');
    assert.equal(M.append(M.fresh([]), '   ', '09:00').lines.length, 0);
});

test('setMark while editing changes the draft too; on another line just the mark', () => {
    let s = M.setMark(M.focus(M.fresh([L('a'), L('b')]), 0), 0, 'blocked');
    assert.deepEqual([s.lines[0].mark, s.draft, s.edit], ['blocked', 'a #blocked', 0]);
    s = M.setMark(s, 1, 'next');
    assert.equal(s.lines[1].mark, 'next');
});

test('move skips Other blocks; removeLine edits the line above and keeps the gap after the heading', () => {
    const other = L('A paragraph', {other: true, raw: 'A paragraph'});
    let s = M.focus(M.fresh([L('a', {blank: 1}), other, L('b')]), 0);
    s = M.move(s, 1);
    assert.equal(s.edit, 2);
    assert.equal(M.focus(s, 1).edit, -1, 'an Other block is not edited here');
    const r = M.removeLine(M.focus(M.fresh([L('a', {blank: 1}), L('b')]), 0), 0);
    assert.deepEqual([texts(r), r.lines[0].blank, r.edit], [['b'], 1, -1]);
});

test('undo and redo work across lines, and undo while typing takes the typing back', () => {
    let s = M.append(M.append(M.fresh([]), 'one', '09:00'), 'two', '09:01');
    s = M.undo(s);
    assert.deepEqual(texts(s), ['one']);
    s = M.redo(s);
    assert.deepEqual(texts(s), ['one', 'two']);
    s = M.undo(M.setDraft(M.focus(s, 0), 'changed'));
    assert.deepEqual(texts(s), ['one', 'two']);
});

test('setTime moves a line before the first later time; untimed lines are passed over; bad times are ignored', () => {
    const s = M.fresh([L('a', {time: '09:00', blank: 1}), L('untimed'), L('c', {time: '11:00'})]);
    const t = M.setTime(s, 2, '08:30');
    assert.deepEqual([texts(t), t.lines[0].blank, t.lines[1].blank], [['c', 'a', 'untimed'], 1, 0]);
    assert.equal(M.setTime(s, 0, '25:00'), s);
    const u = M.setTime(s, 0, '12:00');
    assert.deepEqual([texts(u), u.lines[0].blank, u.lines[2].blank], [['untimed', 'c', 'a'], 1, 0]);
});

test('withDraft drops an empty new line; renderIndex follows it', () => {
    const s = M.newLineAfter(M.focus(M.fresh([L('a'), L('b')]), 0), 0, '10:00');
    assert.deepEqual(M.withDraft(s).map(l => l.text), ['a', 'b']);
    assert.deepEqual([0, 1, 2].map(i => M.renderIndex(s, i)), [0, -1, 1]);
    assert.equal(M.renderIndex(M.fresh([L('a')]), 0), 0);
});

test('adopt takes lines from Go (an appended jot) as one undoable step', () => {
    const s = M.adopt(M.focus(M.fresh([L('a')]), 0), [L('a'), L('jot')]);
    assert.deepEqual([texts(s), s.edit], [['a', 'jot'], -1]);
    assert.deepEqual(texts(M.undo(s)), ['a']);
});

test('summary', () => {
    assert.equal(M.summary([]), 'Nothing yet');
    assert.equal(M.summary([L('a'), L('b', {mark: 'next'}), L('c', {mark: 'blocked'}), L('p', {other: true})]), '3 lines, 1 next, 1 blocked');
    assert.equal(M.summary([L('a')]), '1 line');
});
```

- [ ] **Step 3: Run it to see it fail**

Run: `(cd desktop/frontend && npm test 2>&1 | tail -5)`
Expected: FAIL, `Cannot find module '.../model.ts'`.

- [ ] **Step 4: Write `src/log/model.ts`**

```ts
// The list editor's state as pure functions; `npm test` runs model.test.ts with Node's own runner (no dependencies, so only
// erasable TypeScript here: no enums, no namespaces). The lines come from Go (Notes.LogParse) and go back to it to be written
// (Notes.LogRender): nothing here writes markdown. A line's mark is edited as a trailing "#next" / "#blocked" in its draft,
// so typing or deleting the word works as well as ⌘1-3.

export type Mark = '' | 'next' | 'blocked';
/** One line, as Go's core.LogLine sends it. */
export type Line = {time: string; text: string; mark: Mark; task: boolean; other: boolean; blank: number; raw: string};

export type State = {
    lines: Line[];
    edit: number;      // the line being edited; -1 for none
    draft: string;     // that line's text while it is edited
    past: Line[][];    // undo
    future: Line[][];  // redo
};

const HISTORY = 200;

export const fresh = (lines: Line[]): State => ({lines, edit: -1, draft: '', past: [], future: []});

export const newLine = (time: string, text = '', mark: Mark = ''): Line =>
    ({time, text, mark, task: false, other: false, blank: 0, raw: ''});

/** "09:40": the time a new line gets. */
export function clock(d: Date): string {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

/** A trailing #next / #blocked (the last word, any case) taken off the text. */
export function cutMark(s: string): [string, Mark | null] {
    const m = /(^|\s)#(next|blocked)\s*$/i.exec(s);
    return m ? [s.slice(0, m.index).trimEnd(), m[2].toLowerCase() as Mark] : [s, null];
}

/** The text a line is edited as. A task box already says "next", so there only "blocked" shows as a word. */
export function draftOf(l: Line): string {
    if (!l.mark || (l.task && l.mark === 'next')) return l.text;
    return l.text ? `${l.text} #${l.mark}` : `#${l.mark}`;
}

function minutes(t: string): number | null {
    const m = /^(\d{1,2}):(\d{2})$/.exec(t);
    if (!m || Number(m[1]) > 23 || Number(m[2]) > 59) return null;
    return Number(m[1]) * 60 + Number(m[2]);
}

function record(s: State, lines: Line[]): State {
    return {...s, lines, past: [...s.past, s.lines].slice(-HISTORY), future: []};
}

// The first line keeps the blank lines after the heading.
function withHeadGap(lines: Line[], gap: number): Line[] {
    return lines.map((l, i) => (i === 0 && l.blank !== gap ? {...l, blank: gap} : l));
}

const isEmptyNew = (l: Line | undefined) => !!l && l.raw === '' && l.text === '' && !l.mark && !l.other;

/** Write the draft back into its line. A new line left empty goes away. */
export function commit(s: State): State {
    if (s.edit < 0 || s.edit >= s.lines.length) return s.edit === -1 && s.draft === '' ? s : {...s, edit: -1, draft: ''};
    const l = s.lines[s.edit];
    const [cut, mark] = cutMark(s.draft);
    const text = cut.replace(/\s+$/, '');
    const done: State = {...s, edit: -1, draft: ''};
    if (text === '' && mark === null && isEmptyNew(l)) return {...done, lines: s.lines.filter((_, i) => i !== s.edit)};
    const next: Line = {...l, text, mark: mark ?? (l.task ? l.mark : '')};
    if (next.text === l.text && next.mark === l.mark) return done;
    const lines = s.lines.slice();
    lines[s.edit] = next;
    return record(done, lines);
}

// commit, and where index i is afterwards (an empty new line before it may have gone).
function afterCommit(s: State, i: number): [State, number] {
    const c = commit(s);
    return [c, c.lines.length < s.lines.length && i > s.edit ? i - 1 : i];
}

export const setDraft = (s: State, draft: string): State => ({...s, draft});

/** The lines with the draft applied: what a render or a save should see. */
export const withDraft = (s: State): Line[] => commit(s).lines;

/** Where row i of s.lines is in withDraft(s); -1 when row i is the empty new line being edited. */
export function renderIndex(s: State, i: number): number {
    if (withDraft(s).length === s.lines.length) return i;
    if (i === s.edit) return -1;
    return i > s.edit ? i - 1 : i;
}

/** Edit line i (the line being edited is written first). Other blocks are edited in the full editor, not here. */
export function focus(s: State, i: number): State {
    const [c, j] = afterCommit(s, i);
    if (j < 0 || j >= c.lines.length || c.lines[j].other) return c;
    return {...c, edit: j, draft: draftOf(c.lines[j])};
}

/** ↑ / ↓: the next line that can be edited here. */
export function move(s: State, dir: -1 | 1): State {
    let i = s.edit + dir;
    while (i >= 0 && i < s.lines.length && s.lines[i].other) i += dir;
    return i < 0 || i >= s.lines.length ? s : focus(s, i);
}

/** Enter on line i: it is written and a new line starts after it, at time now. */
export function newLineAfter(s: State, i: number, now: string): State {
    if (s.edit === i && s.draft.trim() === '' && isEmptyNew(s.lines[i])) return s;
    const [c, j] = afterCommit(s, i);
    const lines = c.lines.slice();
    const at = Math.min(j + 1, lines.length);
    lines.splice(at, 0, {...newLine(now), blank: lines.length === 0 ? 1 : 0});
    return {...record(c, lines), edit: at, draft: ''};
}

/** ⌘1 / ⌘2 / ⌘3, or the mark chip: done, next or blocked. While line i is edited its draft shows the new mark. */
export function setMark(s: State, i: number, mark: Mark): State {
    const l = s.lines[i];
    if (!l || l.other) return s;
    const lines = s.lines.slice();
    lines[i] = {...l, mark};
    if (s.edit === i) return {...record(s, lines), draft: draftOf({...lines[i], text: cutMark(s.draft)[0]})};
    return l.mark === mark ? s : record(s, lines);
}

/** Backspace on an empty line: it goes, and the line above is edited. */
export function removeLine(s: State, i: number): State {
    if (i < 0 || i >= s.lines.length) return s;
    const lines = withHeadGap(s.lines.filter((_, k) => k !== i), s.lines[0].blank);
    const out: State = {...record(s, lines), edit: -1, draft: ''};
    const prev = i - 1;
    return prev >= 0 && !lines[prev].other ? {...out, edit: prev, draft: draftOf(lines[prev])} : out;
}

export function undo(s: State): State {
    const c = commit(s);
    if (!c.past.length) return c;
    return {...c, lines: c.past[c.past.length - 1], past: c.past.slice(0, -1), future: [c.lines, ...c.future].slice(0, HISTORY)};
}

export function redo(s: State): State {
    const c = commit(s);
    if (!c.future.length) return c;
    return {...c, lines: c.future[0], future: c.future.slice(1), past: [...c.past, c.lines].slice(-HISTORY)};
}

/** The add field, or From yesterday: a new line at the end at time now. A trailing #next / #blocked in text marks it. */
export function append(s: State, text: string, now: string, mark: Mark = ''): State {
    const c = commit(s);
    const [t, m] = cutMark(text.trim());
    if (!t) return c;
    return record(c, [...c.lines, {...newLine(now, t, m ?? mark), blank: c.lines.length === 0 ? 1 : 0}]);
}

/** A new time for line i: the line moves before the first line with a later time; untimed lines are passed over. */
export function setTime(s: State, i: number, time: string): State {
    const k = minutes(time);
    const [c, j] = afterCommit(s, i);
    const l = c.lines[j];
    if (!l || l.other || k === null || l.time === time) return c;
    const gap = c.lines[0].blank;
    const oldFirst = c.lines[0];
    const rest = c.lines.filter((_, x) => x !== j);
    const later = rest.findIndex(x => { const xk = minutes(x.time); return xk !== null && xk > k; });
    rest.splice(later < 0 ? rest.length : later, 0, {...l, time, blank: 0});
    // Only the first line carries the blank lines after the heading.
    return record(c, rest.map((x, idx) => (idx === 0 ? {...x, blank: gap} : x === oldFirst ? {...x, blank: 0} : x)));
}

/** Lines Go wrote (an appended jot or suggestion): one undoable step; editing ends. */
export function adopt(s: State, lines: Line[]): State {
    return record(commit(s), lines);
}

/** "5 lines, 1 next, 1 blocked" for the page title. */
export function summary(lines: Line[]): string {
    const real = lines.filter(l => !l.other && l.text.trim() !== '');
    if (!real.length) return 'Nothing yet';
    const next = real.filter(l => l.mark === 'next').length;
    const blocked = real.filter(l => l.mark === 'blocked').length;
    const parts = [`${real.length} ${real.length === 1 ? 'line' : 'lines'}`];
    if (next) parts.push(`${next} next`);
    if (blocked) parts.push(`${blocked} blocked`);
    return parts.join(', ');
}
```

- [ ] **Step 5: Run the tests and the type check**

Run: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (tests|pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: `fail 0`, `tsc ok`. A failing case means a bug in the model, not the test, unless the test contradicts the spec: then stop and report.

- [ ] **Step 6: Commit**

```bash
git add desktop/frontend/src/log desktop/frontend/package.json desktop/frontend/tsconfig.json
git commit -m "frontend: the list editor's model, with node --test tests"
```

---

### Task 4: Browser-mock stand-ins

**Files:**
- Create: `desktop/frontend/src/dev/mock-log.ts`
- Modify: `desktop/frontend/src/dev/mock.ts`

- [ ] **Step 1: Write `src/dev/mock-log.ts`**

```ts
// Stand-ins for the list editor's Go methods (desktop/notes_log.go, notes_carry.go) in `npm run dev:mock`. A rough copy,
// good enough to drive the screen in a browser: the real rules for the file live in internal/core/loglist.go.
import type {Line, Mark} from '../log/model';

export type MockPage = {before: string; lines: Line[]; trail: number; after: string};
const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 30));
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
export const dismissed = new Set<string>(); // "day|text"

export function parseLog(body: string): {page: MockPage; ok: boolean} {
    const rows = body.split('\n');
    const h = rows.findIndex(r => /^#{1,6}\s+log\s*$/i.test(r.trim()));
    if (h < 0) return {page: {before: '', lines: [], trail: 0, after: ''}, ok: false};
    let end = rows.findIndex((r, i) => i > h && /^#{1,6}\s/.test(r));
    if (end < 0) end = rows.length;
    const lines: Line[] = [];
    let blank = 0;
    for (const r of rows.slice(h + 1, end)) {
        if (!r.trim()) { blank++; continue; }
        const m = /^[-*+]\s+(\[( |x|X)\]\s+)?((\d{1,2}:\d{2})\s+)?(.*)$/.exec(r);
        if (!m) { lines.push({time: '', text: r, mark: '', task: false, other: true, blank, raw: r}); blank = 0; continue; }
        let text = m[5];
        let mark: Mark = m[2] === ' ' ? 'next' : '';
        const cut = /(^|\s)#(next|blocked)\s*$/i.exec(text);
        if (cut) { mark = cut[2].toLowerCase() as Mark; text = text.slice(0, cut.index).trimEnd(); }
        lines.push({time: m[4] ?? '', text, mark, task: !!m[1], other: false, blank, raw: r});
        blank = 0;
    }
    return {page: {before: rows.slice(0, h + 1).join('\n'), lines, trail: blank, after: rows.slice(end).join('\n')}, ok: true};
}

function lineMd(l: Line): string {
    if (l.other) return l.text;
    const box = l.task ? (l.mark ? '[ ] ' : '[x] ') : '';
    const mark = l.mark === 'blocked' || (l.mark === 'next' && !l.task) ? ` #${l.mark}` : '';
    return `- ${box}${l.time ? l.time + ' ' : ''}${l.text}${mark}`.trimEnd();
}

function render(page: MockPage) {
    const out = [page.before];
    for (const l of page.lines) { for (let i = 0; i < l.blank; i++) out.push(''); out.push(lineMd(l)); }
    for (let i = 0; i < page.trail; i++) out.push('');
    if (page.after) out.push(page.after);
    const words = page.lines.filter(l => !l.other).map(l => l.text).join(' ').split(/\s+/).filter(Boolean).length;
    return {markdown: out.join('\n'), words, html: page.lines.map(l => `<p>${esc(l.text)}</p>`)};
}

export const logMock = {
    LogParse: (body: string) => later(parseLog(body)),
    LogRender: (page: MockPage) => later(render(page)),
    LogAppendLines: (page: MockPage, markdown: string) => {
        const have = new Set(page.lines.map(l => l.text.toLowerCase()));
        const fresh = parseLog('## Log\n' + markdown).page.lines.filter(l => !l.other && !have.has(l.text.toLowerCase()))
            .map((l, i) => ({...l, blank: page.lines.length === 0 && i === 0 ? 1 : 0}));
        const lines = [...page.lines, ...fresh];
        return later({lines, rendered: render({...page, lines}), result: fresh.length ? 'appended' : 'alreadyPresent'});
    },
    CarryDismiss: (day: string, text: string) => { dismissed.add(`${day}|${text.toLowerCase()}`); return later(undefined); },
};
```

- [ ] **Step 2: Wire it into `src/dev/mock.ts`**

1. Import: `import {logMock, parseLog, dismissed} from './mock-log';`
2. In `settings`, the template becomes `'## Log'`.
3. In the history generator (the IIFE that fills `days`), make the three most recent days list pages: put this right after the `const done = lines[i % lines.length];` line (it uses `done`):

```ts
        if (i <= 3) {
            days.set(d, {body: `## Log\n\n- 09:40 ${done}\n- 11:15 ${lines[(i + 2) % lines.length]}\n- 15:30 Waiting on the API keys from ops #blocked\n- 17:45 Write the migration test #next`});
            continue;
        }
```

4. In `Open`, inside the `if (prev && !prev.skipped)` block, try a list source first:

```ts
                    const list = parseLog(prev.body);
                    if (list.ok) {
                        const open = list.page.lines.filter(l => !l.other && l.mark && !dismissed.has(`${day}|${l.text.toLowerCase()}`));
                        if (open.length) carry = {sourceDay: d, sourceLabel: parse(d).toLocaleDateString('en-GB', {weekday: 'short', day: 'numeric', month: 'short'}),
                            items: open.map(l => l.text), marks: open.map(l => l.mark)} as unknown as CarryCard;
                        break;
                    }
```

5. Spread `...logMock,` into `Notes` next to the other mocks.

- [ ] **Step 3: Type-check**: `tsc ok`.

- [ ] **Step 4: Commit**

```bash
git add desktop/frontend/src/dev
git commit -m "frontend: browser-mock stand-ins for the list editor"
```

---

### Task 5: The `LogEditor` component

**Files:**
- Create: `desktop/frontend/src/log/LogEditor.tsx`
- Modify: `desktop/frontend/src/app.css` (append)

- [ ] **Step 1: Write `src/log/LogEditor.tsx`**

```tsx
// The list page's editor: a row per line and the add field. The state is ./model.ts (pure, tested); every change is written
// out by Go (Notes.LogRender), which also gives each line's HTML. DayPage owns saving: it hears the markdown through onChange
// and asks for the latest with markdown() before it flushes.
import {forwardRef, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState} from 'react';
import type {KeyboardEvent} from 'react';
import {Kbd} from '@personal/ui/react';
import {Browser} from '@wailsio/runtime';
import {Notes} from '../api';
import type {LogPage} from '../../bindings/github.com/dasariprashant0/gloamlog/internal/core';
import * as M from './model';

export type LogEditorHandle = {
    /** The page as markdown, the line being edited included. */
    markdown(): Promise<string>;
    /** Adds the bullets in markdown at the end (a jot, the suggestions strip). */
    appendLines(markdown: string): Promise<{result: string; markdown: string}>;
    /** From yesterday's Keep / Done: a line at the end, at the current time. */
    addLine(text: string, mark: M.Mark): void;
};

type Props = {
    day: string; page: LogPage; readOnly: boolean; autoFocus: boolean;
    onChange: (markdown: string, words: number) => void;
    onSummary: (summary: string) => void;
    onToast: (message: string) => void;
};

const RENDER_DELAY = 250;
const MARKS: M.Mark[] = ['', 'next', 'blocked'];
const word = (m: M.Mark) => m || 'done';
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const now = () => M.clock(new Date());
const nextMark = (m: M.Mark): M.Mark => MARKS[(MARKS.indexOf(m) + 1) % MARKS.length];
// ⌘1 / ⌘2 / ⌘3 while typing mark the line; the event is stopped so the window shortcut (Today / Catch up / Review) doesn't fire.
const markKey = (e: KeyboardEvent): M.Mark | null =>
    (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && ['1', '2', '3'].includes(e.key) ? MARKS[Number(e.key) - 1] : null;

export const LogEditor = forwardRef<LogEditorHandle, Props>(function LogEditor({day, page, readOnly, autoFocus, onChange, onSummary, onToast}, ref) {
    const [state, setState] = useState(() => M.fresh(page.lines as unknown as M.Line[]));
    const [html, setHtml] = useState<string[]>([]);
    const stateRef = useRef(state);
    stateRef.current = state;
    const dirty = useRef(false); // the person changed something: only then does DayPage hear about it (opening never writes)
    const seq = useRef(0);
    const timer = useRef(0);

    const act = useCallback((f: (s: M.State) => M.State) => { dirty.current = true; setState(f); }, []);

    const render = useCallback((lines: M.Line[]) => {
        const n = ++seq.current;
        return Notes.LogRender({...page, lines} as unknown as LogPage).then(r => {
            if (n === seq.current) setHtml(r.html ?? []);
            return {markdown: r.markdown, words: r.words};
        });
    }, [page]);

    // Every change: a render after a short pause; DayPage hears the markdown only after a real edit (it saves 600 ms later).
    useEffect(() => {
        clearTimeout(timer.current);
        timer.current = window.setTimeout(() => {
            render(M.withDraft(stateRef.current)).then(r => { if (dirty.current && !readOnly) onChange(r.markdown, r.words); },
                e => onToast(`Couldn't show this page: ${String(e?.message ?? e)}`));
        }, RENDER_DELAY);
        return () => clearTimeout(timer.current);
    }, [state.lines, state.draft]); // eslint-disable-line react-hooks/exhaustive-deps

    useEffect(() => { onSummary(M.summary(state.lines)); }, [state.lines]); // eslint-disable-line react-hooks/exhaustive-deps

    useImperativeHandle(ref, () => ({
        async markdown() {
            clearTimeout(timer.current);
            return (await render(M.withDraft(stateRef.current))).markdown;
        },
        async appendLines(md) {
            const a = await Notes.LogAppendLines({...page, lines: M.withDraft(stateRef.current)} as unknown as LogPage, md);
            if (a.result === 'appended') { dirty.current = true; setState(s => M.adopt(s, a.lines as unknown as M.Line[])); }
            return {result: a.result, markdown: a.rendered.markdown};
        },
        addLine(text, mark) { act(s => M.append(s, text, now(), mark)); },
    }), [page, render, act]);

    function onKey(e: KeyboardEvent<HTMLTextAreaElement>, i: number) {
        const cmd = e.metaKey || e.ctrlKey;
        const ta = e.currentTarget;
        const mark = markKey(e);
        if (mark !== null) { e.preventDefault(); e.stopPropagation(); act(s => M.setMark(s, i, mark)); }
        else if (e.key === 'Enter' && !e.shiftKey && !cmd && !e.nativeEvent.isComposing) { e.preventDefault(); act(s => M.newLineAfter(s, i, now())); }
        else if (cmd && !e.altKey && e.key.toLowerCase() === 'z') { e.preventDefault(); e.stopPropagation(); act(e.shiftKey ? M.redo : M.undo); }
        else if (e.key === 'Backspace' && ta.value === '') { e.preventDefault(); act(s => M.removeLine(s, i)); }
        else if (e.key === 'ArrowUp' && !ta.value.slice(0, ta.selectionStart).includes('\n')) { e.preventDefault(); act(s => M.move(s, -1)); }
        else if (e.key === 'ArrowDown' && !ta.value.slice(ta.selectionEnd).includes('\n')) { e.preventDefault(); act(s => M.move(s, 1)); }
        else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); act(M.commit); }
    }

    return (
        <div className="log">
            <ul className="log-list" aria-label="Lines">
                {state.lines.map((l, i) => (
                    <LogRow key={i} line={l} editing={state.edit === i} draft={state.draft} html={html[M.renderIndex(state, i)] ?? ''}
                            readOnly={readOnly} onEdit={() => act(s => M.focus(s, i))} onDraft={d => act(s => M.setDraft(s, d))}
                            onKey={e => onKey(e, i)} onBlur={() => act(M.commit)} onCycle={() => act(s => M.setMark(s, i, nextMark(l.mark)))}
                            onTime={t => act(s => M.setTime(s, i, t))}/>
                ))}
            </ul>
            {!readOnly && <AddField autoFocus={autoFocus} onAdd={(text, mark) => act(s => M.append(s, text, now(), mark))}/>}
        </div>
    );
});

function LogRow({line, editing, draft, html, readOnly, onEdit, onDraft, onKey, onBlur, onCycle, onTime}: {
    line: M.Line; editing: boolean; draft: string; html: string; readOnly: boolean;
    onEdit: () => void; onDraft: (d: string) => void; onKey: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
    onBlur: () => void; onCycle: () => void; onTime: (t: string) => void;
}) {
    const ta = useRef<HTMLTextAreaElement>(null);
    const [timing, setTiming] = useState(false);
    const cancelTime = useRef(false);
    useLayoutEffect(() => {
        const el = ta.current;
        if (!editing || !el) return;
        if (document.activeElement !== el) { el.focus(); el.setSelectionRange(el.value.length, el.value.length); }
        el.style.height = 'auto';
        el.style.height = `${el.scrollHeight}px`;
    }, [editing, draft]);

    const shown = html || `<p>${esc(line.text)}</p>`;
    if (line.other) return <li className="log-row log-other"><div className="log-text" dangerouslySetInnerHTML={{__html: shown}}/></li>;

    const openLinkOrEdit = (target: EventTarget) => {
        const a = (target as HTMLElement).closest('a');
        if (a) { if (/^(https?:|mailto:)/i.test(a.href)) Browser.OpenURL(a.href).catch(() => window.open(a.href, '_blank')); return; }
        if (!readOnly) onEdit();
    };
    return (
        <li className={`log-row${editing ? ' is-editing' : ''}`}>
            {timing && !readOnly
                ? <input className="input input-sm log-time-input" type="time" aria-label="Time" autoFocus defaultValue={line.time.padStart(5, '0')}
                         onBlur={e => { setTiming(false); if (!cancelTime.current) onTime(e.currentTarget.value); cancelTime.current = false; }}
                         onKeyDown={e => { if (e.key === 'Enter') e.currentTarget.blur(); if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); cancelTime.current = true; e.currentTarget.blur(); } }}/>
                : <button className="log-time" disabled={readOnly} aria-label={line.time ? `Time ${line.time}. Change` : 'Add a time'}
                          onClick={() => setTiming(true)}>{line.time || '–'}</button>}
            {editing
                ? <textarea ref={ta} className="log-input" rows={1} aria-label="Line" value={draft}
                            onChange={e => onDraft(e.target.value)} onKeyDown={onKey} onBlur={onBlur}/>
                : <div className="log-text" role="button" tabIndex={readOnly ? -1 : 0} aria-label={`Edit: ${line.text || 'empty line'}`}
                       onClick={e => { e.preventDefault(); openLinkOrEdit(e.target); }}
                       onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (!readOnly) onEdit(); } }}
                       dangerouslySetInnerHTML={{__html: shown}}/>}
            <button className={`badge log-mark mark-${word(line.mark)}`} disabled={readOnly} aria-label={`Mark: ${word(line.mark)}. Change`}
                    onMouseDown={e => e.preventDefault() /* keep the line being edited */} onClick={onCycle}>{word(line.mark)}</button>
        </li>
    );
}

function AddField({autoFocus, onAdd}: {autoFocus: boolean; onAdd: (text: string, mark: M.Mark) => void}) {
    const [text, setText] = useState('');
    const add = (mark: M.Mark) => { if (!text.trim()) return; onAdd(text, mark); setText(''); };
    return (
        <div className="log-add">
            <label className="sr-only" htmlFor="log-add">Add a line</label>
            <input id="log-add" className="input" placeholder="What did you just do?" autoFocus={autoFocus} value={text}
                   onChange={e => setText(e.target.value)}
                   onKeyDown={e => {
                       const mark = markKey(e);
                       if (mark !== null) { e.preventDefault(); e.stopPropagation(); add(mark); }
                       else if (e.key === 'Enter' && !e.nativeEvent.isComposing) { e.preventDefault(); add(''); }
                   }}/>
            <p className="small muted log-keys"><Kbd>↩</Kbd> add · <Kbd>⌘1</Kbd> done <Kbd>⌘2</Kbd> next <Kbd>⌘3</Kbd> blocked · <Kbd>⇧↩</Kbd> a second row</p>
        </div>
    );
}
```

The Go HTML is safe as innerHTML: `core.MDBody` escapes every text node and allowlists link schemes (checked in the batch 2 review). The fallback escapes too.

- [ ] **Step 2: Append to `src/app.css`** (tokens only; app-only, DESIGN.md §11)

```css
/* The list page (src/log): a row per line, the add field under it. */
.log { display: grid; gap: var(--s4); }
.log-list { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--rule); }
.log-row { display: grid; grid-template-columns: var(--s7) minmax(0, 1fr) auto; gap: var(--s3); align-items: baseline; padding: var(--s2) 0; border-bottom: 1px solid var(--rule); }
.log-row.is-editing { background: var(--wash); }
.log-other { grid-template-columns: minmax(0, 1fr); }
.log-time { color: var(--muted); font-size: var(--text-sm); text-align: left; background: none; border: 0; padding: 0; cursor: pointer; font-variant-numeric: tabular-nums; }
.log-time-input { width: var(--s9); }
.log-text { min-height: 1.55em; cursor: text; }
.log-text p { margin: 0; }
.log-text img { display: block; max-height: var(--s9); margin-top: var(--s1); }
.log-input { width: 100%; resize: none; overflow: hidden; font: inherit; line-height: inherit; color: inherit; padding: 0; border: 0; background: transparent; }
.log-mark { background: none; color: var(--muted); cursor: pointer; }
.log-mark.mark-next { color: var(--accent-text); border-color: var(--accent); }
.log-mark.mark-blocked { color: var(--warning); border-color: var(--warning); }
.log-add { display: grid; gap: var(--s1); }
.log-keys .kbd { margin: 0 var(--s1); }
```

- [ ] **Step 3: Type-check**: `tsc ok`. (Task 6 uses the component; nothing renders it yet.)

- [ ] **Step 4: Commit**

```bash
git add desktop/frontend/src/log/LogEditor.tsx desktop/frontend/src/app.css
git commit -m "frontend: the LogEditor component (rows, add field, keys)"
```

---

### Task 6: `DayPage` opens list pages in the list editor

**Files:**
- Modify: `desktop/frontend/src/screens/DayPage.tsx`

Read the whole file first. Its save pipeline is the safety net (only real edits are written; orphans keep unsaved text); every change below goes through it.

- [ ] **Step 1: Imports and state.** Add imports:

```ts
import {LogEditor, type LogEditorHandle} from '../log/LogEditor';
import * as LogModel from '../log/model';
import type {LogPage} from '../../bindings/github.com/dasariprashant0/gloamlog/internal/core';
```

and after `const [writeAnyway, setWriteAnyway] = useState(false);`:

```ts
    const [listPage, setListPage] = useState<LogPage | null>(null); // the open page's Log section; null for a classic page
    const [full, setFull] = useState(false);                        // "Edit whole page" on a list page
    const [pageKey, setPageKey] = useState(0);                      // remounts the list editor with a new page
    const [summary, setSummary] = useState('');
    const logRef = useRef<LogEditorHandle>(null);
    const listMode = useRef(false);                                 // listPage && !full, for the callbacks
```

- [ ] **Step 2: One way to schedule a save.** Immediately before the `onMessage` callback, add

```ts
    // A change in either editor: save it 600 ms after the last one, if it differs from the file and from the page as it
    // opened (opening never writes).
    const scheduleSave = useCallback((md: string) => {
        const d = loaded.current;
        if (!d) return;
        pending.current = {day: d, md};
        clearTimeout(timer.current);
        timer.current = window.setTimeout(() => {
            const p = pending.current;
            pending.current = null;
            if (p && p.md !== lastSaved.current && p.md !== baseline.current) void save(p.day, p.md);
        }, SAVE_DELAY);
    }, [save]);

    const onLogChange = useCallback((md: string, w: number) => { scheduleSave(md); setWords(w); }, [scheduleSave]);
```

and make `onMessage`'s `'change'` case

```ts
            case 'change': {
                scheduleSave(m.markdown);
                Notes.Words(m.markdown).then(setWords, () => {}); // the count is a hint: on failure the old one stays
                break;
            }
```

(add `scheduleSave` to `onMessage`'s dependency list).

- [ ] **Step 3: `flush` asks the list editor.** In `flush`, replace

```ts
        if (d && !editor.isRawFallback()) {
            const md = editor.getMarkdown();
            if (md !== baseline.current && md !== lastSaved.current) unsaved = {day: d, md};
        } else if (p) unsaved = p;
```

with

```ts
        const md = d && listMode.current && logRef.current ? await logRef.current.markdown().catch(() => null)
            : d && !editor.isRawFallback() ? editor.getMarkdown() : null;
        if (d && md !== null) {
            if (md !== baseline.current && md !== lastSaved.current) unsaved = {day: d, md};
        } else if (p) unsaved = p;
```

- [ ] **Step 4: Open a list page as a list.** In the load effect, replace everything from `const p = await Notes.Open(day);` to the end of the `if (!p.isSkipped) { … }` block with

```ts
            const p = await Notes.Open(day);
            const kept = orphans.current.get(p.day);
            const parsed = p.isSkipped ? null : await Notes.LogParse(kept ?? p.body);
            if (!live) return;
            setPage(p);
            setWords(p.words);
            setRaw(false);
            setLoadError('');
            setWriteAnyway(false);
            loaded.current = null;
            if (!p.isSkipped) {
                show(p.day, kept ?? p.body, p.exists, p.notLocal, parsed?.ok ? parsed.page : null);
                if (kept !== undefined) baseline.current = ''; // the kept text is unsaved: the next flush writes it
            }
```

and change `show` to

```ts
    function show(d: string, body: string, exists: boolean, readOnly = false, list: LogPage | null = null) {
        setListPage(list);
        setFull(false);
        setPageKey(k => k + 1);
        setSummary(list ? LogModel.summary(list.lines as unknown as LogModel.Line[]) : '');
        listMode.current = !!list;
        editor.setMarkdown(body, {cursor: 'start'}); // the full editor holds the page too, for "Edit whole page"
        editor.setReadOnly(readOnly);
        baseline.current = list ? body : editor.getMarkdown(); // the list editor writes an untouched page back byte for byte
        lastSaved.current = exists ? baseline.current : '';
        loaded.current = d;
        (window as unknown as {__dlDoc?: string}).__dlDoc = d;
    }
```

- [ ] **Step 5: Jots and suggestions reach the list editor.** Replace the `'jot-into-page'` effect with

```ts
    useEffect(() => on<{id: string; day: string; heading: string; markdown: string}>('jot-into-page', j => {
        const notLoaded = () => void Notes.JotPlaced(j.id, 'notLoaded', '').catch(() => {}); // capture.go writes the file after 4 s
        if (loaded.current !== j.day) return notLoaded();
        if (listMode.current && logRef.current) {
            logRef.current.appendLines(j.markdown).then(r => {
                void Notes.JotPlaced(j.id, r.result, r.markdown).catch(() => {});
                if (r.result === 'appended') void save(j.day, r.markdown);
            }, notLoaded);
            return;
        }
        if (!editor.isRawFallback()) editor.appendToSection(j.id, j.day, j.heading, j.markdown, {unlessPresent: true});
        else notLoaded();
    }), [save]);
```

and in `insertUnder`, right after `if (!d) return;`:

```ts
        if (listMode.current && logRef.current) {
            void logRef.current.appendLines(markdown).then(r => {
                if (r.result === 'appended') { void save(d, r.markdown); onToast(`Added ${plural(count, 'line')}.`); }
                else onToast('Those lines are already on the page.');
            }, e => onToast(`Couldn't add the lines: ${String(e?.message ?? e)}`));
            return;
        }
```

(make `insertUnder`'s dependencies `[save, onToast]`).

- [ ] **Step 6: Edit whole page, and back.** Add before the `return`:

```ts
    async function editWholePage() {
        if (!logRef.current) return;
        editor.setMarkdown(await logRef.current.markdown(), {cursor: 'start'});
        listMode.current = false;
        setFull(true);
    }

    async function backToList() {
        const parsed = await Notes.LogParse(editor.getMarkdown());
        if (!parsed.ok) return onToast('This page has no Log heading now, so it stays in the full editor.');
        setListPage(parsed.page);
        setPageKey(k => k + 1);
        listMode.current = true;
        setFull(false);
    }
```

- [ ] **Step 7: Render.** In the header, a list page shows the summary as its title and the date in the kicker:

```tsx
                {(label || (listPage && !full)) && <p className="label">{listPage && !full ? [label, page?.title].filter(Boolean).join(' · ') : label}</p>}
                {page && (listPage && !full
                    ? <Display key={day} lines={[summary || 'Nothing yet']}/>
                    : <Display key={day} lines={[weekday, rest.join(' ')]}/>)}
```

Add `&& !listPage` to the old carry card's condition. Put the list editor just before the Milkdown wrapper, and hide Milkdown while the list editor shows:

```tsx
            {listPage && !full && !skipped && (
                <LogEditor key={pageKey} ref={logRef} day={day} page={listPage} readOnly={!!page?.notLocal} autoFocus={day === today}
                           onChange={onLogChange} onSummary={setSummary} onToast={onToast}/>
            )}
            <div hidden={skipped || !!page?.notLocal || (!!listPage && !full)}><Editor onMessage={onMessage} label={`Page for ${page?.title ?? day}`}/></div>
```

In the footer, add first:

```tsx
                    {listPage && (full
                        ? <Button variant="ghost" size="sm" onClick={() => void backToList()}>Back to the list</Button>
                        : <Button variant="ghost" size="sm" onClick={() => void editWholePage()}>Edit whole page</Button>)}
```

- [ ] **Step 8: Type-check, then check it in the browser mock.** `tsc ok`. Start the `gloamlog-mock` preview and verify with `read_page`, clicks and typing:
  - Today (no file, template `## Log`): title "Nothing yet", the add field focused; type a line and Enter: a row with the current time; the title reads "1 line".
  - Click the row: it edits; type ` #next`, press Esc: the chip shows "next". ⌘3 while editing: "blocked", and the window did NOT go to Review.
  - Enter at the end of a line: a new timed line below; Backspace on it: it goes.
  - ⌘Z: the last change is undone.
  - Yesterday (a list page) opens as a list; "Edit whole page" shows Milkdown with the same text; "Back to the list" returns.
  - An older classic page still opens in Milkdown with the old carry card.
  - `window.__failSaves = true`, edit a line, switch day: the "couldn't be saved yet" banner shows; set it back to `false` and press Try again: the banner goes.
  Fix what fails before committing.

- [ ] **Step 9: Commit**

```bash
git add desktop/frontend/src/screens/DayPage.tsx
git commit -m "day page: list pages open in the list editor; saves, jots and suggestions go through it"
```

---

### Task 7: From yesterday on a list page

**Files:**
- Create: `desktop/frontend/src/log/CarryBox.tsx`
- Modify: `desktop/frontend/src/screens/DayPage.tsx`, `desktop/frontend/src/app.css`

- [ ] **Step 1: Write `src/log/CarryBox.tsx`**

```tsx
// "From yesterday" on a list page: the last logged day's open lines. Keep adds a line with its mark, Done adds it as done,
// Drop forgets it for today (Notes.CarryDismiss). Nothing is written to the page until one is picked.
import {useState} from 'react';
import {Button} from '@personal/ui/react';
import {Notes, type CarryCard} from '../api';
import type {Mark} from './model';

export function CarryBox({day, card, onAdd, onToast}: {
    day: string; card: CarryCard; onAdd: (text: string, mark: Mark) => void; onToast: (message: string) => void;
}) {
    const marks = (card as unknown as {marks?: Mark[]}).marks ?? [];
    const [left, setLeft] = useState(() => card.items.map((text, i) => ({text, mark: (marks[i] ?? 'next') as Mark})));
    const [hidden, setHidden] = useState(false);
    if (hidden || left.length === 0) return null;
    const take = (i: number) => setLeft(l => l.filter((_, k) => k !== i));
    const keep = (i: number) => { onAdd(left[i].text, left[i].mark); take(i); };
    const done = (i: number) => { onAdd(left[i].text, ''); take(i); };
    const drop = (i: number) => {
        const it = left[i];
        take(i);
        Notes.CarryDismiss(day, it.text).catch(e => onToast(`Couldn't forget that line: ${String(e?.message ?? e)}`));
    };
    return (
        <section className="carry-box" aria-label={`From ${card.sourceLabel}`}>
            <div className="carry-head">
                <p className="label">From {card.sourceLabel} · {left.length} open</p>
                <div className="row-gap">
                    <Button variant="ghost" size="sm" onClick={() => { left.forEach(it => onAdd(it.text, it.mark)); setLeft([]); }}>Keep all</Button>
                    <Button variant="ghost" size="sm" onClick={() => setHidden(true)}>Hide</Button>
                </div>
            </div>
            <ul className="carry-list">
                {left.map((it, i) => (
                    <li key={it.text} className="carry-row">
                        <span>{it.text} <span className={`badge log-mark mark-${it.mark || 'done'}`}>{it.mark || 'done'}</span></span>
                        <span className="row-gap">
                            <Button variant="ghost" size="sm" onClick={() => done(i)}>Done</Button>
                            <Button variant="ghost" size="sm" onClick={() => keep(i)}>Keep</Button>
                            <Button variant="ghost" size="sm" onClick={() => drop(i)}>Drop</Button>
                        </span>
                    </li>
                ))}
            </ul>
        </section>
    );
}
```

- [ ] **Step 2: Show it** in `DayPage.tsx` (import `CarryBox` from `'../log/CarryBox'`), right after the header:

```tsx
            {listPage && !full && page?.carry && !skipped && (
                <CarryBox key={day} day={day} card={page.carry} onAdd={(t, m) => logRef.current?.addLine(t, m)} onToast={onToast}/>
            )}
```

- [ ] **Step 3: Append to `src/app.css`**

```css
/* From yesterday on a list page. */
.carry-box { display: grid; gap: var(--s2); padding: var(--s3) var(--s4); border: 1px dashed var(--rule); }
.carry-head, .carry-row { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); }
.carry-list { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--s1); }
```

- [ ] **Step 4: Check it in the browser mock**: today shows "From <yesterday> · 2 open" with "Waiting on the API keys from ops" (blocked) and "Write the migration test" (next). Keep: the line joins the list at the current time with its mark. Done: it joins as done. Drop: it goes, and opening Today again (⌘T) doesn't offer it. Keep all adds both. `tsc ok`.

- [ ] **Step 5: Commit**

```bash
git add desktop/frontend/src/log/CarryBox.tsx desktop/frontend/src/screens/DayPage.tsx desktop/frontend/src/app.css
git commit -m "day page: From yesterday on a list page (Done / Keep / Drop)"
```

---

### Task 8: Paste an image into a line

**Files:**
- Modify: `desktop/frontend/src/log/LogEditor.tsx`

- [ ] **Step 1: Add the paste handler.** Import `ClipboardEvent` beside `KeyboardEvent` (`import type {ClipboardEvent, KeyboardEvent} from 'react';`). In `LogEditor`, before `onKey`:

```tsx
    async function onPaste(e: ClipboardEvent<HTMLTextAreaElement>) {
        const file = [...e.clipboardData.files].find(f => f.type.startsWith('image/'));
        if (!file) return; // text pastes as usual
        e.preventDefault();
        const at = e.currentTarget.selectionStart;
        try {
            const b64 = await new Promise<string>((ok, fail) => {
                const r = new FileReader();
                r.onload = () => ok(String(r.result).split(',')[1] ?? '');
                r.onerror = () => fail(r.error);
                r.readAsDataURL(file);
            });
            const path = await Notes.SaveImage(day, file.name || 'pasted.png', file.type, b64);
            act(s => M.setDraft(s, `${s.draft.slice(0, at)}![](${path})${s.draft.slice(at)}`));
        } catch (err) {
            onToast(`Couldn't add the image: ${String((err as Error)?.message ?? err)}`);
        }
    }
```

Pass `onPaste={onPaste}` to each `LogRow`, add `onPaste: (e: ClipboardEvent<HTMLTextAreaElement>) => void` to `LogRow`'s props, and put `onPaste={onPaste}` on its textarea.

- [ ] **Step 2: Check.** `tsc ok`. In the browser mock (its SaveImage returns a made-up path), paste an image into a line: the draft gets `![](assets/…)`. In the real app the row then shows the thumbnail after Esc; that check is the owner's on the next build.

- [ ] **Step 3: Commit**

```bash
git add desktop/frontend/src/log/LogEditor.tsx
git commit -m "list editor: paste an image into a line"
```

---

### Task 9: Full check, docs, status

**Files:**
- Modify: `docs/EDITOR_CONTRACT.md`, `docs/port/STATUS.md`, `docs/superpowers/specs/2026-10-18-writing-redesign-design.md`

- [ ] **Step 1: Every check**

```bash
gofmt -l internal desktop/*.go
go test ./internal/... -count=1 2>&1 | grep -v "telemetry\|upload" | tail -2
(cd desktop && go vet . 2>&1 | grep -v "ld: warning\|^#"; go test . -count=1 2>&1 | grep -v "ld: warning\|telemetry\|^#\|upload" | tail -1)
(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)
```

Expected: no gofmt output, all `ok`, `fail 0`, `tsc ok`.

- [ ] **Step 2: Docs**
  - `docs/EDITOR_CONTRACT.md`: a short section at the top, "Which editor runs": a list page (a `## Log` heading) opens in the list editor (`src/log`; Go writes the file through `Notes.LogRender`); a classic page, and "Edit whole page", use Milkdown under this contract.
  - `docs/port/STATUS.md` under "Built": ``- **Writing redesign, piece 2 (the list page):** line editor (rows, add field, ⌘1-3 while typing, Enter / ⇧Enter / ↑↓ / Backspace / ⌘Z / Esc, time edit, image paste), From yesterday (Done / Keep / Drop), Edit whole page; saves, jots and suggestions go through the same pipeline.`` Under "Known gaps": ``- App-level sync banners still show apart from the page's status line (spec 2.1).``
  - The spec's status line: `Pieces 1 and 2 built; 3 (standup) and 4 (commits) next.` In section 3, add after the keys table: `⌘1-3 mark a line while typing in it or in the add field; elsewhere they keep their app meaning (Today / Catch up / Review).`

- [ ] **Step 3: Commit**

```bash
git add docs
git commit -m "docs: writing redesign piece 2 status; which editor runs when"
```

---

## After this plan

Piece 3 (Standup: the same lines grouped, and Copy standup) and piece 4 (commits added at their time with `InsertTimed`, replacing the suggestions strip on list pages) each get a plan. The installed app picks this up on the next `desktop/build.sh` and reinstall.
