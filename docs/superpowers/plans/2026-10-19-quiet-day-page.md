# A quieter list page: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Declutter the list day page as the [spec](../specs/2026-10-19-quiet-day-page-design.md) says: a two-row head,
a one-line From yesterday, marks only on next / blocked lines, two-row long lines, one add field with a list chip.

**Architecture:** Frontend only (`desktop/frontend/src`). The model (`log/model.ts`) and Go are untouched, so the 21
model tests guard behaviour; each task is checked with `tsc` and in the browser mock (`npm run dev:mock`, preview
`gloamlog-mock`, port 9247). No new unit tests: nothing here is pure logic.

**Tech stack:** React 19 + TypeScript, `@personal/ui` (tokens, `Display`, `Button`, `Segmented`, `Kbd`), app CSS in
`src/app.css` (tokens only, DESIGN.md §2).

**Checks used by every task** (run from `desktop/frontend`):

```bash
npx tsc --noEmit -p . && node --test src/log/model.test.ts
```

Expected: no tsc output, `# pass 21`.

Batches with owner checkpoints: Tasks 1-3, then 4-5, then 6.

---

### Task 1: The head

**Files:** Modify `src/screens/DayPage.tsx` (the `<header className="page-head">` block), `src/app.css` (`.page-head`).

- [ ] **Step 1: Split the head into a list head and the classic head.** Replace the whole `<header className="page-head">…</header>` with:

```tsx
            {listPage && !full ? (
                <header className="page-head page-head-list">
                    <p className="label" aria-live="polite">
                        {[label, page?.title].filter(Boolean).join(' · ')}
                        {recentlySaved && <span className="page-saved"> · Saved</span>}
                    </p>
                    {page && (
                        <div className="page-title-row">
                            <Display key={day} size="sm" lines={[summary || 'Nothing yet']}/>
                            {!skipped && <div role="group" aria-label="View"><Segmented options={VIEWS} value={view} onChange={setView}/></div>}
                        </div>
                    )}
                    {page && !skipped && words < Math.max(1, minWords) && (
                        <p className="small muted">{plural(words, 'word')}. Counts as logged at {minWords}.</p>
                    )}
                </header>
            ) : (
                <header className="page-head">
                    {label && <p className="label">{label}</p>}
                    {page && <Display key={day} lines={[weekday, rest.join(' ')]}/>}
                    {page && !skipped && <p className="small muted page-progress" aria-live="polite">
                        {words >= Math.max(1, minWords) ? `Logged, ${plural(words, 'word')}` : `${plural(words, 'word')}. Counts as logged at ${minWords}.`}
                        {recentlySaved && <span className="page-saved"> · Saved</span>}
                    </p>}
                </header>
            )}
```

- [ ] **Step 2: Styles.** After `.page-saved { … }` in `src/app.css` add:

```css
.page-head-list { gap: var(--s2); }
.page-title-row { display: flex; align-items: center; justify-content: space-between; gap: var(--s4); flex-wrap: wrap; }
```

- [ ] **Step 3: Checks** (above). Expected: pass.
- [ ] **Step 4: Commit** `git commit -am "day page: a two-row head on list pages"` (with the Co-Authored-By line).

### Task 2: Rows: no rules, quiet done marks, two-row long lines

**Files:** Modify `src/log/LogEditor.tsx` (`LogRow`), `src/app.css` (`.log-list`, `.log-row`, new rules).

- [ ] **Step 1: Row classes.** In `LogRow`, after `const shown = …`, add:

```tsx
    // Two rows at most until opened; an image line and a page that can't be opened for editing always show in full.
    const clamp = !readOnly && !editing && !/!\[[^\]]*\]\(/.test(line.text);
```

and replace `<li className={`log-row${editing ? ' is-editing' : ''}`}>` with:

```tsx
        <li className={`log-row${editing ? ' is-editing' : ''}${line.mark ? '' : ' is-done'}${clamp ? ' is-clamped' : ''}`}>
```

- [ ] **Step 2: Styles.** In `src/app.css` replace the `.log-list` and `.log-row` rules with:

```css
.log-list { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--s3); }
.log-row { display: grid; grid-template-columns: var(--s7) minmax(0, 1fr) auto; gap: var(--s3); align-items: baseline; padding: var(--s1) 0; }
```

and after `.log-mark.mark-blocked { … }` add:

```css
/* A done line's mark is the default: shown only on hover, focus or while editing (still in the tab order). */
.log-row.is-done .log-mark { opacity: 0; transition: opacity var(--fast) var(--ease); }
.log-row.is-done:hover .log-mark, .log-row.is-done:focus-within .log-mark, .log-row.is-editing .log-mark { opacity: 1; }
.log-row.is-clamped .log-text { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; }
.log-row.is-clamped .log-text p { display: inline; }
.log-row.is-clamped .log-text p + p::before { content: " "; }
```

- [ ] **Step 3: Checks.** Expected: pass.
- [ ] **Step 4: Commit** `"list page: no row rules, done marks on hover, long lines in two rows"`.

### Task 3: List titles and empty lists

**Files:** Modify `src/log/LogEditor.tsx` (the section map in `LogEditor`'s return), `src/app.css` (`.log`).

- [ ] **Step 1: An empty list says so.** In the section map, replace the `<ul className="log-list" …>…</ul>` with:

```tsx
                    {name && !state.lines.some(l => M.sec(l) === si)
                        ? <p className="small muted">Nothing yet.</p>
                        : <ul className="log-list" aria-label={name ? `${name} lines` : 'Lines'}>
                            {state.lines.map((l, i) => (M.sec(l) !== si && name ? null :
                                <LogRow key={i} line={l} editing={state.edit === i} draft={state.draft} html={html[M.renderIndex(state, i)] ?? ''}
                                        readOnly={readOnly} onEdit={() => act(s => M.focus(s, i))} onDraft={d => act(s => M.setDraft(s, d))}
                                        onKey={e => onKey(e, i)} onPaste={onPaste} onBlur={() => act(M.commit)} onCycle={() => act(s => M.setMark(s, i, nextMark(l.mark)))}
                                        onTime={t => act(s => M.setTime(s, i, t))}/>
                            ))}
                        </ul>}
```

- [ ] **Step 2: Space between lists.** In `src/app.css` replace `.log { display: grid; gap: var(--s4); }` with:

```css
.log { display: grid; gap: var(--s6); }
.log-section { display: grid; gap: var(--s2); }
```

- [ ] **Step 3: Checks.** Expected: pass.
- [ ] **Step 4: Commit** `"list page: room between the lists; an empty list says Nothing yet"`.

**Checkpoint 1:** browser mock, today's split page: two-row head; no rules; a done line's mark appears on hover and on Tab;
a long line is two rows and opens in full on click; the image line in the mock is not clamped; an empty list says
"Nothing yet."; an unlogged day shows the word-count line. Screenshot to the owner.

### Task 4: One add field with a list chip

**Files:** Modify `src/log/LogEditor.tsx` (`AddField`, the section map, a settings read), `src/app.css` (`.log-add`).

- [ ] **Step 1: Read the quick-notes list.** In `LogEditor`, after `const targetOf = …`, add:

```tsx
    // The add field starts on the list quick notes go to (Settings > Page); Work when settings can't be read.
    const [jots, setJots] = useState('Work');
    useEffect(() => { Notes.Settings().then(s => setJots(s.lists?.jots || 'Work'), () => {}); }, []);
```

- [ ] **Step 2: Replace `AddField`** with:

```tsx
function AddField({lists, start, autoFocus, onAdd}: {
    lists: string[]; start: string; autoFocus: boolean; onAdd: (text: string, mark: M.Mark, list: number) => void;
}) {
    const [text, setText] = useState('');
    const [picked, setPicked] = useState(-1); // the list chosen with the chip while the page is open; -1: start's
    const [focused, setFocused] = useState(false);
    const at = picked >= 0 ? picked : Math.max(0, lists.indexOf(start));
    const other = (at + 1) % lists.length;
    const add = (mark: M.Mark) => { if (!text.trim()) return; onAdd(text, mark, at); setText(''); };
    return (
        <div className="log-add">
            <label className="sr-only" htmlFor="log-add">{lists.length > 1 ? `Add a line to ${lists[at]}` : 'Add a line'}</label>
            <div className="log-add-field">
                <input id="log-add" className="input" placeholder="What did you just do?" autoFocus={autoFocus} value={text}
                       onChange={e => setText(e.target.value)} onFocus={() => setFocused(true)} onBlur={() => setFocused(false)}
                       onKeyDown={e => {
                           const mark = markKey(e);
                           if (mark !== null) { e.preventDefault(); e.stopPropagation(); add(mark); }
                           else if (e.key === 'Enter' && !e.nativeEvent.isComposing) { e.preventDefault(); add(''); }
                       }}/>
                {lists.length > 1 && (
                    <button type="button" className="badge log-add-list" aria-label={`Adds to ${lists[at]}. Switch to ${lists[other]}`}
                            onMouseDown={e => e.preventDefault() /* keep typing */} onClick={() => setPicked(other)}>{lists[at]} ▾</button>
                )}
            </div>
            {focused && <p className="small muted log-keys"><Kbd>↩</Kbd> add · <Kbd>⌘1</Kbd> done <Kbd>⌘2</Kbd> next <Kbd>⌘3</Kbd> blocked · <Kbd>⇧↩</Kbd> a second row</p>}
        </div>
    );
}
```

- [ ] **Step 3: One field after the lists.** In the section map delete the per-section
`{!readOnly && <AddField id={`log-add-${si}`} … />}` and, after the map's closing `))}` (still inside `<div className="log">`), add:

```tsx
            {!readOnly && <AddField lists={lists} start={jots} autoFocus={autoFocus}
                                    onAdd={(text, mark, si) => act(s => M.append(s, text, now(), mark, si))}/>}
```

Update the comment above `const lists = …` to: `// A split page shows each list under its name; one add field below them adds to the list its chip names.`

- [ ] **Step 4: Styles.** In `src/app.css` replace `.log-add { … }` with:

```css
.log-add { display: grid; gap: var(--s1); }
.log-add-field { position: relative; }
.log-add-field .input { width: 100%; padding-right: var(--s9); }
.log-add-list { position: absolute; right: var(--s2); top: 50%; transform: translateY(-50%); background: none; color: var(--muted); cursor: pointer; }
```

- [ ] **Step 5: Checks.** Expected: pass.
- [ ] **Step 6: Commit** `"list page: one add field with a Work / Personal chip; key hints while typing"`.

### Task 5: From yesterday as one line

**Files:** Modify `src/log/CarryBox.tsx` (the return), `src/app.css` (`.carry-box` block).

- [ ] **Step 1: Collapsed by default.** Add `const [open, setOpen] = useState(false);` beside the other state, and
replace the returned `<section className="carry-box" …>…</section>` with:

```tsx
        <section className="carry-box" aria-label={`From ${card.sourceLabel}`}>
            <div className="carry-head">
                <p className="small muted">{left.length} open from {card.sourceLabel}</p>
                <div className="row-gap carry-actions">
                    {open && <Button variant="ghost" size="sm" onClick={() => { left.forEach(it => onAdd(it.text, it.mark, it.list)); setLeft([]); }}>Keep all</Button>}
                    {open && <Button variant="ghost" size="sm" onClick={() => setHidden(true)}>Hide</Button>}
                    <Button variant="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(o => !o)}>{open ? 'Close' : 'Review'}</Button>
                </div>
            </div>
            {open && (
                <ul className="carry-list">
                    {left.map((it, i) => (
                        <li key={it.text} className="carry-row">
                            <span>{it.text}{it.mark && <> <span className={`badge log-mark mark-${it.mark}`}>{it.mark}</span></>}</span>
                            <span className="row-gap carry-actions">
                                <Button variant="ghost" size="sm" onClick={() => done(i)}>Done</Button>
                                <Button variant="ghost" size="sm" onClick={() => keep(i)}>Keep</Button>
                                <Button variant="ghost" size="sm" onClick={() => drop(i)}>Drop</Button>
                            </span>
                        </li>
                    ))}
                </ul>
            )}
        </section>
```

- [ ] **Step 2: Styles.** Replace the three `.carry-box` / `.carry-head, .carry-row` / `.carry-list` rules with:

```css
.carry-box { display: grid; gap: var(--s2); padding: var(--s2) 0; margin-bottom: var(--s5); border-top: 1px solid var(--rule); border-bottom: 1px solid var(--rule); }
.carry-head, .carry-row { display: flex; align-items: baseline; justify-content: space-between; gap: var(--s3); }
.carry-actions { flex-shrink: 0; flex-wrap: nowrap; }
.carry-list { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--s2); }
```

- [ ] **Step 3: Checks.** Expected: pass.
- [ ] **Step 4: Commit** `"list page: From yesterday is one line until Review"`.

**Checkpoint 2:** browser mock: the chip starts on Work, switches to Personal, and a line added lands in Personal; the
hints show only while the field has focus; Enter in a line still adds in that line's list; a `## Log` day has no chip;
From yesterday: collapsed line, Review, Done / Keep / Drop (no wrapping), Keep all, Hide; Standup unchanged; dark theme.
Screenshot to the owner.

### Task 6: Docs and build

**Files:** Modify `docs/port/STATUS.md` (Built list), `docs/superpowers/specs/2026-10-18-writing-redesign-design.md`
(status line).

- [ ] **Step 1: STATUS.md.** After the piece 4 bullet add:

```markdown
- **A quieter list page** ([spec](../superpowers/specs/2026-10-19-quiet-day-page-design.md), [plan](../superpowers/plans/2026-10-19-quiet-day-page.md)): a two-row head, From yesterday as one line until Review, marks only on next / blocked lines (a done line's on hover), long lines in two rows until opened, and one add field with a Work / Personal chip.
```

- [ ] **Step 2: Redesign spec status.** Append to its status paragraph: `The list page was made quieter on 2026-10-19
  ([2026-10-19-quiet-day-page-design.md](2026-10-19-quiet-day-page-design.md)).`
- [ ] **Step 3: Commit** `"docs: quieter list page status"`.
- [ ] **Step 4: Build** in the owner's terminal: `cd desktop && go test . && bash build.sh`. Install only on the
  owner's yes (quit Gloamlog, old app to the Trash, copy, open).
