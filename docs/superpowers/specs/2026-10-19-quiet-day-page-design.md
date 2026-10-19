# A quieter list page

Status: approved 2026-10-19. Changes the list page of the [writing redesign](2026-10-18-writing-redesign-design.md)
(section 2, the page; section 3, the line editor). Frontend only: no file format, Go or binding changes.

## Why

The owner, looking at the installed app with the Work / Personal split: "too congested". On a real day:

- The head is four stacked things (kicker, the largest display title "10 lines", List | Standup, "Logged, 614 words") and
  takes a third of the window before the first line.
- Every line carries a boxed "done" chip, the default, repeated on every row.
- From yesterday is a dashed box whose Done / Keep / Drop wrap onto a second row, with the "Work" label jammed against it.
- Two large add fields ("What did you just do?", "Something for you?") and a row of key hints always shown.
- A rule under every row, and Claude's `log_work` notes run 3-7 lines each.

## Decisions

Chosen over "light touch" (fix only title, chips, wrapping, spacing) and "side by side" (Work and Personal as columns;
too narrow for long monospace notes).

### 1. Head

- Row 1: the `.label` kicker, "Today · Monday 19 October"; "· Saved" appears there for 2.2 s after a save.
- Row 2: the title (the line summary, "10 lines", "Nothing yet") in `.display-sm`, with List | Standup on the right.
- "Logged, N words" goes (the sidebar shows Logged). While the day is not logged, one small muted line stays:
  "12 words. Counts as logged at 20."
- Classic pages and the full editor keep today's head.

### 2. From yesterday

- Collapsed by default: one line between `--rule` lines, "2 open from Sun 18 Oct", and a "Review" text button.
- Review opens it: the head gets Keep all and Hide; each item is its text, its mark badge when next / blocked, then
  Done · Keep · Drop as small ghost buttons that never wrap (the text wraps beside them).
- No dashed box. Behaviour is unchanged: nothing is written until a pick; Hide hides it for this visit.

### 3. The lists

- Work and Personal are `.label` titles with `--s6` above. A list with no lines shows "Nothing yet." (muted, small).
- Rows have no rules; rows are `--s3` apart.
- The mark button shows when the line is next or blocked. A done line's mark button is invisible until the row is
  hovered, holds focus, or is being edited; it stays in the tab order and shows on keyboard focus.
- A row that is not being edited shows at most two text rows, ending in "…". Clicking it opens the full markdown to read
  or edit (the existing edit). Not clamped: rows with an image, and read-only pages (clicking can't open them there).

### 4. One add field

- One field below the last list replaces the per-list fields. Placeholder "What did you just do?".
- On a split page a chip on the field's right names the list it adds to ("Work"); clicking it switches to the other list.
  It starts on the quick-notes list from Settings > Page (`settings.lists.jots`; Work if unreadable) and keeps the
  choice while the page is open. A one-list page (`## Log`) has no chip.
- The key hints (↩ add · ⌘1-3 · ⇧↩) show only while the field has focus.
- Enter inside a line still adds a line after it, in that line's list.

## Not changing

Standup, the classic-page editor and its head, the sidebar, the status banners, saving, `model.ts` behaviour.

## Where the code goes

- `desktop/frontend/src/screens/DayPage.tsx`: the head (section 1).
- `desktop/frontend/src/log/CarryBox.tsx`: collapsed strip and open list (section 2).
- `desktop/frontend/src/log/LogEditor.tsx`: rows, empty lists, the single `AddField` with the list chip (sections 3-4).
- `desktop/frontend/src/app.css`: tokens only (DESIGN.md §2).
- `desktop/frontend/src/dev/mock*.ts`: only if the mock lacks `Settings().lists`.

## Tests

`tsc`, the 21 `model.test.ts` tests (unchanged model), then in the browser mock: the head on a logged and an unlogged
day; From yesterday collapsed, opened, Done / Keep / Drop, Keep all, Hide; a done line's mark shows on hover and on Tab;
a long line shows two rows and opens in full on click; an image line is not clamped; the chip switches lists and the
line lands in the chosen list; hints only while focused; a `## Log` page has no chip; Standup unchanged; dark theme.
