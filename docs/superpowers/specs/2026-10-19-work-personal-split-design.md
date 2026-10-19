# Work and Personal: two lists per day

Status: approved 2026-10-19 (the owner, in chat). Builds on the writing redesign
([2026-10-18-writing-redesign-design.md](2026-10-18-writing-redesign-design.md), pieces 1-3 built) and replaces its "one
list per day" rule. Piece 4 (commits added at their time) routes each commit with the rule below.

## Why

Everything Gloamlog collects lands on one list: Zuddl work (`~/Projects/zuddl`) and personal projects
(`~/Projects/personal`) mix on the page, in Copy standup and in the review. The owner wants them apart everywhere: the day
page, the suggestions, the standup and the review.

## Decisions

- **Two lists per day, in the file.** New days start with `## Work` and `## Personal`. Each is a list exactly like `## Log`
  today (timed bullets, `#next` / `#blocked`). Plain markdown, readable anywhere.
- **Routed by folder.** Work came from a folder: Claude Code sessions and git commits carry their project root, `log_work`
  notes carry the folder Claude ran in. The deepest configured folder that contains it decides the list; none matches: Work.
- **Old pages stay as they are.** A `## Log` page is still one list. Nothing is rewritten.
- **Standup and the review copy are Work.** Personal shows below them as its own group and is left out of what is copied.

## 1. The file

```markdown
# 2026-10-19

## Work

- 09:40 Fixed the login redirect loop
- 15:30 Waiting on API keys from ops #blocked

## Personal

- 21:10 Gloamlog: the Standup view
```

- A **list section** is a heading (any level, matched with `NormalizeHeading`) named `Log`, `Work` or `Personal`. A page with
  at least one is a list page (`IsLogPage`). A list heading nested inside an earlier list section is an Other line of that
  section, not a section of its own.
- A list page has its list sections in file order. Every line knows its section. Text between two list sections (another
  heading and what is under it) is kept as written. An untouched page is written back byte for byte, as today.
- **A missing partner.** A page with `## Work` but no `## Personal` (or the reverse) shows the missing list empty. The first
  line added to it writes its heading, at the same level, after the last list section (a blank line before it). A `## Log`
  page has no partner: it stays one list.
- **New days** use the template `## Work\n\n## Personal`. A settings file whose template is still the untouched
  `## Log` moves to it once (the first time `lists` is saved); a custom template stays.
- Words: every line of every list counts toward "logged", as on a `## Log` page.

## 2. Routing

- Settings, `lists`: `work` and `personal` (folders, absolute paths) and `jots` (`"Work"` or `"Personal"`, where a note
  without a folder goes). Defaults: `~/Projects/zuddl`, `~/Projects/personal`, `Work`.
- `ListFor(root)`: the deepest folder in either list that contains `root` (whole path components, `PathContains`) names the
  list; none: Work.
- Where a routed line goes on a page: the section with that name; on a page without it, the missing partner (section 1);
  on a `## Log` page, the Log list.
- Who carries a folder:
  - Claude Code sessions and git commits: the item's project root (`ActivityItem.Root`).
  - `log_work` notes: the helper writes `cwd` into the inbox entry; the note goes where `ListFor(cwd)` says.
  - Quick capture (hotkey, menu bar, menu): no folder, so `lists.jots`.

## 3. The page

- A list page shows its lists one under the other in file order, each under its name with its own add field. A page with
  one list (`## Log`) looks as it does today.
- The title (`5 lines, 1 next, 1 blocked`) counts every list.
- **From yesterday** remembers each line's list (`CarryCard.Lists`); Keep and Done put it back in that list (a `## Log`
  source line goes to Work on a split page).
- **Suggestions from your work** each carry their list; "Insert selected" puts each line in its own list. On a classic page
  nothing changes.
- **Standup**: Done / Next / Blocked from Work (or Log), then a **Personal** group with the Personal lines. Copy standup
  copies Work only.
- Moving a line from one list to the other: Edit whole page (no drag between lists in this piece).

## 4. The review

- On a list page, Work and Log lines go into Done / Next / Blocked as today. Personal lines go into one **Personal** group
  (kind `personal`) after them.
- Copying the review leaves the Personal group out. Export (PDF, markdown) keeps it.

## 5. Settings

Settings > Page: "Work folders" and "Personal folders" (one path per line), and "Quick notes go to: Work / Personal".

## 6. Where the code goes

- `internal/core/loglist.go`: list sections (`LogSection`, `LogLine.Section`), `ListHeadings`, `ParseLog` / `Markdown` over
  every section, the missing partner, `OpenLines` with sections, `(LogPage) Target(list)`.
- `internal/core/settings.go`: `ListsPrefs`, `ListFor`, the template move.
- `internal/core/capturetypes.go`, `daywriter.go`, `inbox.go`, `markdownbodyjots.go`: `CaptureItem.List`, the inbox `cwd`,
  `JotsHeadingFor(body, configured, list)`, duplicate checks over every list.
- `internal/core/carryover.go`: `CarryCard.Lists`. `periodreview.go` / `reviewmodel.go`: the Personal group, `GroupPersonal`.
- `internal/sources/activitydigest.go`: `SuggestedBullet.Root`.
- `desktop/`: `LogAppendLines(page, markdown, list)`, suggestion lists, jot events with their list, the review copy without
  Personal, the MCP helper's `cwd`.
- `desktop/frontend/src/log`: sections in the model, a list per section in `LogEditor`, the standup by list; `DayPage`,
  `CarryBox`, the suggestions strip, Settings > Page.

## 7. Tests

- Core: a two-list page round-trips byte for byte; lines know their section; a nested list heading stays an Other line;
  the missing partner renders only once it has lines; `ListFor` picks the deepest folder; a note with `List` lands in its
  section (file and open page); the inbox `cwd` routes; the template moves once; carry-over keeps lists; the review puts
  Personal in its own group and the copy leaves it out.
- Frontend (`node --test`): appends go to their section; per-section head gaps; standup and copy text use Work only.
- Browser mock: the checks in the plan.

## Not doing

More than two lists, lists named by the user, moving a line between lists in the list editor, choosing the list per line
by hand, auto-detecting work from text.
