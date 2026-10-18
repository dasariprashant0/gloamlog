// Mock additions for the sync screens (merged into Notes in mock.ts). Same names and shapes as desktop/notes_sync.go.
// The folder starts with one conflict (with a line each side lacks) and two days still downloading, so every state can be tried.
import type {ConflictItem, SyncStatus} from '../../bindings/github.com/dasariprashant0/gloamlog/desktop';

const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 60));
const ago = (hours: number) => new Date(Date.now() - hours * 3_600_000);
const stamp = (d: Date) => `${d.toLocaleDateString('en-GB', {weekday: 'short', day: 'numeric', month: 'short'})}, ${d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'}).toLowerCase()}`;

const conflicts: ConflictItem[] = [
    {
        name: '2026-10-08 2.md', day: '2026-10-08', family: 'iCloudNumbered', error: '', mineExists: true, theirsHasText: true,
        mine: {
            title: `This Mac, ${stamp(ago(5))}, 18 words`,
            lines: [{text: '## Done', differs: false}, {text: '- Shipped the sync fix', differs: false}, {text: '- Reviewed the budget', differs: true}, {text: '', differs: false},
                {text: '## Notes', differs: false}, {text: 'Quiet day, mostly heads-down.', differs: false}],
        },
        theirs: {
            title: `Other copy, ${stamp(ago(7))}, 21 words`,
            lines: [{text: '## Done', differs: false}, {text: '- Shipped the sync fix', differs: false}, {text: '- Called Sam about the invoice', differs: true}, {text: '', differs: false},
                {text: '## Notes', differs: false}, {text: 'Quiet day, mostly heads-down.', differs: false}],
        },
    },
];
let pending = ['2026-10-04', '2026-10-05'];

const status = (): SyncStatus => ({
    kind: 'icloud', kindLabel: 'iCloud Drive', source: 'iCloud', conflicts: conflicts.length,
    conflictDays: [...new Set(conflicts.map(c => c.day))], pendingDays: [...pending], blockedDays: [...pending],
});

export const syncMock = {
    SyncState: () => later(status()),
    SyncConflicts: () => later(conflicts.map(c => ({...c}))),
    ResolveConflict: async (name: string, choice: string) => {
        const i = conflicts.findIndex(c => c.name === name);
        if (i < 0) throw new Error('This conflict is already settled.');
        if (choice === 'keep-theirs' && !conflicts[i].theirsHasText) throw new Error('The other copy has no text to take.');
        conflicts.splice(i, 1);
        return later(undefined);
    },
    DownloadDay: (day: string) => { pending = pending.filter(d => d !== day); return later(undefined); },
    DownloadAll: () => { pending = []; return later(undefined); },
};
