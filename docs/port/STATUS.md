# Port status (Swift → Go + Wails v3 + React/TS)

Branch `port/wails`; `main` still ships the Swift app (v0.6.6). The checklist is [PARITY.md](PARITY.md).

## Done (committed)
- **App shell** (`desktop/main.go`): main window, quick-jot panel, Settings window, menu bar item, single instance, same bundle id and `gloamlog://` scheme; the Jot shortcut from Settings (`hotkey.go`). `build.sh` makes the `.app`, `dev.sh` runs with live reload.
- **All of app/Core except Sync** in Go (`internal/core`, 175+ tests incl. a differential test against the Swift core's own output on 5,869 cases) and **Sources** (`internal/sources`, 38 tests on fixtures).
- **Services** (`desktop/notes*.go`): overview, month, heatmap, open/save page (Swift's writable rules), skip, backups, images at `/dlasset/`, search, week, catch-up, settings (incl. folder change with fsync + tests), suggestions.
- **Screens** on the shared design system: sidebar, month calendar, day page (autosave, day switching, carry-over, skipped state, suggestions strip), search, go to date, catch up + session, skip and restore dialogs, Settings window (7 panes).
- **Mock backend**: `npm run dev:mock` in `desktop/frontend` runs the UI in a browser against `src/dev/mock*.ts`.

## In progress (wave 2, parallel; each owns its files, none edits main.go)
- Sync + conflicts (`internal/core` sync files, `notes_sync.go`, `ConflictDialog.tsx`, `SyncChip.tsx`).
- Review + export (`notes_review.go`, `Review.tsx`, `ExportDialog.tsx`).
- Capture + Claude connect + menu bar (`capture.go`, `claude.go`, `menubar.go`, `Jot.tsx`).
- Reminders + notifications (`reminders.go`, `ReminderNotice.tsx`).
- Onboarding + What's new + updater (`Onboarding.tsx`, `notes_onboarding.go`, `updater.go`).

## Next
1. Wire wave 2 into `main.go` / `App.tsx` / `DayPage.tsx` / `mock.ts`; check in the mock preview.
2. `build.sh`: copy `desktop/resources/claude/gloamlog-mcp.py` into the bundle (keeps already-connected Claude users working).
3. On a real Mac (owner): run `bash desktop/dev.sh`, then `bash desktop/build.sh`; check hotkey, menu bar, notifications, an existing notes folder, settings import.
