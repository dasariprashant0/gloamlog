// Mock additions for the reminders (merged into Notes in mock.ts). Same names and shapes as desktop/reminders.go.
// The notice is always due in the mock; snoozing counts down Strict's two a day like the real planner.
const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 40));
const clock = () => new Date().toLocaleTimeString('en-GB', {hour: 'numeric', minute: '2-digit', hour12: true}).replace(/([ap]m)/i, m => m.toLowerCase());

let used = 0;
let perm = 'unknown';
let skipped = false;
const MAX_STRICT = 2;
const SNOOZE_MINUTES = 15;

const notice = () => {
    const left = Math.max(0, MAX_STRICT - used);
    return {
        visible: !skipped, strict: true, day: new Date().toISOString().slice(0, 10), time: clock(),
        snoozeLabel: left === 0 ? 'No snoozes left today' : `Remind me in ${SNOOZE_MINUTES} min · ${left} left`,
        canSnooze: left > 0, showSkipHint: used >= 2,
    };
};

export const remindersMock = {
    CurrentReminderNotice: () => later(notice()),
    SnoozeReminder: () => { if (used < MAX_STRICT) used++; return later(notice()); },
    SkipToday: () => { skipped = true; return later(undefined); },
    NotificationStatus: () => later(perm),
    RequestNotifications: () => { perm = 'allowed'; return later(perm); },
    NextReminder: () => later('Thu 15 Oct, 4:55 pm'),
};
