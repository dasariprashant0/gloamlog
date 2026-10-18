// The slim reminder notice on today's page (PARITY 3.C, 3.O). Go decides when it is due ("reminder-notice" event, or
// Notes.CurrentReminderNotice on mount); the page shows it only while today is open and the person has not dismissed it.
import {Banner, Button} from '@personal/ui/react';
import type {ReminderNoticeState} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

export function ReminderNotice({notice, onSnooze, onSkipDay, onDismiss}: {
    notice: ReminderNoticeState; onSnooze: () => void; onSkipDay: () => void; onDismiss: () => void;
}) {
    if (!notice.visible) return null;
    const text = notice.strict ? `It's ${notice.time}. Time to write up today.` : "Today isn't logged yet.";
    return (
        <Banner tone="accent" action={
            <div className="row-gap">
                {notice.strict && notice.showSkipHint && <span className="small muted">Not today?</span>}
                <Button variant="line" size="sm" disabled={!notice.canSnooze} onClick={onSnooze}>{notice.snoozeLabel}</Button>
                <Button variant="line" size="sm" onClick={onSkipDay}>Skip day</Button>
                <Button variant="line" size="sm" title="Dismiss" onClick={onDismiss}><span aria-label="Dismiss">×</span></Button>
            </div>
        }>{text}</Banner>
    );
}
