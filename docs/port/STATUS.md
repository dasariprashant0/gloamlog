# Port status (Swift → Go + Wails v3 + React/TS)

Branch `port/wails`; `main` still ships the Swift app (v0.6.6). The checklist is [PARITY.md](PARITY.md).

## Built (committed, tests pass, UI checked in the browser mock)
- **Go logic**: all of app/Core in `internal/core` (incl. sync), with a differential test against the Swift core's own output on 5,869 cases; Claude Code + Git sources in `internal/sources`.
- **App** (`desktop/`): main window, Jot panel, Settings window, menu bar item, single instance, same bundle id and `gloamlog://` scheme; Jot shortcut from Settings; capture (journal first, inbox every 15 s); Claude connect; reminders + notifications (incl. scheduled closed-app reminders); sync watcher; updater.
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
- Quit with unsaved edits: no "Your latest edits couldn't be saved" alert yet; no in-memory orphan pages.
- Not ported: auto-add commits (`sources.autoAdd`), VoiceOver announcements, Dock bounce, time-zone change while running, iCloud's own download-status query (polling instead, up to 3 s lag), Share… in review export, RTF in rich copy.

## Cutover (when the checks above pass)
1. `scripts/release.sh`: build with `desktop/build.sh` instead of `app/build.sh`, and bump `VERSION` there (now 0.7.0).
2. Release, then `scripts/update-tap.sh` for Homebrew; the Swift app's updater should then offer it to existing users (same bundle id and checksum file; not yet tried).
3. Remove `app/` and `editor-web/` (the editor now lives in `desktop/frontend/src/editor/`).
