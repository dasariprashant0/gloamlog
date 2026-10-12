> **Superseded.** This document describes the earlier five-box version (v0.1 and v0.2). The current plans are in [docs/v1/](v1/) (PRD_V1, UX_FLOWS, DESIGN_V4, TRD, ARCHITECTURE).

# Roadmap: v0.3 and beyond

This is a ranked list of bets, not a promise. Rank reflects value to people who already use Gloamlog divided by effort and risk, and every item keeps the core promise: local-only, plain markdown, no telemetry unless stated. Items move up when real usage or feedback supports them. Nothing here ships before v0.2 has been used for a few weeks.

## Principles any item must pass

1. Works with the log folder as the single source of truth (markdown files).
2. No network by default. Anything that needs one is opt-in, explicit, and off out of the box.
3. Builds with Command Line Tools only.
4. Does not make the daily write-up slower.

## Ranked ideas

| Rank | Idea | Why this rank | Effort | Signal needed to start |
|------|------|---------------|--------|------------------------|
| 1 | **Homebrew cask** | Biggest cut to install friction for the lowest effort: a cask file pointing at the GitHub release zip and checksum. Also makes upgrading one command. Needs stable tagged releases (v0.2). | S | A stable release URL and checksum scheme; at least a few users who ask for it |
| 2 | **Notarisation + Sparkle-style updates** | Removes the Gatekeeper warning, the main adoption blocker, and gives users a safe upgrade path. Cost: paid Apple Developer account and an update feed. The update check would be the only network call, so it must be opt-in and documented, or replaced by "check releases page" and Homebrew. | M | More than a handful of external installs, or repeated Gatekeeper complaints; decision on the developer account |
| 3 | **Obsidian / Notion export** | Logs are already markdown, so Obsidian needs little more than a daily-note-compatible filename/frontmatter option and a "storage folder = vault subfolder" guide. Notion is harder (API, token, network) and goes second, as a manual "copy as Notion-friendly markdown" before any API. | S (Obsidian), M (Notion) | Users saying they paste logs into these tools by hand |
| 4 | **Tags / mentions** (`#project`, `@person`) | Makes search and weekly review sharper with little new UI: parse tokens from existing text, filter by tag. Plain text stays plain text. | M | Search used often; people asking to filter by project |
| 5 | **Templates gallery** | Builds on customisable sections (v0.2). A handful of bundled section sets (engineering stand-up, marketing, student, support, writer) and import/export of a section config file. Helps new users get value on day one. | S | Section customisation is popular; users share their configs |
| 6 | **Opt-in AI weekly summary** (on-device or user-keyed) | Highest-leverage use of stored logs, but the riskiest for trust: logs can hold sensitive work content. Design: off by default; on-device model first (Apple's local models where available) with no network; a bring-your-own-key option clearly labelled that sends text to the provider the user chose, with a preview of exactly what leaves the Mac. Key stored in Keychain. Never default, never bundled key. | L | Weekly review is used regularly; a workable on-device path exists on macOS 13+ or the project decides to raise the OS floor for this feature only |
| 7 | **iOS / iPad companion via shared folder** | Real demand likely (write at the end of the day from a phone), and no server needed if the folder syncs via iCloud Drive. But it means a second app, App Store or TestFlight, signing, and a developer account. Huge effort for a spare-time project. | XL | Notarisation done, developer account in place, and clear demand; consider a Shortcuts action or plain markdown editor workflow first |
| 8 | **Localisation** | Widens the audience, but needs string extraction, a translation workflow, and contributors. Do after the UI stabilises; strings should be kept in one place from v0.2 to make this cheap. | M | Contributors volunteering to translate; issues from non-English users |

## Smaller candidates (unranked, pick up opportunistically)

- Import of v0.1 settings and a "verify my folder" check for conflicted copies.
- Per-section word counts and a calm "you wrote enough" nudge, not a gamified score.
- Monthly review view, built on the weekly review.
- Optional end-of-week reminder to read the weekly review.
- Shortcuts / AppleScript action: "add a line to today's log".
- Export week or month as a single file (markdown, plain text, PDF).

## Will not do

These conflict with the project's purpose. A PR or issue proposing them will be closed with a link to this list.

| Item | Reason |
|------|--------|
| **Accounts / sign-in** | There is nothing to sign in to. Your logs are files on your Mac. |
| **Cloud sync service run by the project** | Running servers means holding your data and being responsible for it. Use iCloud Drive, Dropbox, Syncthing, or git on the storage folder instead. |
| **Telemetry, analytics, crash reporting** | A private journal must not report on its user. Feedback comes from issues and conversation. |
| **Team or manager dashboards, shared logs** | Turns a private habit into surveillance and changes how people write. |
| **Social features, public profiles, leaderboards** | Streaks are for the person, not an audience. |
| **Ads, paid tiers that gate the core, or bundled third-party SDKs** | Keeps the code small, auditable, and MIT. |
| **Proprietary log format or database** | Markdown files are the product's long-term promise to users. |

## How items get promoted

An idea moves into a versioned plan when it has an owner, a success metric, and a time horizon, per the PRD format. Ideas without those stay here. Open an issue using the feature-request template to propose or vote on one.
