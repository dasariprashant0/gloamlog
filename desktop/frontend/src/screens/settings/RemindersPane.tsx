import {Setting, Segmented, Switch} from '@personal/ui/react';
import {Block, DAY_NAMES, Panel, ResetBar, Soon, systemWeekStart, type PaneProps} from './shared';

const SNOOZE = [5, 10, 15, 20, 30];
const pad = (n: number) => String(n).padStart(2, '0');

/** "Thu 8 Oct, 4:55 pm", or "" when no day is on. Today counts only while its time is still ahead and the day still needs a log. */
function nextReminder(minutes: number, days: number[], todayDone: boolean): string {
    const now = new Date();
    for (let i = 0; i < 8; i++) {
        const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + i, Math.floor(minutes / 60), minutes % 60);
        if (!days.includes(d.getDay() + 1) || (i === 0 && (todayDone || d <= now))) continue;
        const day = d.toLocaleDateString('en-GB', {weekday: 'short', day: 'numeric', month: 'short'});
        const time = d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'}).replace(/\s/g, ' ').toLowerCase();
        return `${day}, ${time}`;
    }
    return '';
}

export function RemindersPane({s, set, overview}: PaneProps) {
    const first = s.weekStart || systemWeekStart();
    const order = Array.from({length: 7}, (_, i) => ((first - 1 + i) % 7) + 1);
    const done = ['logged', 'skipped'].includes(String(overview?.todayStatus));
    const next = nextReminder(s.reminderMinutes, s.weekdays, done);
    const strict = String(s.mode) === 'strict';
    const past = 'Past days are never forced on you; they wait in Catch up.';

    return (
        <Panel>
            <Block title="End of day">
                <Setting title="Remind me at" hint={`Next reminder: ${next || 'none planned'}`}>
                    <input className="input input-sm" type="time" aria-label="Remind me at" required
                        value={`${pad(Math.floor(s.reminderMinutes / 60))}:${pad(s.reminderMinutes % 60)}`}
                        onChange={e => { const [h, m] = e.target.value.split(':').map(Number); if (Number.isFinite(h) && Number.isFinite(m)) set({reminderMinutes: h * 60 + m}); }}/>
                </Setting>
                <Setting title="On these days" hint="Catch up and your streak count the same days. Keep at least one.">
                    <div className="seg" role="group" aria-label="On these days">
                        {order.map(d => {
                            const on = s.weekdays.includes(d);
                            const last = on && s.weekdays.length === 1;
                            return (
                                <button key={d} className="seg-opt small" aria-pressed={on} aria-label={DAY_NAMES[d - 1]} disabled={last}
                                    title={last ? 'Keep at least one day.' : DAY_NAMES[d - 1]}
                                    onClick={() => set(cur => ({weekdays: on ? cur.weekdays.filter(x => x !== d) : [...cur.weekdays, d].sort()}))}>
                                    {DAY_NAMES[d - 1].slice(0, 3)}
                                </button>
                            );
                        })}
                    </div>
                </Setting>
            </Block>
            <Block title="When it fires">
                <Setting title="Style" hint={strict
                    ? `Strict brings Gloamlog to the front, then reopens it every 5 minutes if you close it, until you save or skip today. Two snoozes a day. ${past}`
                    : `Gentle sends one notification and nothing else. Snooze as often as you like. ${past}`}>
                    <Segmented label="Style" options={[['strict', 'Strict'], ['gentle', 'Gentle']]} value={String(s.mode) as 'strict' | 'gentle'}
                        onChange={v => set({mode: v as typeof s.mode})}/>
                </Setting>
                <Setting title="Snooze for">
                    <select className="select" aria-label="Snooze for" style={{width: 'auto'}} value={s.snoozeMinutes}
                        onChange={e => set({snoozeMinutes: Number(e.target.value)})}>
                        {SNOOZE.map(m => <option key={m} value={m}>{m} minutes</option>)}
                    </select>
                </Setting>
            </Block>
            <Block title="Notifications">
                <Setting title="Notification access" hint="Whether macOS lets Gloamlog send notifications, and a way to allow them.">
                    <Soon/>
                </Setting>
                <Setting title="Remind me when Gloamlog is closed" hint={s.closedAppReminders
                    ? 'macOS holds your next 14 reminders, so one arrives even when Gloamlog is quit. Nothing arrives while the Mac sleeps (it comes when it wakes), and a Focus can silence it. Strict\'s window needs Gloamlog running. Open Gloamlog now and then to renew the reminders.'
                    : 'Reminders arrive only while Gloamlog is running. Turn on Open at login (General) to keep it running after a restart.'}>
                    <Switch checked={s.closedAppReminders} onChange={v => set({closedAppReminders: v})} label={<span className="muted small">{s.closedAppReminders ? 'On' : 'Off'}</span>}/>
                </Setting>
            </Block>
            <ResetBar pane="Reminders" summary="the reminder time, days, style, snooze length and closed-app reminders"
                onReset={() => set({reminderMinutes: 1015, weekdays: [2, 3, 4, 5, 6], mode: 'strict' as typeof s.mode, snoozeMinutes: 15, closedAppReminders: true})}/>
        </Panel>
    );
}
