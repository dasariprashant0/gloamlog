#!/bin/bash
# Builds app/Resources/AppIcon.icns (+ AppIcon-1024.png) from make-icon.swift.
set -euo pipefail
cd "$(dirname "$0")/.."
export TMPDIR="${TMPDIR:-/tmp/claude}"; export CLANG_MODULE_CACHE_PATH="$TMPDIR/mc"
mkdir -p "$TMPDIR/mc" Resources
W="$(mktemp -d "$TMPDIR/icon.XXXXXX")"; trap 'rm -rf "$W"' EXIT
swiftc -O Tools/make-icon.swift -o "$W/make-icon"
"$W/make-icon" Resources/AppIcon-1024.png
IS="$W/AppIcon.iconset"; mkdir "$IS"
mk() { # pixels filename; <=64px uses the simplified art
  if [ "$1" -le 64 ]; then "$W/make-icon" "$IS/$2" simple "$1"; else "$W/make-icon" "$IS/$2" full "$1"; fi
}
for s in 16 32 128 256 512; do
  mk "$s" "icon_${s}x${s}.png"; mk $((s*2)) "icon_${s}x${s}@2x.png"
done
mkdir -p Resources/AppIcon.iconset && cp "$IS"/*.png Resources/AppIcon.iconset/
iconutil -c icns "$IS" -o Resources/AppIcon.icns
echo "ok: Resources/AppIcon.icns"
