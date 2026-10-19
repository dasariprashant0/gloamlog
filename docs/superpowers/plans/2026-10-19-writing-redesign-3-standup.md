# Writing redesign, piece 3: Standup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A list page gets a List | Standup switch: Standup shows the same lines grouped as Done, Next and Blocked (times dropped), and "Copy standup" copies them as plain text.

**Architecture:** Pure frontend. The list editor already holds the page's lines with their marks (`src/log/model.ts`), so the grouping and the copy text are two pure functions there, tested with `node --test`. `LogEditor` takes a `view` prop and, in Standup, renders a `StandupView` (same file, beside `LogRow` and `AddField`) from its own state instead of the rows and the add field; it stays mounted, so the draft, undo history and From yesterday's picks survive the switch. `DayPage` owns the view (it survives ⌘[ to yesterday) and shows the `Segmented` switch in the head. Copying goes through the existing `Notes.ReviewCopyText` (Go writes the clipboard; WebKit's async clipboard is unreliable outside a click).

**Tech Stack:** React 19 + TypeScript 5.6, `node --test` (Node 26 runs `.ts` directly), `@personal/ui` (`Segmented`, `Button`).

**Spec:** [docs/superpowers/specs/2026-10-18-writing-redesign-design.md](../specs/2026-10-18-writing-redesign-design.md), section 2 (the head's `Segmented`) and section 4 (Standup view). Pieces 1 and 2 are done: `core.LogLine` (`time`, `text`, `mark`, `task`, `other`), the list editor (`src/log/model.ts`, `src/log/LogEditor.tsx`), `DayPage`'s list mode.

**Decided while planning (2026-10-19):**
- **No Go `Standup(day)`.** Spec section 5 lists one, but the standup is the shown page's lines grouped by a mark the frontend already has. Reading the file in Go would miss an edit not yet saved (the save waits 600 ms), and no second caller needs it. The functions live in `model.ts`; nothing about the file format moves to TypeScript.
- **Copy text:** each group that has lines, as its title and `- ` bullets, file order, no times. Groups with no lines are left out (the spec's example has all three). Inline markdown (bold, code, links) is kept as written, since chat apps read it; images (`![](assets/…)`) are dropped. A line's continuation rows follow it, indented two spaces.
- **The view is per session, not per page:** it stays Standup across ⌘[ / ⌘] (spec: "⌘[ goes to yesterday, for a morning standup"). It is not saved to settings.
- **In Standup** the add field and From yesterday are hidden (From yesterday is hidden with `hidden`, not unmounted, so lines already kept or dropped are not offered again on the way back). The switch shows only on a list page in the list editor (not on a classic page, not in Edit whole page).
- Switching views never writes the page.

**Not in this piece:** the review's Done / Next / Blocked grouping (piece 1 did it); a keyboard shortcut for the switch; commits (piece 4). The suggestions strip stays as it is.

**Commands:**

- TS model tests: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)")`
- Type check: `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)` (never `npx`: it needs the network)
- Browser check: the `gloamlog-mock` preview (`.claude/launch.json`, port 9246). In the mock, today starts empty; the three days before it are list pages when they are weekdays (weekends stay empty), each with two done lines, one blocked ("Waiting on the API keys from ops") and one next ("Write the migration test"); older days are classic pages.
- No Go or bindings change in this piece, so no Go command is needed.

**Checkpoints** (when run inline, stop and show the owner): after Task 1 (tests green), after Task 3 (`tsc ok`, the switch in place), after Task 4 (the browser check with a screenshot).

---

## File structure

| File | Change | Responsibility |
|---|---|---|
| `desktop/frontend/src/log/model.ts` | modify | `Group`, `GROUPS`, `standup(lines)`, `standupText(lines)` |
| `desktop/frontend/src/log/model.test.ts` | modify | Tests for them |
| `desktop/frontend/src/log/LogEditor.tsx` | modify | `View` type, `view` prop, `StandupView` (groups, Copy standup) |
| `desktop/frontend/src/screens/DayPage.tsx` | modify | The view state, the `Segmented` switch, From yesterday hidden in Standup |
| `desktop/frontend/src/app.css` | modify | `.standup*` styles, tokens only |
| `docs/port/STATUS.md`, the spec | modify | Status; where the standup code lives |

---

### Task 1: Standup grouping and copy text (pure, tested)

**Files:**
- Modify: `desktop/frontend/src/log/model.ts` (append at the end)
- Test: `desktop/frontend/src/log/model.test.ts` (append at the end; `L` is the helper at the top of the file)

- [ ] **Step 1: Write the failing tests** (append to `model.test.ts`)

```ts
test('standup groups lines by mark in file order; other blocks and empty lines are left out', () => {
    const lines = [L('a'), L('b', {mark: 'next'}), L('a note', {other: true}), L(''), L('c', {mark: 'blocked'}),
        L('call Sam', {task: true, mark: 'next'}), L('d')];
    assert.deepEqual(M.standup(lines), {done: [0, 6], next: [1, 5], blocked: [4]});
});

test('standupText is the spec example: three titled groups, times dropped', () => {
    const lines = [
        L('Fixed the login redirect loop', {time: '09:40'}),
        L('Waiting on API keys from ops', {time: '15:30', mark: 'blocked'}),
        L('Write the migration test', {time: '17:45', mark: 'next'}),
    ];
    assert.equal(M.standupText(lines),
        'Done\n- Fixed the login redirect loop\nNext\n- Write the migration test\nBlocked\n- Waiting on API keys from ops');
});

test('standupText leaves out empty groups and images, keeps markdown, and indents continuation rows', () => {
    const lines = [L('Shipped **CSV export** `1a2b3c4` ![](assets/a.png)\nNeeds a note'), L('![](assets/b.png)'), L('x', {other: true})];
    assert.equal(M.standupText(lines), 'Done\n- Shipped **CSV export** `1a2b3c4`\n  Needs a note');
    assert.equal(M.standupText([]), '');
});
```

- [ ] **Step 2: Run them to see them fail**

Run: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)|standup")`
Expected: the three new tests fail with `M.standup is not a function` / `M.standupText is not a function`; `ℹ fail 3`.

- [ ] **Step 3: Implement** (append to `model.ts`)

```ts
export type Group = 'done' | 'next' | 'blocked';
/** The standup groups in the order they are shown and copied. */
export const GROUPS: [Group, string][] = [['done', 'Done'], ['next', 'Next'], ['blocked', 'Blocked']];

/** The standup view: indexes into lines by group, in file order. Other blocks and empty lines are left out. */
export function standup(lines: Line[]): Record<Group, number[]> {
    const g: Record<Group, number[]> = {done: [], next: [], blocked: []};
    lines.forEach((l, i) => { if (!l.other && l.text.trim()) g[l.mark || 'done'].push(i); });
    return g;
}

// A line as standup text: images dropped (they mean nothing pasted into a chat), continuation rows indented under it.
const plain = (text: string) =>
    text.replace(/!\[[^\]]*\]\([^)]*\)/g, '').split('\n').map(r => r.trim()).filter(Boolean).join('\n  ');

/** What Copy standup copies: each group that has lines, as its title and bullets; no times. */
export function standupText(lines: Line[]): string {
    const g = standup(lines);
    const out: string[] = [];
    for (const [key, title] of GROUPS) {
        const rows = g[key].map(i => plain(lines[i].text)).filter(Boolean);
        if (rows.length) out.push(title, ...rows.map(r => `- ${r}`));
    }
    return out.join('\n');
}
```

- [ ] **Step 4: Run the tests and the type check**

Run: `(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: `ℹ fail 0`, `tsc ok`.

- [ ] **Step 5: Commit**

```bash
git add desktop/frontend/src/log/model.ts desktop/frontend/src/log/model.test.ts
git commit -m "list editor: standup groups and copy text"
```

---

### Task 2: The Standup view in the list editor

**Files:**
- Modify: `desktop/frontend/src/log/LogEditor.tsx`
- Modify: `desktop/frontend/src/app.css` (append)

- [ ] **Step 1: The `view` prop.** In `LogEditor.tsx`:

1. Change the React type import to `import type {ClipboardEvent, KeyboardEvent, MouseEvent} from 'react';`.
2. Change the UI import to `import {Button, Kbd} from '@personal/ui/react';`.
3. Below `LogEditorHandle`, add:

```tsx
/** What the list page shows: the rows to edit, or the same lines as a standup. */
export type View = 'list' | 'standup';
```

4. In `Props`, add `view: View;` to the first line: `day: string; page: LogPage; readOnly: boolean; autoFocus: boolean; view: View;`
5. Add `view` to the destructured props: `function LogEditor({day, page, readOnly, autoFocus, view, onChange, onSummary, onToast}, ref)`.
6. Right before `return (` in `LogEditor` (after `onKey`, so every hook has run), add:

```tsx
    // The same state in both views: the draft, undo and the From yesterday picks survive a switch. html is per withDraft line.
    if (view === 'standup') return <StandupView lines={M.withDraft(state)} html={html} onToast={onToast}/>;
```

- [ ] **Step 2: Add `StandupView`** at the end of `LogEditor.tsx`, after `AddField`:

```tsx
function StandupView({lines, html, onToast}: {lines: M.Line[]; html: string[]; onToast: (message: string) => void}) {
    const [copied, setCopied] = useState(false);
    const groups = M.standup(lines);
    const text = M.standupText(lines);
    if (M.GROUPS.every(([g]) => !groups[g].length)) return <p className="muted">Nothing on this day yet.</p>;

    async function copy() {
        try {
            await Notes.ReviewCopyText(text); // Go writes the clipboard: WebKit's async clipboard fails outside a click
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (e) {
            onToast(`Couldn't copy the standup: ${String((e as Error)?.message ?? e)}`);
        }
    }
    // A link in a line opens in the browser, never inside the app's window.
    const openLink = (e: MouseEvent) => {
        const a = (e.target as HTMLElement).closest('a');
        if (!a) return;
        e.preventDefault();
        if (/^(https?:|mailto:)/i.test(a.href)) Browser.OpenURL(a.href).catch(() => window.open(a.href, '_blank'));
    };
    return (
        <div className="standup" onClick={openLink}>
            {M.GROUPS.filter(([g]) => groups[g].length).map(([g, title]) => (
                <section key={g} aria-label={title}>
                    <h2 className="label">{title}</h2>
                    <ul className="standup-list">
                        {groups[g].map(i => <li key={i} dangerouslySetInnerHTML={{__html: html[i] || `<p>${esc(lines[i].text)}</p>`}}/>)}
                    </ul>
                </section>
            ))}
            <div><Button variant="line" size="sm" disabled={!text} onClick={() => void copy()}>{copied ? 'Copied ✓' : 'Copy standup'}</Button></div>
        </div>
    );
}
```

(`html[i]` is Go's escaped HTML for that line, the same the rows show; `esc`, `Browser` and `Notes` are already in the file.)

- [ ] **Step 3: Append to `src/app.css`**

```css
/* The standup view of a list page. */
.standup { display: grid; gap: var(--s4); justify-items: start; }
.standup-list { margin: var(--s2) 0 0; padding-left: var(--s4); display: grid; gap: var(--s1); }
.standup-list p { margin: 0; }
.standup-list img { display: block; max-height: var(--s9); margin-top: var(--s1); }
```

- [ ] **Step 4: Type check**

Run: `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: one error, in `DayPage.tsx`: `Property 'view' is missing` (Task 3 passes it). Fix anything else before going on.

- [ ] **Step 5: No commit yet.** Go straight to Task 3; Tasks 2 and 3 commit together so no commit leaves `tsc` failing.

---

### Task 3: The List | Standup switch on the day page

**Files:**
- Modify: `desktop/frontend/src/screens/DayPage.tsx`

- [ ] **Step 1: Imports and state.**

1. Add `Segmented` to the UI import: `import {Banner, Button, Display, Segmented} from '@personal/ui/react';`
2. Change the list editor import to `import {LogEditor, type LogEditorHandle, type View} from '../log/LogEditor';`
3. Below `const SAVE_DELAY = 600;`, add:

```tsx
const VIEWS: [View, string][] = [['list', 'List'], ['standup', 'Standup']];
```

4. Below `const [summary, setSummary] = useState('');`, add:

```tsx
    const [view, setView] = useState<View>('list');                // List | Standup; kept across days (⌘[ for yesterday's standup)
```

- [ ] **Step 2: The switch in the head.** In `<header className="page-head">`, right after the `Display` block (`{page && (listPage && !full ? … : …)}`) and before the progress line, add:

```tsx
                {page && listPage && !full && !skipped && (
                    <div role="group" aria-label="View"><Segmented options={VIEWS} value={view} onChange={setView}/></div>
                )}
```

(`Segmented` shows a visible label when given `label`; the wrapper names the group for screen readers instead, as `Review.tsx` does for its period switch.)

- [ ] **Step 3: Hide From yesterday in Standup without unmounting it.** Replace the `CarryBox` block with:

```tsx
            {/* Hidden, not unmounted, in Standup: lines already kept or dropped must not be offered again on the way back. */}
            {listPage && !full && page?.carry && !skipped && (
                <div hidden={view !== 'list'}>
                    <CarryBox key={day} day={day} card={page.carry} onAdd={(t, m) => logRef.current?.addLine(t, m)} onToast={onToast}/>
                </div>
            )}
```

- [ ] **Step 4: Pass the view to the list editor.** In the `<LogEditor …>` element, add `view={view}` after `autoFocus={day === today}`.

- [ ] **Step 5: Type check**

Run: `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`
Expected: `tsc ok`.

- [ ] **Step 6: Commit** (Tasks 2 and 3 together)

```bash
git add desktop/frontend/src/log/LogEditor.tsx desktop/frontend/src/screens/DayPage.tsx desktop/frontend/src/app.css
git commit -m "day page: List | Standup switch and Copy standup"
```

---

### Task 4: Check it in the browser mock

**Files:** none. If a check fails, fix it in the files above and commit the fix with a message that says what it fixed.

- [ ] **Step 1: Open the mock.** `preview_start` with `{name: "gloamlog-mock"}`. Today is an empty list page: the title says "Nothing yet", a List | Standup switch sits under it with List checked, and From yesterday offers "Waiting on the API keys from ops" (blocked) and "Write the migration test" (next).

- [ ] **Step 2: Standup on an empty day.** Click Standup. Expected: "Nothing on this day yet.", no add field, no From yesterday box. Click List: From yesterday is back with both lines.

- [ ] **Step 3: Fill today.** Click Keep all. In the add field type `Fixed the login redirect loop` and press Enter. The title reads "3 lines, 1 next, 1 blocked". Wait 3 s for the save to settle.

- [ ] **Step 4: Standup.** Click Standup. Expected, in this order: Done ("Fixed the login redirect loop"), Next ("Write the migration test"), Blocked ("Waiting on the API keys from ops"); no times anywhere. The "Saved" note does not appear (switching never writes).

- [ ] **Step 5: Copy standup.** Catch what the mock's `ReviewCopyText` writes, then click the button, with `javascript_tool`:

```js
window.__copied = null;
const cb = navigator.clipboard ?? (Object.defineProperty(navigator, 'clipboard', {value: {}}), navigator.clipboard);
cb.writeText = t => { window.__copied = t; return Promise.resolve(); };
document.querySelector('.standup .btn').click();
await new Promise(r => setTimeout(r, 300));
[window.__copied, document.querySelector('.standup .btn').textContent]
```

Expected: `["Done\n- Fixed the login redirect loop\nNext\n- Write the migration test\nBlocked\n- Waiting on the API keys from ops", "Copied ✓"]`.

- [ ] **Step 6: Edits show up, and survive the switch.** Click List. Edit "Fixed the login redirect loop" to end with ` #next`, press Esc. Click Standup: it is now under Next and there is no Done group. Click List: the line shows the next mark; ⌘Z undoes the edit (undo history survived the switch); From yesterday does not come back (both lines were kept).

- [ ] **Step 7: Yesterday and a classic day.** Click Standup, click outside any text field, press ⌘[ until the most recent weekday before today: it opens in Standup with its four lines (two Done, one Next, one Blocked). Keep pressing ⌘[ until a weekday four or more days back (a classic page in the mock): no switch, the classic editor shows. ⌘T back to Today: still Standup.

- [ ] **Step 8: Edit whole page.** On Today in Standup, click Edit whole page: the switch goes and Milkdown shows the page. Back to the list: Standup again, same lines.

- [ ] **Step 9: Screenshot** the Standup view for the owner (`computer` `{action: "screenshot"}`), and look at it once in dark mode (`resize_window` `{colorScheme: "dark"}`, then `{colorScheme: "light"}`).

---

### Task 5: Full check, docs, status

**Files:**
- Modify: `docs/port/STATUS.md`, `docs/superpowers/specs/2026-10-18-writing-redesign-design.md`

- [ ] **Step 1: Every check**

```bash
(cd desktop/frontend && npm test 2>&1 | grep -E "^ℹ (pass|fail)"; node_modules/.bin/tsc --noEmit -p . && echo tsc ok)
git diff --stat HEAD~2 -- '*.go'
```

Expected: `ℹ fail 0`, `tsc ok`, and no Go file in the diff (this piece changes no Go, so the Go tests are unaffected).

- [ ] **Step 2: Docs**
  - `docs/port/STATUS.md`, under "Built", after the piece 2 line: ``- **Writing redesign, piece 3 (standup)** ([plan](../superpowers/plans/2026-10-19-writing-redesign-3-standup.md)): List | Standup switch on a list page (Done / Next / Blocked, times dropped; kept across ⌘[ / ⌘]) and Copy standup (plain text, images left out).``
  - The spec's status line (line 3): `Status: approved 2026-10-18. Pieces 1, 2 and 3 built; 4 (commits) next.` (keep the rest of the paragraph).
  - The spec, section 5: in the `desktop/notes_*.go` bullet, delete `` `Standup(day)`, ``. After the `desktop/frontend/src/screens/LogPage.tsx` bullet, add: ``- The standup's grouping and copy text are `standup` / `standupText` in `desktop/frontend/src/log/model.ts`: they group the lines the list editor holds, so an edit not yet saved is in the standup too.``

- [ ] **Step 3: Commit**

```bash
git add docs
git commit -m "docs: writing redesign piece 3 status; where the standup lives"
```

---

## After this plan

Piece 4 (commits added at their time with `InsertTimed`, replacing the suggestions strip on list pages) gets its own plan. The installed app picks this up on the next `desktop/build.sh` and reinstall; the real-app check of Copy standup (Go's clipboard write through `ReviewCopyText`) is the owner's on that build.
