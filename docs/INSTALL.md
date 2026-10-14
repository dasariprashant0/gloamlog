# Installing Gloamlog

Gloamlog needs macOS 13 or later. The release download is a universal binary (Apple silicon and Intel). It has only been run on Apple silicon, so please report anything odd on an Intel Mac.

There are four ways to install it. The installer is the easiest because it avoids the first-launch warning described below.

## 1. The installer

```bash
curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash
```

It downloads the latest GitHub release, verifies the published SHA-256, installs to `/Applications` and removes the quarantine flag from that app only. It does not touch your notes or any other app.

Piping a script into `bash` means trusting it, so you may want to read it first:

```bash
curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh -o install.sh
less install.sh
bash install.sh
```

To update later, use **Gloamlog > Check for Updates…** in the app, or run the installer again.

## 2. Homebrew

```bash
brew install --cask --no-quarantine dasariprashant0/gloamlog/gloamlog
```

The cask lives in the project's own tap, [dasariprashant0/homebrew-gloamlog](https://github.com/dasariprashant0/homebrew-gloamlog), because the main Homebrew cask list only accepts notarised apps. Homebrew checks the download against the SHA-256 in the cask. `--no-quarantine` skips the first-launch warning described below; leave it out and you approve the app once instead. Update with `brew upgrade --cask gloamlog` or from the app's Check for Updates…; remove with `brew uninstall --cask gloamlog` (your notes are never touched).

## 3. Manual download

1. Download `Gloamlog.zip` from the [latest release](https://github.com/dasariprashant0/gloamlog/releases/latest).
2. Unzip it and drag `Gloamlog.app` into `/Applications`.
3. Open it. macOS will refuse the first time (next section). Allow it once and it opens normally from then on.

### First launch of a browser-downloaded copy

Safari, Chrome and other browsers mark downloads with a quarantine flag. Gatekeeper checks quarantined apps for an Apple notarisation ticket, Gloamlog does not have one, so you will see a message that macOS could not verify the app is free of malware. On current macOS the right-click > Open shortcut no longer works for this. Instead:

1. Double-click Gloamlog, and dismiss the message (choose Done, not Move to Trash).
2. Open **System Settings > Privacy & Security** and scroll to the Security section.
3. Click **Open Anyway** next to the message about Gloamlog, enter your password or use Touch ID, and confirm with **Open**.

If you prefer the terminal, removing the flag has the same effect: `xattr -dr com.apple.quarantine /Applications/Gloamlog.app`.

You only do this once. The wording of these dialogs changes between macOS releases; if yours differs, look for "Open Anyway" in Privacy & Security.

### Why the installer and the in-app updater avoid this

The warning is triggered by the quarantine flag, and browsers are what set it. `curl` does not set it, and neither does the app's own downloader, so a copy fetched either way is not quarantined. The installer and the updater also remove the flag from the app they install, explicitly, and only from that app. Both check the download's SHA-256 against the checksum file published in the same release before installing. Gloamlog's own signature is "ad-hoc" (it has no developer identity), and the updater also checks that the new app has the same bundle identifier and the expected version and that its signature verifies.

## 4. Build from source

You need the Xcode Command Line Tools (`xcode-select --install`). You do not need Xcode.

```bash
git clone https://github.com/dasariprashant0/gloamlog.git
cd gloamlog
bash app/build.sh
open app/Gloamlog.app
```

This writes `app/Gloamlog.app` and `app/Gloamlog.zip`, ad-hoc signed. A copy you built on your own Mac is not quarantined, so it opens without the approval step. The page editor is a prebuilt bundle committed in the repo, so you do not need Node unless you change the editor itself (`cd editor-web && npm ci --ignore-scripts && npm run build`).

## Why isn't it notarised?

Notarisation requires a paid Apple Developer Program membership (US$99 a year). Gloamlog is a free, MIT-licensed side project without one, so releases are signed ad-hoc and not notarised. That is a deliberate choice, not an oversight, and the cost is the first-launch approval above. It is also why Gloamlog is not in Homebrew's official cask list, which requires apps to pass Gatekeeper. If you do not want to approve an unnotarised app, the source is short enough to read and `bash app/build.sh` gives you your own build. If this changes, the release notes will say so.

## Checking the download by hand

Each release publishes `Gloamlog.zip` and `Gloamlog.zip.sha256`. Put both in the same folder and run:

```bash
cd ~/Downloads
shasum -a 256 -c Gloamlog.zip.sha256
```

You should see `Gloamlog.zip: OK`. Or compare by eye:

```bash
shasum -a 256 Gloamlog.zip
cat Gloamlog.zip.sha256
```

The two hashes must be identical. The checksum is published next to the zip on the same release page, so it catches a corrupted or swapped download but would not help if the release itself were compromised. Building from source is the way to be sure of what you run.

## Uninstall

```bash
bash install.sh --uninstall
```

If you ran the installer through a pipe and no longer have the file: `curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash -s -- --uninstall`. This removes the app and never your notes.

If you connected Claude Code, disconnect first so nothing points at a missing file: Settings > Sources > Disconnect, or `claude mcp remove gloamlog -s user` and delete the block between `<!-- gloamlog:begin -->` and `<!-- gloamlog:end -->` in `~/.claude/CLAUDE.md`. If you turned on "Open at login", turn it off in the app or in System Settings > General > Login Items.

Doing it by hand: quit Gloamlog, then drag `/Applications/Gloamlog.app` to the Trash.

## Where your files live

| What | Where |
|---|---|
| Your notes, one file per day | `~/Gloamlog/YYYY-MM-DD.md` by default (change it in Settings > Storage) |
| Images you paste or drop | `assets/` inside your notes folder |
| Versioned safety copies of pages | `~/Library/Application Support/Gloamlog/backups/` |
| Quick-capture journal (notes not yet on a page) | `~/Library/Application Support/Gloamlog/capture/` |
| Claude Code inbox (entries waiting to be added) | `~/Library/Application Support/Gloamlog/inbox/` |
| App settings | stored by macOS for the app (bundle identifier `io.github.dasariprashant0.gloamlog`) |

Uninstalling does not delete any of these. To remove everything the app ever wrote besides your notes, delete `~/Library/Application Support/Gloamlog/` and run `defaults delete io.github.dasariprashant0.gloamlog`. Your notes folder is yours to keep or delete.
