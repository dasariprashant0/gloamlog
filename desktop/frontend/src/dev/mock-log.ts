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
