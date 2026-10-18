#!/bin/bash
# Runs Gloamlog with live reload: Vite serves the frontend, the Go app loads it from there.
# Edit anything in desktop/frontend/src and the window updates; edit Go and re-run this.
# Usage: bash desktop/dev.sh
set -e
cd "$(dirname "$0")"
(cd frontend && [ -d node_modules ] || npm install)
(cd frontend && npm run build >/dev/null)   # go:embed needs a dist folder to exist
if lsof -nP -iTCP:9245 -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Port 9245 is in use (an earlier dev server?). Stop it, e.g.: lsof -nP -iTCP:9245 -sTCP:LISTEN" >&2
  exit 1
fi
(cd frontend && npm run dev) &
trap 'kill 0' EXIT   # stop the whole group: npm, Vite and the app, so no dev server is left on the port
sleep 1
export MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0"
FRONTEND_DEVSERVER_URL=http://127.0.0.1:9245 go run .
