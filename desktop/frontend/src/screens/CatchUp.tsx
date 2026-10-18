import {useEffect, useRef, useState, type MouseEvent} from 'react';
import {Banner, Button, Display, Empty, Field, Kbd, Row} from '@personal/ui/react';
import {Notes, dayDate, plural, shortDay, type Overview, type Settings} from '../api';
import type {CatchUpData} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import {SkipDialog} from './SkipDialog';

type Props = {
    overview: Overview;
    onOpenDay: (day: string) => void;
    /** Run through these days on the normal page with the session bar. */
    onStartSession: (days: string[]) => void;
    /** The list changed (days skipped or unskipped): reload the overview. */
    onChanged: () => void;
    /** The message to show, with an Undo when there is one. */
    onToast: (message: string, undo?: () => void) => void;
    /** Set when a session just ended with nothing left: shows "All caught up. 3 logged, 1 skipped." */
    finished?: {logged: number; skipped: number};
    /** Buttons for the empty states; a button shows only when its handler is given. */
    onReview?: () => void;
    onOpenCalendar?: () => void;
    onLogStart?: () => void;
};

const dayMonth = (day: string) => dayDate(day).toLocaleDateString('en-GB', {day: 'numeric', month: 'short'});
const scopeTitle = (days: number) => (days >= 365 ? 'Since log start' : `Last ${days} days`);

export function skipToast(days: number, reason: string) {
    return `Skipped ${plural(days, 'day')}${reason ? ` as ${reason}` : ''}.`;
}

export function CatchUpView({overview, onOpenDay, onStartSession, onChanged, onToast, finished, onReview, onOpenCalendar, onLogStart}: Props) {
    const [data, setData] = useState<CatchUpData | null>(null);
    const [settings, setSettings] = useState<Settings | null>(null);
    const [scope, setScope] = useState<number | null>(null); // null = the Settings value; a pick here applies to this screen only
    const [error, setError] = useState('');
    const [picked, setPicked] = useState<string[]>([]);
    const [skipDays, setSkipDays] = useState<string[]>([]);
    const [skipOpen, setSkipOpen] = useState(false);
    const anchor = useRef('');

    // Reload when the scope changes or the overview does (a day was written, skipped or unskipped).
    const stamp = overview.catchUp.join(',');
    useEffect(() => {
        let live = true;
        Promise.all([Notes.CatchUp(scope ?? 0), Notes.Settings()])
            .then(([d, s]) => {
                if (!live) return;
                setData(d); setSettings(s); setError('');
                setPicked(p => p.filter(day => d.days.some(i => i.day === day)));
            })
            .catch(e => { if (live) setError(e instanceof Error ? e.message : String(e)); });
        return () => { live = false; };
    }, [scope, stamp]);

    const rows = data?.days ?? [];
    const order = rows.map(r => r.day);
    const minWords = settings?.minWords ?? 20;
    const current = scope ?? settings?.catchUpWindowDays ?? 30;
    const choices = [...new Set([14, 30, 60, 90, 365, current])].sort((a, b) => a - b);

    const groups: {start: string; days: typeof rows}[] = [];
    for (const r of rows) {
        const last = groups[groups.length - 1];
        if (last && last.start === r.weekStart) last.days.push(r); else groups.push({start: r.weekStart, days: [r]});
    }

    function pick(day: string, e: MouseEvent) {
        const a = order.indexOf(anchor.current), b = order.indexOf(day);
        if (e.shiftKey && a >= 0) setPicked(order.slice(Math.min(a, b), Math.max(a, b) + 1));
        else setPicked(p => (p.includes(day) ? p.filter(d => d !== day) : [...p, day]));
        anchor.current = day;
    }
    function askSkip(days: string[]) { setSkipDays(days); setSkipOpen(true); }

    // ⌘↩ opens the oldest unlogged day.
    const start = useRef(() => {});
    start.current = () => { if (!finished && !skipOpen && order.length) onStartSession(order); };
    useEffect(() => {
        const k = (e: KeyboardEvent) => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); start.current(); } };
        window.addEventListener('keydown', k);
        return () => window.removeEventListener('keydown', k);
    }, []);

    const sentence = rows.length === 0 ? "You're all caught up." : `You have ${plural(rows.length, 'unlogged day')}.`;
    const newUser = !!settings && !settings.logStartDate && overview.summary.logs + overview.summary.skipped === 0;

    let body;
    if (error) {
        body = <Banner tone="danger"><strong>Can't read your notes.</strong> {error} Check the folder in Settings, then try again.</Banner>;
    } else if (finished) {
        const parts = [finished.logged > 0 && `${finished.logged} logged`, finished.skipped > 0 && `${finished.skipped} skipped`].filter(Boolean).join(', ');
        body = <Empty title={parts ? ['All caught up.', `${parts}.`] : ['All caught up.']} action={<div className="row-gap">
            {onReview && <Button variant="line" onClick={onReview}>Review this week</Button>}
            <Button variant="ghost" onClick={() => onOpenDay(overview.today)}>Go to today</Button>
        </div>}>{overview.streak.current > 1 ? `Your streak is ${overview.streak.current} days.` : 'Every working day in this run has a page or a skip.'}</Empty>;
    } else if (!data) {
        body = <p className="loading">Loading…</p>;
    } else if (rows.length === 0 && newUser) {
        body = <Empty title={['Nothing to catch up', 'on yet.']} action={<div className="row-gap">
            {onOpenCalendar && <Button variant="line" onClick={onOpenCalendar}>Open calendar</Button>}
            {onLogStart && <Button variant="ghost" onClick={onLogStart}>Log start date…</Button>}
        </div>}>Days before your first page don't count.</Empty>;
    } else if (rows.length === 0) {
        body = <Empty title={["You're all caught up."]} action={<div className="row-gap">
            <Button variant="line" onClick={() => onOpenDay(overview.today)}>Go to today</Button>
            {onReview && <Button variant="ghost" onClick={onReview}>Review this week</Button>}
        </div>}>Every working day since {data.since ? dayMonth(data.since) : 'the start'} has a page or a skip.</Empty>;
    } else {
        body = <div className="stack">
            <div className="row-gap">
                <Field label="How far back">
                    <select className="select" value={current} onChange={e => setScope(Number(e.target.value))}>
                        {choices.map(d => <option key={d} value={d}>{scopeTitle(d)}</option>)}
                    </select>
                </Field>
                <Button variant="line" onClick={() => askSkip(order)}>Skip all {rows.length} days…</Button>
            </div>
            {picked.length > 0 && (
                <Banner action={<div className="row-gap">
                    <Button variant="line" size="sm" onClick={() => askSkip(order.filter(d => picked.includes(d)))}>Skip…</Button>
                    <Button variant="ghost" size="sm" onClick={() => { setPicked([]); anchor.current = ''; }}>Clear</Button>
                </div>}>
                    <strong>{picked.length} selected.</strong> Shift-click a box to select a range.
                </Banner>
            )}
            {groups.map(g => (
                <section key={g.start} className="stack">
                    <h2 className="label">Week of {dayMonth(g.start)} · {plural(g.days.length, 'unlogged day')}</h2>
                    <ul className="list">
                        {g.days.map(r => (
                            <li key={r.day}>
                                <Row
                                    mark={<input type="checkbox" checked={picked.includes(r.day)} onChange={() => {}} onClick={e => pick(r.day, e)}
                                        aria-label={`Select ${shortDay(r.day)}`} style={{accentColor: 'var(--accent)'}}/>}
                                    title={shortDay(r.day)}
                                    meta={r.started ? `${r.words} of ${minWords} words` : 'Not started'}
                                    end={<span className="row-gap">
                                        <button className="btn btn-line btn-sm" onClick={() => onOpenDay(r.day)}
                                            aria-label={`${r.started ? 'Continue' : 'Write'} ${shortDay(r.day)}`}>{r.started ? 'Continue' : 'Write'}</button>
                                        <button className="btn btn-ghost btn-sm" onClick={() => askSkip([r.day])}
                                            aria-label={`Skip ${shortDay(r.day)}`}>Skip</button>
                                    </span>}/>
                            </li>
                        ))}
                    </ul>
                </section>
            ))}
        </div>;
    }

    return <>
        <div className="head">
            <div className="head-title">
                <Display lines={['Catch up']}/>
                {!finished && !error && data && <p className="muted">{sentence}</p>}
            </div>
            {!finished && (
                <Button onClick={() => onStartSession(order)} disabled={order.length === 0} title="Open the oldest unlogged day (⌘↩)">
                    Start catching up <Kbd>⌘↩</Kbd>
                </Button>
            )}
        </div>
        <section className="block">{body}</section>
        <SkipDialog open={skipOpen} days={skipDays} today={overview.today} many onClose={() => setSkipOpen(false)}
            onSkipped={(days, reason) => {
                setPicked([]);
                onChanged();
                onToast(skipToast(days.length, reason), () => {
                    Notes.Unskip(days).then(() => onChanged())
                        .catch(e => onToast(`Couldn't undo the skip. ${e instanceof Error ? e.message : String(e)}`));
                });
            }}/>
    </>;
}
