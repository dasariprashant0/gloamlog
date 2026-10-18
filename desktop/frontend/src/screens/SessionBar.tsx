import {useEffect, useRef, useState} from 'react';
import {Banner, Button, Kbd} from '@personal/ui/react';
import {Notes, shortDay} from '../api';

type Props = {
    /** The unlogged days when the session began, oldest first. They stay fixed: "3 of 5" never moves. */
    days: string[];
    current: string;
    /** Open this day on the normal page. */
    onGo: (day: string) => void;
    /** Back to the Catch up list. */
    onEnd: () => void;
    /** Ask to skip this day (the lead opens SkipDialog). */
    onSkip: (day: string) => void;
    /** Nothing is left to catch up. Defaults to onEnd. */
    onFinish?: () => void;
    /** Save what is being typed first, so the live list is right. */
    flush?: () => Promise<void> | void;
};

const ONLY_DAY = 'This is the only day left. Write a little more, or skip it.';

export function SessionBar({days, current, onGo, onEnd, onSkip, onFinish, flush}: Props) {
    const [live, setLive] = useState<string[] | null>(null);
    const [note, setNote] = useState('');
    const i = days.indexOf(current);

    // Which of the session's days are still unlogged, for the progress line.
    useEffect(() => {
        let on = true;
        Notes.Overview().then(o => { if (on) setLive(o.catchUp); }).catch(() => {});
        return () => { on = false; };
    }, [current]);
    useEffect(() => { if (!note) return; const t = setTimeout(() => setNote(''), 5000); return () => clearTimeout(t); }, [note]);

    async function next() {
        await flush?.();
        const list = (await Notes.Overview()).catchUp; // the live list: a day written meanwhile is never offered again
        const n = list.find(d => d > current) ?? list.find(d => d !== current); // wrap once
        if (n) { setNote(''); onGo(n); }
        else if (list.includes(current)) setNote(ONLY_DAY);
        else (onFinish ?? onEnd)();
    }
    async function previous() {
        if (i <= 0) return;
        await flush?.();
        setNote('');
        onGo(days[i - 1]);
    }

    // Esc ends, ⌘↩ and ⌥⌘↓ go on, ⌥⌘↑ goes back. Latest handlers through a ref, so the listener is set once.
    const keys = useRef({next, previous, onEnd});
    keys.current = {next, previous, onEnd};
    useEffect(() => {
        const k = (e: KeyboardEvent) => {
            const cmd = e.metaKey || e.ctrlKey;
            if (e.key === 'Escape' && !e.defaultPrevented && !document.querySelector('dialog[open]')) keys.current.onEnd();
            else if (cmd && (e.key === 'Enter' || (e.altKey && e.key === 'ArrowDown'))) { e.preventDefault(); void keys.current.next(); }
            else if (cmd && e.altKey && e.key === 'ArrowUp') { e.preventDefault(); void keys.current.previous(); }
        };
        window.addEventListener('keydown', k);
        return () => window.removeEventListener('keydown', k);
    }, []);

    const total = Math.max(1, days.length);
    const done = live ? days.filter(d => !live.includes(d)).length : 0;
    const where = i >= 0 ? `Catching up ${i + 1} of ${total} · ${shortDay(current)}` : `Catching up · ${shortDay(current)}`;

    return (
        <section aria-label={i >= 0 ? `Catching up, ${i + 1} of ${total}` : 'Catching up'}>
            <Banner tone="accent" action={
                <div className="row-gap">
                    <Button variant="ghost" size="sm" onClick={onEnd} title="Back to the list (Esc)">End</Button>
                    <Button variant="line" size="sm" onClick={() => void previous()} disabled={i <= 0} title="Previous day in this run">Previous</Button>
                    <Button variant="line" size="sm" onClick={() => onSkip(current)} title={`Skip ${shortDay(current)}…`}>Skip day</Button>
                    <Button size="sm" onClick={() => void next()} title="Open the next unlogged day">Next unlogged day <Kbd>⌘↩</Kbd></Button>
                </div>
            }>
                <strong>{note || where}</strong>
            </Banner>
            <div role="progressbar" aria-label="Days done in this run" aria-valuemin={0} aria-valuemax={total} aria-valuenow={done}
                style={{height: 'var(--s1)', background: 'var(--rule)'}}>
                <div style={{height: '100%', width: `${(done / total) * 100}%`, background: 'var(--accent)', transition: 'width var(--base) var(--ease)'}}/>
            </div>
        </section>
    );
}
