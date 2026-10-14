<!-- DRAFT for the owner to review and rewrite in their own words. Do not post as is.
Notes for the owner:
- HN asks for human-written posts and restricts Show HN for new or low-history accounts. Rewrite this so it sounds like you.
- Post only when install.sh works from a clean Mac, the three screenshots exist, and a release is live.
- Do not ask anyone to upvote or comment. Answer every comment, including the hostile ones.
- Replace anything below that is no longer true when you post. -->

# Show HN: Gloamlog – a Mac work journal that Claude Code and Git can write for you

URL: https://github.com/dasariprashant0/gloamlog

## First comment (body)

I wanted an end-of-day work log that I would actually keep, so I built Gloamlog, a native macOS app (13+, MIT). It is one page per day, saved as a plain markdown file in a folder you choose, with a Notion-style editor, a global hotkey (Ctrl-Option-J) to jot a line from any app, catch-up for days you missed, and weekly or monthly reviews you can copy for a standup.

The part I have not seen elsewhere: the log can write itself. Settings > Sources has a "Connect Claude Code" button. It registers a small helper (one Python file, standard library only) with Claude Code and adds a short section to ~/.claude/CLAUDE.md. After that, when a piece of work is done, Claude adds a few sentences to today's page: what it was solving, what it did, what is still open. Turn on Git and your commits and pull requests are added too. You can edit any line. Disconnect undoes the setup.

It is local. No account, no telemetry. The only network request in the app is the "Check for Updates…" menu item, and only when you click it: it asks GitHub for the latest release, downloads it and checks a SHA-256. The helper itself makes no network calls. Be aware that the sentence Claude writes comes from your Claude Code session, which goes to Anthropic the way it already does; Gloamlog adds no other path. PRIVACY.md lists exactly what is read and written.

What is rough, so you do not have to find out:

- It is alpha (0.6.6). Several newer features, such as quick capture over full-screen apps, reminders while the app is quit and iCloud-synced folders, have had little use on real Macs.
- It is not notarised, because I do not have a paid Apple developer account. The install one-liner and the in-app updater avoid the Gatekeeper prompt; a browser-downloaded zip needs System Settings > Privacy & Security > Open Anyway once. docs/INSTALL.md has the steps and a by-hand checksum check.
- The release is a universal binary, but I have only run it on Apple silicon.
- Reading Claude Code's session files (an optional extra) depends on a file format Anthropic does not document, so it may break.
- Search scans every file; fine for years of logs, slow for a decade.

It builds with the Command Line Tools alone (`bash app/build.sh`, no Xcode project, no SwiftPM, no macros) and has no third-party dependencies in the app apart from the bundled editor.

Install: `curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash` (read it first if you like), `brew install --cask --no-quarantine dasariprashant0/gloamlog/gloamlog`, the zip from Releases, or build from source. Site with a 95-second demo: https://dasariprashant0.github.io/gloamlog/

I would like to hear whether the Claude Code entries are useful or noise, and what you would want a work log to capture that this does not.
