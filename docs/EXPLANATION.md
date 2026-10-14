# How Gloamlog works (plain language)

1. **You open the app.** It shows today's page: a title with the date and a blank, Notion-style page. A new day starts with your template (by default the headings *What I did / Finished / Started / Pending / To do next*). Nothing is written to disk until you type.
2. **You write.** Use headings, bullets, to-do lists, quotes, code, tables and images. Type `/` for the block menu. There is no Save button: the page saves itself a moment after you stop typing, and the app keeps safety copies you can restore from the page's `...` menu.
3. **A day counts as logged** once you have written enough words (20 by default; change it in Settings > Page). Template headings do not count, and neither do carried-over tasks.
4. **Any date works.** The sidebar has a month calendar: click any day to open it. Press ⇧⌘T (Go to date) and type `last fri`, `2 oct` or `-3`. Nothing is written until you actually type something.
5. **Catch up.** The Catch up row lists workdays you have not written, oldest first, with a count. *Start catching up* walks you through them one after another, and you can skip several at once (holiday, leave) with a way to undo.
6. **Review.** The Review screen shows a week (or month, year, or a range) with the days you logged, and "Copy as markdown" gives a clean summary to paste into a standup or an update.
7. **Reminders and streaks.** At your reminder time (default 4:55 pm on weekdays) Gloamlog nudges you, strictly or gently as you choose. Skipped days do not break the streak, and writing a missed day later repairs it.
8. **Settings** live behind the gear at the bottom of the sidebar (also ⌘, and the menu-bar item): appearance, week start, reminders, the page template and the words rule, where your logs live, and a list of shortcuts.

## Where your data lives
- Logs: `~/Gloamlog/YYYY-MM-DD.md`, one plain markdown file per day; images in `~/Gloamlog/assets/`. You can move the folder (for example into iCloud Drive) in Settings > Storage.
- Safety copies: `~/Library/Application Support/Gloamlog/backups/`, outside the log folder so they never sync with it.
- Settings: stored by macOS for the app. Nothing is sent anywhere.

## Good to know
- Everything is local: no account, no sync service. The only network request is Check for Updates, when you click it.
- To stop the app: ⌘Q. Remove it from System Settings > General > Login Items if you enabled "Open at login".
- The app is not notarised yet, so the first launch needs System Settings > Privacy & Security > Open Anyway (or build it from source, which needs no approval).
