// Pieces every Settings pane uses: the props a pane gets, a titled block, the "reset this pane" bar and a number field.
import {useEffect, useState, type ReactNode} from 'react';
import {Button, Dialog, SectionTitle} from '@personal/ui/react';
import type {Notes, Overview, Settings} from '../../api';

export type Info = Awaited<ReturnType<typeof Notes.SettingsInfo>>;
/** A change to the settings: the new values, or a function of the current ones (so quick edits never overwrite each other). */
export type Patch = Partial<Settings> | ((current: Settings) => Partial<Settings>);

export interface PaneProps {
    s: Settings;
    set: (patch: Patch) => void;
    info: Info;
    overview: Overview | null;
    /** Read the settings, folder and login item again. */
    reload: () => void;
}

export const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** 1 = Sunday ... 7 = Saturday, like the settings file. */
export const DAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

/** The weekday the system starts its week on (1 = Sunday); Monday when the browser can't say. */
export function systemWeekStart(): number {
    try {
        const l = new Intl.Locale(navigator.language) as Intl.Locale & {getWeekInfo?: () => {firstDay: number}; weekInfo?: {firstDay: number}};
        const first = (l.getWeekInfo?.() ?? l.weekInfo)?.firstDay; // 1 = Monday ... 7 = Sunday
        if (first) return (first % 7) + 1;
    } catch { /* an old engine: use Monday */ }
    return 2;
}

/** "/Users/me/Gloamlog" -> "~/Gloamlog". The home folder is the parent of the default folder. */
export function tilde(path: string, info: Info): string {
    const home = info.defaultFolder.replace(/\/[^/]*$/, '');
    return home && path.startsWith(home + '/') ? '~' + path.slice(home.length) : path;
}

export const DEFAULT_TEMPLATE = '## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next';
export const DEFAULT_CARRY = ['To do next', 'Pending / blocked'];

/** A group of settings under a title. */
export function Block({title, lede, children}: {title: string; lede?: ReactNode; children: ReactNode}) {
    return (
        <section style={{minWidth: 0}}>
            <SectionTitle>{title}</SectionTitle>
            {lede && <p className="lede small muted">{lede}</p>}
            {children}
        </section>
    );
}

/** The panes stack their blocks with this gap. */
export function Panel({children}: {children: ReactNode}) {
    return <div className="stack" style={{gap: 'var(--s7)'}}>{children}</div>;
}

export const Soon = ({children = 'Coming in the next build.'}: {children?: ReactNode}) => <span className="small muted">{children}</span>;

/** "Reset to defaults…" at the foot of a pane. Asks first; pages are never touched. */
export function ResetBar({pane, summary, onReset}: {pane: string; summary: string; onReset: () => void}) {
    const [open, setOpen] = useState(false);
    return (
        <div className="rule-top" style={{paddingTop: 'var(--s4)'}}>
            <div className="row-gap">
                <Button variant="line" size="sm" onClick={() => setOpen(true)}>Reset to defaults…</Button>
                <span className="small muted">Resets {summary}.</span>
            </div>
            <Dialog open={open} onClose={() => setOpen(false)} title={`Reset ${pane} settings?`}
                actions={<>
                    <Button variant="line" onClick={() => setOpen(false)}>Cancel</Button>
                    <Button onClick={() => { setOpen(false); onReset(); }}>Reset</Button>
                </>}>
                <p>This resets {summary}. Your pages are not touched.</p>
            </Dialog>
        </div>
    );
}

/** A whole number in a box. Valid numbers save as you type; leaving the box puts a number outside min..max back inside. */
export function NumberField({value, min, max, label, onCommit}: {value: number; min: number; max: number; label: string; onCommit: (n: number) => void}) {
    const [text, setText] = useState(String(value));
    useEffect(() => setText(t => (Number(t) === value ? t : String(value))), [value]);
    const clamp = (n: number) => Math.min(max, Math.max(min, Math.round(n)));
    return (
        <input className="input input-sm" type="number" inputMode="numeric" min={min} max={max} aria-label={label} value={text}
            style={{width: 'var(--s9)'}}
            onChange={e => {
                setText(e.target.value);
                const n = Number(e.target.value);
                if (e.target.value !== '' && Number.isFinite(n) && n >= min && n <= max) onCommit(Math.round(n));
            }}
            onBlur={() => {
                const n = text === '' || !Number.isFinite(Number(text)) ? value : clamp(Number(text));
                setText(String(n));
                if (n !== value) onCommit(n);
            }}/>
    );
}

/** A text box that saves when you leave it (or press Return). */
export function HeadingInput({value, placeholder, label, onCommit}: {value: string; placeholder: string; label: string; onCommit: (v: string) => void}) {
    const [text, setText] = useState(value);
    useEffect(() => setText(value), [value]);
    const commit = () => { if (text !== value) onCommit(text); };
    return <input className="input input-sm" aria-label={label} placeholder={placeholder} value={text} style={{width: 'calc(var(--s9) * 2)'}}
        onChange={e => setText(e.target.value)} onBlur={commit} onKeyDown={e => { if (e.key === 'Enter') commit(); }}/>;
}
