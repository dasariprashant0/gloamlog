// The day page's one status line (writing redesign spec 2.1): sync copies to review and the reminder, each with its actions,
// as quiet rows between two rules instead of stacked banners. A folder that can't be used still gets the full danger Banner.
import type {ReactNode} from 'react';
import {Button} from '@personal/ui/react';
import type {ReminderNoticeState} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

export type StatusItem = {key: string; text: ReactNode; actions: ReactNode};

export function StatusLine({items}: {items: StatusItem[]}) {
    if (!items.length) return null;
    return (
        <div className="status-line" role="status">
            {items.map(it => (
                <div key={it.key} className="status-item">
                    <span>{it.text}</span>
                    <span className="row-gap status-actions">{it.actions}</span>
                </div>
            ))}
        </div>
    );
}

/** Sync copies waiting to be reviewed (App owns the count and the dismissal). */
export type ConflictNote = {count: number; onReview: () => void; onDismiss: () => void};

export function conflictItem(c: ConflictNote): StatusItem {
    return {
        key: 'sync',
        text: c.count === 1 ? 'Your sync service saved another copy of a day.' : `Your sync service saved ${c.count} other copies of days.`,
        actions: <>
            <Button variant="ghost" size="sm" onClick={c.onReview}>Review…</Button>
            <Button variant="ghost" size="sm" onClick={c.onDismiss}>Dismiss</Button>
        </>,
    };
}

/** The reminder (PARITY 3.C, 3.O): Go decides when it is due; the page shows it while today is open and not dismissed. */
export function reminderItem(n: ReminderNoticeState, on: {snooze: () => void; skipDay: () => void; dismiss: () => void}): StatusItem {
    return {
        key: 'reminder',
        text: <>{n.strict ? `It's ${n.time}. Time to write up today.` : "Today isn't logged yet."}{n.strict && n.showSkipHint && <span className="muted"> Not today?</span>}</>,
        actions: <>
            <Button variant="ghost" size="sm" disabled={!n.canSnooze} onClick={on.snooze}>{n.snoozeLabel}</Button>
            <Button variant="ghost" size="sm" onClick={on.skipDay}>Skip day</Button>
            <Button variant="ghost" size="sm" onClick={on.dismiss}>Dismiss</Button>
        </>,
    };
}
