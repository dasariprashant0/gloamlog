# Port status (Swift → Go + Wails v3 + React/TS)

Branch `port/wails`; `main` still ships the Swift app (v0.6.6). The checklist is [PARITY.md](PARITY.md).

## Built (committed, tests pass, UI checked in the browser mock)
- **Go logic**: all of app/Core in `internal/core` (incl. sync), with a differential test against the Swift core's own output on 5,869 cases; Claude Code + Git sources in `internal/sources`.
- **App** (`desktop/`): main window, Jot panel, Settings window, menu bar item, single instance, same bundle id and `gloamlog://` scheme; Jot shortcut from Settings; capture (journal first, inbox every 15 s); Claude connect; reminders + notifications (incl. scheduled closed-app reminders); sync watcher; updater.
- **Writing redesign, piece 1 (core)** ([spec](../superpowers/specs/2026-10-18-writing-redesign-design.md), [plan](../superpowers/plans/2026-10-18-writing-redesign-1-core.md)): list pages (`## Log`, #next / #blocked) in counting, capture, carry-over and the review; new days are list pages (an untouched old template moves once); the app now sets the jot rules on every settings change (the port never did). Until piece 2, list pages open in the classic editor and the old carry-over card.
- **Screens**: sidebar, calendar, day page (safe autosave, day switching, carry-over, suggestions strip, reminder notice, still-downloading page), search, go to date, catch up + session, skip / restore / conflict dialogs, sync chip, review (week / month / year / range, copy, export), Settings (7 panes), onboarding, What's new, update dialog.
- **Run it**: `bash desktop/dev.sh` (live reload) · `bash desktop/build.sh` (makes `desktop/Gloamlog.app` + zip) · `npm run dev:mock` in `desktop/frontend` (UI in a browser, fake data).

## Not verified yet (needs a real Mac; the agent sandbox can't open app windows)
- The app window itself, the Jot panel over other apps, the global shortcut, the menu bar icon and menu.
- Notifications: permission prompt, snooze / skip buttons, a closed-app reminder arriving while the app is quit.
- An existing notes folder and the one-time settings import from the Swift app (`~/Library/Application Support/Gloamlog/settings.json` is created on first run).
- iCloud: the download request (`brctl download`) on a real iCloud folder; PDF export through the print panel (`PDF_ENABLED` in `ExportDialog.tsx` turns it off).
- Claude Code: Connect on a Mac where `claude` is installed; the helper must sit at `Gloamlog.app/Contents/Resources/claude/gloamlog-mcp.py` (build.sh copies it).

## Known gaps vs the Swift app
- Menu bar is a native menu, not the Swift popover (no Jot field, streak, week strip, snooze in it).
- Quit with unsaved edits: no "Your latest edits couldn't be saved" alert yet (a failed save is kept in memory and retried, with a banner, while the app runs).
- Not ported: auto-add commits (`sources.autoAdd`), VoiceOver announcements, Dock bounce, time-zone change while running, iCloud's own download-status query (polling instead, up to 3 s lag), Share… in review export, RTF in rich copy.

## Safer than the Swift app, on purpose
Found in the line-by-line review; each has a test.
- Zip export replaces an old file with one rename; it never deletes first (Swift deleted recursively, so a failed move lost the old file or a folder).
- Export stops when a different image already has the same name, instead of deleting it.
- The inbox's 50-per-look cap counts good entries, so junk files can't hide newer `log_work` notes.
- A settings file that can't be read (permissions, disk) stays in place and blocks saves; only one that reads but won't decode is set aside.
- Removing the CLAUDE.md block tidies only where it was; Swift collapsed blank lines across the whole file.

## Cutover (when the checks above pass)
1. `scripts/release.sh`: build with `desktop/build.sh` instead of `app/build.sh`, and bump `VERSION` there (now 0.7.0).
2. Release, then `scripts/update-tap.sh` for Homebrew; the Swift app's updater should then offer it to existing users (same bundle id and checksum file; not yet tried).
3. Remove `app/` and `editor-web/` (the editor now lives in `desktop/frontend/src/editor/`).
