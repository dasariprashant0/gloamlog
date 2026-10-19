// Stand-ins for the list editor's Go methods (desktop/notes_log.go, notes_carry.go) in `npm run dev:mock`. A rough copy,
// good enough to drive the screen in a browser: the real rules for the file live in internal/core/loglist.go.
import type {Line, Mark} from '../log/model';

export type MockSection = {name: string; join: string; trail: number; virtual: boolean};
export type MockPage = {before: string; lines: Line[]; sections: MockSection[]; trail: number; after: string};
const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 30));
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
export const dismissed = new Set<string>(); // "day|text"
const LISTS: Record<string, string> = {log: 'Log', work: 'Work', personal: 'Personal'};
const level = (r: string) => /^(#{1,6})\s/.exec(r)?.[1].length ?? 0;

function readLines(rows: string[], section: number): {lines: Line[]; trail: number} {
    const lines: Line[] = [];
    let blank = 0;
    for (const r of rows) {
        if (!r.trim()) { blank++; continue; }
        const m = /^[-*+]\s+(\[( |x|X)\]\s+)?((\d{1,2}:\d{2})\s+)?(.*)$/.exec(r);
        if (!m) { lines.push({time: '', text: r, mark: '', task: false, other: true, blank, raw: r, section}); blank = 0; continue; }
        let text = m[5];
        let mark: Mark = m[2] === ' ' ? 'next' : '';
        const cut = /(^|\s)#(next|blocked)\s*$/i.exec(text);
        if (cut) { mark = cut[2].toLowerCase() as Mark; text = text.slice(0, cut.index).trimEnd(); }
        lines.push({time: m[4] ?? '', text, mark, task: !!m[1], other: false, blank, raw: r, section});
        blank = 0;
    }
    return {lines, trail: blank};
}

export function parseLog(body: string): {page: MockPage; ok: boolean} {
    const rows = body.split('\n');
    const blocks: {at: number; end: number; name: string; lvl: number}[] = [];
    rows.forEach((r, i) => {
        const m = /^(#{1,6})\s+(log|work|personal)\s*$/i.exec(r.trim());
        if (!m || (blocks.length && i < blocks[blocks.length - 1].end)) return; // a list heading inside a list is a line of it
        const lvl = m[1].length;
        let end = rows.findIndex((x, k) => k > i && level(x) > 0 && level(x) <= lvl);
        if (end < 0) end = rows.length;
        blocks.push({at: i, end, name: LISTS[m[2].toLowerCase()], lvl});
    });
    if (!blocks.length) return {page: {before: '', lines: [], sections: [], trail: 0, after: ''}, ok: false};
    const page: MockPage = {before: rows.slice(0, blocks[0].at + 1).join('\n'), lines: [], sections: [], trail: 0, after: ''};
    blocks.forEach((b, i) => {
        const {lines, trail} = readLines(rows.slice(b.at + 1, b.end), i);
        page.lines.push(...lines);
        page.sections.push({name: b.name, join: i ? rows.slice(blocks[i - 1].end, b.at + 1).join('\n') : '', trail, virtual: false});
    });
    const last = blocks[blocks.length - 1];
    page.after = last.end < rows.length ? rows.slice(last.end).join('\n') : '';
    page.trail = page.sections[page.sections.length - 1].trail;
    const names = blocks.map(b => b.name);
    if (names.includes('Work') !== names.includes('Personal')) {
        const missing = names.includes('Work') ? 'Personal' : 'Work';
        page.sections.push({name: missing, join: '#'.repeat(blocks.find(b => b.name !== 'Log')!.lvl) + ' ' + missing, trail: 0, virtual: true});
    }
    return {page, ok: true};
}

function lineMd(l: Line): string {
    if (l.other) return l.text;
    const box = l.task ? (l.mark ? '[ ] ' : '[x] ') : '';
    const mark = l.mark === 'blocked' || (l.mark === 'next' && !l.task) ? ` #${l.mark}` : '';
    return `- ${box}${l.time ? l.time + ' ' : ''}${l.text}${mark}`.trimEnd();
}

function render(page: MockPage) {
    const out = [page.before];
    let wroteVirtual = false;
    page.sections.forEach((s, i) => {
        const mine = page.lines.filter(l => (l.section ?? 0) === i);
        if (s.virtual) {
            if (!mine.length) return;
            if (out[out.length - 1] !== '') out.push('');
            out.push(s.join);
            wroteVirtual = true;
        } else if (i) out.push(s.join);
        for (const l of mine) { for (let k = 0; k < l.blank; k++) out.push(''); out.push(lineMd(l)); }
        for (let k = 0; k < s.trail; k++) out.push('');
    });
    if (page.after) { if (wroteVirtual && out[out.length - 1] !== '') out.push(''); out.push(page.after); }
    const words = page.lines.filter(l => !l.other).map(l => l.text).join(' ').split(/\s+/).filter(Boolean).length;
    return {markdown: out.join('\n'), words, html: page.lines.map(l => `<p>${esc(l.text)}</p>`)};
}

export const logMock = {
    LogParse: (body: string) => later(parseLog(body)),
    LogRender: (page: MockPage) => later(render(page)),
    LogAppendLines: (page: MockPage, markdown: string, list: string, _timed = false) => { // no mock path sends a timed line
        const section = Math.max(0, page.sections.findIndex(s => s.name === list));
        const have = new Set(page.lines.map(l => l.text.toLowerCase()));
        let empty = !page.lines.some(l => (l.section ?? 0) === section);
        const fresh = readLines(markdown.split('\n'), section).lines.filter(l => !l.other && !have.has(l.text.toLowerCase()))
            .map(l => { const out = {...l, blank: empty ? 1 : 0}; empty = false; return out; });
        const lines = [...page.lines, ...fresh];
        return later({lines, rendered: render({...page, lines}), result: fresh.length ? 'appended' : 'alreadyPresent'});
    },
    CarryDismiss: (day: string, text: string) => { dismissed.add(`${day}|${text.toLowerCase()}`); return later(undefined); },
};
