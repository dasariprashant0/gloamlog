// What sync shows, quiet by design: nothing appears while there is nothing to say.
//   useSyncState    the state, read again whenever `refreshKey` changes (bump it on the "folder-changed" event)
//   SyncChip        one line in the sidebar footer: "2 conflicts to review" or "3 days still downloading"
//   ConflictBanner  above a day page while a conflict copy exists; closed with Dismiss, back for a new conflict
//   DownloadingPage the page for a day that is still in the cloud: nothing is written, it opens when it arrives
//   SyncSettings    the Sync section of Settings > Storage
import {useEffect, useState} from 'react';
import {Banner, Button, Display, Setting} from '@personal/ui/react';
import {Notes, plural} from '../api';
import type {SyncStatus} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

/** The sync state, or null until it has been read. Read again each time `refreshKey` changes. */
export function useSyncState(refreshKey = 0): SyncStatus | null {
    const [state, setState] = useState<SyncStatus | null>(null);
    useEffect(() => {
        let live = true;
        Notes.SyncState().then(s => { if (live) setState(s); }).catch(() => { /* the chip stays as it was; the next change asks again */ });
        return () => { live = false; };
    }, [refreshKey]);
    return state;
}

type ChipProps = {
    state: SyncStatus | null;
    /** Open the conflict dialog. */
    onReview: () => void;
    /** Open Settings > Storage (where "Download all" is). */
    onOpenStorage: () => void;
};

/** One line above the footer: "2 conflicts to review" (opens the dialog) or "3 days still downloading" (opens Settings). */
export function SyncChip({state, onReview, onOpenStorage}: ChipProps) {
    const conflicts = state?.conflicts ?? 0;
    const pending = state?.pendingDays?.length ?? 0;
    if (!state || (conflicts === 0 && pending === 0)) return null;
    const text = conflicts > 0 ? `${plural(conflicts, 'conflict')} to review` : `${plural(pending, 'day')} still downloading`;
    const help = conflicts > 0
        ? 'Your sync service saved a second version of a day. Nothing is deleted: you choose what to keep.'
        : `Waiting for these days to arrive from ${state.source}. Gloamlog won't write over them.`;
    return (
        <button className="btn btn-ghost btn-sm" title={help} aria-label={conflicts > 0 ? `${text}. Review them` : text}
            style={conflicts > 0 ? {color: 'var(--warning)'} : undefined} onClick={conflicts > 0 ? onReview : onOpenStorage}>
            {text}{conflicts > 0 && <strong> · Review</strong>}
        </button>
    );
}

type BannerProps = {
    state: SyncStatus | null;
    /** The folder's name, for "2 conflicts in Gloamlog." */
    folder: string;
    onReview: () => void;
};

/** Above a day page while a conflict copy exists: closed with Dismiss until the set of conflicts changes. */
export function ConflictBanner({state, folder, onReview}: BannerProps) {
    const [dismissed, setDismissed] = useState('');
    const conflicts = state?.conflicts ?? 0;
    const key = `${conflicts}:${state?.conflictDays?.join(',') ?? ''}`;
    if (!state || conflicts === 0 || dismissed === key) return null;
    return (
        <Banner tone="warning" action={<span className="row-gap">
            <Button variant="ghost" size="sm" onClick={onReview}>Review…</Button>
            <Button variant="ghost" size="sm" onClick={() => setDismissed(key)}>Dismiss</Button>
        </span>}>
            <strong>{plural(conflicts, 'conflict')}</strong> in {folder}. Your sync service saved another copy of a day.
            Nothing is deleted: choose which version to keep, or keep both.
        </Banner>
    );
}

type DownloadingProps = {
    day: string;
    /** The day's title, "Wednesday 14 October" (Page.title). */
    title: string;
    /** "iCloud", the provider's name, or "your cloud folder" (SyncStatus.source). */
    source: string;
    onOpenStorage: () => void;
};

/** The page for a day that is not on this Mac yet. No editor, no template, nothing is written: the caller reloads the page when the
 *  folder changes, so it opens by itself when the day arrives. Asks the provider for the day when it appears. */
export function DownloadingPage({day, title, source, onOpenStorage}: DownloadingProps) {
    useEffect(() => { void Notes.DownloadDay(day).catch(() => { /* asked again by Try again and by the next scan */ }); }, [day]);
    return (
        <div className="stack">
            <Display lines={[title]} size="sm"/>
            <p><strong>Downloading from {source}…</strong></p>
            <p className="muted" style={{maxWidth: 'var(--measure)'}}>
                This day is in your shared folder but not on this Mac yet. Gloamlog is waiting for it and won't start a new page here,
                so nothing you wrote on another Mac gets overwritten. It opens by itself when it arrives.
            </p>
            <div className="row-gap">
                <Button onClick={() => void Notes.DownloadDay(day).catch(() => {})}>Try again</Button>{/* a failed request: the page stays here and the next scan asks again */}
                <Button variant="ghost" onClick={onOpenStorage}>Open Storage settings</Button>
            </div>
        </div>
    );
}

type SettingsProps = {
    state: SyncStatus | null;
    onReview: () => void;
};

/** Settings > Storage: the folder's kind, what sync is waiting for, and the way into the conflict dialog. */
export function SyncSettings({state, onReview}: SettingsProps) {
    if (!state) return null;
    const conflicts = state.conflicts, pending = state.pendingDays.length;
    const kindHelp = state.kind === 'local'
        ? 'Only on this Mac. Pick a folder in iCloud Drive or Dropbox to use one log on two Macs.'
        : state.kind === 'network'
            ? 'On a network or external drive. Gloamlog checks it every few seconds.'
            : `Gloamlog doesn't run sync. ${state.source} moves the files and Gloamlog watches the folder.`;
    return (
        <div className="stack">
            <Setting title="Folder type" hint={kindHelp}><span>{state.kindLabel}</span></Setting>
            {conflicts > 0 && (
                <Banner tone="warning" action={<Button variant="line" size="sm" onClick={onReview}>Resolve conflicts</Button>}>
                    <strong>{plural(conflicts, 'conflict')}</strong> to resolve.
                </Banner>
            )}
            {pending > 0 && (
                <Banner action={<Button variant="line" size="sm" onClick={() => void Notes.DownloadAll()}>Download all</Button>}>
                    {plural(pending, 'day')} still downloading from {state.source}.
                </Banner>
            )}
            {conflicts === 0 && pending === 0 && (
                <p className="small muted">{state.kind === 'local' ? 'This folder is only on this Mac.' : 'Every day in this folder is on this Mac.'}</p>
            )}
            <p className="small muted">
                If two Macs change the same day, Gloamlog asks which version to keep (or keeps both) and saves the other one under Safety copies.
                Days that are still downloading are never written over.
            </p>
        </div>
    );
}
