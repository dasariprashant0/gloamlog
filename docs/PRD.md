> **Superseded.** This document describes the earlier five-box version (v0.1 and v0.2). The current plans are in [docs/v1/](v1/) (PRD_V1, UX_FLOWS, DESIGN_V4, TRD, ARCHITECTURE).

# PRD: Gloamlog v0.2

**Status**: Draft for build. v0.1 compiled, never launched or UI-tested.
**Author**: Prashant Dasari  **Version**: 0.2  **Last updated**: 2026-10-06
**Platform**: macOS 13+, SwiftUI, built with Command Line Tools only (no Xcode, so no Swift macros: use `ObservableObject` / `@StateObject`, not `@State` / `@Observable`).
**License**: MIT, open source.

---

## 1. Problem

Most working days end with no clear answer to: what did I do, what is stuck, what do I pick up tomorrow? Work is fragmented across many small tasks and tools, so context evaporates overnight and the next morning starts with re-orientation. Optional journalling does not stick. A habit needs a prompt, a small enforced structure, and a reason to come back.

v0.1 proved the shape (five required sections, a nag at a fixed time, markdown on disk) but is a brittle tool: it loses text if the window closes, nags on days off, has no settings beyond a time, and gives nothing back after you write. v0.2 turns it into something people can install, trust, and keep using, and into a repo others can contribute to.

**Evidence**: n=1 (author). No usage data; no telemetry by design. Everything here is a hypothesis, tested by self-use and by feedback from people who install it.

## 2. Goals

1. **Habit**: prompt at a chosen time, in a way the user controls (Strict or Gentle), and make writing cheap (draft autosave, carry-over, templates of your own sections).
2. **Reward**: give back value for writing (streak, heatmap, yesterday card, search, weekly review).
3. **Trust**: local-only, no network, no telemetry, plain markdown the user can move to iCloud/Dropbox or grep.
4. **Open-source readiness**: a stranger can install in under 2 minutes, build it from source, run tests, and send a PR.

## 3. Non-Goals (v0.2)

- Accounts, cloud sync service, any network call, telemetry, crash reporting.
- Notion/Obsidian export, AI summaries, localisation, iOS companion (see `docs/ROADMAP.md`).
- Notarisation and auto-update (documented as a known limitation).
- Team features, shared logs, manager visibility.
- Rich text or attachments. Sections are plain markdown text.

## 4. Scope and MoSCoW

| # | Feature | MoSCoW |
|---|---------|--------|
| F1 | Draft autosave | Must |
| F2 | Skip day (holiday/leave), streak-neutral | Must |
| F3 | Yesterday card + carry-over | Must |
| F4 | Streak + 12-week activity heatmap | Should |
| F5 | Menu bar item with status | Must |
| F6 | Native notification + window nag, Strict/Gentle | Must |
| F7 | Search across logs | Should |
| F8 | Weekly review + "Copy week as markdown" | Should |
| F9 | Settings (reminder time, weekdays, strictness, launch at login, storage folder, snooze minutes) | Must |
| F10 | Customisable sections | Should |
| F11 | Keyboard shortcuts | Should |
| F12 | First-run onboarding | Must |
| F13 | App icon | Should |
| F14 | Unit tests | Must |
| F15 | CI | Must |
| F16 | Tagged-release workflow | Must |
| F17 | OSS repo hygiene (LICENSE MIT, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, issue/PR templates) | Must |
| F18 | Notarisation, auto-update | Won't (v0.2) |
| F19 | Export, AI summary, localisation, iOS | Won't (v0.2), see roadmap |

Constraint: "Must" items are the release gate. If time runs short, cut Should items in this order: F13, F8, F4, F7, F10, F11.

## 5. Why this makes people keep using it

Reminders alone get disabled. Retention comes from loops where each day's effort pays back:

- **Low friction to start**: the Yesterday card carries over yesterday's "To do next" and "Pending", so the page is never blank. Writing becomes editing.
- **Nothing is lost**: autosave removes the fear of closing the window mid-thought.
- **Visible progress**: the streak and the 12-week heatmap give a small, honest sense of momentum. Skipped days are streak-neutral, so leave and holidays never punish the user (a broken streak is the usual reason habit apps get abandoned).
- **Compounding value**: search turns months of logs into a personal memory ("when did I last touch X?"). The weekly review turns five entries into a summary you can paste into a standup, a report, or a note.
- **Respect for the user**: Gentle mode, snooze, weekday toggles, and skip day keep the nag from becoming the reason to uninstall. The nag is a tool the user tunes, not a trap.
- **Ownership**: plain markdown in a folder of the user's choice. No lock-in means no reason to distrust it.

## 6. User stories and acceptance criteria

### F1 Draft autosave
*As a user, I want my half-written log kept so closing the window or quitting never loses text.*
- [ ] Given I type in any section, when I pause for 2 seconds (debounced) or switch day/quit, then a draft is written to disk.
- [ ] Given a draft exists and I reopen the day, then the fields are restored and a "Draft" label is shown.
- [ ] Given I press Save with all required sections filled, then the final log is written and the draft is removed.
- [ ] A draft never counts as logged: streak, heatmap, and the nag treat the day as unlogged.
- [ ] Drafts are stored in the storage folder in a hidden location (e.g. `.drafts/YYYY-MM-DD.md`) and are never read by search or weekly review.
- [ ] A failed draft write does not block typing; the error is shown inline, non-modal.

### F2 Skip day
*As a user on holiday or leave, I want to mark a day skipped so the app stops nagging and my streak survives.*
- [ ] Given a day is unlogged, when I choose "Skip today" (button, menu bar, or shortcut), then the day is recorded as skipped and no nag or notification fires for it.
- [ ] A skipped day does not break the streak and does not add to it.
- [ ] A skipped day shows distinctly in the sidebar and heatmap (not as missed, not as logged).
- [ ] I can skip a future date or a past unlogged date, and can undo a skip.
- [ ] If I save a real log on a skipped day, the skip is cleared and the day counts as logged.
- [ ] Skips are stored as a marker file in the storage folder so they travel with the logs.

### F3 Yesterday card + carry-over
*As a user, I want to see what I planned and left open yesterday so I can start from it.*
- [ ] Given a previous logged day exists (most recent before today, skipping weekends/skipped days), when I open Today, then a card shows its "To do next" and "Pending / blocked" content.
- [ ] Given the card, when I press "Carry over", then that content is inserted into today's matching sections without overwriting text I already typed (appended, with a clear separator).
- [ ] Carry-over is explicit; nothing is pre-filled silently.
- [ ] If there is no previous log, the card is hidden.
- [ ] If sections are customised and the named sections no longer exist, the card shows only what maps; otherwise it hides.

### F4 Streak + heatmap
*As a user, I want to see my consistency so I am motivated not to break it.*
- [ ] Streak = consecutive scheduled workdays logged; skipped days and non-workdays are neutral (neither extend nor break).
- [ ] Today unlogged before the reminder time does not break the streak; it breaks only once the day has passed unlogged.
- [ ] A 12-week heatmap shows each day as logged / skipped / missed / not-scheduled / future, with a legend and a tooltip date.
- [ ] Current and longest streak are displayed. All values are computed from files on disk; no separate state to corrupt.
- [ ] Heatmap is readable in light and dark mode and has accessibility labels.

### F5 Menu bar item
*As a user, I want to see at a glance whether today is done without opening the app.*
- [ ] A menu bar icon shows state: done, pending, skipped (distinct symbols, not colour alone).
- [ ] Menu lists: status line, streak, "Open Gloamlog", "Skip today", "Snooze", "Open folder", "Settings", "Quit".
- [ ] Menu bar item can be hidden in Settings; the app still runs and nags.
- [ ] Clicking "Open Gloamlog" brings the main window forward.

### F6 Native notification + window nag, Strict/Gentle
*As a user, I choose how hard the app pushes me.*
- [ ] At reminder time on a scheduled, unlogged, unskipped day, a native notification is posted (permission requested during onboarding; app degrades to window-only if denied).
- [ ] **Strict**: window comes to front at reminder time and re-opens (without re-stealing focus if already visible) on a 60-second check until the day is logged or skipped. Matches v0.1 behaviour.
- [ ] **Gentle**: notification only, plus the menu bar state; the window is never forced forward. Notification repeats at snooze interval, capped at 3 repeats per day.
- [ ] "Snooze" (notification action, menu bar, or window) delays the next nag by the configured snooze minutes.
- [ ] No nag fires once the day is logged or skipped, or on non-scheduled days.
- [ ] App launched after the reminder time with an unlogged day nags once on launch.

### F7 Search
*As a user, I want to find what I wrote months ago.*
- [ ] A search field (⌘F) searches all log files in the storage folder, case- and diacritic-insensitive.
- [ ] Results list date, section, and a snippet with the match highlighted; selecting a result opens that day.
- [ ] Results appear in under 300 ms for 1,000 log files on a typical Mac.
- [ ] Empty query clears results; no results shows a clear empty state.
- [ ] Search is local and reads only the storage folder; drafts are excluded.

### F8 Weekly review + Copy week as markdown
*As a user, I want to see my week in one page and paste it elsewhere.*
- [ ] Weekly review shows Monday to Sunday of the selected week with each logged day's sections grouped by section type (all "Finished", then all "Started", and so on) or by day (toggle).
- [ ] Shows counts: days logged, skipped, missed.
- [ ] "Copy week as markdown" puts a well-formed markdown document on the clipboard (week heading, one heading per day or section, no app chrome) and confirms with a transient "Copied".
- [ ] Weeks with no logs show an empty state, not a blank page.
- [ ] Previous/next week navigation; defaults to the current week.

### F9 Settings
*As a user, I want the app to fit my schedule and storage preferences.*
- [ ] Settings window opens with ⌘, and from the menu bar.
- [ ] Reminder time (time picker); weekday toggles (any subset of 7 days); strictness (Strict/Gentle); launch at login (on/off, via `SMAppService`, reflecting real registration status); storage folder; snooze minutes (e.g. 5, 10, 15, 30, 60).
- [ ] All settings persist across relaunch and take effect without restart.
- [ ] Storage folder picker (`NSOpenPanel`): changing it does not move or delete existing logs; user is asked whether to move existing files, and the default stays `~/Gloamlog`. Picking iCloud Drive or Dropbox folders works.
- [ ] If the storage folder becomes unavailable (unmounted, deleted), the app shows a clear error and does not silently write elsewhere.
- [ ] Launch at login defaults to off and is offered during onboarding (v0.1 registered silently; v0.2 asks).

### F10 Customisable sections
*As a user, I want sections that match my work, not the defaults.*
- [ ] Default sections remain: What I did, Finished, Started, Pending / blocked, To do next.
- [ ] In Settings, I can add, rename, reorder, and remove sections; at least 1 section must remain.
- [ ] Each section can be marked required or optional; Save is enabled when all required sections are non-empty.
- [ ] Section list is stored in a small config file in the storage folder so it syncs with the logs.
- [ ] Existing logs with sections that are no longer configured are still displayed (read-only block labelled "legacy section") and never deleted.
- [ ] Renaming a section does not lose old content: file parsing matches by heading text, and rename offers to rewrite existing files or leave them as legacy.
- [ ] The `## ` prefix inside user text is escaped on save so it cannot split a section (fixes a known v0.1 limitation).

### F11 Keyboard shortcuts
- [ ] ⌘S save; ⌘F search; ⌘, settings; ⌘T jump to today; ⌘[ / ⌘] previous/next day; ⌘⇧C copy week as markdown (in weekly review); ⌘⇧K skip today; ⌘1..⌘9 jump to section n.
- [ ] Shortcuts appear in menu items and in a "Keyboard shortcuts" help list.
- [ ] No shortcut conflicts with system or text editing defaults.

### F12 First-run onboarding
*As a new user, I want to be set up in under a minute.*
- [ ] On first launch a short flow (max 4 steps) covers: what the app does, reminder time and weekdays, strictness, notification permission and launch at login (both opt-in), storage folder.
- [ ] Every step has sensible defaults and a Skip; completing with only "Next" gives a working setup.
- [ ] Onboarding does not appear again after completion; reachable from Settings ("Run setup again").
- [ ] Explains the unsigned/Gatekeeper reality honestly if the app detects it is quarantined (optional).

### F13 App icon
- [ ] A 1024x1024 source plus a generated `.icns` with all required sizes, wired into `build.sh` and `Info.plist` (`CFBundleIconFile`).
- [ ] Looks correct in Dock, menu bar template variant (monochrome, separate asset), Finder, and Login Items.
- [ ] Icon source is committed under an open licence compatible with MIT.

### F14 Unit tests
- [ ] Pure logic is separated from UI so it is testable without Xcode: markdown parse/serialise (round trip, `## ` escaping), logged/skipped/draft state, streak computation, heatmap data, carry-over merge, weekly aggregation, search, nag decision (due / snoozed / strict / gentle / non-workday), settings defaults.
- [ ] Tests run with Command Line Tools only (no XCTest dependency on Xcode; a minimal assertion runner via `swiftc` or SwiftPM `swift test` if it works under CLT, to be confirmed, see open questions).
- [ ] `./test.sh` runs locally and exits non-zero on failure.
- [ ] Date logic takes an injected clock and calendar so tests are deterministic, including DST and week boundaries.

### F15 CI
- [ ] GitHub Actions workflow on push and PR to `main`: build with `swiftc` on a macOS runner, run tests, fail on warnings in the build script.
- [ ] Status badge in README.
- [ ] CI must not need secrets.
- [ ] CI is green on `main` before any release tag.

### F16 Tagged-release workflow
- [ ] Pushing a tag `vX.Y.Z` builds the app, runs tests, produces `Gloamlog.zip` and a SHA-256 checksum, and creates a GitHub Release with notes pulled from `CHANGELOG.md`.
- [ ] Release is ad-hoc signed and clearly labelled "not notarised" with install steps.
- [ ] Build is reproducible from a clean clone using only documented steps.
- [ ] Targets both arm64 and x86_64 (universal binary), or documents the single architecture.

### F17 Open-source repo hygiene
- [ ] `LICENSE` (MIT), `CONTRIBUTING.md` (build, test, PR flow, scope rules: no network, no telemetry), `CODE_OF_CONDUCT.md` (Contributor Covenant), `SECURITY.md` (private reporting route, supported versions, threat model: local files only), `.github/ISSUE_TEMPLATE/` (bug, feature request), `.github/PULL_REQUEST_TEMPLATE.md`.
- [ ] README updated: install, build, test, privacy promise, Gatekeeper steps, screenshots, doc links, badge, contribution pointer.
- [ ] `CHANGELOG.md` updated for 0.2.0 following Keep a Changelog.
- [ ] No personal paths, names of employers, or proprietary content in the repo, history, or sample data.

## 7. Cross-cutting requirements

- **Privacy**: no network code at all. No analytics, no crash reporter, no update checker. A grep for `URLSession` / network entitlements in CI is an acceptable guard.
- **Data format**: one file per day, `YYYY-MM-DD.md`, `# date` heading then one `## ` heading per section. v0.1 files remain readable unchanged (backward compatible).
- **Data safety**: writes are atomic. The app never deletes a log file. Folder change never deletes data.
- **Accessibility**: VoiceOver labels on heatmap and status icons; state never conveyed by colour alone; respects dark mode and Reduce Motion.
- **Performance**: cold launch under 1 second to a usable window; typing never blocked by autosave.
- **Build**: `cd app && ./build.sh` still works with Command Line Tools only; no macros, no third-party dependencies.

## 8. Success criteria

### OSS readiness (release gate for v0.2.0)
| Criterion | Target |
|-----------|--------|
| Install time for a new user (download zip, unzip, move to Applications, first launch including Gatekeeper step, finish onboarding) | Under 2 minutes, timed with at least 3 people who have not seen it |
| Build from clean clone to running app | Under 5 minutes using README only |
| Tests | All green in CI on `main` and on the release tag; core logic (F1-F8, nag decision) covered |
| Docs complete | README, PRD, ROADMAP, CHANGELOG, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, templates all present and linked |
| Release workflow | One dry-run tag produces a working zip and checksum |
| Privacy | Zero network calls verified by code search and Little Snitch/`lsof` spot check |
| Manual UI smoke test | Checklist (save, relaunch, draft restore, skip, nag Strict and Gentle, menu bar, settings persistence, folder change, onboarding) executed on macOS 13 and the latest macOS |

### Product (self-reported, no telemetry)
| Metric | Target | Window |
|--------|--------|--------|
| Scheduled weekdays with a saved or skipped log | >= 85% | First 4 weeks |
| Logs saved within 60 minutes of reminder | >= 70% | First 4 weeks |
| Still in use unprompted | Yes | 8 weeks |
| Installers who got it running without help | >= 75% | At sharing |
| People who still use it after 2 weeks | >= 2 of those who try it | 4 weeks after release |
| Unprompted GitHub issues or PRs from outside the author | >= 1 | 8 weeks after release |

Kill signal: if the author keeps typing junk ("n/a") to unlock Save, or keeps quitting the app to avoid the nag, the required-section and Strict rules are producing noise. Make more sections optional and default to Gentle.

## 9. Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **Unverified UI**: v0.1 was compiled but never launched; v0.2 adds much more UI (menu bar, settings, heatmap, onboarding). Layout, focus handling, notifications, login item behaviour are all untested. | High | High | Manual smoke-test checklist before tagging; keep logic in testable pure functions; ship early builds to 2-3 testers before 0.2.0. |
| **No notarisation**: app is ad-hoc signed only. Gatekeeper warns or blocks; users may distrust or give up. | High | Medium | Clear README steps (right-click > Open, `xattr` command); publish SHA-256; keep source buildable; notarisation on roadmap (needs paid Apple Developer account). |
| **Gatekeeper friction on newer macOS**: recent releases tightened the right-click > Open path (may require System Settings > Privacy & Security > "Open Anyway"). | High | Medium | Verify steps on the newest macOS and document both paths with screenshots. |
| Notifications denied or unsupported for an unsigned app; Strict nag is then the only signal. | Medium | Medium | Degrade to window/menu bar; do not depend on notification permission. |
| CLT-only toolchain: XCTest and SwiftPM may be unavailable or behave differently without Xcode; CI runner toolchain differs from local. | Medium | Medium | Spike on `swift test` under CLT first; fall back to a plain assertion runner compiled by `swiftc`. |
| Single-file code (`Gloamlog.swift`) becomes unmanageable with v0.2 scope and hard to test. | High | Medium | Split into a few files (logic vs UI) as part of F14; update `docs/FILE_STRUCTURE.md`. |
| Storage in iCloud/Dropbox: partial sync, conflicted copies, evicted files, or slow reads. | Medium | Medium | Atomic writes; tolerate conflict-copy filenames by ignoring non `YYYY-MM-DD.md` files; document that two Macs editing the same day can conflict. |
| Customisable sections make parsing ambiguous (rename, duplicate titles, `## ` in text). | Medium | Medium | Escape on save; unique titles enforced; legacy-section display; round-trip tests. |
| Strict nag drives users to quit the app, silencing it. | Medium | Medium | Gentle mode, snooze, weekday toggles, skip day; watch the kill signal. |
| Login-item registration needs user approval in System Settings on some macOS versions. | Medium | Low | Opt-in in onboarding; Settings shows real status with a link to Login Items. |
| Mac asleep or app not running at reminder time. | Medium | Medium | Known limit; nag fires on launch/wake if past due. |
| Scope: this is a large release for a spare-time project. | High | Medium | MoSCoW cut order in section 4; Musts only are the tag gate. |

## 10. Assumptions

- The Mac is awake and the app is running at reminder time.
- Local time and calendar are correct; time-zone changes shift the reminder with the system clock.
- Users are comfortable with one Gatekeeper workaround step.
- The chosen storage folder is writable and not blocked by macOS privacy prompts (a first-use permission prompt for Documents/iCloud folders is acceptable).

## 11. Open questions

- [ ] Does `swift test` / XCTest work under Command Line Tools only on CI and locally, or is a custom assertion runner needed? (Spike before F14. Owner: author.)
- [ ] Notification APIs for an ad-hoc signed app: does `UNUserNotificationCenter` authorisation work reliably, or is `NSUserNotification`-style fallback needed?
- [ ] Skip markers: separate marker files (`YYYY-MM-DD.skip`) or a single `skipped.json`? Marker files merge better under sync conflicts; confirm.
- [ ] Should a missed past day be backfillable under the all-required rule, or may past days be saved with fewer sections (optional sections)?
- [ ] Gentle mode: is "notification plus up to 3 repeats" the right default, or only one?
- [ ] Heatmap and streak when the weekday set changes mid-history: use the current setting for all history, or store the schedule with each day?
- [ ] Universal binary from `swiftc` on a CI runner: is cross-arch build reliable under CLT only?
- [ ] Is the shell/launchd script variant (`daily-log.sh`, `daily-log.py`) kept in the repo, moved to `extras/`, or removed to avoid confusing newcomers?
- [ ] Icon: commission, generate, or draw by hand; and under what licence?
- [ ] Is paying for an Apple Developer account justified once there are real users? (Decision moved to roadmap trigger.)

## 12. Release plan

| Step | Gate |
|------|------|
| 1. Spikes: CLT test runner, notification auth, universal build | Answers recorded in open questions |
| 2. Split code into logic and UI; write tests for existing behaviour | Tests green locally |
| 3. Build Musts: F1, F2, F3, F5, F6, F9, F12 | Manual smoke test passes |
| 4. Build Shoulds as time allows (cut order in section 4) | Smoke test updated |
| 5. Repo hygiene, CI, release workflow, README, CHANGELOG | CI green; dry-run tag produces release |
| 6. Share with 2-3 testers; time the install | Install under 2 minutes; blocking bugs fixed |
| 7. Tag `v0.2.0` | All OSS-readiness criteria met |

Rollback: users keep v0.1 zip from the releases page; v0.2 does not change the log file format incompatibly, so downgrading loses only settings, drafts, and skip markers.
