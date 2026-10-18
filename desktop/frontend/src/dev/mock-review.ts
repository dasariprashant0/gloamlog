// Mock additions for the Review screen (merged into Notes in mock.ts as `...reviewMock`). Same names and shapes as
// desktop/notes_review.go. Plain methods, not arrows: they read the days through `this` (the Notes object they are merged into).
// It builds a simplified review (the real rules are in internal/core): good enough to see every state of the screen.
const pad = (n: number) => String(n).padStart(2, '0');
const key = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const parse = (k: string) => { const [y, m, d] = k.split('-').map(Number); return new Date(y, m - 1, d); };
const add = (k: string, n: number) => { const d = parse(k); d.setDate(d.getDate() + n); return key(d); };
const fmt = (k: string, o: Intl.DateTimeFormatOptions) => parse(k).toLocaleDateString('en-GB', o);
const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`;
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const inline = (s: string) => esc(s).replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 60));

/** A tiny markdown to HTML: headings, bullets, to-dos and paragraphs. */
function html(md: string): string {
    const out: string[] = [];
    let list = false;
    for (const raw of md.split('\n')) {
        const l = raw.trimEnd();
        const bullet = /^\s*[-*] (\[[ x]\] )?(.*)$/.exec(l);
        if (!bullet && list) { out.push('</ul>'); list = false; }
        if (bullet) {
            if (!list) { out.push('<ul>'); list = true; }
            const task = bullet[1] ? ` class="task">${bullet[1].includes('x') ? '☑' : '☐'} ` : '>';
            out.push(`<li${task}${inline(bullet[2])}</li>`);
        } else if (/^#{1,6} /.test(l)) { const n = l.indexOf(' '); out.push(`<h${n}>${inline(l.slice(n + 1))}</h${n}>`); }
        else if (l.trim()) out.push(`<p>${inline(l)}</p>`);
    }
    if (list) out.push('</ul>');
    return out.join('');
}

function period(req: any, today: string) {
    const scale: string = req.scale;
    const anchor = req.day || today;
    const monday = (k: string) => add(k, -((parse(k).getDay() + 6) % 7));
    const at = (k: string) => {
        if (scale === 'month') { const d = parse(k); return {start: key(new Date(d.getFullYear(), d.getMonth(), 1)), end: key(new Date(d.getFullYear(), d.getMonth() + 1, 0))}; }
        if (scale === 'year') return {start: `${k.slice(0, 4)}-01-01`, end: `${k.slice(0, 4)}-12-31`};
        return {start: monday(k), end: add(monday(k), 6)};
    };
    let p = scale === 'range'
        ? (() => { let a = req.from || add(today, -6), b = req.to || today; if (b < a) [a, b] = [b, a]; return {start: a, end: b < today ? b : today}; })()
        : at(anchor);
    if (req.shift) {
        const d = parse(p.start);
        if (scale === 'week') p = at(add(p.start, 7 * req.shift));
        else if (scale === 'month') p = at(key(new Date(d.getFullYear(), d.getMonth() + req.shift, 1)));
        else if (scale === 'year') p = at(key(new Date(d.getFullYear() + req.shift, 0, 1)));
        else { const n = Math.round((parse(p.end).getTime() - d.getTime()) / 86_400_000) + 1; p = {start: add(p.start, n * req.shift), end: add(p.end, n * req.shift)}; if (p.end > today) p.end = today; }
    }
    return {scale, ...p};
}

const statusWord: Record<string, string> = {logged: 'logged', partial: 'started', missed: 'not logged', skipped: 'skipped', off: 'not a working day', future: 'upcoming'};

export const reviewMock = {
    async Review(this: any, req: any) {
        const t: string = (await this.Overview()).today;
        const p = period(req, t);
        const all: string[] = [];
        for (let d = p.start; d <= p.end; d = add(d, 1)) all.push(d);
        const pages = new Map<string, any>((await Promise.all(all.map(async d => [d, await this.Open(d)] as const))));
        const counts = {logged: 0, skipped: 0, started: 0, unlogged: 0};
        const missing: string[] = [];
        for (const d of all) {
            if (d > t) continue;
            const s = pages.get(d).status;
            if (s === 'logged') counts.logged++;
            else if (s === 'skipped' && req.includeSkipped) counts.skipped++;
            else if ((s === 'partial' || s === 'missed') && d < t) { if (s === 'partial') counts.started++; else counts.unlogged++; missing.push(d); }
        }
        const notLogged = counts.started + counts.unlogged;
        const total = counts.logged + counts.skipped + notLogged;
        const countsLine = total ? `Logged ${counts.logged} of ${total}${counts.skipped ? ` · ${counts.skipped} skipped` : ''}${notLogged ? ` · ${notLogged} not logged` : ''}` : 'Nothing to count yet.';

        const days: any[] = [];
        const groups = new Map<string, any[]>();
        if (p.scale !== 'year') {
            for (const d of all) {
                const pg = pages.get(d);
                if (pg.isSkipped) {
                    if (req.includeSkipped) days.push({day: d, label: fmt(d, {weekday: 'short', day: 'numeric', month: 'short'}), spoken: fmt(d, {weekday: 'long', day: 'numeric', month: 'long'}), status: 'skipped', words: 0, skipReason: pg.skipReason, html: ''});
                    continue;
                }
                if (!pg.exists || pg.words === 0) continue;
                days.push({day: d, label: fmt(d, {weekday: 'short', day: 'numeric', month: 'short'}), spoken: fmt(d, {weekday: 'long', day: 'numeric', month: 'long'}), status: pg.status, words: pg.words, skipReason: '', html: html(pg.body)});
                let title = 'Other notes';
                for (const chunk of (pg.body as string).split(/^## /m).filter(Boolean)) {
                    const [h, ...rest] = chunk.split('\n');
                    const text = rest.join('\n').trim();
                    if (!text) continue;
                    title = h.trim();
                    groups.set(title, [...(groups.get(title) ?? []), {day: d, label: fmt(d, {weekday: 'short', day: 'numeric'}), text, html: html(text)}]);
                }
            }
        }
        const months = p.scale !== 'year' ? [] : await Promise.all(Array.from({length: 12}, async (_, i) => {
            const month = `${p.start.slice(0, 4)}-${pad(i + 1)}`;
            const first = `${month}-01`, last = key(new Date(Number(month.slice(0, 4)), i + 1, 0));
            const c = {logged: 0, skipped: 0, started: 0, unlogged: 0};
            for (let d = first; d <= last && d < t; d = add(d, 1)) {
                const s = pages.get(d)?.status;
                if (s === 'logged') c.logged++; else if (s === 'skipped' && req.includeSkipped) c.skipped++; else if (s === 'partial') c.started++; else if (s === 'missed') c.unlogged++;
            }
            const future = first > t, n = c.started + c.unlogged, inert = future || c.logged + c.skipped + n === 0;
            const name = fmt(first, {month: 'long'});
            const line = inert ? '–' : [`${c.logged} logged`, c.skipped && `${c.skipped} skipped`, n && `${n} not logged`].filter(Boolean).join(' · ');
            return {month, name, line, counts: c, isFuture: future, inert, spoken: `${name}, ${inert ? (future ? 'not started' : 'nothing to count') : line}`};
        }));
        const tiles = p.scale !== 'week' ? [] : all.map(d => {
            const pg = pages.get(d);
            const bottom = pg.status === 'logged' || pg.status === 'partial' ? plural(pg.words, 'word') : pg.status === 'skipped' ? (pg.skipReason || 'Skipped') : pg.status === 'missed' ? 'Write' : '';
            return {day: d, weekday: fmt(d, {weekday: 'short'}), number: Number(d.slice(8)), status: pg.status, bottom, isToday: d === t, opens: d <= t,
                spoken: `${fmt(d, {weekday: 'long', day: 'numeric', month: 'long'})}, ${statusWord[pg.status]}`};
        });
        const empty = p.scale === 'year' ? months.every((m: any) => m.inert) : days.length === 0;
        const title = p.scale === 'week' ? `Week of ${fmt(p.start, {day: 'numeric'})} to ${fmt(p.end, {day: 'numeric', month: 'short', year: 'numeric'})}`
            : p.scale === 'month' ? fmt(p.start, {month: 'long', year: 'numeric'}) : p.scale === 'year' ? p.start.slice(0, 4)
            : `${fmt(p.start, {day: 'numeric', month: 'short'})} to ${fmt(p.end, {day: 'numeric', month: 'short', year: 'numeric'})}`;
        const tasks = days.reduce((n, d) => n + ((d.html as string).match(/☑/g)?.length ?? 0), 0);
        return later({
            period: p, title, fileName: `Gloamlog ${p.start}`, countsLine, tasksLine: tasks ? `${plural(tasks, 'task')} done` : '', counts, notLogged,
            empty, emptyText: p.scale === 'week' ? 'Nothing logged this week yet.' : p.scale === 'month' ? `Nothing logged in ${fmt(p.start, {month: 'long'})} yet.`
                : p.scale === 'year' ? `No pages in ${p.start.slice(0, 4)} yet.` : 'Nothing logged in this range yet.',
            canPrev: true, canNext: p.end < t, isCurrent: p.start <= t && t <= p.end, today: t, minWords: 20,
            catchUpLabel: p.scale === 'week' ? 'Catch up this week' : p.scale === 'month' ? 'Catch up this month' : 'Catch up this range',
            missingDays: missing, tiles, days, groups: [...groups].map(([title, items]) => ({title: title.replace(/^[^\p{L}\p{N}]+/u, ''), kind: title === 'Other notes' ? 'other' : 'heading', items})), months,
        });
    },
    async ReviewText(this: any, req: any, flavor: string) {
        const d = await this.Review(req);
        const md = `# ${d.title}\n\n${d.countsLine}\n\n` + (req.bySection
            ? d.groups.map((g: any) => `## ${g.title}\n\n${g.items.map((i: any) => `- **${i.label}**: ${i.text.split('\n')[0]}\n`).join('')}`).join('\n')
            : d.days.map((x: any) => `## ${x.label}\n\n${x.status === 'skipped' ? `_Skipped: ${x.skipReason}_` : x.html.replace(/<[^>]+>/g, ' ')}\n`).join('\n'));
        if (flavor === 'plain') return later({title: d.title, text: md.replace(/^#+ /gm, '').replace(/\*\*/g, ''), html: ''});
        if (flavor === 'rich') return later({title: d.title, text: md, html: `<html><body>${html(md)}</body></html>`});
        if (flavor === 'print') return later({title: d.title, text: '', html: html(md)});
        return later({title: d.title, text: md, html: ''});
    },
    ReviewCopyText: (text: string) => { navigator.clipboard?.writeText(text).catch(() => {}); return later(undefined); },
    async ReviewExportInfo(this: any, req: any) {
        const d = await this.Review(req);
        const days = d.period.scale === 'year' ? d.counts.logged : d.days.length;
        return later({name: d.fileName, days, images: 0,
            detail: `${plural(days, 'day')}. Skipped days ${req.includeSkipped ? 'included' : 'left out'}. Jots ${req.includeJots ? 'included' : 'left out'}.`});
    },
    // The picker answers with a made-up place; a folder named "Locked" makes the write fail so the failure banner can be tried.
    async ReviewChoosePlace(this: any, req: any, kind: string) {
        const d = await this.Review(req);
        return later(kind === 'folder' ? '/Users/someone/Documents' : `/Users/someone/Documents/${d.fileName}${kind === 'zip' ? '.zip' : '.md'}`);
    },
    async ReviewWrite(this: any, req: any, kind: string, path: string) {
        const d = await this.Review(req);
        if (path.includes('Locked')) throw new Error('permission denied');
        const days = d.period.scale === 'year' ? d.counts.logged : d.days.length;
        return later({line: `Saved ${plural(days, 'day')}.`, path: kind === 'folder' ? `${path}/${d.fileName}` : path});
    },
};
