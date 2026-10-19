// The list editor's state as pure functions; `npm test` runs model.test.ts with Node's own runner (no dependencies, so only
// erasable TypeScript here: no enums, no namespaces). The lines come from Go (Notes.LogParse) and go back to it to be written
// (Notes.LogRender): nothing here writes markdown. A line's mark is edited as a trailing "#next" / "#blocked" in its draft,
// so typing or deleting the word works as well as ⌘1-3.

export type Mark = '' | 'next' | 'blocked';
/** One line, as Go's core.LogLine sends it. section: which list of the page (an index into LogPage.sections; 0 if absent). */
export type Line = {time: string; text: string; mark: Mark; task: boolean; other: boolean; blank: number; raw: string; section?: number};

/** The list a line is in. */
export const sec = (l: Line) => l.section ?? 0;

export type State = {
    lines: Line[];
    edit: number;      // the line being edited; -1 for none
    draft: string;     // that line's text while it is edited
    past: Line[][];    // undo
    future: Line[][];  // redo
};

const HISTORY = 200;

export const fresh = (lines: Line[]): State => ({lines, edit: -1, draft: '', past: [], future: []});

export const newLine = (time: string, text = '', mark: Mark = '', section = 0): Line =>
    ({time, text, mark, task: false, other: false, blank: 0, raw: '', section});

// Where a new last line of list `section` goes in the flat array: after its last line, else before the first later list.
function endOf(lines: Line[], section: number): number {
    let at = -1;
    lines.forEach((l, i) => { if (sec(l) === section) at = i + 1; });
    if (at >= 0) return at;
    const later = lines.findIndex(l => sec(l) > section);
    return later < 0 ? lines.length : later;
}

const hasList = (lines: Line[], section: number) => lines.some(l => sec(l) === section);

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

const isEmptyNew = (l: Line | undefined) => !!l && l.raw === '' && l.text === '' && !l.mark && !l.other;

// History never holds an empty new line: undo would bring it back as a real (saved) line.
function record(s: State, lines: Line[]): State {
    return {...s, lines, past: [...s.past, s.lines.filter(l => !isEmptyNew(l))].slice(-HISTORY), future: []};
}


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
    const section = lines[j] ? sec(lines[j]) : 0;
    lines.splice(at, 0, {...newLine(now, '', '', section), blank: hasList(lines, section) ? 0 : 1});
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
    const gone = s.lines[i];
    const wasFirst = s.lines.findIndex(l => sec(l) === sec(gone)) === i;
    const lines = s.lines.filter((_, k) => k !== i);
    const next = lines.findIndex(l => sec(l) === sec(gone));
    if (wasFirst && next >= 0) lines[next] = {...lines[next], blank: gone.blank}; // the list's new first line keeps the gap after its heading
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
export function append(s: State, text: string, now: string, mark: Mark = '', section = 0): State {
    const c = commit(s);
    const [t, m] = cutMark(text.trim());
    if (!t) return c;
    const lines = c.lines.slice();
    lines.splice(endOf(lines, section), 0, {...newLine(now, t, m ?? mark, section), blank: hasList(lines, section) ? 0 : 1});
    return record(c, lines);
}

/** A new time for line i: the line moves before the first line with a later time; untimed lines are passed over. */
export function setTime(s: State, i: number, time: string): State {
    const k = minutes(time);
    const [c, j] = afterCommit(s, i);
    const l = c.lines[j];
    if (!l || l.other || k === null || l.time === time) return c;
    const list = sec(l);
    const firstAt = c.lines.findIndex(x => sec(x) === list);
    const gap = c.lines[firstAt].blank;
    const oldFirst = c.lines[firstAt];
    const rest = c.lines.filter((_, x) => x !== j);
    // Within its own list: before the first later time, else at the end of the list.
    const later = rest.findIndex(x => { const xk = minutes(x.time); return sec(x) === list && xk !== null && xk > k; });
    rest.splice(later < 0 ? endOf(rest, list) : later, 0, {...l, time, blank: 0});
    // Only a list's first line carries the blank lines after its heading.
    const first = rest.findIndex(x => sec(x) === list);
    return record(c, rest.map((x, idx) => (idx === first ? {...x, blank: gap} : x === oldFirst ? {...x, blank: 0} : x)));
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

export type Group = 'done' | 'next' | 'blocked';
/** The standup groups in the order they are shown and copied. */
export const GROUPS: [Group, string][] = [['done', 'Done'], ['next', 'Next'], ['blocked', 'Blocked']];

/** The standup view: indexes into lines by group, in file order. Other blocks and empty lines are left out. */
// A line of the Personal list (names: the page's list names by section; none = a one-list page).
const isPersonal = (l: Line, names?: string[]) => names?.[sec(l)] === 'Personal';
const shows = (l: Line) => !l.other && !!l.text.trim();

export function standup(lines: Line[], names?: string[]): Record<Group, number[]> {
    const g: Record<Group, number[]> = {done: [], next: [], blocked: []};
    lines.forEach((l, i) => { if (shows(l) && !isPersonal(l, names)) g[l.mark || 'done'].push(i); });
    return g;
}

/** The Personal list's lines for the standup view (shown apart, never copied). */
export function personal(lines: Line[], names?: string[]): number[] {
    return lines.flatMap((l, i) => (shows(l) && isPersonal(l, names) ? [i] : []));
}

// A line as standup text: images dropped (they mean nothing pasted into a chat), continuation rows indented under it.
const plain = (text: string) =>
    text.replace(/!\[[^\]]*\]\([^)]*\)/g, '').split('\n').map(r => r.trim()).filter(Boolean).join('\n  ');

/** What Copy standup copies: each group that has lines, as its title and bullets; no times. */
export function standupText(lines: Line[], names?: string[]): string {
    const g = standup(lines, names);
    const out: string[] = [];
    for (const [key, title] of GROUPS) {
        const rows = g[key].map(i => plain(lines[i].text)).filter(Boolean);
        if (rows.length) out.push(title, ...rows.map(r => `- ${r}`));
    }
    return out.join('\n');
}
