# Writing redesign: one log list, in time order

Status: approved 2026-10-18. Pieces 1, 2 and 3 built; 4 (commits) next. Builds on the Go + Wails port
(`port/wails`). Supersedes the five-heading day page for new days; [EDITOR_CONTRACT.md](../../EDITOR_CONTRACT.md) still
governs the full-page editor.

## Why

Writing a day felt like filling in a form. Four problems, all confirmed by the owner:

1. **Starting blank.** The page opens empty although the app already holds the day's jots, Claude's `log_work` notes and
   commits.
2. **Picking sections.** Five headings (What I did, Finished, Started, Pending / blocked, To do next), three of which
   overlap; every line needs a bucket first.
3. **Clutter.** Sync, reminder, carry-over and suggestion panels surround the editor.
4. **The editor.** A general rich-text editor on a free-form page: cursor jumps, lists and headings behave oddly.

How the owner works: writes in bits through the day, then tidies; uses the log for standups, weekly or review reports,
their own memory, and tomorrow's to-do; wants Claude's notes and commits to land on the page by themselves.

## Decisions

- One list per day, in time order. A line is done unless marked `#next` or `#blocked`.
- Claude's notes and commits are written straight onto the page as ordinary lines.
- A line editor replaces the rich-text editor for list pages; the full editor stays one click away.
- Old pages (five headings) are never rewritten and keep working everywhere.

## 1. The file

```markdown
## Log

- 09:40 Fixed the login redirect loop
- 11:15 Reviewed the pricing table copy
- 13:02 Add CSV export `1a2b3c4`
- 15:30 Waiting on API keys from ops #blocked
- 17:45 Write the migration test #next
  Needs the staging database first.
```

- **List page:** a page that has a `## Log` heading (any level, matched with `NormalizeHeading`). The heading name is
  a constant, `LogHeading = "Log"`, not a setting. Everything else is a **classic page**.
- **Line:** a bullet `- `, an optional `HH:MM ` time, the text, an optional trailing mark. Continuation lines are
  indented two spaces (the existing jot rule, `RenderJot`).
- **Marks:** the last whitespace-separated token of the line's first row, case-insensitive: `#next` or `#blocked`.
  `#next` in the middle of a sentence is plain text. Task syntax is read too: `- [ ]` is next and `- [x]` is done
  (to-do jots already write `- [ ]`). No mark means done.
- **Writing marks:** the editor writes `#next` / `#blocked`; marking a `- [ ]` line done ticks it (`- [x]`). A line
  the person did not touch is written back byte for byte.
- **No source tag.** Nothing records "from Claude" or "from git"; a commit keeps its short hash in backticks, which is
  also its de-duplication key.
- **Order:** file order is the order shown. New lines from the add field go at the end. Lines with a time that arrive
  later (a commit, a delivered jot) go before the first line with a later time; untimed lines keep their place.
  Editing a line's time moves it to its time position.
- **Content outside `## Log`** (added by hand in the full editor) is kept untouched and shown below the list.
- **New days** use the template `## Log` (the default `Settings.Template` changes). Settings > Page can still set the
  heading template; such days are classic pages.
- **Logged** keeps its rule: words ≥ `MinWords` (default 20). Times and marks are not words (a commit's hash counts
  as one). On a list page every Log line counts, whatever "count jots toward logged" says: the person's lines and
  delivered notes look the same there.

## 2. The page

From top to bottom (mockup in the chat transcript of 2026-10-18):

1. **One status line.** Sync copies to review, the next reminder, a folder problem: one line, each with its action
   (Review, Snooze, Fix). A folder that is missing or read-only still shows the full danger `Banner`.
2. **Head.** A `.label` kicker ("Today · Sun 18 Oct") and a short title that says where the day stands ("5 lines,
   1 blocked"; "Nothing yet" on an empty day). `Segmented`: List | Standup.
3. **From yesterday** (only when there is something): the open lines (`#next`, `#blocked`, `- [ ]`) of the most recent
   logged day before this one, 14-day lookback as now. Each has Done, Keep and Drop; the box has Keep all and Hide.
   - Keep adds the line to this day at the current time with its mark.
   - Done adds it as a done line ("Write the onboarding email" without the mark).
   - Drop writes nothing to the page; the line's `ItemKey` is remembered in `carry-dismissed.json` in the support
     folder, keyed by the target day, pruned after 30 days.
   - A line already on this day (same `ItemKey`) is not offered again.
   - Nothing is written until the person picks, so an untouched day stays empty.
4. **The list** (section 3).
5. **The add field**, focused when the page opens: "What did you just do?". Enter adds the line with the current
   time. `#next` typed in the text, or ⌘2, marks it.

Removed from the list page: the five empty headings, the reminder banner, the carry-over card and the suggestions
strip. Classic pages keep today's layout and editor.

## 3. The line editor

- Each line is a row: time (muted), text, mark. A row is shown rendered (bold, code, links, image thumbnail); the row
  being edited shows its plain markdown in a textarea that grows with its text.
- Keys:

  | Key | Does |
  |---|---|
  | Enter | new line after this one, current time |
  | ⇧Enter | a continuation row inside the same line |
  | ↑ / ↓ at the first / last row | move to the line above / below |
  | ⌘1 / ⌘2 / ⌘3 | done / next / blocked |
  | Backspace on an empty line | remove it |
  | ⌘Z / ⇧⌘Z | undo / redo, across lines |
  | Esc | leave the line |

  ⌘1-3 mark a line while typing in it or in the add field; elsewhere they keep their app meaning (Today / Catch up /
  Review).

- Clicking a time edits it (`HH:MM`); the line moves to its time position.
- Pasting an image saves it through the existing `SaveImage` (`assets/`, content-hash names) and puts
  `![](assets/<name>)` into the line.
- **Edit whole page** opens the existing full editor (Milkdown, unchanged) on the raw markdown, for anything the list
  does not cover. Closing it returns to the list.
- Saving is unchanged: the same debounced `Save`, the `writable()` rule (only real edits are written), the orphan
  retry banner when a save fails.
- Accessibility: the list is a `ul`; each row is reachable by keyboard; the mark is a real button with
  `aria-label="Mark: next"`; the add field has a visible label for screen readers.

## 4. Standup, review and the rest

- **Standup view:** the shown day as three groups, Done, Next and Blocked; times dropped. "Copy standup" copies plain
  text:

  ```
  Done
  - Fixed the login redirect loop
  Next
  - Write the migration test
  Blocked
  - Waiting on API keys from ops
  ```

  ⌘[ (the existing previous-day key) goes to yesterday, for a morning standup. The view never guesses which day is meant.
- **Review, "Group by section":** groups are Done, Next and Blocked. Classic pages map onto them:

  | Classic heading | Group |
  |---|---|
  | What I did, Finished, Started | Done |
  | To do next | Next |
  | Pending / blocked | Blocked |
  | anything else | its own group, after the three (as now) |

  These groups apply when the template is a list template; with a classic template the review is unchanged (the
  Swift differential test checks it). "Open at the end" comes from the last list page's marks. "Group by day", copy,
  PDF and export are unchanged.
- **Carry-over** on list pages follows the marks (section 2); on classic pages it keeps `CarryOverHeadings`.
- **Jot** (⌃⌥J) and **`log_work`**: on a list page the line goes at the end of `## Log`, through the existing
  journal-first `DayWriter` (notes arrive within seconds; only commits use time order). The suggestions strip, while
  it exists, adds to Log on a list page too. On a classic page they keep going under `JotsHeading`.
- **Commits:** for today only, commits from the projects ticked in Settings > Sources are added as lines at their
  commit time, through `DayWriter`, skipped when their short hash is already on the page. Checked when the page
  opens and every 5 minutes while the app runs (the sources cache already limits the cost). A past day has an "Add
  this day's commits" button instead; history is never rewritten by itself. Claude Code prompts and edited-file
  lists are not added.
- **Search** is unchanged; marks are searchable text.

## 5. Where the code goes

- `internal/core/loglist.go` (new, pure): parse a body into `LogPage{Lines, Before, After}`; `LogLine{Time, Text,
  Mark, Raw}`; serialize (untouched lines byte for byte); `InsertTimed`; `OpenLines`. Golden tests.
- `internal/core` changes: `DefaultTemplate`; carry-over picks marks on list pages; review groups map classic
  headings; capture placement targets `## Log` when the page has it; word count drops times, marks and the hash.
- `desktop/notes_*.go`: `CarryOffer(day)` / `CarryDismiss(day, key)`, `AddCommits(day)`; the commit
  timer next to the reminder loop.
- `desktop/frontend/src/screens/LogPage.tsx` (new) with `LogLine.tsx`, `CarryBox.tsx`, `StatusLine.tsx`; `DayPage`
  picks `LogPage` or the classic editor by page kind. App-only styles in `app.css` (DESIGN.md §11); move a piece to
  `@personal/ui` only when a second app needs it.
- The standup's grouping and copy text are `standup` / `standupText` in `desktop/frontend/src/log/model.ts`: they group
  the lines the list editor holds, so an edit not yet saved is in the standup too.
- Docs: update UX_SPEC.md (day page), EDITOR_CONTRACT.md (which editor runs when), PARITY.md / STATUS.md.

## 6. Tests

- Golden tests for parse and serialize, including a round trip on every page in the Swift diff fixtures (classic pages
  come back byte for byte).
- Unit tests: mark parsing (trailing only, task syntax, case), `InsertTimed` order, carry offers and dismissals,
  review mapping, word count, commit de-duplication.
- The Swift differential test stays green: classic-page behaviour does not change.
- Frontend: `tsc`, then the keyboard flows checked in the browser mock (add, mark, move, undo, time edit, image
  paste, Edit whole page and back), and the status line, carry box and standup copy.

## Build order

1. Core: `loglist.go`, marks, carry-over by marks, review mapping, capture into `## Log`, new default template.
2. The line editor and the list page.
3. Standup view and copy.
4. Commits added automatically.

Each piece is tested and committed on its own.

## Not doing

Drag to reorder, more marks than two, nested bullets beyond continuation rows, several log sections per day, a
source tag per line, adding Claude Code prompts, converting old pages.
