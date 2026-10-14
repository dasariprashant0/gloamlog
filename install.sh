#!/bin/bash
# Gloamlog installer.   curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash
#
#   bash install.sh                  install or update to the latest release
#   bash install.sh --version 0.6.5  install that release
#   bash install.sh --uninstall      remove the app (your notes and backups are never touched)
#
# What it does: asks GitHub for the release, downloads Gloamlog.zip and Gloamlog.zip.sha256, checks the SHA-256 (it stops if it differs),
# unpacks, checks the bundle id and version, quits a running Gloamlog (it never force-kills it), swaps the app in (the old one is put
# back if the copy fails), and removes the quarantine flag from that one app, so macOS does not show its "can't be opened" warning.
# The app is not notarised (free project, no paid Apple account); the checksum is the integrity check. Read this file before you run it.
set -euo pipefail

REPO="dasariprashant0/gloamlog"
APP="Gloamlog.app"
BUNDLE_ID="io.github.dasariprashant0.gloamlog"
API="${GLOAMLOG_API_URL:-}"                      # test hook: a URL (or file://) that returns the release JSON

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

MODE=install; WANT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall) MODE=uninstall ;;
    --version) shift; WANT="${1:-}"; [ -n "$WANT" ] || die "--version needs a number, like 0.6.5" ;;
    -h|--help) sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
  shift
done

[ "$(uname -s)" = "Darwin" ] || die "Gloamlog is a macOS app."
MAJOR="$(sw_vers -productVersion | cut -d. -f1)"
[ "$MAJOR" -ge 13 ] 2>/dev/null || die "Gloamlog needs macOS 13 or later (this Mac has $(sw_vers -productVersion))."

if [ -n "${GLOAMLOG_DEST:-}" ]; then DEST_DIR="$GLOAMLOG_DEST"            # test hook
elif [ -w /Applications ]; then DEST_DIR="/Applications"
else DEST_DIR="$HOME/Applications"; fi

quit_app() {
  [ -n "${GLOAMLOG_DEST:-}" ] && return 0                                   # a test install never touches a running app
  pgrep -x Gloamlog >/dev/null 2>&1 || return 0
  say "Gloamlog is running; asking it to quit (it saves your page first)..."
  osascript -e 'tell application id "'"$BUNDLE_ID"'" to quit' >/dev/null 2>&1 || true
  for _ in $(seq 1 50); do pgrep -x Gloamlog >/dev/null 2>&1 || return 0; sleep 0.2; done
  die "Gloamlog is still running. Quit it (Cmd-Q) and run this again."
}

if [ "$MODE" = uninstall ]; then
  quit_app
  found=0
  if [ -n "${GLOAMLOG_DEST:-}" ]; then DIRS=("$DEST_DIR"); else DIRS=("$DEST_DIR" "$HOME/Applications" "/Applications"); fi   # a test run only touches its own folder
  for d in "${DIRS[@]}"; do
    if [ -d "$d/$APP" ]; then rm -rf "$d/$APP" && say "Removed $d/$APP"; found=1; fi
  done
  [ "$found" = 1 ] || say "Gloamlog is not installed."
  say "Your notes and backups were not touched. If you connected Claude Code, also run: claude mcp remove gloamlog -s user"
  exit 0
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/gloamlog-install.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

if [ -z "$API" ]; then
  if [ -n "$WANT" ]; then API="https://api.github.com/repos/$REPO/releases/tags/v${WANT#v}"
  else API="https://api.github.com/repos/$REPO/releases/latest"; fi
fi
say "Looking up the release..."
curl -fsSL -H "Accept: application/vnd.github+json" "$API" -o "$TMP/release.json" || die "could not reach GitHub, or there is no such release."

TAG="$(grep -m1 '"tag_name"' "$TMP/release.json" | sed -E 's/.*"tag_name"[^"]*"([^"]+)".*/\1/')"
VERSION="${TAG#v}"
ZIP_URL="$(grep -o '"browser_download_url"[^,}]*Gloamlog\.zip"' "$TMP/release.json" | head -1 | sed -E 's/.*: *"([^"]+)"/\1/')"
SHA_URL="$(grep -o '"browser_download_url"[^,}]*Gloamlog\.zip\.sha256"' "$TMP/release.json" | head -1 | sed -E 's/.*: *"([^"]+)"/\1/')"
[ -n "$TAG" ] && [ -n "$ZIP_URL" ] && [ -n "$SHA_URL" ] || die "that release has no Gloamlog.zip and Gloamlog.zip.sha256, so it can't be verified."

say "Downloading Gloamlog $VERSION..."
curl -fsSL "$ZIP_URL" -o "$TMP/Gloamlog.zip" || die "download failed."
curl -fsSL "$SHA_URL" -o "$TMP/Gloamlog.zip.sha256" || die "checksum download failed."

WANT_SUM="$(awk '{print $1; exit}' "$TMP/Gloamlog.zip.sha256" | tr 'A-F' 'a-f')"
GOT_SUM="$(shasum -a 256 "$TMP/Gloamlog.zip" | awk '{print $1}')"
[ -n "$WANT_SUM" ] && [ "$WANT_SUM" = "$GOT_SUM" ] || die "the download does not match its published checksum, so it was NOT installed."
say "Checksum OK."

ditto -x -k "$TMP/Gloamlog.zip" "$TMP/out" || die "could not unpack the download."
NEW="$TMP/out/$APP"
[ -d "$NEW" ] || die "the download does not contain $APP."
PLIST="$NEW/Contents/Info.plist"
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$PLIST" 2>/dev/null)" = "$BUNDLE_ID" ] || die "the download is not Gloamlog (wrong bundle id)."
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$PLIST" 2>/dev/null)" = "$VERSION" ] || die "the app version does not match release $TAG."
codesign --verify --deep --strict "$NEW" 2>/dev/null || die "the app's signature does not verify."

quit_app
mkdir -p "$DEST_DIR"
OLD="$DEST_DIR/$APP.old"
rm -rf "$OLD"
if [ -d "$DEST_DIR/$APP" ]; then mv "$DEST_DIR/$APP" "$OLD" || die "could not move the old app aside."; fi
if ditto "$NEW" "$DEST_DIR/$APP"; then
  rm -rf "$OLD"
else
  rm -rf "$DEST_DIR/$APP"; [ -d "$OLD" ] && mv "$OLD" "$DEST_DIR/$APP"
  die "could not copy the new app; the old one was put back."
fi
xattr -dr com.apple.quarantine "$DEST_DIR/$APP" 2>/dev/null || true       # this one app only, so macOS shows no first-launch warning

say ""
say "Installed Gloamlog $VERSION in $DEST_DIR"
[ "$DEST_DIR" = "$HOME/Applications" ] && say "(/Applications was not writable, so it went in your own Applications folder.)"
say "Open it:  open \"$DEST_DIR/$APP\""
say "Update later from the Gloamlog menu: Check for Updates..."
