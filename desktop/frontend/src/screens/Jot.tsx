// The Jot panel (its own small window, opened by the global shortcut or the menu bar): one field, Return adds the note to
// today's page. The Go side (desktop/capture.go) makes the note durable in the journal before it answers, so "Added" is true.
import {useCallback, useEffect, useRef, useState} from 'react';
import type {KeyboardEvent} from 'react';
import {Banner, Button, Kbd} from '@personal/ui/react';
import {Notes, Windows} from '../api';

type Phase = 'typing' | 'added' | 'kept' | 'failed';

const ADDED_MS = 600;  // "Added", then the panel closes
const KEPT_MS = 2600;  // a message the person has to read

const two = (n: number) => String(n).padStart(2, '0');
const clock = (d: Date) => `${two(d.getHours())}:${two(d.getMinutes())}`;

export function Jot() {
    const [text, setText] = useState('');
    const [phase, setPhase] = useState<Phase>('typing');
    const [message, setMessage] = useState('');
    const [stamp, setStamp] = useState('');
    const field = useRef<HTMLTextAreaElement>(null);
    const timer = useRef<number>(0);
    const busy = useRef(false);

    // The time the note will carry, when time stamps are on. Read again each time the panel comes forward.
    const refresh = useCallback(() => {
        setStamp(clock(new Date()));
        Notes.Settings().then(s => { if (!s.capture.timestamps) setStamp(''); }).catch(() => undefined);
    }, []);

    useEffect(() => {
        refresh();
        const comeForward = () => {
            window.clearTimeout(timer.current);
            // A draft comes back as it was left; a note that was just added starts a fresh one.
            setPhase(p => { if (p === 'added' || p === 'kept') setText(''); return p === 'failed' ? 'failed' : 'typing'; });
            refresh();
            field.current?.focus();
        };
        window.addEventListener('focus', comeForward);
        field.current?.focus();
        return () => { window.removeEventListener('focus', comeForward); window.clearTimeout(timer.current); };
    }, [refresh]);

    // The field grows with the note, one line to six, then scrolls.
    useEffect(() => {
        const el = field.current;
        if (!el) return;
        el.style.height = 'auto';
        el.style.height = `${el.scrollHeight}px`;
    }, [text]);

    const close = useCallback(() => { void Windows.HideJot(); }, []);

    const finish = (next: Phase, msg: string, ms: number) => {
        setPhase(next);
        setMessage(msg);
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => { setText(''); setPhase('typing'); close(); }, ms);
    };

    const submit = useCallback(async () => {
        const note = text.trim();
        if (busy.current || note === '' || note === '[]') return; // nothing to add
        busy.current = true;
        try {
            const r = await Notes.Jot(text);
            if (r.outcome === 'empty') return;
            if (r.outcome === 'kept') finish('kept', r.message, KEPT_MS);
            else finish('added', r.message, ADDED_MS);
        } catch {
            setPhase('failed');
        } finally {
            busy.current = false;
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [text]);

    const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
        if (e.key === 'Escape') { e.preventDefault(); setText(''); setPhase('typing'); close(); return; } // Esc discards the draft
        // Return adds; Shift or Option Return starts a new line; while an input method is composing, Return is its own.
        if (e.key === 'Enter' && !e.nativeEvent.isComposing && !e.shiftKey && !e.altKey) { e.preventDefault(); void submit(); }
    };

    const locked = phase === 'added' || phase === 'kept';
    return (
        <div className="jot">
            <div className="row-gap">
                <p className="label" id="jot-title">Jot to Today</p>
                {stamp && <span className="small muted" aria-label={`Time ${stamp}`}>{stamp}</span>}
            </div>
            <textarea ref={field} className="textarea" rows={1} aria-labelledby="jot-title" value={text} readOnly={locked}
                      placeholder="Jot a line for today…" style={{resize: 'none'}}
                      onChange={e => { setText(e.target.value); if (phase === 'failed') setPhase('typing'); }}
                      onKeyDown={onKeyDown}/>
            <div className="small" role="status" aria-live="polite">
                {phase === 'typing' && <span className="muted"><Kbd>Return</Kbd> adds · <Kbd>⇧Return</Kbd> new line · <Kbd>Esc</Kbd> cancels</span>}
                {phase === 'added' && <strong>{message}</strong>}
                {phase === 'kept' && <span>{message}</span>}
                {phase === 'failed' && (
                    <Banner tone="danger" action={<Button variant="line" size="sm" onClick={() => void submit()}>Try again</Button>}>
                        <strong>Couldn't save.</strong> Your note is still here.
                    </Banner>
                )}
            </div>
        </div>
    );
}
