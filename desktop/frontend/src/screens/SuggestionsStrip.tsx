// "Suggested from your work": lines from Claude Code sessions and Git commits for this day, ticked and inserted under the
// sources heading (PARITY 3.F). Shown only when a source is on and the first-use consent was given.
import {useEffect, useState} from 'react';
import {Button} from '@personal/ui/react';
import {Notes, Windows, plural} from '../api';
import type {Suggestions} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import type {SuggestedBullet} from '../../bindings/github.com/dasariprashant0/gloamlog/internal/sources';

const HIDDEN_KEY = 'gloamlog.suggestions.hiddenDays';
const OPEN_KEY = 'gloamlog.suggestions.expanded';
const read = <T,>(k: string, fallback: T): T => { try { const v = localStorage.getItem(k); return v ? JSON.parse(v) : fallback; } catch { return fallback; } };
const write = (k: string, v: unknown) => { try { localStorage.setItem(k, JSON.stringify(v)); } catch { /* storage off: not remembered */ } };

export function SuggestionsStrip({day, pageMarkdown, onInsert, onToast}: {
    day: string; pageMarkdown: () => string; onInsert: (heading: string, markdown: string, count: number) => void;
    onToast: (message: string, undo?: () => void) => void;
}) {
    const [data, setData] = useState<Suggestions | null>(null);
    const [looking, setLooking] = useState(false);
    const [error, setError] = useState('');
    const [added, setAdded] = useState<string[]>([]);
    const [ticked, setTicked] = useState<string[]>([]);
    const [open, setOpen] = useState(() => read(OPEN_KEY, true));
    const [hidden, setHidden] = useState<string[]>(() => read(HIDDEN_KEY, []));

    async function look(again: boolean) {
        setLooking(true);
        try {
            const s = await Notes.Suggestions(day, again);
            setData(s);
            const bullets = s.result?.bullets ?? [];
            setAdded(bullets.length ? await Notes.AlreadyAdded(bullets, pageMarkdown()) : []);
            setTicked([]);
            setError('');
        } catch (e) {
            // A newer look replaced this one (context canceled): nothing to show. Anything else is a real failure.
            const msg = String((e as Error)?.message ?? e);
            if (!/cancel/i.test(msg)) setError(msg);
        }
        setLooking(false);
    }
    useEffect(() => { setData(null); void look(false); }, [day]); // eslint-disable-line react-hooks/exhaustive-deps

    if (error && !data) return <p className="small danger suggest" role="alert">Couldn't look at your work: {error}</p>;
    if (!data || data.off || hidden.includes(day)) return null;
    const bullets = (data.result?.bullets ?? []).filter(b => !added.includes(b.id));
    const chosen = bullets.filter(b => ticked.includes(b.id));

    function notToday() {
        const next = [...hidden.filter(d => d !== day), day].slice(-90);
        setHidden(next);
        write(HIDDEN_KEY, next);
        onToast('Suggestions hidden for this day.', () => { const back = next.filter(d => d !== day); setHidden(back); write(HIDDEN_KEY, back); });
    }
    async function insert(list: SuggestedBullet[]) {
        if (!list.length) return;
        let lines: string;
        try {
            lines = await Notes.SuggestionLines(list);
        } catch (e) {
            onToast(`Couldn't add the suggestions. ${String((e as Error)?.message ?? e)}`);
            return;
        }
        onInsert(data!.heading, lines, list.length);
        setAdded(a => [...a, ...list.map(b => b.id)]);
        setTicked([]);
    }
    const toggle = (id: string) => setTicked(t => t.includes(id) ? t.filter(x => x !== id) : [...t, id]);

    return (
        <section className="suggest rule-top" aria-label="Suggested from your work">
            <div className="suggest-head">
                <button className="label" aria-expanded={open} onClick={() => { setOpen(!open); write(OPEN_KEY, !open); }}>
                    {open ? '▾' : '▸'} Suggested from your work{bullets.length ? ` · ${bullets.length}` : ''}
                </button>
                <div className="row-gap">
                    <Button variant="ghost" size="sm" onClick={() => void look(true)} disabled={looking}>Look again</Button>
                    <Button variant="ghost" size="sm" onClick={notToday}>Not today</Button>
                </div>
            </div>
            {open && <>
                {data.noneOn ? (
                    <p className="small muted">No projects are ticked yet, so nothing is read. <button className="btn btn-ghost btn-sm" onClick={() => void Windows.ShowSettings()}>Choose projects…</button></p>
                ) : looking && !bullets.length ? <p className="small muted">Looking at your work…</p>
                  : bullets.length === 0 ? <p className="small muted">Nothing new from your work for this day.</p> : <>
                    <ul className="suggest-list">
                        {bullets.map(b => (
                            <li key={b.id}>
                                <label className="suggest-item">
                                    <input type="checkbox" checked={ticked.includes(b.id)} onChange={() => toggle(b.id)}/>
                                    <span className="row-body"><span>{b.text}</span>{b.detail && <span className="row-meta">{b.detail}</span>}</span>
                                </label>
                            </li>
                        ))}
                    </ul>
                    <div className="row-gap">
                        <Button variant="line" size="sm" onClick={() => setTicked(bullets.map(b => b.id))}>Select all</Button>
                        <Button variant="line" size="sm" disabled={!chosen.length} onClick={() => void insert(chosen)}>
                            {chosen.length ? `Insert ${plural(chosen.length, 'line')}` : 'Insert selected'}
                        </Button>
                    </div>
                </>}
                {[...(data.result?.notes ?? []), data.result?.hiddenLine, data.result?.lastScanLine].filter(Boolean).map(l => <p key={l} className="small muted">{l}</p>)}
            </>}
        </section>
    );
}
