# Gloamlog: Swift to Go + Wails + React/TS parity checklist

Written 2026-10-18 from the code at v0.6.6 (`CFBundleShortVersionString` 0.6.6, HEAD 0561304). **The code wins over the docs.** Docs read: README, CHANGELOG, PRIVACY, docs/EDITOR_CONTRACT, docs/INSTALL, docs/v1/* (headers), docs/FILE_STRUCTURE, docs/ARCHITECTURE. Code read: all of `app/Core/**`, all non-view logic in `app/UI/**` (AppModel*, DayEditor, EditorBridge*, controllers, Settings panes, Sync, Updater, HotKey, Notifier, ClosedAppReminders, ClaudeSetup, Sources*), `app/Tools/**` headers, `app/build.sh`, `install.sh`, `scripts/*`, `tests/*`, `editor-web/` build + contract.

Conventions: `Core/X` = `app/Core/X.swift`, `UI/X` = `app/UI/X.swift`. "Day key" = `YYYY-MM-DD`. Checkbox = a thing the port must do or consciously drop. `[sec]` marks safety/data-loss relevant items.

Doc staleness (do not use as spec): `docs/PRD.md`, `TRD.md`, `UX_SPEC.md`, `ARCHITECTURE.md`, `ROADMAP.md` and most of `DESIGN_SYSTEM.md` describe the v0.1/0.2 five-box app (marked "Superseded"). `docs/FILE_STRUCTURE.md` is mostly current. `docs/v1/*` are plans; several planned items were NOT built (schemaVersion in settings, SQLite index, LaunchAgent helper, nested settings groups). Disagreements found are listed in section 7.

---

## 1. On-disk contract

### 1.1 Locations

| What | Path | Created by | Notes |
|---|---|---|---|
| Notes folder (user-configurable) | default `~/Gloamlog/` (`Settings.defaultFolder`, `Core/Settings.swift`) | `LogStore.ensureFolder` (onboarding / Choose folder / Create it again) | Plain files; may live in iCloud Drive / Dropbox / OneDrive / network volume. |
| Day files | `<notes>/YYYY-MM-DD.md` | `LogStore.save/skip` | One per day, UTF-8. Only names matching exactly `^\d{4}-\d{2}-\d{2}\.md$` are days (`LogStore.dayFiles`). |
| Images | `<notes>/assets/<YYYY-MM-DD>-<8 hex>.<ext>` | `AssetStore.save` | Referenced from markdown as `![](assets/2026-10-06-1a2b3c4d.png)` (relative to notes folder). ext in png/jpg/jpeg/gif/webp, max 20 MB. 8 hex = start of SHA-256 of bytes; on collision with different bytes uses 12, 16, 24, 64 hex. |
| Safety copies (version history) | `~/Library/Application Support/Gloamlog/backups/<YYYY-MM-DD>/<yyyyMMdd-HHmmss>.md` (UTC stamp) | `LogStore.backUpCurrent` | Outside the notes folder so they never sync. Newest 10 per day. See 1.6. |
| Conflict copies kept | `~/Library/Application Support/Gloamlog/backups/conflicts/<yyyyMMdd-HHmmss>-<orig name>` (+ `-2-`, `-3-` for same-second) and `...-<day> (this Mac).md` | `ConflictFiles.keep` | UTC stamp. Never deleted by the app. |
| Capture journal | `~/Library/Application Support/Gloamlog/capture/journal.jsonl`, mode 0600, dir 0700 | `CaptureJournal` | Sibling of `backups/` (`AppModel.init`: `backupDir.deletingLastPathComponent()/capture`). See 1.7. |
| Claude inbox | `~/Library/Application Support/Gloamlog/inbox/<epoch-ms>-<8hex>.json` | `gloamlog-mcp.py` | See 1.8, 2.1. |
| App preferences | `UserDefaults.standard`, domain `io.github.dasariprashant0.gloamlog` -> `~/Library/Preferences/io.github.dasariprashant0.gloamlog.plist` | `Settings.save`, AppModel | See 1.11. |
| Old-name migration sources | defaults domain `local.dailylog.app`; folder `~/Library/Application Support/Daily Log/` | n/a | `Core/LegacyMigration.swift`; see 1.11. |
| Temp files | `.gloamlog-tmp-<pid>-<uuid>` in the target's folder (or backups root for backups) | `AtomicFile.write` | Hidden, swept at first write of a store if the pid is dead. Never listed. |
| Claude Code integration files | `~/.claude/CLAUDE.md` (marked block), `~/.claude/settings.json` (optional hook, user pastes), `~/.claude.json` (written by `claude mcp add`, not by us), `~/.claude/projects/**/*.jsonl` (read only) | `ClaudeSetup`, Sources | See section 2. |
| Bundle resources | `Gloamlog.app/Contents/Resources/{editor/index.html, claude/gloamlog-mcp.py, AppIcon.icns, THIRD_PARTY_LICENSES.md}` | build.sh | The `.py` path is recorded in the user's Claude config; see 2.1 and risk R3. |
| Legacy scripts | `~/Gloamlog/daily-log.sh`, `daily-log.py`, `~/Library/LaunchAgents/com.user.dailylog.plist` | v0.0 scripts | Obsolete (docs/ARCHITECTURE.md); no code in `app/` touches them. Drop. |

No sidecar index, no sync-state file, no DB. Search cache, sources cache, planner snapshot are in memory or UserDefaults only. Version history = `backups/`. Sync state = derived from folder listing on each scan.

### 1.2 Day file format (`Core/MarkdownFormat.swift`, `Core/Models.swift`)

Written form (always, by `MarkdownFormat.serialize`):

```
# 2026-10-14
<blank line>
<body, trimmed: no leading blank lines, no trailing whitespace>
<single trailing \n>
```

- No front matter. No metadata. The only machine line is the first line `# YYYY-MM-DD`.
- `body` is free-form GitHub-flavoured markdown (the editor writes ATX headings, `-` bullets, `1.` lists, `- [ ]`/`- [x]`, `---`, fenced code, `**bold**`, `*italic*`, `~~strike~~`, `![alt](assets/x.png "caption")`, GFM tables, hard break = `\` + newline; see docs/EDITOR_CONTRACT.md "Editor bundle").
- Parse (`MarkdownFormat.parse`): strip leading `U+FEFF`; CRLF to LF; drop leading blank lines; if first line trimmed is `# ` + well-formed date (any date, not necessarily the file's) drop it; `trimBody`; then skip-marker check; then `unescapeLegacy` (a line starting with one or more `\` followed by `## ` loses one backslash, outside fenced code). Nothing is escaped on save.
- Empty/whitespace-only body on save = the file is deleted (after a backup), unless the existing file is a skip marker (left alone). `save()` never creates a file for an empty body. Identical serialised text is not rewritten.
- Skip marker file: `# 2026-10-14\n\n> Skipped: Holiday\n` or `> Skipped` (no reason). Recognised only when it is the whole body (single line), text after `> Skipped` is empty or starts with `:`. Reason: newlines to spaces, trimmed. `> Skipped the meeting` in longer text is just text.
- v0.1/v0.2 files (`## 📝 What I did` ... emoji headings) load as ordinary markdown; heading matching ignores emoji/punctuation (`normalizeHeading`).
- File perms of an existing day file are copied onto the replacement (`AtomicFile`).
- Day-key validation (`DayKey.isWellFormed`): exactly 10 bytes, digits and `-` at 4 and 7; calendar validity is checked separately (`DayKey.date`). Opening range in UI: `2000-01-01`..today.
- Folder entries ignored as non-days: names starting with `.`, `assets`, the backups folder if inside notes folder, anything not `YYYY-MM-DD.md` (counted as "not recognised" in Settings > Storage).

Synthetic example of a normal day (default template, Claude entry, jot, carry-over, suggestion bullets):

```markdown
# 2026-10-14

## Carried over from Tue 13 Oct

- [ ] Reply to Ana about the invoice
- [ ] Review pricing PR

## What I did

- 09:12 Called Sam about the migration
- 14:35 Fixed the card-order bug by sorting before the merge. Open: add a regression test.
- Committed: Add pricing table (acme-site 3f2a9c1)
- Pull request: https://github.com/acme/site/pull/42 (acme-site)

## Finished

Shipped the pricing table.

## Started

## Pending / blocked

- Waiting on legal

## To do next

- [ ] Add regression test for card order
```

Where the entries come from:
- **Jots** (hotkey, panel, menu-bar field, Claude `log_work`): `- HH:mm text` or `- [ ] text` appended at end of the **Jots block** (see 1.3). Heading text = `Settings.capture.jotsHeading` (default `Jots`; `ClaudeSetup.connect` rewrites a default `Jots` to `What I did`).
- **Claude entry**: identical to a jot with `source:"claude"`. The inbox JSON `ts` is ignored; the line's time stamp and day are taken when the app drains the inbox (see risk in section 7).
- **Git/Claude Code suggested bullets and auto-add**: markdown list items under heading `Settings.sources.heading` (default `What I did`), text from `ActivityDigest.markdown` (backslash-escapes `\ ` * _ [ ] < > ~ | & #`). Auto-add writes via `CapturePlacement.apply` with no time stamp.
- **Carry-over**: `## Carried over from <EEE d MMM>` + `- [ ] item` lines inserted at the **top** of the body (`MarkdownBody.insertCarryOver`).
- **Keep both** conflict merge: base text kept exactly, then `\n\n## From another copy\n\n` + the other copy's lines that are not in the base (`Core/Sync/BothVersions.swift`).

### 1.3 Markdown semantics the port must reproduce exactly (`Core/MarkdownBody.swift`, `MarkdownBody+Jots.swift`, `CapturePlacement.swift`)

Everything below is pure and drives streaks, reminders, catch-up, reviews and capture. Port as one Go package with golden tests generated from the Swift implementation (section 5).

- **Line scanner** (`scan`): kinds blank / heading / fence / code / text. ATX headings only (`#`..`######`, up to 3 leading spaces, `#tag` is not a heading, optional closing `#`s stripped). Fences ``` or ~~~ (>=3, closing >= opening length, backtick fence info may not contain a backtick). HTML comments `<!-- -->` stripped from text/heading lines, multi-line aware, not inside fences; `\<!--` (backslash-escaped, odd count) does not open a comment.
- **Word counting** (`words`, `tokenWords`): token = whitespace-delimited chunk with >= 1 letter/number (Unicode general categories L*, Nd, Nl, No). Ignored: heading lines, fence lines (code between counts), blank, empty list/task markers, `![..](..)` / `![..][..]` images, HTML tags `</?[A-Za-z][A-Za-z0-9-]*(\s[^<>]*)?/?>`, HTML comments. Stripped before counting: blockquote `>` (any nesting), one list marker (`-*+` or `\d{1,9}[.)]`), task box `[ ]`/`[x]`/`[X]`. CJK (Han, Hiragana, Katakana ranges in `isCJK`) run counts `ceil(len/2)` words; other no-space scripts count per chunk.
- **Carried-over exclusion**: task lines (`checked != nil`) under a heading whose normalised title is `carried over from` or starts with it (any level; block ends at next heading of same/higher level; deeper sub-headings stay inside) never count; non-task text there counts.
- **Jots block** (`jotsBlocks`): heading whose `normalizeHeading` equals `normalizeHeading(jotsHeading)` (empty/emoji-only heading means `jots`), any level, ends at next heading of same or higher level. Every Jots block counts as jots. Jot words excluded from "logged" unless `jotsCountTowardLogged`; a leading `H:mm`/`HH:mm` (00:00..23:59, 2-digit minutes, then space/end) is never a word. `jotCount` = list items indented < 2 spaces (tab = 4).
- **Status** (`DayDocument.status(minWords)`): skip marker -> skipped; `w = words + (countJots ? jotWords : 0)`; `w >= max(1,minWords)` logged; `w > 0 || jotCount > 0` partial; else missed (= no log).
- **`hasContent`**: any content line (not blank/heading/fence, after marker strip non-empty); an image or carried-over block alone counts.
- **`normalizeHeading`**: keep letters, numbers, combining marks (except variation selectors FE00-FE0F, E0100-E01EF); everything else collapses to one space; lowercased.
- **`sections`, `partition`, `shiftHeadings`, `pruneEmptySections`, `trimBody`, `itemKey`, `plainText`, `removeNoise`**: used by search, week/month review, exports, carry-over. Semantics in file header comments; `trimBody` = CRLF to LF, drop leading blank lines, strip trailing whitespace (keeps first-line indentation).
- **`CapturePlacement.render/apply/contains/key/jotKeys/lastJotKey`**: escaping rules for note text (heading/blockquote/bullet/ordered/setext/thematic/fence at line start get a backslash; `<!--` becomes `<\!--`), placement at end of first Jots block (joined without blank line when it ends in a list, else after one blank line; no block: trimmed page + blank + `## <Jots heading>` + blank + line), closing an unclosed fence/comment first, idempotent replay via `key` (list marker, task box, escapes, soft/hard breaks flattened, whitespace collapsed; case and time are part of identity). Multi-line note = one item, continuation lines indented 2 spaces, blank lines inside a note dropped.
- **`stripUntouchedTemplate`**: a page that is exactly the template (line by line, blanks ignored) followed by only a Jots block is written back as just the Jots block (template is editor-only display).
- **Carry-over** (`Core/CarryOver.swift`): source = most recent previous day (14-day lookback) that is **logged** (not skipped, not partial); items = non-empty, non-code, unticked lines under `carryOverHeadings` (matched by `normalizeHeading`, sub-headings included), markers stripped, images/tags removed, de-duplicated by `itemKey`; label `EEE d MMM`; heading text `Carried over from <label>`; heading word in strip: `Yesterday` / weekday name (<=6 days) / `d MMM yyyy`. Apply is idempotent (same heading at any level, or all items already present).
- **Default template** (`Settings.defaultTemplate`, byte-exact): `## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next`. Presets (`UI/TemplateEditor.swift`): Daily review (default), Standup `## Yesterday\n\n## Today\n\n## Blockers` (carry headings `Today`,`Blockers`), Blank (empty).

### 1.4 Day status, streak, catch-up (`Core/Streak.swift`, `Calendar/*`)

- Statuses: logged, partial, skipped, missed, off, future. `Status.resolve`: day > today = future; has file state = that; day < `since` = off (since = `logStartDate` else earliest day with a file, else today); scheduled weekday = missed else off.
- `Settings.weekdays` uses Calendar numbering 1=Sunday..7=Saturday; default `{2,3,4,5,6}`.
- Streak: from `since` to today; logged +1; skipped neutral; unlogged today neutral; unlogged/partial past scheduled day resets; non-scheduled neutral; best tracked; cap 40000 steps.
- Heatmap: 12 columns (weeks, oldest first, first weekday per calendar), 7 cells each; marks: filled square logged, half disc started, amber ring not logged, dash skipped, faint dot non-workday, accent ring today, blank future.
- Catch-up list: scheduled days in last N days (N clamped 7..365, default 30) strictly before today, on/after log start, status missed or partial, oldest first. Days still downloading (`sync.isBlocked`) are excluded.
- `pinLogStart(before:)`: first save/skip on a day earlier than the earliest page pins `logStartDate` to the old earliest page (only while user never set one).
- MonthGrid: always 6x7, columns start at `calendar.firstWeekday`, cells outside month are real days.
- DateJump grammar (`Core/Calendar/DateJump.swift`): `today`, `yesterday`, `-3`, `3 days ago`, `2 weeks ago`, weekday (`fri` = most recent on/before today; `last fri` = strictly before), `2 oct`, `oct 2`, `2nd October 2026`, `2026-10-02` (also `/` `.`), `10/2`, `10/2/26`, `10/2/2026` (field order from `calendar.locale`, nil/root = en_US). Never future, never before 2000-01-01, impossible dates nil, `tomorrow`/future ISO give "hasn't happened yet".

### 1.5 Reminders state and rules (`Core/ReminderPlanner.swift`, `Schedule/ReminderScheduler.swift`)

- Nag window: scheduled weekday, `reminderMinutes <= now < 23:00` local, today status missed or partial. Strict: first fire = front window + notification, snooze up to 2/day (snooze then re-front without second notification), reopen (orderFront, no activate) when window closed >= 5 min and >= 5 min since last fire, skip hint after 3 reopens. Gentle: one notification, unlimited snoozes. Grace: 120 s after launch, 60 s after settings change, 120 s after undo skip.
- Closed-app plan: next 14 scheduled days, each a calendar-trigger local notification `id = gloamlog.reminder.<day>` at `reminderMinutes`; none for logged/skipped days, days before log start, today if time passed; snooze id `gloamlog.snooze.<day>`; category `reminder` with actions Open / Snooze N min / Skip today; copy title `Today isn't logged yet`, body `It's <h:mm am/pm>. Open Gloamlog when you're ready.` (never page text, streak or names). In-app copy (`AppModel.postReminder`): within 5 min of the reminder time `Time to write up your day` / `It takes a few minutes. Gloamlog is ready when you are.`; later `Today isn't logged yet` / `It's <time>. Open Gloamlog whenever you're ready.` (scheduled copy says `when you're ready`).

### 1.6 Version history / backups (`Core/LogStore.swift`)

- Before `save()` overwrites OR deletes an existing day file, the old bytes are copied to `backups/<day>/<yyyyMMdd-HHmmss>.md` (UTC, via AtomicFile with tempDir = backups root). Keep newest 10 per day (`maxBackupsPerDay`). Names always increase (same-second or clock-back bumps stamp to newest+1 s).
- No backup when: new text equals file, file is a skip marker, file has only its title / empty body, backups disabled (no backupDir).
- `minBackupInterval` = 120 s in the app for ordinary autosave overwrites (skips the copy if newest backup is younger); deletes, restores, conflicts and "changed elsewhere" saves force a copy (interval set to 0 around the call).
- Backup failure never fails the save; surfaced as `SaveOutcome.backupWarning` / `lastBackupWarning` ("Earlier versions couldn't be kept: ...").
- `restoreBackup`: backs up current file first (throws `backupFailed`, nothing changed, if impossible), then writes the old copy back. `listBackups` newest first with word count (jots included) and a 120-char plain preview in the sheet.
- Backups are never listed in `listDays`, search, summaries.

### 1.7 Capture journal (`Core/Capture/CaptureJournal.swift`)

`journal.jsonl`, one JSON object per line, UTF-8, keys sorted, slashes unescaped:
```
{"day":"2026-10-14","id":"8E0B...UUID","source":"hotkey","stamp":"14:32","text":"call Sam","todo":false,"ts":"2026-10-14T12:32:01.250Z","v":1}
{"ack":"8E0B...UUID"}
```
- `ts` ISO-8601 UTC with whole milliseconds (hand-formatted); reading tolerates no millis, a numeric epoch, missing `v`/`todo`, unknown keys, torn last line, garbage lines, duplicate ids (first wins); a record needs usable id/day (well-formed)/text/ts. An ack is any object with string `ack`, anywhere in the file.
- Append = `open(O_WRONLY|O_APPEND|O_CREAT, 0600)`, write one line, `F_FULLFSYNC` (the durability point); if file does not end in `\n` start on a fresh line; failed write is `ftruncate`d back; first record also fsyncs the directory. Dir created 0700; file tightened to 0600 if wider.
- `compact()` at launch (first `replayJournal`): rewrite with raw lines of pending items only, via AtomicFile, no-op if unchanged.
- `CaptureItem` fields: id (UUID string), ts, day (day of capture time, never changes), text (trimmed, control chars removed except `\n`, CR/CRLF/NEL/LS/PS to `\n`, tab/VT/FF to space, max 4000 chars), stamp (`HH:mm` of capture time when timestamps on and not a to-do), todo (text started with `[]` or `[ ]` and `todoShorthand` on; marker removed), source (`hotkey` | `menubar` | `menu` | `claude`).

### 1.8 Claude inbox (`Core/Capture/Inbox.swift`, `Resources/claude/gloamlog-mcp.py`)

`inbox/<epoch-ms>-<uuid8>.json`, written hidden (`.XXXX.tmp`) then `os.replace`d:
```json
{"text": "Fixed the card-order bug by sorting before the merge.", "ts": "2026-10-14T14:35:00Z"}
```
Reader: `*.json` (non-hidden), oldest mtime first (name tiebreak), max 50 per look, max 64 KiB each, must be a JSON object with non-empty trimmed string `text`; invalid files are skipped and never deleted. After the entry is journaled as a jot (`source:"claude"`) the file is deleted. `GLOAMLOG_INBOX` env var overrides the directory (writer only).

### 1.9 Sync-related names (`Core/Sync/FolderEntry.swift`)

Recognised in the notes folder (everything else ignored):
- `2026-10-08.md` day. `.2026-10-08.md.icloud` / `.2026-10-08 2.md.icloud` iCloud placeholder.
- Conflict copies: `2026-10-08 2.md` (iCloud/Finder, N>=2, <=3 digits), `2026-10-08 (conflicted copy).md` and `2026-10-08 (<Name>'s MacBook's conflicted copy 2026-10-09).md` (Dropbox: parenthesised, contains `conflicted copy`), `2026-10-08.sync-conflict-20261009-101530-ABCDEFG.md` (Syncthing).
- Month must be 01-12 and day 01-31 (calendar not consulted). Dataless = `st_flags & 0x40000000` (SF_DATALESS) on the day file.
- `LogStore` refuses read/write/skip/unskip/restore of a not-local day (`LogError.notLocal`).
- `folderStamp()`: sorted `name mtime.sec.nsec size[ dataless]` rows of day files; used to decide whether to re-read (readdir + fstatat only).

### 1.10 Generated text formats (byte-level contract for copies/exports)

- Week markdown (`WeeklyReview.markdown`, `ReviewFormats.markdown`): `# Week of 5 Oct – 11 Oct 2026` (en dash, `dash:true`), blank, counts line `Logged 4 of 5 · 1 skipped · 2 not logged` (middle dot), optional tasks line `12 tasks done · 3 still open`; by Day: `## Mon 5 Oct` + body headings shifted to >= level 3, skipped = `## Mon 5 Oct\n_Skipped: reason_`; by Section: `## <Heading>` + `- **Mon 5 Oct**: first line` + continuation lines indented 2; Year: `- **October**: 20 logged · 1 skipped · 2 not logged`; empty: `_Nothing written in this period._`. Output ends with one `\n`.
- Titles/file names (`ReviewFormats.title/fileName`): `Week of 5 to 11 Oct 2026`, `September 2026`, `2026`, `5 Sep to 12 Oct 2026`; file `Gloamlog 2026-W41` (ISO week of the week's mid day), `Gloamlog 2026-10`, `Gloamlog 2026`, `Gloamlog 2026-09-05 to 2026-10-12`.
- Export bundle (`ExportBundle`): `<name>.md` + `assets/` containing only used images (links rewritten to `assets/<file>`, name clashes get `2-` prefix), layouts file / folder (`<name>`, `<name> 2`...) / zip via `/usr/bin/ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl`. Result line `Saved 5 days and 2 images.`
- Rich copy: pasteboard `.rtf` (if < 200 KB) + `.html` + plain `.string` (the markdown). Plain copy: `MarkdownHTML.plain` (no `#`/`**`/backticks; headings become `Title:` lines; links `text (url)`). PDF: print-CSS HTML with strict CSP, rendered by WKWebView print operation to PDF (fallback `createPDF`), 30 s watchdog, images embedded as data: URIs.
- Date formats (all `en_US_POSIX` names, in the app calendar's zone): `EEE d MMM` (Mon 5 Oct), `EEEE, d MMMM yyyy` (page header), `d MMM yyyy`, `MMMM yyyy`. Times: `DateFormatter` short style lowercased (`4:55 pm`, or `16:55` on 24 h Macs). Jot stamp is always 24 h `HH:mm`.

### 1.11 Preferences (UserDefaults) with types and defaults

Everything is stored under the app's defaults domain. The main blob is JSON `Data` (Swift `JSONEncoder` defaults) under key **`dailylog.settings.v2`** (name kept from v0.2 on purpose). Decoding is tolerant: any missing key takes its default; old `sections` array with no `template` becomes `## <title>` blocks; unknown keys ignored; a damaged `sources` value never loses other keys. `normalized()` clamps on load and save.

| JSON key (`Settings`) | Type | Default | Clamp / notes |
|---|---|---|---|
| `reminderMinutes` | Int | 1015 (16:55) | 0...1439 |
| `weekdays` | [Int] (Set) | [2,3,4,5,6] | filter 1...7; empty resets to Mon-Fri; array order not stable |
| `mode` | "strict" \| "gentle" | strict | |
| `snoozeMinutes` | Int | 15 | must be in {5,10,15,20,30} else 15 |
| `storageFolder` | URL as string | `file:///Users/<u>/Gloamlog` (trailing `/` when the dir exists) | Swift encodes `URL.absoluteString`, with `\/` escaping |
| `launchAtLogin` | Bool | true in struct; forced false until `onboarded` | UI state mirrors `SMAppService.mainApp.status` (enabled or requiresApproval) |
| `template` | String | default template 1.3 | |
| `carryOverHeadings` | [String] | ["To do next","Pending / blocked"] | trimmed, empties dropped |
| `minWords` | Int | 20 | 1...500 |
| `onboarded` | Bool | false | |
| `logStartDate` | String? day key | absent | dropped if malformed; omitted when nil |
| `appearance` | "system"\|"light"\|"dark" | system | |
| `weekStart` | Int? 1...7 | absent = follow system | |
| `catchUpWindowDays` | Int | 30 | 7...365 |
| `capture.hotKey` | {keyCode:UInt32, carbonModifiers:UInt32}? | {38, 6144} (⌃⌥J: controlKey 0x1000 \| optionKey 0x0800) | **Bug:** `nil` (= Off) is omitted by the synthesized encoder and a missing key decodes as the default, so "Off" does not survive a relaunch. Port: write explicit `null`, read missing as default (compatible with existing files), read `null` as Off. |
| `capture.timestamps` | Bool | true | |
| `capture.todoShorthand` | Bool | true | |
| `capture.jotsHeading` | String | "Jots" | applied process-wide via `LogStore.jotsRules` |
| `capture.jotsCountTowardLogged` | Bool | false | |
| `closedAppReminders` | Bool | true | |
| `sources.claude` | Bool | false | |
| `sources.git` | Bool | false | |
| `sources.consented` | Bool | false | consent sheet accepted |
| `sources.allowed` | [String] abs paths | [] | ticked roots; with `allProjects` it holds the SKIPPED roots |
| `sources.allProjects` | Bool | false | |
| `sources.autoAdd` | Bool | false | |
| `sources.gitRoots` | [String] | [] | |
| `sources.heading` | String | "What I did" | trimmed; empty falls back to default |

Example blob (Swift key order not significant):
```json
{"reminderMinutes":1015,"weekdays":[2,3,4,5,6],"mode":"strict","snoozeMinutes":15,"storageFolder":"file:\/\/\/Users\/me\/Gloamlog\/","launchAtLogin":false,"template":"## What I did\n\n## Finished\n\n## Started\n\n## Pending \/ blocked\n\n## To do next","carryOverHeadings":["To do next","Pending \/ blocked"],"minWords":20,"onboarded":true,"logStartDate":"2026-09-01","appearance":"system","catchUpWindowDays":30,"capture":{"hotKey":{"keyCode":38,"carbonModifiers":6144},"timestamps":true,"todoShorthand":true,"jotsHeading":"Jots","jotsCountTowardLogged":false},"closedAppReminders":true,"sources":{"claude":false,"git":false,"consented":false,"allowed":[],"allProjects":false,"autoAdd":false,"gitRoots":[],"heading":"What I did"}}
```

Other keys in the same domain:

| Key | Type | Default | Meaning / file |
|---|---|---|---|
| `dailylog.planner.v1` | JSON Data `PlannerState` {day:String, snoozesUsed:Int, snoozedUntil:Date?, firstFired:Bool, lastFire:Date?, reopenCount:Int, hiddenSince:Date?, graceUntil:Date?} | absent | **Dates are Swift default `deferredToDate` = Double seconds since 2001-01-01 UTC** (unix 1790000000 encodes 811692800). On launch `atLaunch` keeps only `snoozesUsed` if same day, sets 120 s grace. (`Core/ReminderPlanner.swift`, `UI/AppModel.swift`) |
| `dailylog.whatsNew.0.6` | Bool | absent | "What's new in 0.6" sheet seen (set also by onboarding finish) |
| `dailylog.catchNoticeDay` | String day key | "" | Today notice dismissed for that day |
| `dailylog.settingsPane` | String (general/reminders/page/storage/shortcuts/sources/about) | general | last Settings pane |
| `dailylog.sources.hiddenDays` | [String] | [] | "Not today" for suggestions, last 90 kept |
| `showMenuBar` | Bool | true (absent = true) | menu-bar item inserted |
| `yesterdayExpanded` | Bool | false | From yesterday strip collapsed state |
| `suggestionsExpanded` | Bool | true | Suggested from your work strip |
| `review.scale` | "week"\|"month"\|"year"\|"range" | week | `UI/ReviewController.swift` |
| `review.skipped` | Bool | true | Include skipped days |
| `review.jots` | Bool | false | Include Jots |
| `review.grouping` | "day"\|"section" | absent = scale default (Day for week, Section otherwise) | |
| `remindMinutes` | Int | absent | v1 (pre-0.2) legacy: if present and no `dailylog.settings.v2`, `reminderMinutes` is taken from it and `onboarded=true` |

Legacy migration (`Core/LegacyMigration.swift`, real app only): copy from defaults suite `local.dailylog.app` every key with prefix `dailylog.` plus `showMenuBar`, `yesterdayExpanded` when the new domain lacks it; move `~/Library/Application Support/Daily Log` to `.../Gloamlog` only when the new folder does not exist. Never deletes or overwrites. Idempotent; runs every launch.

Port decision needed (open question Q1): keep reading `UserDefaults`/plist (binary plist via `howett.net/plist`, cfprefsd write caching) vs. one-time import to a Go-owned JSON config. If the Wails app uses a different bundle id it will not see this domain; if the same id, it can share. Either way keep the exact key names and encodings above for import.

### 1.12 Dates and time zones

- All "day" logic uses `Calendar.current` (system zone, locale) with `firstWeekday` overridden by `Settings.weekStart` (`AppModel.makeCalendar`). Day arithmetic is calendar-based (`date(byAdding: .day)`), never +86400 s: DST-safe, including zones with midnight DST (tested).
- UTC: backup file names, conflict copy stamps, journal `ts`, inbox `ts` (ignored), Claude transcript `timestamp` (`...Z`), update-check nothing.
- Local with offset: git `--since/--until` formatted `yyyy-MM-dd'T'HH:mm:ssZZZ` in the calendar's zone (since = day start - 2 days, until = day end + 1 day; then cut to the day by commit time).
- Observers re-run `refreshClock` on `NSCalendarDayChanged`, `NSSystemClockDidChange`, `NSSystemTimeZoneDidChange`.
- Notification triggers are year-month-day-hour-minute components resolved by macOS in the Mac's zone.

---

## 2. Integrations

### 2.1 Claude Code writes entries: MCP server `gloamlog`, tool `log_work`

Mechanism: a ~70-line **stdio MCP server in Python 3 (stdlib only)** that ships in the app bundle at `Contents/Resources/claude/gloamlog-mcp.py` (source: `app/Resources/claude/gloamlog-mcp.py`). It does NOT talk to the app; it drops a JSON file in the inbox folder (1.8) and the app drains it.

- Transport: JSON-RPC 2.0, **one JSON object per line** on stdin/stdout. Notifications (no `id`) get no reply. Non-JSON lines are ignored.
- `initialize` -> `{"protocolVersion": <echo of client's params.protocolVersion, default "2024-11-05">, "capabilities": {"tools": {}}, "serverInfo": {"name": "gloamlog", "version": "1.0"}}`.
- `tools/list` -> one tool:
```json
{"name":"log_work",
 "description":"Write one entry in the user's Gloamlog work journal. Call it once when a meaningful piece of work is done (a problem solved, a deliverable finished, a decision made, or when the user says they are done), not after every step. Write 1-3 short plain sentences in the user's voice: the problem being solved, what was done, and what is still open. No secrets, tokens, customer data, file dumps or code.",
 "inputSchema":{"type":"object","properties":{"summary":{"type":"string","description":"The entry, 1-3 short sentences."}},"required":["summary"]}}
```
- `tools/call` name `log_work`, arguments `{"summary": "..."}`: text is whitespace-collapsed (`" ".join(split())`), cut to 1500 chars; empty -> result text `Nothing to add.`; else file written, result `{"content":[{"type":"text","text":"Added to Gloamlog."}]}`; `OSError` -> `{"content":[{"type":"text","text":"Could not write to Gloamlog: <e>"}],"isError":true}`. Unknown tool -> error `-32602 unknown tool`. Unknown method -> `-32601 method not found`. `ping` -> `{}`.
- Inbox dir: `$GLOAMLOG_INBOX` or `~/Library/Application Support/Gloamlog/inbox`; file `<ms>-<uuid8>.json` with `{"text":..., "ts":"<UTC %Y-%m-%dT%H:%M:%SZ>"}`; temp `.<random>.tmp` then `os.replace`.
- App side (`UI/AppModel+Capture.swift drainInbox`): at launch (next main-loop turn), on every app activation, every 15 s (real app only), and on `gloamlog://refresh`. Each entry -> `jot(text:, source:"claude")` (journal first, then page) then the file is removed (even if `jot` returned nil).
- Install / configure (`UI/ClaudeSetup.swift`), triggered by Settings > Sources "Connect Claude Code" or onboarding step 4, after an NSAlert listing the three changes:
  1. `claude mcp remove gloamlog -s user` (ignore result), then `claude mcp add --scope user gloamlog -- python3 '<bundle>/Contents/Resources/claude/gloamlog-mcp.py'` (run via `/bin/zsh -lic`, a login shell, because a Dock-launched app has no PATH; `claude` found first at `~/.claude/local/claude`, `~/.local/bin/claude`, `/opt/homebrew/bin/claude`, `/usr/local/bin/claude`, else `command -v claude`). Claude Code itself writes the entry into `~/.claude.json` (user scope; not touched by Gloamlog).
  2. Add/replace a marked block in `~/.claude/CLAUDE.md` (`Core/ClaudeMd.swift`; atomic string write; file/dir created if missing):
  ```
  <!-- gloamlog:begin -->
  ## Gloamlog work log
  When a meaningful piece of work is finished in a session (a problem solved, a deliverable done, a decision made, or I say I'm done), call the gloamlog `log_work` tool once with 1-3 plain sentences: the problem I was solving, what was done, and what is still open. Not after every step. No secrets, customer data or code.
  <!-- gloamlog:end -->
  ```
  An existing hand-written section mentioning `log_work` counts as set up and is left alone; the marked block is replaced in place; `remove` strips it plus the blank line before.
  3. If `capture.jotsHeading == "Jots"` set it to `What I did`.
- Status (`currentState`): `claude mcp get gloamlog` exit 0 and output contains `gloamlog` -> `connected` if output contains the current script path else `moved` ("Needs reconnecting", Reconnect button); not found -> `notConnected`; no `claude` binary -> `noClaude` ("Claude Code not found"). States: unknown/checking/working show a spinner.
- Disconnect: `claude mcp remove gloamlog -s user` + remove marked block; failure texts: `Couldn't remove the helper from Claude Code. Run: claude mcp remove gloamlog -s user`.
- Manual variants: "Copy" buttons in Settings: setup command (`copySetupCommand`), CLAUDE.md text (`copyClaudeInstructions`), Stop hook (`copyHook`).
- Hook (optional, user pastes into `~/.claude/settings.json`; the app never edits that file):
```json
"hooks": { "Stop": [ { "hooks": [ { "type": "command", "command": "pgrep -x Gloamlog >/dev/null && open -g gloamlog://refresh || true" } ] } ] }
```
  `gloamlog://refresh` (registered as `CFBundleURLTypes` scheme `gloamlog`) makes the app call `sources.autoTick(force:true)` and `drainInbox()`. If the port renames the process, the hook's `pgrep -x Gloamlog` breaks for existing users.
- Requirement: `python3` on PATH of Claude Code's environment (macOS CLT stub). Port may replace the Python with a Go subcommand but **must keep the file path working** for already-registered users (R3).

### 2.2 Claude Code session reader (Sources > Claude Code) (`Core/Sources/ClaudeCodeSource.swift`, `JSONLReader.swift`)

Read-only, local. Used for the Suggestions strip and auto-add.
- Roots: `$CLAUDE_CONFIG_DIR` (if set, normalised) and `~/.claude`; session files = `<root>/projects/<project-dir>/<session>.jsonl` directly inside each project folder; symlinked dirs not followed; files containing `.orphaned-` or `.superseded-` skipped; hidden skipped. A file whose mtime < start of the day cannot hold that day.
- Privacy gate: first working folder (`cwd`) is read from the raw bytes of the first 512 KB (`"cwd":"` or `"cwd": "`); if the project is not ticked the file is never parsed further (cached as skipped). `ProjectPolicy`: project = `cwd` with `/.claude/worktrees/<name>` folded into its main folder; exact, case-insensitive match on the root; with `allProjects` the list is a skip list.
- Record shape assumed (private Claude format, "unverified" in code comments): JSON object per line with `type` in `user|assistant`, `timestamp` (UTC `...Z`), `message{role, content: String | [block]}`, `cwd`, `sessionId`, `isSidechain`, `isMeta`, `isCompactSummary`, `isVisibleInTranscriptOnly`. Blocks: `text`, `tool_use{id,name,input}`, `tool_result{tool_use_id,content}`. Edit tools = Write, Edit, MultiEdit (`input.file_path`), NotebookEdit (`input.notebook_path`); `Bash` `input.command`. Lines up to 8 MiB read in 4 MiB chunks; longer lines skipped; an unterminated last line left unread.
- Prompts: first line of user text with system wrappers removed (`<system-reminder>`, `<command-*>`, `<bash-*>`, `<ide_*>`, `<local-command-*>`, `<user-prompt-submit-hook>`...), placeholders `[Pasted text...]` removed, noise prefixes dropped (`Caveat:`, `[Request interrupted`, `This session is being continued`, `API Error`...), bare slash command dropped, needs >= 3 words, cut to 400; sidechain and synthetic (`isMeta`...) excluded; at most 3 session-first prompts per project per day.
- Commits: Bash command matching `\bgit\s+(...)commit(?![\w-])`; subject from heredoc or `-m/-am/--message` arg (only the text after `git commit`); result text must match `^\[branch <7-40 hex>\] subject` else the commit is marked failed and dropped; `!` shell commands in `<bash-input>` also counted. PRs: `gh pr create` / `glab mr create`, URLs matched in the result for github.com/gitlab.com/bitbucket.org pull/merge request forms (<= 5 per project).
- Per project per day: one session item (count of sessions, active seconds = sum of record-time periods with gap <= 15 min), <= 3 prompts, <= 60 files by edit count, <= 12 commits, <= 5 PRs. Format-drift notice when `records >= 30 && usable*5 < records*4`.
- Caching: in-memory per file keyed (path, mtime, size, policy key); >600 entries clears. Nothing persisted.
- Retention note: if no bullets and the day is > 30 days ago: "Claude Code no longer keeps sessions this old."

### 2.3 Git source (`Core/Sources/GitSource.swift`)

- Git binary: first executable of `/Library/Developer/CommandLineTools/usr/bin/git`, `/Applications/Xcode.app/Contents/Developer/usr/bin/git`, `/opt/homebrew/bin/git`, `/usr/local/bin/git` (never `/usr/bin/git`, the install-prompt stub). None: notice `Git isn't installed on this Mac, so commits can't be listed.`
- Repo discovery: user-added folders (`sources.gitRoots`, NSOpenPanel "Add folder…"); folder with `.git` is a repo (not descended); else walk up to 3 levels, skip hidden, symlinks, and `node_modules Library Pods venv target build dist vendor`; max 5000 visited, 400 repos. Only repos whose root is ticked (`policy`).
- Commands (no shell, `Process`, stdin/stderr null, 10 s timeout then terminate, output cap 1 MB, env exactly `PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin HOME=<home> GIT_TERMINAL_PROMPT=0 GIT_OPTIONAL_LOCKS=0 GIT_PAGER=cat GIT_CONFIG_NOSYSTEM=1`):
  - `git -C <repo> config --get user.email` (missing -> notice "Set your email with `git config --global user.email` to see your commits.")
  - `git -C <repo> -c core.fsmonitor=false log --branches --no-merges -i --fixed-strings --author=<email> --since=<ISO> --until=<ISO> --max-count=300 --format=%H%x1f%ae%x1f%ct%x1f%s`
  - then keep lines whose author email equals the user's (case-insensitive) and whose commit time is inside the day; item `commit` with 7-char sha as evidence.
- Polling: none for git itself; it runs as part of a scan (open day with a source on, `Look again`, 2-minute auto timer, `gloamlog://refresh`).
- Scan merging (`SourcesController.scan`): Git first, then Claude Code; `ActivityDigest.bullets` dedups by item id; a commit without hash is dropped if another item shows the same subject with a hash.

### 2.4 Digest, redaction, insertion (`Core/Sources/ActivityDigest.swift`, `Redactor.swift`)

- Bullet texts (exact): `Worked in <project> with Claude Code for about <dur> (<n> session(s), <m> file(s) edited)` (dur: `under a minute`, `<m> min` rounded to 5 from 10 min, `<h>h <m>m`); `Worked on <project>: <prompt line>`; `Committed: <subject> (<project> <sha7>)`; `Pull request: <url> (<project>)`. Limits: prompt 140, subject 120, name 60, URL 200; cut at a word near the end + `…`, after redaction.
- Redactor: 17 regex rules (private keys, `scheme://user:pass@`, AWS ids, GitHub/Slack/`sk-`/Stripe/Google/JWT tokens, `Bearer/Basic`, `name=value` and `name: value` secret pairs, `--password x` flags, emails -> `[email]`, long hex (not 40-char), long base64-ish) -> `[redacted]`; home dir -> `~`, other `/Users/x/` -> `~/`. Idempotent. `Redactor.summary` line `4 values hidden (2 emails, 2 tokens)`. Port the exact patterns from `Core/Sources/Redactor.swift`.
- Insert (user ticks lines): one `appendToSection` transaction on the live page under `sources.headingText` (adds the heading at the end if absent), `unlessPresent:true`, skip lines already on the page (`isAdded`: `evidence` substring of the page's plain text). Undo = editor ⌘Z.
- Auto-add (Settings "Add commits for me"): Timer 120 s (+ forced at +3 s after launch and on `gloamlog://refresh`, min 20 s between runs): only `commit` and `pullRequest` bullets for **today**; open page -> via editor; no page -> load file (skipped day: not touched), `CapturePlacement.apply` (item id new UUID, no stamp, text = escaped bullet) + `store.save`.

### 2.5 Other network and external touch points

- **Update check** (`Core/UpdateCheck.swift`, `UI/Updater.swift`): only when the user clicks Check for Updates…: GET `https://api.github.com/repos/dasariprashant0/gloamlog/releases/latest` (`Accept: application/vnd.github+json`, timeout 15 s, ephemeral session). Release JSON fields used: `tag_name` (leading `v` stripped), `draft`/`prerelease` (rejected), `html_url`, `body`, `assets[].name` / `browser_download_url` for `Gloamlog.zip` (required) and `Gloamlog.zip.sha256`. Hosts must be https and `github.com` or `*.github.com`. Details in section 6.
- No telemetry, no other network. The editor page runs under a CSP that blocks all network.
- `NSWorkspace.open` for http/https/mailto links from the editor; `x-apple.systempreferences:` deep links for notifications and Login Items.

---

## 3. Features by surface

Legend: file lists are Swift sources (`UI/` unless prefixed `Core/`). "States" = every user-visible variant to reproduce. Strings quoted are the shipped copy; keep them (voice: plain, no exclamation marks, never says "missed" in UI, uses "not logged").

### 3.A App shell, windows, lifecycle (`GloamlogApp.swift`, `AppDelegate.swift`, `AppModel.swift`, `MainView.swift`)

- [ ] Single main window titled `Gloamlog` (id `main`; the title string is how the app finds its window), default 1000x760, min 860x600, content-min resizable, title text hidden, unified toolbar; NavigationSplitView sidebar 220-280 pt (ideal 240).
- [ ] Closing the window keeps the app running (reminders and hotkey depend on it); Dock click with no visible window reopens it (`applicationShouldHandleReopen`); `⌘Q` quits.
- [ ] Quit flush `[sec]` (`applicationShouldTerminate`): ask editor for final text, save, 1.5 s ceiling; if unsaved edits remain -> alert "Your latest edits couldn't be saved." [Copy text and quit] [Don't quit] (copies `# day\n\ntext` blocks of unsaved pages).
- [ ] Dock icon menu: Today, Jot…, Catch Up, Settings….
- [ ] Standard About panel (`Check for Updates…`, `About Gloamlog`, `Save Layout Report` debug item writes `/tmp/gloamlog-layout.{txt,png}`; port may drop the debug item). Credits text: "A calm end-of-day log. Plain markdown files on your Mac. No account, no tracking. The only network request is the update check you ask for."
- [ ] URL scheme `gloamlog://refresh` (re-scan sources + drain inbox), `gloamlog://layout` (debug).
- [ ] Launch sequence: legacy migration (real app only) -> load settings -> if `onboarded` false open Onboarding sheet else if `dailylog.whatsNew.0.6` unset open "What's new in 0.6" sheet -> `start()` (notifications setup, ClosedAppReminders install, load editor with today, 30 s timer, sync start, observers, first `tick`) -> sources `startAuto()`; Settings window prewarmed hidden 1.5 s after launch; search cache warmed 3 s after launch off-main.
- [ ] 30 s tick: `refreshClock` (day rollover), `reloadIfFolderChanged` (stamp-gated), open page external-change check, retry unsaved, retry orphans, ReminderPlanner.decide + perform (skipped while screens asleep/session inactive).
- [ ] Observers: day/clock/zone change -> `refreshClock`; app activate -> reload-if-changed, external change check, retry saves, refresh notification permission, replay capture, drain inbox; app resign / window close / system sleep -> `flushEditor`; wake -> tick after 15 s, replay capture after 5 s, re-plan closed-app reminders; screens sleep/wake and session resign/become active toggle `screensAsleep` (no reminders then).
- [ ] Appearance override (System/Light/Dark) applied to every window, popover and editor (`SettingsEffects.apply`); editor theme follows effective appearance.
- [ ] Folder-health model: `folderProblem` missing/notWritable (set from checkFolder / save errors) drives banners, menu-bar icon, popover state line.
- [ ] Orphans `[sec]`: pages left with unsaved text (folder unavailable) are kept in memory, retried on tick/activate/Try again, offered via "Copy my text"; a jot for such a day joins the orphan text.
- [ ] VoiceOver announcements (`UIAnnounce.say`): "Logged for today", "Carried over from yesterday", "Catching up, 3 of 5, <date>", "All caught up", "Skipped N days.", "Added N line(s)", "Conflict settled...", "Couldn't save. Your text is still on screen." (assertive).
- [ ] Reduce-motion: skip Dock bounce (`requestUserAttention`) and panel animation; Reduce Transparency makes sidebar solid.

### 3.B Sidebar (`Sidebar.swift`, `CalendarView.swift`, `SyncChip.swift`)

- [ ] Search field `Search your logs` (⌘F focuses; Esc/clear button clears) with live results; typing switches the detail to Search, clearing returns to previous destination.
- [ ] Rows: Today (glyph per status: checkmark.circle.fill logged, minus.circle skipped, moon.zzz not a workday, half circle started, circle not logged; ⌘T / ⌘1), Catch up (amber count badge, hidden at 0, `99+` cap; ⌘2), Review (⌘3).
- [ ] Month calendar (below ~580 pt window height becomes a one-week strip with week arrows): month arrows (limited to 2000-01 .. this month), every day <= today is a button, marks (disc/half disc/amber ring/dash/dot), today ring, selected = open page, calendar follows the open page, status spoken ("Tuesday 6 October, not logged").
- [ ] Recent list: 7 newest pages (days with a file, excluding today); rows show logged/started/skipped glyph (+`skipped` text); row context menu: Show in Finder, Copy as Markdown (not for skipped days; copies the body), Skip this day (only when not logged/skipped); empty text "Pages you write appear here."
- [ ] Footer: streak chip/"No streak yet" (opens 12-week heatmap popover; "Best: N"), Settings gear (⌘,), Sync chip (`N conflict(s) to review` -> sheet, `N day(s) still downloading` -> Settings > Storage), waiting-jots line `N jot(s) waiting` + `Try now` (help: "Saved on this Mac. They are added to today's page when your log folder is back.").
- [ ] Go to date button + popover (`GoToDateView.swift`): field (placeholder "last fri, 2 oct, -3"), live preview `Fri 9 Oct`, errors "Couldn't read that date. Try 2 Oct, last friday or 2026-10-02." / "That day hasn't happened yet."; calendar below; ⇧⌘T.
- [ ] Toolbar (on a day): previous day `‹` (⌘[), `Today`, next day `›` (⌘]; disabled on today), Go to date.

### 3.C Day page (`PageView.swift`, `PageHeader.swift`, `DayEditor.swift`, `AppModel+Editor.swift`, `EditorBridge*.swift`)

- [ ] Layout: centred column 720 pt content + 48 pt gutters; scrolling header area limited to 50% of window height; editor fills the rest.
- [ ] Header: big date `EEEE, d MMMM yyyy`; relative label for past days ("Yesterday", "N days ago", "N weeks ago", "N months/years ago"); streak chip (today only); live word progress `12 words. Counts as logged at 20.` / `Logged, 63 words`; jot count `· N jots`; `Saved` indicator fading after 2.2 s; "Before your log start. It won't appear in Catch up." footnote for days before log start; skipped state label.
- [ ] `…` menu: Copy page as markdown / plain text / rich text, Carry over (when available), Skip day…, Restore previous version…, Go to date…, Open folder, Catch up, Settings….
- [ ] Autosave `[sec]`: debounce 600 ms after a user `change` (editor emits `change` 300 ms after user edit); also on day switch, window close, app resign, sleep, quit, Save Now (⌘S). A page that was only opened is never written; a page with no file and no net change is never created; typing a heading alone is a real edit; emptied page deletes the file after backup; a page that is only empty `#` headings is treated as emptied; template-over-jots page writes only the jots (`stripUntouchedTemplate`).
- [ ] New/empty page shows the template in the editor only; every template heading gets an empty paragraph under it (`openTemplateLines`, not an edit; caret ends in first empty line).
- [ ] Day-switch protocol `[sec]`: one script swaps document, returns the old page's token + final markdown + new normalised markdown (read twice for `stable`); old page saved from exactly what the user saw; tokens (`window.__dlDoc`) guard every read/replace/append (stale = refuse); `retiredMarkdown` guard drops a late `change` equal to the old page's text.
- [ ] External change detection: on tick/activate compare disk body with `diskBody`; clean page reloads quietly in place (cursor kept); page with unsaved edits keeps ours, sets `forceBackupNext`, shows "This page changed elsewhere. Your version here replaces it when it saves; the other one stays under Restore previous version."; skipped-state change reopens the day.
- [ ] Skipped page: no editor; text "You skipped this day. It doesn't affect your streak." + [Write a log anyway] (loads template, not written until edit; typing replaces the marker) + [Undo skip].
- [ ] Banners/notices (all variants): folder missing ("Your log folder can't be found." [Choose folder…] [Try again] [Create it again]), folder not writable ("Gloamlog can't save to this folder." [Choose folder…] [Try again] [Copy my text]), save error ("Couldn't save this page." + detail [Try again] [Copy my text]), orphans ("A page you left couldn't be saved yet."), editor unavailable ("The editor isn't available."), raw-text fallback ("This page can't be edited safely." + [Show in Finder]), load failed ("This page couldn't be opened for editing." [Try again]), reminder slim notice (Strict: "It's <time>. Time to write up today." with snooze label `Remind me in 15 min · 2 left`, "Skip day", "Not today?" hint after 3 reopens; Gentle: "Today isn't logged yet."), Catch-up notice once a day ("You haven't logged Thu 8 Oct." [Write it] [Skip day] or "You have N unlogged days." [Catch up] [Later]), rollover ("It's now <date>. You're still writing <date>." [Start today's page]), backup warning, page notice (6 s auto-dismiss), still-downloading page (3.R).
- [ ] Editor-host behaviours: ⌘[ ⌘] offered to the main menu before the page; links open only http/https/mailto in the default browser; image upload round trip (`uploadImage` -> `AssetStore.save` -> relative path); native open panel for "Choose image" (png/jpeg/gif/webp); remote images blocked; asset scheme `dlasset://local/<relative path>` served only through `AssetStore.resolve` (404 otherwise; Content-Type from extension, `Cache-Control: no-store`, `nosniff`); non-persistent web data store; `ready` always means "reload state" (setMarkdown, theme, placeholder, readOnly); web-process-terminated -> save last known text, new page, reload same day; 10 s ready watchdog -> "The editor is taking a while to start."; transparent page background; accessibility label "Daily log page" / "Page for <spoken date>".
- [ ] `reveal(text:)` after opening a search hit (selects first occurrence via `window.find`, 350 ms delay).
- [ ] Restore previous version sheet (3.R).

### 3.D Editor web bundle (`editor-web/`, `docs/EDITOR_CONTRACT.md`)

- [ ] Reuse `editor-web/src/main.js` + `theme.css` + `build.mjs` unchanged where possible (Milkdown Crepe 7.22.2, esbuild 0.28.2, single inline `index.html`, CSP `default-src 'none'; script-src 'sha256-<hash>'; style-src 'unsafe-inline'; img-src dlasset: data: blob:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'`). Wails serves assets differently: re-point `img-src` and the `dlasset:` rewrite, and replace `window.webkit.messageHandlers.dl` (`handler` in main.js line ~74) with a Wails binding adapter; keep message shapes.
- [ ] JS -> host messages: `ready`, `change{markdown}` (debounced 300 ms, user edits only), `uploadImage{id,name,mime,dataBase64}` (answered via `DailyLogEditor._resolveUpload(id, path|null, err|null)`), `openLink{url}`, `log{level,message}`, `rawFallback{reason}`, `appendResult{id,result:"appended"|"alreadyPresent"|"stale",markdown|null}`.
- [ ] Host -> JS API `window.DailyLogEditor`: `setMarkdown(md,{cursor:"start"|"end"|"keep",focus})`, `getMarkdown()`, `isRawFallback()`, `setTheme`, `setReadOnly`, `focus`, `setPlaceholder`, `insertMarkdown(md, where)`, `appendToSection(callId, token, heading, markdown, {unlessPresent})`, `_resolveUpload`. Host-side composed scripts to port from `EditorBridge.swift`: `swapDocument`, `read(token)`, `replace(token, expecting, md, cursor)`, `insert`, `openTemplateLines`, `reveal`, `pushTheme`. Calls before `ready` queue.
- [ ] Invariants: programmatic calls never emit `change`; round trip idempotent (`serialize(parse(x)) == x`); rawFallback shows read-only raw text and `getMarkdown()` returns the original; empty paragraphs and unfilled image blocks never written; `appendToSection` is ONE ProseMirror transaction (+ empty closing one), caret/selection mapped not set, own undo step, never emits `change`, `stale` when token mismatches or raw fallback.
- [ ] Editor fixtures `editor-web/test/fixtures/*.md` (16 files incl. a 1,271-line `long.md`) and `test/index.html` harness (`node editor-web/dev/serve.mjs`) remain the regression suite (node/browser, not Swift).

### 3.E From yesterday / carry-over (`PageHeader.swift` FromYesterdayStrip, `AppModel+Day.swift`, `Core/CarryOver.swift`)

- [ ] Strip on Today only, collapsible (state in `yesterdayExpanded`), header `From yesterday` + `<label> · N item(s)` (first 8 items then "and N more"), button `Carry over N item(s)` (⇧⌘Y, menu Page > Carry Over from Yesterday), hidden once applied (`isApplied`).
- [ ] Carry over = `CarryOver.apply` on the editor's CURRENT markdown via `replace(expecting:)` (refuses if user typed meanwhile), `programmaticEdit` saves it; `Carried over from yesterday.` + Undo for 10 s restores previous markdown; announced to VoiceOver.
- [ ] Recomputed on page open, settings change of headings, day rollover.

### 3.F Suggested from your work strip (`SuggestionsStrip.swift`, `SourcesController.swift`)

- [ ] Shown only when a source is on, consent given, day not hidden, page editable; collapsible (`suggestionsExpanded`), title `Suggested from your work`.
- [ ] States: looking at your work (spinner + progress done/total), nothing found, "No projects are ticked yet, so nothing is read." [Choose projects…], list of checkable bullets (+ detail line of files), `Select all`, `Insert selected`, `Look again`, `Not today` (hides for that day, 8 s undo, remembered 90 days), notes line ("N records skipped", format-changed warning, retention note, git notices), hidden-values line ("4 values hidden (2 emails, 2 tokens)"), last-scan line "Last look: Thu 8 Oct, 12 files read, 0 records skipped."
- [ ] Cancel/generation logic: a newer scan or setting change cancels the old; results re-used for 300 s with same settings key.
- [ ] Insert, auto-add, dedupe rules: see 2.4.

### 3.G Skip, undo skip (`SkipSheet.swift`, `AppModel+Day.swift`, `AppModel+CatchUp.swift`)

- [ ] Sheet titles: "Skip today?" / "Skip this day?" / "Skip these N days?" (list mode from Catch up). Reason chips Holiday, Leave, Sick, Day off, Other… (free text up to 40 chars), optional "Through a later date" range (scheduled days only), copy "No reminders, and your streak stays as it is. You can undo this any time." / "A day with writing can't be skipped, so nothing you wrote is touched."; default Enter, Esc cancel.
- [ ] Core rules: `skip` refuses a day with writing (headings-only pages are overwritten); `skipRange` writes one marker per scheduled weekday in range, cap 3700 days, leaves written days; batch `skipDays` all-or-nothing with rollback, ignores already-skipped days, undo (`unskipDays`) removes only still-pure markers; a skip before the first page pins log start.
- [ ] Errors: "That day already has writing. Skipping would overwrite it.", "None of those days is a scheduled day, or they already have writing.", folder messages.
- [ ] After a batch skip: 8 s Undo bar "Skipped N days as <reason>."; single skip on open day reopens it as skipped; unskipping today sets 120 s reminder grace.

### 3.H Catch up + session (`CatchUpView.swift`, `SessionBar.swift`, `AppModel+CatchUp.swift`)

- [ ] Screen: unlogged working days oldest first grouped by week; per row `Write` or `Continue` (partial) + `Skip`; ⇧-click / ⌘-click multi-select then `Skip…`; `Start catching up` (⌘↩ "Open the oldest unlogged day"); `Show every unlogged day` (clears `catchRange` from "Catch up this week").
- [ ] Empty/other states: nothing unlogged but older days exist -> "You're all caught up." / "Every working day since <date> has a page or a skip." [Go to today] [Review this week]; new user (no pages) -> "Nothing to catch up on yet." / "Days before your first page don't count." [Open calendar] [Log start date…] (opens Settings > Page); session finished -> "All caught up. 3 logged, 1 skipped." [Review this week] [Go to today]; header "You have N unlogged days." vs "You're all caught up."; scope label `Last N days` (or `Since log start` when window >= 365); row status text `Logged` / `N of 20 words` (partial, button `Continue`) / `Not started` (button `Write`).
- [ ] Session bar above the normal page: `Catching up 3 of 5 · Wed 7 Oct`, End (Esc / "Back to the list"), previous day in run, Skip day, Next unlogged day (⌘↩; ⌥⌘↓ in session; ⌥⌘↑ previous), wrap once, note "This is the only day left. Write a little more, or skip it." (5 s). Days fixed at session start; next day always asked of the live list.
- [ ] `select(.catchUp)` reloads folder state first; Today notice rules (3.C).

### 3.I Search (`SearchView.swift`, `AppModel+Browse.swift`, `Core/Search.swift`, `SearchIndex.swift`)

- [ ] Query in sidebar field; 150 ms debounce; latest-only (switchToLatest); flush editor first so just-typed text is searchable; runs off main thread over an in-memory index (per-day signature `mtime.ns size`, re-read only when changed; not-local days left out and never cached as empty; deleted days drop out).
- [ ] Matching: case- and diacritic-insensitive (`NSString` semantics), words AND-ed within a heading-delimited section (heading text counts), one hit per section, newest day first then document order; snippet ~40 chars before / 80 after the first (earliest) word with `…` edges; skipped days not searched; heading filter picker `All headings` + template headings (matched by normalised title).
- [ ] Results UI: grouped by day with heading label, match highlighted, count summary, empty state ("... Try a shorter word."), `⌘G` / `⇧⌘G` next/previous hit (cursor wraps), Return opens selected, click opens day and reveals match.

### 3.J Review, copy, export (`ReviewView.swift`, `WeeklyReviewView.swift`, `ReviewController.swift`, `ReviewExporter.swift`, `ExportSheet.swift`, `ReviewCommands.swift`, `Core/Review/*`, `Core/WeeklyReview.swift`, `Core/Export/*`)

- [ ] Screen: title + `‹ This week ›`, counts line `Logged 4 of 5 · 1 skipped · 2 not logged`, scale control Week / Month / Year / Range (Month ⌘4, Year ⌘5; Week ⌘3), Range pickers (max 3700 days, either order), `Last week`, `This month`, `Catch up` (scoped to the week), Export menu (⇧⌘E sheet), checkboxes `Include skipped days` / `Include Jots`, Group by Day | Section (week default Day, others Section; remembered).
- [ ] Week: seven tiles (weekday, number, status mark, bottom: `63 words` / `Write` / reason or `Skipped`), each opens the day (future days inert), empty weeks still show tiles; Month/Range: cards by Day or by Section (items dated); Year: 12 month rows (`20 logged · 1 skipped · 2 not logged`, dashes for future months, row opens that month); tasks line `12 tasks done · 3 still open`.
- [ ] Build rules: counts via Status.resolve + CatchUp.isMissing; groups = template headings (template order) then Jots (if included) then `Other notes` then `Skipped days`; Jots split out of pages unless included; ticked tasks de-duplicated by itemKey; open tasks = last logged page's carry-over items; ranges <= 100 days built synchronously, longer off-main with generation tag; Year reads no pages.
- [ ] Copy as Rich Text / Markdown / Plain Text (menu File > Copy as, Export sheet, ⇧⌘C = markdown), Copy page (same three) from page `…` menu, menu-bar `Copy this week's summary` (rich).
- [ ] Save as Markdown file (+assets folder), Folder, Zip, PDF (NSSavePanel; names per 1.10); results line `Saved 5 days and 2 images.` + Show in Finder; failure state with Retry / Choose another place… / detail; busy `Making the PDF…`; `Share…` via NSSharingServicePicker anchored to a view.
- [ ] Export sheet shows what will be written: "5 days, 2 images. Skipped days included. Jots left out."
- [ ] Empty states: "Nothing logged this week yet.", "Nothing logged in this range yet.", "No working days to count yet.", "Nothing to count yet."

### 3.K Quick capture: Jot panel, hotkey, popover field (`JotPanel.swift`, `JotView.swift`, `HotKey.swift`, `AppModel+Capture.swift`, `Core/Capture/*`)

- [ ] Floating panel `Jot to Today`, 520 pt wide, non-activating NSPanel (takes keys without activating the app or raising the main window), floating, joins all Spaces, appears over full-screen apps, shown on the screen under the pointer with its top 22% down the visible frame, focus returns to the previous app on close.
- [ ] Field: 1-6 lines (44-144 pt, scrolls beyond), header shows the time stamp the note will carry (`14:32`, hidden if timestamps off), hint `Return adds · ⇧Return new line · Esc cancels` (⌥Return also newline), Return on empty or `[]` does nothing, Esc closes and discards draft, clicking away hides but keeps the draft, shortcut again while open refocuses; `Added to Today` shows 0.6 s then closes; kept state 2.6 s: "Kept on this Mac. It will be added when your log folder is back."; failed: "Couldn't save. Your note is still here." + Try again.
- [ ] Menu-bar popover Jot field (`PopoverJotField`): same routing, source `menubar`, flash Added / Kept on this Mac / Not saved; `N jots waiting` + `Try now`.
- [ ] DayWriter semantics `[sec]` (see `Core/Capture/DayWriter.swift` header): journal (fsync) BEFORE routing; open+loaded page -> `appendJot` through the editor (acknowledged when the editor's autosave has written the line: `acknowledgeSaved`); `.notLoaded` retries (bounded timer 0.5 s x 12); `.stale` re-routes once; no page -> load file, `CapturePlacement.apply`, one save per day for all pending notes, ack; folder unavailable/save error -> note stays pending; never writes a day's file while an editor exists for that day; replay at launch, folder return, wake, activation, folder change, page load; first replay compacts the journal; idempotent via `jotKeys`/editor-spelled key; a skipped day is written over; a note keeps the day it was captured on.
- [ ] Global hotkey (section 4): default ⌃⌥J, record/Off/Reset in Settings, failure messages (taken in-process, owned by macOS, needs ⌃ or ⌘, other OSStatus).
- [ ] Menu command `Page > Jot…` shows the recorded shortcut.

### 3.L Menu bar item (`MenuBarView.swift`, `Heatmap.swift` WeekStrip, `GloamlogApp.swift`)

- [ ] `MenuBarExtra` (window style, 300 pt popover), icon SF Symbols by state: `exclamationmark.triangle` folder problem, `checkmark.circle` logged, `moon.zzz` skipped, `circle.dashed` not a workday, `pencil.circle.fill` due, `book.closed` not yet logged; accessibility labels e.g. "Gloamlog: today logged". Toggle in Settings (`showMenuBar`).
- [ ] Popover: `Today · Thu 8 Oct`, state line ("Logged today ✓", "Skipped today (Holiday)", "Not a workday", "Due now: 4:55 pm", "Not logged yet", "Saving problem: folder unavailable"), reminder line `Next reminder: Tomorrow, 4:55 pm · Strict`, Jot field, streak line, last-7-days strip, buttons: Open Gloamlog (⌘O), Jot… (shortcut), Copy this week's summary, snooze (when due; "Remind me in 15 min · 1 left" / "No snoozes left today"), Skip today… / "Today is already logged", Settings… (⌘,), Quit Gloamlog (⌘Q).

### 3.M Settings window (`SettingsWindow.swift`, `SettingsView.swift`, `SettingsTheme.swift`, panes)

Common: AppKit window 680 pt wide, height fitted to pane, toolbar tabs with icons (General, Reminders, Page, Storage, Shortcuts, Sources, About), title follows pane, ←/→ switch pane when not typing, ⌘W hides, remembers last pane (`dailylog.settingsPane`), everything applies at once (no Save), per-pane `Reset to defaults…` with confirmation (General, Reminders, Page only; never touches pages, folder, backups), one window reused, prewarmed hidden.

- [ ] **General** (`SettingsPaneGeneral.swift`): Appearance (System/Light/Dark segmented); Week starts on (System/Monday/Sunday/Saturday; help names the system's); Open Gloamlog at login (SMAppService; messages "Approve Gloamlog in System Settings > General > Login Items." + [Open Login Items], "Couldn't change the login item: ..."); Show in the menu bar.
- [ ] **Reminders** (`SettingsPaneReminders.swift`): Remind me at (time picker; help `Next reminder: Thu 8 Oct, 4:55 pm` or "none planned"); On these days (seven toggle buttons in week order, at least one stays on); Style Strict/Gentle with help text; Snooze for 5/10/15/20/30 minutes; notification status row (on / off + [Open System Settings] / not asked + [Allow]); Remind me when Gloamlog is closed toggle (disabled when denied) with limits help (14 days, sleep, Focus).
- [ ] **Page** (`SettingsPanePage.swift`, `TemplateEditor.swift`): A day counts as logged after N words (1-500 stepper); Log start date (date field, `Use first log`, `Start from today`, help text for follow-first-log vs pinned); Catch-up window (7, 14, 30, 60, 90, 180, 1 year, plus stored odd value); New day template (markdown TextEditor, `Start from…` presets, `Reset to default`, 500 ms debounce, untouched pages follow template changes, pages with writing never change); Carried-over headings list (add/remove).
- [ ] **Storage** (`SettingsPaneStorage.swift`, `AppModel+Storage.swift`, `SyncChip.swift` SyncStorageSection): Folder path (+ summary `12 logs · 2 skipped days · 1 file not recognised`), Show in Finder, Change… (NSOpenPanel), Use default; folder problem rows; result row with "Show them" for conflicts; Safety copies location + Open; Sync section (folder kind label Local / iCloud Drive / Dropbox / OneDrive / GoogleDrive / Network or external drive, status, Download all, Resolve conflicts).
  - [ ] Folder change flow `[sec]`: flush editor into OLD folder first; alerts: empty old -> just switch; old has logs & new empty -> [Copy logs] [Move logs] [Leave them] [Cancel] (move = copy then delete only byte-identical sources); new already has logs -> [Use this folder] [Use and copy mine over] [Cancel] (never overwrites same-day files; collisions listed); images in `assets/` copied/moved along (symlinked assets leaving the folder not followed); not writable alert; same folder alert; then new store, reload, reopen day.
- [ ] **Shortcuts** (`SettingsPaneShortcuts.swift`, `ShortcutRecorder.swift`): Jot recorder row (record via local key-down monitor only while Settings is key; swallows keys; Reset, Off; failure row with Record another…/Off), toggles Time stamp, `[]` makes a to-do, Jots count toward logging a day; text field Jots go under; read-only shortcut lists (Go / Page / Search and copy / Settings, see 3.P).
- [ ] **Sources** (`SettingsPaneSources.swift`, `SourcesController.swift`, `ClaudeSetup.swift`): privacy status line, switches Claude Code / Git (first enable shows consent sheet "Let Gloamlog read your Claude Code and Git activity?" [Not now] [Turn on]; nothing on before accept), Every session and tab, Claude writes my log (Connect/Connected+Disconnect/Needs reconnecting+Reconnect/Claude Code not found/spinner) + message line, Add commits for me, Instant update [Copy hook], Insert under (heading); Git folders list (Add folder…/Remove, help); Projects list (name, abbreviated path · tags `Claude Code, Git` / `not found`, toggle; sorted ticked first, then recent, then name; states: turn on a source, looking, none found variants), last-scan line; the list refreshes when the pane appears.
- [ ] **About** (`SettingsPaneAbout.swift`): icon, `Version 0.6.6` (+ build if different), blurb, [Run setup again…] (re-opens onboarding, changes only what is edited), [Third-party licences] (opens bundled THIRD_PARTY_LICENSES.md), "Released under the MIT licence."

### 3.N Onboarding and What's new (`Onboarding.swift`, `MainView.swift` WhatsNewSheet)

- [ ] 4-step sheet (last optional): 1 Welcome + template cards (Daily review / Standup / Blank page; your own template stays unless a card is picked on re-run); 2 "When should we ask?" reminder time, weekdays, Strict/Gentle ("Comes to the front until you save or skip the day." / "One notification. You decide when."), words rule ("A day counts after N words..."); 3 "Where should logs live?" On this Mac `~/Gloamlog` / iCloud Drive `~/Library/Mobile Documents/com~apple~CloudDocs/Gloamlog` (only if iCloud Drive exists) / Another folder…, found-logs summary + "Log starts" date (when logs already exist), Open Gloamlog at login (unticked default; opt-in); 4 "Let Claude write your log" -> Connect Claude Code / Connecting… / Connected / Reconnect / "Claude Code was not found on this Mac. You can connect it later in Settings > Sources."; buttons Back/Next/`Start writing`/`Skip setup`.
- [ ] Finish: `applied(to:)` writes settings (`onboarded=true`, folder only on first run; re-run folder goes through the copy/move dialog), sets `whatsNew` key, opens today with focus. "Skip setup" applies defaults.
- [ ] "What's new in 0.6" sheet (shown once for existing users): 6 lines (Go to date, Catch up, Jot with the real shortcut or menu bar, Review/export ⇧⌘E, closed-app reminders, Settings ⌘,) + [Take a look at Settings] [Done]. Port should decide the text for the new version.

### 3.O Reminders and notifications (`AppModel+Reminders.swift`, `Notifier.swift`, `ClosedAppReminders.swift`, `Core/ReminderPlanner.swift`, `Core/Schedule/ReminderScheduler.swift`)

- [ ] In-app planner: see 1.5. `perform`: `.notify` -> notification; `.bringToFront(notify)` -> notification (if first), show window + activate, open today with focus, Dock bounce unless Reduce Motion; `.reopen` -> orderFrontRegardless without activating.
- [ ] Snooze (`⇧⌘L`, notification button, menu-bar, slim notice): Strict max 2/day then label "No snoozes left today"; Gentle unlimited; `snoozeMinutes` choice; cancels delivered notifications.
- [ ] Notifications: one id `dailylog.reminder` (replaced not stacked), `.banner` presentation in foreground, category `reminder` actions Open (foreground) / Snooze N min / Skip today; content never includes log text.
- [ ] Closed-app plan applied by diff (`ReminderScheduler.diff`; only ids under `gloamlog.reminder.` touched; max 20 s between re-plans; signature gate; force on launch/activate/wake/midnight/clock/zone/permission change); logging or skipping today cancels its pending + delivered requests immediately; one notification per day (`postInApp` defers to a delivered scheduled one < 120 s old); snooze from a banner schedules a one-off `gloamlog.snooze.<day>` request when before 23:00 same day; banner click for an older day opens that day; Snooze/Skip launched-from-notification hides the window again.
- [ ] Permission states `.unknown/.allowed/.denied` shown in Settings; everything degrades silently without a bundle/notification center.

### 3.P Keyboard shortcuts and menus (`AppCommands.swift`, `ReviewCommands.swift`, `SettingsWindow.swift SettingsCommands`, `SettingsPaneShortcuts.swift`)

| Menu | Item | Shortcut | Enabled when |
|---|---|---|---|
| App | About Gloamlog / Check for Updates… / Save Layout Report | | |
| App | Settings… | ⌘, | |
| File | Save Now | ⌘S | day page with editor |
| File | Copy Page / Week / Review as Markdown | ⇧⌘C | page or review |
| File | Open Log Folder in Finder | ⇧⌘O | |
| File | Export… | ⇧⌘E | review |
| File | Copy as ▸ Rich Text / Markdown / Plain Text | | review or page |
| File | Save Review as Markdown… / as PDF… / Share Review… | | review |
| Edit | Search Logs / Next Result / Previous Result | ⌘F / ⌘G / ⇧⌘G | |
| Go | Today | ⌘T and ⌘1 | |
| Go | Catch Up / Week Review | ⌘2 / ⌘3 | |
| Go | Previous / Next Day (or Week/Month/Year/Period on review) | ⌘[ / ⌘] | can step |
| Go | Go to Date… | ⇧⌘T | |
| Go | Next Unlogged Day / Previous Unlogged Day | ⌘↩ / ⌥⌘↑ (session: ⌥⌘↓ next) | list non-empty / session |
| Review | Week / Month ⌘4 / Year ⌘5 / Range / Last Week / This Month; toggles Include Skipped Days, Include Jots; Group By Day or Section | | |
| Page | Focus the Page | ⌘E | editor shown |
| Page | Carry Over from Yesterday | ⇧⌘Y | strip available |
| Page | Skip Day… | ⇧⌘K | |
| Page | Remind Me Later | ⇧⌘L | due and can snooze |
| Page | Jot… | the recorded global hotkey (default ⌃⌥J) | |
| Page | Restore Previous Version… / Settings… | | day page |
| (window) | ⌘1 hidden button = Today; Settings window ←/→ = switch pane; Return = open selected search hit; Esc in search clears | | |

### 3.Q Heatmap and streak popover (`Heatmap.swift`, `PageHeader.swift` StreakPopover)

- [ ] 12-week grid (columns = weeks, rows = weekdays from `firstWeekday`), shape-coded states with 3:1 contrast, click a cell opens that day; header `N day streak` / `No streak yet`, `Best: N`; last-7-days strip in the menu bar.

### 3.R Sync, conflicts, restore version (`SyncController.swift`, `SyncChip.swift`, `ConflictSheet.swift`, `RestoreVersionSheet.swift`, `Core/Sync/*`)

- [ ] Folder watcher: FSEvents (0.5 s latency, file events, watch-root) for local/cloud folders, polling every 5 s for network/removable volumes or when FSEvents fails; ignores hidden files, `assets/`, our own writes (`OwnWrites` signature = `mtime.nsec size` recorded by AtomicFile); relevant = day file, iCloud placeholder or conflict copy; lost-events flags force a rescan; on change: stamp-gated reload + external-change check at once; 30 s fallback timer and activation scan.
- [ ] Scan (`FolderScanner`): day files / placeholders / conflict copies; "not here" = dataless flag, placeholder name, or (iCloud Drive only) `ubiquitousItemDownloadingStatus` notDownloaded; `downloaded` = stale local copy readable; rescan cost = lstat per file when unchanged. Requests download (`startDownloadingUbiquitousItem`, else a 1-byte read) for pending days; `Download all`.
- [ ] Not-local day: opening shows "Downloading from iCloud…" page ("This day is in your shared folder but not on this Mac yet. Gloamlog is waiting for it and won't start a new page here, so nothing you wrote on another Mac gets overwritten. It opens by itself when it arrives." + [Try again] [Open Storage settings]); excluded from Catch up; save while a day went back to cloud errors "This day is still downloading from your cloud folder. Your text stays here and is saved when it arrives."
- [ ] Conflict banner above a day page + sidebar chip; sheet `Two versions of this day`: this Mac vs other copy side by side with differing lines marked, header `<who>, <EEE d MMM, h:mm am>, N words`; buttons `Keep both` (default), `Keep mine`, `Take theirs`, `Previous/Next conflict`, `No conflicts left.`; explanation "Lines with a bar are only in that version. Keep both adds the other copy's extra lines under ...".
- [ ] Resolve algorithm `[sec]`: flush editor; refuse if unsaved edits; read both sides; compute new body (Keep mine: none; Take theirs: other body, refuse if empty; Keep both: `BothVersions.merge`, no change if nothing new); 1. copy conflict file into `backups/conflicts` and verify bytes (+ this Mac's page when it will change); 2. save chosen text through normal save with forced backup; 3. delete conflict file only if still byte-identical to the kept copy. Identical copies are settled silently. Announce "Conflict settled. The other version is kept in your safety copies."
- [ ] Restore Previous Version sheet: list of versions newest first (`EEE d MMM, h:mm` local from UTC stamp, N words, 120-char preview), Restore, Cancel/Close, empty text "No earlier versions yet. They appear after you change a page that was already saved."; copy "Gloamlog keeps the last 10 versions of each day. Restoring saves the current page as a version first, so you can undo it."; errors "Couldn't keep a copy of the current page, so nothing was changed." / "Backups are turned off." / folder / generic.

### 3.S Updater (`Updater.swift`, `Core/UpdateCheck.swift`)

- [ ] See section 6.4 for the exact flow. Dialogs: "No updates published yet" (404), "Couldn't check for updates", "You're up to date", "Gloamlog X is available" [Download and Install] [Release Notes] [Later] (notes <= 600 chars), progress panel "Updating Gloamlog" with "Downloading Gloamlog X…" / "Restarting Gloamlog…", "Update not installed" failures, "Can't install automatically" fallback [Open Download Page].

### 3.T Misc user-visible behaviours

- [ ] `Fmt.plural` rule (`-ch -sh -s -x -z` take `es`; used for "match"/"matches"), `cleanHeading` (drops leading emoji/symbols for display; files keep raw heading), `Banner`/`SlimNotice`/button styles (visual only).
- [ ] Design tokens (`UI/Theme.swift`): bg `#F9FAF9`/`#141716`, surface `#FFFFFF`/`#1C201E`, sidebar `#F0F2F0`/`#101312`, border `#DCE1DD`/`#2E3532`, text `#1B201E`/`#E8ECEA`, secondary `#566059`/`#A3ADA7`, accent `#0E7A5F`/`#45C29C`, warning `#9A5B00`/`#E0A23A`, danger `#B3261E`/`#F0847A`, heat 5-step; spacing 4/8/12/16/20/24/32/40/48; radii 4/6/10; column max 720; system font (SF).

---

## 4. Platform needs (native macOS APIs and how Swift does each)

App is **not sandboxed** (no entitlements file, ad-hoc `codesign --force --sign -`, no hardened runtime). All file access is by plain path; no security-scoped bookmarks. macOS TCC still prompts for protected folders (Documents, Desktop, iCloud Drive, `~/.claude` read via Files & Folders: the Claude source emits the notice "Gloamlog can't read Claude Code's files. Allow access in System Settings > Privacy & Security > Files & Folders.").

| Need | Swift API (file) | Behaviour to preserve / port notes |
|---|---|---|
| Menu-bar item with popover containing a text field | SwiftUI `MenuBarExtra(.window)` (`GloamlogApp.swift`, `MenuBarView.swift`); SF Symbols icon, dynamic by state | Toggle via `showMenuBar` default; field must take keyboard in a popover. |
| Floating non-activating panel over any app / full-screen | `NSPanel` `[.borderless,.nonactivatingPanel]`, `canBecomeKey=true`, `.floating`, `.canJoinAllSpaces + .fullScreenAuxiliary`, positioned on pointer's screen (`JotPanel.swift`) | Previous app must stay active; AppKit text view for Return / ⇧Return / Esc handling (`JotTextField`). Webview-hosted panel needs a non-activating NSPanel host. |
| Global hotkey | Carbon `RegisterEventHotKey` (signature `GLMJ`) + `InstallEventHandler`; `CopySymbolicHotKeys` to detect macOS-owned combos (`HotKey.swift`) | No Accessibility/Input Monitoring permission. Needs ⌃ or ⌘. Same combo twice in-process = -9878; other-process clash undetectable. Recorder = local `NSEvent` key-down monitor. Keycode 38 = J. |
| Notifications (scheduled + immediate + actions) | `UserNotifications`: `UNCalendarNotificationTrigger`, `UNTimeIntervalNotificationTrigger`, categories/actions, `getPending/getDelivered`, delegate (`Notifier.swift`, `ClosedAppReminders.swift`) | Needs a real bundle id/signature; ad-hoc builds may be denied (silent fallback to in-app reminder). 14-day rolling plan re-applied on launch/activate/wake/midnight. Request `[.alert]` only. |
| Launch at login | `SMAppService.mainApp.register/unregister/status`, `openSystemSettingsLoginItems()` (`AppModel+Storage.swift`) | Opt-in; status `requiresApproval` message. |
| Folder watching | `FSEventStream` (file events, 0.5 s latency, watch root) with 5 s polling fallback (`Core/Sync/FolderWatcher.swift`); own-write echo suppression | Go: fsnotify or FSEvents cgo; keep polling fallback for network volumes, own-write signature, stat-based `folderStamp`. |
| Cloud files | `URLResourceKey.isUbiquitousItemKey`, `ubiquitousItemDownloadingStatusKey`, `FileManager.startDownloadingUbiquitousItem`, `lstat` `st_flags & SF_DATALESS (0x40000000)`, `.name.icloud` placeholders (`Core/Sync/CloudFile.swift`, `SyncScan.swift`) | Needs cgo/objc for ubiquity keys; dataless via `syscall.Lstat` flags. Provider detection by path (`com~apple~CloudDocs`, `CloudStorage/<Provider>-...`, `Dropbox`) and `volumeIsLocal`. |
| Atomic, crash-safe writes | POSIX `open/write/fsync/rename`, `F_FULLFSYNC` for journal (`AtomicFile.swift`, `CaptureJournal.swift`) | `.gloamlog-tmp-<pid>-<uuid>`, `kill(pid,0)` stale sweep, permission copy. |
| Pasteboard flavours | `NSPasteboard` (.string, .html, .rtf) with RTF from HTML (`ReviewExporter.swift`) | RTF only if < 200 KB. |
| PDF export | `WKWebView` off-screen + `NSPrintOperation` (save disposition) verified with PDFKit; fallback `createPDF`; 30 s watchdog | |
| Save/open panels | `NSSavePanel`, `NSOpenPanel` (folder chooser, repo chooser, image chooser) | |
| Share | `NSSharingServicePicker` | |
| Zip | `/usr/bin/ditto` | |
| Dock icon policy / menu / reopen | Regular app (no `LSUIElement`); `applicationDockMenu`; `applicationShouldHandleReopen`; `applicationShouldTerminateAfterLastWindowClosed=false`; `requestUserAttention` | |
| Quit gating | `applicationShouldTerminate` -> `.terminateLater` + `reply(toApplicationShouldTerminate:)` | |
| URL scheme | `CFBundleURLTypes` `gloamlog`; `application(_:open:)` | `refresh`, `layout`. |
| Appearance | `NSApp.appearance` (aqua/darkAqua/nil); `effectiveAppearance` pushes theme to editor | |
| Settings window | `NSWindow` + `NSToolbar` selectable items + hosted SwiftUI; key handling for ←/→ | |
| Accessibility | `NSAccessibility.post(.announcementRequested)`, labels/traits throughout | Port to ARIA live regions + native announcements. |
| Spawning `claude`, `git`, `ditto`, `codesign`, `sh` | `Process` (login shell `/bin/zsh -lic` only for `claude`) | Git: no shell, explicit env. |
| Opening links | `NSWorkspace.shared.open`, `x-apple.systempreferences:` | |
| System notifications of sleep/wake/session | `NSWorkspace` notification center (will/did sleep, screens sleep/wake, session active) | Suppress reminders while screens asleep. |
| SHA-256 | CryptoKit (assets, updater) | `crypto/sha256`. |
| Realpath/symlink safety | `realpath()` in `AssetStore` | Refuse assets outside notes folder. |
| WebView host | `WKWebView` non-persistent store, `WKURLSchemeHandler` `dlasset`, message handler `dl`, nav policy (only first load), `_setDrawsBackground` private key, key-equivalent interception for ⌘[ ⌘], process-terminated recovery | Wails uses WKWebView on macOS: implement asset route, binding adapter, menu accelerator precedence. |
| Locale/calendar | `Calendar.current`, `DateFormatter`, `Locale` (time style, `DateFormatter.dateFormat(fromTemplate:"yMd")` for field order) | See Q16. |
| Unicode text ops | `Unicode.Scalar.properties.generalCategory`, `NSString.range(of:options:[.caseInsensitive,.diacriticInsensitive])`, `NSRegularExpression` (Redactor, ClaudeText) | See Q17. |
| Version/bundle info | `Bundle.main.infoDictionary` (CFBundleShortVersionString) | |
| Debug-only | `NSWindow` layout report; editor-check/snapshot harnesses | Drop. |

---

## 5. Tests worth porting

Existing automated tests (all assert-based, no XCTest, no SwiftPM):
- `tests/main.swift` (2,612 lines, **123 `test()` blocks**, compiled with `app/Core/**`; `tests/run-tests.sh`): pure Core, run in CI (`macos-latest`).
- `tests/run-kill-tests.sh` + `app/Tools/kill-test/main.swift`: SIGKILL crash safety.
- `app/Tools/editor-check/` (~457 `check()` calls, real windows + real WKWebView, **not CI**, needs a normal terminal): app-layer scenarios, bridge, nav, settings, capture.
- `editor-web/test/index.html` (+16 fixtures, 134 assertion lines): editor round trips and bridge, run via `node editor-web/dev/serve.mjs`.
- `app/Tools/snapshot/` (headless UI PNG renderer, visual only).

### 5.1 `tests/main.swift` groups -> Go golden/unit tests (line ranges are `test(` locations)

| Lines | Behaviours pinned (port each as table/golden tests) |
|---|---|
| 39-104 | Word counting: basics, headings, list/task markers, quotes; code fences; images/HTML/comments; emoji and non-Latin (CJK `ceil(n/2)`, Thai chunks); `hasContent`; `DayDocument.status` boundaries |
| 105-143 | `normalizeHeading` ("📌 To do next" -> "to do next"); `sections(of:)` levels/nesting/fences/edge headings; `trimBody`, `shiftHeadings`, `pruneEmptySections`, `partition` |
| 145-187 | Day file format: v0.1/v0.2 files load unchanged and re-save is stable; legacy `\## ` unescape outside code; title/BOM/CRLF/skip markers/serialize |
| 189-294 | Store: save/load no escaping, unicode, images, idempotent, date-like first line; emptied page = no log; empty save never creates a file or touches a skip marker; skip/unskip/skipRange; `fileStates` follows words rule; `listDays`/`folderSummary` ignore assets and foreign files; typed errors for missing/unwritable folder; atomic writes leave no temp; bad days |
| 295-542 | Backups: overwrite copies old file first; delete keeps a copy; nothing-worth-keeping = no copy; newest 10 kept (names increase on a frozen clock); identical content no-op, skip markers/title-only not copied; failures never fail the save; restore refuses without safety copy; restore round trip (restore backs up current); backups never listed/searched/counted; UTC names, junk ignored, clock set back, throttle, off by default |
| 544-601 | Settings: defaults, round trip, tolerant decode, normalize; migration from v0.2 JSON (`sections` -> template) |
| 603-697 | Assets: save names/dedupe/ext allowlist/size cap; `fileExtension(mime:name:)`; resolve inside folder only; symlinks cannot escape |
| 700-766 | Streak across weekend/skip/miss; heatmap 12 weeks, statuses, today ring; DST day keys/heatmap/streak across fall-back and spring-forward |
| 767-866 | ReminderPlanner: not due/ineligible days; strict first fire, re-open every 5 min, skip hint, 23:00 cutoff; strict snooze max 2 then re-fire without second notification; gentle never forces, notifies once, unlimited snooze; launched after reminder time, asleep, settings/undo grace, day rollover; `isPending`; state is Codable |
| 867-933 | WeeklyReview aggregate by template heading + Other notes; markdown by day and by section |
| 934-1085 | Carry-over: card/items/apply/idempotent; previous LOGGED day only, lookback, minWords, headings; carried tasks never count toward logged (one click cannot log a day); which headings/lines count, nested and multiple blocks; apply never changes word count |
| 1086-1126 | Search: headings, snippets, matching rules |
| 1127-1164 | End to end through real files: store -> fileStates -> streak/heatmap/status/planner agree |
| 1165-1264 | Log start (backfill creates no wall of missed days; nil keeps old rule; fresh install no past misses); settings M1 fields/clamping; legacy migration (keys copied once, never overwritten; support folder moves once); default folder `Gloamlog` |
| 1278-1413 | MonthGrid: Monday/Sunday-first October 2026, year boundaries, 6 rows always, leap February, DST (42 unique consecutive days, midnight-DST zones), statuses, `isScheduled`, log start cuts missed to off, invalid month gives no rows |
| 1414-1550 | DateJump: keywords, relative days, weekday names (fri vs last fri), month names (no-year = most recent past), numeric dates follow calendar locale, ISO never; unreadable/impossible/future/pre-2000 = nil; case/whitespace/punctuation; whole-year round trip with independent oracle; boundaries; junk never crashes |
| 1551-1809 | CatchUp: lists missed/partial before today oldest first; exact window edges + clamp 7...365; log start cuts; DST window; fast for a year; `next(after:)`; `skipDays` one marker per day / leaves writing alone / all-or-nothing rollback / `unskipDays` pure markers only / skip-undo leaves folder byte-identical / streak unchanged by skip+undo / end-to-end real files; 30 days fast |
| 1810-1888 | Week review completeness: no-file weeks list every scheduled day as missed; weeks start on `firstWeekday`; current week future days; `missingDays` equals CatchUp restricted to the week |
| 1889-2005 | `folderStamp`: stable over (name, mtime, size); add/edit/delete/rename/touch change it; ignores assets/non-md/hidden/folders/backups; never reads file contents; 3,650 files < 50 ms |
| 2006-2073 | Kill regressions: killed write leaves only a hidden temp, swept by next store's first save (log + backups folder); skip also sweeps; failed save leaves no temp; permissions preserved |
| 2074-2310 | Capture: render (stamp/no stamp/to-do/multi-line, escaping of `# > - + * 1.`); apply (no Jots block creates one; end of existing block joined to list; heading-level boundaries, case, emoji, configured heading; unclosed fence/comment closed; fuzz: page lines never lost/reordered/edited, note always recognised); `contains` (marker style, escapes, ticked box, soft breaks); Jots rules (counts split, leading time not a word, `loggedWords`, `isJotsOnly`, `stripUntouchedTemplate`) |
| 2311-2463 | CaptureJournal: append/pending/ack order, folder created, 0600; one JSON line per record, any text survives; torn last line never loses earlier notes and next append not glued; unknown/garbled/future lines skipped, old records and duplicates tolerated; unwritable folder typed error, recovers; wider permissions tightened; compact (2,000 items) keeps order/mode, no temp, no-op when clean; drop without ack loses nothing |
| 2464-2533 | Sources all-projects mode; UpdateCheck (versions, release JSON, checksum, trusted hosts); heredoc before `git commit` not the message; Inbox (oldest first, junk skipped, emoji heading takes jots); ClaudeMd add/replace/remove/keep hand-written |
| 2534-2612 | Search index: cached equals uncached (headings, skipped days, heading filter), file parsed once per run; changed file re-read (own save, outside write, touched mtime); deleted file drops; placeholder never cached; `warmSearch`; replaced store starts empty; warm and search may overlap |

### 5.2 Other suites to port or keep

- [ ] Kill test (`app/Tools/kill-test/main.swift`): 40 rounds; child saves 2 KB pages round-robin over 5 days with backups on, parent SIGKILLs at random 5-300 ms; invariants: every day file equals some complete version v; acked(D) <= v <= started(D); no day vanishes; <= 1 stray temp, removed by next save; backups complete. Reproduce as a Go subprocess kill test.
- [ ] Editor-check scenarios (map to Playwright/Wails e2e or Go+webview harness): page open writes nothing; first edit creates file with title+template+text; no cross-day leakage on switch; late edit before switch saved to OLD day; past-day edit makes backup; typed-then-deleted creates no file; emptied page removes file with backup; external change quiet reload vs notice; carry-over strip/insert/undo; skip refuses written day; skipped page has no editor, "Write a log anyway"; save failure keeps text, quit flush reports unsaved, orphan retried; template change reloads untouched page only; nav (Today/step/⌘[ ⌘] gaps, Go to date, refresh ticks do not re-read folder, midnight rollover, week start Mon/Sun, backfill/log start, month vs strip calendar, 860x600 min); Review (tiles, empty week); Settings (cold open < 3 s, panes, persistence, reset, reminder re-plan, schedule cases: Wed morning -> today, Wed evening -> tomorrow, today logged -> tomorrow, Fri evening/Sat -> Mon 12 Oct); Capture (hotkey registration cases, crash/replay exactly-once, folder-away keep + replay, markdown-looking notes, panel behaviours incl. Esc discards, Return empty no-op, Shift-Return newline, one note multi-line, window-closed page, popover field); editor bridge (ready, swap tokens, stale read refused, replace/insert refusal, no `change` on programmatic edits, hostile text round trip/U+2028, `dlasset` scheme refusals: non-image, symlink escape, traversal, other host, 404; remote image CSP-blocked; paste image upload; link policy; 3,000-paragraph swap < 5 s; bridge released when dropped).
- [ ] Editor web tests (`editor-web/test/`): keep as-is; add a CI job (Node) since they are not in `ci.yml`.
- [ ] Round-trip idempotence on the 16 fixtures (`serialize(parse(x)) == x`).

### 5.3 Gaps: Core code with NO committed tests (write new golden tests; generate expected output from the Swift Core)

`Redactor` (all 17 rules, idempotence, 40-char hash kept), `ClaudeText`/`ClaudeTime`/`TranscriptParser`/`ClaudeCodeSource` (prompt/commit/PR extraction, format-drift stat, caching), `GitSource` (args/env/timeouts/discovery limits), `ActivityDigest` (bullet text, durations, truncation, dedupe, markdown escape, `isAdded`), `ProjectPolicy`/`SourcePath` (worktree folding, case-insensitive exact match), `FolderEntryClassifier`, `BothVersions`, `ConflictFiles`, `FolderScanner`, `CloudFile`, `ReminderScheduler` (14-day plan, diff, copy), `PeriodReview`/`ReviewFormats`/`MarkdownHTML`/`ExportBundle` (counts, groups, titles, file names, HTML escaping, link policy, export rewrite), `DayWriter` (only exercised in editor-check), `JSONLReader` (chunk boundaries, long-line skipping), `UpdateCheck` beyond the 1 existing test. `editor-check` references missing `ScenarioReview/ScenarioSources/ScenarioReminders/ScenarioSync` files (the sync one is an empty stub).
- [ ] **Differential harness (recommended):** compile `app/Core/**` into a tiny Swift CLI (as `tests/run-tests.sh` already does) that reads inputs and dumps JSON for word counts, scans, status, CapturePlacement, carry-over, partition, review markdown, search hits, redactor, digest; check the dumps in as Go golden files.

---

## 6. Release and distribution

### 6.1 Build today (`app/build.sh`)
- Needs only Xcode Command Line Tools (no Xcode, no SwiftPM, no Node: editor ships prebuilt in `app/Resources/editor/index.html`, committed).
- `swiftc -O -parse-as-library -swift-version 5 -target <arch>-apple-macos13.0` over `Core/**` + `UI/**`, arm64 and x86_64 in parallel, `lipo -create` into `Gloamlog.app/Contents/MacOS/Gloamlog` (`GLOAMLOG_NATIVE=1` = this arch only). Minimum macOS 13.0.
- Copies `AppIcon.icns`, `editor/`, `claude/`, `../THIRD_PARTY_LICENSES.md` into Resources; writes Info.plist inline (bundle id `io.github.dasariprashant0.gloamlog`, `CFBundleShortVersionString`/`CFBundleVersion` both `0.6.6`, `CFBundleURLTypes` scheme `gloamlog`, `LSMinimumSystemVersion` 13.0, `LSApplicationCategoryType` `public.app-category.productivity`, `NSHighResolutionCapable`, `NSUserNotificationAlertStyle` none; no `LSUIElement`, no usage descriptions).
- Version string appears in `build.sh` twice (release.sh greps `<string>$V</string>`) and in `AppDelegate.showAbout` fallback `"0.6.6"`.
- `codesign --force --sign - Gloamlog.app` (**ad-hoc; not notarised, no hardened runtime, no entitlements**). `ditto -c -k --keepParent Gloamlog.app Gloamlog.zip`.
- Editor rebuild: `cd editor-web && npm ci --ignore-scripts && npm run build` (esbuild writes the single HTML; `npm run licenses` regenerates `THIRD_PARTY_LICENSES.md`).
- CI (`.github/workflows/ci.yml`): `macos-latest`: `bash tests/run-tests.sh`, `bash app/build.sh`, upload `app/Gloamlog.zip`. `pages.yml`: deploys `site/` (static site) on push. Dependabot present. editor-check / kill-test / editor-web tests are not in CI.

### 6.2 Release (`scripts/release.sh <version> "<notes>"`)
1. Version bumped in `app/build.sh` first (script checks). 2. `bash app/build.sh`. 3. `(cd app && shasum -a 256 Gloamlog.zip > Gloamlog.zip.sha256)` (content `<64 hex>  Gloamlog.zip`). 4. `gh release create v<version> app/Gloamlog.zip app/Gloamlog.zip.sha256 --title "Gloamlog <version>" --notes ...`. 5. `bash scripts/update-tap.sh <version>`.
- `scripts/update-tap.sh`: downloads `Gloamlog.zip.sha256` of the release (`gh release download`), clones `git@github.com:dasariprashant0/homebrew-gloamlog.git` into `.tap-update.XXXXXX` inside the repo (so the personal SSH key/identity apply), `sed -i ''` rewrites `version "..."` and `sha256 "..."` lines in `Casks/gloamlog.rb`, commits `gloamlog <version>`, pushes. Cask file content is not in this repo (see Q10). Release assets must be exactly `Gloamlog.zip` and `Gloamlog.zip.sha256`; the zip's single top-level entry is `Gloamlog.app`.
- Repo `dasariprashant0/gloamlog`; demo MP4 is attached to release `v0.6.6` (`gloamlog-demo.mp4`) and linked from README.

### 6.3 Installer (`install.sh`) and Homebrew
- `curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash`; flags `--version X`, `--uninstall`, `-h`. Requires macOS >= 13 (`sw_vers`). Test hooks: `GLOAMLOG_API_URL`, `GLOAMLOG_DEST`. Dest `/Applications` if writable else `~/Applications`.
- Flow: GET release JSON (`/releases/latest` or `/releases/tags/v<X>`), grep `tag_name`, `browser_download_url` for `Gloamlog.zip` and `.zip.sha256` (both required), download, compare SHA-256 (stop on mismatch), `ditto -x -k`, verify `CFBundleIdentifier == io.github.dasariprashant0.gloamlog`, `CFBundleShortVersionString == tag`, `codesign --verify --deep --strict`, quit running app via `osascript -e 'tell application id ... to quit'` (up to 10 s, never force-kill), move old to `.old`, `ditto` new, restore old on failure, `xattr -dr com.apple.quarantine`. Uninstall removes the `.app` only and prints `claude mcp remove gloamlog -s user`.
- Homebrew: `brew install --cask --no-quarantine dasariprashant0/gloamlog/gloamlog` (own tap, because the main cask list needs notarisation).
- Manual: unzip, drag to Applications; first launch of a browser-downloaded copy needs Privacy & Security > Open Anyway.

### 6.4 In-app updater feed format (`Core/UpdateCheck.swift`, `UI/Updater.swift`)
- Feed = GitHub "latest release" JSON (see 2.5). Requires non-draft, non-prerelease, asset `Gloamlog.zip`; checksum asset optional for the check but **required to auto-install** (else only opens the release page: "This release has no checksum file, so Gloamlog won't install it by itself."). `isNewer`: numeric dotted versions, non-numeric parts = 0.
- Install: both asset URLs must be https on `github.com`/`*.github.com`; app's own folder must be writable; download checksum file, parse first token (64 hex, lower-cased); download zip (progress bar from `URLSessionDownloadTask.progress`); SHA-256 must match; `ditto -x -k`; new bundle's `CFBundleIdentifier` must equal the running app's and version must equal the release's; `codesign --verify --deep --strict`; then a detached `/bin/sh` script (args: pid, new path, dest path) waits for this pid to exit (<= 60 s), moves old to `<dest>.old`, `ditto` new in (restore old on failure), strips quarantine, `open`s the new app, deletes the work dir; app calls `NSApp.terminate` (the quit-flush path runs). Files downloaded by the app carry no quarantine flag.
- Trust model: GitHub + checksum only (no signature).

### 6.5 What changes when the binary becomes a Wails app
- [ ] **Bundle/identity:** decide bundle id (keep `io.github.dasariprashant0.gloamlog` to share defaults, `SMAppService` login item, notification permission, TCC grants, Homebrew/installer identity checks) -- installer and updater both hard-check the id.
- [ ] **Build:** `wails build` (Go + frontend) replaces `swiftc`; universal (arm64+amd64) via `-platform darwin/universal`; keep Info.plist keys (URL scheme, min version, category); keep `Contents/Resources/claude/gloamlog-mcp.py` path (R3); bundle `THIRD_PARTY_LICENSES.md` (add Go/JS dependency licences, keep Milkdown etc.).
- [ ] **Release scripts:** `release.sh` version grep targets `app/build.sh` strings; update to the new version source. `update-tap.sh` unchanged if asset names and zip layout stay (`Gloamlog.app` top level).
- [ ] **Updater/installer:** same asset names and checksum; the swap script works unchanged for any `.app`; Go reimplementation of the check (`net/http`, `crypto/sha256`, `ditto`/`codesign` via `os/exec`), keep host allow-list and bundle-id/version checks.
- [ ] **Signing:** still ad-hoc unless a Developer ID is introduced; Wails/Go binaries must be re-signed after bundling (`codesign --force --deep --sign -`) or `codesign --verify --deep --strict` in installer/updater fails; UserNotifications and SMAppService behave differently for ad-hoc vs Developer ID.
- [ ] **CI:** `go test ./...` + the Swift-generated golden check + Node editor tests; macOS runner for `wails build`.
- [ ] **Process name:** Claude Stop hook and `install.sh` use `pgrep -x Gloamlog` and `osascript ... application id`; keep executable name `Gloamlog`.
- [ ] **Docs/site/INSTALL/README** mention Swift/`swiftc`/Command Line Tools build; update build-from-source instructions.

---

## 7. Open questions and ambiguities

- **Q1 Settings store.** UserDefaults binary plist vs Go-owned config; same vs new bundle id; how to read `Data` blobs and Swift-encoded dates; whether the Swift app and the Wails app may run side by side against the same notes folder (they would also share `Application Support/Gloamlog/{capture,inbox,backups}`; two drainers on one inbox/journal could double-add: the journal is idempotent per id but not across two journals).
- **Q2 Hotkey "Off" bug (confirmed by a 10-line Swift repro).** Synthesized `Codable` omits `nil`, decoder treats a missing key as the default, so Off is lost on relaunch. Decide: fix in the port (explicit `null`) while still accepting old files.
- **Q3 Inbox `ts` is ignored.** A Claude entry is placed on the day and with the `HH:mm` of the moment the app drains the inbox (activation, 15 s timer, launch), not when it was written. Entries written while the app is closed land on "today" at next launch; around midnight an entry can land on the wrong day. Keep (parity) or honour `ts` (UTC) in the port?
- **Q4 Two "heading" settings.** `capture.jotsHeading` (default `Jots`; Claude entries and Jots) and `sources.heading` (default `What I did`; suggestions/auto-add). The `SuggestionsStrip` header comment and Settings placeholder say `Worked on` is the default; the code default is `What I did` (`SourcesPrefs.defaultHeading`, test-pinned by comment). `ClaudeSetup.connect` only flips `jotsHeading`.
- **Q5 About says "No network requests"** (`SettingsPaneAbout.swift`) while the Updater makes one on click; credits text in the About panel says the update check is the only one. README/PRIVACY are consistent with the latter.
- **Q6 `launchAtLogin` value** is stored in settings JSON but the truth is `SMAppService` status; the stored bool is rewritten from the status after each change. Default true in the struct is overridden to false until onboarding completes.
- **Q7 Test claims vs repo.** README says the core "has automated tests"; large parts (Sources, Sync, Review, Export, Schedule, DayWriter) have none committed. `ScenarioKit.swift` mentions scenario files that do not exist. The `editor-check` "six tabs" check text predates the seventh pane (About).
- **Q8 Docs vs code.** `docs/EDITOR_CONTRACT.md` does not list the host-composed `replace`, `insert`, `openTemplateLines`, `reveal` scripts (they live in `EditorBridge.swift`). `docs/FILE_STRUCTURE.md` says `Gloamlog.app`/`.zip` are "generated, not committed" but both are tracked in git (`git ls-files app`). `docs/v1/TRD.md` D-F plans `schemaVersion`; none exists. `docs/ARCHITECTURE.md` is the single-file v0.1 app.
- **Q9 Real-world verification.** CHANGELOG "Known issues": quick capture over full-screen apps, notifications with the app quit, real iCloud placeholders, and Sources have not been exercised on a real Mac; `ClaudeCodeSource` field names are "assumptions, all unverified against real files" in its own header. The port inherits these as unverified parity targets; collect real transcript fixtures.
- **Q10 Homebrew cask content** (`Casks/gloamlog.rb` in `homebrew-gloamlog`) is not in this repo: its `url`, `app`, `uninstall`/`zap` stanzas and caveats are unknown here; only `version`/`sha256` lines are edited by `update-tap.sh`.
- **Q11 Python dependency.** The MCP helper needs `python3` at Claude Code's runtime (CLT stub on a clean Mac). Replace with a Go subcommand? Existing registrations point to `.../Resources/claude/gloamlog-mcp.py` run by `python3`; the file must keep working or the status shows `moved`/broken.
- **Q12 Unicode/locale fidelity.** Word counts use Swift's `generalCategory` tables and Han/Hiragana/Katakana ranges; search uses `NSString` case + diacritic folding; `Fmt.time`, `DateJump` numeric order and weekday names depend on locale/calendar (`en_US_POSIX` for names, `calendar.locale` for field order). Go's Unicode version and `x/text` folding may differ on rare characters; decide the supported locale behaviour (frontend `Intl` vs Go tables) and test the boundary cases.
- **Q13 Calendar semantics.** Week start via `Calendar.dateInterval(of: .weekOfYear)` with overridden `firstWeekday`; ISO week number only for file names (`Gloamlog 2026-W41` uses the week's Thursday-ish mid day `start+3` in an `iso8601` calendar).
- **Q14 Global hotkey clashes** with other apps cannot be detected (code comment, CHANGELOG known issue).
- **Q15 PDF export** depends on WKWebView print-to-PDF with an off-screen window; unclear how Wails exposes this without a second webview.
- **Q16 Notification permission and ad-hoc signing.** `Notifier.center` returns nil for a non-`.app` bundle; ad-hoc apps may be denied. Behaviour on a Wails-built bundle is unverified.
- **Q17 Window/menu parity.** SwiftUI `Commands` provide system menus (Edit text-editing group is replaced by Search items) and `⌘[`/`⌘]` interception before the web view; Wails menu accelerators and WKWebView key handling may order differently.
- **Q18 Not specified anywhere:** maximum notes-folder size/perf budgets (docs/v1/TRD §7 lists budgets; search ~85 ms at 10 years of synthetic logs per CHANGELOG), what happens to the `dailylog.*` defaults of someone who never upgraded past v0.2 (covered by tolerant decode), and whether Settings "Reset" should also reset sources (it does not: Storage, Shortcuts, Sources, About have no reset).
