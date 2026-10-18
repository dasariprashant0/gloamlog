// One day's page: header (date, progress, saved state), From yesterday, the editor, and its banners.
// Saving rules (Swift DayEditor): only real edits are written, 600 ms after the last change and at once on day switch, on
// leaving the page and on window blur; saves run one at a time; opening a page never writes it.
import {useCallback, useEffect, useRef, useState} from 'react';
import {Banner, Button, Display} from '@personal/ui/react';
import {Browser} from '@wailsio/runtime';
import {Notes, Windows, plural, relativeDay, type Page} from '../api';
import {Editor, editor, type EditorMessage} from '../editor/Editor';
import {SuggestionsStrip} from './SuggestionsStrip';
import {DownloadingPage} from './SyncChip';
import {ReminderNotice} from './ReminderNotice';
import type {ReminderNoticeState} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import {on} from '../events';

const SAVE_DELAY = 600;

export function DayPage({day, today, minWords, active, folderError, reloadKey, onDay, onSaved, onSkip, onRestore, onToast}: {
    day: string; today: string; minWords: number; active: boolean; folderError: string; reloadKey: number; // bump to re-read the day from disk
    onDay: (day: string) => void; onSaved: () => void; onSkip: (days: string[]) => void; onRestore: (day: string) => void;
    onToast: (message: string, undo?: () => void) => void;
}) {
    const [page, setPage] = useState<Page | null>(null);
    const [words, setWords] = useState(0);
    const [savedAt, setSavedAt] = useState(0);
    const [saveError, setSaveError] = useState('');
    const [loadError, setLoadError] = useState('');
    const [raw, setRaw] = useState(false);
    const [writeAnyway, setWriteAnyway] = useState(false);

    const loaded = useRef<string | null>(null);   // the day whose text is in the editor (null: none, or a skipped page)
    const baseline = useRef('');                  // the editor's text right after loading: anything else is an edit
    const lastSaved = useRef('');                 // what the file holds now, as the editor writes it
    const pending = useRef<{day: string; md: string} | null>(null);
    const timer = useRef(0);
    const chain = useRef<Promise<unknown>>(Promise.resolve());
    const appending = useRef(0);                  // lines being added by the suggestions strip, for the reply

    const save = useCallback((d: string, md: string) => {
        chain.current = chain.current.then(() => Notes.Save(d, md)).then(out => {
            if (loaded.current === d) lastSaved.current = md;
            void Notes.EditorSaved(d, md); // capture: jots now on the page are done
            setSaveError('');
            setSavedAt(Date.now());
            if (out?.backupWarning) onToast(out.backupWarning);
            onSaved();
        }, err => setSaveError(String(err?.message ?? err)));
        return chain.current;
    }, [onSaved, onToast]);

    // Writes whatever is unsaved now: a scheduled save, or text typed so recently the editor has not reported it yet.
    const flush = useCallback(() => {
        clearTimeout(timer.current);
        const p = pending.current;
        pending.current = null;
        const d = loaded.current;
        if (d && !editor.isRawFallback()) {
            const md = editor.getMarkdown();
            if (md !== baseline.current && md !== lastSaved.current) return save(d, md);
        } else if (p) return save(p.day, p.md);
        return chain.current;
    }, [save]);

    // Open the day: save the old one first, then swap the document in the editor.
    useEffect(() => {
        let live = true;
        (async () => {
            const previous = loaded.current;
            await flush();
            if (previous && previous !== day) void Notes.EditorClosed(previous);
            void Notes.EditorOpened(day); // from now on a jot for this day comes through the editor, not the file
            const p = await Notes.Open(day);
            if (!live) return;
            setPage(p);
            setWords(p.words);
            setRaw(false);
            setLoadError('');
            setWriteAnyway(false);
            loaded.current = null;
            if (!p.isSkipped) show(p.day, p.body, p.exists, p.notLocal);
        })().catch(e => live && setLoadError(String(e?.message ?? e)));
        return () => { live = false; };
    }, [day, reloadKey]); // eslint-disable-line react-hooks/exhaustive-deps

    function show(d: string, body: string, exists: boolean, readOnly = false) {
        editor.setMarkdown(body, {cursor: 'start'});
        editor.setReadOnly(readOnly);
        baseline.current = editor.getMarkdown();
        lastSaved.current = exists ? baseline.current : '';
        loaded.current = d;
        // The document's token: appendToSection refuses ("stale") when the page in the editor is no longer this day.
        (window as unknown as {__dlDoc?: string}).__dlDoc = d;
    }

    // Adds lines under a heading as one undoable step in the live page; the editor replies with appendResult.
    const insertUnder = useCallback((heading: string, markdown: string, count: number) => {
        const d = loaded.current;
        if (!d) return;
        appending.current = count;
        editor.appendToSection(`add-${Date.now()}`, d, heading, markdown, {unlessPresent: true});
    }, []);

    // Jots for the open day arrive through the editor so the next autosave can't write over them (capture.go).
    useEffect(() => on<{id: string; day: string; heading: string; markdown: string}>('jot-into-page', j => {
        if (loaded.current === j.day && !editor.isRawFallback()) editor.appendToSection(j.id, j.day, j.heading, j.markdown, {unlessPresent: true});
        else void Notes.JotPlaced(j.id, 'notLoaded', '');
    }), []);

    // Today's reminder notice (reminders.go); dismissed stays dismissed until the next notice.
    const [notice, setNotice] = useState<ReminderNoticeState | null>(null);
    const [noticeGone, setNoticeGone] = useState(false);
    useEffect(() => {
        Notes.CurrentReminderNotice().then(setNotice, () => {}); // no notice is the normal state
        return on<ReminderNoticeState>('reminder-notice', n => { setNotice(n); setNoticeGone(false); });
    }, []);

    // Leaving the page (search, catch up, review) or the window: write now.
    useEffect(() => { if (!active) void flush(); }, [active, flush]);
    useEffect(() => {
        const out = () => void flush();
        window.addEventListener('blur', out);
        window.addEventListener('beforeunload', out);
        return () => { window.removeEventListener('blur', out); window.removeEventListener('beforeunload', out); };
    }, [flush]);

    const onMessage = useCallback((m: EditorMessage) => {
        switch (m.type) {
            case 'ready':
                // The editor replays queued calls right after this message; take the baseline once it has.
                setTimeout(() => { if (loaded.current) { baseline.current = editor.getMarkdown(); if (lastSaved.current) lastSaved.current = baseline.current; } }, 0);
                break;
            case 'change': {
                const d = loaded.current;
                if (!d) return;
                pending.current = {day: d, md: m.markdown};
                clearTimeout(timer.current);
                timer.current = window.setTimeout(() => {
                    const p = pending.current;
                    pending.current = null;
                    if (p && p.md !== lastSaved.current) void save(p.day, p.md);
                }, SAVE_DELAY);
                Notes.Words(m.markdown).then(setWords, () => {}); // the count is a hint: on failure the old one stays
                break;
            }
            case 'uploadImage': {
                const d = loaded.current ?? day;
                Notes.SaveImage(d, m.name, m.mime, m.dataBase64)
                    .then(path => editor._resolveUpload(m.id, path, null), err => editor._resolveUpload(m.id, null, String(err?.message ?? err)));
                break;
            }
            case 'openLink':
                if (/^(https?:|mailto:)/i.test(m.url)) Browser.OpenURL(m.url).catch(() => window.open(m.url, '_blank'));
                break;
            case 'appendResult': {
                const d = loaded.current;
                if (!m.id.startsWith('add-')) { // a jot from the panel or Claude (capture.go asked via jot-into-page)
                    void Notes.JotPlaced(m.id, m.result, m.markdown ?? '');
                    if (m.result === 'appended' && d && m.markdown != null) { void save(d, m.markdown); Notes.Words(m.markdown).then(setWords, () => {}); }
                    break;
                }
                if (m.result === 'appended' && d && m.markdown != null) {
                    void save(d, m.markdown);
                    Notes.Words(m.markdown).then(setWords, () => {}); // an append emits no change event
                    onToast(`Added ${plural(appending.current, 'line')}.`);
                } else if (m.result === 'alreadyPresent') onToast('Those lines are already on the page.');
                else onToast('The page changed while adding. Try again.');
                break;
            }
            case 'rawFallback':
                setRaw(true);
                break;
            case 'log':
                // console.info, never console.error: the engine forwards console.error here, which would loop.
                if (m.level === 'error') console.info('[editor error]', m.message);
                break;
        }
    }, [day, save, onToast]);

    async function carryOver() {
        if (!page?.carry) return;
        const before = editor.getMarkdown();
        const after = await Notes.CarryOver(page.carry, before);
        editor.setMarkdown(after, {cursor: 'keep'});
        await save(day, editor.getMarkdown());
        setPage({...page, carryApplied: true});
        onToast('Carried over from yesterday.', async () => {
            editor.setMarkdown(before, {cursor: 'keep'});
            await save(day, editor.getMarkdown());
            setPage(p => p && {...p, carryApplied: false});
        });
    }

    async function undoSkip() {
        await Notes.Unskip([day]);
        onSaved();
        const p = await Notes.Open(day);
        setPage(p);
        show(p.day, p.body, p.exists);
    }

    const skipped = !!page?.isSkipped && !writeAnyway;
    const [weekday, ...rest] = (page?.title ?? '').split(' ');
    const label = day === today ? 'Today' : relativeDay(day, today);
    const recentlySaved = Date.now() - savedAt < 2200;

    return (
        <article className="page" hidden={!active}>
            <PageNav day={day} today={today} onDay={onDay}/>
            {folderError && <Banner tone="danger"><strong>Gloamlog can't save to your log folder.</strong> {folderError}</Banner>}
            {saveError && <Banner tone="danger" action={<div className="row-gap"><Button variant="line" size="sm" onClick={() => void flush()}>Try again</Button>
                <Button variant="line" size="sm" onClick={() => navigator.clipboard.writeText(`# ${day}\n\n${editor.getMarkdown()}`)}>Copy my text</Button></div>}>
                <strong>Couldn't save this page.</strong> {saveError}</Banner>}
            {loadError && <Banner tone="danger" action={<Button variant="line" size="sm" onClick={() => onDay(day)}>Try again</Button>}><strong>This page couldn't be opened for editing.</strong> {loadError}</Banner>}
            {raw && <Banner tone="warning"><strong>This page can't be edited safely.</strong> Its text is shown as it is in the file, which is untouched.</Banner>}

            {notice?.visible && !noticeGone && day === today && !skipped && (
                <ReminderNotice notice={notice} onDismiss={() => setNoticeGone(true)}
                                onSnooze={() => Notes.SnoozeReminder().then(setNotice, e => onToast(String(e?.message ?? e)))}
                                onSkipDay={() => onSkip([day])}/>
            )}

            <header className="page-head">
                {label && <p className="label">{label}</p>}
                {page && <Display key={day} lines={[weekday, rest.join(' ')]}/>}
                {page && !skipped && <p className="small muted page-progress" aria-live="polite">
                    {words >= Math.max(1, minWords) ? `Logged, ${plural(words, 'word')}` : `${plural(words, 'word')}. Counts as logged at ${minWords}.`}
                    {recentlySaved && <span className="page-saved"> · Saved</span>}
                </p>}
            </header>

            {page?.carry && !page.carryApplied && !skipped && (
                <section className="carry rule-top">
                    <p className="label">From yesterday · {page.carry.sourceLabel} · {plural(page.carry.items.length, 'item')}</p>
                    <ul className="carry-items">{page.carry.items.slice(0, 8).map(i => <li key={i}>{i}</li>)}</ul>
                    {page.carry.items.length > 8 && <p className="small muted">and {page.carry.items.length - 8} more</p>}
                    <Button variant="line" size="sm" onClick={carryOver}>Carry over {plural(page.carry.items.length, 'item')}</Button>
                </section>
            )}

            {page && !skipped && day <= today && (
                <SuggestionsStrip day={day} pageMarkdown={() => editor.getMarkdown()} onInsert={insertUnder} onToast={onToast}/>
            )}

            {skipped && page && (
                <section className="skipped">
                    <p>You skipped this day{page.skipReason ? ` (${page.skipReason})` : ''}. It doesn't affect your streak.</p>
                    <div className="row-gap">
                        <Button variant="line" onClick={() => { setWriteAnyway(true); show(day, '', false); }}>Write a log anyway</Button>
                        <Button variant="ghost" onClick={undoSkip}>Undo skip</Button>
                    </div>
                </section>
            )}
            {page?.notLocal && <DownloadingPage day={day} title={page.title} source={''} onOpenStorage={() => void Windows.ShowSettings()}/>}
            <div hidden={skipped || !!page?.notLocal}><Editor onMessage={onMessage} label={`Page for ${page?.title ?? day}`}/></div>

            {page && !skipped && (
                <footer className="page-foot row-gap">
                    {page.status !== 'logged' && day <= today && <Button variant="ghost" size="sm" onClick={() => onSkip([day])}>Skip day…</Button>}
                    {page.exists && <Button variant="ghost" size="sm" onClick={() => onRestore(day)}>Restore previous version…</Button>}
                </footer>
            )}
        </article>
    );
}

/** ‹ Today › and Go to date, above the page. */
function PageNav({day, today, onDay}: {day: string; today: string; onDay: (day: string) => void}) {
    const [jump, setJump] = useState('');
    const [error, setError] = useState('');
    const step = (n: number) => {
        const d = new Date(Number(day.slice(0, 4)), Number(day.slice(5, 7)) - 1, Number(day.slice(8, 10)) + n);
        onDay([d.getFullYear(), String(d.getMonth() + 1).padStart(2, '0'), String(d.getDate()).padStart(2, '0')].join('-'));
    };
    async function go() {
        const d = await Notes.JumpTo(jump);
        if (!d) return setError("Couldn't read that date. Try 2 Oct, last friday or 2026-10-02.");
        if (d > today) return setError("That day hasn't happened yet.");
        setError('');
        setJump('');
        onDay(d);
    }
    return (
        <div className="page-nav">
            <div className="row-gap">
                <Button variant="line" size="sm" onClick={() => step(-1)} title="Previous day (⌘[)">‹</Button>
                <Button variant="line" size="sm" onClick={() => onDay(today)} disabled={day === today} title="Today (⌘T)">Today</Button>
                <Button variant="line" size="sm" onClick={() => step(1)} disabled={day >= today} title="Next day (⌘])">›</Button>
            </div>
            <form className="jump" onSubmit={e => { e.preventDefault(); void go(); }}>
                <label className="sr-only" htmlFor="jump">Go to date</label>
                <input id="jump" className="input input-sm" placeholder="last fri, 2 oct, -3" value={jump} aria-invalid={!!error}
                       onChange={e => { setJump(e.target.value); setError(''); }}/>
                {error && <p className="small danger" role="alert">{error}</p>}
            </form>
        </div>
    );
}
