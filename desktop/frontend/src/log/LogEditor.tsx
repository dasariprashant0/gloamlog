// The list page's editor: a row per line and the add field. The state is ./model.ts (pure, tested); every change is written
// out by Go (Notes.LogRender), which also gives each line's HTML. DayPage owns saving: it hears the markdown through onChange
// and asks for the latest with markdown() before it flushes.
import {forwardRef, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState} from 'react';
import type {ClipboardEvent, KeyboardEvent, MouseEvent} from 'react';
import {Button, Kbd} from '@personal/ui/react';
import {Browser} from '@wailsio/runtime';
import {Notes} from '../api';
import type {LogPage} from '../../bindings/github.com/dasariprashant0/gloamlog/internal/core';
import * as M from './model';

export type LogEditorHandle = {
    /** The page as markdown, the line being edited included. */
    markdown(): Promise<string>;
    /** Adds the bullets in markdown at the end (a jot, the suggestions strip). */
    appendLines(markdown: string): Promise<{result: string; markdown: string}>;
    /** From yesterday's Keep / Done: a line at the end, at the current time. */
    addLine(text: string, mark: M.Mark): void;
};

/** What the list page shows: the rows to edit, or the same lines as a standup. */
export type View = 'list' | 'standup';

type Props = {
    day: string; page: LogPage; readOnly: boolean; autoFocus: boolean; view: View;
    onChange: (markdown: string, words: number) => void;
    onSummary: (summary: string) => void;
    onToast: (message: string) => void;
};

const RENDER_DELAY = 250;
const MARKS: M.Mark[] = ['', 'next', 'blocked'];
const word = (m: M.Mark) => m || 'done';
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const now = () => M.clock(new Date());
const nextMark = (m: M.Mark): M.Mark => MARKS[(MARKS.indexOf(m) + 1) % MARKS.length];
// ⌘1 / ⌘2 / ⌘3 while typing mark the line; the event is stopped so the window shortcut (Today / Catch up / Review) doesn't fire.
const markKey = (e: KeyboardEvent): M.Mark | null =>
    (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && ['1', '2', '3'].includes(e.key) ? MARKS[Number(e.key) - 1] : null;

export const LogEditor = forwardRef<LogEditorHandle, Props>(function LogEditor({day, page, readOnly, autoFocus, view, onChange, onSummary, onToast}, ref) {
    const [state, setState] = useState(() => M.fresh(page.lines as unknown as M.Line[]));
    const [html, setHtml] = useState<string[]>([]);
    const stateRef = useRef(state);
    stateRef.current = state;
    const dirty = useRef(false); // the person changed something: only then does DayPage hear about it (opening never writes)
    const seq = useRef(0);
    const timer = useRef(0);

    const act = useCallback((f: (s: M.State) => M.State) => { dirty.current = true; setState(f); }, []);

    const render = useCallback((lines: M.Line[]) => {
        const n = ++seq.current;
        return Notes.LogRender({...page, lines} as unknown as LogPage).then(r => {
            if (n === seq.current) setHtml(r.html ?? []);
            return {markdown: r.markdown, words: r.words};
        });
    }, [page]);

    // Every change: a render after a short pause; DayPage hears the markdown only after a real edit (it saves 600 ms later).
    useEffect(() => {
        clearTimeout(timer.current);
        timer.current = window.setTimeout(() => {
            render(M.withDraft(stateRef.current)).then(r => { if (dirty.current && !readOnly) onChange(r.markdown, r.words); },
                e => onToast(`Couldn't show this page: ${String(e?.message ?? e)}`));
        }, RENDER_DELAY);
        return () => clearTimeout(timer.current);
    }, [state.lines, state.draft]); // eslint-disable-line react-hooks/exhaustive-deps

    useEffect(() => { onSummary(M.summary(state.lines)); }, [state.lines]); // eslint-disable-line react-hooks/exhaustive-deps

    useImperativeHandle(ref, () => ({
        async markdown() {
            clearTimeout(timer.current);
            return (await render(M.withDraft(stateRef.current))).markdown;
        },
        async appendLines(md) {
            const a = await Notes.LogAppendLines({...page, lines: M.withDraft(stateRef.current)} as unknown as LogPage, md);
            if (a.result === 'appended') { dirty.current = true; setState(s => M.adopt(s, a.lines as unknown as M.Line[])); }
            return {result: a.result, markdown: a.rendered.markdown};
        },
        addLine(text, mark) { act(s => M.append(s, text, now(), mark)); },
    }), [page, render, act]);

    async function onPaste(e: ClipboardEvent<HTMLTextAreaElement>) {
        const file = [...e.clipboardData.files].find(f => f.type.startsWith('image/'));
        if (!file) return; // text pastes as usual
        e.preventDefault();
        const at = e.currentTarget.selectionStart;
        try {
            const b64 = await new Promise<string>((ok, fail) => {
                const r = new FileReader();
                r.onload = () => ok(String(r.result).split(',')[1] ?? '');
                r.onerror = () => fail(r.error);
                r.readAsDataURL(file);
            });
            const path = await Notes.SaveImage(day, file.name || 'pasted.png', file.type, b64);
            act(s => M.setDraft(s, `${s.draft.slice(0, at)}![](${path})${s.draft.slice(at)}`));
        } catch (err) {
            onToast(`Couldn't add the image: ${String((err as Error)?.message ?? err)}`);
        }
    }

    function onKey(e: KeyboardEvent<HTMLTextAreaElement>, i: number) {
        const cmd = e.metaKey || e.ctrlKey;
        const ta = e.currentTarget;
        const mark = markKey(e);
        if (mark !== null) { e.preventDefault(); e.stopPropagation(); act(s => M.setMark(s, i, mark)); }
        else if (e.key === 'Enter' && !e.shiftKey && !cmd && !e.nativeEvent.isComposing) { e.preventDefault(); act(s => M.newLineAfter(s, i, now())); }
        else if (cmd && !e.altKey && e.key.toLowerCase() === 'z') { e.preventDefault(); e.stopPropagation(); act(e.shiftKey ? M.redo : M.undo); }
        else if (e.key === 'Backspace' && ta.value === '') { e.preventDefault(); act(s => M.removeLine(s, i)); }
        else if (e.key === 'ArrowUp' && !ta.value.slice(0, ta.selectionStart).includes('\n')) { e.preventDefault(); act(s => M.move(s, -1)); }
        else if (e.key === 'ArrowDown' && !ta.value.slice(ta.selectionEnd).includes('\n')) { e.preventDefault(); act(s => M.move(s, 1)); }
        else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); act(M.commit); }
    }

    // The same state in both views: the draft, undo and the From yesterday picks survive a switch. html is per withDraft line.
    if (view === 'standup') return <StandupView lines={M.withDraft(state)} html={html} onToast={onToast}/>;

    return (
        <div className="log">
            <ul className="log-list" aria-label="Lines">
                {state.lines.map((l, i) => (
                    <LogRow key={i} line={l} editing={state.edit === i} draft={state.draft} html={html[M.renderIndex(state, i)] ?? ''}
                            readOnly={readOnly} onEdit={() => act(s => M.focus(s, i))} onDraft={d => act(s => M.setDraft(s, d))}
                            onKey={e => onKey(e, i)} onPaste={onPaste} onBlur={() => act(M.commit)} onCycle={() => act(s => M.setMark(s, i, nextMark(l.mark)))}
                            onTime={t => act(s => M.setTime(s, i, t))}/>
                ))}
            </ul>
            {!readOnly && <AddField autoFocus={autoFocus} onAdd={(text, mark) => act(s => M.append(s, text, now(), mark))}/>}
        </div>
    );
});

function LogRow({line, editing, draft, html, readOnly, onEdit, onDraft, onKey, onPaste, onBlur, onCycle, onTime}: {
    line: M.Line; editing: boolean; draft: string; html: string; readOnly: boolean;
    onEdit: () => void; onDraft: (d: string) => void; onKey: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
    onPaste: (e: ClipboardEvent<HTMLTextAreaElement>) => void;
    onBlur: () => void; onCycle: () => void; onTime: (t: string) => void;
}) {
    const ta = useRef<HTMLTextAreaElement>(null);
    const [timing, setTiming] = useState(false);
    const cancelTime = useRef(false);
    useLayoutEffect(() => {
        const el = ta.current;
        if (!editing || !el) return;
        if (document.activeElement !== el) { el.focus(); el.setSelectionRange(el.value.length, el.value.length); }
        el.style.height = 'auto';
        el.style.height = `${el.scrollHeight}px`;
    }, [editing, draft]);

    const shown = html || `<p>${esc(line.text)}</p>`;
    if (line.other) return <li className="log-row log-other"><div className="log-text" dangerouslySetInnerHTML={{__html: shown}}/></li>;

    const openLinkOrEdit = (target: EventTarget) => {
        const a = (target as HTMLElement).closest('a');
        if (a) { if (/^(https?:|mailto:)/i.test(a.href)) Browser.OpenURL(a.href).catch(() => window.open(a.href, '_blank')); return; }
        if (!readOnly) onEdit();
    };
    return (
        <li className={`log-row${editing ? ' is-editing' : ''}`}>
            {timing && !readOnly
                ? <input className="input input-sm log-time-input" type="time" aria-label="Time" autoFocus defaultValue={line.time.padStart(5, '0')}
                         onBlur={e => { setTiming(false); if (!cancelTime.current) onTime(e.currentTarget.value); cancelTime.current = false; }}
                         onKeyDown={e => { if (e.key === 'Enter') e.currentTarget.blur(); if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); cancelTime.current = true; e.currentTarget.blur(); } }}/>
                : <button className="log-time" disabled={readOnly} aria-label={line.time ? `Time ${line.time}. Change` : 'Add a time'}
                          onClick={() => setTiming(true)}>{line.time || '–'}</button>}
            {editing
                ? <textarea ref={ta} className="log-input" rows={1} aria-label="Line" value={draft}
                            onChange={e => onDraft(e.target.value)} onKeyDown={onKey} onPaste={onPaste} onBlur={onBlur}/>
                : <div className="log-text" role="button" tabIndex={readOnly ? -1 : 0} aria-label={`Edit: ${line.text || 'empty line'}`}
                       onClick={e => { e.preventDefault(); openLinkOrEdit(e.target); }}
                       onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (!readOnly) onEdit(); } }}
                       dangerouslySetInnerHTML={{__html: shown}}/>}
            <button className={`badge log-mark mark-${word(line.mark)}`} disabled={readOnly} aria-label={`Mark: ${word(line.mark)}. Change`}
                    onMouseDown={e => e.preventDefault() /* keep the line being edited */} onClick={onCycle}>{word(line.mark)}</button>
        </li>
    );
}

function AddField({autoFocus, onAdd}: {autoFocus: boolean; onAdd: (text: string, mark: M.Mark) => void}) {
    const [text, setText] = useState('');
    const add = (mark: M.Mark) => { if (!text.trim()) return; onAdd(text, mark); setText(''); };
    return (
        <div className="log-add">
            <label className="sr-only" htmlFor="log-add">Add a line</label>
            <input id="log-add" className="input" placeholder="What did you just do?" autoFocus={autoFocus} value={text}
                   onChange={e => setText(e.target.value)}
                   onKeyDown={e => {
                       const mark = markKey(e);
                       if (mark !== null) { e.preventDefault(); e.stopPropagation(); add(mark); }
                       else if (e.key === 'Enter' && !e.nativeEvent.isComposing) { e.preventDefault(); add(''); }
                   }}/>
            <p className="small muted log-keys"><Kbd>↩</Kbd> add · <Kbd>⌘1</Kbd> done <Kbd>⌘2</Kbd> next <Kbd>⌘3</Kbd> blocked · <Kbd>⇧↩</Kbd> a second row</p>
        </div>
    );
}

function StandupView({lines, html, onToast}: {lines: M.Line[]; html: string[]; onToast: (message: string) => void}) {
    const [copied, setCopied] = useState(false);
    const groups = M.standup(lines);
    const text = M.standupText(lines);
    if (M.GROUPS.every(([g]) => !groups[g].length)) return <p className="muted">Nothing on this day yet.</p>;

    async function copy() {
        try {
            await Notes.ReviewCopyText(text); // Go writes the clipboard: WebKit's async clipboard fails outside a click
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (e) {
            onToast(`Couldn't copy the standup: ${String((e as Error)?.message ?? e)}`);
        }
    }
    // A link in a line opens in the browser, never inside the app's window.
    const openLink = (e: MouseEvent) => {
        const a = (e.target as HTMLElement).closest('a');
        if (!a) return;
        e.preventDefault();
        if (/^(https?:|mailto:)/i.test(a.href)) Browser.OpenURL(a.href).catch(() => window.open(a.href, '_blank'));
    };
    return (
        <div className="standup" onClick={openLink}>
            {M.GROUPS.filter(([g]) => groups[g].length).map(([g, title]) => (
                <section key={g} aria-label={title}>
                    <h2 className="label">{title}</h2>
                    <ul className="standup-list">
                        {groups[g].map(i => <li key={i} dangerouslySetInnerHTML={{__html: html[i] || `<p>${esc(lines[i].text)}</p>`}}/>)}
                    </ul>
                </section>
            ))}
            <div><Button variant="line" size="sm" disabled={!text} onClick={() => void copy()}>{copied ? 'Copied ✓' : 'Copy standup'}</Button></div>
        </div>
    );
}
