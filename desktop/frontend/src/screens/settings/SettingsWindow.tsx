// The Settings window: tabs across the top, one pane below. Every change saves at once (no Save button); a save that fails
// shows a banner. Port of app/UI/SettingsWindow.swift and SettingsView.swift (docs/port/PARITY.md 3.M).
import {useCallback, useEffect, useRef, useState} from 'react';
import {Banner, Button, Tabs, useTheme} from '@personal/ui/react';
import {Notes} from '../../api';
import type {Overview, Settings} from '../../api';
import {AboutPane} from './AboutPane';
import {GeneralPane} from './GeneralPane';
import {PagePane} from './PagePane';
import {RemindersPane} from './RemindersPane';
import {errText, type Info, type Patch} from './shared';
import {ShortcutsPane} from './ShortcutsPane';
import {SourcesPane} from './SourcesPane';
import {StoragePane} from './StoragePane';

type Pane = 'general' | 'reminders' | 'page' | 'storage' | 'shortcuts' | 'sources' | 'about';
const PANES: [Pane, string][] = [['general', 'General'], ['reminders', 'Reminders'], ['page', 'Page'], ['storage', 'Storage'], ['shortcuts', 'Shortcuts'], ['sources', 'Sources'], ['about', 'About']];
const PANE_KEY = 'gloamlog.settingsPane';

function storedPane(): Pane {
    try {
        const p = localStorage.getItem(PANE_KEY);
        if (PANES.some(([id]) => id === p)) return p as Pane;
    } catch { /* storage can be off: start on General */ }
    return 'general';
}

export function SettingsWindow() {
    const [theme, setTheme] = useTheme();
    const [pane, setPane] = useState<Pane>(storedPane);
    const [s, setS] = useState<Settings | null>(null);
    const [info, setInfo] = useState<Info | null>(null);
    const [overview, setOverview] = useState<Overview | null>(null);
    const [error, setError] = useState('');
    const cur = useRef<Settings | null>(null);   // the newest settings, including edits still being saved
    const pending = useRef(0);                   // saves started and not finished
    const queue = useRef<Promise<unknown>>(Promise.resolve());

    const reload = useCallback(async () => {
        const [inf, ov] = await Promise.all([Notes.SettingsInfo(), Notes.Overview().catch(() => null)]);
        setInfo(inf);
        setOverview(ov);
        if (pending.current === 0) {   // never replace edits that are still on their way to the file
            const saved = await Notes.Settings();
            cur.current = saved;
            setS(saved);
        }
    }, []);

    useEffect(() => {
        reload().catch(e => setError(errText(e)));
        const again = () => { reload().catch(() => undefined); }; // reload shows its own error banner
        window.addEventListener('focus', again);
        return () => window.removeEventListener('focus', again);
    }, [reload]);

    // Edits show at once and are saved one after another, in order.
    const set = useCallback((patch: Patch) => {
        if (!cur.current) return;
        const next = {...cur.current, ...(typeof patch === 'function' ? patch(cur.current) : patch)} as Settings;
        cur.current = next;
        setS(next);
        pending.current++;
        queue.current = queue.current
            .then(() => Notes.SaveSettings(next))
            .then(saved => {
                setError('');
                if (--pending.current === 0) { cur.current = saved; setS(saved); }
            })
            .catch(e => { pending.current--; setError(errText(e)); });
    }, []);

    const go = useCallback((p: Pane) => {
        setPane(p);
        try { localStorage.setItem(PANE_KEY, p); } catch { /* fine */ }
    }, []);

    // Left and right arrows change pane when nothing is being typed.
    useEffect(() => {
        const key = (e: KeyboardEvent) => {
            if ((e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') || e.ctrlKey || e.altKey || e.shiftKey || e.metaKey) return;
            const t = e.target as HTMLElement | null;
            if (t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName))) return;
            const i = PANES.findIndex(([id]) => id === pane) + (e.key === 'ArrowRight' ? 1 : -1);
            if (i >= 0 && i < PANES.length) go(PANES[i][0]);
        };
        window.addEventListener('keydown', key);
        return () => window.removeEventListener('keydown', key);
    }, [pane, go]);

    const props = s && info ? {s, set, info, overview, reload: () => { reload().catch(() => undefined); }} : null; // reload shows its own error banner
    return (
        <div className="window">
            <header className="window-head settings-head">
                <Tabs items={PANES} current={pane} onChange={go}/>
            </header>
            <main className="window-body">
                {error && (
                    <div style={{marginBottom: 'var(--s5)'}}>
                        <Banner tone="danger" action={<Button size="sm" variant="line" onClick={() => { setError(''); reload().catch(() => undefined); }}/* reload shows its own error banner */>Show saved settings</Button>}>
                            <strong>Can't save.</strong> {error} Your last change may not be kept.
                        </Banner>
                    </div>
                )}
                {!props ? <p className="loading">Loading settings…</p> : (
                    <>
                        {pane === 'general' && <GeneralPane {...props} theme={theme} setTheme={setTheme}/>}
                        {pane === 'reminders' && <RemindersPane {...props}/>}
                        {pane === 'page' && <PagePane {...props}/>}
                        {pane === 'storage' && <StoragePane {...props}/>}
                        {pane === 'shortcuts' && <ShortcutsPane {...props}/>}
                        {pane === 'sources' && <SourcesPane {...props}/>}
                        {pane === 'about' && <AboutPane {...props}/>}
                    </>
                )}
            </main>
        </div>
    );
}
