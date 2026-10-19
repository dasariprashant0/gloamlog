# Port status (Swift → Go + Wails v3 + React/TS)

Branch `port/wails`; `main` still ships the Swift app (v0.6.6). The checklist is [PARITY.md](PARITY.md).

## Built (committed, tests pass, UI checked in the browser mock)
- **Go logic**: all of app/Core in `internal/core` (incl. sync), with a differential test against the Swift core's own output on 5,869 cases; Claude Code + Git sources in `internal/sources`.
- **App** (`desktop/`): main window, Jot panel, Settings window, menu bar item, single instance, same bundle id and `gloamlog://` scheme; Jot shortcut from Settings; capture (journal first, inbox every 15 s); Claude connect; reminders + notifications (incl. scheduled closed-app reminders); sync watcher; updater.
- **Writing redesign, piece 1 (core)** ([spec](../superpowers/specs/2026-10-18-writing-redesign-design.md), [plan](../superpowers/plans/2026-10-18-writing-redesign-1-core.md)): list pages (`## Log`, #next / #blocked) in counting, capture, carry-over and the review; new days are list pages (an untouched old template moves once); the app now sets the jot rules on every settings change (the port never did).
- **Writing redesign, piece 2 (the list page)** ([plan](../superpowers/plans/2026-10-18-writing-redesign-2-page.md)): line editor (rows, add field, ⌘1-3 while typing, Enter / ⇧Enter / ↑↓ / Backspace / ⌘Z / Esc, time edit, image paste), From yesterday (Done / Keep / Drop), Edit whole page; saves, jots and suggestions go through the same pipeline.
- **Writing redesign, piece 3 (standup)** ([plan](../superpowers/plans/2026-10-19-writing-redesign-3-standup.md)): List | Standup switch on a list page (Done / Next / Blocked, times dropped; kept across ⌘[ / ⌘]) and Copy standup (plain text, images left out).
- **Work and Personal lists** ([spec](../superpowers/specs/2026-10-19-work-personal-split-design.md), [plan](../superpowers/plans/2026-10-19-work-personal-split.md)): new days have `## Work` and `## Personal`; Claude Code sessions, commits and `log_work` notes go to a list by folder (Settings > Page), quick notes to the chosen list; From yesterday and suggestions keep their list; standup and the review copy are Work only, Personal shown apart. `## Log` pages stay one list.
- **Writing redesign, piece 4 (commits)** ([plan](../superpowers/plans/2026-10-19-writing-redesign-4-commits.md)): with "Add commits for me" on, today's commits go onto its list page at their commit time, in their folder's list (page open, every 5 minutes, `gloamlog://refresh`), once each by short hash; a past list page has "Add this day's commits"; the suggestions strip shows on classic pages only.
- **A quieter list page** ([spec](../superpowers/specs/2026-10-19-quiet-day-page-design.md), [plan](../superpowers/plans/2026-10-19-quiet-day-page.md)): a two-row head, From yesterday as one line until Review, marks only on next / blocked lines (a done line's on hover), long lines in two rows until opened, and one add field with a Work / Personal chip.
- **Settings placeholders filled**: Sources has the project picker (tick projects; with Every session and tab, switch one off to skip it) and Connect / Disconnect Claude Code; Reminders shows Notification access (Allowed / Off / Not asked yet, Allow, Open System Settings); About has Run setup again…. Still "Coming in the next build": Third-party licences, and Storage's sync status / Download all / Resolve conflicts.
- **Screens**: sidebar, calendar, day page (safe autosave, day switching, carry-over, suggestions strip, reminder notice, still-downloading page), search, go to date, catch up + session, skip / restore / conflict dialogs, sync chip, review (week / month / year / range, copy, export), Settings (7 panes), onboarding, What's new, update dialog.
- **Run it**: `bash desktop/dev.sh` (live reload) · `bash desktop/build.sh` (makes `desktop/Gloamlog.app` + zip) · `npm run dev:mock` in `desktop/frontend` (UI in a browser, fake data).

## Not verified yet (needs a real Mac; the agent sandbox can't open app windows)
- The app window itself, the Jot panel over other apps, the global shortcut, the menu bar icon and menu.
- Notifications: permission prompt, Open System Settings landing on Gloamlog's notifications, snooze / skip buttons, a closed-app reminder arriving while the app is quit.
- An existing notes folder and the one-time settings import from the Swift app (`~/Library/Application Support/Gloamlog/settings.json` is created on first run).
- iCloud: the download request (`brctl download`) on a real iCloud folder; PDF export through the print panel (`PDF_ENABLED` in `ExportDialog.tsx` turns it off).
- Claude Code: Connect on a Mac where `claude` is installed; the helper must sit at `Gloamlog.app/Contents/Resources/claude/gloamlog-mcp.py` (build.sh copies it).

## Known gaps vs the Swift app
- Menu bar is a native menu, not the Swift popover: today's state, the streak and this week's days are text lines; no Jot field (Jot… opens the panel) and no snooze in it.
- Not ported: iCloud's own download-status query (polling instead, up to 3 s lag), Share… in review export (needs the native share sheet), RTF in rich copy (plain text and markdown are copied).

## Safer than the Swift app, on purpose
Found in the line-by-line review; each has a test.
- Zip export replaces an old file with one rename; it never deletes first (Swift deleted recursively, so a failed move lost the old file or a folder).
- Export stops when a different image already has the same name, instead of deleting it.
- The inbox's 50-per-look cap counts good entries, so junk files can't hide newer `log_work` notes.
- A settings file that can't be read (permissions, disk) stays in place and blocks saves; only one that reads but won't decode is set aside.
- Removing the CLAUDE.md block tidies only where it was; Swift collapsed blank lines across the whole file.

## Done since the gap list (2026-10-19)
- Quit with edits that couldn't be saved is held (⌘Q, the Dock, the menu bar's Quit): the page asks Try again / Copy my text / Quit anyway (`desktop/quit.go`). Not yet tried in the real app.
- Day page: sync copies and the reminder are one status line (writing redesign spec 2.1); folder problems keep the full banner; other views keep the sync banner.
- Menu bar: the streak and this week's days under today's state.
- Settings > Storage: the Sync section (folder type, Resolve conflicts, Download all). Settings > About: Third-party licences (`npm run licences` in `desktop/frontend` writes `THIRD_PARTY_LICENSES.md`; build.sh bundles it).
- A time-zone change while the app runs is followed (`timezone.go`, checked every minute); the Dock bounces when a reminder brings the app forward, unless Reduce Motion is on (`dock_darwin.go`); VoiceOver hears Logged for today, Catching up N of M, All caught up, Conflict settled and lines carried over (`src/announce.ts`).
- Known flaky test: `TestAddCommitsGoThroughAnOpenPageAsTimedJots` failed once in about 40 full runs of the desktop package (never alone in 40 runs); the cause is not found yet.
- Claude integration: `log_work` notes carry the folder Claude names (they went to Work when the helper, started before an update, sent none), and the helper reloads itself after an app update. Commits: Git also reads the repositories Claude Code worked in (0 → 89 commits found on 19 Oct), and a day's commits in one project are one line that keeps its count.

## Cutover (when the checks above pass)
1. ~~`scripts/release.sh`: build with `desktop/build.sh`~~ done 2026-10-19; `VERSION` in `desktop/build.sh` is 0.7.0. The script releases on the owner's run only.
2. Release, then `scripts/update-tap.sh` for Homebrew; the Swift app's updater should then offer it to existing users (same bundle id and checksum file; not yet tried).
3. Remove `app/` and `editor-web/` (the editor now lives in `desktop/frontend/src/editor/`).
