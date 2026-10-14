# Privacy

Gloamlog is local-first. There is no account, no analytics, no telemetry and no crash reporting. This page lists what the app reads, what it writes and what can leave your Mac. It describes version 0.6.6; if you find a case where the code does something different, please [open an issue](https://github.com/dasariprashant0/gloamlog/issues).

## What leaves your Mac

Only one thing, and only when you ask for it: **Gloamlog menu > Check for Updates…**

- It sends one request to `api.github.com` for the latest release of this repository.
- If a newer version exists and you click **Download and Install**, it downloads that release's zip and its checksum file from `github.com`.
- Nothing about you or your notes is sent. The request is unauthenticated. Nothing runs in the background and the app never checks by itself.

The page editor is bundled inside the app and runs under a Content-Security-Policy that blocks all network access.

## What the app writes

- **Your notes:** one markdown file per day in your log folder (`~/Gloamlog` by default), and pasted or dropped images in `assets/` next to them.
- **Safety copies** of your pages, in `~/Library/Application Support/Gloamlog/backups/`.
- **A quick-capture journal** in `~/Library/Application Support/Gloamlog/capture/`, so a jot is not lost before it reaches a page.
- **Settings**, stored by macOS for the app.
- **System notifications** for reminders, scheduled with macOS.
- **A login item** only if you turn on "Open at login".

## What it reads if you turn Sources on

All of this is off by default. The first time you switch a source on, a sheet lists exactly what is read and you can decline.

- **Claude Code** (Settings > Sources): the session files in `~/.claude/projects` (and `$CLAUDE_CONFIG_DIR` if set). To list your projects it reads the working folder named at the start of each file. For a project you tick (or every project, if you switch on "Every session and tab"), it reads the file and keeps only the first line of the prompts you typed, the names of files Claude edited, git commit messages from its commands, pull-request links, and timestamps. Claude's replies, other tool output and file contents are not kept.
- **Git:** in folders you add, it looks for repositories up to three levels down. For the ones you tick it runs `git log` and `git config user.email` to find your own commits (hash, subject, time). It never fetches, pushes or changes a repository.
- Tokens, keys, emails and long secrets are hidden before anything is shown, and your home folder is shown as `~`. Gloamlog stores nothing it reads: it looks again when you open a day. The only thing it writes is lines into your page, the ones you choose or, if you switch on "Add commits for me", today's commits and pull requests.
- Claude Code keeps its session files for a limited time on its own schedule; Gloamlog does not copy them.

## Connect Claude Code

Clicking **Connect Claude Code** (after a confirmation) runs `claude mcp add` to register a small Python helper that ships inside the app, and adds a marked section to `~/.claude/CLAUDE.md`. Disconnect undoes both. The helper uses only the Python standard library, makes no network requests and writes small JSON files to `~/Library/Application Support/Gloamlog/inbox/`, which the app turns into lines on today's page and deletes.

The sentence in each entry is written by the model during your Claude Code session. Your Claude Code sessions already go to Anthropic under your own Claude Code setup; Gloamlog adds no network path of its own, but it does not stop Claude Code from sending what you give it. Do not ask Claude to put secrets in your log.

## Other things worth knowing

- The app registers a `gloamlog://` URL scheme. `gloamlog://refresh` only makes Gloamlog look again for today's commits and waiting Claude entries. The optional Claude Code "Stop" hook you can copy from Sources uses it.
- The bundled editor includes third-party MIT-licensed code, listed in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).
