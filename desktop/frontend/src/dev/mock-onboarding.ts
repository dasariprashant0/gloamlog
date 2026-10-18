// Mock additions for onboarding, What's new and the updater (merged into Notes in mock.ts by the lead). Same names and shapes
// as desktop/notes_onboarding.go, desktop/updater.go and the Claude calls in desktop/claude.go.
// Try the update states in a browser with ?update=none|error|uptodate|available (default available) and
// ?install=failed|fallback; ?folder=logs makes the folder step find existing logs.
const later = <T,>(v: T, ms = 60) => new Promise<T>(r => setTimeout(() => r(v), ms));
const query = (k: string, d: string) => { try { return new URLSearchParams(location.search).get(k) ?? d; } catch { return d; } };

let seen = false;
let claudeState = query('claude', 'notConnected');

const release = {
    version: '0.8.0',
    notes: 'Fixes the export sheet and makes search faster.\n\nThe Jot panel now remembers where you left it.',
    pageURL: 'https://github.com/dasariprashant0/gloamlog/releases/tag/v0.8.0',
    zipURL: 'https://github.com/dasariprashant0/gloamlog/releases/download/v0.8.0/Gloamlog.zip',
    shaURL: 'https://github.com/dasariprashant0/gloamlog/releases/download/v0.8.0/Gloamlog.zip.sha256',
};

export const onboardingMock = {
    OnboardingInfo: (folder: string) => later({
        icloudFolder: '/Users/someone/Library/Mobile Documents/com~apple~CloudDocs/Gloamlog',
        defaultFolder: '/Users/someone/Gloamlog',
        found: folder.includes('logs') || folder.includes('Documents') || query('folder', '') === 'logs'
            ? {count: 42, first: '2026-01-05', last: '2026-10-14'} : null,
    }),
    WhatsNewSeen: () => later(seen),
    MarkWhatsNewSeen: () => { seen = true; return later(undefined); },

    ClaudeStatus: () => later({
        state: claudeState,
        message: claudeState === 'noClaude' ? 'Claude Code not found' : claudeState === 'connected' ? 'Connected' : 'Not connected',
    }, 400),
    ConnectClaude: () => { claudeState = 'connected'; return later({ok: true, state: 'connected',
        message: 'Connected. Start a new Claude Code session; entries will appear under What I did.'}, 600); },

    CheckForUpdates: () => {
        const state = query('update', 'available');
        return later({state, current: '0.7.0', release: state === 'available' ? release : {version: '', notes: '', pageURL: '', zipURL: '', shaURL: ''}}, 700);
    },
    InstallUpdate: (_release: typeof release) => {
        const status = query('install', 'failed');
        return later(status === 'fallback'
            ? {status, message: "Gloamlog can't replace itself in /Applications."}
            : {status: 'failed', message: 'The download failed.'}, 1500);
    },
    OpenReleasePage: (_url: string) => later(undefined),
};
