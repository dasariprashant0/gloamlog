// Mock additions for the settings screens (merged into Notes in mock.ts). Same names and shapes as desktop/notes_settings.go.
// The import below is circular on purpose: Notes is only touched when a method runs, after both modules have loaded.
import {Notes} from './mock';

const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 60));
let login = false;
const info = () => ({version: '0.7.0', backupDir: '/Users/someone/Library/Application Support/Gloamlog/backups', backupDirExists: true,
    defaultFolder: '/Users/someone/Gloamlog', loginEnabled: login});
const nameOf = (p: string) => p.split('/').filter(Boolean).pop() ?? p;

export const settingsMock = {
    SettingsInfo: () => later(info()),
    SetLoginItem: (on: boolean) => { login = on; return later(info()); },
    OpenLoginItems: () => later(undefined),
    Reveal: (_paths: string[]) => later(undefined),
    RecreateFolder: () => later(undefined),
    // The picker answers with a folder that already holds logs when the name says so, so both questions can be tried.
    ChooseFolder: (message: string) => later(message.includes('repositor') ? '/Users/someone/Projects' : '/Users/someone/Documents/Logs'),
    PlanFolderChange: async (path: string) => {
        const s = await Notes.Settings();
        return later({path, name: nameOf(path), same: path === s.storageFolder, writable: !path.includes('Locked'),
            mine: 12, theirs: path.includes('Existing') ? 3 : 0});
    },
    ChangeFolder: async (path: string, mode: string) => {
        const s = await Notes.Settings();
        await Notes.SaveSettings({...s, storageFolder: path});
        const text = mode === 'use' ? `Now saving to ${nameOf(path)}.` : `${mode === 'move' ? 'Moved' : 'Copied'} 12 logs and 2 images.`;
        return later({text, conflicts: mode === 'use' ? [] : [`${path}/2026-10-01.md`]});
    },
};
