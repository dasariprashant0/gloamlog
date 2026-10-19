import {useEffect, useState} from 'react';
import {Banner, Button, Dialog, Setting} from '@personal/ui/react';
import {Notes} from '../../api';
import {on} from '../../events';
import {ConflictDialog} from '../ConflictDialog';
import {SyncSettings, useSyncState} from '../SyncChip';
import {Block, errText, Panel, tilde, type PaneProps} from './shared';

type Plan = Awaited<ReturnType<typeof Notes.PlanFolderChange>>;
type Notice = {text: string; conflicts: string[]};

const MAX_COPIES = 10; // core.MaxBackupsPerDay
const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

export function StoragePane({s, info, overview, reload}: PaneProps) {
    const [plan, setPlan] = useState<Plan | null>(null);
    const [notice, setNotice] = useState<Notice | null>(null);
    const [error, setError] = useState('');
    const isDefault = s.storageFolder.replace(/\/$/, '') === info.defaultFolder;
    // Sync: read again whenever the folder changes (Go's "folder-changed"), and after a conflict is settled.
    const [syncKey, setSyncKey] = useState(0);
    const sync = useSyncState(syncKey);
    const [conflictsOpen, setConflictsOpen] = useState(false);
    useEffect(() => on('folder-changed', () => setSyncKey(k => k + 1)), []);
    const sum = overview?.summary;

    async function change(path: string) {
        setError('');
        try {
            const p = await Notes.PlanFolderChange(path);
            if (p.same) return setError('That is already your log folder.');
            if (!p.writable) return setError(`Gloamlog can't save to this folder. You may not have permission to write to "${p.name}". Nothing was changed.`);
            if (p.mine === 0) return run(p.path, 'use');
            setPlan(p);
        } catch (e) {
            setError(errText(e));
        }
    }

    async function run(path: string, mode: 'use' | 'copy' | 'move') {
        setPlan(null);
        try {
            const r = await Notes.ChangeFolder(path, mode);
            setNotice({text: r.text, conflicts: r.conflicts ?? []});
        } catch (e) {
            setError(errText(e));
        }
        reload();
    }

    async function choose() {
        setError('');
        try {
            const path = await Notes.ChooseFolder('Choose where Gloamlog saves your logs.');
            if (path) await change(path);
        } catch (e) {
            setError(errText(e));
        }
    }

    const cancel = <Button variant="line" onClick={() => setPlan(null)}>Cancel</Button>;
    const empty = plan?.theirs === 0;

    return (
        <Panel>
            <Block title="Log folder">
                {overview?.folderError && (
                    <Banner tone="danger" action={<div className="row-gap">
                        <Button size="sm" variant="line" onClick={async () => { try { await Notes.RecreateFolder(); } catch (e) { setError(errText(e)); } reload(); }}>Create it again</Button>
                        <Button size="sm" variant="line" onClick={reload}>Try again</Button>
                    </div>}>
                        <strong>Can't use this folder.</strong> Gloamlog can't find it or can't write to it. {overview.folderError}
                    </Banner>
                )}
                {error && <Banner tone="danger" action={<Button size="sm" variant="line" onClick={() => setError('')}>Dismiss</Button>}><strong>Not changed.</strong> {error}</Banner>}
                {notice && (
                    <Banner tone="accent" action={<div className="row-gap">
                        {notice.conflicts.length > 0 && <Button size="sm" variant="line" onClick={() => Notes.Reveal(notice.conflicts)}>Show them</Button>}
                        <Button size="sm" variant="line" onClick={() => setNotice(null)}>Dismiss</Button>
                    </div>}>{notice.text}</Banner>
                )}
                <Setting title="Folder" hint={sum && `${plural(sum.logs, 'log')} · ${sum.skipped} skipped ${sum.skipped === 1 ? 'day' : 'days'}${sum.unrecognized > 0 ? ` · ${plural(sum.unrecognized, 'file')} not recognised` : ''}`}>
                    <code title={s.storageFolder} style={{overflowWrap: 'anywhere'}}>{tilde(s.storageFolder, info)}</code>
                </Setting>
                <div className="row-gap" style={{justifyContent: 'flex-end', paddingBottom: 'var(--s3)'}}>
                    <Button variant="line" size="sm" onClick={() => Notes.Reveal([s.storageFolder])}>Show in Finder</Button>
                    <Button variant="line" size="sm" onClick={choose}>Change…</Button>
                    <Button variant="line" size="sm" disabled={isDefault} onClick={() => change(info.defaultFolder)}>Use default</Button>
                </div>
                <p className="small muted">Changing the folder never deletes a log: Gloamlog asks whether to copy or move them first. Pick a folder in iCloud Drive or Dropbox to keep your logs on every Mac.</p>
            </Block>
            <Block title="Safety copies">
                <Setting title="Location" hint={info.backupDirExists ? undefined : 'Created when a page is first overwritten.'}>
                    <div className="row-gap" style={{flexWrap: 'nowrap'}}>
                        <code title={info.backupDir} style={{overflowWrap: 'anywhere'}}>{tilde(info.backupDir, info)}</code>
                        <Button variant="line" size="sm" disabled={!info.backupDirExists} onClick={() => Notes.Reveal([info.backupDir])}>Open</Button>
                    </div>
                </Setting>
                <p className="small muted">Before a page is overwritten, Gloamlog keeps up to {MAX_COPIES} earlier versions of it here, outside your log folder, so they never sync. To bring one back, open the page and choose Restore previous version in the page's menu.</p>
            </Block>
            <Block title="Sync">
                {sync ? <SyncSettings state={sync} onReview={() => setConflictsOpen(true)}/> : <p className="small muted">Checking the folder…</p>}
            </Block>
            {/* The open page lives in the main window: its own copy is saved by the next flush there; settled days reload on folder-changed. */}
            {conflictsOpen && <ConflictDialog open onClose={() => setConflictsOpen(false)} onResolved={() => setSyncKey(k => k + 1)}/>}

            <Dialog open={!!plan} onClose={() => setPlan(null)}
                title={plan ? (empty ? `Move your ${plan.mine} logs to "${plan.name}"?` : `"${plan.name}" already has ${plan.theirs} logs.`) : ''}
                actions={plan && (empty ? <>
                    {cancel}
                    <Button variant="line" onClick={() => run(plan.path, 'use')}>Leave them</Button>
                    <Button variant="line" onClick={() => run(plan.path, 'move')}>Move logs</Button>
                    <Button onClick={() => run(plan.path, 'copy')}>Copy logs</Button>
                </> : <>
                    {cancel}
                    <Button variant="line" onClick={() => run(plan.path, 'use')}>Use this folder</Button>
                    <Button onClick={() => run(plan.path, 'copy')}>Use and copy mine over</Button>
                </>)}>
                <p>{empty ? 'Copy keeps your originals where they are.' : 'Days that exist in both are never overwritten.'}</p>
            </Dialog>
        </Panel>
    );
}
