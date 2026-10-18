#!/bin/bash
# Builds desktop/Gloamlog.app + Gloamlog.zip from the Go + Wails app. Needs Go, Node and the Xcode Command Line Tools.
# Same bundle id, URL scheme and minimum macOS as the Swift app (app/build.sh), so it replaces it in place.
# Usage: bash desktop/build.sh        GLOAMLOG_NATIVE=1 builds this Mac's architecture only (faster).
set -e
cd "$(dirname "$0")"
VERSION="0.7.0"

(cd frontend && npm ci --no-audit --no-fund && npm run build)

APP="Gloamlog.app"
rm -rf "$APP" Gloamlog.zip
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

# One compile per architecture, then lipo into a universal binary. Wails needs cgo on macOS.
export CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0"
ARCHS="arm64 amd64"; [ "${GLOAMLOG_NATIVE:-}" = 1 ] && ARCHS="$(go env GOARCH)"
PARTS=""
for A in $ARCHS; do
  GOOS=darwin GOARCH=$A go build -tags production -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$APP/Contents/MacOS/Gloamlog-$A" .
  PARTS="$PARTS $APP/Contents/MacOS/Gloamlog-$A"
done
lipo -create $PARTS -output "$APP/Contents/MacOS/Gloamlog"
rm $PARTS

cp ../app/Resources/AppIcon.icns "$APP/Contents/Resources/AppIcon.icns"
# The log_work helper Claude Code runs. Same path inside the bundle as the Swift build, because users who already
# connected Claude Code have this path in their Claude config.
cp -R resources/claude "$APP/Contents/Resources/claude"
[ -f ../THIRD_PARTY_LICENSES.md ] && cp ../THIRD_PARTY_LICENSES.md "$APP/Contents/Resources/THIRD_PARTY_LICENSES.md"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Gloamlog</string>
<key>CFBundleDisplayName</key><string>Gloamlog</string>
<key>CFBundleIdentifier</key><string>io.github.dasariprashant0.gloamlog</string>
<key>CFBundleExecutable</key><string>Gloamlog</string>
<key>CFBundleIconFile</key><string>AppIcon</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$VERSION</string>
<key>CFBundleURLTypes</key><array><dict><key>CFBundleURLName</key><string>Gloamlog</string><key>CFBundleURLSchemes</key><array><string>gloamlog</string></array></dict></array>
<key>CFBundleVersion</key><string>$VERSION</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>LSApplicationCategoryType</key><string>public.app-category.productivity</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSUserNotificationAlertStyle</key><string>none</string>
</dict></plist>
PLIST

codesign --force --sign - "$APP"          # ad-hoc: runs locally, not notarised (same as the Swift build)
ditto -c -k --keepParent "$APP" Gloamlog.zip
echo "Built: $PWD/$APP and $PWD/Gloamlog.zip"
