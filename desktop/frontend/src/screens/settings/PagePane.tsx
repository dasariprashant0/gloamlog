import {useEffect, useRef, useState} from 'react';
import {Button, Field, Setting} from '@personal/ui/react';
import {Block, DEFAULT_CARRY, DEFAULT_TEMPLATE, NumberField, Panel, ResetBar, type PaneProps} from './shared';

const WINDOWS = [7, 14, 30, 60, 90, 180, 365];
const windowLabel = (d: number) => (d === 365 ? '1 year' : `${d} days`);
const pad = (n: number) => String(n).padStart(2, '0');
const localDay = () => { const d = new Date(); return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`; };

const PRESETS: {title: string; markdown: string; carry: string[]}[] = [
    {title: 'Daily review', markdown: DEFAULT_TEMPLATE, carry: DEFAULT_CARRY},
    {title: 'Standup', markdown: '## Yesterday\n\n## Today\n\n## Blockers', carry: ['Today', 'Blockers']},
    {title: 'Blank page', markdown: '', carry: DEFAULT_CARRY},
];

export function PagePane({s, set, overview}: PaneProps) {
    const today = overview?.today ?? localDay();
    const windows = WINDOWS.includes(s.catchUpWindowDays) ? WINDOWS : [...WINDOWS, s.catchUpWindowDays].sort((a, b) => a - b);
    const pinned = !!s.logStartDate;

    return (
        <Panel>
            <Block title="Logging">
                <Setting title="A day counts as logged after" hint="Headings, empty list items and images don't count as words. A page with fewer words shows as partly written.">
                    <div className="row-gap">
                        <NumberField label="Words to log a day" value={s.minWords} min={1} max={500} onCommit={n => set({minWords: n})}/>
                        <span className="small muted">words</span>
                    </div>
                </Setting>
            </Block>
            <Block title="Catching up">
                <Setting title="Log start date" hint={pinned
                    ? 'Days before this date never count as unlogged and never break your streak. Pages you write earlier are still saved.'
                    : 'Following your first log. Days before it never count as unlogged and never break your streak.'}>
                    <div className="row-gap">
                        <input className="input input-sm" type="date" aria-label="Log start date" min="2000-01-01" max={today}
                            value={s.logStartDate ?? ''} style={{width: 'auto'}}
                            onChange={e => { if (e.target.value >= '2000-01-01' && e.target.value <= today) set({logStartDate: e.target.value}); }}/>
                        <Button variant="line" size="sm" disabled={!pinned} title="Follow the earliest page in your log folder" onClick={() => set({logStartDate: ''})}>Use first log</Button>
                        <Button variant="line" size="sm" disabled={s.logStartDate === today} title="Ignore every day before today" onClick={() => set({logStartDate: today})}>Start from today</Button>
                    </div>
                </Setting>
                <Setting title="Catch-up window" hint="How far back Catch up looks for days you haven't written. Older days stay reachable from the calendar.">
                    <select className="select" aria-label="Catch-up window" style={{width: 'auto'}} value={s.catchUpWindowDays}
                        onChange={e => set({catchUpWindowDays: Number(e.target.value)})}>
                        {windows.map(d => <option key={d} value={d}>{windowLabel(d)}</option>)}
                    </select>
                </Setting>
            </Block>
            <Block title="New day template">
                <TemplateEditor template={s.template} onTemplate={template => set({template})} onPreset={(template, carryOverHeadings) => set({template, carryOverHeadings})}
                    isDefault={s.template === DEFAULT_TEMPLATE && s.carryOverHeadings.join('\n') === DEFAULT_CARRY.join('\n')}/>
            </Block>
            <Block title="Carried over from yesterday">
                <CarryHeadings headings={s.carryOverHeadings} onChange={carryOverHeadings => set({carryOverHeadings})}/>
            </Block>
            <ResetBar pane="Page" summary="the word count, catch-up window, log start, template and headings"
                onReset={() => set({minWords: 20, catchUpWindowDays: 30, logStartDate: '', template: DEFAULT_TEMPLATE, carryOverHeadings: DEFAULT_CARRY})}/>
        </Panel>
    );
}

/** The template as plain markdown. Typing saves half a second after you stop. */
function TemplateEditor({template, onTemplate, onPreset, isDefault}: {template: string; onTemplate: (t: string) => void; onPreset: (t: string, c: string[]) => void; isDefault: boolean}) {
    const [text, setText] = useState(template);
    const timer = useRef<number | undefined>(undefined);
    // A reset or a preset shows up here, unless you are in the middle of typing.
    useEffect(() => { if (timer.current === undefined) setText(template); }, [template]);
    const edit = (v: string) => {
        setText(v);
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => { timer.current = undefined; onTemplate(v); }, 500);
    };
    const apply = (markdown: string, carry: string[]) => {
        window.clearTimeout(timer.current);
        timer.current = undefined;
        setText(markdown);
        onPreset(markdown, carry);
    };
    return (
        <div className="stack">
            <div className="row-gap">
                <span className="small muted">Start from</span>
                {PRESETS.map(p => <Button key={p.title} variant="line" size="sm" onClick={() => apply(p.markdown, p.carry)}>{p.title}</Button>)}
                <Button variant="ghost" size="sm" disabled={isDefault} onClick={() => apply(DEFAULT_TEMPLATE, DEFAULT_CARRY)}>Reset to default</Button>
            </div>
            <Field label="Template, in markdown" hint="Pre-filled into a new day's page. Nothing is saved until you start writing. Use ## for headings and - [ ] for to-dos. Pages you already wrote are never changed.">
                <textarea className="textarea" rows={7} spellCheck={false} value={text} onChange={e => edit(e.target.value)}/>
            </Field>
        </div>
    );
}

/** The headings "From yesterday" carries over. Edits save when you leave the box. */
function CarryHeadings({headings, onChange}: {headings: string[]; onChange: (h: string[]) => void}) {
    const next = useRef(0);
    const make = (list: string[]) => list.map(text => ({id: next.current++, text}));
    const [items, setItems] = useState(() => make(headings));
    const clean = (list: {text: string}[]) => list.map(i => i.text.trim()).filter(Boolean);
    // A reset or a preset shows up here; an empty row you just added stays.
    useEffect(() => { if (clean(items).join('\n') !== headings.join('\n')) setItems(make(headings)); }, [headings]); // eslint-disable-line react-hooks/exhaustive-deps
    const commit = (list: {text: string}[]) => { if (clean(list).join('\n') !== headings.join('\n')) onChange(clean(list)); };
    return (
        <div className="stack">
            <ul className="list">
                {items.map(item => (
                    <li key={item.id} className="row-gap" style={{padding: 'var(--s2) 0', flexWrap: 'nowrap'}}>
                        <input className="input input-sm" aria-label="Heading to carry over" placeholder="Heading" value={item.text}
                            onChange={e => setItems(cur => cur.map(i => (i.id === item.id ? {...i, text: e.target.value} : i)))}
                            onBlur={() => commit(items)}/>
                        <Button variant="ghost" size="sm" onClick={() => { const rest = items.filter(i => i.id !== item.id); setItems(rest); commit(rest); }}>Remove</Button>
                    </li>
                ))}
            </ul>
            <div className="row-gap"><Button variant="line" size="sm" onClick={() => setItems(cur => [...cur, ...make([''])])}>Add heading</Button></div>
            <p className="small muted">From yesterday offers the lines written under these headings (a heading's case and emoji don't matter).</p>
        </div>
    );
}
