import {useState} from 'react';
import {Banner, Button, Dialog, Facts, Setting} from '@personal/ui/react';
import {Notes} from '../../api';
import {Block, errText, Panel, Soon, tilde, type PaneProps} from './shared';

type Plan = Awaited<ReturnType<typeof Notes.PlanFolderChange>>;
type Notice = {text: string; conflicts: string[]};

const MAX_COPIES = 10; // core.MaxBackupsPerDay
const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

/** Where the folder lives, from its path: the same labels the Swift app used. */
function folderKind(path: string): string {
    if (path.includes('/Library/Mobile Documents/')) return 'iCloud Drive';
    if (/\/Dropbox(\/|$)/.test(path)) return 'Dropbox';
    if (path.includes('/OneDrive')) return 'OneDrive';
    if (/\/(GoogleDrive|Google Drive)/.test(path)) return 'Google Drive';
    if (path.startsWith('/Volumes/')) return 'Network or external drive';
    return 'Local';
}

export function StoragePane({s, info, overview, reload}: PaneProps) {
    const [plan, setPlan] = useState<Plan | null>(null);
    const [notice, setNotice] = useState<Notice | null>(null);
    const [error, setError] = useState('');
    const isDefault = s.storageFolder.replace(/\/$/, '') === info.defaultFolder;
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
                <Facts items={[['Folder kind', folderKind(s.storageFolder)]]}/>
                <p className="small muted" style={{marginTop: 'var(--s3)'}}>Sync status, Download all and Resolve conflicts need the sync checks. <Soon/></p>
            </Block>

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
