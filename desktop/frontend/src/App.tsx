// The main window: sidebar on the left, the routed page on the right.
import {useCallback, useEffect, useRef, useState} from 'react';
import {Banner, Shell, Toast} from '@personal/ui/react';
import {Notes, Windows, addDays, type Overview} from './api';
import {Sidebar, type Route} from './components/Sidebar';
import {DayPage} from './screens/DayPage';
import {SearchView} from './screens/SearchView';
import {CatchUpView} from './screens/CatchUp';
import {SessionBar} from './screens/SessionBar';
import {SkipDialog} from './screens/SkipDialog';
import {RestoreDialog} from './screens/RestoreDialog';
import {Review} from './screens/Review';
import {Onboarding} from './screens/Onboarding';
import {WhatsNew} from './screens/WhatsNew';
import {UpdateDialog} from './screens/UpdateDialog';
import {ConflictDialog} from './screens/ConflictDialog';
import {ConflictBanner, SyncChip, useSyncState} from './screens/SyncChip';
import {on} from './events';

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
    const [onboarding, setOnboarding] = useState<'first' | 'again' | null>(null);
    const [whatsNew, setWhatsNew] = useState(false);
    const [updateOpen, setUpdateOpen] = useState(false);
    const [conflictsOpen, setConflictsOpen] = useState(false);
    const [syncKey, setSyncKey] = useState(0);
    const sync = useSyncState(syncKey);
    const searchRef = useRef<HTMLInputElement>(null);
    const before = useRef<Route | null>(null);

    const reload = useCallback(() => {
        Notes.Overview().then(o => { setOverview(o); setError(''); }, e => setError(String(e?.message ?? e)));
        Notes.Settings().then(s => setMinWords(s.minWords), () => {}); // keep the default 20; the overview banner reports folder trouble
        setVersion(v => v + 1);
    }, []);
    useEffect(reload, [reload]);
    // Reload on return to the window (another app may have written the folder) and every 30 s (day rollover).
    useEffect(() => {
        const t = setInterval(reload, 30_000);
        window.addEventListener('focus', reload);
        return () => { clearInterval(t); window.removeEventListener('focus', reload); };
    }, [reload]);

    // First run: onboarding. An existing user on a new version: What's new, once.
    const checkedIntro = useRef(false);
    useEffect(() => {
        if (!overview || checkedIntro.current) return;
        checkedIntro.current = true;
        if (!overview.onboarded) setOnboarding('first');
        else Notes.WhatsNewSeen().then(seen => !seen && setWhatsNew(true), () => {}); // not knowing: don't nag
    }, [overview]);

    // Events from Go: the folder changed (sync), a note was captured, the tray asks for today, a reminder opens a day.
    useEffect(() => {
        const offs = [
            on('folder-changed', () => { reload(); setSyncKey(k => k + 1); setPageKey(k => k + 1); }),
            on('capture-changed', reload),
            on('open-today', () => Notes.Overview().then(o => go({kind: 'day', day: o.today}), () => {})), // the folder banner already says why
            on<string>('reminder-open-day', d => go({kind: 'day', day: d})),
            on('check-updates', () => setUpdateOpen(true)),
            on('run-setup', () => setOnboarding('again')),
        ];
        return () => offs.forEach(off => off());
    }, [reload]); // eslint-disable-line react-hooks/exhaustive-deps

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
        const pages = await Promise.all(days.map(d => Notes.Open(d).catch(() => null)) /* a summary count only */);
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
            if (e.shiftKey && e.key.toLowerCase() === 't') { // ⇧⌘T: go to date
                e.preventDefault();
                if (day) go({kind: 'day', day});
                setTimeout(() => document.getElementById('jump')?.focus(), 0);
                return;
            }
            const f = !e.shiftKey && keys[e.key];
            if (f) { e.preventDefault(); f(); }
        };
        window.addEventListener('keydown', k);
        return () => window.removeEventListener('keydown', k);
    });

    const r = route ?? {kind: 'day', day: today};
    const side = <Sidebar overview={overview} route={r} query={query} searchRef={searchRef} version={version} onQuery={onQuery} onGo={go}
                          foot={sync && <SyncChip state={sync} onReview={() => setConflictsOpen(true)} onOpenStorage={() => void Windows.ShowSettings()}/>}/>;

    return (
        <Shell side={side}>
            {error && <Banner tone="danger"><strong>Gloamlog can't read your log folder.</strong> {error}</Banner>}
            {sync && <ConflictBanner state={sync} folder={overview?.folder ?? ''} onReview={() => setConflictsOpen(true)}/>}
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
            {r.kind === 'review' && today && <Review today={today} onToast={notify} onOpenDay={d => go({kind: 'day', day: d})}
                                                     onCatchUp={() => go({kind: 'catchup'})}/>}
            {skipDays && <SkipDialog open days={skipDays} today={today} onClose={() => setSkipDays(null)}
                        onSkipped={(days, reason) => {
                            reload();
                            setPageKey(k => k + 1);
                            notify(days.length === 1 ? `Skipped ${days[0] === today ? 'today' : 'this day'}${reason ? ' as ' + reason : ''}.` : `Skipped ${days.length} days${reason ? ' as ' + reason : ''}.`,
                                () => Notes.Unskip(days).then(() => { reload(); setPageKey(k => k + 1); }));
                        }}/>}
            {restoreDay && <RestoreDialog open day={restoreDay} onClose={() => setRestoreDay(null)}
                           onRestored={() => { setPageKey(k => k + 1); reload(); notify('Restored the earlier version. The one it replaced is under Restore previous version.'); }}/>}
            {onboarding && <Onboarding open firstRun={onboarding === 'first'}
                                       onDone={() => { setOnboarding(null); reload(); setPageKey(k => k + 1); if (today) go({kind: 'day', day: today}); }}/>}
            <WhatsNew open={whatsNew} onClose={() => setWhatsNew(false)} onSettings={() => { setWhatsNew(false); void Windows.ShowSettings(); }}/>
            {updateOpen && <UpdateDialog open onClose={() => setUpdateOpen(false)}/>}
            {conflictsOpen && <ConflictDialog open onClose={() => setConflictsOpen(false)}
                                              onResolved={() => { reload(); setSyncKey(k => k + 1); setPageKey(k => k + 1); }}/>}
            {toast && <Toast message={toast.message} action={toast.undo ? 'Undo' : undefined}
                             onAction={() => { toast.undo?.(); setToast(null); }} onDone={() => setToast(null)}/>}
        </Shell>
    );
}
