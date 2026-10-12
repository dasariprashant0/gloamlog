> **Superseded.** This document describes the earlier five-box version (v0.1 and v0.2). The current plans are in [docs/v1/](v1/) (PRD_V1, UX_FLOWS, DESIGN_V4, TRD, ARCHITECTURE).

# Gloamlog: Technical Requirements

Status: describes the code as it exists in `app/Gloamlog.swift`, `app/build.sh`, `daily-log.sh`, `daily-log.py`. Nothing has been built or run yet; every claim below is from reading source, not from execution.

## 1. Platform and toolchain

- Target: macOS 13+ (`LSMinimumSystemVersion` 13.0, `-target <arch>-apple-macos13.0`).
- Architecture: whatever `uname -m` returns on the build machine (arm64 or x86_64). Single-arch binary, no universal build.
- Toolchain: `swiftc` from Xcode Command Line Tools only. No Xcode project, no SwiftPM, no dependencies.
- Frameworks: SwiftUI, ServiceManagement (`SMAppService`), AppKit (implicit).
- Flags: `-O -parse-as-library -swift-version 5`. `-parse-as-library` is required because the entry point is `@main`.
- Single source file: `app/Gloamlog.swift` (~185 lines).

## 2. Build, sign, package (`app/build.sh`)

Run from `app/`: `./build.sh` (`set -e`).

1. `rm -rf "Gloamlog.app" Gloamlog.zip`; create `Gloamlog.app/Contents/MacOS`.
2. `swiftc` compiles to `Contents/MacOS/Gloamlog`.
3. `Info.plist` written by heredoc: bundle id `io.github.dasariprashant0.gloamlog`, name/display name "Gloamlog", executable `Gloamlog`, `APPL`, version `1.0`, min OS 13.0, `NSPrincipalClass` NSApplication, `NSHighResolutionCapable`. No icon, no `LSUIElement` (app shows in Dock).
4. `codesign --force --sign - "$APP"` (ad-hoc).
5. `ditto -c -k --keepParent` produces `Gloamlog.zip`.

Outputs: `app/Gloamlog.app`, `app/Gloamlog.zip`.

## 3. Data format: `~/Gloamlog/YYYY-MM-DD.md`

One file per day. Directory is `NSHomeDirectory()/daily-log`, created on save. Date is formatted with `en_US_POSIX` locale, local timezone.

Exact layout written by `Store.save`:

```
# 2026-10-06

## 📝 What I did
<text>

## ✅ Finished
<text>

## 🚀 Started
<text>

## ⏳ Pending / blocked
<text>

## 📌 To do next
<text>

```

Keys and headings (must match exactly, emoji included): `did` "📝 What I did", `finished` "✅ Finished", `started` "🚀 Started", `pending` "⏳ Pending / blocked", `todo` "📌 To do next".

Parsing rules (`Store.load`):
- File read as UTF-8, split on `\n`.
- A line with prefix `## ` starts a new section; the remainder must equal a known heading, else the section is discarded (`cur = nil`, content ignored).
- All other lines (including the `# date` title and anything before the first `##`) are appended to the current buffer, or dropped if no current section.
- Section text is trimmed of leading/trailing whitespace and newlines on flush; internal blank lines are preserved.
- Missing file or unreadable file returns an empty dictionary.
- Saved values are trimmed on write. Writes are atomic (`write(atomically: true)`), overwriting the day's file.

"Logged" = all five sections non-empty after trimming (`Store.isLogged`). Save button is disabled until all five are filled.

Known limitation: a user line that begins with `## ` splits the section. The text after it is treated as a new heading; if it is not one of the five, the remainder of that section is silently dropped on next load (and lost on next save). Workaround: use any other prefix (`###`, `-`). Also: `\r\n` line endings are not handled; a hand-edited file with CRLF will not match headings.

The legacy Python script writes the same format, so files are interchangeable. Difference: Python treats any non-empty file as "already logged"; the app requires all five sections non-empty.

## 4. Scheduling

Mechanism is in-process, not launchd.

- `AppDelegate.applicationDidFinishLaunching`: `try? SMAppService.mainApp.register()` (login item, failure ignored); schedules a repeating 60 s `Timer`; calls `nag()` once immediately.
- `applicationShouldTerminateAfterLastWindowClosed` returns false: closing the window leaves the app running so the timer keeps firing. Cmd-Q quits it.
- `nag()` runs only if all hold: weekday in `2...6` (Mon to Fri, `Calendar.current`, 1 = Sunday); minutes since midnight >= `due`; today not logged. `due` = `UserDefaults["remindMinutes"]` or default 16:55 (1015). Reminder time is edited in the UI via `@AppStorage`.
- Needs a main-capable window in `NSApp.windows`; if none, returns without action.
- `firedDay` logic: if `firedDay != today` (first fire today) the window is always brought to front; after that it is only re-surfaced if it is not visible or is miniaturised. `firedDay` is in memory only, so a relaunch re-fires once. While the log is unsaved and the time has passed, a closed or minimised window reopens within ~60 s; a visible window is left alone.
- Timer fires on the main run loop; sleep/wake just resumes ticking, so a missed time fires on the first tick after wake. If the app is not running (not logged in, quit), no reminder occurs.

## 5. Non-functional requirements

- No network access, no telemetry, no analytics, no third-party code. The app does not import anything networked.
- Local files only: `~/Gloamlog/*.md` plus one `UserDefaults` key (`remindMinutes`).
- Privacy: log content never leaves the machine unless the user moves the files.
- Low footprint: one 60 s timer; `Store.isLogged` re-reads files from disk (see limitations).
- Opens in the user's own markdown tooling; plain text for portability.

## 6. Security notes

- Ad-hoc signed (`--sign -`), not notarised, no Developer ID. Recipients of the zip hit Gatekeeper: right-click, Open the first time, or `xattr -dr com.apple.quarantine`.
- No App Sandbox, no entitlements file, no hardened runtime. The app has normal user-level file access and reads/writes `~/Gloamlog`; it does not touch anything else.
- Registers itself as a login item without prompting; user can remove it under System Settings, General, Login Items.
- Log files are unencrypted and world-readable per the user's umask.
- Legacy script: binds `127.0.0.1` on an ephemeral port, no auth or CSRF token; any local process can post to it during its short lifetime.

## 7. Limitations

- `## ` line splitting (section 3).
- Hard-coded section set; changing it means editing source and mixed old/new files parse partially.
- Fixed Mon to Fri; no holiday or leave handling; no snooze or dismiss other than saving.
- Sidebar and `isLogged` re-read from disk on every view update; fine for tens of files, not thousands.
- No concurrency control: two instances or an external editor can overwrite each other (atomic write, last writer wins).
- `SMAppService.register()` on an ad-hoc signed app in a non-standard location may fail or require approval; errors are swallowed.
- A day selected from the sidebar before today's file exists is created only on Save.
- Reminder time change takes effect on the next tick (read each time).
- Single-architecture binary.

## 8. Test strategy

Nothing has been run: the app has not been compiled in CI, launched, or tested by the author of this doc. Status below is intent, not result.

No test target exists (no SwiftPM/Xcode). Realistic options, in order of value:
1. `Store.load`/`save` round trip: write a temp dir (needs `Store.dir` to be injectable; today it is a `static let`, so a test would currently need to refactor or use a sandboxed `HOME`). Cases: five sections round trip; trimming; emoji headings; missing file; unknown heading dropped; a `## ` user line (documents the limitation); empty section makes `isLogged` false.
2. `nag()` decision logic: extract the guard (weekday, minutes, logged, `firedDay`, visibility) into a pure function of (now, due, logged, firedDay, windowState) and test the table. Currently coupled to `NSApp`/`Store`, so not unit-testable as written.
3. Manual smoke: build, launch, fill five boxes, save, inspect file; set reminder to a minute ahead, close window, confirm it reappears; Cmd-Q; check Login Items.

SwiftUI views are not worth automated tests here.

## 9. Decisions log

**ADR-1: ObservableObject + @StateObject instead of @State.**
Context: in this environment `@State` is an attached macro needing the `SwiftUIMacros` compiler plugin, which ships with Xcode but not Command Line Tools; the first attempt failed to compile. Decision: `Nav` and `DayModel` are `ObservableObject` with `@Published`, held via `@StateObject`; `@AppStorage` and `@Binding` are used where they do not require the plugin. Consequence: slightly more boilerplate, works with CLT only. Revisit if Xcode becomes a requirement.

**ADR-2: In-app 60 s timer + login item instead of launchd.**
Context: the script version used a launchd `StartCalendarInterval` job. Decision: the app owns its schedule so the time is user-editable in the UI, it can re-raise its own window, and there is nothing to install besides the app. Consequence: reminder works only while the app is running; granularity is 60 s; relies on `SMAppService` for survival across logins.

**ADR-3: Markdown files instead of the Notion API.**
Decision: plain `.md` files in `~/Gloamlog`. Reasons: no network, no tokens or credentials to store, no account dependency, works offline, greppable, user can import to Notion by hand. Consequence: no sync or cross-device access.

**ADR-4: Single source file, no project.** Keeps the build to one `swiftc` call and the repo reviewable in one read. Revisit when tests are added.
