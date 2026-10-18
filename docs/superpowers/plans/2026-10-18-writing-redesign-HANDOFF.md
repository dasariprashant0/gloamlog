# Handoff: writing redesign, piece 2 (2026-10-18)

For the next session. Read this, then the plan. Nothing here is a new decision: everything is in the spec and the plans.

## Where things are

- **Spec:** [specs/2026-10-18-writing-redesign-design.md](../specs/2026-10-18-writing-redesign-design.md) (approved).
- **Piece 1, core rules:** done and reviewed ([plan](2026-10-18-writing-redesign-1-core.md)). List pages (`## Log`, `#next` / `#blocked`) in counting, capture, carry-over and the review; new days are list pages. Last fix commit 2099c48.
- **Piece 2, the list page:** [plan](2026-10-18-writing-redesign-2-page.md), being run **inline, in batches with checkpoints** (the owner's choice).
  - Task 1 (Go side: `LogParse`, `LogRender`, `LogAppendLines`, bindings regenerated): **done**, commit 343e5f3.
  - Task 2 (From yesterday dismissals): **in progress**. `desktop/notes_carry_test.go` is written, uncommitted, and fails as it should (`n.CarryDismiss undefined`). Next: write `desktop/notes_carry.go` exactly as plan Task 2 Step 3 (note: `core.AtomicWrite(data []byte, dest, tempDir string)` takes `""` as tempDir), change `Open` at `desktop/notes.go:255` per Step 4, run the tests, regenerate the bindings, type-check, commit. A review hook also asked for a pruning boundary case: add one (a key dated exactly 30 days before today is kept, 31 days is removed; today is 2026-10-14 in `captureNotes`).
  - Tasks 3-9: not started. Checkpoints with the owner after Task 3 and after Task 6.
- **Decided while planning:** ⌘1 / ⌘2 / ⌘3 mark a line done / next / blocked while typing in it or in the add field (stopPropagation so the app's Today / Catch up / Review shortcuts don't also fire); elsewhere they keep their app meaning.

## How to work here (things that cost time this session)

- **Go commands:** `export GOCACHE=/tmp/claude-501/gocache GOMODCACHE=/tmp/claude-501/gomodcache GOFLAGS=-mod=mod GOTELEMETRY=off`. Filter noise with `grep -v "ld: warning\|telemetry\|upload\|^#"`.
- **Bindings:** `(cd desktop && /tmp/claude-501/gobin/wails3 generate bindings -d frontend/bindings -ts .)`. The proxy.golang.org denials it prints are harmless.
- **Type check:** `(cd desktop/frontend && node_modules/.bin/tsc --noEmit -p . && echo tsc ok)`. Never `npx` (needs the network).
- **TS tests (from Task 3):** `npm test` runs `node --test` (Node 26 runs `.ts` itself; only erasable TS in `src/log/model.ts`).
- **Browser check:** the `gloamlog-mock` preview in loadout's `.claude/launch.json` (port 9246). In mock mode the Go bindings are swapped for `src/dev/mock.ts`.
- **GateGuard hook:** the first Write/Edit of each file is blocked until you state importers/callers, API, data and the owner's instruction; state them in your reply, then retry the same call in the next turn. A "destructive" Bash command (e.g. `git rm`) needs the files, a rollback line and the instruction, then the same command again.
- **Sandbox:** can't open apps, touch `/Applications`, or reach GitHub/npm; run those in the owner's terminal tab (`run_in_terminal`). Build + install: `cd desktop && bash build.sh`, then replace `/Applications/Gloamlog.app` (move the old one to the Trash, never delete).
- **Subagents:** the auto-mode classifier sometimes refuses reviewer launches; don't retry, review in the session instead.

## Repo state

- `daily-log`, branch `port/wails`: 26 commits ahead of `origin/port/wails` (986b367), not pushed. The owner pushes. Personal git identity only; commits end with the `Co-Authored-By` line.
- A tool's notes file (`internal/core/.cto-toolkit/learning/patterns.jsonl`, harmless metadata) is in commit fe7c4d0's history, untracked and ignored since 2099c48. Ask the owner before pushing whether to rewrite it out of history.
- The installed `/Applications/Gloamlog.app` is older than pieces 1 and 2: rebuild and reinstall once piece 2 is in.
- `design-system`: nothing unpushed. `loadout`: its own unpushed commits belong to another session; leave them.

## After piece 2

Piece 3 (Standup view and Copy standup) and piece 4 (commits added at their time, replacing the suggestions strip) each need a plan. Then log the work with gloamlog `log_work`.
