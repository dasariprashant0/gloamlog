# Work and Personal: two lists per day — Implementation Plan

> **For agentic workers:** run inline, in batches with checkpoints (the owner's choice). TDD per task: the listed tests
> first, see them fail, then the code, then the commands below. Steps use checkbox (`- [ ]`) syntax.

**Goal:** a day page holds `## Work` and `## Personal` lists; everything that arrives is routed by folder; standup and the
review copy are Work only.

**Spec:** [docs/superpowers/specs/2026-10-19-work-personal-split-design.md](../specs/2026-10-19-work-personal-split-design.md).

**Architecture:** the file format stays in Go. `LogPage` gains `Sections []LogSection` and every `LogLine` a `Section` index;
`Markdown()` writes each section's lines (picked by index, so the screen may append in any order) between the texts that
were around them. A page with one of Work / Personal gets the other as a *virtual* section that writes nothing until it has
lines. The frontend model keeps one flat `lines` array (undo, focus and moves work across lists unchanged); appends take a
section, head gaps and time order are per section.

**Commands** (Go needs `export GOCACHE=/tmp/claude-501/gocache GOMODCACHE=/tmp/claude-501/gomodcache GOFLAGS=-mod=mod GOTELEMETRY=off`):

- Core tests: `go test ./internal/... -count=1 2>&1 | grep -v "telemetry\|upload" | tail -3`
- Desktop tests: the module cache in `/tmp` may be gone and the sandbox can't fetch: run
  `cd desktop && go vet . && go test . -count=1 2>&1 | grep -v "ld: warning" | tail -3; cd ..` in the owner's terminal tab.
- Bindings: `(cd desktop && /tmp/claude-501/gobin/wails3 generate bindings -d frontend/bindings -ts .)`; if the binary is
  gone, run it in the owner's terminal with their `wails3`.
- TS: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
- Browser: the `gloamlog-mock` preview (`.claude/launch.json`, port 9247).

**Checkpoints:** after Batch 1 (core, tests green) and after Batch 2 (the browser check, screenshots).

---

## Batch 1: core

### Task 1: List sections in `loglist.go`

**Files:** `internal/core/loglist.go`, `internal/core/loglist_test.go`

Types:

```go
// ListHeadings name the sections a list page keeps its lines under: the one-list page's Log, and the split page's Work and Personal.
var ListHeadings = []string{LogHeading, ListWork, ListPersonal}
const ListWork, ListPersonal = "Work", "Personal"

type LogSection struct {
	Name    string `json:"name"`    // "Log", "Work" or "Personal" (canonical, whatever the heading's case)
	Join    string `json:"join"`    // for a section after the first: the page from the end of the previous section up to and including this heading line
	Trail   int    `json:"trail"`   // blank lines after its last line
	Virtual bool   `json:"virtual"` // the missing partner: written (Join is its heading line) only once it has lines
}
// LogLine gains: Section int `json:"section"` (index into LogPage.Sections)
// LogPage gains: Sections []LogSection `json:"sections"`; Trail stays (= the last real section's trail) for old callers
```

Behaviour:

- `listBlocks(scanned)`: the jots blocks of every name in `ListHeadings`, in file order, dropping a block whose heading lies
  inside an earlier kept block (nested: it stays an Other line). `IsLogPage` = `len(listBlocks) > 0`.
- `ParseLog`: `Before` = up to the first list heading; each section parsed with today's loop, its lines get `Section = i`;
  `Join` for i > 0 = `lines[end(i-1) : heading(i)+1]`; `After` from the last section's end. If the names hold Work xor
  Personal, add the other as `Virtual` with `Join` = the existing list heading's `#`s + " " + name.
- `Markdown`: `Before`; for each section i: real → `Join` (i > 0), its lines (by `Section == i`, in slice order), `Trail`
  blanks; virtual with lines → a blank line unless the previous output ends in one, the heading, its lines. A blank line
  before a non-empty `After` when a virtual section was written. No `Sections` (an old caller) → today's single-section
  output with `Trail`.
- `(p LogPage) Target(list string) int`: the section named `list`; else the first section (a Log page).
- `OpenLines(body)`: every section's open lines (Section set).
- `InsertTimed` is unchanged (callers pass one section's lines).

Tests (add to `loglist_test.go`):

```go
func TestSplitPageRoundTripsAndKnowsSections(t *testing.T) {
	body := "# d\n\n## Work\n\n- 09:40 fixed it\n- 15:30 keys #blocked\n\n## Notes\nfree text\n\n## Personal\n\n- 21:10 standup view\n\n## After\nx"
	p, ok := ParseLog(body)
	if !ok || len(p.Sections) != 2 || p.Sections[0].Name != "Work" || p.Sections[1].Name != "Personal" {
		t.Fatalf("%v %+v", ok, p.Sections)
	}
	if got := []int{p.Lines[0].Section, p.Lines[1].Section, p.Lines[2].Section}; got[0] != 0 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("sections %v", got)
	}
	if p.Markdown() != body {
		t.Fatalf("not byte for byte:\n%q", p.Markdown())
	}
	if p.Target("Personal") != 1 || p.Target("Work") != 0 {
		t.Fatal("target")
	}
}

func TestLogPageStaysOneListAndNestedHeadingIsOther(t *testing.T) {
	body := "## Log\n\n- a\n\n### Personal\n- b\n"
	p, _ := ParseLog(body)
	if len(p.Sections) != 1 || p.Target("Personal") != 0 || p.Markdown() != body {
		t.Fatalf("%+v", p.Sections)
	}
	if !p.Lines[1].Other {
		t.Fatal("a nested list heading is an Other line")
	}
}

func TestMissingPartnerWritesOnlyWithLines(t *testing.T) {
	body := "## Work\n\n- a\n\n## Notes\nx"
	p, _ := ParseLog(body)
	if len(p.Sections) != 2 || !p.Sections[1].Virtual || p.Markdown() != body {
		t.Fatalf("%+v\n%q", p.Sections, p.Markdown())
	}
	p.Lines = append(p.Lines, LogLine{Time: "21:00", Text: "home", Section: 1, Blank: 1})
	if got := p.Markdown(); got != "## Work\n\n- a\n\n## Personal\n\n- 21:00 home\n\n## Notes\nx" {
		t.Fatalf("%q", got)
	}
}

func TestOpenLinesCoverEveryList(t *testing.T) {
	got := OpenLines("## Work\n- a #next\n## Personal\n- b #blocked\n")
	if len(got) != 2 || got[1].Section != 1 {
		t.Fatalf("%+v", got)
	}
}
```

- [ ] Write the tests, see them fail (`p.Sections undefined`), implement, `go test ./internal/core -count=1`.
- [ ] Commit: `core: a list page holds several lists (Log, Work, Personal)`.

### Task 2: Settings `lists`, `ListFor`, the template move

**Files:** `internal/core/settings.go`, `internal/core/settings_test.go`

- `ListsPrefs{Work []string "work"; Personal []string "personal"; Jots string "jots"}`; `DefaultListsPrefs(home)` =
  `{[home/Projects/zuddl], [home/Projects/personal], "Work"}`; `Settings.Lists` (`"lists"`), decoded like `sources`
  (a damaged value falls back to the defaults); `Normalized` keeps `Jots` to Work / Personal.
- `SplitTemplate = "## Work\n\n## Personal"`; `DefaultTemplate = SplitTemplate`. In `decodeOver`: no `"lists"` key and a
  template equal to `"## " + LogHeading` → `SplitTemplate` (once: `lists` is written on the next save).
- `(p ListsPrefs) ListFor(root string) string`: the longest folder in either list with `PathContains(folder, root)`; none: Work.

Tests: the defaults; `ListFor` deepest match (`~/Projects/personal/zuddl-notes` under a Personal folder inside Work, etc.)
and none → Work; an old file with template `## Log` and no `lists` moves to the split template, one with `lists` keeps
`## Log`; a damaged `lists` keeps the other settings.

- [ ] Tests, fail, implement, pass. Commit: `settings: Work and Personal folders; new days have two lists`.

### Task 3: Notes find their list

**Files:** `capturetypes.go`, `inbox.go`, `daywriter.go`, `markdownbodyjots.go`, `captureplacement.go` (only if needed), tests;
`desktop/resources/claude/gloamlog-mcp.py`.

- `CaptureItem.List string json:"list,omitempty"`; `CaptureAt(text, source, list string, at)` and `Capture(text, source, list)`;
  `DrainInbox` passes `ListFor(entry.Cwd)` when the entry has a `cwd`, else `prefs.Jots`. `InboxEntry.Cwd` from `"cwd"`.
- `JotsHeadingFor(body, configured, list)`: on a list page (or an empty day when new days are list pages) the name of
  `ParseLog(body).Target(list)`'s section — or `list` itself when the page lacks it (a virtual partner, or an empty day);
  else `configured`.
- Duplicate checks (`AcknowledgeSaved`, `fileHolds`, `completed`) read every list section on a list page
  (`jotKeysOf(body)` = the union over `ListHeadings`).
- The MCP helper writes `"cwd": os.getcwd()` beside `text` and `ts`.

Tests: a jot with `List: "Personal"` lands under `## Personal` in the file (and creates it after `## Work` when missing); a
`## Log` page takes it under Log; an inbox entry with a Personal `cwd` lands in Personal; a replayed jot already under
Personal is not added twice.

- [ ] Tests, fail, implement, pass (`go test ./internal/... -count=1`). Commit: `capture: notes go to their list`.

### Task 4: Carry-over and the review

**Files:** `carryover.go`, `periodreview.go`, `reviewmodel.go`, `reviewformats.go` (only a helper), tests.

- `CarryCard.Lists []string json:"lists"` (a list page source: the name of each item's section; `"Work"` for a Log line);
  `OpenItems` returns it alongside marks (carry-over-heading items: `""`).
- Review: `GroupPersonal ReviewGroupKind = "personal"`. In list mode a Personal line goes into a per-day "Personal" bucket,
  emitted as one group titled "Personal" after Done / Next / Blocked; Log and Work lines group as today. `outsideLog` keeps
  the text between lists (each `Join` minus its heading line). Work / Personal headings are not "extra" classic headings.
- `WithoutPersonal(s ReviewSummary) ReviewSummary` drops that group; `desktop` uses it for copies only (Task 5).

Tests: carry card lists; a split week's review has Done / Next / Blocked from Work and a Personal group; `WithoutPersonal`.

- [ ] Tests, fail, implement, pass. Commit: `carry-over and review know Work and Personal`.

**Checkpoint 1:** core tests green; show the owner.

---

## Batch 2: the app

### Task 5: Desktop Go

**Files:** `desktop/notes_log.go`, `notes_sources.go`, `capture.go`, `notes_review.go`, `notes_carry.go`, tests; bindings.

- `LogAppendLines(page, markdown, list string)`: parsed lines get `Section = page.Target(list)` and the head gap of that
  section; duplicates checked over the whole page.
- `LogRender` unchanged (Markdown handles sections).
- Suggestions: `internal/sources` `SuggestedBullet.Root` (set where the bullet is built); the desktop result gives each bullet
  `List = s.Lists.ListFor(Root)` (a `Lists map[id]string` on `Suggestions`, or a field on a desktop wrapper — keep the
  sources package free of core settings).
- Capture: the quick-capture entry points pass `s.Lists.Jots`; `jot-into-page` carries `list` and the heading from
  `JotsHeadingFor(body, configured, item.List)`.
- `ReviewText`: copies `core.WithoutPersonal(b.sum)`.
- `notes_carry.go` `offerFor`: unchanged logic, keeps `Lists` in step with `Items` / `Marks`.

Tests: `LogAppendLines` to Personal on a Work-only page; offerFor keeps lists aligned. Regenerate bindings; `tsc ok`.

- [ ] Commit: `desktop: lists for appends, suggestions, jots and the review copy`.

### Task 6: The frontend model

**Files:** `desktop/frontend/src/log/model.ts`, `model.test.ts`

- `Line.section: number`; `newLine(time, text, mark, section = 0)`; `append(s, text, now, mark, section = 0)` adds at the end
  of that section (after its last line in the flat array, or after the previous sections when empty); the first line of a
  section gets `blank: 1`; `newLineAfter` copies the section; `setTime` sorts within the section; `removeLine` / head gaps
  per section.
- `standup(lines, sections?)` and `standupText` take the Work (or Log) section only; `personal(lines, sections)` lists the
  Personal ones; `summary` counts every list.

Tests: appends land in their section and keep the other's order; per-section head gap; setTime stays in its section;
standupText leaves Personal out.

- [ ] Commit: `list editor: lines know their list`.

### Task 7: The page

**Files:** `LogEditor.tsx`, `CarryBox.tsx`, `DayPage.tsx`, `SuggestionsStrip.tsx`, `app.css`, `src/dev/mock*.ts`

- `LogEditor` renders one list per section (title = name when there is more than one section), each with its own add field
  (`id` per section). `addLine(text, mark, list)` and `appendLines(md, list)` route with the section of that name (else the
  first). The standup view: Work groups, then a "Personal" group; Copy standup copies Work.
- `CarryBox` passes each item's list; `DayPage` routes jots (`j.list`) and suggestions (each bullet's list; one append per
  list).
- Mock: the template `## Work\n\n## Personal`; recent days split; `LogAppendLines(page, md, list)`; suggestions with lists;
  carry cards with `lists`.

- [ ] `tsc ok`, tests green. Commit: `day page: Work and Personal lists`.

### Task 8: Settings > Page

**Files:** the Page pane in `desktop/frontend/src/screens/settings/`

- "Work folders" / "Personal folders" (textarea, one path per line, saved on blur) and "Quick notes go to" (Segmented
  Work / Personal), saved through `SaveSettings` like the other fields.

- [ ] Commit: `settings: Work and Personal folders`.

### Task 9: Browser check

In the mock: today shows Work and Personal, each with an add field; adding to each writes its own heading; From yesterday
puts a Personal line back in Personal; Insert selected sends a personal-project suggestion to Personal; Standup shows
Work groups and a Personal group, and Copy standup copies Work only; a `## Log` day still shows one list; Edit whole page
shows both headings; Settings shows the folders. Screenshot for the owner.

**Checkpoint 2.**

### Task 10: Docs

- The redesign spec: status line and "Not doing" point to the split spec; `docs/port/STATUS.md` adds the split;
  `docs/EDITOR_CONTRACT.md` "Which editor runs": Log, Work and Personal are list headings.
- Commit: `docs: Work and Personal lists`.
