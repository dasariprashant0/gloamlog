#!/bin/bash
# Regenerates internal/core/testdata/chunkb_swift.json: compiles app/Core/** with swiftdiff/main.swift and runs it over chunkb_cases.json.
# Needs the Xcode command line tools (like tests/run-tests.sh). The "xcrun cache" warning inside a sandbox is harmless.
set -e
cd "$(dirname "$0")/../../../.."
T="${TMPDIR:-/tmp}"
export CLANG_MODULE_CACHE_PATH="$T/dailylog-clang-cache"
mkdir -p "$CLANG_MODULE_CACHE_PATH"
swiftc -swift-version 5 -module-cache-path "$CLANG_MODULE_CACHE_PATH" -target "$(uname -m)-apple-macos13.0" \
  $(find app/Core -name '*.swift' | sort) internal/core/testdata/swiftdiff/main.swift -o "$T/chunkb-swiftdiff"
"$T/chunkb-swiftdiff" internal/core/testdata/chunkb_cases.json internal/core/testdata/chunkb_swift.json
