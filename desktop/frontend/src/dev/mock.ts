// A stand-in for the Go services, so the UI runs in a plain browser: `npm run dev:mock`. vite.config.ts swaps the generated
// bindings for this file in mock mode only; production builds never include it. Data lives in memory and resets on reload.
// Keep the function names and result shapes the same as desktop/notes.go and the Windows service.
import type {BackupInfo, CarryCard, Hit, MonthCell, Overview, Page, Settings, Week} from '../api';
import {settingsMock} from './mock-settings';
import {catchupMock} from './mock-catchup';

const pad = (n: number) => String(n).padStart(2, '0');
const key = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const parse = (k: string) => { const [y, m, d] = k.split('-').map(Number); return new Date(y, m - 1, d); };
const add = (k: string, n: number) => { const d = parse(k); d.setDate(d.getDate() + n); return key(d); };
const weekday = (k: string) => parse(k).getDay() + 1; // 1 = Sunday, like Foundation
const today = () => key(new Date());
const words = (body: string) => body.split('\n').filter(l => !l.trim().startsWith('#')).join(' ').split(/\s+/).filter(Boolean).length;

const settings: Settings = {
    reminderMinutes: 1015, weekdays: [2, 3, 4, 5, 6], mode: 'strict' as Settings['mode'], snoozeMinutes: 15,
    storageFolder: '/Users/someone/Gloamlog', launchAtLogin: true,
    template: '## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next',
    carryOverHeadings: ['To do next', 'Pending / blocked'], minWords: 20, onboarded: true, appearance: 'system' as Settings['appearance'],
    catchUpWindowDays: 30, closedAppReminders: true, showMenuBar: true,
    capture: {hotKey: {keyCode: 38, carbonModifiers: 6144}, timestamps: true, todoShorthand: true, jotsHeading: 'Jots', jotsCountTowardLogged: false},
    sources: {claude: false, git: false, consented: false, allowed: [], allProjects: false, autoAdd: false, gitRoots: [], heading: 'What I did'},
} as Settings;

// Six weeks of history: most workdays written, a few started, one skipped, a few not logged.
const days = new Map<string, {body: string; skipped?: string}>();
(() => {
    const lines = ['Fixed the export sheet so images keep their names.', 'Paired on the reminder planner; snooze now counts down.',
        'Wrote the release notes for 0.6 and sent them round.', 'Reviewed the sync conflict copy with the team.',
        'Shipped the search index; typing feels instant now.', 'Planned next week: catch-up session and the review screen.'];
    for (let i = 42; i >= 1; i--) {
        const d = add(today(), -i), wd = weekday(d);
        if (wd === 1 || wd === 7) continue;
        if (i % 11 === 0) continue; // not logged
        if (i === 9) { days.set(d, {body: '', skipped: 'Holiday'}); continue; }
        const short = i % 7 === 3;
        const done = lines[i % lines.length];
        days.set(d, {body: short ? `## What I did\n- Looked at ${done.toLowerCase()}` :
            `## What I did\n- ${done}\n- ${lines[(i + 2) % lines.length]}\n\n## To do next\n- [ ] Tidy the ${['sidebar', 'calendar', 'review', 'jot panel'][i % 4]}\n- [ ] Answer the open questions`});
    }
})();

function status(d: string): string {
    const t = today();
    if (d > t) return 'future';
    const f = days.get(d);
    if (f?.skipped) return 'skipped';
    if (f) return words(f.body) >= settings.minWords ? 'logged' : 'partial';
    return settings.weekdays.includes(weekday(d)) && d >= add(t, -42) ? 'missed' : 'off';
}

const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 60));

export const Notes = {
    Settings: () => later({...settings}),
    SaveSettings: (s: Settings) => { Object.assign(settings, s); return later({...settings}); },
    Overview: (): Promise<Overview> => {
        const t = today();
        const catchUp: string[] = [];
        for (let i = settings.catchUpWindowDays; i >= 1; i--) { const d = add(t, -i); if (status(d) === 'missed' || status(d) === 'partial') catchUp.push(d); }
        let current = 0;
        for (let d = add(t, -1); ; d = add(d, -1)) {
            const s = status(d);
            if (s === 'logged') current++; else if (s !== 'skipped' && s !== 'off') break;
            if (d < add(t, -60)) break;
        }
        const recent = [...days.keys()].filter(d => d !== t).sort().reverse().slice(0, 7);
        return later({today: t, todayStatus: status(t), catchUp, streak: {current, best: Math.max(current, 14)}, recent,
            folder: settings.storageFolder, folderError: '', onboarded: settings.onboarded,
            summary: {logs: days.size, skipped: [...days.values()].filter(v => v.skipped).length, unrecognized: 0}} as Overview);
    },
    Month: (year: number, month: number): Promise<MonthCell[][]> => {
        const first = new Date(year, month - 1, 1);
        const start = new Date(first);
        start.setDate(1 - ((first.getDay() + 6) % 7)); // weeks start on Monday
        const rows: MonthCell[][] = [];
        for (let r = 0; r < 6; r++) {
            const row: MonthCell[] = [];
            for (let c = 0; c < 7; c++) {
                const d = new Date(start); d.setDate(start.getDate() + r * 7 + c);
                const k = key(d);
                row.push({day: k, inMonth: d.getMonth() === month - 1, status: status(k), isToday: k === today(), isScheduled: settings.weekdays.includes(weekday(k))} as MonthCell);
            }
            if (r < 4 || row.some(c => c.inMonth)) rows.push(row);
        }
        return later(rows);
    },
    Heatmap: (weeks: number) => {
        const out = [];
        let monday = today(); while (weekday(monday) !== 2) monday = add(monday, -1);
        for (let w = weeks - 1; w >= 0; w--) {
            const col = [];
            for (let i = 0; i < 7; i++) { const d = add(monday, -7 * w + i); col.push({day: d, status: status(d), isToday: d === today()}); }
            out.push(col);
        }
        return later(out);
    },
    JumpTo: (input: string) => {
        const s = input.trim().toLowerCase();
        if (s === 'today') return later(today());
        if (s === 'yesterday') return later(add(today(), -1));
        if (/^-\d+$/.test(s)) return later(add(today(), Number(s)));
        if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return later(s);
        return later('');
    },
    Open: (day: string): Promise<Page> => {
        const f = days.get(day);
        const title = parse(day).toLocaleDateString('en-GB', {weekday: 'long', day: 'numeric', month: 'long'});
        let carry: CarryCard | null = null;
        if (day === today()) {
            for (let d = add(day, -1); d > add(day, -10); d = add(d, -1)) {
                const prev = days.get(d);
                if (prev && !prev.skipped) {
                    const items = prev.body.split('\n## To do next\n')[1]?.split('\n').filter(l => l.startsWith('- ')).map(l => l.replace(/^- (\[ \] )?/, '')) ?? [];
                    if (items.length) carry = {sourceDay: d, sourceLabel: parse(d).toLocaleDateString('en-GB', {weekday: 'short', day: 'numeric', month: 'short'}), items};
                    break;
                }
            }
        }
        const body = f ? f.body : settings.template;
        return later({day, title, body, exists: !!f, isSkipped: !!f?.skipped, skipReason: f?.skipped ?? '', status: status(day),
            words: f ? words(f.body) : 0, notLocal: false, carry, carryApplied: !!carry && body.includes('Carried over')} as Page);
    },
    Save: (day: string, body: string) => {
        if (body.trim()) days.set(day, {body}); else days.delete(day);
        return later({backedUp: true, backupWarning: ''});
    },
    CarryOver: (card: CarryCard, body: string) =>
        later(`${body.trimEnd()}\n\n## Carried over from ${card.sourceLabel}\n${card.items.map(i => `- [ ] ${i}`).join('\n')}\n`),
    Skip: (day: string, reason: string) => { days.set(day, {body: '', skipped: reason}); return later(undefined); },
    SkipDays: (list: string[], reason: string) => { list.forEach(d => days.set(d, {body: '', skipped: reason})); return later(list); },
    Unskip: (list: string[]) => { list.forEach(d => days.get(d)?.skipped && days.delete(d)); return later(list); },
    Backups: (day: string): Promise<BackupInfo[]> =>
        later(days.has(day) ? [{date: new Date(Date.now() - 3_600_000).toISOString(), path: `/backups/${day}/a.md`, words: 41} as unknown as BackupInfo] : []),
    Restore: (day: string) => later({day, body: days.get(day)?.body ?? '', isSkipped: false, skipReason: ''}),
    SaveImage: (day: string, name: string) => later(`assets/${day}-${name}`),
    Search: (query: string): Promise<Hit[]> => {
        const q = query.trim().toLowerCase();
        if (!q) return later([]);
        const hits: Hit[] = [];
        for (const [day, f] of [...days].sort((a, b) => b[0].localeCompare(a[0]))) {
            const at = f.body.toLowerCase().indexOf(q);
            if (at < 0) continue;
            const from = Math.max(0, at - 40);
            hits.push({day, heading: 'What I did', before: (from ? '…' : '') + f.body.slice(from, at), match: f.body.slice(at, at + q.length), after: f.body.slice(at + q.length, at + q.length + 80)} as Hit);
        }
        return later(hits);
    },
    Week: (day: string) => later({summary: {weekStart: day, weekEnd: add(day, 6), days: [], sections: [], loggedCount: 3, workdayCount: 5, skippedCount: 1, missingDays: []}, markdown: '# Week\n'} as unknown as Week),
    Words: (body: string) => later(words(body)),
    Suggestions: (day: string) => later({off: false, noneOn: false, heading: 'What I did', result: {day, notes: [], hiddenLine: '1 value hidden (1 token)',
        lastScanLine: 'Last look: today, 4 files read, 0 records skipped.', stats: {files: 4, skipped: 0}, hidden: {},
        bullets: [
            {id: 'c1', kind: 'commit', project: 'gloamlog', text: 'Committed: Port the day page to React (gloamlog 3fa2c19)', detail: '', evidence: '3fa2c19', time: ''},
            {id: 's1', kind: 'session', project: 'gloamlog', text: 'Worked in gloamlog with Claude Code for about 1h 20m (3 sessions, 12 files edited)', detail: 'DayPage.tsx, notes.go, app.css', evidence: 'Worked in gloamlog with Claude Code', time: ''},
            {id: 'p1', kind: 'prompt', project: 'design-system', text: 'Worked on design-system: make the theme follow other windows', detail: '', evidence: 'make the theme follow other windows', time: ''},
        ]}}),
    SuggestionLines: (bullets: {text: string}[]) => later(bullets.map(b => '- ' + b.text).join('\n')),
    AlreadyAdded: (bullets: {id: string; evidence: string}[], page: string) => later(bullets.filter(b => b.evidence && page.includes(b.evidence)).map(b => b.id)),
    SourceProjects: () => later([]),
    HotKeyStatus: () => later(''),
    ...settingsMock,
    ...catchupMock,
};

export const Windows = {
    ShowMain: () => later(undefined),
    ShowJot: () => later(undefined),
    HideJot: () => later(undefined),
    ShowSettings: () => { window.open('/settings.html', '_blank'); return later(undefined); },
};
