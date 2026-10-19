// One day's page: header (date, progress, saved state), From yesterday, the editor, and its banners.
// Saving rules (Swift DayEditor): only real edits are written, 600 ms after the last change and at once on day switch, on
// leaving the page and on window blur; saves run one at a time; opening a page never writes it.
import {useCallback, useEffect, useRef, useState} from 'react';
import {Banner, Button, Display, Segmented} from '@personal/ui/react';
import {Browser} from '@wailsio/runtime';
import {Notes, Windows, plural, relativeDay, type Page} from '../api';
import {Editor, editor, type EditorMessage} from '../editor/Editor';
import {SuggestionsStrip, type InsertPart} from './SuggestionsStrip';
import {DownloadingPage} from './SyncChip';
import {ReminderNotice} from './ReminderNotice';
import type {ReminderNoticeState} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import {on} from '../events';
import {LogEditor, type LogEditorHandle, type View} from '../log/LogEditor';
import * as LogModel from '../log/model';
import {CarryBox} from '../log/CarryBox';
import type {LogPage} from '../../bindings/github.com/dasariprashant0/gloamlog/internal/core';

const SAVE_DELAY = 600;
const VIEWS: [View, string][] = [['list', 'List'], ['standup', 'Standup']];

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
    const [listPage, setListPage] = useState<LogPage | null>(null); // the open page's Log section; null for a classic page
    const [full, setFull] = useState(false);                        // "Edit whole page" on a list page
    const [pageKey, setPageKey] = useState(0);                      // remounts the list editor with a new page
    const [summary, setSummary] = useState('');
    const [view, setView] = useState<View>('list');                // List | Standup; kept across days (⌘[ for yesterday's standup)
    const logRef = useRef<LogEditorHandle>(null);
    const listMode = useRef(false);                                 // listPage && !full, for the callbacks

    const loaded = useRef<string | null>(null);   // the day whose text is in the editor (null: none, or a skipped page)
    const baseline = useRef('');                  // the editor's text right after loading: anything else is an edit
    const lastSaved = useRef('');                 // what the file holds now, as the editor writes it
    const pending = useRef<{day: string; md: string} | null>(null);
    const timer = useRef(0);
    const chain = useRef<Promise<unknown>>(Promise.resolve());
    const appending = useRef(0);                  // lines being added by the suggestions strip, for the reply
    // Pages whose save failed, by day (Swift's "orphans"): their text is kept here, never dropped, and saved again on every
    // flush and every 30 s until it succeeds. Switching days after a failed save used to lose that text.
    const orphans = useRef(new Map<string, string>());
    const [orphanCount, setOrphanCount] = useState(0);

    // Saves one page; resolves true when it reached the file. Saves run one at a time.
    const save = useCallback((d: string, md: string): Promise<boolean> => {
        const run = chain.current.then(() => Notes.Save(d, md)).then(out => {
            if (loaded.current === d) lastSaved.current = md;
            if (orphans.current.delete(d)) setOrphanCount(orphans.current.size);
            void Notes.EditorSaved(d, md).catch(() => {}); // capture also checks the file every 15 s
            setSaveError('');
            setSavedAt(Date.now());
            if (out?.backupWarning) onToast(out.backupWarning);
            onSaved();
            return true;
        }, err => {
            setSaveError(String(err?.message ?? err));
            return false;
        });
        chain.current = run;
        return run;
    }, [onSaved, onToast]);

    // Writes whatever is unsaved now: pages left unsaved earlier, a scheduled save, or text typed so recently the editor has
    // not reported it yet. A save that fails keeps its text in orphans, so leaving the page never loses it.
    const flush = useCallback(async () => {
        clearTimeout(timer.current);
        const p = pending.current;
        pending.current = null;
        for (const [od, omd] of [...orphans.current]) await save(od, omd);
        const d = loaded.current;
        let unsaved: {day: string; md: string} | null = null;
        const md = d && listMode.current && logRef.current ? await logRef.current.markdown().catch(() => null)
            : d && !editor.isRawFallback() ? editor.getMarkdown() : null;
        if (d && md !== null) {
            if (md !== baseline.current && md !== lastSaved.current) unsaved = {day: d, md};
        } else if (p) unsaved = p;
        if (unsaved && !(await save(unsaved.day, unsaved.md))) orphans.current.set(unsaved.day, unsaved.md);
        setOrphanCount(orphans.current.size);
    }, [save]);
    useEffect(() => {
        const t = setInterval(() => { if (orphans.current.size) void flush(); }, 30_000);
        return () => clearInterval(t);
    }, [flush]);

    // Open the day: save the old one first, then swap the document in the editor.
    useEffect(() => {
        let live = true;
        (async () => {
            const previous = loaded.current;
            await flush();
            // capture.go is told which page owns which day; if it misses this, jots fall back to the file (still kept)
            if (previous && previous !== day) void Notes.EditorClosed(previous).catch(() => {}); // see above: jots fall back to the file
            void Notes.EditorOpened(day).catch(() => {}); // from now on a jot for this day comes through the editor
            const p = await Notes.Open(day);
            const kept = orphans.current.get(p.day);
            const parsed = p.isSkipped ? null : await Notes.LogParse(kept ?? p.body);
            if (!live) return;
            setPage(p);
            setWords(p.words);
            setRaw(false);
            setLoadError('');
            setWriteAnyway(false);
            loaded.current = null;
            if (!p.isSkipped) {
                show(p.day, kept ?? p.body, p.exists, p.notLocal, parsed?.ok ? parsed.page : null);
                if (kept !== undefined) baseline.current = ''; // the kept text is unsaved: the next flush writes it
            }
        })().catch(e => live && setLoadError(String(e?.message ?? e)));
        return () => { live = false; };
    }, [day, reloadKey]); // eslint-disable-line react-hooks/exhaustive-deps

    function show(d: string, body: string, exists: boolean, readOnly = false, list: LogPage | null = null) {
        setListPage(list);
        setFull(false);
        setPageKey(k => k + 1);
        setSummary(list ? LogModel.summary(list.lines as unknown as LogModel.Line[]) : '');
        listMode.current = !!list;
        editor.setMarkdown(body, {cursor: 'start'}); // the full editor holds the page too, for "Edit whole page"
        editor.setReadOnly(readOnly);
        baseline.current = list ? body : editor.getMarkdown(); // the list editor writes an untouched page back byte for byte
        lastSaved.current = exists ? baseline.current : '';
        loaded.current = d;
        // The document's token: appendToSection refuses ("stale") when the page in the editor is no longer this day.
        (window as unknown as {__dlDoc?: string}).__dlDoc = d;
    }

    // The suggestions strip's lines (classic pages only: a list page gets its commits by themselves) go under heading as one
    // undoable step; the editor replies with appendResult.
    const insertUnder = useCallback((heading: string, parts: InsertPart[]) => {
        const d = loaded.current;
        const count = parts.reduce((n, p) => n + p.count, 0);
        if (!d || !count) return;
        appending.current = count;
        editor.appendToSection(`add-${Date.now()}`, d, heading, parts.map(p => p.markdown).join('\n'), {unlessPresent: true});
    }, []);

    // Jots for the open day arrive through the editor so the next autosave can't write over them (capture.go).
    useEffect(() => on<{id: string; day: string; heading: string; list?: string; markdown: string; timed?: boolean}>('jot-into-page', j => {
        const notLoaded = () => void Notes.JotPlaced(j.id, 'notLoaded', '').catch(() => {}); // capture.go writes the file after 4 s
        if (loaded.current !== j.day) return notLoaded();
        if (listMode.current && logRef.current) {
            logRef.current.appendLines(j.markdown, j.list || 'Work', !!j.timed).then(r => {
                void Notes.JotPlaced(j.id, r.result, r.markdown).catch(() => {});
                if (r.result === 'appended') void save(j.day, r.markdown);
            }, notLoaded);
            return;
        }
        if (!editor.isRawFallback()) editor.appendToSection(j.id, j.day, j.heading, j.markdown, {unlessPresent: true});
        else notLoaded();
    }), [save]);

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

    // A change in either editor: save it 600 ms after the last one, if it differs from the file and from the page as it
    // opened (opening never writes).
    const scheduleSave = useCallback((md: string) => {
        const d = loaded.current;
        if (!d) return;
        pending.current = {day: d, md};
        clearTimeout(timer.current);
        timer.current = window.setTimeout(() => {
            const p = pending.current;
            pending.current = null;
            if (p && p.md !== lastSaved.current && p.md !== baseline.current) void save(p.day, p.md);
        }, SAVE_DELAY);
    }, [save]);

    const onLogChange = useCallback((md: string, w: number) => { scheduleSave(md); setWords(w); }, [scheduleSave]);

    const onMessage = useCallback((m: EditorMessage) => {
        switch (m.type) {
            case 'ready':
                // The editor replays queued calls right after this message; take the baseline once it has. A list page's
                // baseline is its file text (the list editor writes that back byte for byte), never Milkdown's version of it.
                setTimeout(() => { if (loaded.current && !listMode.current) { baseline.current = editor.getMarkdown(); if (lastSaved.current) lastSaved.current = baseline.current; } }, 0);
                break;
            case 'change': {
                if (listMode.current) break; // Milkdown is hidden behind the list editor: nothing typed there
                scheduleSave(m.markdown);
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
                    void Notes.JotPlaced(m.id, m.result, m.markdown ?? '').catch(() => {}); // no answer: capture.go re-routes after 4 s
                    if (m.result === 'appended' && d && m.markdown != null) { void save(d, m.markdown); Notes.Words(m.markdown).then(setWords, () => {}); } // count is a hint: the old one stays
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
    }, [day, save, scheduleSave, onToast]);

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

    async function editWholePage() {
        if (!logRef.current) return;
        const md = await logRef.current.markdown().catch(e => { onToast(`Couldn't open the whole page: ${String(e?.message ?? e)}`); return null; });
        if (md === null) return; // the list stays as it is
        editor.setMarkdown(md, {cursor: 'start'});
        if (md === baseline.current) { // untouched: Milkdown's version of it is not an edit (as on 'ready')
            const shown = editor.getMarkdown();
            if (lastSaved.current === baseline.current) lastSaved.current = shown;
            baseline.current = shown;
        }
        listMode.current = false;
        setFull(true);
    }

    // A past list page: its commits go in at their times, each in its list (they arrive as jots); today's come by themselves.
    async function addCommits() {
        try {
            const n = await Notes.AddCommits(day);
            onToast(n ? `Added ${plural(n, 'commit')}.` : 'No new commits for this day.');
        } catch (e) {
            onToast(String((e as Error)?.message ?? e));
        }
    }

    async function backToList() {
        const parsed = await Notes.LogParse(editor.getMarkdown()).catch(e => { onToast(`Couldn't go back to the list: ${String(e?.message ?? e)}`); return null; });
        if (!parsed) return; // the full editor stays, with its text
        if (!parsed.ok) return onToast('This page has no Log heading now, so it stays in the full editor.');
        setListPage(parsed.page);
        setPageKey(k => k + 1);
        listMode.current = true;
        setFull(false);
    }

    // What "Copy my text" copies: the editor that is showing.
    const pageText = async () => listMode.current && logRef.current ? logRef.current.markdown() : editor.getMarkdown();

    const skipped = !!page?.isSkipped && !writeAnyway;
    const [weekday, ...rest] = (page?.title ?? '').split(' ');
    const label = day === today ? 'Today' : relativeDay(day, today);
    const recentlySaved = Date.now() - savedAt < 2200;

    return (
        <article className="page" hidden={!active}>
            <PageNav day={day} today={today} onDay={onDay}/>
            {folderError && <Banner tone="danger"><strong>Gloamlog can't save to your log folder.</strong> {folderError}</Banner>}
            {saveError && <Banner tone="danger" action={<div className="row-gap"><Button variant="line" size="sm" onClick={() => void flush()}>Try again</Button>
                <Button variant="line" size="sm" onClick={async () => navigator.clipboard.writeText(`# ${day}\n\n${await pageText()}`)}>Copy my text</Button></div>}>
                <strong>Couldn't save this page.</strong> {saveError}</Banner>}
            {orphanCount > 0 && <Banner tone="warning" action={<div className="row-gap"><Button variant="line" size="sm" onClick={() => void flush()}>Try again</Button>
                <Button variant="line" size="sm" onClick={() => navigator.clipboard.writeText([...orphans.current].map(([d, md]) => `# ${d}\n\n${md}`).join('\n\n'))}>Copy my text</Button></div>}>
                <strong>{orphanCount === 1 ? "A page you left couldn't be saved yet." : `${orphanCount} pages you left couldn't be saved yet.`}</strong> Its text is kept here and saved as soon as it can be.</Banner>}
            {loadError && <Banner tone="danger" action={<Button variant="line" size="sm" onClick={() => onDay(day)}>Try again</Button>}><strong>This page couldn't be opened for editing.</strong> {loadError}</Banner>}
            {raw && <Banner tone="warning"><strong>This page can't be edited safely.</strong> Its text is shown as it is in the file, which is untouched.</Banner>}

            {notice?.visible && !noticeGone && day === today && !skipped && (
                <ReminderNotice notice={notice} onDismiss={() => setNoticeGone(true)}
                                onSnooze={() => Notes.SnoozeReminder().then(setNotice, e => onToast(String(e?.message ?? e)))}
                                onSkipDay={() => onSkip([day])}/>
            )}

            <header className="page-head">
                {(label || (listPage && !full)) && <p className="label">{listPage && !full ? [label, page?.title].filter(Boolean).join(' · ') : label}</p>}
                {page && (listPage && !full
                    ? <Display key={day} lines={[summary || 'Nothing yet']}/>
                    : <Display key={day} lines={[weekday, rest.join(' ')]}/>)}
                {page && listPage && !full && !skipped && (
                    <div role="group" aria-label="View"><Segmented options={VIEWS} value={view} onChange={setView}/></div>
                )}
                {page && !skipped && <p className="small muted page-progress" aria-live="polite">
                    {words >= Math.max(1, minWords) ? `Logged, ${plural(words, 'word')}` : `${plural(words, 'word')}. Counts as logged at ${minWords}.`}
                    {recentlySaved && <span className="page-saved"> · Saved</span>}
                </p>}
            </header>

            {/* Hidden, not unmounted, in Standup: lines already kept or dropped must not be offered again on the way back. */}
            {listPage && !full && page?.carry && !skipped && (
                <div hidden={view !== 'list'}>
                    <CarryBox key={day} day={day} card={page.carry} onAdd={(t, m, l) => logRef.current?.addLine(t, m, l)} onToast={onToast}/>
                </div>
            )}

            {page?.carry && !page.carryApplied && !skipped && !listPage && (
                <section className="carry rule-top">
                    <p className="label">From yesterday · {page.carry.sourceLabel} · {plural(page.carry.items.length, 'item')}</p>
                    <ul className="carry-items">{page.carry.items.slice(0, 8).map(i => <li key={i}>{i}</li>)}</ul>
                    {page.carry.items.length > 8 && <p className="small muted">and {page.carry.items.length - 8} more</p>}
                    <Button variant="line" size="sm" onClick={carryOver}>Carry over {plural(page.carry.items.length, 'item')}</Button>
                </section>
            )}

            {page && !skipped && day <= today && !listPage && (
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
            {listPage && !full && !skipped && (
                <LogEditor key={pageKey} ref={logRef} day={day} page={listPage} readOnly={!!page?.notLocal} autoFocus={day === today} view={view}
                           onChange={onLogChange} onSummary={setSummary} onToast={onToast}/>
            )}
            <div hidden={skipped || !!page?.notLocal || (!!listPage && !full)}><Editor onMessage={onMessage} label={`Page for ${page?.title ?? day}`}/></div>

            {page && !skipped && (
                <footer className="page-foot row-gap">
                    {listPage && (full
                        ? <Button variant="ghost" size="sm" onClick={() => void backToList()}>Back to the list</Button>
                        : <Button variant="ghost" size="sm" onClick={() => void editWholePage()}>Edit whole page</Button>)}
                    {listPage && !full && day < today && !page.notLocal &&
                        <Button variant="ghost" size="sm" onClick={() => void addCommits()}>Add this day's commits</Button>}
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
