// "Check for Updates…": runs the check when it opens and shows each state, then the install progress. Port of
// app/UI/Updater.swift (docs/port/PARITY.md 3.S and 6.4). The only network traffic is the check the person asked for and, on
// "Download and Install", the release itself (Notes.CheckForUpdates / Notes.InstallUpdate; the app restarts on success).
import {useEffect, useState} from 'react';
import {Events} from '@wailsio/runtime';
import {Banner, Button, Dialog} from '@personal/ui/react';
import {Notes} from '../api';
import type {InstallOutcome, UpdateResult} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';
import {errText} from './settings/shared';

/** The "update-progress" event: phase is downloading (fraction 0...1), verifying or restarting. */
type UpdateProgress = {phase: string; fraction: number};

type Phase =
    | {kind: 'checking'}
    | {kind: 'result'; res: UpdateResult}
    | {kind: 'installing'; res: UpdateResult; progress: UpdateProgress}
    | {kind: 'outcome'; res: UpdateResult; out: InstallOutcome};

type Props = {open: boolean; onClose: () => void};

export function UpdateDialog({open, onClose}: Props) {
    const [phase, setPhase] = useState<Phase>({kind: 'checking'});
    const [error, setError] = useState('');

    useEffect(() => {
        if (!open) return;
        let live = true;
        setPhase({kind: 'checking'});
        setError('');
        Notes.CheckForUpdates()
            .then(res => { if (live) setPhase({kind: 'result', res}); })
            .catch(() => { if (live) setPhase({kind: 'result', res: {state: 'error', current: '', release: {} as UpdateResult['release']}}); });
        return () => { live = false; };
    }, [open]);

    // Progress from the Go side while an install runs.
    useEffect(() => {
        if (!open) return;
        const off = Events.On('update-progress', ev => {
            const p = ev.data as UpdateProgress;
            setPhase(cur => (cur.kind === 'installing' ? {...cur, progress: p} : cur));
        });
        return off;
    }, [open]);

    async function install(res: UpdateResult) {
        setError('');
        setPhase({kind: 'installing', res, progress: {phase: 'downloading', fraction: 0}});
        try {
            const out = await Notes.InstallUpdate(res.release);
            setPhase({kind: 'outcome', res, out});
        } catch (e) {
            setPhase({kind: 'outcome', res, out: {status: 'failed', message: errText(e)}});
        }
    }
    const openPage = (res: UpdateResult) => Notes.OpenReleasePage(res.release.pageURL).catch(e => setError(errText(e)));
    const again = () => { setPhase({kind: 'checking'}); Notes.CheckForUpdates().then(res => setPhase({kind: 'result', res})).catch(e => setError(errText(e))); };

    const close = <Button variant="line" onClick={onClose}>Close</Button>;
    let title = 'Check for updates', body: React.ReactNode = null, actions: React.ReactNode = close, dismiss = onClose;

    if (phase.kind === 'checking') {
        title = 'Check for updates';
        body = <p className="muted" role="status">Checking for updates…</p>;
    } else if (phase.kind === 'result') {
        const {res} = phase;
        if (res.state === 'none') {
            title = 'No updates published yet';
            body = <p>Gloamlog {res.current} is the only version so far.</p>;
        } else if (res.state === 'uptodate') {
            title = "You're up to date";
            body = <p>Gloamlog {res.current} is the latest version.</p>;
        } else if (res.state === 'available') {
            title = `Gloamlog ${res.release.version} is available`;
            body = <>
                <p>You have {res.current}.</p>
                {res.release.notes && <p className="small muted" style={{whiteSpace: 'pre-wrap', maxHeight: 'calc(var(--s9) * 2)', overflowY: 'auto'}}>{res.release.notes}</p>}
            </>;
            actions = <>
                <Button variant="ghost" onClick={onClose}>Later</Button>
                <Button variant="line" onClick={() => void openPage(res)}>Release Notes</Button>
                <Button onClick={() => void install(res)}>Download and Install</Button>
            </>;
        } else {
            title = "Couldn't check for updates";
            body = <p>Check your internet connection and try again.</p>;
            actions = <>{close}<Button onClick={again}>Try again</Button></>;
        }
    } else if (phase.kind === 'installing') {
        const {res, progress} = phase;
        title = 'Updating Gloamlog';
        const downloading = progress.phase === 'downloading';
        const restarting = progress.phase === 'restarting';
        body = <div className="stack" role="status">
            <p>{restarting ? 'Restarting Gloamlog…' : downloading ? `Downloading Gloamlog ${res.release.version}…` : `Checking Gloamlog ${res.release.version}…`}</p>
            <progress style={{width: '100%', accentColor: 'var(--accent)'}} max={1} aria-label="Update progress"
                value={downloading && progress.fraction > 0 ? progress.fraction : undefined}/>
        </div>;
        actions = null;
        dismiss = () => undefined; // nothing to cancel mid-way: the swap only starts after every check passed
    } else {
        const {res, out} = phase;
        if (out.status === 'fallback') {
            title = "Can't install automatically";
            body = <p>{out.message}</p>;
            actions = <>
                <Button variant="line" onClick={onClose}>Cancel</Button>
                <Button onClick={() => void openPage(res)}>Open Download Page</Button>
            </>;
        } else {
            title = 'Update not installed';
            body = <p>{out.message} You can still download it from the Gloamlog releases page.</p>;
            actions = <>{close}<Button onClick={() => void openPage(res)}>Open Download Page</Button></>;
        }
    }

    return (
        <Dialog open={open} onClose={dismiss} title={title} actions={actions}>
            <div className="stack">
                {body}
                {error && <Banner tone="danger">{error}</Banner>}
            </div>
        </Dialog>
    );
}
