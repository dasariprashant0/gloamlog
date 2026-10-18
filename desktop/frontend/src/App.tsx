// The main window: sidebar on the left, the routed page on the right.
import {useCallback, useEffect, useRef, useState} from 'react';
import {Banner, Empty, Shell, Toast} from '@personal/ui/react';
import {Notes, Windows, addDays, type Overview} from './api';
import {Sidebar, type Route} from './components/Sidebar';
import {DayPage} from './screens/DayPage';
import {SearchView} from './screens/SearchView';
import {CatchUpView} from './screens/CatchUp';
import {SessionBar} from './screens/SessionBar';
import {SkipDialog} from './screens/SkipDialog';
import {RestoreDialog} from './screens/RestoreDialog';

type ToastState = {message: string; undo?: () => void} | null;

export function App() {
    const [overview, setOverview] = useState<Overview | null>(null);
    const [error, setError] = useState('');
    const [minWords, setMinWords] = useState(20);
    const [route, setRoute] = useState<Route | null>(null);
    const [day, setDay] = useState<string | null>(null);   // the day page keeps its last day while another screen shows
    const [query, setQuery] = useState('');
    const [toast, setToast] = useState<ToastState>(null);
    const [version, setVersion] = useState(0);              // bumps after every save, skip or settings change
    const [session, setSession] = useState<string[] | null>(null);   // catch-up: the days fixed at its start
    const [finished, setFinished] = useState<{logged: number; skipped: number} | undefined>();
    const [skipDays, setSkipDays] = useState<string[] | null>(null);
    const [restoreDay, setRestoreDay] = useState<string | null>(null);
    const [pageKey, setPageKey] = useState(0);
    const searchRef = useRef<HTMLInputElement>(null);
    const before = useRef<Route | null>(null);

    const reload = useCallback(() => {
        Notes.Overview().then(o => { setOverview(o); setError(''); }, e => setError(String(e?.message ?? e)));
        Notes.Settings().then(s => setMinWords(s.minWords), () => {});
        setVersion(v => v + 1);
    }, []);
    useEffect(reload, [reload]);
    // Reload on return to the window (another app may have written the folder) and every 30 s (day rollover).
    useEffect(() => {
        const t = setInterval(reload, 30_000);
        window.addEventListener('focus', reload);
        return () => { clearInterval(t); window.removeEventListener('focus', reload); };
    }, [reload]);

    const today = overview?.today ?? '';
    useEffect(() => {
        if (today && !route) go({kind: 'day', day: today});
    }, [today]); // eslint-disable-line react-hooks/exhaustive-deps

    function go(r: Route) {
        if (r.kind === 'day') setDay(r.day);
        if (r.kind !== 'day') setSession(null);
        if (r.kind === 'catchup') setFinished(undefined);
        if (r.kind !== 'search') setQuery('');
        setRoute(r);
    }
    function onQuery(q: string) {
        setQuery(q);
        if (q.trim() && route?.kind !== 'search') { before.current = route; setRoute({kind: 'search'}); }
        if (!q.trim() && route?.kind === 'search') setRoute(before.current ?? {kind: 'day', day: today});
    }
    const notify = useCallback((message: string, undo?: () => void) => setToast({message, undo}), []);

    // A catch-up session ends with nothing left: count how its days ended up, then show "All caught up".
    async function finishSession() {
        const days = session ?? [];
        const pages = await Promise.all(days.map(d => Notes.Open(d).catch(() => null)));
        setFinished({logged: pages.filter(p => p?.status === 'logged').length, skipped: pages.filter(p => p?.isSkipped).length});
        setSession(null);
        setRoute({kind: 'catchup'});
        reload();
    }

    // Keyboard: ⌘T / ⌘1 today, ⌘2 catch up, ⌘3 review, ⌘[ ⌘] previous / next day, ⌘F search, ⌘, settings.
    useEffect(() => {
        const k = (e: KeyboardEvent) => {
            if (!e.metaKey || e.altKey || e.ctrlKey) return;
            const keys: Record<string, () => void> = {
                t: () => go({kind: 'day', day: today}), 1: () => go({kind: 'day', day: today}),
                2: () => go({kind: 'catchup'}), 3: () => go({kind: 'review'}),
                '[': () => day && go({kind: 'day', day: addDays(day, -1)}),
                ']': () => day && day < today && go({kind: 'day', day: addDays(day, 1)}),
                f: () => searchRef.current?.focus(), ',': () => void Windows.ShowSettings(),
            };
            const f = !e.shiftKey && keys[e.key];
            if (f) { e.preventDefault(); f(); }
        };
        window.addEventListener('keydown', k);
        return () => window.removeEventListener('keydown', k);
    });

    const r = route ?? {kind: 'day', day: today};
    const side = <Sidebar overview={overview} route={r} query={query} searchRef={searchRef} version={version} onQuery={onQuery} onGo={go}/>;

    return (
        <Shell side={side}>
            {error && <Banner tone="danger"><strong>Gloamlog can't read your log folder.</strong> {error}</Banner>}
            {session && day && r.kind === 'day' && (
                <SessionBar days={session} current={day} onGo={d => go({kind: 'day', day: d})} onEnd={() => go({kind: 'catchup'})}
                            onSkip={d => setSkipDays([d])} onFinish={finishSession}/>
            )}
            {day && overview && (
                <DayPage day={day} today={today} minWords={minWords} active={r.kind === 'day'} folderError={overview.folderError}
                         reloadKey={pageKey} onDay={d => go({kind: 'day', day: d})} onSaved={reload}
                         onSkip={setSkipDays} onRestore={setRestoreDay} onToast={notify}/>
            )}
            {r.kind === 'search' && <SearchView query={query} onOpen={d => go({kind: 'day', day: d})}/>}
            {r.kind === 'catchup' && overview && (
                <CatchUpView overview={overview} finished={finished} onToast={notify} onChanged={reload}
                             onOpenDay={d => go({kind: 'day', day: d})}
                             onStartSession={days => { setSession(days); setDay(days[0]); setRoute({kind: 'day', day: days[0]}); }}
                             onReview={() => go({kind: 'review'})} onOpenCalendar={() => go({kind: 'day', day: today})}
                             onLogStart={() => void Windows.ShowSettings()}/>
            )}
            {r.kind === 'review' && <Empty title={['Review']}>The week, month and year review arrives in the next build.</Empty>}
            {skipDays && <SkipDialog open days={skipDays} today={today} onClose={() => setSkipDays(null)}
                        onSkipped={(days, reason) => {
                            reload();
                            setPageKey(k => k + 1);
                            notify(days.length === 1 ? `Skipped ${days[0] === today ? 'today' : 'this day'}${reason ? ' as ' + reason : ''}.` : `Skipped ${days.length} days${reason ? ' as ' + reason : ''}.`,
                                () => Notes.Unskip(days).then(() => { reload(); setPageKey(k => k + 1); }));
                        }}/>}
            {restoreDay && <RestoreDialog open day={restoreDay} onClose={() => setRestoreDay(null)}
                           onRestored={() => { setPageKey(k => k + 1); reload(); notify('Restored the earlier version. The one it replaced is under Restore previous version.'); }}/>}
            {toast && <Toast message={toast.message} action={toast.undo ? 'Undo' : undefined}
                             onAction={() => { toast.undo?.(); setToast(null); }} onDone={() => setToast(null)}/>}
        </Shell>
    );
}
