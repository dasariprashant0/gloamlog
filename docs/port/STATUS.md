# Port status (Swift → Go + Wails v3 + React/TS)

Paused 2026-10-18. Branch `port/wails`; `main` still ships the Swift app (v0.6.6). The checklist is [PARITY.md](PARITY.md).

## Done (committed)
- `desktop/`: Wails v3 (beta.30, pinned) app with the main window, quick-jot panel, Settings window (empty page), menu bar item, single instance, same bundle id and `gloamlog://` scheme. `build.sh` makes the `.app`; `dev.sh` runs with live reload.
- Frontend on the shared design system (`~/Projects/personal/design-system`, `@personal/ui`) with gloamlog's green accent; the Milkdown editor ported in `src/editor/` (only the host bridge, asset URL and theme changed).
- `internal/core` chunk A: storage, markdown, settings (+ one-time import from the Swift app's UserDefaults), streak, catch-up, calendar, search, carry-over, weekly review, reminder planner, update check; 113 tests.
- `desktop/notes.go`: the `Notes` service the UI calls (overview, month, heatmap, open/save page with Swift's `writable` rules, skip, backups, images at `/dlasset/`, search, week); bindings in `desktop/frontend/bindings` (`wails3 generate bindings -d frontend/bindings -ts .` from `desktop/`).

## In the working tree, NOT committed, NOT reviewed
- `internal/core` chunk B, stopped mid-way: capturejournal, captureplacement, capturetypes, daywriter, inbox, markdownhtml, reviewmodel (+ an edit to markdownbodyjots). Still missing: exportbundle, reviewformats, periodreview, reminderscheduler, and their tests. Review and test these before committing, or delete them and rerun the chunk.

## Next
1. Fix: the `Windows` service (main.go) is not picked up by the binding generator (only `Notes` is generated). Likely needs a pointer receiver / `application.NewService(&Windows{})`.
2. Finish chunk B; then chunk C as its own package (`internal/sources`: Claude Code + Git sources, redactor, digest) and Sync.
3. M1 screens (main window: sidebar, month calendar, day page with autosave + day switching, search, go to date) — `src/api.ts` helpers are in.
4. In parallel (disjoint files): Settings window panes (3.M); catch-up + session, skip dialog, restore versions (3.G, 3.H, 3.R).
5. M3: jot panel + hotkey + capture, menu bar popover, reminders/notifications, Claude connect + inbox drain (keep `Contents/Resources/claude/gloamlog-mcp.py` path), review/export, onboarding, updater.
