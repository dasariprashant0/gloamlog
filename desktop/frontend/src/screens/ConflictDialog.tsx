import {useEffect, useRef, useState, type KeyboardEvent} from 'react';
import {Banner, Button, Dialog} from '@personal/ui/react';
import {Notes, dayDate} from '../api';
import {announce} from '../announce';
import type {ConflictItem, ConflictSide} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

const UNSAVED = "This page has edits that couldn't be saved yet. Fix that first, so nothing you typed is lost.";
const SETTLED = 'Conflict settled. The other version is kept in your safety copies.';

type Choice = 'keep-mine' | 'keep-theirs' | 'keep-both';

type Props = {
    open: boolean;
    onClose: () => void;
    /** Saves the open page, so "this Mac" includes what was typed a moment ago. Resolve false when it has edits that could not be
     *  saved: the dialog then says so and shows nothing to choose from. Leave it out when no page is open. */
    flush?: () => Promise<boolean>;
    /** A conflict of this day was settled (the page on disk may have changed): reload it if it is open. */
    onResolved: (day: string) => void;
};

function message(e: unknown): string {
    return e instanceof Error ? e.message : String(e);
}

/** One version of the day: its title, and every line with a bar on the ones the other version lacks. */
function Column({side}: {side: ConflictSide}) {
    return (
        <div className="stack">
            <strong className="small">{side.title}</strong>
            <div role="group" aria-label={side.title} style={{maxHeight: 'calc(var(--s9) * 3)', overflowY: 'auto', background: 'var(--wash-2)', padding: 'var(--s2) 0'}}>
                {side.lines.map((l, i) => (
                    <div key={i} aria-label={l.differs ? `Only in this version: ${l.text}` : undefined} className="small"
                        style={{
                            whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', minHeight: '1lh', padding: `0 var(--s2) 0 var(--s3)`,
                            ...(l.differs ? {background: 'var(--wash)', boxShadow: 'inset var(--s1) 0 0 var(--warning)'} : {}),
                        }}>
                        {l.text || ' '}
                    </div>
                ))}
            </div>
        </div>
    );
}

export function ConflictDialog({open, onClose, flush, onResolved}: Props) {
    const [items, setItems] = useState<ConflictItem[] | null>(null);
    const [index, setIndex] = useState(0);
    const [error, setError] = useState('');
    const [done, setDone] = useState('');
    const [busy, setBusy] = useState(false);
    const anchor = useRef<HTMLDivElement>(null);
    const flushRef = useRef(flush); // the caller's function may change on every render; reading it here must not reload the list
    flushRef.current = flush;

    // The conflict view is wider than a plain question: take the page's content width, from the token, once the dialog is open.
    useEffect(() => {
        const d = anchor.current?.closest('dialog');
        if (open && d) d.style.width = 'min(var(--content), 92vw)';
    }, [open]);

    async function load(keep = 0): Promise<ConflictItem[]> {
        const list = await Notes.SyncConflicts();
        setItems(list);
        setIndex(Math.min(keep, Math.max(0, list.length - 1)));
        return list;
    }

    useEffect(() => {
        if (!open) return;
        let live = true;
        setItems(null); setIndex(0); setError(''); setDone(''); setBusy(false);
        (async () => {
            if (flushRef.current && !(await flushRef.current())) { if (live) { setItems([]); setError(UNSAVED); } return; }
            const list = await Notes.SyncConflicts();
            if (live) setItems(list);
        })().catch(e => { if (live) { setItems([]); setError(message(e)); } });
        return () => { live = false; };
    }, [open]);

    const item = items?.[index];
    const ready = !!item && !item.error && !!item.mine && !!item.theirs;

    async function choose(choice: Choice) {
        if (!item || busy) return;
        setBusy(true);
        setError(''); setDone('');
        try {
            if (flushRef.current && !(await flushRef.current())) { setError(UNSAVED); return; }
            await Notes.ResolveConflict(item.name, choice);
            announce('Conflict settled.');
            onResolved(item.day);
            const left = await load(index);
            setDone(SETTLED);
            if (left.length === 0) onClose();
        } catch (e) {
            setError(message(e));
        } finally {
            setBusy(false);
        }
    }

    function step(delta: number) {
        if (!items?.length) return;
        setIndex((index + delta + items.length) % items.length);
        setError(''); setDone('');
    }

    const enter = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !(e.target instanceof HTMLButtonElement) && ready && !busy) { e.preventDefault(); void choose('keep-both'); }
    };
    const none = items !== null && items.length === 0;
    const longDay = item ? dayDate(item.day).toLocaleDateString('en-GB', {weekday: 'long', day: 'numeric', month: 'long'}) : '';
    const many = (items?.length ?? 0) > 1;

    return (
        <Dialog open={open} onClose={onClose} title="Two versions of this day" actions={<>
            <Button variant="line" onClick={onClose}>Close</Button>
            <Button variant="line" onClick={() => void choose('keep-mine')} disabled={busy || !ready || !item?.mineExists}
                title={item?.mineExists === false ? 'This Mac has no page for this day' : "Keep this Mac's page as it is"}>Keep mine</Button>
            <Button variant="line" onClick={() => void choose('keep-theirs')} disabled={busy || !ready || !item?.theirsHasText}
                title={item?.theirsHasText === false ? 'The other copy has no text' : "Replace this Mac's page with the other copy"}>Take theirs</Button>
            <Button onClick={() => void choose('keep-both')} disabled={busy || !ready}
                title={'Keep this Mac\'s page and add the other copy\'s extra lines under "From another copy"'}>Keep both</Button>
        </>}>
            <div className="stack" ref={anchor} onKeyDown={enter}>
                {items === null && <p className="muted">Loading…</p>}
                {none && !error && <p className="muted">No conflicts left.</p>}
                {item && (
                    <div className="row-gap">
                        <p className="small muted" style={{flex: 1}}>{longDay}{many ? ` · ${index + 1} of ${items!.length}` : ''}</p>
                        {many && <>
                            <Button variant="ghost" size="sm" onClick={() => step(-1)}>Previous conflict</Button>
                            <Button variant="ghost" size="sm" onClick={() => step(1)}>Next conflict</Button>
                        </>}
                    </div>
                )}
                {ready && (
                    <>
                        <div className="two" style={{gap: 'var(--s5)'}}>
                            <Column side={item.mine!}/>
                            <Column side={item.theirs!}/>
                        </div>
                        <p className="small muted">
                            Lines with a bar are only in that version. Keep both adds the other copy's extra lines under "From another copy".
                            The version you don't keep is saved under Safety copies, so nothing is deleted.
                        </p>
                    </>
                )}
                {item?.error && <Banner tone="warning">{item.error}</Banner>}
                {error && <Banner tone="danger">{error}</Banner>}
                <p className="sr-only" role="status">{done}</p>
            </div>
        </Dialog>
    );
}
