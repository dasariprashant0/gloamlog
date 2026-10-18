import {useEffect, useState, type KeyboardEvent} from 'react';
import {Banner, Button, Dialog} from '@personal/ui/react';
import {Notes, dayDate, plural} from '../api';
import type {BackupVersion} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

/** "Mon 5 Oct, 4:30 pm" in the local time zone (the stamp in the file name is UTC). */
function stamp(iso: string): string {
    const d = new Date(iso);
    return `${d.toLocaleDateString('en-GB', {weekday: 'short', day: 'numeric', month: 'short'})}, ${d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'}).toLowerCase()}`;
}

function restoreMessage(e: unknown): string {
    const m = e instanceof Error ? e.message : String(e);
    if (m.includes('could not back up')) return "Couldn't keep a copy of the current page, so nothing was changed.";
    if (m.includes('backups are off')) return 'Backups are turned off.';
    if (m.includes('storage folder is missing')) return "Your notes folder can't be found. Pick another folder in Settings.";
    if (m.includes('not writable')) return "Gloamlog can't save to this folder. Pick another folder in Settings.";
    return m;
}

type Props = {
    open: boolean;
    day: string;
    onClose: () => void;
    /** The restored page's markdown body, so the day page can show it. The dialog closes itself after this. */
    onRestored: (body: string) => void;
};

export function RestoreDialog({open, day, onClose, onRestored}: Props) {
    const [items, setItems] = useState<BackupVersion[] | null>(null);
    const [picked, setPicked] = useState('');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (!open) return;
        let live = true;
        setItems(null); setPicked(''); setError(''); setBusy(false);
        Notes.BackupVersions(day)
            .then(v => { if (live) { setItems(v); setPicked(v[0]?.path ?? ''); } })
            .catch(e => { if (live) { setItems([]); setError(restoreMessage(e)); } });
        return () => { live = false; };
    }, [open, day]);

    async function restore() {
        const v = items?.find(i => i.path === picked);
        if (!v || busy) return;
        setBusy(true);
        setError('');
        try {
            const doc = await Notes.Restore(day, {date: v.date, path: v.path, words: v.words});
            onRestored(doc.body);
            onClose();
        } catch (e) {
            setError(restoreMessage(e));
        } finally {
            setBusy(false);
        }
    }

    const enter = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !(e.target instanceof HTMLButtonElement)) { e.preventDefault(); void restore(); }
    };
    const none = items !== null && items.length === 0;
    const longDay = dayDate(day).toLocaleDateString('en-GB', {weekday: 'long', day: 'numeric', month: 'long'});

    return (
        <Dialog open={open} onClose={onClose} title="Restore a previous version" actions={<>
            <Button variant="line" onClick={onClose}>{none ? 'Close' : 'Cancel'}</Button>
            <Button onClick={() => void restore()} disabled={!picked || busy}>Restore</Button>
        </>}>
            <div className="stack" onKeyDown={enter}>
                <p className="small muted">{longDay}. Gloamlog keeps the last 10 versions of each day. Restoring saves the current page as a version first, so you can undo it.</p>
                {items === null && <p className="muted">Loading…</p>}
                {none && <p className="muted">No earlier versions yet. They appear after you change a page that was already saved.</p>}
                {items !== null && items.length > 0 && (
                    <ul className="list" role="radiogroup" aria-label="Versions, newest first" style={{maxHeight: 'calc(var(--s9) * 3)', overflowY: 'auto'}}>
                        {items.map(v => (
                            <li key={v.path}>
                                <label className="row">
                                    <input type="radio" name="version" checked={picked === v.path} onChange={() => setPicked(v.path)}
                                        style={{accentColor: 'var(--accent)'}}/>
                                    <span className="row-body">
                                        <span className="row-title">{stamp(v.date)}</span>
                                        {v.preview && <span className="row-meta">{v.preview}</span>}
                                    </span>
                                    <span className="row-end small muted">{plural(v.words, 'word')}</span>
                                </label>
                            </li>
                        ))}
                    </ul>
                )}
                {error && <Banner tone="danger">{error}</Banner>}
            </div>
        </Dialog>
    );
}
