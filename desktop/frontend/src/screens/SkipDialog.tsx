import {useEffect, useState, type KeyboardEvent} from 'react';
import {Banner, Button, Dialog, Field, Segmented} from '@personal/ui/react';
import {Notes, addDays, dayDate, plural, shortDay} from '../api';

const REASONS: [string, string][] = [['', 'No reason'], ['Holiday', 'Holiday'], ['Leave', 'Leave'], ['Sick', 'Sick'], ['Day off', 'Day off'], ['Other…', 'Other…']];
const MAX_REASON = 40;
const NONE_RANGE = 'None of those days is a scheduled day, or they already have writing.';
const NONE_LIST = 'None of those days could be skipped. They may already have writing.';

/** What went wrong, in plain words (the Go errors are short codes). */
export function skipMessage(e: unknown): string {
    const m = e instanceof Error ? e.message : String(e);
    if (m.includes('the day has writing')) return 'That day already has writing. Skipping would overwrite it.';
    if (m.includes('storage folder is missing')) return "Your notes folder can't be found. Pick another folder in Settings.";
    if (m.includes('not writable')) return "Gloamlog can't save to this folder. Pick another folder in Settings.";
    return m;
}

type Props = {
    open: boolean;
    /** One day (a day page or the session bar), or the days picked in Catch up. */
    days: string[];
    today: string;
    /** List mode: skip exactly `days` with one reason, no date range. Default: more than one day. */
    many?: boolean;
    onClose: () => void;
    /** The days that were skipped and the reason ("" for none). The dialog closes itself after this. */
    onSkipped: (days: string[], reason: string) => void;
};

export function SkipDialog({open, days, today, many, onClose, onSkipped}: Props) {
    const start = days[0] ?? today;
    const list = many ?? days.length > 1;
    const [reason, setReason] = useState('');
    const [other, setOther] = useState('');
    const [range, setRange] = useState(false);
    const [through, setThrough] = useState(start);
    const [weekdays, setWeekdays] = useState<number[]>([2, 3, 4, 5, 6]);
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (!open) return;
        setReason(''); setOther(''); setRange(false); setThrough(start); setError(''); setBusy(false);
        Notes.Settings().then(s => setWeekdays(s.weekdays)).catch(() => {});
    }, [open, start]);

    const end = through > start ? through : start;
    const isRange = !list && range && end > start;
    let workdays = 0;
    for (let d = start, i = 0; d <= end && i < 3700; d = addDays(d, 1), i++) if (weekdays.includes(dayDate(d).getDay() + 1)) workdays++;

    const why = reason === 'Other…' ? other.trim().slice(0, MAX_REASON) : reason;
    const title = list ? (days.length === 1 ? 'Skip this day?' : `Skip these ${days.length} days?`)
        : isRange ? 'Skip these days?' : start === today ? 'Skip today?' : 'Skip this day?';
    const label = list ? (days.length === 1 ? 'Skip day' : `Skip ${days.length} days`) : isRange ? `Skip ${plural(workdays, 'day')}` : 'Skip day';

    async function commit() {
        if (busy || days.length === 0) return;
        setBusy(true);
        setError('');
        try {
            let done: string[];
            if (list) {
                done = await Notes.SkipDays(days, why);
                if (done.length === 0) throw new Error(days.length === 1 ? skipMessage(new Error('the day has writing')) : NONE_LIST);
            } else if (isRange) {
                done = await Notes.SkipRange(start, end, why);
                if (done.length === 0) throw new Error(NONE_RANGE);
            } else {
                await Notes.Skip(start, why);
                done = [start];
            }
            onSkipped(done, why);
            onClose();
        } catch (e) {
            setError(skipMessage(e));
        } finally {
            setBusy(false);
        }
    }

    // Return is the default button, as in the sheet; a focused button keeps its own Return.
    const enter = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !(e.target instanceof HTMLButtonElement)) { e.preventDefault(); void commit(); }
    };

    return (
        <Dialog open={open} onClose={onClose} title={title} actions={<>
            <Button variant="line" onClick={onClose}>Cancel</Button>
            <Button onClick={() => void commit()} disabled={busy || days.length === 0}>{label}</Button>
        </>}>
            <div className="stack" onKeyDown={enter}>
                {list && days.length > 0 && <p className="muted">{days.length === 1 ? dayDate(start).toLocaleDateString('en-GB', {weekday: 'long', day: 'numeric', month: 'long'})
                    : `${shortDay(days[0])} to ${shortDay(days[days.length - 1])}`}</p>}
                <p className="small muted">No reminders, and your streak stays as it is. You can undo this any time.</p>
                <Segmented label="Reason (optional)" options={REASONS} value={reason} onChange={setReason}/>
                {reason === 'Other…' && (
                    <Field label={`Reason (up to ${MAX_REASON} characters)`}>
                        <input className="input" maxLength={MAX_REASON} value={other} onChange={e => setOther(e.target.value)} autoFocus/>
                    </Field>
                )}
                {!list && <>
                    <Segmented label="Skip" options={[['one', start === today ? 'Today only' : 'This day only'], ['range', 'Through a later date']]}
                        value={range ? 'range' : 'one'} onChange={v => setRange(v === 'range')}/>
                    {range && (
                        <Field label="Through" hint={isRange ? plural(workdays, 'workday') : undefined}>
                            <input className="input" type="date" min={start} value={through} onChange={e => e.target.value && setThrough(e.target.value)}/>
                        </Field>
                    )}
                </>}
                <p className="small muted">A day with writing can't be skipped, so nothing you wrote is touched.</p>
                {error && <Banner tone="danger">{error}</Banner>}
            </div>
        </Dialog>
    );
}
