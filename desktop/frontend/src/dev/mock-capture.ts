// Mock additions for quick capture and the Claude Code connection (merged into Notes in mock.ts as `...captureMock`).
// Same names and shapes as desktop/capture.go and desktop/claude.go. Handy triggers: a jot containing "offline" is kept
// ("folder unavailable"), one containing "fail" throws, so every state of the Jot panel can be tried in a browser.
const later = <T,>(v: T) => new Promise<T>(r => setTimeout(() => r(v), 60));

type ClaudeState = 'connected' | 'moved' | 'notConnected' | 'noClaude';
let claude: ClaudeState = 'notConnected';
let waiting = 0;
const MESSAGES: Record<ClaudeState, string> = {
    connected: 'Connected', moved: 'Needs reconnecting', notConnected: 'Not connected', noClaude: 'Claude Code not found',
};
const SCRIPT = '/Applications/Gloamlog.app/Contents/Resources/claude/gloamlog-mcp.py';
const copy: Record<string, string> = {
    setup: `claude mcp add --scope user gloamlog -- python3 "${SCRIPT}"`,
    instructions: '## Gloamlog work log\nWhen a meaningful piece of work is finished in a session (a problem solved, a deliverable done, a decision made, or I say I\'m done), call the gloamlog `log_work` tool once with 1-3 plain sentences: the problem I was solving, what was done, and what is still open. Not after every step. No secrets, customer data or code.',
    hook: '"hooks": { "Stop": [ { "hooks": [ { "type": "command", "command": "pgrep -x Gloamlog >/dev/null && open -g gloamlog://refresh || true" } ] } ] }',
};

export const captureMock = {
    Jot: async (text: string) => {
        const note = text.trim();
        const day = new Date().toLocaleDateString('en-CA'); // yyyy-mm-dd, local
        if (note === '' || note === '[]') return later({outcome: 'empty', message: '', waiting, day, heading: 'Jots'});
        if (note.includes('fail')) { await later(0); throw new Error('mock: the write failed'); }
        if (note.includes('offline')) {
            waiting += 1;
            return later({outcome: 'kept', message: 'Kept on this Mac. It will be added when your log folder is back.', waiting, day, heading: 'Jots'});
        }
        return later({outcome: 'added', message: "Added to today's page under Jots.", waiting, day, heading: 'Jots'});
    },
    JotsWaiting: () => later(waiting),
    RetryJots: () => { waiting = 0; return later(0); },
    EditorOpened: (_day: string) => later(undefined),
    EditorClosed: (_day: string) => later(undefined),
    JotPlaced: (_id: string, _result: string, _markdown: string) => later(undefined),
    EditorSaved: (_day: string, _body: string) => later(undefined),

    ClaudeStatus: () => later({state: claude, message: MESSAGES[claude]}),
    ConnectClaude: () => {
        claude = 'connected';
        return later({ok: true, message: 'Connected. Start a new Claude Code session; entries will appear under What I did.', state: claude});
    },
    DisconnectClaude: () => {
        claude = 'notConnected';
        return later({ok: true, message: 'Disconnected.', state: claude});
    },
    ClaudeCopyText: (kind: string) => later(copy[kind] ?? ''),
};
