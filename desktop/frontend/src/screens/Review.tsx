// The Review screen: week, month, year or a range, with copy and export. Every rule (counts, groups, formats, file names) is in
// the Go core; this file keeps the screen's own state: which period, what to include, and the result of the last save.
// What is shown is exactly what Copy and Save write (the requests below always name the period on screen).
import {useCallback, useEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {Banner, Button, Display, Empty, Segmented, Switch} from '@personal/ui/react';
import {Window} from '@wailsio/runtime';
import {Notes, addDays} from '../api';
import type {ReviewData, ReviewRequest} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import {ExportDialog, PDF_ENABLED, type CopyFlavor, type SaveKind} from './ExportDialog';
import {DayList, GroupList, REVIEW_CSS, WeekTiles, YearList} from './ReviewParts';

type Scale = 'week' | 'month' | 'year' | 'range';
type Grouping = 'day' | 'section';
type View = {scale: Scale; day: string; from: string; to: string};
type Options = {includeSkipped: boolean; includeJots: boolean; grouping: Grouping | null}; // grouping null = the scale's own default

const SCALES: [Scale, string][] = [['week', 'Week'], ['month', 'Month'], ['year', 'Year'], ['range', 'Range']];
const STORE = 'gloamlog.review';
const scaleOf = (p: {scale: unknown}) => p.scale as Scale;
const errText = (e: unknown) => (e instanceof Error ? e.message : String((e as {message?: string})?.message ?? e));

function loadOptions(): {scale: Scale; options: Options} {
    try {
        const s = JSON.parse(localStorage.getItem(STORE) ?? '{}');
        return {
            scale: SCALES.some(([k]) => k === s.scale) ? s.scale : 'week',
            options: {includeSkipped: s.includeSkipped !== false, includeJots: s.includeJots === true, grouping: s.grouping === 'day' || s.grouping === 'section' ? s.grouping : null},
        };
    } catch { return {scale: 'week', options: {includeSkipped: true, includeJots: false, grouping: null}}; } // storage off or damaged: defaults
}
function remember(scale: Scale, o: Options) {
    try { localStorage.setItem(STORE, JSON.stringify({scale, ...o})); } catch { /* storage can be off; the screen still works */ }
}

// The period the person was looking at survives leaving the screen and coming back.
let kept: View | null = null;

type Failure = {text: string; detail: string; kind: SaveKind; path: string};

export function Review({today, onOpenDay, onCatchUp, onToast}: {
    today: string;
    onOpenDay: (day: string) => void;
    /** Catch up, limited to these days (the unlogged days of the period on screen). */
    onCatchUp: (days: string[]) => void;
    onToast: (message: string, undo?: () => void) => void;
}) {
    const first = useRef(loadOptions());
    const [view, setView] = useState<View>(() => kept ?? {scale: first.current.scale, day: today, from: today, to: today});
    const [options, setOptions] = useState<Options>(first.current.options);
    const [raw, setRaw] = useState<ReviewData | null>(null);
    const [error, setError] = useState('');
    const [tick, setTick] = useState(0);
    const [menu, setMenu] = useState(false);
    const [exportOpen, setExportOpen] = useState(false);
    const [copied, setCopied] = useState(false);
    const [busy, setBusy] = useState('');
    const [saved, setSaved] = useState<{line: string; path: string} | null>(null);
    const [failure, setFailure] = useState<Failure | null>(null);
    const [print, setPrint] = useState<{title: string; html: string} | null>(null);
    // What was read, once it is for the scale that is selected (a change of scale shows Loading until the new one arrives).
    const data = raw && scaleOf(raw.period) === view.scale ? raw : null;
    const gen = useRef(0);
    const menuBox = useRef<HTMLDivElement>(null);

    const grouping: Grouping = options.grouping ?? (view.scale === 'week' ? 'day' : 'section');
    const reqOf = useCallback((v: View, shift = 0): ReviewRequest => ({
        scale: v.scale, day: v.day, from: v.from, to: v.to, shift,
        includeSkipped: options.includeSkipped, includeJots: options.includeJots, bySection: grouping === 'section',
    }), [options.includeSkipped, options.includeJots, grouping]);
    // Copies and exports name the period that is on screen, so the file always matches what was read.
    const shown = (): ReviewRequest => {
        const p = data?.period;
        return reqOf(p ? {scale: scaleOf(p), day: p.start, from: p.start, to: p.end} : view);
    };

    // Read the review again when the period or what is included changes, when the day rolls over and when the window returns.
    useEffect(() => {
        kept = view;
        const mine = ++gen.current;
        Notes.Review(reqOf(view, 0)).then(d => {
            if (mine !== gen.current) return;
            setRaw(d); setError('');
            if (view.scale === 'range' && (d.period.start !== view.from || d.period.end !== view.to)) {
                setView(v => ({...v, from: d.period.start, to: d.period.end})); // the core capped or trimmed the range
            }
        }, e => { if (mine === gen.current) setError(errText(e)); });
    }, [view.scale, view.day, view.from, view.to, options.includeSkipped, options.includeJots, today, tick]); // eslint-disable-line react-hooks/exhaustive-deps
    useEffect(() => {
        const again = () => setTick(t => t + 1);
        window.addEventListener('focus', again);
        return () => window.removeEventListener('focus', again);
    }, []);
    useEffect(() => remember(view.scale, options), [view.scale, options]);

    const clearResult = () => { setSaved(null); setFailure(null); };
    function move(v: View) { clearResult(); setView(v); }

    // ---- moving around ----
    function setScale(s: Scale) {
        if (s === view.scale) return;
        const p = data?.period;
        if (s === 'range') move({scale: s, day: view.day, from: p?.start ?? view.day, to: p ? (p.end < today ? p.end : today) : today});
        else move({...view, scale: s, day: view.scale === 'range' && p ? p.end : view.day});
    }
    async function step(dir: number) {
        try {
            const d = await Notes.Review(reqOf(view, dir));
            move({scale: view.scale, day: d.period.start, from: d.period.start, to: d.period.end});
        } catch (e) { setError(errText(e)); }
    }
    const goThis = () => move({...view, day: today});
    const lastWeek = () => move({scale: 'week', day: addDays(today, -7), from: view.from, to: view.to});
    const thisMonth = () => move({scale: 'month', day: today, from: view.from, to: view.to});
    const openMonth = (m: string) => move({scale: 'month', day: `${m}-01`, from: view.from, to: view.to});
    // Either order is fine: moving one date past the other takes the other with it.
    function setRange(changed: 'from' | 'to', value: string) {
        if (!value) return;
        let {from, to} = view;
        if (changed === 'from') { from = value; if (to < from) to = from; } else { to = value; if (from > to) from = to; }
        move({...view, from, to});
    }
    const choose = (patch: Partial<Options>) => setOptions(o => ({...o, ...patch}));

    // ---- copying ----
    function flash() {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    }
    async function copy(flavor: CopyFlavor) {
        setMenu(false);
        const req = shown();
        try {
            if (flavor !== 'rich') {
                await Notes.ReviewCopyText((await Notes.ReviewText(req, flavor)).text);
                return flash();
            }
            // WebKit wants the write to start inside the click, so the clipboard gets promises, not results.
            const payload = Notes.ReviewText(req, 'rich');
            try {
                await navigator.clipboard.write([new ClipboardItem({
                    'text/html': payload.then(p => new Blob([p.html], {type: 'text/html'})),
                    'text/plain': payload.then(p => new Blob([p.text], {type: 'text/plain'})),
                })]);
            } catch {
                const p = await payload;
                if (!copyHtmlBySelection(p.html)) {
                    await Notes.ReviewCopyText(p.text);
                    onToast('Copied as Markdown. Formatted text isn\'t available here.');
                    return flash();
                }
            }
            flash();
        } catch (e) { onToast(`Couldn't copy. ${errText(e)}`); }
    }

    // ---- saving ----
    async function save(kind: SaveKind) {
        setMenu(false); setExportOpen(false); clearResult();
        const req = shown();
        if (kind === 'pdf') return makePdf(req);
        try {
            const path = await Notes.ReviewChoosePlace(req, kind);
            if (path) await write(req, kind, path);
        } catch (e) { fail(kind, '', errText(e)); }
    }
    async function write(req: ReviewRequest, kind: SaveKind, path: string) {
        setFailure(null);
        try {
            const out = await Notes.ReviewWrite(req, kind, path);
            setSaved({line: out.line, path: out.path});
        } catch (e) { fail(kind, path, errText(e)); }
    }
    function fail(kind: SaveKind, path: string, detail: string) {
        const name = kind === 'folder' ? (data?.fileName ?? 'the folder') : (path.split('/').pop() || 'the file');
        setFailure({text: `Couldn't save “${name}”.`, detail: `The folder may be read-only or the disk full.${detail ? ` ${detail}` : ''}`, kind, path});
    }
    async function makePdf(req: ReviewRequest) {
        if (!PDF_ENABLED) return;
        setBusy('Making the PDF…');
        try { setPrint(await Notes.ReviewText(req, 'print')); }
        catch (e) { setBusy(''); setFailure({text: 'Couldn’t make the PDF.', detail: `Markdown is still available. ${errText(e)}`, kind: 'pdf', path: ''}); }
    }
    // The print view is in the page; once it has painted, the native print window opens (choose "Save as PDF" there).
    useEffect(() => {
        if (!print) return;
        const id = requestAnimationFrame(() => requestAnimationFrame(() => {
            Promise.resolve(Window.Print()).catch(() => window.print()).finally(() => { setPrint(null); setBusy(''); });
            onToast('In the print window, choose PDF, then Save as PDF.');
        }));
        return () => cancelAnimationFrame(id);
    }, [print]); // eslint-disable-line react-hooks/exhaustive-deps
    const show = () => { if (saved?.path) Notes.Reveal([saved.path]).catch(e => onToast(`Couldn't open Finder. ${errText(e)}`)); };

    // ---- keys: ⌘3 week, ⌘4 month, ⌘5 year, ⇧⌘E export, ⇧⌘C copy as Markdown, ⌘[ ⌘] previous / next period ----
    const keys = useRef<(e: KeyboardEvent) => boolean>(() => false);
    keys.current = e => {
        const k = e.key.toLowerCase();
        if (k === '3' && !e.shiftKey) setScale('week');
        else if (k === '4' && !e.shiftKey) setScale('month');
        else if (k === '5' && !e.shiftKey) setScale('year');
        else if (k === 'e' && e.shiftKey) setExportOpen(true);
        else if (k === 'c' && e.shiftKey) void copy('markdown');
        else if (k === '[' && !e.shiftKey && data?.canPrev) void step(-1);
        else if (k === ']' && !e.shiftKey && data?.canNext) void step(1);
        else return false;
        return true;
    };
    useEffect(() => {
        // Capture phase, so the day page's own ⌘[ ⌘] never see these while the review is open.
        const on = (e: KeyboardEvent) => {
            if (!e.metaKey || e.altKey || e.ctrlKey || exportOpen) return;
            if (keys.current(e)) { e.preventDefault(); e.stopImmediatePropagation(); }
        };
        window.addEventListener('keydown', on, true);
        return () => window.removeEventListener('keydown', on, true);
    }, [exportOpen]);

    // The Export menu closes on Escape and on a click anywhere else.
    useEffect(() => {
        if (!menu) return;
        const out = (e: MouseEvent) => { if (!menuBox.current?.contains(e.target as Node)) setMenu(false); };
        const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setMenu(false); };
        window.addEventListener('mousedown', out);
        window.addEventListener('keydown', esc);
        return () => { window.removeEventListener('mousedown', out); window.removeEventListener('keydown', esc); };
    }, [menu]);

    const noun = view.scale === 'range' ? 'period' : view.scale;
    const lines = data ? (scaleOf(data.period) === 'week' && data.title.startsWith('Week of ') ? ['Week of', data.title.slice(8)] : [data.title]) : ['Review'];
    const capped = view.scale === 'range' && !!data && view.from < data.period.start;

    const header = (
        <div className="head">
            <div className="head-title">
                <p className="label">Review</p>
                <Display lines={lines}/>
            </div>
            <div className="rv-menu" ref={menuBox}>
                <button className="btn btn-solid" onClick={() => setMenu(m => !m)} disabled={!data} aria-haspopup="menu" aria-expanded={menu}
                        title="Copy, save or export this review (⇧⌘E)">{copied ? 'Copied ✓' : 'Export'}</button>
                {menu && (
                    <div className="rv-menu-list" role="menu" aria-label="Export">
                        <MenuItem onClick={() => copy('rich')}>Copy as Rich Text</MenuItem>
                        <MenuItem onClick={() => copy('markdown')}>Copy as Markdown</MenuItem>
                        <MenuItem onClick={() => copy('plain')}>Copy as Plain Text</MenuItem>
                        <div className="rv-menu-sep" role="separator"/>
                        <MenuItem onClick={() => save('markdown')}>Save as Markdown…</MenuItem>
                        <MenuItem onClick={() => save('pdf')} disabled={!PDF_ENABLED}>{PDF_ENABLED ? 'Save as PDF…' : 'Save as PDF… · Coming in the next build'}</MenuItem>
                        <div className="rv-menu-sep" role="separator"/>
                        <MenuItem onClick={() => { setMenu(false); setExportOpen(true); }}>More Export Options…</MenuItem>
                    </div>
                )}
            </div>
        </div>
    );

    let body;
    if (error && !data) {
        body = <Banner tone="danger" action={<Button variant="ghost" size="sm" onClick={() => setTick(t => t + 1)}>Try again</Button>}>
            <strong>Can't read your notes.</strong> {error} Check the folder in Settings, then try again.
        </Banner>;
    } else if (!data) {
        body = <p className="loading">Loading…</p>;
    } else if (data.empty) {
        body = <Empty title={[data.emptyText]} action={<div className="row-gap">
            {scaleOf(data.period) !== 'year' && data.notLogged > 0 && <Button variant="line" onClick={() => onCatchUp(data.missingDays)}>{data.catchUpLabel}</Button>}
            <Button variant="ghost" onClick={() => onOpenDay(today)}>Go to today</Button>
        </div>}>Pages show up here when you write or skip a day.</Empty>;
    } else if (scaleOf(data.period) === 'year') {
        body = <YearList months={data.months} onOpenMonth={openMonth}/>;
    } else if (grouping === 'day') {
        body = <DayList days={data.days} minWords={data.minWords} onOpenDay={onOpenDay}/>;
    } else {
        body = <GroupList groups={data.groups}/>;
    }

    return <>
        <style>{REVIEW_CSS}</style>
        {header}
        {data && (
            <div className="rv-counts small muted">
                <span>{data.countsLine}</span>
                {data.tasksLine && <span>{data.tasksLine}</span>}
            </div>
        )}
        <div className="rv-bar">
            <div role="group" aria-label="Review period">
                <Segmented options={SCALES} value={view.scale} onChange={setScale}/>
            </div>
            <div className="row-gap">
                <button className="btn btn-ghost btn-sm" onClick={() => step(-1)} disabled={!data?.canPrev} aria-label={`Previous ${noun} (⌘[)`} title={`Previous ${noun} (⌘[)`}>‹</button>
                {view.scale !== 'range' && <Button variant="line" size="sm" onClick={goThis} disabled={!data || data.isCurrent}>{`This ${noun}`}</Button>}
                <button className="btn btn-ghost btn-sm" onClick={() => step(1)} disabled={!data?.canNext} aria-label={`Next ${noun} (⌘])`} title={`Next ${noun} (⌘])`}>›</button>
            </div>
            {data && !data.empty && scaleOf(data.period) !== 'year' && data.notLogged > 0 && (
                <span className="rv-end"><Button variant="line" size="sm" onClick={() => onCatchUp(data.missingDays)}>{data.catchUpLabel}</Button></span>
            )}
        </div>
        {view.scale === 'range' && (
            <div className="rv-range">
                <label className="small muted" htmlFor="rv-from">From</label>
                <input id="rv-from" type="date" className="input input-sm" max={today} value={view.from} onChange={e => setRange('from', e.target.value)}/>
                <label className="small muted" htmlFor="rv-to">to</label>
                <input id="rv-to" type="date" className="input input-sm" max={today} value={view.to} onChange={e => setRange('to', e.target.value)}/>
                {capped && <span className="small muted">Ranges are limited to 3,700 days. Showing the latest ones.</span>}
            </div>
        )}
        <div className="rv-bar">
            <Button variant="ghost" size="sm" onClick={lastWeek}>Last week</Button>
            <Button variant="ghost" size="sm" onClick={thisMonth}>This month</Button>
            <span className="rv-end row-gap">
                <Switch label="Skipped days" checked={options.includeSkipped} onChange={v => choose({includeSkipped: v})}/>
                {view.scale !== 'year' && <>
                    <Switch label="Jots" checked={options.includeJots} onChange={v => choose({includeJots: v})}/>
                    <Segmented label="Group by" options={[['day', 'Day'], ['section', 'Section']]} value={grouping} onChange={g => choose({grouping: g})}/>
                </>}
            </span>
        </div>

        {busy && <p className="muted small rv-notice" role="status">{busy}</p>}
        {failure && (
            <div className="rv-notice">
                <Banner tone="danger" action={<div className="row-gap">
                    {failure.kind !== 'pdf' && failure.path && <Button variant="ghost" size="sm" onClick={() => void write(shown(), failure.kind, failure.path)}>Try again</Button>}
                    {failure.kind !== 'pdf' && <Button variant="ghost" size="sm" onClick={() => { const k = failure.kind; setFailure(null); void save(k); }}>Choose another place…</Button>}
                    <Button variant="ghost" size="sm" onClick={() => setFailure(null)}>Dismiss</Button>
                </div>}>
                    <strong>{failure.text}</strong> {failure.detail}
                </Banner>
            </div>
        )}
        {saved && (
            <div className="rv-notice">
                <Banner tone="accent" action={<div className="row-gap">
                    <Button variant="ghost" size="sm" onClick={show}>Show in Finder</Button>
                    <Button variant="ghost" size="sm" onClick={() => setSaved(null)}>Dismiss</Button>
                </div>}>
                    <strong>{saved.line}</strong>
                </Banner>
            </div>
        )}
        <span className="sr-only" role="status">{copied ? 'Copied' : ''}</span>

        <section className="rv-body">
            {data && scaleOf(data.period) === 'week' && view.scale === 'week' && <WeekTiles tiles={data.tiles} onOpenDay={onOpenDay}/>}
            <div style={data && scaleOf(data.period) === 'week' && view.scale === 'week' ? {marginTop: 'var(--s6)'} : undefined}>{body}</div>
        </section>

        <ExportDialog open={exportOpen} title={data?.title ?? 'review'} req={shown()} onClose={() => setExportOpen(false)}
                      onCopy={f => { setExportOpen(false); void copy(f); }} onSave={save}/>
        {print && createPortal(
            <div className="rv-print" aria-hidden="true"><div dangerouslySetInnerHTML={{__html: print.html}}/></div>, document.body)}
    </>;
}

function MenuItem({children, onClick, disabled}: {children: string; onClick: () => void; disabled?: boolean}) {
    return <button className="rv-menu-item" role="menuitem" onClick={onClick} disabled={disabled}>{children}</button>;
}

/** Rich copy without the async clipboard: select the formatted text in a hidden editable box and copy it. */
function copyHtmlBySelection(html: string): boolean {
    const box = document.createElement('div');
    box.contentEditable = 'true';
    box.style.cssText = 'position:fixed;left:-9999px;top:0;';
    box.innerHTML = new DOMParser().parseFromString(html, 'text/html').body.innerHTML;
    document.body.appendChild(box);
    const range = document.createRange();
    range.selectNodeContents(box);
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(range);
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { ok = false; }
    sel?.removeAllRanges();
    document.body.removeChild(box);
    return ok;
}
