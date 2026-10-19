// "From yesterday" on a list page: one line ("2 open from Sun 18 Oct") until Review opens the last logged day's open lines.
// Keep adds a line with its mark, Done adds it as done, Drop forgets it for today (Notes.CarryDismiss). Nothing is written
// to the page until one is picked.
import {useState} from 'react';
import {Button} from '@personal/ui/react';
import {Notes, type CarryCard} from '../api';
import type {Mark} from './model';

export function CarryBox({day, card, onAdd, onToast}: {
    day: string; card: CarryCard; onAdd: (text: string, mark: Mark, list: string) => void; onToast: (message: string) => void;
}) {
    const marks = (card as unknown as {marks?: Mark[]}).marks ?? [];
    // Each line goes back to its own list (Work or Personal); an item without one goes to Work.
    const [left, setLeft] = useState(() => card.items.map((text, i) => ({text, mark: (marks[i] ?? 'next') as Mark, list: card.lists?.[i] || 'Work'})));
    const [hidden, setHidden] = useState(false);
    const [open, setOpen] = useState(false);
    if (hidden || left.length === 0) return null;
    const take = (i: number) => setLeft(l => l.filter((_, k) => k !== i));
    const keep = (i: number) => { onAdd(left[i].text, left[i].mark, left[i].list); take(i); };
    const done = (i: number) => { onAdd(left[i].text, '', left[i].list); take(i); };
    const drop = (i: number) => {
        const it = left[i];
        take(i);
        Notes.CarryDismiss(day, it.text).catch(e => onToast(`Couldn't forget that line: ${String(e?.message ?? e)}`));
    };
    return (
        <section className="carry-box" aria-label={`From ${card.sourceLabel}`}>
            <div className="carry-head">
                <p className="small muted">{left.length} open from {card.sourceLabel}</p>
                <div className="row-gap carry-actions">
                    {open && <Button variant="ghost" size="sm" onClick={() => { left.forEach(it => onAdd(it.text, it.mark, it.list)); setLeft([]); }}>Keep all</Button>}
                    {open && <Button variant="ghost" size="sm" onClick={() => setHidden(true)}>Hide</Button>}
                    <Button variant="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(o => !o)}>{open ? 'Close' : 'Review'}</Button>
                </div>
            </div>
            {open && (
                <ul className="carry-list">
                    {left.map((it, i) => (
                        <li key={it.text} className="carry-row">
                            <span>{it.text}{it.mark && <> <span className={`badge log-mark mark-${it.mark}`}>{it.mark}</span></>}</span>
                            <span className="row-gap carry-actions">
                                <Button variant="ghost" size="sm" onClick={() => done(i)}>Done</Button>
                                <Button variant="ghost" size="sm" onClick={() => keep(i)}>Keep</Button>
                                <Button variant="ghost" size="sm" onClick={() => drop(i)}>Drop</Button>
                            </span>
                        </li>
                    ))}
                </ul>
            )}
        </section>
    );
}
