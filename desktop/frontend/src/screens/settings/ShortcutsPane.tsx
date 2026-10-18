import {useEffect, useState} from 'react';
import {Button, Kbd, Row, SectionTitle, Setting, Switch} from '@personal/ui/react';
import {Block, HeadingInput, Panel, type PaneProps} from './shared';
import {DEFAULT_HOT_KEY, hotKeyAllowed, hotKeyLabel, hotKeyOf} from './keys';

// The list follows the menus (docs/port/PARITY.md 3.P). Each of these is also a menu item.
const GROUPS: {title: string; items: [string, string][]}[] = [
    {title: 'Go', items: [['Today', '⌘T  ⌘1'], ['Catch up', '⌘2'], ['Week review', '⌘3'], ['Previous day', '⌘['], ['Next day', '⌘]'], ['Go to date…', '⇧⌘T'],
        ['Next unlogged day', '⌘↩'], ['Previous unlogged day', '⌥⌘↑'], ['Month review', '⌘4'], ['Year review', '⌘5'], ['Export…', '⇧⌘E']]},
    {title: 'Page', items: [['Focus the page', '⌘E'], ['Save now', '⌘S'], ['Carry over from yesterday', '⇧⌘Y'], ['Skip day…', '⇧⌘K'], ['Remind me later', '⇧⌘L']]},
    {title: 'Search and copy', items: [['Search logs', '⌘F'], ['Next search result', '⌘G'], ['Previous search result', '⇧⌘G'], ['Copy as markdown', '⇧⌘C'], ['Open log folder in Finder', '⇧⌘O']]},
    {title: 'Settings', items: [['Open Settings', '⌘,'], ['Switch pane', '←  →']]},
];

export function ShortcutsPane({s, set}: PaneProps) {
    const [recording, setRecording] = useState(false);
    const [hint, setHint] = useState('');
    const key = s.capture.hotKey;

    // Keys are read only while this window is in front and recording is on; clicking away stops it.
    useEffect(() => {
        if (!recording) return;
        const down = (e: KeyboardEvent) => {
            e.preventDefault();
            e.stopPropagation();
            if (e.repeat || ['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) return;
            if (e.code === 'Escape' && !(e.ctrlKey || e.altKey || e.shiftKey || e.metaKey)) return setRecording(false);
            const k = hotKeyOf(e);
            if (!k) return setHint("That key can't be used.");
            if (!hotKeyAllowed(k)) return setHint("Include ⌃ or ⌘ so it can't clash with typing.");
            setRecording(false);
            set(cur => ({capture: {...cur.capture, hotKey: k}}));
        };
        const stop = () => setRecording(false);
        window.addEventListener('keydown', down, true);
        window.addEventListener('blur', stop);
        return () => { window.removeEventListener('keydown', down, true); window.removeEventListener('blur', stop); };
    }, [recording, set]);

    const setHotKey = (hotKey: typeof key) => { setRecording(false); setHint(''); set(cur => ({capture: {...cur.capture, hotKey}})); };
    const toggle = (field: 'timestamps' | 'todoShorthand' | 'jotsCountTowardLogged') => (v: boolean) => set(cur => ({capture: {...cur.capture, [field]: v}}));
    const on = (v: boolean) => <span className="muted small">{v ? 'On' : 'Off'}</span>;
    const isDefault = !!key && key.keyCode === DEFAULT_HOT_KEY.keyCode && key.carbonModifiers === DEFAULT_HOT_KEY.carbonModifiers;

    return (
        <Panel>
            <Block title="Jot from anywhere">
                <Setting title="Shortcut" hint={recording ? (hint || 'Press the new shortcut. Escape cancels.')
                    : key ? "Press it in any app to add a line to today's page." : 'Off. Use Jot… in the menu bar item.'}>
                    <div className="row-gap">
                        <Button variant={recording ? 'solid' : 'line'} onClick={() => { setHint(''); setRecording(r => !r); }}>
                            {recording ? 'Press the new shortcut' : key ? hotKeyLabel(key) : 'Off'}
                        </Button>
                        <Button variant="ghost" size="sm" disabled={isDefault} onClick={() => setHotKey({...DEFAULT_HOT_KEY})}>Reset</Button>
                        <Button variant="ghost" size="sm" disabled={!key} onClick={() => setHotKey(null)}>Off</Button>
                    </div>
                </Setting>
                <Setting title="Time stamp" hint="Starts each jot with the time, like 14:32.">
                    <Switch checked={s.capture.timestamps} onChange={toggle('timestamps')} label={on(s.capture.timestamps)}/>
                </Setting>
                <Setting title="[] makes a to-do" hint="A jot that starts with [] becomes a task.">
                    <Switch checked={s.capture.todoShorthand} onChange={toggle('todoShorthand')} label={on(s.capture.todoShorthand)}/>
                </Setting>
                <Setting title="Jots count toward logging a day" hint="Off: a day with only jots still reads as started.">
                    <Switch checked={s.capture.jotsCountTowardLogged} onChange={toggle('jotsCountTowardLogged')} label={on(s.capture.jotsCountTowardLogged)}/>
                </Setting>
                <Setting title="Jots go under" hint="The heading quick notes and Claude's entries are added under. Type What I did to keep them with the rest of your day.">
                    <HeadingInput value={s.capture.jotsHeading} placeholder="Jots" label="Jots go under this heading"
                        onCommit={v => set(cur => ({capture: {...cur.capture, jotsHeading: v}}))}/>
                </Setting>
            </Block>
            <div className="two" style={{gap: 'var(--s6)'}}>
                {[[GROUPS[0], GROUPS[3]], [GROUPS[1], GROUPS[2]]].map((col, i) => (
                    <div key={i} className="stack" style={{gap: 'var(--s6)', alignContent: 'start'}}>
                        {col.map(g => (
                            <section key={g.title}>
                                <SectionTitle>{g.title}</SectionTitle>
                                <ul className="list">
                                    {g.items.map(([name, keys]) => (
                                        <li key={name}><Row title={name} end={<span className="row-gap" style={{justifyContent: 'flex-end'}}>{keys.split('  ').map(k => <Kbd key={k}>{k}</Kbd>)}</span>}/></li>
                                    ))}
                                </ul>
                            </section>
                        ))}
                    </div>
                ))}
            </div>
            <p className="small muted">Every shortcut here is also a menu item, with the shortcut shown beside it.</p>
        </Panel>
    );
}
