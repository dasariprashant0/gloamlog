// What "Connect Claude Code" changes, shown before it runs: in the setup sheet's Claude Code step and in Settings > Sources.
// The steps are desktop/claude.go's ConnectClaude. They carry their numbers in the text: the design system draws no list markers.
export function ClaudeSteps() {
    return <>
        Gloamlog will:
        <ol>
            <li>1. Register its small helper with Claude Code for all your projects (it runs claude mcp add).</li>
            <li>2. Add a short section to ~/.claude/CLAUDE.md that tells Claude when to write a log entry.</li>
            <li>3. Put Claude's entries in your day's list (on a Daily review page, under "What I did").</li>
        </ol>
        Nothing is sent anywhere, and Disconnect undoes steps 1 and 2. Start a new Claude Code session afterwards.
    </>;
}
