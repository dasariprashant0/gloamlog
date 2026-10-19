// The setup sheet, 4 steps (the last, Claude Code, is optional). Everything is pre-filled; the login item is opt-in.
//   1 Welcome + the page template   2 Reminder + words to log a day   3 Folder + log start (existing logs) + login item   4 Claude Code
// First run: every answer applies; "Skip setup" applies the defaults. "Run setup again" (firstRun={false}) starts from the
// current settings and changes only what is edited: a folder switch asks the usual copy / move question, a template of your
// own stays unless you pick a card. Port of app/UI/Onboarding.swift (docs/port/PARITY.md 3.N).
import {useCallback, useEffect, useMemo, useState} from 'react';
import {Banner, Button, Dialog, Field, Segmented, Switch} from '@personal/ui/react';
import {Notes, Windows, plural} from '../api';
import type {Settings} from '../api';
import type {ClaudeInfo, FolderPlan, OnboardingInfo} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import {ClaudeSteps} from '../components/ClaudeSteps';
import {CLASSIC_TEMPLATE, DAY_NAMES, DEFAULT_CARRY, DEFAULT_TEMPLATE, NumberField, errText, systemWeekStart} from './settings/shared';

type Preset = {id: string; title: string; detail: string; markdown: string; carry: string[]; preview: string[]};
const PRESETS: Preset[] = [
    {id: 'log', title: 'Log list', detail: 'A Work list and a Personal list through the day. Mark lines next or blocked.', markdown: DEFAULT_TEMPLATE, carry: DEFAULT_CARRY,
        preview: ['09:40 Fixed the login loop', '17:45 Write the test #next']},
    {id: 'review', title: 'Daily review', detail: 'Five prompts for the end of the day.', markdown: CLASSIC_TEMPLATE, carry: DEFAULT_CARRY,
        preview: ['What I did', 'Finished', 'Started', 'Pending / blocked', 'To do next']},
    {id: 'standup', title: 'Standup', detail: "What moved, what's next, what's stuck.", markdown: '## Yesterday\n\n## Today\n\n## Blockers',
        carry: ['Today', 'Blockers'], preview: ['Yesterday', 'Today', 'Blockers']},
    {id: 'blank', title: 'Blank page', detail: 'An empty page. Add headings as you go.', markdown: '', carry: DEFAULT_CARRY, preview: []},
];

type Choice = 'local' | 'icloud' | 'custom';
type Step = 1 | 2 | 3 | 4;
type Claude = ClaudeInfo | 'checking' | 'working' | null;

const pad = (n: number) => String(n).padStart(2, '0');
const localDay = () => { const d = new Date(); return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`; };
const longDay = (day: string) => {
    const [y, m, d] = day.split('-').map(Number);
    return new Date(y, m - 1, d).toLocaleDateString('en-GB', {day: 'numeric', month: 'short', year: 'numeric'});
};
const TITLES: Record<Step, string> = {1: 'Write up your day, every day.', 2: 'When should we ask?', 3: 'Where should logs live?', 4: 'Let Claude write your log'};
const FALLBACK_DAYS = [2, 3, 4, 5, 6];

type Props = {
    open: boolean;
    /** true: the first launch. false: "Run setup again" from Settings > About. */
    firstRun: boolean;
    /** Setup finished, was skipped or (when run again) cancelled. On a first run, open today's page with the cursor in it. */
    onDone: () => void;
};

export function Onboarding({open, firstRun, onDone}: Props) {
    const [step, setStep] = useState<Step>(1);
    const [saved, setSaved] = useState<Settings | null>(null);
    const [loginNow, setLoginNow] = useState(false);
    const [info, setInfo] = useState<OnboardingInfo | null>(null);
    const [preset, setPreset] = useState<string | null>('log');     // null: your own template, kept on a re-run
    const [touched, setTouched] = useState(false);
    const [minutes, setMinutes] = useState(1015);
    const [days, setDays] = useState<number[]>(FALLBACK_DAYS);
    const [mode, setMode] = useState<'strict' | 'gentle'>('strict');
    const [minWords, setMinWords] = useState(20);
    const [choice, setChoice] = useState<Choice>('local');
    const [custom, setCustom] = useState('');
    const [login, setLogin] = useState(false);
    const [logStart, setLogStart] = useState('');                       // '': follow the first log
    const [claude, setClaude] = useState<Claude>(null);
    const [confirming, setConfirming] = useState(false);
    const [claudeNote, setClaudeNote] = useState('');                    // what Connect said, in words
    const [plan, setPlan] = useState<FolderPlan | null>(null);          // a re-run folder switch that has to ask
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);

    const folderOf = useCallback((c: Choice, i: OnboardingInfo | null, cust: string) =>
        c === 'icloud' && i?.icloudFolder ? i.icloudFolder : c === 'custom' && cust ? cust : (i?.defaultFolder ?? ''), []);
    const folder = folderOf(choice, info, custom);

    // Start from the current settings every time the sheet opens.
    useEffect(() => {
        if (!open) return;
        let live = true;
        setStep(1); setTouched(false); setError(''); setBusy(false); setPlan(null); setClaude(null); setClaudeNote(''); setConfirming(false); setLogStart('');
        (async () => {
            const [s, si] = await Promise.all([Notes.Settings(), Notes.SettingsInfo()]);
            const i = await Notes.OnboardingInfo(s.storageFolder);
            if (!live) return;
            setSaved(s);
            setInfo(i);
            setMinutes(s.reminderMinutes); setDays(s.weekdays.length ? s.weekdays : FALLBACK_DAYS);
            setMode(String(s.mode) === 'gentle' ? 'gentle' : 'strict'); setMinWords(s.minWords);
            setLoginNow(si.loginEnabled);
            if (firstRun) { setPreset('log'); setChoice('local'); setCustom(''); setLogin(false); return; }
            setPreset(PRESETS.find(p => p.markdown === s.template)?.id ?? null);
            setLogin(si.loginEnabled);
            const here = s.storageFolder.replace(/\/$/, '');
            if (here === i.defaultFolder) { setChoice('local'); setCustom(''); }
            else if (i.icloudFolder && here === i.icloudFolder) { setChoice('icloud'); setCustom(''); }
            else { setChoice('custom'); setCustom(s.storageFolder); }
            if (s.logStartDate) setLogStart(s.logStartDate);
        })().catch(e => { if (live) setError(errText(e)); });
        return () => { live = false; };
    }, [open, firstRun]);

    // Day pages already in the chosen folder, so a person with existing logs can say where the log starts.
    useEffect(() => {
        if (!open || !saved || !folder) return;
        let live = true;
        Notes.OnboardingInfo(folder).then(i => { if (live) { setInfo(i); if (!i.found) setLogStart(''); } }).catch(() => undefined); // no summary: the step still works
        return () => { live = false; };
    }, [open, saved, folder]);

    // The Claude Code step asks `claude` (slow), so only when it is shown.
    useEffect(() => {
        if (!open || step !== 4) return;
        let live = true;
        setClaude('checking');
        Notes.ClaudeStatus().then(c => { if (live) setClaude(c); }).catch(() => { if (live) setClaude(null); });
        return () => { live = false; };
    }, [open, step]);

    const found = info?.found ?? null;
    const today = localDay();
    const pretty = (path: string) => { const home = (info?.defaultFolder ?? '').replace(/\/[^/]*$/, ''); return home && path.startsWith(home + '/') ? '~' + path.slice(home.length) : path; };

    function cancel() {
        if (busy) return;
        onDone();
    }

    async function pickCustom() {
        setError('');
        try {
            const p = await Notes.ChooseFolder('Choose where Gloamlog saves your logs.');
            if (p) { setCustom(p); setChoice('custom'); }
            else if (!custom) setChoice('local');
        } catch (e) {
            setError(errText(e));
            if (!custom) setChoice('local');
        }
    }

    function pickChoice(c: Choice) {
        setChoice(c);
        if (c === 'custom' && !custom) void pickCustom();
    }

    async function connect() {
        setConfirming(false);
        setClaude('working');
        try {
            const r = await Notes.ConnectClaude();
            setClaude({state: r.state, message: r.message} as ClaudeInfo);
            setClaudeNote(r.message);
        } catch (e) {
            setClaude(null);
            setError(errText(e));
        }
    }

    /** Writes the settings. useDefaults (Skip setup): only the template and onboarded; every other answer stays at its default. */
    async function finish(useDefaults: boolean) {
        if (busy) return;
        setBusy(true);
        setError('');
        try {
            const cur = await Notes.Settings();
            const p = PRESETS.find(x => x.id === preset);
            const next: Settings = {...cur};
            if (p && (firstRun || touched)) { next.template = p.markdown; next.carryOverHeadings = p.carry; }
            if (!useDefaults) {
                next.reminderMinutes = minutes;
                next.weekdays = days.length ? [...days].sort() : FALLBACK_DAYS;
                next.mode = mode as Settings['mode'];
                next.minWords = minWords;
                if (firstRun) next.storageFolder = folder;
                if (logStart && found) next.logStartDate = logStart;
            }
            next.onboarded = true;
            await Notes.SaveSettings(next);
            if (!useDefaults && login !== (firstRun ? false : loginNow)) {
                // stay open on a failure: closing at once would hide the message and the login item would quietly not be set
                const failed = await Notes.SetLoginItem(login).then(() => '', e => errText(e));
                if (failed) { setError(`Your settings were saved, but the login item wasn't set: ${failed} You can turn it on later in Settings.`); setBusy(false); return; }
            }
            if (firstRun) {
                await Notes.RecreateFolder().catch(() => undefined); // best effort: the first save makes the folder too
                await Notes.MarkWhatsNewSeen().catch(() => undefined); // a new person has nothing to be told is new
            }
            if (!firstRun && !useDefaults && folder && folder.replace(/\/$/, '') !== cur.storageFolder.replace(/\/$/, '')) {
                const fp = await Notes.PlanFolderChange(folder);
                if (!fp.same) {
                    if (!fp.writable) { setError(`Gloamlog can't save to this folder. You may not have permission to write to "${fp.name}". The folder was not changed.`); setBusy(false); return; }
                    if (fp.mine > 0) { setPlan(fp); setBusy(false); return; }
                    await Notes.ChangeFolder(fp.path, 'use');
                }
            }
            onDone();
        } catch (e) {
            setError(errText(e));
        }
        setBusy(false);
    }

    async function switchFolder(mode: 'use' | 'copy' | 'move') {
        if (!plan) return;
        setBusy(true);
        try {
            await Notes.ChangeFolder(plan.path, mode);
            setPlan(null);
            onDone();
        } catch (e) {
            setPlan(null);
            setError(errText(e));
        }
        setBusy(false);
    }

    const toggleDay = (d: number) => setDays(cur => (cur.includes(d) ? cur.filter(x => x !== d) : [...cur, d]));
    const weekFirst = saved?.weekStart || systemWeekStart();
    const order = useMemo(() => Array.from({length: 7}, (_, i) => ((weekFirst - 1 + i) % 7) + 1), [weekFirst]);

    // ---- the one question a re-run folder switch can ask ----
    if (plan) {
        const empty = plan.theirs === 0;
        return (
            <Dialog open={open} onClose={() => setPlan(null)}
                title={empty ? `Move your ${plan.mine} logs to "${plan.name}"?` : `"${plan.name}" already has ${plan.theirs} logs.`}
                actions={empty ? <>
                    <Button variant="line" onClick={() => setPlan(null)} disabled={busy}>Cancel</Button>
                    <Button variant="line" onClick={() => void switchFolder('use')} disabled={busy}>Leave them</Button>
                    <Button variant="line" onClick={() => void switchFolder('move')} disabled={busy}>Move logs</Button>
                    <Button onClick={() => void switchFolder('copy')} disabled={busy}>Copy logs</Button>
                </> : <>
                    <Button variant="line" onClick={() => setPlan(null)} disabled={busy}>Cancel</Button>
                    <Button variant="line" onClick={() => void switchFolder('use')} disabled={busy}>Use this folder</Button>
                    <Button onClick={() => void switchFolder('copy')} disabled={busy}>Use and copy mine over</Button>
                </>}>
                <p>{empty ? 'Copy keeps your originals where they are.' : 'Days that exist in both are never overwritten.'}</p>
            </Dialog>
        );
    }

    const last = step === 4;
    const title = step === 1 && !firstRun ? 'Your page' : TITLES[step];
    return (
        <Dialog open={open} onClose={firstRun ? () => undefined : cancel} title={title} actions={<>
            {step === 1 && (
                <Button variant="ghost" onClick={() => (firstRun ? void finish(true) : cancel())} disabled={busy}>{firstRun ? 'Skip setup' : 'Cancel'}</Button>
            )}
            {step > 1 && <Button variant="line" onClick={() => setStep(s => (s - 1) as Step)} disabled={busy}>Back</Button>}
            <Button onClick={() => (last ? void finish(false) : setStep(s => (s + 1) as Step))} disabled={busy || (step === 2 && days.length === 0)}>
                {last ? (firstRun ? 'Start writing' : 'Done') : 'Continue'}
            </Button>
        </>}>
            <div className="stack" aria-live="polite">
                <p className="label" aria-label={`Step ${step} of 4`}>Step {step} of 4</p>
                {error && <Banner tone="danger" action={<Button size="sm" variant="line" onClick={() => setError('')}>Dismiss</Button>}>{error}</Banner>}

                {step === 1 && <>
                    <p>Each day gets one page. Write freely, with headings, lists, to-dos and images, or start from a few prompts.</p>
                    <ul className="list" role="radiogroup" aria-label="Start your page with">
                        {PRESETS.map(p => (
                            <li key={p.id}>
                                <label className="row">
                                    <input type="radio" name="preset" checked={preset === p.id} style={{accentColor: 'var(--accent)'}}
                                        onChange={() => { setPreset(p.id); setTouched(true); }}/>
                                    <span className="row-body">
                                        <span className="row-title">{p.title}</span>
                                        <span className="row-meta">{p.detail}{p.preview.length > 0 && ` ${p.preview.join(' · ')}`}</span>
                                    </span>
                                </label>
                            </li>
                        ))}
                    </ul>
                    <p className="small muted">{firstRun
                        ? 'Your pages are plain markdown files you own. Nothing leaves your Mac. You can change the template any time in Settings.'
                        : preset === null
                            ? 'Your own template stays as it is unless you pick one of these. Pages you already wrote are never changed.'
                            : 'Pages you already wrote are never changed. The template only fills new days.'}</p>
                </>}

                {step === 2 && <>
                    <Field label="Remind me at">
                        <input className="input input-sm" type="time" required style={{width: 'auto'}}
                            value={`${pad(Math.floor(minutes / 60))}:${pad(minutes % 60)}`}
                            onChange={e => { const [h, m] = e.target.value.split(':').map(Number); if (Number.isFinite(h) && Number.isFinite(m)) setMinutes(h * 60 + m); }}/>
                    </Field>
                    <div className="seg" role="group" aria-label="On these days">
                        {order.map(d => {
                            const on = days.includes(d);
                            return (
                                <button key={d} className="seg-opt small" aria-pressed={on} aria-label={DAY_NAMES[d - 1]} title={DAY_NAMES[d - 1]} onClick={() => toggleDay(d)}>
                                    {DAY_NAMES[d - 1].slice(0, 3)}
                                </button>
                            );
                        })}
                    </div>
                    {days.length === 0 && <p className="small danger" role="alert">Pick at least one day.</p>}
                    <Segmented label="How firm?" options={[['strict', 'Strict'], ['gentle', 'Gentle']]} value={mode} onChange={setMode}/>
                    <p className="small muted">{mode === 'strict' ? 'Comes to the front until you save or skip the day.' : 'One notification. You decide when.'}</p>
                    <div className="row-gap">
                        <span>A day counts as logged after</span>
                        <NumberField label="Words to log a day" value={minWords} min={1} max={500} onCommit={setMinWords}/>
                        <span className="muted">words</span>
                    </div>
                    <p className="small muted">A day counts once the page has this many words. You can always skip a day or snooze. Change all of this later in Settings.</p>
                </>}

                {step === 3 && <>
                    <ul className="list" role="radiogroup" aria-label="Where logs live">
                        <FolderRow checked={choice === 'local'} onPick={() => pickChoice('local')} title="On this Mac" meta={info ? pretty(info.defaultFolder) : '~/Gloamlog'}/>
                        {info?.icloudFolder && <FolderRow checked={choice === 'icloud'} onPick={() => pickChoice('icloud')} title="iCloud Drive" meta="Gloamlog"/>}
                        <FolderRow checked={choice === 'custom'} onPick={() => pickChoice('custom')} title="Another folder…"
                            meta={custom ? pretty(custom) : undefined}
                            end={choice === 'custom' && custom ? <Button size="sm" variant="line" onClick={() => void pickCustom()}>Choose…</Button> : undefined}/>
                    </ul>
                    {found ? (
                        <div className="stack rule-top" style={{paddingTop: 'var(--s3)'}}>
                            <p>Found {plural(found.count, 'log')}, {longDay(found.first)} to {longDay(found.last)}. Nothing is moved or rewritten.</p>
                            <Field label="Log starts" hint="Days before it never count as unlogged.">
                                <input className="input input-sm" type="date" style={{width: 'auto'}} min={found.first} max={today < found.first ? found.first : today}
                                    value={logStart || found.first}
                                    onChange={e => { const v = e.target.value; if (v >= found.first && v <= today) setLogStart(v === found.first ? '' : v); }}/>
                            </Field>
                        </div>
                    ) : <p className="small muted">Pick iCloud Drive or Dropbox to keep logs on every Mac. Images you add live next to your pages in an assets folder.</p>}
                    <Switch checked={login} onChange={setLogin} label="Open Gloamlog at login"/>
                    <p className="small muted">So the reminder can fire after a restart. Everything in this setup lives in Settings.</p>
                </>}

                {step === 4 && <>
                    <p>If you use Claude Code, Gloamlog can give it a small helper. When a piece of work is done, Claude adds a short entry to today's page: the problem, what it did, what is still open. Your Git commits and pull requests can be added too.</p>
                    <ClaudeControl claude={claude} note={claudeNote} confirming={confirming} onAsk={() => setConfirming(true)} onCancel={() => setConfirming(false)} onConnect={() => void connect()}/>
                    <p className="small muted">Optional. The button shows exactly what it changes before it does anything, nothing leaves your Mac, and Settings &gt; Sources can undo it. You can skip this and connect later.</p>
                </>}
            </div>
        </Dialog>
    );
}

function FolderRow({checked, onPick, title, meta, end}: {checked: boolean; onPick: () => void; title: string; meta?: string; end?: React.ReactNode}) {
    return (
        <li>
            <label className="row">
                <input type="radio" name="folder" checked={checked} onChange={onPick} style={{accentColor: 'var(--accent)'}}/>
                <span className="row-body">
                    <span className="row-title">{title}</span>
                    {meta && <span className="row-meta" style={{overflowWrap: 'anywhere'}}>{meta}</span>}
                </span>
                {end !== undefined && <span className="row-end">{end}</span>}
            </label>
        </li>
    );
}

function ClaudeControl({claude, note, confirming, onAsk, onCancel, onConnect}: {claude: Claude; note: string; confirming: boolean; onAsk: () => void; onCancel: () => void; onConnect: () => void}) {
    const later = <p className="muted">Claude Code was not found on this Mac. You can connect it later in Settings &gt; Sources.</p>;
    if (claude === null) {
        return <div className="stack">{later}<div><Button variant="line" size="sm" onClick={() => void Windows.ShowSettings()}>Open Settings</Button></div></div>;
    }
    if (claude === 'checking') return <p className="muted" role="status">Looking for Claude Code…</p>;
    if (claude === 'working') return <p className="muted" role="status">Connecting…</p>;
    if (claude.state === 'noClaude') return later;
    if (confirming) {
        return (
            <Banner tone="warning" action={<div className="row-gap"><Button size="sm" onClick={onConnect}>Connect</Button><Button size="sm" variant="line" onClick={onCancel}>Cancel</Button></div>}>
                <strong>Connect Claude Code to Gloamlog?</strong> <ClaudeSteps/>
            </Banner>
        );
    }
    return (
        <div className="stack">
            {claude.state === 'connected'
                ? <p role="status"><strong>Connected</strong> to Claude Code.</p>
                : <div><Button variant="line" onClick={onAsk}>{claude.state === 'moved' ? 'Reconnect Claude Code' : 'Connect Claude Code'}</Button></div>}
            {note && <p className="small muted">{note}</p>}
            {!note && claude.state === 'moved' && <p className="small muted">{claude.message}</p>}
        </div>
    );
}
