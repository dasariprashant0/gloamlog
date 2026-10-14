# Changelog

All notable changes to this project. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [SemVer](https://semver.org/).

## 0.6.4
- Settings > Shortcuts > Jots go under: the heading that quick notes and Claude's entries are added under (it had no field before).

## 0.6.3
- Claude writes your log: a small Claude Code helper (`log_work`) drops an entry in an inbox and Gloamlog adds it to today's page, so the log says what you were solving and what you did instead of session names. Set up from Settings > Sources.
- Auto-add now only writes commits and pull requests. Session counts and prompt snippets are no longer added by themselves.
- Fix: a script heredoc earlier in a command was read as the commit message.

## 0.6.2
- Fix: a tall header or suggestions list made the page ask for more height than a laptop window has, so the whole window was drawn too tall with its top cut off. The header area now scrolls instead.

## 0.6.1
- Check for Updates… (Gloamlog menu): looks at the latest GitHub release and, on a click, downloads it, checks its SHA-256, swaps the app in and relaunches. The only network request in the app, and only when you click. Releases come from `scripts/release.sh`.
- Sources: "Every session and tab" mode (no per-project ticking, with a skip list), "Add them for me" (auto-writes today's lines), `gloamlog://refresh` URL for a Claude Code Stop hook, and system-generated prompts are no longer listed as work.

## [Unreleased] - 0.6.0 (alpha, feature-complete build)

### Added
- **Any date, catch up, calendar** (0.4): month calendar in the sidebar, Go to date (`⇧⌘T`, "last fri", "2 oct", "-3"), a Catch up list with "start catching up", bulk skip with undo, a log start date so backfilling never creates a wall of missed days, a Week review that works for empty weeks, and a real Settings window with a visible entry point.
- **Quick capture** (0.5): global shortcut (default `⌃⌥J`) with a floating panel over any app, a Jot field in the menu-bar popover, a write-ahead journal, safe routing into an open page, jots excluded from the logged rule by default.
- **Reviews and export** (0.6): Week, Month, Year and Range reviews; copy as Markdown, plain text and rich text; Save as PDF; Save as Markdown with only the images used; Share.
- **Reminders when the app is closed**: a rolling 14-day schedule of system notifications with Open, Snooze and Skip today actions.
- **Folder sync safety**: a watcher for iCloud Drive and Dropbox folders, a "downloading" state for undownloaded days, and conflict-copy resolution (Keep mine / Take theirs / Keep both; nothing is deleted).
- **Sources (off by default)**: "Suggested from your work" from local Claude Code sessions and Git commits, per-project opt-in, secrets redacted, read-only.
- Editor: `appendToSection` for capture and suggestions (203 editor checks).
- Crash safety: a kill-test (SIGKILL mid-save) found and fixed leftover temp files; new atomic write helper.

### Fixed
- A fresh install no longer shows every past workday as "missed".
- `⌘[` and `⌘]` now reach the Go menu inside lists, tables and code blocks instead of being swallowed by the editor.
- Version number and What's New now match the build.

### Known issues
- Not notarised: a downloaded copy needs System Settings > Privacy & Security > *Open Anyway*; building from source needs no approval.
- Quick capture, reviews, sync, closed-app reminders and Sources are compiled and unit-tested where pure, but have **not been exercised on a real Mac**: the global shortcut over full-screen apps, notifications with the app quit, real iCloud placeholders and the Claude Code transcript format are all unverified.
- macOS lets two apps register the same global shortcut, so Gloamlog cannot warn about a clash with another app (it does refuse shortcuts macOS owns).
- Search scans every file; it is fast for years of logs but not for a decade. A faster index is planned.
- The day boundary follows the current time zone; travelling across zones can show an odd "today".

## [0.3.0] - 2026-10-07 (checkpoint, not released)

### Changed
- **Renamed to Gloamlog** (was Daily Log). New bundle identifier `io.github.dasariprashant0.gloamlog`; default log folder for new installs is `~/Gloamlog`. A first-launch migration carries over settings and backups from the old name.
- **One free-form page per day** replaces the five fixed boxes. The five prompts are now an optional, editable template of headings.
- The page is a **Notion-style editor** (Milkdown Crepe, MIT) bundled into one self-contained file. It runs with a Content-Security-Policy that forbids all network access.
- **Autosave** replaces the Save button. A day counts as logged once you write `minWords` words (default 20). Carried-over tasks do not count.
- Core logic reworked and covered by 580+ automated tests; 130+ checks run the real editor in WebKit.

### Added
- Versioned backups of every page (kept outside the log folder) and "Restore previous version".
- Image support: paste or drop images; they are stored in `assets/` next to your logs.
- Carry over yesterday's to-dos and pending items; weekly review by day or by section; search with heading context.

### Removed
- The v0.1 shell/Python script version; the app replaces it.

## [0.2.0] - 2026-10-06 (internal)
- Five-section SwiftUI app with sidebar, streak and heatmap, search, weekly review, menu bar item, Settings, onboarding, strict/gentle reminders, skip days, app icon, tests and CI.

## [0.1.0] - 2026-10-06 (internal, then called Daily Log)
- First native SwiftUI app: five required sections, reminder at 4:55pm on weekdays, markdown logs.
