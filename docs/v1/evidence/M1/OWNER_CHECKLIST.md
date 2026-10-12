# Your hands-on check (about 45 minutes)

Run this on your own Mac with the built app. Pass or fail, no "mostly". Copy your log folder first if it holds anything you care about:

```bash
cp -R ~/Gloamlog ~/Gloamlog-backup 2>/dev/null; open ~/Projects/personal/daily-log/app/Gloamlog.app
```

If macOS refuses to open it, go to System Settings > Privacy & Security and choose *Open Anyway*. (If a previous copy is running, quit it first.)

## Milestone 1: any date, catch up, Settings (R1 to R9)

| # | Do this | Pass if |
|---|---|---|
| R1 | Click a workday from last month that has no page in the sidebar calendar. Write 2 or 3 sentences. | The page opens with your template, nothing is written until you type, and about 2 seconds after typing that date's `YYYY-MM-DD.md` appears in `~/Gloamlog`. |
| R2 | Click **Review**, go to last week. Open each of its workdays from that screen and write about 20 words in each. | Every day opens, even with no page, and the week reads 5 of 5 logged. |
| R3 | With 3 unwritten workdays, click **Catch up** > **Start catching up** and write about 20 words in each. Time it. | 2 minutes or less to the third day; the list ends on "All caught up". |
| R4 | In Catch up, select 3 days and skip them with the reason "Leave". | 3 skip markers, streak unchanged, the days leave the list, Undo brings them back. |
| R5 | Click the **gear** at the bottom of the sidebar (not ⌘, and not the menu). Change reminder time, reminder style, appearance and week start; open Storage and click *Show in Finder*; quit and relaunch. | Settings opens in front in under a second; every change survives; Finder shows plain `YYYY-MM-DD.md` files; the same window opens from the menu-bar item and ⌘,. |
| R6 | Type in today's page, wait 2 seconds, force-quit (Activity Monitor). Type again and press ⌘Q at once. Relaunch. | All your text is there both times. |
| R7 | Set the reminder 2 minutes ahead and close the window. Follow the reminder and write the day up. | The reminder appears and opens today's page; once you pass the minimum words the menu-bar item shows logged and the nagging stops. |
| R8 | Search a word you wrote last week. | The hit and snippet appear in under a second; clicking opens the day at that text. |
| R9 | Open last week, **Copy as markdown**, paste into Notes. | It reads well: only your writing, no app chrome. |
| Keys | Put the cursor inside a bullet list and press ⌘[ and ⌘]. | The app moves to the previous / next day (it should not change list indentation). |

## Later milestones (added as they land)
- **Quick capture:** press ⌃⌥J in another app (a browser, a full-screen app) and type a note; it appears under `## Jots` with the time; typing in today's page while you jot loses nothing; rename the log folder, jot, rename it back and the note arrives; change the shortcut in Settings and the old one stops.
- **Reviews and export:** Review > Month and Year; copy as Markdown, Plain text and Rich text; Save as PDF; Save as Markdown with images.
- **Reminders when closed:** quit Gloamlog and wait for the reminder time; the notification arrives and opens today's page.

Tell me which rows fail and what you saw. Anything that looks off, even if it works, is worth saying.
