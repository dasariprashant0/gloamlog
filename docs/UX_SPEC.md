> **Superseded.** This document describes the earlier five-box version (v0.1 and v0.2). The current plans are in [docs/v1/](v1/) (PRD_V1, UX_FLOWS, DESIGN_V4, TRD, ARCHITECTURE).

# Gloamlog v0.2: UX Spec

**Status**: Design, not built. Extends v1 (`docs/PRD.md`, `docs/DESIGN_SYSTEM.md`, `docs/ARCHITECTURE.md`, `app/Gloamlog.swift`).
**Platform**: macOS 13+, SwiftUI only, no network, no telemetry, plain markdown on disk, built without Xcode (no macros: `ObservableObject` + `@StateObject` + `@AppStorage`, never `@State`/`@Observable`).
**Decided scope** (not up for debate here): 1 draft autosave, 2 skip day, 3 Yesterday card + carry-over, 4 streak + 12-week heatmap, 5 menu bar item, 6 notification + window nag with Strict/Gentle, 7 search, 8 weekly review + copy as markdown, 9 settings window, 10 customisable sections, 11 keyboard shortcuts, 12 first-run onboarding.

Open decisions I made on your behalf are marked **[Decision]**. Things I could not verify without running the app are marked **[Verify]**.

---

## 1. Personas

| | Priya, marketing lead | Marcus, backend engineer | Sana, part-time contractor |
|---|---|---|---|
| Situation | Many small tasks across tools; loses Monday morning re-orienting. | Deep-work days; hates interruptions; wants handover notes for himself. | Works Tue/Wed/Fri; takes unpredictable leave. |
| Wants | A firm end-of-day prompt that does not let her skip the hard sections. | A quiet reminder he can act on when he surfaces from focus. Logs in iCloud Drive so his laptop and desktop agree. | Reminders only on her days; leave that never "breaks" anything. |
| Mode | Strict | Gentle | Gentle, custom weekdays |
| Fears | Forgetting; junk entries. | Being yanked out of flow; sync conflicts. | Guilt UI for days she was legitimately off. |
| Features that matter most | Strict, Yesterday card, carry-over, streak | Gentle, drafts, folder in iCloud, search | Weekday toggles, Skip day, streak neutrality |

Design target: Priya must be nudged firmly, Marcus must never be ambushed, Sana must never be shamed. One app, one setting (strictness) separates them.

---

## 2. Information architecture

```
Gloamlog.app
├── Main window (single Window scene, min 820x600, default 980x740)
│   ├── Sidebar
│   │   ├── Search field (⌘F)
│   │   ├── Today                      -> Day page (today)
│   │   ├── This week                  -> Weekly review (current week)
│   │   ├── History (grouped by month) -> Day page (past, editable)
│   │   └── Footer: streak + 12-week heatmap (cells open that day)
│   └── Detail
│       ├── Day page
│       │   ├── Nag banner (only when due, unlogged, today)
│       │   ├── Header (title, date, status chip)
│       │   ├── Yesterday card (today only, when there is something to carry)
│       │   ├── Sections (user-defined, default 5)
│       │   └── Action row (Save, status, Skip day, Open folder)
│       ├── Skipped-day page
│       ├── Weekly review (any week, prev/next)
│       └── Search results (replaces detail while a query is active)
├── Menu bar extra (MenuBarExtra, .window style popover)
├── Settings window (⌘,)  General | Sections | Storage
├── Onboarding (sheet on the main window, first run only)
└── System surfaces: notification (Open / Snooze 15 min), Dock bounce
```

Navigation rules:
- The sidebar is the only navigation. No tabs, no nested windows besides Settings.
- "Today" is always present and always first. Missed workdays are **not** listed in History (no guilt list). They show as empty bordered cells in the heatmap, and clicking one opens an empty page for that day ("Write it up late").
- Selection model stays `Nav.sel`, extended with a destination enum: `.day(String) | .week(String) | .search`.

On-disk contract the UX depends on (details belong in TRD):

| Thing | Location / format |
|---|---|
| Log | `<folder>/YYYY-MM-DD.md`: `# date`, then `## <section title>` blocks (unchanged from v1; default titles keep their emoji so v1 files load). |
| Skipped day | Same file name. Body is a single blockquote: `> Skipped: Holiday` (reason optional: `> Skipped`). Readable in any editor. |
| Draft | **[Decision]** `~/Library/Application Support/Gloamlog/drafts/YYYY-MM-DD.json`, outside the storage folder. Reason: drafts must survive an unwritable or unsynced folder, and half-written text should not sync between Macs or create iCloud conflict copies. |
| Settings | `UserDefaults` (`@AppStorage`). Sections list stored as JSON in one key. |
| Logged | File exists, is not a skip marker, has at least one `##` section, and **every `##` section in that file is non-empty**. Adding a section later does not un-log old days. |
| Scheduled day | A weekday ticked in Settings. A log or skip on a non-scheduled day is allowed and counts (see streak). |

---

## 3. Screen inventory and wireframes

Width budget: 80 columns of ASCII; `[ ]` button, `( )` radio, `[x]` checked, `▓` filled cell.

### 3.1 Main window: Today (due, unlogged, Strict)

```
┌────────────────────┬─────────────────────────────────────────────────────────┐
│ ⌕ Search           │ ┌─ Banner (only when due and unlogged) ───────────────┐ │
│                    │ │ It's 4:55 pm. Time to write up today.               │ │
│ ● Today        draft│ │             [Remind me in 15 min · 2 left] [Skip day]│ │
│ ▤ This week        │ └─────────────────────────────────────────────────────┘ │
│                    │                                                         │
│ OCTOBER            │  Daily log                                              │
│ ✓ Mon, 5 Oct      │  Tuesday, 6 October 2026               ● Draft saved 4:12│
│ ⊖ Fri, 2 Oct  skip │ ─────────────────────────────────────────────────────── │
│ ✓ Thu, 1 Oct      │ ┌─ Yesterday (Mon 5 Oct) ─────────────────────────────┐ │
│ SEPTEMBER          │ │ To do next                                           │ │
│ ✓ Wed, 30 Sep     │ │   Review pricing page copy; ping Ana about the form  │ │
│ ✓ Tue, 29 Sep     │ │ Pending / blocked                                    │ │
│  ...               │ │   Waiting on legal for the DPA                       │ │
│                    │ │                [Carry over to Pending ▾]  [Hide]     │ │
│ ────────────────── │ └─────────────────────────────────────────────────────┘ │
│ 🔥 12 day streak   │                                                         │
│    Best: 31        │  What I did                                             │
│ M ▓▓▓▓▓▓▓▓▓▓▓▓   │ ┌─────────────────────────────────────────────────────┐ │
│ T ▓▓▓▓▓▓▓▓▓▓▓□   │ │ Everything you worked on today…                      │ │
│ W ▓▓▓▓▓▓▓▓▓▓▓·   │ └─────────────────────────────────────────────────────┘ │
│ T ▓▓▓▓▓▓▓▓⊖▓▓·   │  Finished            (… 4 more sections …)             │
│ F ▓▓▓▓▓▓▓▓▓▓⊖·   │                                                         │
│ S ·············   │  [ Save log ]  3/5 filled      Skip day…   Open folder  │
│ S ·············   │                                                         │
└────────────────────┴─────────────────────────────────────────────────────────┘
```

Notes:
- Banner is inline content at the top of the detail pane, not a sheet or alert. It never blocks typing or other windows.
- Status chip (top right) cycles: `Not started` / `Draft saved 4:12 pm` / `Saved ✓ 4:20 pm` / `Unsaved changes` (past days) / `Couldn't save` (error).
- Empty section placeholders (v1) plus a quiet text button "Nothing to report" at the block's bottom right, visible only while empty. It inserts `Nothing.` so the user can satisfy the required rule honestly instead of typing junk. This answers the PRD kill signal.
- Section blocks grow with content up to 360 pt, then scroll internally (v1 fixed 90 pt min does not grow).
- Save button: `Save log` (enabled at n/n). Footer text mirrors the status chip for VoiceOver redundancy.

### 3.2 Sidebar states

```
Row types (icon + label + trailing)        Meaning
● Today                         [draft]    Today, draft exists, not logged
○ Today                                    Today, nothing yet
✓ Today                                    Today logged (checkmark.circle.fill)
⊖ Fri, 2 Oct                    [skipped]  Skipped (minus.circle)
✓ Thu, 1 Oct                               Logged
● Wed, 30 Sep                   [draft]    Past day with unsaved edits (pencil.circle)
▤ This week                                Weekly review, current week
```

- Icons differ by **shape**, never by colour alone. Accent colour applies only to the logged check.
- History rows grouped under month headers (`Section`), newest first. The list shows only days that have a file (log, skip) or a draft.
- Footer card: streak line, "Best: N", and the heatmap (3.3). Footer is fixed to the bottom; History scrolls above it. Footer hides below 600 pt window height rather than overlapping.
- Context menu on a History row: `Skip this day`, `Show in Finder`, `Copy as Markdown`.

### 3.3 Streak and heatmap (sidebar footer; also reused in the menu bar popover as the last 7 days only)

```
12 weeks, oldest left, newest right. Rows follow system first weekday.
 Legend (shown on hover/help, and in VoiceOver summary):
   ▓ logged   ⊖ skipped (hatched)   □ missed workday (bordered, empty)
   · non-workday (faint dot)        ◉ today (ring)   blank = future
```

Streak rules (computed from files each launch; no stored counter):
1. Walk back from today. If today is unlogged and not skipped, start from yesterday (today is "not yet", not a break).
2. Scheduled day logged: +1. Scheduled day skipped: neutral (no +1, no break). Non-scheduled day: neutral if empty; +1 if logged (extra effort is not penalised).
3. A past scheduled day that is unlogged and not skipped ends the streak.
4. "Best" is the longest run under the same rules over all files.
5. Changing weekday settings re-evaluates all history with the current schedule. **[Decision]** simple and deterministic; the alternative (versioned schedules) is not worth it.
6. Zero streak copy: "No streak yet" (not "0 days", not "Streak lost").

Cell size 11 pt with 3 pt gap: 12 columns = 162 pt, fits the 190 pt minimum sidebar. Each cell is a button that opens that day (missed day opens a blank page).

### 3.4 Menu bar popover (MenuBarExtra, `.menuBarExtraStyle(.window)`)

```
 menu bar:  [ ✎ ]  (state icon, see below)

┌──────────────────────────────────────┐
│ Today · Tue 6 Oct                    │
│ ● Not logged yet                     │   state line
│ Reminder at 4:55 pm · Strict         │   secondary
│ ──────────────────────────────────── │
│ 🔥 12 day streak            Best 31  │
│ M T W T F S S  (last 7 days, cells)  │
│ ▓ ▓ ▓ ⊖ ▓ · ·                        │
│ ──────────────────────────────────── │
│ [ Open Gloamlog           ⌘O ]      │   primary
│ Remind me in 15 min   (when due)     │
│ Skip today…                          │
│ ──────────────────────────────────── │
│ Settings…                       ⌘,   │
│ Quit Gloamlog                  ⌘Q   │
└──────────────────────────────────────┘
```

State line variants: `Logged today ✓ 4:20 pm` · `Not logged yet` · `Due now: 4:55 pm` · `Skipped today (Holiday)` · `Not a workday` · `Saving problem: folder unavailable`.

Menu bar icon (template images, shape differs per state, plus title text for VoiceOver):

| State | SF Symbol | Accessibility title |
|---|---|---|
| Before reminder, unlogged | `book.closed` | Gloamlog: today not yet logged |
| Due, unlogged | `pencil.circle.fill` | Gloamlog: time to write up today |
| Logged | `checkmark.circle` | Gloamlog: today logged |
| Skipped / not a workday | `moon.zzz` / `circle.dashed` | Gloamlog: skipped today / no log needed today |
| Folder problem | `exclamationmark.triangle` | Gloamlog: can't save, folder unavailable |

Toggle in Settings: "Show in menu bar" (`MenuBarExtra(isInserted:)`, default on). Clicking Open raises the main window (does not activate if already frontmost).

### 3.5 Skip-day confirmation (sheet on main window; also from menu bar, which opens the main window first)

```
┌────────────────────────────────────────────────────┐
│  Skip today?                                       │
│                                                    │
│  No reminder, no nag, and your streak stays as it  │
│  is. You can undo this any time.                   │
│                                                    │
│  Reason (optional)                                 │
│  [Holiday] [Leave] [Sick] [Day off] [Other…]       │
│                                                    │
│  ( ) Today only                                    │
│  ( ) Today through  [ Fri 9 Oct ▾ ]  (3 workdays)  │
│                                                    │
│  Your draft for today is kept, not deleted.        │
│                                                    │
│                         [ Cancel ]  [ Skip day ]   │
└────────────────────────────────────────────────────┘
```

- Reason chips are single-select (segmented-style); "Other…" reveals a one-line text field (max 40 chars). Reason is stored in the marker file.
- Range option writes one marker file per scheduled day in range. **[Decision]** include; it is a small extension and leave weeks are the main real-world use. Cut if scope tightens: single day alone satisfies the brief.
- Default button is `Skip day`; Return confirms, Esc cancels. If today is already logged, the sheet is unavailable (menu item disabled with "Today is already logged").
- Result: banner disappears, notification and pending requests are cancelled, sidebar row becomes `⊖ Skipped`, toast-free (the page itself changes, see 3.6).

### 3.6 Skipped-day page

```
  Daily log
  Tuesday, 6 October 2026                         ⊖ Skipped · Holiday
 ───────────────────────────────────────────────────────────────────
  You skipped this day. It doesn't affect your streak.

  [ Write a log anyway ]   [ Undo skip ]
```

"Write a log anyway" replaces the marker once saved; if the user abandons, the skip stays.

### 3.7 Weekly review

```
┌────────────────────┬─────────────────────────────────────────────────────────┐
│ (sidebar)          │  Week of 5 – 11 Oct 2026          [‹] [This week] [›]   │
│ ▤ This week  <     │  Logged 4 of 5 · 1 skipped · streak 12                  │
│                    │  ┌ Group by: (•) Day  ( ) Section ┐   [ Copy as markdown ]│
│                    │ ─────────────────────────────────────────────────────── │
│                    │  Mon 5 Oct                                    [Open]    │
│                    │   What I did   Drafted pricing copy; reviewed the form…  │
│                    │   Finished     Pricing page v2                           │
│                    │   Started      Webinar landing page                      │
│                    │   Pending      Waiting on legal                          │
│                    │   To do next   Review copy with Ana                      │
│                    │  Tue 6 Oct                       Not logged yet · [Open] │
│                    │  Wed 7 Oct                       ⊖ Skipped · Holiday     │
│                    │  ...                                                    │
└────────────────────┴─────────────────────────────────────────────────────────┘
```

- By Section mode groups each section title with a bullet list "Mon: …" across the week. Default is Day.
- Long text in a day collapses to 4 lines with "Show more" (per section). Copy always includes full text.
- Weeks start on the system's first weekday. Prev/next change the week; "This week" returns. Future weeks are disabled.
- Empty week: see 7.
- `Copy as markdown` copies exactly the displayed grouping; button label changes to `Copied ✓` for 2 s (not a toast).

Copied format (Day grouping):

```markdown
# Week of 5 Oct – 11 Oct 2026

Logged 4 of 5 workdays · 1 skipped

## Mon 5 Oct

### What I did
...

### Finished
...

## Wed 7 Oct
_Skipped: Holiday_
```

Section grouping uses `## <section title>` then `- **Mon 5 Oct**: first paragraph...` lines with the day's full text indented. Unlogged days are omitted from the text (not listed as missed).

### 3.8 Search results

```
┌────────────────────┬─────────────────────────────────────────────────────────┐
│ ⌕ pricing      (x) │  Search: "pricing"                       6 matches in 4  │
│                    │  Section: [ All sections ▾ ]                days        │
│ (sidebar still     │ ─────────────────────────────────────────────────────── │
│  visible; selecting│  Mon, 5 Oct 2026                                         │
│  a row clears      │   What I did   …drafted the **pricing** page copy and…   │
│  search)           │   To do next   …review **pricing** page with Ana…        │
│                    │  Thu, 1 Oct 2026                                         │
│                    │   Finished     …shipped **pricing** table update…        │
└────────────────────┴─────────────────────────────────────────────────────────┘
```

- Uses `.searchable` (macOS 13). Case- and diacritic-insensitive substring; whitespace-separated words are AND-ed. Results update as you type with a 150 ms debounce. Matches highlighted via `AttributedString`.
- Index is built in memory at launch and updated on save/refresh (logs are small). No persistent index file.
- Click a result (or Return on the selected one) opens that day and focuses the matched section; Esc clears search and returns to the previous destination.
- Includes drafts? **[Decision]** No. Search covers saved files only, to avoid showing text that is not on disk.

### 3.9 Settings window (`Settings` scene, ⌘,)

Tabs: General, Sections, Storage. Fixed width 520, height fits content.

```
General
┌──────────────────────────────────────────────────────────┐
│ Remind me at        [ 4:55 PM ⌃ ]                         │
│ On these days       [M] [T] [W] [T] [F] [ S ] [ S ]       │
│                                                          │
│ When the reminder fires                                  │
│  (•) Strict   Bring Gloamlog to the front and keep      │
│               re-opening it every 5 minutes until you    │
│               save or skip the day.                      │
│  ( ) Gentle   Send one notification. Nothing else.       │
│                                                          │
│ Snooze for          [ 15 minutes ▾ ]  (5, 10, 15, 20, 30)│
│   Strict allows 2 snoozes a day. Gentle allows any.      │
│                                                          │
│ [x] Open Gloamlog at login                              │
│ [x] Show in menu bar                                     │
│ Notifications: On   [Open System Settings]               │
└──────────────────────────────────────────────────────────┘

Sections
┌──────────────────────────────────────────────────────────┐
│ ≡  What I did            Hint: Everything you worked… ✎ ⌫│
│ ≡  Finished              Hint: What got done and clo… ✎ ⌫│
│ ≡  Started               ...                           ✎ ⌫│
│ ≡  Pending / blocked     ...                           ✎ ⌫│
│ ≡  To do next            ...                           ✎ ⌫│
│ [ + Add section ]                      [ Reset to default ]│
│ Every section is required when you save. 1 to 10 sections.│
└──────────────────────────────────────────────────────────┘

Storage
┌──────────────────────────────────────────────────────────┐
│ Logs are saved in                                        │
│  ~/Library/Mobile Documents/com~apple~CloudDocs/Gloamlog │
│  [ Choose folder… ]  [ Show in Finder ]  [ Use default ] │
│  128 logs · 3 skipped days · 2 files not recognised      │
│                                                          │
│ Tip: pick a folder in iCloud Drive or Dropbox to keep    │
│ your logs on every Mac.                                  │
└──────────────────────────────────────────────────────────┘
```

Behaviour:
- Time picker: hour+minute, same control as v1 (`DatePicker`, `.hourAndMinute`).
- Weekday toggles: seven checkable buttons using `Toggle` with `.toggleStyle(.button)`, labelled with full names for VoiceOver ("Monday"). At least one day must stay on (the last one disables itself with help text "Keep at least one day").
- Strict/Gentle: radio group (`Picker` with `.radioGroup`), each with a one-line consequence under the label.
- Launch at login: `SMAppService.mainApp.register()/unregister()`. If state is `.requiresApproval`, show inline "Approve in System Settings > General > Login Items" with a button. v1 registered silently; v0.2 makes it visible and reversible.
- Sections tab: rows reorder with `List` `.onMove`; name is edited inline (`TextField`); hint is optional and editable in a small popover (✎); remove (⌫) asks for confirmation (7.4). Changes apply immediately; they affect **new** edits only. Names: 1 to 40 chars, unique (case-insensitive), cannot start with `#`.
- Storage tab: see flow 4.8.
- Settings changes never raise the main window (nag suppressed for 60 s after any settings change).

### 3.10 Onboarding (sheet over main window, 3 steps, Back/Continue, progress dots)

```
Step 1 of 3 - Welcome
┌───────────────────────────────────────────────────────┐
│  Write up your day, every day.                        │
│                                                       │
│  At a time you choose, Gloamlog asks five short      │
│  questions: what you did, what's done, what you       │
│  started, what's stuck, what's next.                  │
│                                                       │
│  • Your logs are plain markdown files you own.        │
│  • Nothing leaves your Mac. No account. No tracking.  │
│                                                       │
│                                    [ Continue ]       │
└───────────────────────────────────────────────────────┘

Step 2 of 3 - Reminder
┌───────────────────────────────────────────────────────┐
│  When should we ask?                                  │
│  Time  [ 4:55 PM ]   Days [M][T][W][T][F][ S ][ S ]   │
│                                                       │
│  How firm?                                            │
│  ┌──────────────────────┐ ┌──────────────────────┐    │
│  │ (•) Strict           │ │ ( ) Gentle           │    │
│  │ Comes to the front   │ │ One notification.    │    │
│  │ until you save or    │ │ You decide when.     │    │
│  │ skip the day.        │ │                      │    │
│  └──────────────────────┘ └──────────────────────┘    │
│  You can always skip a day or snooze. Change this     │
│  later in Settings.                                   │
│                                                       │
│                         [ Back ]  [ Continue ]        │
└───────────────────────────────────────────────────────┘
  (Continue triggers the macOS notification permission prompt, after the
   reason is on screen. Denied: see 7.7.)

Step 3 of 3 - Folder
┌───────────────────────────────────────────────────────┐
│  Where should logs live?                              │
│  (•) ~/Gloamlog   (default, on this Mac)             │
│  ( ) iCloud Drive / Gloamlog                          │
│  ( ) Another folder…  [ Choose… ]                     │
│                                                       │
│  Pick iCloud Drive or Dropbox to keep logs on every   │
│  Mac. You can change this later.                      │
│                                                       │
│  [x] Open Gloamlog at login                          │
│                         [ Back ]  [ Start logging ]   │
└───────────────────────────────────────────────────────┘
```

- Everything is pre-filled; a user who accepts defaults presses Continue, Continue, Start logging.
- "iCloud Drive / Gloamlog" option appears only if the iCloud Drive folder exists on disk.
- Existing v1 users (a `~/Gloamlog` folder with logs exists, or `remindMinutes` is set): skip onboarding, keep Strict and their reminder time, show a one-time sheet "What's new in 0.2" with 4 bullets and a "Take a look at Settings" button.
- Esc does not dismiss onboarding; a visible `Skip setup` text button on step 1 applies defaults.

### 3.11 Nag surfaces (not screens, but specified)

| Surface | Strict | Gentle |
|---|---|---|
| Notification at reminder time | Yes (once) | Yes (once) |
| Main window forced forward | Yes, on first fire; re-opens (see 5.2) | Never |
| In-window banner on Today | Yes, when due | Yes, when due and the user opens the window |
| Menu bar icon `pencil.circle.fill` | Yes | Yes |
| Dock icon bounce | Once at first fire | No |

---

## 4. Flows

Notation: `[user]` acts, `(app)` responds.

### 4.1 First run
1. [user] launches the app for the first time. (app) shows main window with onboarding sheet.
2. Step 1 read, Continue. Step 2: time, days, strictness, Continue. (app) asks macOS for notification permission.
3. Step 3: folder, login item, Start logging. (app) creates the folder (if absent), registers login item (if ticked), schedules reminders, dismisses sheet, shows empty Today page with first section focused.
4. Empty-state line on Today: "Nothing logged yet. Take a few minutes to write up today, or come back at 4:55 pm."
5. If it is already past the reminder time and today is a workday, no nag fires in the first 2 minutes (setup is the interaction).

### 4.2 Daily flow (Strict, happy path)
1. 4:55 pm. (app) posts a notification and brings the window forward, banner at top, first empty section focused.
2. [user] types. (app) autosaves a draft every ~1 s after the last keystroke; status chip reads `Draft saved 4:57 pm`.
3. Optional: [user] uses Yesterday card, carries items over (4.6).
4. 5/5 filled. [user] presses ⌘S or ⌘Return. (app) writes the markdown file, deletes the draft, status `Saved ✓ 5:05 pm`, sidebar row becomes ✓, banner and notifications clear, streak updates, menu bar icon becomes `checkmark.circle`. The only motion is the status text change (and the streak number ticking up, 150 ms, reduced-motion safe).
5. No extra congratulation modal. One quiet line under the action row for 4 s if the streak grew: "12 days in a row."

### 4.3 Strict-mode forcing
```
reminder time (workday, unlogged, not skipped, not snoozed)
 └─ fire #1: notification + window to front + activate + Dock bounce
      ├─ [user saves]            -> stop. Nothing more today.
      ├─ [user skips day]        -> stop.
      ├─ [user snoozes] (max 2)  -> silent until snooze ends, then fire #1 again (counts as a new fire; no extra notification)
      ├─ [user closes/minimises] -> wait 5 min -> re-open window WITHOUT activating (see 5.2)
      │     └─ 3rd re-open that evening: banner adds a single extra line
      │        "Not today? Skip day"  (once; no escalation)
      └─ 23:00 or midnight -> nag ends for the day. Menu bar shows pending. Day remains writable.
```
User can always: ⌘Q (the app quits, nag stops), ⌘W/⌘H, switch apps. The nag never blocks them.

### 4.4 Snooze
1. Entry points: banner button, notification action "Snooze 15 min", menu bar "Remind me in 15 min", ⌘⇧L in the main window.
2. (app) records `snoozedUntil = now + N` (not persisted across launch, but persisted as "snoozes used today" count to resist quit-and-relaunch gaming: stored with the date).
3. Strict, 2 snoozes per day. After the second, the button becomes disabled text "No snoozes left today" and the banner offers Skip day. Gentle: unlimited, no counter.
4. During snooze: menu bar icon unchanged (still pending), window stays closed if closed, no notifications.
5. When snooze ends: Strict fires as 4.3 fire #1 without a second notification; Gentle posts the notification again.

### 4.5 Skip day
1. [user] chooses Skip day from: action row, banner, menu bar, History/heatmap context menu, or ⌘⇧K.
2. (app) shows the sheet (3.5). [user] picks reason and scope, Skip day.
3. (app) writes marker file(s), cancels pending notifications for those days, clears banner, updates sidebar/heatmap (hatched), streak unchanged.
4. Undo: skipped-day page `Undo skip` deletes the marker. If that day is today and past reminder time, the nag resumes under normal rules (with a 2 min grace, never instantly on undo).
5. Skipping a day that has a draft keeps the draft. Skipping a day that is already logged is not offered.

### 4.6 Carry-over (Yesterday card)
1. Card appears on Today when the most recent previous logged day has non-empty `To do next` or `Pending / blocked` text. Heading is "Yesterday" if that day is yesterday, otherwise its weekday ("Friday") or date if older than 6 days.
2. [user] clicks `Carry over to Pending` (primary action of a split `Menu`; the chevron menu lists every other section: "Carry over to Finished", "Carry over to Started", etc.).
3. (app) appends the card's text to the chosen block, one blank line separated, as plain lines prefixed `- ` only if the source did not already use list markers; lines are tagged `(from Mon 5 Oct)` once per carry. De-duplicates identical lines already present. Button turns into `Carried ✓` plus `Undo` for 10 s (an undo of exactly that insertion).
4. Draft autosave runs as usual. The card collapses to a single line "Yesterday: carried over" after carry, expandable.
5. `Hide` removes the card for today only. **[Decision]** Both source sections go to one target by default ("Pending / blocked" if present, else the first section). The user picks the target per action; there is no "Both" magic that guesses wrong.
6. Card text is read-only, selectable, and long text is clamped to 6 lines with "Show more".

### 4.7 Weekly review
1. [user] selects "This week" (⌘2) or the menu bar "Open" then ⌘2. (app) shows 3.7 for the current week.
2. [user] toggles Day/Section, steps through weeks with ⌘[ and ⌘] (when Weekly is focused), or clicks `Open` on a day.
3. `Copy as markdown` (⌘⇧C) puts markdown on the pasteboard; label confirms `Copied ✓`.

### 4.8 Change storage folder with existing logs
```
Settings > Storage > Choose folder…  -> NSOpenPanel (directories only, "Choose")
 ├─ Pre-check new folder: writable? (else error 7.1, nothing changes)
 ├─ Case A: new folder is empty
 │    Dialog: "Move your 128 logs to 'Gloamlog'?"
 │      [Move logs]  [Copy logs]  [Leave them]   [Cancel]
 │      Default button: Copy logs   (safest; originals untouched)
 ├─ Case B: new folder already has logs (e.g. second Mac)
 │    Dialog: "'Gloamlog' already has 96 logs."
 │      Body: "Use them as they are, or also copy over the 128 from your
 │             current folder. Days that exist in both are never overwritten."
 │      [Use this folder]  [Use and copy mine over]  [Cancel]
 │      Conflicts (same date in both): destination wins; the old file stays
 │      where it is; a result line lists them with a Reveal button.
 └─ After: (app) switches folder, reloads sidebar/streak/heatmap, shows a result
    line in the Storage tab: "Copied 124 logs. 4 days already existed in the new
    folder and were left alone. [Show them]"
```
- Never deletes originals, even on "Move": move is implemented as copy then remove only the files successfully copied and byte-identical. Failure mid-way leaves remaining files in place and reports counts.
- Drafts are unaffected (they live outside the folder).
- Reminder state and settings do not change.
- If the chosen folder is the same as the current one: no-op with "That's already your log folder."

---

## 5. Forcing-UX philosophy

### 5.1 Principle: firm, not hostile

The app exists because gentle prompts get ignored (PRD problem statement). The nag is the product. It stays acceptable because it follows five rules:

1. **One clear task.** Every nag surface points to the same thing: write the day or skip the day.
2. **Always an exit.** Skip day and a limited snooze are visible on every nag surface; ⌘Q always works. A user who cannot escape quits the app, which silences it forever (PRD risk). Escape hatches protect the habit.
3. **Bounded.** Fixed re-open interval, daily snooze cap, nag ends at 23:00, never on non-workdays, skipped days or logged days.
4. **Constant, not escalating.** The 5th interruption looks and sounds exactly like the 1st. No red, no urgency words, no shrinking snooze, no counters of failures.
5. **Honest and neutral voice.** State the time and the task. Never moralise, never mention the streak as a threat.

### 5.2 What a Strict nag MAY do
- Post one notification at reminder time (with Open and Snooze actions).
- Bring the main window to front and activate the app **once** at the first fire of the day (matches v1 F6).
- Bounce the Dock icon once.
- Re-open the main window if the user closed or minimised it, at most once every 5 minutes. Re-opens use `orderFrontRegardless` **without** activating the app, so keystrokes in the user's current app are not stolen mid-sentence (the window appears in front, key focus stays elsewhere). **[Verify]** that SwiftUI `Window` honours this on macOS 13; fallback is to activate only if the frontmost app has been idle for 3 s (`CGEventSource` idle time).
- Show a banner and menu bar pending icon until resolved.
- After the 3rd re-open in one evening, add one supportive line to the banner: "Not today? Skip day." Once.
- Re-fire after a snooze ends.

### 5.3 What a Strict nag MAY NOT do
- Block quitting, hiding, closing, or app switching. No modal that disables other UI. No full-screen or always-on-top level.
- Steal keyboard focus after the first fire of the day (5.2).
- Re-open more often than every 5 minutes, or at all when the screen is locked, display asleep, or screensaver active.
- Escalate: no sound loops, no repeated notifications (a single notification is replaced, not stacked), no badge counts, no red styling, no wording change.
- Shame: no "you have missed N days", no streak-at-risk warning, no countdowns.
- Nag on a non-scheduled day, a skipped day, a logged day, or after 23:00 (nag cutoff is a constant in v0.2; see Open Questions).
- Re-open itself within 60 s of a Settings change, within 2 min of an undo-skip, or within 2 min of login launch.
- Delete or alter any text in the user's draft.
- Include log text, streak numbers or any content in notifications.

### 5.4 Gentle
One notification at reminder time, replaced (not repeated) if snoozed. The window is never forced. The menu bar icon shows pending, the banner shows when the window is opened. The Gentle user is trusted to open the app; the app owes them clarity, not pressure.

### 5.5 Escape hatches summary

| Hatch | Strict | Gentle |
|---|---|---|
| Skip day | Always available, one confirmation | Same |
| Snooze | 2 per day, N minutes (setting) | Unlimited |
| Close/hide window | Allowed; re-open per 5.2 | Allowed; no re-open |
| ⌘Q | Always quits; nag stops | Same |
| Per-day weekdays | Settings | Settings |
| "Nothing to report" | Satisfies a required section honestly | Same |

---

## 6. UI copy

Voice: plain, calm, second person, sentence case, verbs on buttons, no exclamation marks, no emoji in system strings (section names are user data). Times follow the user's locale; examples use en-US.

### 6.1 Core strings

| Where | String |
|---|---|
| Page title | `Daily log` |
| Status chip | `Not started` / `Draft saved 4:12 pm` / `Saved ✓ 4:20 pm` / `Unsaved changes` / `Couldn't save` |
| Progress | `3 of 5 filled` (replaces `3/5 filled`; VoiceOver reads it naturally) |
| Primary button | `Save log` (past day with edits: `Save changes`) |
| Secondary | `Skip day…` · `Open folder` · `Revert changes` |
| "Nothing" link | `Nothing to report` (inserts `Nothing.`) |
| Streak | `12 day streak` / `1 day streak` / `No streak yet` · `Best: 31` |
| Today empty | `Nothing logged yet. Take a few minutes to write up today, or come back at 4:55 pm.` |
| Banner (due) | `It's 4:55 pm. Time to write up today.` |
| Banner buttons | `Remind me in 15 min` · `Remind me in 15 min · 1 left` · `No snoozes left today` · `Skip day` |
| Banner extra (3rd re-open) | `Not today? Skip day.` |
| Banner, Gentle | `Today isn't logged yet.` |
| Notification title | `Time to write up your day` |
| Notification body | `It takes a few minutes. Gloamlog is ready when you are.` |
| Notification actions | `Open` · `Snooze 15 min` |
| Late notification (launched after reminder, Gentle) | title `Today isn't logged yet` / body `It's 6:10 pm. Open Gloamlog whenever you're ready.` |
| Draft restored | `Draft restored from 4:12 pm.` with `Discard draft` text button |
| Past-day edit banner | `Editing Monday, 28 September 2026` |
| Yesterday card | heading `Yesterday` / `Friday` / `Fri, 2 Oct`; labels `To do next`, `Pending / blocked`; buttons `Carry over to Pending` · menu `Carry over to…` · `Hide`; after: `Carried ✓` · `Undo` |
| Yesterday collapsed | `Yesterday: carried over. Show` |
| Skip sheet | title `Skip today?` / body `No reminder, no nag, and your streak stays as it is. You can undo this any time.` / `Reason (optional)` / `Today only` / `Today through` / `Your draft for today is kept, not deleted.` / `Cancel` · `Skip day` |
| Skip sheet (range) | title `Skip these days?` / `3 workdays: Wed 7 to Fri 9 Oct` / `Skip 3 days` |
| Skipped page | `You skipped this day. It doesn't affect your streak.` · `Write a log anyway` · `Undo skip` |
| Menu bar | `Open Gloamlog` · `Remind me in 15 min` · `Skip today…` · `Settings…` · `Quit Gloamlog` |
| Menu bar, logged | `Logged today ✓` · skip item disabled: `Today is already logged` |
| Search | placeholder `Search your logs` · `6 matches in 4 days` · `No matches for "pricing". Try a shorter word.` · section filter `All sections` |
| Week review | `Week of 5 to 11 Oct 2026` · `Logged 4 of 5 · 1 skipped` · `Copy as markdown` · `Copied ✓` · `Not logged yet` · `Skipped · Holiday` |
| Week empty | `Nothing logged this week yet.` |

### 6.2 Settings strings

| Control | Label | Help text |
|---|---|---|
| Time | `Remind me at` | |
| Days | `On these days` | `Keep at least one day.` |
| Strictness | `When the reminder fires` | |
| Strict | `Strict` | `Brings Gloamlog to the front and keeps re-opening it every 5 minutes until you save or skip the day.` |
| Gentle | `Gentle` | `Sends one notification. Nothing else.` |
| Snooze | `Snooze for` | `Strict allows 2 snoozes a day. Gentle allows any number.` |
| Login | `Open Gloamlog at login` | `Needed for reminders after a restart.` |
| Menu bar | `Show in menu bar` | |
| Notifications off | `Notifications are off for Gloamlog.` | `Gentle needs them. Strict still opens the window.` + `Open System Settings` |
| Login needs approval | `Approve Gloamlog in System Settings > General > Login Items.` | `Open Login Items` |
| Sections footer | `Every section is required when you save. 1 to 10 sections.` | |
| Remove section confirm | `Remove "Pending / blocked"?` / `New logs won't have this section. What you already wrote stays in your files and in past days.` / `Remove section` · `Keep section` | |
| Rename note | `Past logs keep the old name. They still open fine.` | |
| Storage | `Logs are saved in` · `128 logs · 3 skipped days` · `2 files not recognised` · `Show them` | `Tip: pick a folder in iCloud Drive or Dropbox to keep your logs on every Mac.` |

### 6.3 Folder-change strings

| Case | Title | Body | Buttons |
|---|---|---|---|
| Empty destination | `Move your 128 logs to "Gloamlog"?` | `Copy keeps your originals where they are.` | `Copy logs` (default) · `Move logs` · `Leave them` · `Cancel` |
| Destination has logs | `"Gloamlog" already has 96 logs.` | `Days that exist in both are never overwritten.` | `Use this folder` · `Use and copy mine over` · `Cancel` |
| Result | `Copied 124 logs. 4 days already existed in the new folder and were left alone.` | | `Show them` · `Done` |
| Same folder | `That's already your log folder.` | | `OK` |

### 6.4 Onboarding strings

| Step | Copy |
|---|---|
| 1 | Title `Write up your day, every day.` Body `At a time you choose, Gloamlog asks five short questions: what you did, what's done, what you started, what's stuck, and what's next.` Bullets `Your logs are plain markdown files you own.` · `Nothing leaves your Mac. No account, no tracking.` Buttons `Continue` · `Skip setup` |
| 2 | Title `When should we ask?` / `How firm?` Strict `Strict: Comes to the front until you save or skip the day.` Gentle `Gentle: One notification. You decide when.` Footnote `You can always skip a day or snooze. Change this later in Settings.` |
| 3 | Title `Where should logs live?` Options `On this Mac: ~/Gloamlog` · `iCloud Drive: Gloamlog` · `Another folder…` Help `Pick iCloud Drive or Dropbox to keep logs on every Mac.` Checkbox `Open Gloamlog at login` Button `Start logging` |
| What's new (v1 users) | `What's new in 0.2` · `Skip a day without breaking your streak.` · `Gentle mode and notifications.` · `Menu bar item, search, weekly review.` · `Choose your own sections and folder.` · `Take a look at Settings` |

### 6.5 Error and edge strings

| Situation | Copy |
|---|---|
| Folder missing | `Your log folder can't be found.` / `"~/Documents/Gloamlog" isn't there any more. It may have been moved or an external drive is disconnected.` / `Choose folder…` · `Try again` · `Create it again` |
| Folder unwritable | `Gloamlog can't save to this folder.` / `You may not have permission to write to "Gloamlog". Your text is safe as a draft on this Mac.` / `Choose folder…` · `Try again` · `Copy my text` |
| Save failed (generic) | `Couldn't save today's log.` / `<system reason>. Your draft is kept. Try again or choose another folder.` / `Try again` |
| iCloud file downloading | `Downloading from iCloud…` |
| File changed elsewhere | `This log changed on another Mac.` / `You have unsaved edits here.` / `Keep mine` · `Use the other version` (mine is kept as a draft) |
| Midnight rollover | `It's now Tuesday, 7 October. You're still writing Monday, 6 October.` · `Start today's log` (after save) |
| Clock changed | `The date changed. Gloamlog updated your reminder.` (menu bar tooltip only, no dialog) |
| Notification denied | see 6.2 |
| Section name taken | `A section named "Finished" already exists.` |
| Section name empty | `Give this section a name.` |

---

## 7. Empty, error and edge states

### 7.1 Storage folder missing or unwritable
- Detection: on launch, on window focus, on every save, and on folder change. Check exists and writable (create then remove a hidden temp file).
- Banner at top of every Day page (not a dialog): copy in 6.5. Status chip: `Couldn't save`.
- Drafts keep autosaving locally (outside the folder), so no text is lost.
- Save stays enabled (user can retry once fixed). `Copy my text` puts all sections as markdown on the pasteboard.
- Sidebar shows days already loaded; History shows `Can't read folder` row if listing fails.
- Menu bar icon `exclamationmark.triangle`; state line `Saving problem: folder unavailable`.
- Nag still fires (the user must act) but the banner leads with the folder problem, not the reminder.
- Never silently fall back to a different folder. Choosing another folder is always explicit.

### 7.2 Midnight rollover while typing
- The editor is bound to the day it was opened for. Text typed at 23:58 and still being typed at 00:03 belongs to the opened day. Nothing moves.
- At 00:00 the sidebar re-labels: the previous day loses "Today" and gets its date label; a new `Today` row appears. Selection stays on the previous day (no jump).
- Banner on the open page: `It's now Tuesday, 7 October. You're still writing Monday, 6 October.`
- On save, the app offers `Start today's log`. The nag for the new day follows normal rules from the reminder time (not at midnight).
- The old day does not become "missed" for nag purposes; it can be saved late. In the heatmap an unlogged-after-midnight workday becomes a bordered empty cell until saved.
- Draft: continues saving under the opened day's key.

### 7.3 Editing past days
- Allowed for any day with a file or any workday in the heatmap.
- No nag, no banner except `Editing Monday, 28 September 2026`, subtle tinted header.
- Same rule: all sections must be non-empty to save. `Revert changes` discards edits back to the file contents.
- If edits empty a section, Save is disabled and the file stays untouched until fixed or reverted. Draft autosave keeps edits.
- Skipped days can be converted to logs (3.6) and vice versa (Undo skip deletes only the marker).
- Past-day edits cannot affect the streak until saved.

### 7.4 Custom sections: renamed, removed, reordered
- Each section has a stable internal id plus an alias list of previous titles. Loading a file maps `##` titles through current title, then aliases, so renamed sections still load old logs into the right block.
- A `##` heading in a past file that matches no current section or alias is a **retired section**: shown below the configured ones with a muted label `Retired section` and its text, editable, preserved on save. Text is never dropped.
- Removing a section affects future/new content only (confirmation in 6.2). Reordering changes display and new-file order only; existing files are re-written in current order on next save.
- A new section added later appears empty in old days; old days stay Logged until the user edits and saves them (then it is required).
- Last remaining section cannot be removed. Maximum 10.

### 7.5 Very long text
- No character limit. Blocks grow to 360 pt then scroll inside.
- Autosave debounce stays 1 s; the write is off the main thread. Weekly review and search truncate previews, not the underlying data.
- Pasting rich text pastes as plain text. Tabs and emoji are preserved.
- A line beginning with `## ` in user text must round-trip without splitting a section (v1 ponytail). UX requirement: the editor shows exactly what the user typed after reload; implementation chooses an escape on save and unescape on load.

### 7.6 Clock, timezone, DST
- "Today" and the reminder use the current local calendar day and wall-clock time. Travel shifts them with the system.
- On `NSSystemClockDidChange`, `NSSystemTimeZoneDidChange`, `NSCalendarDayChanged`, and wake: recompute today, re-schedule notifications, reset the in-memory "fired today" flag if the date key changed.
- Clock moved back across a logged day: the earlier date opens its existing file; no duplicate is created.
- Clock moved forward over several days: missed workdays become bordered cells; no backfill dialog, no retroactive nag.
- DST: a reminder at 2:30 am on a spring-forward night does not exist; reminders default to 4:55 pm so this is rare. If it occurs the system's resolved time applies.

### 7.7 Launched after reminder time
- On launch (login, relaunch, or manual) on a scheduled, unlogged, unskipped day past reminder time:
  - **Strict**: after a 2-minute grace if launched at login (let the desktop settle), 10 s grace if launched by the user; then fire #1 (window front, no notification).
  - **Gentle**: one notification using the "late" copy (6.1), then silence.
  - Both: menu bar shows pending.
- If the launch is after 23:00, nothing fires; menu bar shows pending.
- If notifications are denied and Gentle is selected: banner on first open and in Settings explains; the menu bar icon is the fallback signal.

### 7.8 Laptop asleep through the reminder
- Reminders are scheduled as `UNCalendarNotificationTrigger` requests (rolling 7 scheduled days) in addition to the in-process timer so the notification survives the app being suspended. **[Verify]** delivery after wake on macOS 13.
- On wake (`NSWorkspace.didWakeNotification`): wait 15 s for unlock; if now is past the reminder, the day is unlogged and before the cutoff: Strict fires #1 once; Gentle posts at most one "late" notification and only if none was delivered today.
- Never fire a burst for several missed reminders. Only today matters.
- If the machine wakes after 23:00, nothing fires; the pending state is visible in the menu bar.

### 7.9 Other edge states

| State | Behaviour |
|---|---|
| App quit with unsaved text | Draft flushed synchronously in `applicationWillTerminate`, on window close, on resign active, on day switch. Next open: `Draft restored from 4:12 pm.` |
| Crash | Draft is at most ~1 s stale. |
| Draft newer than file | Load draft. Draft older than file (file edited elsewhere): load file, discard draft silently if text identical, else ask (6.5 "changed on another Mac"). |
| Save while sections empty | ⌘S flushes the draft and focuses the first empty section; status `2 sections still empty`. No error dialog. |
| First weekday | Heatmap and week rows follow `Calendar.current.firstWeekday`. |
| No logs ever | Sidebar History shows `Your logs will appear here.`; heatmap shows empty cells; streak `No streak yet`. |
| Unrecognised files in folder (conflict copies like `2026-10-06 2.md`) | Ignored in the sidebar; Storage tab shows `2 files not recognised` with `Show them` (reveal in Finder). Never deleted. |
| Weekend/non-workday opened | Today row says `Not a workday` chip; page fully usable; no nag. |
| Window closed, app running | Same as v1: app stays alive; Dock/menu bar remain. |
| All weekdays off | Prevented in Settings. |
| Very small window | Footer card hides below 600 pt height; sidebar can collapse (`NavigationSplitView` standard). |
| Reminder after a log saved today | Nothing fires; pending requests for today are cancelled on save. |
| Settings reminder time moved to a past time today | Normal rules apply but the 60 s suppression prevents an instant pop. |

---

## 8. Keyboard shortcut map

Registered through `.commands` menu items, so they appear in the menu bar and are discoverable. All work without the mouse.

| Action | Shortcut | Context |
|---|---|---|
| Save log / Save changes | ⌘S and ⌘Return | Day page |
| Jump to next / previous section | ⌥⌘↓ / ⌥⌘↑ | Day page (Tab inserts a tab in a text block; do not rebind it) |
| Jump to first empty section | ⌘E | Day page |
| Skip day… | ⌘⇧K | Anywhere in the main window; menu bar item too |
| Remind me later (snooze) | ⌘⇧L | When due |
| Carry over (default target) | ⌘⇧Y | Day page with Yesterday card |
| Today | ⌘1 | Main window |
| This week | ⌘2 | Main window |
| Previous / next day or week | ⌘[ / ⌘] | Day page or Weekly review |
| Search | ⌘F | Focuses sidebar search; Esc clears |
| Next / previous search result | ⌘G / ⌘⇧G | Search results |
| Copy week as markdown | ⌘⇧C | Weekly review |
| Open storage folder in Finder | ⌘⇧O | Main window |
| Settings | ⌘, | Anywhere (standard `Settings` scene) |
| Open Gloamlog | ⌘O | Menu bar popover (when focused) |
| Toggle sidebar | ⌃⌘S | Standard `NavigationSplitView` |
| Close window | ⌘W | Standard |
| Hide / Quit | ⌘H / ⌘Q | Standard |

Notes:
- No global (system-wide) hotkey in v0.2. It needs Carbon or a third-party framework and Accessibility prompts. Use the menu bar icon.
- ⌘Return inside a text block does not insert a newline (it saves); plain Return does insert one.
- No conflicts with system shortcuts; ⌘E (Use Selection for Find) is rarely relied on in this app, kept for "next empty section" because that is the dominant action.

---

## 9. Accessibility requirements

### 9.1 VoiceOver labels (explicit; v1 had none)

| Element | Label / value / hint |
|---|---|
| Text block | Label: section title ("What I did"). Value: current text. Hint: "Required. Text area." Placeholder text exposed as the hint, not read as content. |
| "Nothing to report" | `Insert "Nothing" in What I did` |
| Save button | `Save log` · disabled hint `3 of 5 sections filled. Fill all sections to save.` |
| Status chip | Live region. Announce on change: "Draft saved" (polite, throttled to once per 10 s), "Saved" (assertive), "Couldn't save, folder unavailable" (assertive). |
| Sidebar rows | `Today, not logged, draft saved` · `Monday 5 October, logged` · `Friday 2 October, skipped, holiday` |
| Heatmap | Container: `Activity, last 12 weeks: 41 logged, 3 skipped, 4 missed`. Each cell: `Monday 5 October, logged` / `…, skipped` / `…, missed` / `…, not a workday`. Cells are reachable by rotor (list of buttons), not required to be traversed individually. |
| Streak | `12 day streak. Best 31.` |
| Banner | Role: group, label `Reminder`. Buttons carry full text including remaining snoozes. |
| Menu bar item | Accessibility title from table in 3.4; popover state line is the first element. |
| Yesterday card | Label `Yesterday's plan`. Buttons: `Carry over to Pending / blocked`, `Hide Yesterday card`. After carry: announce `Carried over to Pending`. |
| Skip sheet | Reason chips as radio buttons (`Holiday, selected`). |
| Weekday toggles | `Monday, on` / `Monday, off`. |
| Search results | Each result: `<date>, <section>, <snippet>`; match highlighting is visual only. |

### 9.2 Focus order and focus management
- Day page order: banner buttons → Yesterday card actions → section 1 … n → Save → Skip day → Open folder. Sidebar and detail are separate regions reachable with ⌃F7 / Tab as standard.
- On opening Today after a nag or onboarding: focus the first empty section. On opening a past day by user action: focus the first block.
- After Save: focus stays where it was (the Save button), the status announcement carries the result.
- Sheets (skip, folder-change, onboarding): focus the first control; Esc cancels (except onboarding); focus returns to the invoking control on dismiss.
- Skip day undo and Carry undo are reachable by keyboard for their full 10 s window and the focus is not moved away from the card.
- Full Keyboard Access: every button, toggle, chip, heatmap cell, and menu item is focusable; no hover-only affordances (the "Nothing to report" link is shown on focus as well as when the block is empty).

### 9.3 Reduced motion
- Respect `@Environment(\.accessibilityReduceMotion)`: no slide or bounce animations on banner, card collapse or sheet content. Use instant state changes or a short cross-fade (≤150 ms) where meaning would otherwise be lost.
- Dock bounce is a system request; skip it when Reduce Motion is on.
- The streak number change and heatmap cell transitions are instant under reduced motion.

### 9.4 Contrast and visual
- All text ≥ 4.5:1 in light and dark. Placeholders use `.secondary`, not `.tertiary` (v1 gap), and are verified against the 5% fill. **[Verify]** with a contrast checker in a running build.
- Non-text UI (heatmap cell borders, block borders, focus rings) ≥ 3:1. Heatmap states differ by shape/pattern, not hue alone: filled square, hatched (skipped), bordered empty (missed), faint dot (non-workday), ring (today).
- Block fill is supplemented by a 1 pt border when Increase Contrast is on (`accessibilityContrast`), and by a stronger focus ring always.
- Colour is never the only carrier: status chips pair an icon shape with text; the error state uses text plus `exclamationmark.triangle`, not just red.
- Do not hide the system focus ring.
- Honour Reduce Transparency: no vibrancy dependence for text legibility.

### 9.5 Other
- No time-limited UI except the 10 s undo for carry-over and skip; both also have a permanent route (re-open card, `Undo skip` page).
- Notifications use standard actions, readable by VoiceOver, with no content from logs.
- All buttons ≥ 24 pt tall (macOS control sizes), sidebar rows ≥ 28 pt.
- Hit target for heatmap cells is 14 pt (11 pt cell + padding), acknowledged as small; they are never the only route to a day.

---

## 10. Standard-SwiftUI feasibility (no Xcode, no macros)

| Need | API | Notes |
|---|---|---|
| Menu bar item | `MenuBarExtra(isInserted:)`, `.menuBarExtraStyle(.window)` | macOS 13+. Icon state via a computed `systemImage`. |
| Settings | `Settings { TabView … }` scene | macOS 13+; ⌘, is automatic. |
| Search | `.searchable(text:)` | macOS 13+ on `NavigationSplitView`. |
| Notifications | `UNUserNotificationCenter` + `UNCalendarNotificationTrigger` + actions | **[Verify]** works for an ad-hoc-signed app. |
| Launch at login | `SMAppService.mainApp` | Already used in v1; add `.status` display. |
| Folder choice | `NSOpenPanel` | App is not sandboxed (ad-hoc), so a plain path in `UserDefaults` suffices. |
| Drafts | `Timer`/`DispatchWorkItem` debounce in `DayModel` (`ObservableObject`) | Flush in `applicationWillTerminate`, `NSWindow.willCloseNotification`, `NSApplication.didResignActiveNotification`. |
| Day rollover/clock | `NSCalendarDayChanged`, `NSSystemClockDidChange`, `NSSystemTimeZoneDidChange`, `NSWorkspace.didWakeNotification` | Notification Center observers in `AppDelegate`. |
| Section focus | `@FocusState` with an enum is a property wrapper, not a macro | Fine. Menu commands call through `focusedValue` (declared via `FocusedValueKey`). |
| Reorder | `List` + `.onMove` | macOS standard. |
| Split-button carry-over | `Menu { … } label: { … } primaryAction: { … }` | macOS 13+. |
| Heatmap | `Grid` / `LazyHGrid` with `RoundedRectangle` + `.accessibilityElement` | `Grid` is macOS 13+. |

---

## 11. Explicitly out of scope for v0.2
Global hotkey; per-section "optional" toggles; custom nag cutoff and re-open interval settings; full-text index file; tags; export formats beyond markdown; iCloud conflict resolution beyond the single-day prompt in 6.5; per-day-of-week reminder times; Notion export; notarisation.

## 12. Open questions
- [ ] Nag cutoff at 23:00 is a constant. Should it be a setting, or should Strict ignore cutoffs entirely ("until saved" literally)?
- [ ] Default strictness for brand-new users: Strict (product premise, preselected here) or Gentle (safer for strangers)?
- [ ] Is the 2-snoozes/day cap right? Tune after a week of real use.
- [ ] Skip range (3.5) worth shipping in v0.2 or cut to single day?
- [ ] Should today count in the streak when logged late (after midnight, day before)? Currently yes, it counts for the day the file is named after.
- [ ] Draft location outside the folder means a second Mac does not see an unfinished draft. Acceptable?
- [ ] Menu bar-only mode (hide Dock icon) deferred. Needs `NSApp.setActivationPolicy`.

## 13. Acceptance checks (UX, for the first run-through of v0.2)
1. Type 3 sections, force-quit the app, relaunch: text is back with "Draft restored".
2. Skip a Wednesday, log Thursday: streak continues; heatmap Wednesday hatched.
3. Strict at 4:55 pm with another app frontmost: window appears and activates once; close it: it re-opens after 5 min without stealing keystrokes; third re-open shows "Not today? Skip day."
4. Gentle: notification at 4:55 pm only; no window movement all evening.
5. Change folder to an iCloud Drive folder with 3 existing logs: nothing overwritten, conflicts listed.
6. Rename "To do next" to "Tomorrow": old logs still load into the renamed block.
7. Make the folder read-only: banner appears, text stays as draft, nothing lost, copy-my-text works.
8. VoiceOver walk of Today, menu bar popover, heatmap, Settings: every control has a label from 9.1; no unlabeled buttons.
9. Reduce Motion on: no animated transitions; Increase Contrast on: block borders visible.
10. Sleep through 4:55 pm, wake at 5:30 pm: exactly one fire; none after 23:00.
