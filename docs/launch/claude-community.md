<!-- DRAFT for the owner to review. Do not post as is.
Intended for r/ClaudeAI, r/ClaudeCode or the Claude community Discord/forum. Different angle from the r/macapps post: the workflow and exactly what the helper does.
Before posting: read each community's rules and pinned posts, use the required flair, ask the moderators if self-promotion needs permission, and disclose that it is your project (done below). Do not post the same text in several places. -->

**Title:** I wrote a small MCP helper so Claude Code adds a work-log entry when it finishes a task (open source, local)

**Body:**

I'm the author of Gloamlog, a free MIT-licensed macOS work journal. One page per day, saved as plain markdown. I'm sharing the Claude Code part because it might be useful on its own.

How it works:

- A single Python file (standard library only, no network) is an MCP server with one tool, `log_work(summary)`. The tool description tells Claude to call it once when a meaningful piece of work is done, with 1 to 3 plain sentences: the problem, what was done, what is still open. No secrets or code.
- It does not talk to the app directly. It writes a small JSON file to `~/Library/Application Support/Gloamlog/inbox`. The Gloamlog app picks it up (about every 15 seconds while running, or at launch), adds it to today's page and deletes the file.
- Setup is `claude mcp add --scope user gloamlog -- python3 /Applications/Gloamlog.app/Contents/Resources/claude/gloamlog-mcp.py`, plus a short section in `~/.claude/CLAUDE.md` that tells Claude when to call it. The app's "Connect Claude Code" button (Settings > Sources) does both after showing you what it will change, and Disconnect undoes both. The helper file is about 80 lines, so it is easy to read or reuse.

The log fills in with what you were solving and what is left, rather than a list of session names. Optionally, Sources can also read your Claude Code session files (first line of prompts, edited file names, commit messages, PR links) and Git commits to suggest lines, per project, with secrets redacted, read-only and only after you turn it on and accept a consent sheet.

Privacy: Gloamlog makes no network requests except the "Check for Updates…" menu item when you click it. The sentence Claude writes comes from your Claude Code session, which goes to Anthropic as it normally does; the helper adds no path of its own.

What's rough:

- Alpha (0.6.5). The entries depend on Claude following the instruction, so some sessions will write none and some will write too much. I'd like to hear how it behaves for you.
- The session-file reading depends on a Claude Code file format that is not documented and could change.
- Not notarised (no paid Apple account): use the installer one-liner in the README, or build from source. macOS 13+; the release is universal but only run on Apple silicon so far.

Repo: https://github.com/dasariprashant0/gloamlog (the helper is `app/Resources/claude/gloamlog-mcp.py`)
