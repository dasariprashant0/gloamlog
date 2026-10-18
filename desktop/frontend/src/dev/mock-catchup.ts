// Mock additions for the catchup screens (merged into Notes in mock.ts). Same names and shapes as desktop/notes_catchup.go.
// Plain methods, not arrows: they call the other mock methods through `this` (the Notes object they are merged into).
const pad = (n: number) => String(n).padStart(2, '0');
const key = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const parse = (k: string) => { const [y, m, d] = k.split('-').map(Number); return new Date(y, m - 1, d); };
const add = (k: string, n: number) => { const d = parse(k); d.setDate(d.getDate() + n); return key(d); };
const monday = (k: string) => add(k, -((parse(k).getDay() + 6) % 7));

export const catchupMock = {
    async CatchUp(this: any, windowDays: number) {
        const s = await this.Settings();
        const swap = windowDays > 0 && windowDays !== s.catchUpWindowDays;
        if (swap) await this.SaveSettings({...s, catchUpWindowDays: windowDays});
        const o = await this.Overview();
        if (swap) await this.SaveSettings(s);
        const days = await Promise.all((o.catchUp as string[]).map(async d => {
            const p = await this.Open(d);
            const started = p.status === 'partial';
            return {day: d, weekStart: monday(d), started, words: started ? p.words : 0};
        }));
        return {days, since: add(o.today, -42)};
    },
    async SkipRange(this: any, from: string, to: string, reason: string) {
        const s = await this.Settings();
        const done: string[] = [];
        for (let d = from; d <= to; d = add(d, 1)) {
            if (!s.weekdays.includes(parse(d).getDay() + 1)) continue;
            const p = await this.Open(d);
            if (p.exists && !p.isSkipped && p.words > 0) continue;
            await this.Skip(d, reason);
            done.push(d);
        }
        return done;
    },
    async BackupVersions(this: any, day: string) {
        const p = await this.Open(day);
        if (!p.exists) return [];
        const preview = (p.body as string).split('\n').filter(l => l.trim() && !l.startsWith('#')).join(' ').replace(/[-*[\] ]+/g, ' ').trim().slice(0, 120);
        const ago = (hours: number) => new Date(Date.now() - hours * 3_600_000).toISOString();
        return [
            {date: ago(1), path: `/backups/${day}/c.md`, words: p.words, preview},
            {date: ago(26), path: `/backups/${day}/b.md`, words: Math.max(1, Math.round(p.words / 2)), preview: preview.slice(0, 50)},
            {date: ago(50), path: `/backups/${day}/a.md`, words: 3, preview: ''},
        ];
    },
};
