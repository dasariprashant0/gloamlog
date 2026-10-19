#!/bin/bash
# Publishes a release the in-app "Check for Updates…" can install: bash scripts/release.sh 0.7.0 "what changed"
# 1. bump VERSION in desktop/build.sh and commit  2. run this  3. it builds, checksums, uploads and updates the Homebrew tap.
# Builds the Go + Wails app (desktop/); the Swift app (app/) is no longer released.
set -e
cd "$(dirname "$0")/.."
V="${1:?version, e.g. 0.7.0}"; NOTES="${2:-Gloamlog $V}"
grep -q "^VERSION=\"$V\"" desktop/build.sh || { echo "desktop/build.sh is not at version $V"; exit 1; }
bash desktop/build.sh
(cd desktop && shasum -a 256 Gloamlog.zip > Gloamlog.zip.sha256)
gh release create "v$V" desktop/Gloamlog.zip desktop/Gloamlog.zip.sha256 --title "Gloamlog $V" --notes "$NOTES"
bash scripts/update-tap.sh "$V"
