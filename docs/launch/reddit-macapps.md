<!-- DRAFT for the owner to review. Do not post as is.
Before posting:
- r/macapps rules change; read the live sidebar and pinned posts, and message the moderators first if a developer post needs approval.
- Use the flair and post template the sub asks for. Disclose that you made it (done below).
- Attach the three screenshots (docs/screenshots/). No upvote requests. Reply to every comment.
- If the install one-liner or the Releases download is not live yet, do not post. -->

**Title:** I made a free, open-source work journal for Mac that can write itself from Claude Code and Git (Gloamlog, alpha)

**Body:**

Disclosure: I'm the developer. Gloamlog is free, MIT-licensed and has no account, ads or tracking.

I kept skipping the end-of-day note, so I built a Mac app that makes it light. Each day is one page, saved as a plain markdown file in a folder you pick (iCloud Drive or Dropbox work, with conflict handling). It has a Notion-style editor, a global hotkey (Ctrl-Option-J) that adds a time-stamped line to today from any app, a Catch Up list for days you missed, a calendar, and week, month and year reviews you can copy as Markdown, plain text or rich text, or save as PDF.

What is different: if you use Claude Code, a "Connect Claude Code" button in Settings > Sources registers a small helper so Claude adds a short "what I solved / what I did / what's open" entry to today's page when it finishes a piece of work. Sources can also add your Git commits and pull requests. All of it is optional and off until you turn it on, and it stays on your Mac. The one network request in the app is "Check for Updates…" when you click it.

Install: `curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash`, or the zip from the Releases page, or build from source (`bash app/build.sh`, Command Line Tools only).

What's rough:

- Alpha (0.6.5). Some newer features have had little real-world use.
- Not notarised (no paid Apple account). The installer and the in-app updater avoid the Gatekeeper prompt; a browser download needs System Settings > Privacy & Security > Open Anyway once.
- The release build is Apple silicon only, macOS 13 or later.
- Search is slow for a decade of logs.

Repo, screenshots and docs: https://github.com/dasariprashant0/gloamlog

Feedback I'd like: does a Mac journal that fills in from your tools beat writing it by hand, and what breaks on your setup?
