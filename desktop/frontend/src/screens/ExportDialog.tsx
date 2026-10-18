// ⇧⌘E: every way out of a review in one dialog. It says what would be written before anything is.
import {useEffect, useState} from 'react';
import {Button, Dialog} from '@personal/ui/react';
import {Notes} from '../api';
import type {ReviewExportInfo, ReviewRequest} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

export type SaveKind = 'markdown' | 'folder' | 'zip' | 'pdf';
export type CopyFlavor = 'rich' | 'markdown' | 'plain';

/** Set to false to turn Save as PDF off ("Coming in the next build"). */
export const PDF_ENABLED = true;

const SAVE_AS: [SaveKind, string][] = [
    ['markdown', 'Markdown file with an assets folder'],
    ['folder', 'Folder with the Markdown and its images'],
    ['zip', 'Zip of that folder'],
    ['pdf', 'PDF'],
];

export function ExportDialog({open, title, req, onClose, onCopy, onSave}: {
    open: boolean; title: string; req: ReviewRequest;
    onClose: () => void; onCopy: (flavor: CopyFlavor) => void; onSave: (kind: SaveKind) => void;
}) {
    const [kind, setKind] = useState<SaveKind>('markdown');
    const [info, setInfo] = useState<ReviewExportInfo | null>(null);
    const [error, setError] = useState('');

    // What would be written: read when the dialog opens, never written.
    useEffect(() => {
        if (!open) return;
        let live = true;
        setInfo(null); setError('');
        Notes.ReviewExportInfo(req).then(i => live && setInfo(i), e => live && setError(String(e?.message ?? e)));
        return () => { live = false; };
    }, [open]); // eslint-disable-line react-hooks/exhaustive-deps

    return (
        <Dialog open={open} onClose={onClose} title={`Export ${title}`}
                actions={<>
                    <Button variant="ghost" onClick={onClose}>Cancel</Button>
                    <Button onClick={() => onSave(kind)}>Save…</Button>
                </>}>
            <p className="muted small" style={{marginBottom: 'var(--s4)'}}>{error ? `Can't read this review. ${error}` : (info?.detail ?? 'Counting…')}</p>
            <div className="stack">
                <div className="stack" style={{gap: 'var(--s2)'}}>
                    <span className="label">Copy</span>
                    <div className="row-gap">
                        <Button variant="line" size="sm" onClick={() => onCopy('rich')} title="Pastes formatted into Notes, Mail and chat; Markdown where only text fits">Rich text</Button>
                        <Button variant="line" size="sm" onClick={() => onCopy('markdown')}>Markdown</Button>
                        <Button variant="line" size="sm" onClick={() => onCopy('plain')} title="No # or ** marks: for a chat box">Plain text</Button>
                    </div>
                </div>
                <fieldset className="stack" style={{gap: 'var(--s2)', border: 0, padding: 0, margin: 0}}>
                    <legend className="label" style={{marginBottom: 'var(--s2)'}}>Save as</legend>
                    {SAVE_AS.map(([k, text]) => {
                        const off = k === 'pdf' && !PDF_ENABLED;
                        return (
                            <label key={k} className="small" style={{display: 'flex', gap: 'var(--s2)', alignItems: 'baseline', opacity: off ? 0.5 : 1}}>
                                <input type="radio" name="review-save-kind" checked={kind === k} disabled={off} onChange={() => setKind(k)}
                                       style={{accentColor: 'var(--accent)'}}/>
                                <span>{text}{off && <span className="muted"> · Coming in the next build</span>}</span>
                            </label>
                        );
                    })}
                </fieldset>
            </div>
        </Dialog>
    );
}
