// The pieces the Review screen is drawn from: the week strip, the day and section lists, the year list and the markdown
// clamp. Also the stylesheet for the .rv-* classes (tokens only, except the print rules): move it into app.css when convenient.
import {useCallback, useEffect, useLayoutEffect, useRef, useState, type MouseEvent} from 'react';
import {Row} from '@personal/ui/react';
import {Browser} from '@wailsio/runtime';
import type {ReviewDayView, ReviewGroupView, ReviewMonthView, ReviewTile} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

export const REVIEW_CSS = `
.rv-bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3) var(--s5); margin-top: var(--s5); }
.rv-bar .rv-end { margin-left: auto; }
.rv-counts { display: grid; gap: var(--s1); margin-top: var(--s4); }
.rv-range { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s3); margin-top: var(--s4); }
.rv-range .input { width: auto; }
.rv-notice { margin-top: var(--s4); }
.rv-body { margin-top: var(--s6); }

.rv-menu { position: relative; }
.rv-menu-list { position: absolute; right: 0; top: calc(100% + var(--s1)); z-index: 20; min-width: max-content; padding: var(--s1) 0; background: var(--background); box-shadow: inset 0 0 0 1px var(--foreground); }
.rv-menu-item { display: block; width: 100%; padding: var(--s2) var(--s4); text-align: left; font-size: var(--text-sm); white-space: nowrap; }
.rv-menu-item:hover, .rv-menu-item:focus-visible { background: var(--wash); }
.rv-menu-sep { margin: var(--s1) 0; border-top: 1px solid var(--rule); }

.rv-tiles { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: var(--s2); }
.rv-tile { display: grid; gap: var(--s1); justify-items: center; align-content: start; padding: var(--s3) var(--s1); box-shadow: inset 0 0 0 1px var(--rule); text-align: center; font-size: var(--text-xs); color: var(--muted); }
.rv-tile:hover:not(:disabled) { background: var(--wash-2); }
.rv-tile:disabled { opacity: 0.5; cursor: default; }
.rv-tile.off { opacity: 0.65; }
.rv-tile.today { box-shadow: inset 0 0 0 2px var(--accent); }
.rv-tile-top { display: flex; align-items: center; gap: var(--s2); color: var(--foreground); }
.rv-num { font-size: var(--text-base); font-weight: 500; }
.rv-bottom { min-height: 1.55em; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rv-tile.missed .rv-bottom { color: var(--warning); font-weight: 500; }
.rv-mark { width: var(--s2); height: var(--s2); flex: none; }
.rv-mark.logged { background: var(--accent); }
.rv-mark.partial { background: linear-gradient(90deg, var(--accent) 50%, transparent 50%); box-shadow: inset 0 0 0 1px var(--accent); }
.rv-mark.missed { box-shadow: inset 0 0 0 1px var(--warning); }
.rv-mark.skipped { height: 2px; background: var(--muted); }
.rv-mark.off, .rv-mark.future { width: 2px; height: 2px; background: var(--ink-4); }

.rv-day { padding: var(--s5) 0; border-top: 1px solid var(--rule); }
.rv-day-head { display: flex; align-items: baseline; gap: var(--s3); margin-bottom: var(--s2); }
.rv-title { font-size: var(--text-base); font-weight: 500; margin-right: auto; }
.rv-item { display: grid; grid-template-columns: var(--s8) minmax(0, 1fr); gap: var(--s3); padding: var(--s2) 0; }
.rv-when { font-size: var(--text-xs); color: var(--muted); padding-top: var(--s1); }
.rv-dim { opacity: 0.5; }

.rv-md { font-size: var(--text-sm); max-width: var(--measure); overflow-wrap: anywhere; }
.rv-md > :first-child { margin-top: 0; }
.rv-md > :last-child { margin-bottom: 0; }
.rv-md p, .rv-md ul, .rv-md ol, .rv-md pre, .rv-md blockquote, .rv-md table { margin: 0 0 var(--s2); }
.rv-md ul { list-style: disc; padding-left: var(--s5); }
.rv-md ol { list-style: decimal; padding-left: var(--s5); }
.rv-md li.task { list-style: none; margin-left: calc(-1 * var(--s4)); }
.rv-md h1, .rv-md h2, .rv-md h3, .rv-md h4, .rv-md h5, .rv-md h6 { margin: var(--s3) 0 var(--s1); font-size: var(--text-sm); font-weight: 500; color: var(--muted); }
.rv-md code { padding: 0 var(--s1); background: var(--wash-2); }
.rv-md pre { padding: var(--s2) var(--s3); background: var(--wash-2); overflow: auto; white-space: pre-wrap; }
.rv-md pre code { padding: 0; background: none; }
.rv-md blockquote { padding-left: var(--s3); border-left: 2px solid var(--rule); color: var(--muted); }
.rv-md table { border-collapse: collapse; }
.rv-md th, .rv-md td { padding: var(--s1) var(--s2); border: 1px solid var(--rule); text-align: left; }
.rv-md img { display: block; max-width: 100%; height: auto; }
.rv-md .img { color: var(--muted); }
.rv-md a { text-decoration: underline; text-underline-offset: 3px; }
.rv-clamp { max-height: calc(var(--text-sm) * 1.55 * 9); overflow: hidden; }

/* The print view (Save as PDF): hidden on screen; when printing it is the only thing on the page. Paper is white with black
   ink in every theme, so these few rules use plain values instead of tokens. */
.rv-print { display: none; }
@media print {
  #root { display: none !important; }
  .rv-print { display: block; color: #000; background: #fff; font: 11pt/1.45 -apple-system, "Helvetica Neue", Helvetica, Arial, sans-serif; }
  .rv-print h1 { font-size: 20pt; margin: 0 0 6pt; } .rv-print h2 { font-size: 14pt; margin: 18pt 0 4pt; } .rv-print h3 { font-size: 12pt; margin: 12pt 0 3pt; }
  .rv-print h4, .rv-print h5, .rv-print h6 { font-size: 11pt; margin: 10pt 0 2pt; }
  .rv-print h1, .rv-print h2, .rv-print h3, .rv-print h4, .rv-print h5, .rv-print h6 { break-after: avoid; break-inside: avoid; }
  .rv-print p { margin: 0 0 6pt; } .rv-print ul, .rv-print ol { margin: 0 0 6pt; padding-left: 18pt; } .rv-print li { margin: 2pt 0; break-inside: avoid; }
  .rv-print ul { list-style: disc; } .rv-print ol { list-style: decimal; } .rv-print li.task { list-style: none; margin-left: -14pt; }
  .rv-print blockquote { margin: 6pt 0; padding-left: 10pt; border-left: 2pt solid #bbb; color: #444; }
  .rv-print pre { padding: 6pt; background: #eee; white-space: pre-wrap; break-inside: avoid; } .rv-print code { background: #eee; }
  .rv-print table { border-collapse: collapse; margin: 6pt 0; } .rv-print th, .rv-print td { border: 0.5pt solid #bbb; padding: 3pt 6pt; text-align: left; }
  .rv-print img { max-width: 100%; max-height: 280pt; break-inside: avoid; } .rv-print .img { color: #444; } .rv-print hr { border: 0; border-top: 0.5pt solid #bbb; }
}
`;

/** Markdown the core turned into HTML. Shows nine lines, then "Show more"; links open in the browser, never in this window. */
export function Markdown({html, clamp}: {html: string; clamp?: boolean}) {
    const [open, setOpen] = useState(false);
    const [long, setLong] = useState(false);
    const box = useRef<HTMLDivElement>(null);
    const measure = useCallback(() => {
        const el = box.current;
        if (el && clamp && !open) setLong(el.scrollHeight > el.clientHeight + 1);
    }, [clamp, open]);
    useLayoutEffect(measure, [measure, html]);
    useEffect(() => {
        window.addEventListener('resize', measure);
        return () => window.removeEventListener('resize', measure);
    }, [measure]);

    function click(e: MouseEvent) {
        const a = (e.target as HTMLElement).closest('a');
        if (!a) return;
        e.preventDefault();
        const href = a.getAttribute('href') ?? '';
        Browser.OpenURL(href).catch(() => window.open(href, '_blank'));
    }
    return (
        <div>
            <div ref={box} className={clamp && !open ? 'rv-md rv-clamp' : 'rv-md'} onClick={click} onLoadCapture={measure}
                 dangerouslySetInnerHTML={{__html: html}}/>
            {clamp && (long || open) && <button className="btn btn-ghost btn-sm" onClick={() => setOpen(o => !o)}>{open ? 'Show less' : 'Show more'}</button>}
        </div>
    );
}

/** Seven days, each opens its day; days still to come are inert; empty weeks still show them. */
export function WeekTiles({tiles, onOpenDay}: {tiles: ReviewTile[]; onOpenDay: (day: string) => void}) {
    return (
        <div className="rv-tiles" role="group" aria-label="Days of this week">
            {tiles.map(t => (
                <button key={t.day} disabled={!t.opens} className={`rv-tile ${t.status}${t.isToday ? ' today' : ''}`}
                        aria-label={t.spoken} title={t.opens ? t.spoken : 'Upcoming'} onClick={() => onOpenDay(t.day)}>
                    <span>{t.weekday}</span>
                    <span className="rv-tile-top"><span className="rv-num">{t.number}</span><span className={`rv-mark ${t.status}`} aria-hidden="true"/></span>
                    <span className="rv-bottom">{t.bottom}</span>
                </button>
            ))}
        </div>
    );
}

/** "Group by Day": each day's writing, or its skip. */
export function DayList({days, minWords, onOpenDay}: {days: ReviewDayView[]; minWords: number; onOpenDay: (day: string) => void}) {
    return (
        <div>
            {days.map(d => (
                <section key={d.day} className="rv-day">
                    <div className="rv-day-head">
                        <h3 className="rv-title">{d.label}</h3>
                        {d.status === 'skipped' && <span className="small muted">Skipped{d.skipReason ? ` · ${d.skipReason}` : ''}</span>}
                        {d.status === 'partial' && <span className="small muted">{d.words} of {minWords} words</span>}
                        <button className="btn btn-ghost btn-sm" onClick={() => onOpenDay(d.day)} aria-label={`Open ${d.spoken}`}>Open</button>
                    </div>
                    {d.status !== 'skipped' && <Markdown html={d.html} clamp/>}
                </section>
            ))}
        </div>
    );
}

/** "Group by Section": one heading's roll-up across the period (or Jots, Other notes, Skipped days), every item dated. */
export function GroupList({groups}: {groups: ReviewGroupView[]}) {
    return (
        <div>
            {groups.map((g, i) => (
                <section key={i} className="rv-day">
                    <h3 className="rv-title rv-day-head">{g.title}</h3>
                    {g.items.map((it, j) => (
                        <div key={j} className="rv-item">
                            <span className="rv-when">{it.label}</span>
                            {g.kind === 'skipped' ? <span className="small muted">{it.text}</span> : <Markdown html={it.html} clamp/>}
                        </div>
                    ))}
                </section>
            ))}
        </div>
    );
}

/** A year: twelve rows, one click opens a month; a month that has not begun is a dash and inert. */
export function YearList({months, onOpenMonth}: {months: ReviewMonthView[]; onOpenMonth: (month: string) => void}) {
    return (
        <ul className="list" aria-label={`Months of ${months[0]?.month.slice(0, 4) ?? 'the year'}`}>
            {months.map(m => (
                <li key={m.month} className={m.inert ? 'rv-dim' : undefined}>
                    <Row title={m.name} meta={m.line} onClick={m.isFuture ? undefined : () => onOpenMonth(m.month)}/>
                </li>
            ))}
        </ul>
    );
}
