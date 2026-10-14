# Gloamlog Design System v2 (for v0.2)

Native macOS, SwiftUI, macOS 13+, no third-party dependencies, no Swift macros (no `@Observable`, no `#Preview`; state is `ObservableObject` + `@StateObject` / `@Published`, plus `@AppStorage` and `@Environment`, as in v1).

> **Verification status: nothing here has been seen on screen.** v1 compiles but has never been launched or visually checked. This document is a spec, not a record. Contrast ratios were computed (WCAG 2.x relative luminance; spot-checked with the antislop contrast tool, which matched). Spacing, motion feel, dark-mode appearance, icon legibility at 16px and every "proposed" component are unverified until a running build is inspected in light, dark and Increase Contrast.

## 0. Implemented vs proposed

| Area | Status |
|---|---|
| Sidebar list (today + past days, logged/not logged), page header (title, date), 5 text blocks, Save, "n/5 filled", reminder picker, Open folder | **Implemented in v1** (`app/Gloamlog.swift`), restyled by this spec |
| Everything else below: palette/Theme.swift, section card with check, progress ring, sticky save bar, streak chip, Yesterday card, month grouping, skipped/missed states, heatmap, search, weekly review, menu bar item, Settings window, onboarding, banners, motion, app icon | **Proposed, not built** |
| Streak, skip, search and weekly review contradict PRD v1 non-goals; they are v0.2 scope and need the PRD updated | **Decision needed** |

## 1. Principles

1. **Calm page, one signal.** The page is paper. Green appears only where something is done or selected. Warm amber and red are reserved for "needs attention" and "failed".
2. **The margin is the progress bar.** Each section carries its own completion mark in the gutter. The whole-page progress ring is a summary of those marks, never the only indicator.
3. **Document first.** Text is large, rows are quiet, cards have a hairline border and no shadow. Nothing floats except the save bar's top rule and popovers.
4. **Honest states.** Missed, skipped and off-day are three different things and look different (shape, not just colour). No invented numbers: every figure is computed from files on disk.
5. **System-native where it matters.** System fonts, SF Symbols, native `Form`, `Settings`, `MenuBarExtra`, `.help` tooltips, system focus behaviour. Custom only for colour, the ring, and the heatmap.
6. **Colour is never the only carrier.** Every status has its own SF Symbol shape and a text or tooltip equivalent.

Anti-slop choices, on record: one accent, no gradients, no glow, no glass/blur surfaces, no left-edge colour stripes, no decorative dots, no emoji in the UI chrome, no tracked-uppercase labels, no fake stats. Radius is a three-step scale; pills are only for chips.

## 2. Colour

One neutral family (cool grey-green, hue ~150) plus one accent, "ledger green". Pure black and pure white are not used for backgrounds or text (surface cards are `#FFFFFF` in light only as the one elevated plane).

### 2.1 Palette

| Token | Light | Dark | Use |
|---|---|---|---|
| `bg` | `#F9FAF9` | `#141716` | Window / detail background |
| `surface` | `#FFFFFF` | `#1C201E` | Section cards, search field, popovers |
| `sidebar` | `#F0F2F0` | `#101312` | Sidebar background, Yesterday card |
| `border` | `#DCE1DD` | `#2E3532` | Hairlines, card borders, dividers |
| `borderStrong` | `#BCC5BF` | `#4A534F` | Card hover, ring track, unchecked gutter circle |
| `hover` | `#E9EDEA` | `#252B28` | Row hover, disabled button fill |
| `textPrimary` | `#1B201E` | `#E8ECEA` | Titles, body, editor text |
| `textSecondary` | `#566059` | `#A3ADA7` | Dates, labels, helper text |
| `textTertiary` | `#636D67` | `#8A948E` | Placeholders, metadata, counts |
| `accent` | `#0E7A5F` | `#45C29C` | Primary button, check fills, ring, selection |
| `onAccent` | `#FFFFFF` | `#06241C` | Text/icon on accent fill |
| `accentHover` | `#0B6A52` | `#5BD0AB` | Button hover |
| `accentPressed` | `#095A45` | `#7BE0BF` | Button pressed |
| `accentText` | `#0B6A52` | `#45C29C` | Accent-coloured text on tints (chips, "Saved") |
| `accentTint` | `#DDF0E8` | `#1B3A31` | Selected row, chip bg, success banner, search match highlight base |
| `success` | = `accent` | = `accent` | Logged state (same hue by design) |
| `warning` | `#9A5B00` | `#E0A23A` | Missed day glyph, warning banner icon |
| `warningTint` | `#FBF0DC` | `#3A2E14` | Warning banner bg |
| `danger` | `#B3261E` | `#F0847A` | Save error, destructive text |
| `dangerTint` | `#FBE9E7` | `#3D1F1C` | Error banner bg |
| `heat0`..`heat4` | see 2.3 | see 2.3 | Heatmap ramp |
| `focusRing` | `accent` @ 30% alpha, 3pt | `accent` @ 40% alpha, 3pt | Outside ring on focused editor/field |

Opacity-based fills (`Color.primary.opacity(0.05)`) from v1 are retired: they give unpredictable contrast.

### 2.2 Contrast (computed)

Normal text needs 4.5:1, large text and non-text UI 3:1.

| Pair | Light | Dark |
|---|---|---|
| `textPrimary` on `bg` | 15.78 | 15.14 |
| `textPrimary` on `surface` | 16.51 | 13.82 |
| `textPrimary` on `sidebar` | 14.67 | 15.68 |
| `textPrimary` on `hover` | 13.97 | 12.11 |
| `textSecondary` on `bg` / `surface` / `sidebar` | 6.25 / 6.54 / 5.81 | 7.81 / 7.13 / 8.09 |
| `textSecondary` on `hover` | 5.53 | 6.25 |
| `textTertiary` on `bg` / `surface` / `sidebar` | 5.13 / 5.37 / 4.77 | 5.76 / 5.26 / 5.97 |
| `textTertiary` on `hover` | 4.54 | not checked |
| `accent` on `bg` / `surface` / `sidebar` | 5.06 / 5.29 / 4.70 | 8.12 / 7.41 / 8.41 |
| `onAccent` on `accent` (primary button) | 5.29 | 7.40 |
| `onAccent` on `accentHover` | 6.57 | 8.66 |
| `accentText` on `accentTint` | 5.5 (approx, from 0B6A52) | 5.57 |
| `textPrimary` / `textSecondary` on `accentTint` (selected row) | 13.92 / 5.51 | 10.39 / 5.36 |
| `warning` on `bg` / on `warningTint` | 5.19 / 4.81 | 8.08 / 5.95 |
| `danger` on `bg` / on `dangerTint` | 6.25 / 5.58 | 7.12 / 5.87 |
| `textPrimary` on `warningTint` / `dangerTint` | 14.62 / 14.08 | 11.14 / 12.48 |

Known gaps, stated plainly:
- `textTertiary` on `accentTint` in dark is **3.95:1** (fails AA for small text). Selected rows therefore use `textSecondary` or `textPrimary`, never `textTertiary`.
- `accent` (`#0E7A5F`) directly on `accentTint` is 4.46:1 (just under). Text on tints must use `accentText`, not `accent`.
- `border` on `bg` is 1.27:1 and `borderStrong` on `surface` is 1.77:1 (light) / 2.07:1 (dark). Both are below the 3:1 WCAG 1.4.11 target for input boundaries. The section card edge is decorative: the editor is identified by its placeholder, its label, hover (`borderStrong`) and the focus ring (`accent`, 5.06:1). If real use shows the cards are hard to find, add an Increase Contrast branch via `@Environment(\.colorSchemeContrast)` that swaps `border` for a `#8D9892` / `#7B8680` boundary (about 3:1). Not verified.
- Disabled controls are exempt from contrast rules but must still be legible by shape and label.

### 2.3 Heatmap ramp (5 steps, sequential, one hue)

Step meaning: number of sections filled on that weekday. Skipped and future are drawn differently (section 6.9).

| Step | Meaning | Light | Dark |
|---|---|---|---|
| `heat0` | no file / 0 filled | `#E6EBE8` | `#222826` |
| `heat1` | 1-2 filled | `#BFE3D3` | `#17493B` |
| `heat2` | 3 filled | `#7CC6A9` | `#1F7059` |
| `heat3` | 4 filled | `#2E9F7E` | `#2D9C7D` |
| `heat4` | 5 of 5 (logged) | `#0E7A5F` | `#5FD6B2` |

Cell vs `surface` contrast (light): 1.21, 1.39, 2.00, 3.29, 5.29. (dark): 1.10, 1.61, 2.76, 4.83, 9.22. Steps 0 to 2 are low contrast against the card by design (they read as "not done"); the 5-of-5 step is the one that must pop and does. Each cell also has a tooltip, so the ramp is never the only way to read a value.

## 3. Typography

System fonts only: SF Pro for UI and body, SF Mono for file paths. Use `.monospacedDigit()` on every count, date number and time so columns do not jitter.

| Role | SwiftUI | Size / weight | Tracking | Line spacing |
|---|---|---|---|---|
| Page title (the day) | `.system(size: 32, weight: .semibold)` | 32 / 600 | -0.5 (`.tracking(-0.5)`) | default |
| Page subtitle | `.system(size: 13, weight: .regular)` | 13 / 400, `textTertiary` | 0 | default |
| Section label | `.system(size: 15, weight: .semibold)` | 15 / 600 | 0 | default |
| Editor text | `.system(size: 15, weight: .regular)` | 15 / 400 | 0 | `.lineSpacing(4)` (about 1.5) |
| Placeholder | same as editor, `textTertiary` | 15 / 400 | 0 | same |
| Body / banner / settings | `.system(size: 13)` | 13 / 400 | 0 | default |
| Button label | `.system(size: 13, weight: .semibold)` | 13 / 600 | 0 | n/a |
| Sidebar row | `.system(size: 13)`; today and selected `.medium` | 13 / 400-500 | 0 | n/a |
| Sidebar month header | `.system(size: 12, weight: .semibold)`, `textTertiary` | 12 / 600 | 0 | n/a |
| Meta / count / tooltip text | `.system(size: 11)` + `.monospacedDigit()` | 11 / 400-500 | +0.1 | n/a |
| Path | `.system(size: 12, design: .monospaced)` | 12 / 400 | 0 | n/a |
| Onboarding headline | `.system(size: 28, weight: .semibold)` | 28 / 600 | -0.4 | default |

Rules: no all-caps labels, no letter-spaced headings, no serif. Emphasis is weight, not colour. Minimum text size 11. The text view and window follow user font settings only as far as macOS allows (macOS has no Dynamic Type); sizes are fixed.

**Section titles on disk contain emoji** (`## 📝 What I did`) and are the parse keys in `Store.load`. Do not change them in the file format. The UI shows a derived `displayTitle` with the leading emoji and space stripped ("What I did"). Keep `sections` titles as-is for I/O and add a `displayTitle` computed property.

## 4. Spacing, radii, elevation

**Grid: 4pt.** Allowed values: 4, 8, 12, 16, 20, 24, 32, 40, 48, 64. Anything else needs a written reason.

| Token | pt | Use |
|---|---|---|
| `space1` | 4 | Icon-to-text gap inside chips |
| `space2` | 8 | Row gaps, button icon gap |
| `space3` | 12 | Card inner vertical gap, sidebar row padding V=6 pairs |
| `space4` | 16 | Card padding, banner gap |
| `space5` | 20 | Gutter circle column |
| `space6` | 24 | Between section cards |
| `space8` | 32 | Header to first card |
| `space10` | 40 | Page vertical padding |
| `space12` | 48 | Page horizontal padding |

| Radius token | pt | Use |
|---|---|---|
| `radiusSm` | 4 | Heatmap cells (3 is allowed there), search match highlight |
| `radiusMd` | 6 | Buttons, search field, sidebar rows, inputs |
| `radiusLg` | 10 | Section cards, Yesterday card, banners, sheets |
| `radiusFull` | capsule | Streak chip, onboarding step bar only |

Use `RoundedRectangle(cornerRadius:, style: .continuous)` everywhere.

| Elevation | Spec | Use |
|---|---|---|
| 0 flat | border 1pt `border` only | Cards, sidebar, header |
| 1 raised | light: `0 1 2 rgba(0,0,0,0.06)`, `0 4 12 rgba(0,0,0,0.08)`; dark: no shadow, border lightens to `borderStrong` | Popovers, menus, custom tooltip (if any) |
| 2 overlay | light: `0 8 24 rgba(20,30,26,0.14)`; dark: `0 8 24 rgba(0,0,0,0.5)` | Onboarding sheet (system sheet shadow is fine; do not override) |

Shadows are tinted toward the neutral hue, never pure black on light. The save bar uses a top hairline (`border`), not a shadow.

## 5. Window and layout

| Item | Value |
|---|---|
| Window | default 1000x760, min 860x600 (v1: 980x740, min 820x600) |
| Sidebar | width 240 ideal, min 220, max 280 |
| Detail column | content max width 720, centred horizontally in the detail pane |
| Page padding | 48 horizontal, 40 top, 24 bottom (above save bar) |
| Header to first card | 32 |
| Between section cards | 16 (24 before the save bar) |
| Save bar | height 64, pinned bottom, `bg`, 1pt top `border` |

Centred reading column (not leading-aligned as v1) because the detail pane is wide and the content is a single 720pt document. Cards and header share the same 720 column.

## 6. Components

### 6.1 Page header with date and streak chip

```
[ Tuesday, 6 October                         ] [ 6 day streak ]
[ Daily log, 2026                            ]
```

- Row: `HStack(alignment: .firstTextBaseline)`; title left, chip right, `Spacer` between.
- Title: page-title style, `textPrimary`. Date format `EEEE, d MMMM`. Subtitle: `Daily log` + year, 13pt `textTertiary`, 4pt below.
- Past day: title is that day's date; add year to the subtitle only if it differs from the current year.
- Streak chip: height 24, horizontal padding 10, capsule, fill `accentTint`, text 12pt semibold `accentText`, `.monospacedDigit()`. Text `"N day streak"`, no icon. Shown only when N >= 2 and only on the Today view. Tooltip: "Consecutive logged weekdays. Skipped days do not break it." (Rule is proposed; decide in the PRD.)
- A 1pt `border` rule sits 24 below the subtitle, then 32 to the first card.
- States: no chip when streak < 2; if today is the 1st log ever, no chip. VoiceOver: chip reads "6 day streak".

### 6.2 Section card (empty / focused / filled)

Structure (top to bottom inside a card of full column width):

```
[gutter mark 20x20] [Section label 15/600]                  
[ editor, min height 84 ]
```

- Card: `surface`, 1pt `border`, `radiusLg` 10, padding 16 all sides (bottom 14). Label row height 22, 12 below it the editor.
- Editor: `TextEditor`, `.scrollContentBackground(.hidden)`, no own background, 15pt, `lineSpacing(4)`, min height 84 (3 lines), left inset 0 (card padding provides the margin). Placeholder overlay `textTertiary`, top-left, `allowsHitTesting(false)`. `.accessibilityLabel(displayTitle)` and hint on the editor itself (placeholder is not exposed).
- **Gutter mark (completion check):** 20x20 circle at the leading edge of the label row.
  - Empty: 1.5pt stroke `borderStrong`, no fill.
  - Filled (trimmed text non-empty): fill `accent`, centred SF Symbol `checkmark` 10pt bold in `onAccent`. Transition: fill scales 0.6 to 1.0 with the check drawn by opacity, 160ms ease-out (section 8). Under Reduce Motion: instant.
  - Whitespace-only counts as empty (same as v1).
- States:

| State | Border | Fill | Other |
|---|---|---|---|
| Empty, idle | `border` 1pt | `surface` | placeholder visible, gutter empty |
| Empty, hover | `borderStrong` 1pt | `surface` | cursor I-beam over editor |
| Focused (any content) | `accent` 1.5pt | `surface` | outside ring `focusRing` 3pt; label stays `textPrimary` |
| Filled, idle | `border` 1pt | `surface` | gutter filled |
| Filled, hover | `borderStrong` 1pt | `surface` | gutter filled |

- Focus order: top to bottom, Tab moves editor to editor. Because `TextEditor` swallows Tab, document that Tab inserts a tab character unless the implementer intercepts; recommendation: leave default and rely on ⌘-arrow / click, mark as unverified.
- Spacing note: the card, not the heading above a grey box (v1), is the unit. No section heading outside the card.

### 6.3 Completion ring and progress

- Used once: in the save bar. Diameter 28, stroke 3, round caps. Track `borderStrong`, progress `accent`, start at 12 o'clock, clockwise, fraction `filled / 5`.
- Centre text: `filled` as 11pt semibold `textPrimary` `.monospacedDigit()`. At 5 of 5 the number is replaced by SF Symbol `checkmark` 11pt bold `accent`.
- Label to its right (12 gap): `"3 of 5 sections"` 13pt `textSecondary`. At 5: `"All five done"`. After save: `"Saved"` (see 6.4).
- Animation: `trim` end value animates 240ms ease-out per change. Reduce Motion: no animation.
- Accessibility: one element, `.accessibilityLabel("Progress")`, value `"3 of 5 sections filled"`.

### 6.4 Save button states

Lives in the save bar, trailing. Height 32, min width 96, horizontal padding 16, `radiusMd`, label 13pt semibold. Use a custom `ButtonStyle` (not `.borderedProminent`, which takes the system accent).

| State | Fill | Label | Notes |
|---|---|---|---|
| Disabled (`filled < 5`) | `hover` | `textTertiary` "Save log" | not focusable; tooltip lists missing sections: "Still needed: Finished, To do next" |
| Enabled | `accent` | `onAccent` "Save log" | default action, `.keyboardShortcut(.defaultAction)` and ⌘S |
| Hover | `accentHover` | same | |
| Pressed | `accentPressed`, scale 0.98 | same | 80ms |
| Saving | `accent` at 70% | "Saving" + 12pt `ProgressView` | only if write exceeds 150ms; local write normally does not |
| Saved (clean) | `accentTint` | `accentText` "Saved" + `checkmark` 11pt | disabled until the next edit, holds as steady state |
| Dirty after save | back to Enabled with label "Save changes" | | any edit clears Saved |
| Error | Enabled | | error goes in a banner (6.14), not next to the button |

### 6.5 Yesterday card

Shown above the first section card on **Today only**, when yesterday's file exists and is logged. Purpose: continuity (what was pending, what is next).

- Container: fill `sidebar`, no border, `radiusLg` 10, padding 16, 24 below before the first section card.
- Header row: `"From yesterday"` 13pt semibold `textSecondary`, trailing `"Mon 5 Oct"` 12pt `textTertiary`, trailing-most a disclosure chevron (`chevron.down` / `chevron.right`, 10pt) to collapse. Collapsed state persists (`@AppStorage("yesterdayCollapsed")`).
- Body (expanded): two blocks, `Pending / blocked` then `To do next`, each: label 12pt semibold `textTertiary`, text 14pt `textSecondary` max 4 lines with `"Show all"` text button (13pt `accentText`) when truncated. 12 between blocks.
- Per-block trailing action: `doc.on.doc` 12pt, tooltip "Copy", copies the text to the pasteboard.
- Hidden when: no yesterday file, yesterday not fully logged, yesterday was skipped, or today is Monday and the previous workday is used instead (use the last logged workday, label with its date).
- Honest-state rule: no placeholder content ever; if the source is empty the card is not shown.

### 6.6 Sidebar rows, month grouping

Search field at top (6.10), then a pinned `This week` row (6.11), then month groups.

Group structure: month header, then rows, newest first. The current month is expanded; the previous month expanded; older months collapsed by default; expand state is session-only.

- Month header: height 28, top padding 12 (first group 0), `"October 2026"` 12pt semibold `textTertiary`, `chevron.down` / `chevron.right` 9pt leading, 6 gap. Whole row clickable. Trailing: `"4/5"` weekday-logged count, 11pt `textTertiary` mono digits, only when collapsed.
- Row: height 30, horizontal padding 10, `radiusMd`, 1pt vertical gap between rows, `HStack(spacing: 8)`.
  - Glyph column 16 wide, 14pt SF Symbol (monochrome / palette as below).
  - Title 13pt: `"Today"` for the current day (weight `.semibold`), else `"Mon 5 Oct"` (`EEE d MMM`). Weekday abbreviations are part of the day label because month grouping removed the full date.
  - Trailing (today only, when partial): `"3/5"` 11pt `textTertiary`.
- Row states:

| Status | Symbol | Colour | Meaning |
|---|---|---|---|
| Today, 0 filled | `circle` | `textSecondary` | not started |
| Today, partial | `circle.lefthalf.filled` | `textSecondary` | in progress |
| Logged (5/5) | `checkmark.circle.fill` | `accent` | complete |
| Skipped | `minus.circle` | `textTertiary` | user chose to skip |
| Missed (past weekday, no log, not skipped) | `circle.dashed` | `warning` | no log |
| Selected | any of the above on `accentTint` fill, title `textPrimary` `.medium` | | no stripe, no outline |
| Hover | `hover` fill | | |

- Weekends and unlisted days are not rows. A weekend with a file appears as Logged.
- Collapsed month groups hide rows but never hide Today.
- VoiceOver label per row: `"Monday 5 October, logged"` / `"skipped"` / `"missed"`.
- Today is always first row in the current month.

### 6.7 12-week heatmap

Placement: bottom of the sidebar (below the list, pinned) and also at the head of the Weekly review. Mon-Fri only, matching the reminder rule.

- Grid: 12 columns (weeks, oldest left) by 5 rows (Mon at top). Cell 14x14, gap 3, `radiusSm` 3. Total 201 x 82. Pinned container padding 16, top hairline.
- Fill: ramp per 2.3. Future days: no fill, 1pt `border` outline. Skipped: `heat0` fill plus a 1pt `textTertiary` horizontal dash (`Rectangle` 6x1.5 centred), so skipped is distinguishable from missed without colour. Missed: `heat0` (plain). Today: add 1.5pt `accent` outline (outer).
- Legend beneath (4 gap): `"Less"` 11pt `textTertiary`, 5 swatches 10x10 gap 3, `"More"`. Sits on one line, right-aligned.
- Tooltip: `.help("Tue 6 Oct: 5 of 5 sections")`. Skipped: `"Tue 6 Oct: skipped"`. Missed: `"Tue 6 Oct: not logged"`. Native `.help` only (appears after the system delay, about 1s). No custom tooltip view.
- Accessibility: the whole grid is one element with label `"Last 12 weeks, 41 of 60 weekdays logged"` (computed); individual cells not exposed.
- Hover: cell scales to 1.0 with `borderStrong` outline? No. Hover only shows the tooltip (no motion, avoids jitter).
- Empty (new user): cells render as `heat0`; caption `"Your first weeks fill in as you log."` 11pt `textTertiary`.

### 6.8 Search field and results row

- Field: top of sidebar, margin 12, height 28, `radiusMd`, fill `surface`, 1pt `border`, padding horizontal 8. Leading `magnifyingglass` 12pt `textTertiary`, 6 gap, text 13pt, placeholder `"Search logs"` `textTertiary`. Trailing `xmark.circle.fill` 12pt `textTertiary` when non-empty. Focus: border `accent`, ring `focusRing`. `⌘F` focuses it.
- Behaviour: debounce 150ms; scans the markdown files in `~/Gloamlog` (case- and diacritic-insensitive); minimum 2 characters.
- While a query is active the sidebar list is replaced by results; clearing returns the month list.
- Result row: min height 52, padding 10, `radiusMd`, same hover/selected fills as sidebar rows. Line 1: `"Mon 5 Oct"` 13pt semibold. Line 2: snippet 12pt `textSecondary`, max 2 lines, about 90 characters centred on the match. The match run: `textPrimary` on `accentTint`, `radiusSm` 4 background (implemented with `AttributedString` `backgroundColor`). Line 3: section label `"Pending / blocked"` 11pt `textTertiary`.
- A day with several hits shows its best snippet plus `"+2 more"` 11pt `textTertiary`.
- Empty: `"No logs mention “xyz”."` 13pt `textSecondary` and below `"Search covers every day in ~/Gloamlog."` 12pt `textTertiary`. Error (folder unreadable): banner 6.14.

### 6.9 Weekly review layout

Reached from the pinned `This week` sidebar row (`calendar`, 14pt). Read-only, built only from saved files (no network, no model).

```
Week of 5 October                       [ Copy as markdown ]
4 of 5 weekdays logged
[ 12-week heatmap ]   (6.7, full width of column, cell 18, gap 4)
-------------------------------------------------
Open items          (Pending / blocked, all days this week)
  Mon 5   Waiting on legal for ...
  Wed 7   ...
Finished
  Mon 5   ...
Started
To do next  (latest day only)
```

- Same 720 column. Title 32pt like the page header; week range subtitle 13pt `textTertiary`.
- Summary line: `"4 of 5 weekdays logged"` plus, only if non-zero, `"1 skipped"` and `"1 missed"`, separated by a comma and a space (no decorative separators).
- Order of sections: `Pending / blocked` first (titled "Open items"), then `Finished`, `Started`, `What I did`, and last `To do next` (latest day only). This leads with what needs attention.
- Each section: a card (6.2 container, border only, no editor). Heading 15pt semibold, 12 gap, entries as rows: day column 56pt wide (12pt `textTertiary` mono digits, `"Mon 5"`), text 14pt `textPrimary`, row gap 8. Max 6 lines per entry with `"Show all"`.
- Days with no content for that section are omitted. A section with no entries shows `"Nothing this week."` 13pt `textTertiary`.
- `Copy as markdown` is a standard bordered 28pt-high button (`radiusMd`, border `border`, fill `surface`, 13pt label).
- Empty week: see 6.13.

### 6.10 Menu bar icon states

`MenuBarExtra` (macOS 13). Image is a template (monochrome, adapts to the menu bar). Size 16pt via `.imageScale(.medium)`. Accent colour is not used in the menu bar.

| State | SF Symbol | When |
|---|---|---|
| Logged | `checkmark.circle.fill` | today 5 of 5 saved |
| Pending (before reminder time, or in progress) | `circle` | weekday, not yet logged, reminder time not reached |
| Due / overdue | `circle.lefthalf.filled` | weekday, past reminder time, not logged |
| Skipped | `minus.circle` | user skipped today |
| Off-day | `moon.zzz` | weekend / non-workday (reminder inactive) |

Menu content (native menu, 13pt): status line (disabled item) `"Today: 3 of 5 sections"`; `Open Gloamlog` (`⌘O`); `Skip today` (weekdays only, not when logged); divider; `Settings…` (`⌘,`); `Quit Gloamlog` (`⌘Q`). The symbol shape differs in every state so it does not depend on colour. Updating the icon on the existing 60s timer is enough.

### 6.11 Settings form layout

Native `Settings` scene (macOS 13: open with `NSApp.sendAction(Selector(("showSettingsWindow:")), to: nil, from: nil)`, since `SettingsLink` is macOS 14+). One pane, `Form` with `.formStyle(.grouped)`. Window width 460, height fits content. Replaces the inline reminder picker and Open folder from v1.

| Group | Control | Notes |
|---|---|---|
| Reminder | `DatePicker` "Remind me at" (hour/minute) | default 4:55 PM, `@AppStorage("remindMinutes")` |
| Reminder | Five weekday toggles, Mon to Fri, as `Toggle` with `.toggleStyle(.button)` height 28 | proposed (PRD F15) |
| Reminder | `Toggle` "Bring the window to the front" | on = v1 behaviour |
| Storage | Path label, 12pt mono `textSecondary`, `"~/Gloamlog"` | |
| Storage | `Button` "Show in Finder" | replaces v1 "Open folder" |
| Startup | `Toggle` "Open at login" | `SMAppService.mainApp` register / unregister (v1 registers silently; this makes it visible and reversible) |
| Appearance | `Toggle` "Use system accent colour" | proposed, default off; when on, `accent` tokens map to `Color.accentColor` and tints derive at 14% alpha |

Group header text 13pt semibold (system default in grouped `Form`); helper text under rows 12pt `textSecondary`. Keep labels sentence case. No Save button (changes apply immediately).

### 6.12 Onboarding screens

Shown once (`@AppStorage("onboarded")` false) as a sheet over the main window, 560x440, `radiusLg`, elevation 2. Content padding 40. Step indicator: three capsules 28x4, gap 6, active `accent`, inactive `border`, top-left. `Skip` text button top-right (13pt `textSecondary`). Primary button right-bottom (6.4 style, height 32). Back button (plain text) left-bottom from step 2.

1. **Welcome.** Headline `"Write up your day in five boxes."` (28pt). Body 15pt `textSecondary` max 52 characters per line: `"Gloamlog saves plain markdown files that you own."` Beneath: a live preview of the five section labels as real gutter marks, stacked 8 apart (empty circles; the last one draws its check once, 300ms after appearing; static under Reduce Motion). CTA `Continue`.
2. **Reminder.** Headline `"When should it remind you?"` Time picker (large, 15pt) and the weekday toggles from 6.11. Helper: `"It comes to the front at this time until today's log is saved."` CTA `Continue`.
3. **Where it lives.** Headline `"Your logs stay on this Mac."` Path in a mono pill-less row (`surface` field, 1pt `border`, `radiusMd`, 12pt mono): `~/Gloamlog`. Toggle `Open at login` (explained: `"So the reminder can fire after a restart."`). CTA `Start today's log` closes the sheet and focuses the first editor.

No stock illustrations, no marketing copy. If login-item registration needs approval, show the warning banner (6.14) in step 3 with `"Open System Settings"` action.

### 6.13 Empty states

Each states cause plus next action; none uses an illustration.

| Where | Message | Action |
|---|---|---|
| Sidebar, no past days | `"Past days appear here once you save a log."` (12pt `textTertiary`, padding 16) | none |
| Today, 0 filled | ring at 0, `"0 of 5 sections"`; placeholders in each card; focus lands in the first card on launch | none (the page is the prompt) |
| Weekly review, nothing logged | `"Nothing logged this week yet. Save today's log to start the review."` centred in the column, 15pt `textSecondary` | `Go to today` text button |
| Search, no result | see 6.8 | `Clear search` |
| Heatmap, new user | caption in 6.7 | none |
| Yesterday card | not rendered | n/a |

### 6.14 Banners

Inline, at the top of the detail column (above the page header), full 720 width, `radiusLg` 10, padding 12 vertical / 16 horizontal, `HStack(spacing: 12)`: leading 14pt SF Symbol, body 13pt `textPrimary` (max 2 lines), trailing action buttons (text, 13pt semibold `accentText`) and a `xmark` dismiss (12pt `textTertiary`, 20x20 hit area, tooltip "Dismiss").

| Type | Fill | Icon | Examples |
|---|---|---|---|
| Info | `sidebar` + 1pt `border` | `info.circle` `textSecondary` | `"Reminders are paused on weekends."` |
| Success | `accentTint` | `checkmark.circle.fill` `accentText` | `"Saved to ~/Gloamlog/2026-10-06.md"` (auto-dismiss after 4s) |
| Warning | `warningTint` | `exclamationmark.triangle` `warning` | `"Open at login needs your approval."` + `Open System Settings`; `"2 files in ~/Gloamlog could not be read."` |
| Error | `dangerTint` | `xmark.octagon` `danger` | `"Couldn't save: <localizedDescription>"` + `Try again` |

Rules: one banner at a time (newest replaces), errors never auto-dismiss, banner text always says what failed and what to do. VoiceOver: post an `.announcement` for new banners. Replaces v1's red text beside the Save button.

## 7. Dark mode behaviour

- Follows system appearance (`NSApp.appearance = nil`); no in-app theme toggle in v0.2.
- Implemented with dynamic `NSColor` (section 10), not `colorScheme` branching in views.
- Elevation switches from shadow to lighter borders (section 4). Surfaces get lighter as they go up (`bg` 141716, `surface` 1C201E, `hover` 252B28); the sidebar is the darkest plane (101312), the inverse of light mode where it is the second lightest.
- `accent` brightens (`#0E7A5F` to `#45C29C`) and `onAccent` flips to a dark green so the button text keeps at least 7:1. Heatmap ramp is re-derived, not inverted: step 4 is the brightest.
- No pure black (`#000`) or pure white backgrounds in either appearance.
- **Increase Contrast** (`colorSchemeContrast == .increased`): swap `border` to `borderStrong` for card edges; unverified, see 2.2.
- **Reduce Transparency**: no custom blurs exist, so nothing to degrade (native sidebar vibrancy from `NavigationSplitView` remains system-managed; if the sidebar fill token is applied explicitly, vibrancy is replaced by the flat `sidebar` colour).

## 8. Motion

Principle: motion confirms an action or a state change. Nothing loops, bounces or pulses.

| Moment | Property | Duration | Curve |
|---|---|---|---|
| Section check appears | scale 0.6 to 1, opacity | 160ms | ease-out (`.easeOut(duration: 0.16)`) |
| Ring progress | `trim` end | 240ms | ease-out |
| Border focus change | colour | 120ms | ease-in-out |
| Button hover / press | fill colour / scale 0.98 | 100ms / 80ms | ease-out |
| Saved state | fill swap `accent` to `accentTint` | 200ms | ease-in-out; label cross-fade |
| Row hover | fill | 80ms | linear |
| Banner in / out | opacity + 6pt Y slide | 200ms in, 140ms out | ease-out / ease-in |
| Month group collapse | height | 180ms | ease-in-out |
| Day switch | none (content swaps instantly; `.id(day)` as in v1) | 0 | |
| Onboarding step | opacity cross-fade | 200ms | ease-in-out |

Rules: animate opacity, scale and colour only (not layout geometry, except collapse). Exits are shorter than entrances. Reduce Motion: read `@Environment(\.accessibilityReduceMotion)`; when true, set every duration to 0 (use `nil` animation) except opacity fades, which stay but are capped at 100ms. Implement through one helper `Theme.animation(_:reduce:)` so views never branch.

## 9. App icon brief

> **Superseded (0.6.5).** The shipped icon is "Horizon": a half sun on a horizon over three fading journal lines, on a deep-green to warm-peach dusk sky. It is generated by `bash app/Tools/make-icon.sh` (`app/Tools/make-icon.swift`; the old page-and-check concept below and two other concepts are kept as alternates there). The brief below is the original idea and is kept for history.

**Concept:** a sheet of paper with a few ruled lines and one big check, on a deep ledger-green field. One idea, one shape. "The day, done."

**Construction (1024 x 1024 canvas, origin top-left; flip Y for CoreGraphics):**

| Element | Geometry | Colour |
|---|---|---|
| Body (macOS icon plate) | rect x=100, y=100, 824x824, continuous corner radius 185 (superellipse; approximate with a `UIBezierPath`-style rounded rect radius 185 if `.continuous` is unavailable) | fill linear gradient top `#1B4A3E` to bottom `#102E27` (vertical, 100% to 100%); this is the only gradient and is a light source, not decoration |
| Body shadow | offset y 12, blur 24 | `rgba(0,0,0,0.28)` |
| Body top light | inner 1pt stroke along the top half, inset 1 | `rgba(255,255,255,0.14)` |
| Paper sheet | rect x=272, y=232, 480x560, radius 36 | `#F6F8F6` |
| Sheet shadow | offset y 8, blur 20 | `rgba(0,0,0,0.25)` |
| Rule 1 | rounded rect x=336, y=320, w=380, h=36, radius 18 | `#CBD6D0` |
| Rule 2 | rounded rect x=336, y=404, w=300, h=36, radius 18 | `#CBD6D0` |
| Rule 3 | rounded rect x=336, y=488, w=160, h=36, radius 18 | `#CBD6D0` |
| Check badge | circle centre (624, 656), radius 116 | `#45C29C` |
| Check mark | polyline (570, 660) to (608, 700) to (682, 608), stroke width 40, round caps and joins | `#0B2A22` |

Safe area: all art lies inside the 824 plate with at least 100pt margin to the plate edge except the sheet shadow. Keep the three rules and badge as the only interior detail so it holds at 16px (at 16px the sheet reads as a white card with a green circle; rules may vanish, which is acceptable).

**Programmatic drawing (CoreGraphics):** draw into a 1024x1024 `CGContext`, save PNG, then generate the iconset sizes 16, 32, 64, 128, 256, 512, 1024 (and @2x pairs) with `sips` / `iconutil` inside `build.sh`. Stroke widths scale linearly; at 16 and 32 omit the three rules and the sheet shadow.

**Do not:** add text, add a second accent, add glow, use the system `checkmark.seal` glyph, or tint the paper warm.

## 10. Swift token mapping (Theme.swift)

Dynamic colour helper and a sample of the table below. No macros.

```swift
import SwiftUI
import AppKit

enum Theme {
    static func dyn(_ light: UInt32, _ dark: UInt32) -> Color {
        Color(nsColor: NSColor(name: nil) { a in
            let isDark = a.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
            let v = isDark ? dark : light
            return NSColor(srgbRed: CGFloat((v >> 16) & 0xFF) / 255,
                           green: CGFloat((v >> 8) & 0xFF) / 255,
                           blue: CGFloat(v & 0xFF) / 255, alpha: 1)
        })
    }
    static let bg = dyn(0xF9FAF9, 0x141716)
    // ...the rest per the table
}
```

| Token (`Theme.`) | Value (light / dark, or constant) |
|---|---|
| `bg` | `0xF9FAF9` / `0x141716` |
| `surface` | `0xFFFFFF` / `0x1C201E` |
| `sidebar` | `0xF0F2F0` / `0x101312` |
| `border` | `0xDCE1DD` / `0x2E3532` |
| `borderStrong` | `0xBCC5BF` / `0x4A534F` |
| `hover` | `0xE9EDEA` / `0x252B28` |
| `textPrimary` | `0x1B201E` / `0xE8ECEA` |
| `textSecondary` | `0x566059` / `0xA3ADA7` |
| `textTertiary` | `0x636D67` / `0x8A948E` |
| `accent` | `0x0E7A5F` / `0x45C29C` |
| `onAccent` | `0xFFFFFF` / `0x06241C` |
| `accentHover` | `0x0B6A52` / `0x5BD0AB` |
| `accentPressed` | `0x095A45` / `0x7BE0BF` |
| `accentText` | `0x0B6A52` / `0x45C29C` |
| `accentTint` | `0xDDF0E8` / `0x1B3A31` |
| `warning` | `0x9A5B00` / `0xE0A23A` |
| `warningTint` | `0xFBF0DC` / `0x3A2E14` |
| `danger` | `0xB3261E` / `0xF0847A` |
| `dangerTint` | `0xFBE9E7` / `0x3D1F1C` |
| `heat` (array of 5) | light `E6EBE8 BFE3D3 7CC6A9 2E9F7E 0E7A5F`; dark `222826 17493B 1F7059 2D9C7D 5FD6B2` |
| `space1..space12` | 4, 8, 12, 16, 20, 24, 32, 40, 48 (use names `s1 s2 s3 s4 s5 s6 s8 s10 s12`) |
| `radiusSm / Md / Lg` | 4 / 6 / 10 (cells: 3) |
| `columnMax` | 720 |
| `sidebarWidth` | min 220, ideal 240, max 280 |
| `saveBarHeight` | 64 |
| `ringSize` / `ringStroke` | 28 / 3 |
| `heatCell` / `heatGap` | 14 / 3 |
| `rowHeight` | 30 |
| `fastDur / baseDur / slowDur` | 0.12 / 0.20 / 0.24 |

## 11. Accessibility checklist for v0.2 (all unverified)

- Every status has a distinct SF Symbol and an accessibility label (rows, menu bar, heatmap summary).
- Editors get `.accessibilityLabel` (section display title) and a hint; placeholder text is not the label.
- Ring and heatmap are single accessibility elements with computed values.
- Keyboard: ⌘S save, ⌘F search, ⌘, settings, Tab order card to card; focus ring visible in both appearances.
- Reduce Motion handled through `Theme.animation`; Increase Contrast branch proposed (2.2).
- Hit targets: macOS pointer targets of 24pt or more for icon buttons (banner dismiss 20x20 with 24 padding area); sidebar rows are 30pt tall.
- Verify in a running build with VoiceOver, Full Keyboard Access, Dark, and Increase Contrast before treating any of this as done.

## 12. Build order suggestion

1. `Theme.swift` and replace v1's opacity fills and system accent button (small, low-risk, visible).
2. Section card with gutter check, then save bar with ring and the custom button style.
3. Sidebar rows with statuses and month grouping (needs skipped storage decision).
4. Banners, settings window, menu bar item.
5. Yesterday card, heatmap, search, weekly review, onboarding, app icon.
6. Visual verification pass, then update this document with measured results.
