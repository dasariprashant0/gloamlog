# Gloamlog

**A private Mac work journal for the end of your day.** One calm, Notion-style page per day, saved as plain markdown on your Mac. No account, no cloud, no telemetry.

> **Status: alpha (v0.6), feature-complete but not yet proven in daily use.** Everything below is built and compiles, and the core logic is covered by ~5,000 automated tests. The newest features (quick capture, reviews and export, folder sync, reminders when closed, Claude Code and Git suggestions) have not been exercised on a real Mac yet, so expect rough edges. Bug reports are welcome.

## What it does

- **Write.** One page per day with headings, bullets, to-do lists, quotes, code, tables, links and images (type `/` for the block menu). It autosaves, keeps safety copies, and starts each day from an optional template of headings you control. A day counts as *logged* once you write enough words (you set the number).
- **Any date, any week.** A calendar in the sidebar opens any day; `⇧⌘T` jumps by typing `last fri`, `2 oct` or `-3`. **Catch up** lists the workdays you missed and walks you through them; skip holidays and leave in bulk, with undo. A streak that forgives backfilled days.
- **Jot from anywhere.** Press `⌃⌥J` in any app (full-screen apps included), type a thought, press Return: it lands as a time-stamped bullet under *Jots* on today's page, even while you are typing there. A write-ahead journal means a note is never lost.
- **Review and export.** Review a week, month, year or any range; copy as Markdown, plain text or rich text for a standup; save as PDF; export as Markdown with just the images you used.
- **Reminders that find you.** A strict or gentle end-of-day reminder, snooze, and system notifications that still arrive when Gloamlog is closed.
- **Suggested from your work (off by default).** Turn it on and Gloamlog reads your local Claude Code sessions and Git commits, redacts secrets, and offers an editable list of what you worked on, per project you opt in. Read-only; nothing leaves your Mac.
- **Safe with sync.** Keep your logs in iCloud Drive or Dropbox and use two Macs: Gloamlog watches the folder, never treats a not-yet-downloaded day as empty, and resolves "conflicted copy" files with *Keep mine / Take theirs / Keep both* (nothing is ever deleted).
- **Settings you can find.** A real Settings window (gear in the sidebar, `⌘,`): appearance, week start, reminders, template, words rule, storage, shortcuts, sources.

Your logs are ordinary files, one per day: `~/Gloamlog/2026-10-12.md`; images live in `~/Gloamlog/assets/`; safety copies are kept outside the log folder. Open them in any editor.

## Privacy

Everything stays on your Mac. The app makes **no network requests**: the editor is bundled and runs under a Content-Security-Policy that blocks all network access, and there is no account and no analytics. The suggestions feature is read-only and local. Third-party code in the editor is MIT-licensed and listed in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

## Build from source

Needs macOS 13 or later and the Xcode Command Line Tools (`xcode-select --install`). Full Xcode is **not** required.

```bash
git clone https://github.com/dasariprashant0/gloamlog.git
cd gloamlog
bash app/build.sh        # builds app/Gloamlog.app (ad-hoc signed)
open app/Gloamlog.app
```

The app is not notarised yet. A copy you built yourself opens normally; a downloaded copy needs System Settings > Privacy & Security > *Open Anyway* once.

```bash
bash tests/run-tests.sh                 # core logic
bash tests/run-kill-tests.sh            # crash safety: kill -9 in the middle of saves
bash app/Tools/editor-check/run.sh      # real windows and the editor in WebKit (run from a normal terminal)
```

The editor is a prebuilt bundle (`app/Resources/editor/index.html`); you only need Node if you change the editor itself (`cd editor-web && npm ci --ignore-scripts && npm run build`).

## What's next

Faster search for very large logs, a notarised release with a Homebrew install, and fixes from real-world use. See [docs/STRATEGY.md](docs/STRATEGY.md) and the plans in [docs/v1/](docs/v1/). Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md).

## Docs

[About](docs/ABOUT.md) · [How it works](docs/EXPLANATION.md) · [File structure](docs/FILE_STRUCTURE.md) · [Strategy](docs/STRATEGY.md) · [PRD](docs/v1/PRD_V1.md) · [UX flows](docs/v1/UX_FLOWS.md) · [Design](docs/v1/DESIGN_V4.md) · [Architecture](docs/v1/ARCHITECTURE.md) · [Editor contract](docs/EDITOR_CONTRACT.md) · [Changelog](CHANGELOG.md)

## License

[MIT](LICENSE). Not affiliated with Anthropic.
