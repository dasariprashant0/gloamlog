// The sidebar's month: one button per day up to today, marked by status; it follows the open page.
import {useEffect, useState} from 'react';
import {Notes, dayDate, type MonthCell} from '../api';

const SPOKEN: Record<string, string> = {logged: 'logged', partial: 'started, not logged yet', skipped: 'skipped', missed: 'not logged', off: 'not a workday', future: ''};

export function MonthCalendar({selected, today, version, onPick}: {selected: string | null; today: string; version: number; onPick: (day: string) => void}) {
    const anchor = dayDate(selected ?? today);
    const [shown, setShown] = useState({y: anchor.getFullYear(), m: anchor.getMonth() + 1});
    const [rows, setRows] = useState<MonthCell[][]>([]);

    // Follow the open page into its month.
    useEffect(() => {
        const d = dayDate(selected ?? today);
        setShown({y: d.getFullYear(), m: d.getMonth() + 1});
    }, [selected, today]);

    useEffect(() => {
        let live = true;
        Notes.Month(shown.y, shown.m).then(r => live && setRows(r ?? [])).catch(() => live && setRows([]));
        return () => { live = false; };
    }, [shown.y, shown.m, version]);

    const t = dayDate(today);
    const atThisMonth = shown.y === t.getFullYear() && shown.m === t.getMonth() + 1;
    const step = (n: number) => setShown(({y, m}) => {
        const d = new Date(y, m - 1 + n, 1);
        return {y: d.getFullYear(), m: d.getMonth() + 1};
    });
    const title = new Date(shown.y, shown.m - 1, 1).toLocaleDateString('en-GB', {month: 'long', year: 'numeric'});

    return (
        <section className="cal" aria-label="Calendar">
            <div className="cal-head">
                <button className="cal-step" onClick={() => step(-1)} aria-label="Previous month" disabled={shown.y <= 2000 && shown.m <= 1}>‹</button>
                <span className="small">{title}</span>
                <button className="cal-step" onClick={() => step(1)} aria-label="Next month" disabled={atThisMonth}>›</button>
            </div>
            <div className="cal-grid" role="grid">
                {rows[0]?.map(c => <span key={'h' + c.day} className="cal-dow" aria-hidden="true">{dayDate(c.day).toLocaleDateString('en-GB', {weekday: 'narrow'})}</span>)}
                {rows.flat().map(c => {
                    const n = dayDate(c.day).getDate();
                    if (c.status === 'future') return <span key={c.day} className={`cal-day future${c.inMonth ? '' : ' out'}`}>{n}</span>;
                    const spoken = `${dayDate(c.day).toLocaleDateString('en-GB', {weekday: 'long', day: 'numeric', month: 'long'})}${SPOKEN[c.status] ? ', ' + SPOKEN[c.status] : ''}`;
                    return (
                        <button key={c.day} className={`cal-day mark-${c.status}${c.inMonth ? '' : ' out'}${c.isToday ? ' today' : ''}`}
                                aria-current={c.day === selected ? 'date' : undefined} aria-label={spoken} title={spoken} onClick={() => onPick(c.day)}>
                            {n}
                        </button>
                    );
                })}
            </div>
        </section>
    );
}
