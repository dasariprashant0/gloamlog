#!/bin/bash
# Builds "Gloamlog.app" + Gloamlog.zip (shareable). Needs Xcode Command Line Tools only (no Xcode, no SwiftPM, no Node:
# the page editor ships prebuilt in Resources/editor/, see ../editor-web/).
# A harmless "xcrun cache file" sandbox warning can be ignored. Usage: ./build.sh
set -e
cd "$(dirname "$0")"
export TMPDIR="${TMPDIR:-/tmp}"
export CLANG_MODULE_CACHE_PATH="${CLANG_MODULE_CACHE_PATH:-$TMPDIR/dailylog-clang-cache}"
mkdir -p "$CLANG_MODULE_CACHE_PATH"

if [ ! -f Resources/editor/index.html ]; then
  echo "error: app/Resources/editor/index.html is missing." >&2
  echo "       The page editor is a prebuilt web bundle. Restore it from git, or rebuild it with:" >&2
  echo "       (cd editor-web && npm ci --ignore-scripts && npm run build)" >&2
  exit 1
fi

APP="Gloamlog.app"
rm -rf "$APP" Gloamlog.zip
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

swiftc -O -parse-as-library -swift-version 5 -module-cache-path "$CLANG_MODULE_CACHE_PATH" \
  -target "$(uname -m)-apple-macos13.0" $(find Core UI -name '*.swift' | sort) -o "$APP/Contents/MacOS/Gloamlog"   # recursive: subfolders like Core/Calendar are included (no spaces in source paths)

cp Resources/AppIcon.icns "$APP/Contents/Resources/AppIcon.icns"
cp -R Resources/editor "$APP/Contents/Resources/editor"
[ -f ../THIRD_PARTY_LICENSES.md ] && cp ../THIRD_PARTY_LICENSES.md "$APP/Contents/Resources/THIRD_PARTY_LICENSES.md"

cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Gloamlog</string>
<key>CFBundleDisplayName</key><string>Gloamlog</string>
<key>CFBundleIdentifier</key><string>io.github.dasariprashant0.gloamlog</string>
<key>CFBundleExecutable</key><string>Gloamlog</string>
<key>CFBundleIconFile</key><string>AppIcon</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.6.1</string>
<key>CFBundleURLTypes</key><array><dict><key>CFBundleURLName</key><string>Gloamlog</string><key>CFBundleURLSchemes</key><array><string>gloamlog</string></array></dict></array>
<key>CFBundleVersion</key><string>0.6.1</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>LSApplicationCategoryType</key><string>public.app-category.productivity</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSUserNotificationAlertStyle</key><string>none</string>
</dict></plist>
PLIST

codesign --force --sign - "$APP"          # ad-hoc: runs locally, not notarised
ditto -c -k --keepParent "$APP" Gloamlog.zip
echo "Built: $PWD/$APP and $PWD/Gloamlog.zip"
