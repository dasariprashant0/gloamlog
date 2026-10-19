# Writing redesign, piece 4: Commits on the list page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The day's commits from the projects ticked in Settings > Sources are written onto a list page as ordinary Log lines at their commit time (`- 13:02 Add CSV export `1a2b3c4``), once each; on list pages this replaces the suggestions strip.

**Architecture:** A commit goes through the existing capture writer (`core.DayWriter`: journal first, an open page takes it in its editor, a closed day is written to the file), as a *timed* note: `CaptureItem.Timed`. A timed note is always stamped and is put among the Log lines with `core.InsertTimed` instead of at the end: in `ApplyJot` (file path) and in `Notes.LogAppendLines` (open list page, told by a `timed` flag on the `jot-into-page` event). A new `desktop/notes_commits.go` turns a sources scan into those notes: `AddCommits(day)` (the past-day button) and `autoAddCommits()` (today, while "Add commits for me" is on: when today's page opens, every 5 minutes, on `gloamlog://refresh`). The short hash is the de-duplication key: a commit whose hash is in the page, waits in the journal, or was added this session is skipped.

**Tech Stack:** Go (`internal/core`, `internal/sources`, `desktop`, Wails v3 beta.30 bindings), React 19 + TypeScript 5.6, `@personal/ui`.

**Spec:** [docs/superpowers/specs/2026-10-18-writing-redesign-design.md](../specs/2026-10-18-writing-redesign-design.md), section 1 (order: "lines with a time that arrive later go before the first line with a later time"), section 2 (the strip is removed from list pages), section 4 (Commits) and section 5. Pieces 1-3 are done: `core.ParseLog` / `InsertTimed` (`internal/core/loglist.go`), capture into `## Log`, the list editor (`src/log/`), the standup.

**Decided while planning (2026-10-19):**
- **Through the capture writer, not a new write path.** The writer already owns the rule "never write a day's file while its page is open" and makes delivery durable and idempotent. A commit is a note with `Timed: true`; nothing else about capture changes.
- **"Add commits for me" (`sources.autoAdd`) gates the automatic adds.** The switch already exists in Settings > Sources (ported from Swift, never wired: STATUS lists "auto-add commits" as not ported) and is off by default. Today's commits arrive by themselves only while it is on; the past-day button works whenever the sources are on. Its hint text changes from "about every 2 minutes" and "commits and pull requests" to what this piece does.
- **Which commits:** commit bullets of the normal sources scan (`sources.Bullets`, so redacted) that have a short hash and a time; pull requests, prompts and sessions are not added (spec: "Claude Code prompts and edited-file lists are not added"). The line text is the subject escaped with `sources.EscapeMarkdown` (as the strip escapes it), a space, and the hash in backticks.
- **De-duplication** is a plain `strings.Contains(page, sha)` over the page and the journal's waiting notes, plus a per-session set. Lines the strip inserted earlier ("Committed: … (acme 1a2b3c4)") hold the hash too, so those commits are not added a second time.
- **Only list pages.** A classic page (or a new day on a classic template) gets no commits and keeps the suggestions strip. On a list page the strip is gone, also in Edit whole page.
- **Scan:** its own `context.Background()` scan with the shared `Scanner`, not `Notes.Suggestions`, whose "newer scan cancels the older" rule would let the timer cancel the strip's look on a classic page.
- **Timer:** 5 minutes (the spec), a plain goroutine started from `main.go` next to `startCapture`.

**Not in this piece** (known ceilings): a commit line the person deletes comes back after a restart (the per-session set is in memory; add a `commits-dismissed.json` like `carry-dismissed.json` if it bothers the owner); a commit that arrives while the page is in Edit whole page lands at the end of Log (Milkdown's append), not at its time; pull requests. `Notes.Suggestions` keeps its now-unused "Log on a list page" heading branch.

**Commands:**

- Go, in the sandbox first:

```bash
export GOCACHE=/tmp/claude-501/gocache GOMODCACHE=/tmp/claude-501/gomodcache GOFLAGS=-mod=mod GOTELEMETRY=off
```

  - Core and sources: `go test ./internal/... -count=1 2>&1 | grep -v "telemetry\|upload" | tail -5`
  - Desktop: `(cd desktop && go vet . 2>&1 | grep -v "ld: warning\|^#"; go test . -count=1 2>&1 | grep -v "ld: warning\|telemetry\|^#\|upload" | tail -5)`
  - **If a Go command fails to download a module** (the `/tmp` module cache is empty and the Go proxy is blocked in the sandbox), run the same command in the owner's terminal tab with `run_in_terminal`, from `~/Projects/personal/daily-log`, without the `export` line, and read the result with `read_terminal`.
- Bindings, after any change to a `Notes` method signature or a bound type: `(cd desktop && /tmp/claude-501/gobin/wails3 generate bindings -d frontend/bindings -ts .)`. If that binary is gone, in the owner's terminal: `cd ~/Projects/personal/daily-log/desktop && go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.30 generate bindings -d frontend/bindings -ts .`
- TS model tests: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)")`
- Type check: `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)` (never `npx`: it needs the network)
- Browser check: the `gloamlog-mock` preview (`.claude/launch.json`, port 9246). In the mock, today starts empty; the three days before it are list pages when they are weekdays; older days are classic pages. The mock has no Go side: no `jot-into-page` event ever arrives, so commit lines appearing on the page is checked by the Go tests and, on the next build, by the owner.
- **GateGuard hook:** the first Write/Edit of each file is blocked until you state its importers/callers, the API it changes, the data it touches and the owner's instruction; state them, then retry the same call.

**Checkpoints** (when run inline, stop and show the owner): after Task 2 (core green), after Task 4 (Go green, bindings regenerated, `tsc ok`), after Task 6 (browser check with a screenshot).

---

## File structure

| File | Change | Responsibility |
|---|---|---|
| `internal/sources/activitydigest.go` | modify | `SuggestedBullet.Subject` / `.Sha` on commit bullets; `CommitLine(b)` |
| `internal/sources/activitydigest_test.go` | modify | Tests for them |
| `internal/core/capturetypes.go` | modify | `CaptureItem.Timed` |
| `internal/core/capturejournal.go` | modify | `"timed":true` written and read back |
| `internal/core/daywriter.go` | modify | `CaptureTimed`; an appended timed note is known by its own line only |
| `internal/core/captureplacement.go` | modify | `ApplyJot` puts a timed note by time on a list page (`applyTimed`) |
| `internal/core/*_test.go` | modify | Journal, writer and placement tests |
| `desktop/notes_log.go`, `desktop/notes_log_test.go` | modify | `LogAppendLines(page, markdown, timed)` |
| `desktop/capture.go` | modify | `timed` on `jot-into-page`; the session set; today's commits on page open and on refresh |
| `desktop/notes_commits.go` (new), `desktop/notes_commits_test.go` (new) | create | `AddCommits`, `autoAddCommits`, `startCommits` |
| `desktop/main.go` | modify | `startCommits(notes)` |
| `desktop/frontend/bindings/…` | regenerate | `AddCommits`, `LogAppendLines`' new argument, `SuggestedBullet`'s new fields |
| `desktop/frontend/src/log/LogEditor.tsx` | modify | `appendLines(markdown, timed)` |
| `desktop/frontend/src/screens/DayPage.tsx` | modify | Pass `timed`; strip on classic pages only; "Add this day's commits" |
| `desktop/frontend/src/screens/settings/SourcesPane.tsx` | modify | The two hints |
| `desktop/frontend/src/dev/mock.ts`, `mock-log.ts` | modify | `AddCommits`; the new argument |
| `docs/port/STATUS.md`, the spec | modify | Status |

---

### Task 1: Commit subject and hash on the bullet, and the line it becomes

**Files:**
- Modify: `internal/sources/activitydigest.go` (`SuggestedBullet` at line 14; the commit branch of `Bullets` around line 194; append `CommitLine` after `EscapeMarkdown`)
- Test: `internal/sources/activitydigest_test.go`

- [ ] **Step 1: Write the failing tests.** In `TestBulletsExactTexts`, replace the commit bullet check

```go
	if b := d.Bullets[2]; b.ID != "commit|/work/acme|3f2a9c1" || b.Evidence != "acme 3f2a9c1" {
		t.Errorf("commit bullet: %+v", b)
	}
```

with

```go
	if b := d.Bullets[2]; b.ID != "commit|/work/acme|3f2a9c1" || b.Evidence != "acme 3f2a9c1" || b.Subject != "Add pricing table" || b.Sha != "3f2a9c1" {
		t.Errorf("commit bullet: %+v", b)
	}
	if b := d.Bullets[1]; b.Subject != "" || b.Sha != "" {
		t.Errorf("only a commit has a subject and a hash: %+v", b)
	}
```

and append to the file:

```go
func TestCommitLine(t *testing.T) {
	line, ok := CommitLine(SuggestedBullet{Kind: KindCommit, Subject: "Fix *the* build #next", Sha: "1a2b3c4"})
	if !ok || line != "Fix \\*the\\* build \\#next `1a2b3c4`" {
		t.Errorf("escaped like BulletsMarkdown, hash in backticks: %q %v", line, ok)
	}
	for _, b := range []SuggestedBullet{
		{Kind: KindCommit, Subject: "no hash"},
		{Kind: KindCommit, Subject: "odd hash", Sha: "12`45"},
		{Kind: KindCommit, Sha: "1a2b3c4"},
		{Kind: KindPrompt, Subject: "x", Sha: "1a2b3c4"},
	} {
		if _, ok := CommitLine(b); ok {
			t.Errorf("not a line: %+v", b)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/sources -count=1 2>&1 | tail -5` (or the owner's terminal, see Commands)
Expected: build failure, `b.Subject undefined` / `undefined: CommitLine`.

- [ ] **Step 3: Implement.** In `SuggestedBullet`, after the `Time` field, add:

```go
	Subject  string       `json:"subject"`  // a commit: its subject, redacted and cut like Text; "" for other kinds
	Sha      string       `json:"sha"`      // a commit: its short hash; "" when unknown
```

In `Bullets`, the commit bullet becomes:

```go
			out = append(out, SuggestedBullet{ID: "commit|" + first.Root + "|" + id, Kind: KindCommit, Project: name,
				Text: text + ")", Evidence: ev, Time: c.Time, Subject: subject, Sha: sha})
```

After `EscapeMarkdown`, add:

```go
// CommitLine is a commit bullet as a list-page line: its subject escaped like BulletsMarkdown, then its short hash in backticks
// (the page's de-duplication key). ok is false for anything but a commit with a subject and a hex hash (a transcript is
// untrusted text: nothing else may sit inside the backticks).
func CommitLine(b SuggestedBullet) (string, bool) {
	if b.Kind != KindCommit || b.Subject == "" || b.Sha == "" || strings.Trim(b.Sha, "0123456789abcdef") != "" {
		return "", false
	}
	return EscapeMarkdown(b.Subject) + " `" + b.Sha + "`", true
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/sources -count=1 2>&1 | tail -5`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/activitydigest.go internal/sources/activitydigest_test.go
git commit -m "sources: commit bullets carry their subject and hash; CommitLine"
```

---

### Task 2: Timed notes in the capture writer

**Files:**
- Modify: `internal/core/capturetypes.go:12-20`, `internal/core/capturejournal.go:201-271`, `internal/core/daywriter.go` (header, `CaptureAt` at line 125, `completed` at line 433), `internal/core/captureplacement.go` (header, `ApplyJot` at line 111)
- Test: `internal/core/capturejournal_test.go`, `internal/core/daywriter_test.go`, `internal/core/captureplacement_test.go`

- [ ] **Step 1: Write the failing tests.** Append to `capturejournal_test.go`:

```go
func TestJournalKeepsATimedNote(t *testing.T) {
	j := NewCaptureJournal(t.TempDir())
	timed := jitem(1)
	timed.Timed = true
	must(t, j.Append(timed))
	must(t, j.Append(jitem(2)))
	p := j.Pending()
	expect(t, len(p) == 2 && p[0].Timed && !p[1].Timed, "%+v", p)
	expect(t, !strings.Contains(journalLine(jitem(2)), "timed"), "an ordinary note's line is unchanged")
}
```

Append to `captureplacement_test.go`:

```go
func TestCaptureApplyTimedGoesInTimeOrderOnAListPage(t *testing.T) {
	item := CaptureItem{Text: "Add CSV export `1a2b3c4`", Stamp: "13:02", Timed: true}
	got := ApplyJot(item, "## Log\n\n- 09:40 Fixed it\n- a line without time\n- 15:30 Later\n\n## Notes\nkept", LogHeading)
	expect(t, got == "## Log\n\n- 09:40 Fixed it\n- a line without time\n- 13:02 Add CSV export `1a2b3c4`\n- 15:30 Later\n\n## Notes\nkept", "%q", got)
	got = ApplyJot(item, "## Log\n\n- 15:30 Later", LogHeading)
	expect(t, got == "## Log\n\n- 13:02 Add CSV export `1a2b3c4`\n- 15:30 Later", "the earliest line goes first: %q", got)
	got = ApplyJot(item, "## Jots\n\n- 09:00 a", "Jots")
	expect(t, got == "## Jots\n\n- 09:00 a\n- 13:02 Add CSV export `1a2b3c4`", "a classic page: at the end, like any note: %q", got)
	got = ApplyJot(item, "## Log\n\n- 09:00 a\n\n```\ncode", LogHeading)
	expect(t, JotPresent(item, got, LogHeading) && countOf(got, "1a2b3c4") == 1, "a page ending in an open fence: the fence is closed first: %q", got)
}
```

Append to `daywriter_test.go`:

```go
func TestCaptureTimedPlacesTheLineByItsTimeAlwaysStamped(t *testing.T) {
	c := newCapCore(t)
	save(t, c.store, capToday, "## Log\n\n- 09:00 a\n- 15:00 c")
	prefs := DefaultCapturePrefs()
	prefs.Timestamps = false // a commit is stamped anyway: its time is where it goes
	w := NewDayWriter(c.store, NewCaptureJournal(c.journalDir), newCapLookup(), func() CapturePrefs { return prefs },
		func() time.Time { return capNow }, func() Calendar { return ny })
	_, ok := w.CaptureTimed("[] Add CSV export `1a2b3c4`", "git", mk(2026, 10, 7, 13, 2))
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
	r, _ := w.CaptureTimed("x `5d6e7f8`", "git", mk(2026, 10, 7, 13, 2))
	expect(t, r.Waiting == 1 && len(page.received) == 1 && page.received[0].Timed, "handed to the page as a timed note")
	w.AcknowledgeSaved(capToday, "## Log\n\n- 15:00 c")
	expect(t, w.Waiting() == 1, "a save without the commit's line does not acknowledge it")
	w.AcknowledgeSaved(capToday, page.reply)
	expect(t, w.Waiting() == 0, "the save that holds it does")
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/core -count=1 2>&1 | tail -5`
Expected: build failure, `timed.Timed undefined` / `w.CaptureTimed undefined`.

- [ ] **Step 3: The field and the journal.** In `capturetypes.go`, after the `Source` field of `CaptureItem`:

```go
	Timed  bool      `json:"timed,omitempty"` // goes among a list page's lines by its time (a commit), not at the end
```

In `capturejournal.go`, `journalLine`: replace

```go
	b.WriteString(`,"text":` + jsonString(item.Text) + `,"todo":`)
```

with

```go
	b.WriteString(`,"text":` + jsonString(item.Text))
	if item.Timed {
		b.WriteString(`,"timed":true`) // only when set: an ordinary note's line stays as Swift wrote it
	}
	b.WriteString(`,"todo":`)
```

In `parseJournalLine`, after `todo, _ := obj["todo"].(bool)` add `timed, _ := obj["timed"].(bool)`, and give the item `Timed: timed`:

```go
	return journalEntry{isItem: true, item: CaptureItem{ID: id, Ts: ts, Day: day, Text: text, Stamp: stamp, Todo: todo, Source: source, Timed: timed}}
```

- [ ] **Step 4: The writer.** In `daywriter.go`, replace `CaptureAt` (its comment line, signature and body down to `item.Stamp = …`'s closing brace) so the function reads:

```go
// CaptureAt is Capture dated with at: the note's day and time stamp come from at, not from the clock.
func (w *DayWriter) CaptureAt(text, source string, at time.Time) (CaptureReceipt, bool) {
	return w.capture(text, source, at, false)
}

// CaptureTimed is CaptureAt for a line that belongs at its own time (a commit): always stamped, never a to-do, and put among a
// list page's Log lines by time (ApplyJot, InsertTimed) rather than at the end. On a classic page it goes under Jots like any note.
func (w *DayWriter) CaptureTimed(text, source string, at time.Time) (CaptureReceipt, bool) {
	return w.capture(text, source, at, true)
}

func (w *DayWriter) capture(text, source string, at time.Time, timed bool) (CaptureReceipt, bool) {
	p := w.prefs()
	norm, todo, ok := normaliseCapture(text, p.TodoShorthand && !timed)
	if !ok {
		return CaptureReceipt{}, false
	}
	loc := w.cal().location()
	local := at.In(loc)
	item := CaptureItem{ID: newUUID(), Ts: at, Day: DayKeyString(at, loc), Text: norm, Todo: todo, Source: source, Timed: timed}
	if (p.Timestamps || timed) && !todo {
		item.Stamp = fmt.Sprintf("%02d:%02d", local.Hour(), local.Minute())
	}
```

(the rest of the old body, from `w.mu.Lock()` to the final `return`, stays as it is). In the header list, after the `CaptureAt(text, source, at)` line, add:

```go
//	CaptureTimed(text, source, at)   CaptureAt for a commit: always stamped, never a to-do, put by time on a list page (ApplyJot)
```

In `completed`, `case JotAppended:` becomes:

```go
	case JotAppended:
		// The note is the last item of that section, except a timed one (the page put it by time): that one is known by its own
		// line only, or another line's save would acknowledge it.
		if k, ok := LastJotKey(r.NormalizedMarkdown, JotsHeadingFor(r.NormalizedMarkdown, w.prefs().JotsHeading)); ok && !item.Timed {
			w.editorKeys[item.ID] = k
		}
		w.sent[item.ID] = sentNote{page: page, at: w.clock(), confirmed: true} // the editor's autosave writes it; AcknowledgeSaved acks it
```

- [ ] **Step 5: The placement.** In `captureplacement.go`, make `ApplyJot` start with the timed case:

```go
func ApplyJot(item CaptureItem, body, jotsHeading string) string {
	if out, ok := applyTimed(item, body); ok {
		return out
	}
	line := RenderJot(item)
```

and add after `ApplyJot`:

```go
// applyTimed puts a timed note (a commit) among a list page's Log lines by its time. ok is false on a classic page, or when the
// line would not show as a note there (the page ends inside an open code fence): ApplyJot then places it as usual.
func applyTimed(item CaptureItem, body string) (string, bool) {
	if !item.Timed {
		return "", false
	}
	p, ok := ParseLog(body)
	if !ok {
		return "", false
	}
	l, ok := parseLogRows(splitLines(RenderJot(item)))
	if !ok {
		return "", false
	}
	p.Lines = InsertTimed(p.Lines, l)
	out := TrimBody(p.Markdown())
	return out, JotPresent(item, out, LogHeading)
}
```

In the file header, after the `ApplyJot …` paragraph, add:

```go
//	            A timed note (CaptureItem.Timed: a commit) goes among a list page's Log lines by its time (InsertTimed) instead.
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/... -count=1 2>&1 | grep -v "telemetry\|upload" | tail -5`
Expected: `ok` for `internal/core` and `internal/sources`. If `TestCaptureApplyTimedGoesInTimeOrderOnAListPage`'s open-fence case fails, print `got` and check the fence closer comes before the line.

- [ ] **Step 7: Commit**

```bash
git add internal/core/capturetypes.go internal/core/capturejournal.go internal/core/daywriter.go internal/core/captureplacement.go internal/core/capturejournal_test.go internal/core/daywriter_test.go internal/core/captureplacement_test.go
git commit -m "core: timed notes go among a list page's lines by their time"
```

**Checkpoint:** show the owner the test output.

---

### Task 3: An open list page takes a commit at its time

**Files:**
- Modify: `desktop/notes_log.go:50-77`, `desktop/capture.go:138-161`, `desktop/frontend/src/log/LogEditor.tsx:12-19, 77-81`, `desktop/frontend/src/screens/DayPage.tsx:153-165`, `desktop/frontend/src/dev/mock-log.ts:51`
- Test: `desktop/notes_log_test.go`

- [ ] **Step 1: Write the failing test.** In `notes_log_test.go`, add `, false` as the last argument of every existing `n.LogAppendLines(…)` call (five calls), then append:

```go
func TestLogAppendLinesTimedGoesInTimeOrder(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	p := n.LogParse("## Log\n\n- 09:00 a\n- 15:00 c").Page
	a := n.LogAppendLines(p, "- 13:02 b `1a2b3c4`", true)
	if a.Result != "appended" || a.Rendered.Markdown != "## Log\n\n- 09:00 a\n- 13:02 b `1a2b3c4`\n- 15:00 c" {
		t.Fatalf("%s: %q", a.Result, a.Rendered.Markdown)
	}
	if again := n.LogAppendLines(n.LogParse(a.Rendered.Markdown).Page, "- 13:02 b `1a2b3c4`", true); again.Result != "alreadyPresent" {
		t.Fatalf("a commit already on the page is not added again: %s", again.Result)
	}
	if first := n.LogAppendLines(n.LogParse("## Log").Page, "- 13:02 b", true); first.Rendered.Markdown != "## Log\n\n- 13:02 b" {
		t.Fatalf("the first line of an empty Log: %q", first.Rendered.Markdown)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run the desktop test command with `-run LogAppend` (`go test . -run LogAppend -count=1`), in the owner's terminal if the sandbox cannot fetch modules.
Expected: build failure, `too many arguments in call to n.LogAppendLines`.

- [ ] **Step 3: Implement `LogAppendLines`.** Replace its comment and function with:

```go
// LogAppendLines adds the bullets of markdown (a jot as capture renders it) at the end of the Log, or each by its time when
// timed (a commit: core.InsertTimed), skipping any whose text is already a line. Used while the list editor has the page open.
func (n *Notes) LogAppendLines(page core.LogPage, markdown string, timed bool) LogAppended {
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
		switch {
		case timed:
			lines = core.InsertTimed(lines, l)
		case len(lines) == 0:
			l.Blank = 1 // a blank line between the heading and the first line
			lines = append(lines, l)
		default:
			lines = append(lines, l)
		}
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

- [ ] **Step 4: The event says which.** In `capture.go`, `AppendJot`'s emit becomes:

```go
	c.emit("jot-into-page", map[string]any{"id": item.ID, "day": item.Day, "heading": heading, "markdown": core.RenderJot(item), "timed": item.Timed})
```

and the file header's event line becomes `//  Events emitted:  "jot-into-page" {id, day, heading, markdown, timed}   a page must add the line (see JotPlaced; timed: by its time)`.

- [ ] **Step 5: Run the desktop tests**

Run: the desktop command (vet + all tests).
Expected: `ok`.

- [ ] **Step 6: Regenerate the bindings** (see Commands). Expected: in `frontend/bindings/…/desktop/notes.ts`, `LogAppendLines(page, markdown, timed: boolean)`; in `…/internal/sources/models.ts`, `SuggestedBullet` gains `subject` and `sha`. Nothing else should change; if other files change, look at why before going on.

- [ ] **Step 7: The frontend passes it on.** In `LogEditor.tsx`, the handle's doc and type:

```tsx
    /** Adds the bullets in markdown: at the end (a jot), or each at its time when timed (a commit). */
    appendLines(markdown: string, timed: boolean): Promise<{result: string; markdown: string}>;
```

and its implementation:

```tsx
        async appendLines(md, timed) {
            const a = await Notes.LogAppendLines({...page, lines: M.withDraft(stateRef.current)} as unknown as LogPage, md, timed);
```

(the two lines after it stay). In `DayPage.tsx`, the `jot-into-page` subscription's type gains `timed?: boolean` and the list call passes it:

```tsx
    useEffect(() => on<{id: string; day: string; heading: string; markdown: string; timed?: boolean}>('jot-into-page', j => {
```

```tsx
            logRef.current.appendLines(j.markdown, !!j.timed).then(r => {
```

`insertUnder`'s list branch still calls `appendLines(markdown)`: Task 5 deletes that branch; for now make it `appendLines(markdown, false)` so `tsc` passes.

In `mock-log.ts`, `LogAppendLines` takes the argument and ignores it (no mock path sends a timed line):

```ts
    LogAppendLines: (page: MockPage, markdown: string, _timed = false) => {
```

- [ ] **Step 8: Type check**

Run: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: `ℹ fail 0`, `tsc ok`.

- [ ] **Step 9: Commit**

```bash
git add desktop/notes_log.go desktop/notes_log_test.go desktop/capture.go desktop/frontend/bindings desktop/frontend/src/log/LogEditor.tsx desktop/frontend/src/screens/DayPage.tsx desktop/frontend/src/dev/mock-log.ts
git commit -m "list editor: a timed jot goes in at its time"
```

---

### Task 4: AddCommits and the automatic adds

**Files:**
- Create: `desktop/notes_commits.go`, `desktop/notes_commits_test.go`
- Modify: `desktop/capture.go` (`captureState`, `captureFor`, `EditorOpened`, `handleURL`), `desktop/main.go:149`

- [ ] **Step 1: Write the failing tests.** Create `desktop/notes_commits_test.go`:

```go
package main

import (
	"testing"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/dasariprashant0/gloamlog/internal/sources"
)

const commitDay = "2026-10-14" // captureNotes' today

// commitNotes is captureNotes with Git on for every project, and a scan that finds these bullets on any day.
func commitNotes(t *testing.T, bullets ...sources.SuggestedBullet) (*Notes, *recorder) {
	t.Helper()
	n, _, _, events := captureNotes(t, true)
	s := n.Settings()
	s.Sources.Git, s.Sources.Consented, s.Sources.AllProjects = true, true, true
	n.apply(s)
	old := scanCommits
	scanCommits = func(_, day, _ string, _ core.Calendar, _ core.SourcesPrefs) sources.ScanResult {
		return sources.ScanResult{Day: day, Bullets: bullets}
	}
	t.Cleanup(func() { scanCommits = old })
	return n, events
}

func commitAt(h, m int, subject, sha string) sources.SuggestedBullet {
	return sources.SuggestedBullet{Kind: sources.KindCommit, Subject: subject, Sha: sha, Time: time.Date(2026, 10, 14, h, m, 0, 0, time.Local)}
}

func dayBody(t *testing.T, n *Notes, day string) string {
	t.Helper()
	d, err := n.store.Load(day)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		return ""
	}
	return d.Body
}

func savePage(t *testing.T, n *Notes, day, body string) {
	t.Helper()
	if _, err := n.store.Save(day, body); err != nil {
		t.Fatal(err)
	}
}

func TestAddCommitsPutsEachAtItsTimeOnce(t *testing.T) {
	n, _ := commitNotes(t,
		commitAt(9, 15, "Add CSV export", "1a2b3c4"),
		commitAt(11, 0, "Fix *the* build", "5d6e7f8"),
		commitAt(9, 30, "a commit without a hash", ""),
		sources.SuggestedBullet{Kind: sources.KindPrompt, Text: "Worked on acme: a prompt", Time: time.Date(2026, 10, 14, 9, 40, 0, 0, time.Local)})
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started\n- 10:00 standup")
	added, err := n.AddCommits(commitDay)
	want := "## Log\n\n- 09:00 started\n- 09:15 Add CSV export `1a2b3c4`\n- 10:00 standup\n- 11:00 Fix \\*the\\* build `5d6e7f8`"
	if body := dayBody(t, n, commitDay); err != nil || added != 2 || body != want {
		t.Fatalf("added %d, err %v, page:\n%s", added, err, body)
	}
	if again, _ := n.AddCommits(commitDay); again != 0 {
		t.Fatalf("a hash already on the page is not added again (%d)", again)
	}
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started") // the person deleted both lines
	if again, _ := n.AddCommits(commitDay); again != 0 || dayBody(t, n, commitDay) != "## Log\n\n- 09:00 started" {
		t.Fatalf("a commit line deleted this session stays deleted: %d, %q", again, dayBody(t, n, commitDay))
	}
}

func TestAddCommitsKnowsLinesTheStripInserted(t *testing.T) {
	n, _ := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	savePage(t, n, commitDay, "## Log\n\n- Committed: Add CSV export (acme 1a2b3c4)")
	if added, _ := n.AddCommits(commitDay); added != 0 {
		t.Fatalf("the hash is on the page already (%d)", added)
	}
}

func TestAddCommitsGoThroughAnOpenPageAsTimedJots(t *testing.T) {
	n, events := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started")
	n.EditorOpened(commitDay)
	if added, err := n.AddCommits(commitDay); err != nil || added != 1 {
		t.Fatalf("added %d, err %v", added, err)
	}
	got := events.named("jot-into-page")
	if len(got) != 1 {
		t.Fatalf("jot-into-page events = %v", got)
	}
	ev := got[0].(map[string]any)
	if ev["day"] != commitDay || ev["heading"] != core.LogHeading || ev["markdown"] != "- 09:15 Add CSV export `1a2b3c4`" || ev["timed"] != true {
		t.Errorf("event = %v", ev)
	}
	if body := dayBody(t, n, commitDay); body != "## Log\n\n- 09:00 started" {
		t.Errorf("the file was written under an open page: %q", body)
	}
}

func TestTodaysCommitsArriveByThemselvesOnlyWithAddCommitsForMe(t *testing.T) {
	n, _ := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started")
	if got := n.autoAddCommits(); got != 0 {
		t.Fatalf("Add commits for me is off by default, yet %d were added", got)
	}
	s := n.Settings()
	s.Sources.AutoAdd = true
	n.apply(s)
	if got := n.autoAddCommits(); got != 1 {
		t.Fatalf("added %d, want 1", got)
	}
}

func TestAddCommitsLeavesClassicPagesAndNeedsTheSources(t *testing.T) {
	n, _ := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	classic := "## What I did\n\nwords"
	savePage(t, n, "2026-10-13", classic)
	if added, err := n.AddCommits("2026-10-13"); err != nil || added != 0 || dayBody(t, n, "2026-10-13") != classic {
		t.Fatalf("a classic page keeps the strip: %d, %v, %q", added, err, dayBody(t, n, "2026-10-13"))
	}
	if _, err := n.AddCommits("2026-10-15"); err == nil {
		t.Fatal("a day after today has no commits to add")
	}
	s := n.Settings()
	s.Sources.Consented = false
	n.apply(s)
	if _, err := n.AddCommits(commitDay); err == nil {
		t.Fatal("with the sources off the button says so")
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: the desktop test command with `-run Commits`.
Expected: build failure, `undefined: scanCommits` / `n.AddCommits undefined`.

- [ ] **Step 3: The session set.** In `capture.go`, add a field at the end of `captureState`:

```go
	commits map[string]bool // "day|sha" of the commits handed to the writer this session (notes_commits.go)
```

and in `captureFor`, after `replies: map[string]func(core.JotAppendResult){},` add `commits: map[string]bool{},`.

- [ ] **Step 4: Create `desktop/notes_commits.go`**

```go
package main

// Commits on a list page (writing redesign spec, section 4): the day's commits from the ticked projects become Log lines at
// their commit time, "- 13:02 Add CSV export `1a2b3c4`", through the capture writer as timed notes (journal first; an open page
// takes them in its editor; core.InsertTimed puts them in time order). The short hash is the de-duplication key: a commit whose
// hash is in the page, waits in the journal, or was added this session is not added again.
//
// Today's commits arrive by themselves while "Add commits for me" (sources.autoAdd) is on: when today's page opens, every
// commitTick, and on gloamlog://refresh. A past day has a button (AddCommits); history is never rewritten by itself. Classic
// pages get nothing here: they keep the suggestions strip.

import (
	"context"
	"errors"
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

// AddCommits puts day's commits on day's list page at their commit times and returns how many it added. A classic page gets
// none. The past-day button calls it; today's commits come by themselves (autoAddCommits).
func (n *Notes) AddCommits(day string) (int, error) {
	n.mu.Lock()
	s, cal, home, now := n.settings, n.cal(), n.home, n.now()
	n.mu.Unlock()
	today := core.DayKeyString(now, time.Local)
	if !core.DayKeyIsWellFormed(day) || day > today {
		return 0, errors.New("not a day with commits: " + day)
	}
	prefs := s.Sources
	if !prefs.AnyOn() || !prefs.Consented || (!prefs.AllProjects && len(prefs.Allowed) == 0) {
		return 0, errSourcesOff
	}
	c := captureFor(n)
	c.syncFolder()
	if c.headingFor(day, s.Capture.JotsHeading) != core.LogHeading {
		return 0, nil // a classic page (or an unreadable one): the suggestions strip covers it
	}
	result := scanCommits(home, day, today, cal, prefs)

	have := "" // the page and the notes waiting for it: where a commit already written shows its hash
	if doc, err := c.store.Load(day); err == nil && doc != nil {
		have = doc.Body
	}
	for _, it := range c.journal.Pending() {
		if it.Day == day {
			have += "\n" + it.Text
		}
	}
	added := 0
	for _, b := range result.Bullets {
		line, ok := sources.CommitLine(b)
		if !ok || b.Time.IsZero() || strings.Contains(have, b.Sha) {
			continue
		}
		key := day + "|" + b.Sha
		c.mu.Lock()
		seen := c.commits[key]
		c.commits[key] = true
		c.mu.Unlock()
		if seen {
			continue // added this session: a line the person deleted stays deleted
		}
		if _, ok := c.writer.CaptureTimed(line, "git", b.Time); ok {
			added++
		}
	}
	if added > 0 {
		c.notify()
	}
	return added, nil
}

// autoAddCommits adds today's commits while "Add commits for me" is on. Off, or nothing new: it tries again next time.
func (n *Notes) autoAddCommits() int {
	n.mu.Lock()
	on := n.settings.Sources.AutoAdd
	n.mu.Unlock()
	if !on {
		return 0
	}
	added, _ := n.AddCommits(core.DayKeyString(n.now(), time.Local))
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
```

(Two `AddCommits` running at once, say the timer and today's page opening, can hand the same commit to the writer twice; delivery skips a line already in the page, so the page still gets it once.)

- [ ] **Step 5: Wire the triggers.** In `capture.go`, `EditorOpened` ends:

```go
	c.mu.Unlock()
	go c.replay()
	if day == core.DayKeyString(n.now(), time.Local) {
		go n.autoAddCommits() // today's commits, while "Add commits for me" is on (notes_commits.go)
	}
}
```

`handleURL`, after `c.emit("refresh-sources", nil)`, adds:

```go
	go n.autoAddCommits() // a Claude Code session just ended: its commits too
```

In `main.go`, after `startCapture(app, notes)`:

```go
	// Today's commits on its list page every 5 minutes, while "Add commits for me" is on (notes_commits.go).
	startCommits(notes)
```

- [ ] **Step 6: Run the desktop tests**

Run: the desktop command (vet + all tests).
Expected: `ok`. The older capture tests call `EditorOpened` too; with the sources off, `autoAddCommits` returns at once and changes nothing they look at.

- [ ] **Step 7: Regenerate the bindings and type-check.** Expected: `notes.ts` gains `AddCommits(day: string): $CancellablePromise<number>`; `tsc ok`.

- [ ] **Step 8: Commit**

```bash
git add desktop/notes_commits.go desktop/notes_commits_test.go desktop/capture.go desktop/main.go desktop/frontend/bindings
git commit -m "desktop: commits added to the list page at their time"
```

**Checkpoint:** show the owner the Go test output and `tsc ok`.

---

### Task 5: The day page: no strip on list pages, a button on past days

**Files:**
- Modify: `desktop/frontend/src/screens/DayPage.tsx`, `desktop/frontend/src/screens/settings/SourcesPane.tsx:50-55`, `desktop/frontend/src/dev/mock.ts:179`

- [ ] **Step 1: `insertUnder` is for the classic page only now.** Replace its comment and body with:

```tsx
    // The suggestions strip's lines (classic pages only) go under a heading as one undoable step; the editor replies with appendResult.
    const insertUnder = useCallback((heading: string, markdown: string, count: number) => {
        const d = loaded.current;
        if (!d) return;
        appending.current = count;
        editor.appendToSection(`add-${Date.now()}`, d, heading, markdown, {unlessPresent: true});
    }, []);
```

- [ ] **Step 2: The strip only on classic pages.** Its condition becomes:

```tsx
            {page && !skipped && day <= today && !listPage && (
```

- [ ] **Step 3: "Add this day's commits".** After `editWholePage` / `backToList`, add:

```tsx
    // A past list page: its commits go in at their times (they arrive as jots); today's come by themselves.
    async function addCommits() {
        try {
            const n = await Notes.AddCommits(day);
            onToast(n ? `Added ${plural(n, 'commit')}.` : 'No new commits for this day.');
        } catch (e) {
            onToast(String((e as Error)?.message ?? e));
        }
    }
```

and in the footer, right after the Edit whole page / Back to the list block:

```tsx
                    {listPage && !full && day < today && !page.notLocal &&
                        <Button variant="ghost" size="sm" onClick={() => void addCommits()}>Add this day's commits</Button>}
```

- [ ] **Step 4: Settings > Sources says what happens.** In `SourcesPane.tsx`, the "Add commits for me" hint becomes:

```tsx
                <Setting title="Add commits for me" hint="Writes today's commits into a list page by themselves, at their commit time: when the page opens, about every 5 minutes, and right away when a Claude Code session ends (see the hook below). Edit or delete them like any other line. A past day has an Add this day's commits button.">
```

and in the "Instant update" hint, `instead of waiting up to 2 minutes` becomes `instead of waiting up to 5 minutes`.

- [ ] **Step 5: The mock.** In `mock.ts`, after the `AlreadyAdded` line:

```ts
    AddCommits: (_day: string) => later(0),
```

- [ ] **Step 6: Type check**

Run: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: `ℹ fail 0`, `tsc ok`.

- [ ] **Step 7: Commit**

```bash
git add desktop/frontend/src/screens/DayPage.tsx desktop/frontend/src/screens/settings/SourcesPane.tsx desktop/frontend/src/dev/mock.ts
git commit -m "day page: commits replace the suggestions strip on list pages"
```

---

### Task 6: Check it in the browser mock

**Files:** none. If a check fails, fix it in the files above and commit the fix with a message that says what it fixed.

- [ ] **Step 1: Open the mock.** `preview_start` with `{name: "gloamlog-mock"}`. Today is an empty list page. Expected: no "Suggested from your work" section anywhere on it, and no "Add this day's commits" button in the footer (today's come by themselves). Check with `find` for "Suggested from your work" (no match) and "Add this day's commits" (no match).

- [ ] **Step 2: A past list page.** Click outside any text field and press ⌘[ until the most recent weekday before today (a list page in the mock). Expected: still no strip; the footer has Edit whole page, then "Add this day's commits". Click it: a toast "No new commits for this day." (the mock finds none).

- [ ] **Step 3: Edit whole page.** On that day, click Edit whole page: Milkdown shows the page, the strip does not appear, and the "Add this day's commits" button is gone. Click Back to the list.

- [ ] **Step 4: A classic page.** Keep pressing ⌘[ until a weekday four or more days back (a classic page in the mock). Expected: the "Suggested from your work" strip is there, with its three mock suggestions, and no "Add this day's commits" button. Tick the commit suggestion and click Insert 1 line: the classic editor gets the line (the strip still works where it remains).

- [ ] **Step 5: Settings.** Open Settings > Sources: the "Add commits for me" hint reads as in Task 5 Step 4.

- [ ] **Step 6: Console.** `read_console_messages` with `onlyErrors: true`: no new errors.

- [ ] **Step 7: Screenshot** the past list page with its footer button for the owner (`computer` `{action: "screenshot"}`).

**Checkpoint:** show the owner the screenshot.

---

### Task 7: Full check, docs, status

**Files:**
- Modify: `docs/port/STATUS.md`, `docs/superpowers/specs/2026-10-18-writing-redesign-design.md`

- [ ] **Step 1: Every check**

```bash
go test ./internal/... -count=1 2>&1 | grep -v "telemetry\|upload" | tail -3
(cd desktop && go vet . 2>&1 | grep -v "ld: warning\|^#"; go test . -count=1 2>&1 | grep -v "ld: warning\|telemetry\|^#\|upload" | tail -1)
(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)
```

Expected: `ok` for every Go package, `ℹ fail 0`, `tsc ok`. (The Go lines in the owner's terminal if the sandbox cannot fetch modules.)

- [ ] **Step 2: Docs**
  - `docs/port/STATUS.md`, under "Built", after the piece 3 line: ``- **Writing redesign, piece 4 (commits)** ([plan](../superpowers/plans/2026-10-19-writing-redesign-4-commits.md)): with "Add commits for me" on, today's commits go onto its list page at their commit time (page open, every 5 minutes, `gloamlog://refresh`), once each by short hash; a past list page has "Add this day's commits"; the suggestions strip shows on classic pages only.``
  - `docs/port/STATUS.md`, "Known gaps": in the "Not ported:" line, delete `auto-add commits (`sources.autoAdd`), `.
  - The spec's status line (line 3): `Status: approved 2026-10-18. Pieces 1-4 built.` (keep the rest of the paragraph).
  - The spec, section 4, **Jot** bullet: replace `The suggestions strip, while it exists, adds to Log on a list page too.` with `The suggestions strip shows on classic pages only.`
  - The spec, section 4, **Commits** bullet: after `for today only,` insert `while Settings > Sources > "Add commits for me" is on,`.
  - The spec, section 5: the `desktop/notes_*.go` bullet becomes: ``- `desktop/notes_*.go`: `CarryOffer(day)` / `CarryDismiss(day, key)`; `notes_commits.go`: `AddCommits(day)` and the 5-minute commit timer. A commit is a timed capture note (`CaptureItem.Timed`, `DayWriter.CaptureTimed`): `ApplyJot` and `LogAppendLines` put it by its time with `InsertTimed`.``

- [ ] **Step 3: Commit**

```bash
git add docs
git commit -m "docs: writing redesign piece 4 status"
```

---

## After this plan

All four pieces are in. The installed app picks this up on the next `desktop/build.sh` and reinstall; the real-app check is the owner's on that build: switch on Settings > Sources > "Add commits for me", make a commit in a ticked repository, and see it land on today's page at its time within 5 minutes (or at once with the Stop hook), and not again after a second check.
