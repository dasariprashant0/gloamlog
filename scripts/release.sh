#!/bin/bash
# Publishes a release the in-app "Check for Updates…" can install: bash scripts/release.sh 0.6.1 "what changed"
# 1. bump the version in app/build.sh (both strings) and commit  2. run this  3. it builds, checksums and uploads.
set -e
cd "$(dirname "$0")/.."
V="${1:?version, e.g. 0.6.1}"; NOTES="${2:-Gloamlog $V}"
grep -q "<string>$V</string>" app/build.sh || { echo "app/build.sh is not at version $V"; exit 1; }
bash app/build.sh
(cd app && shasum -a 256 Gloamlog.zip > Gloamlog.zip.sha256)
gh release create "v$V" app/Gloamlog.zip app/Gloamlog.zip.sha256 --title "Gloamlog $V" --notes "$NOTES"
