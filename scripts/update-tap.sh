#!/bin/bash
# Points the Homebrew tap (github.com/dasariprashant0/homebrew-gloamlog) at a published release:  bash scripts/update-tap.sh 0.6.6
# scripts/release.sh calls it after the GitHub release exists. Needs gh (logged in) and SSH push access to the tap. Run it from the
# main repository so its git identity (and SSH key) are used for the tap commit.
set -e
V="${1:?version, e.g. 0.6.6}"
TAP="git@github.com:dasariprashant0/homebrew-gloamlog.git"
SHA="$(gh release download "v$V" -R dasariprashant0/gloamlog -p Gloamlog.zip.sha256 -O - | awk '{print $1}')"
[ "${#SHA}" = 64 ] || { echo "no checksum found on release v$V" >&2; exit 1; }
NAME="$(git config user.name)"; EMAIL="$(git config user.email)"
D="$(mktemp -d "$(pwd)/.tap-update.XXXXXX")"          # inside the repo folder, so the personal SSH key and identity apply
trap 'rm -rf "$D"' EXIT
git clone -q "$TAP" "$D/tap"
F="$D/tap/Casks/gloamlog.rb"
sed -i '' -e "s/^  version \".*\"/  version \"$V\"/" -e "s/^  sha256 \".*\"/  sha256 \"$SHA\"/" "$F"
grep -q "version \"$V\"" "$F" && grep -q "sha256 \"$SHA\"" "$F"
if git -C "$D/tap" diff --quiet; then echo "tap already at $V"; exit 0; fi
git -C "$D/tap" -c user.name="$NAME" -c user.email="$EMAIL" commit -qam "gloamlog $V"
git -C "$D/tap" push -q origin HEAD
echo "tap updated: gloamlog $V ($SHA)"
