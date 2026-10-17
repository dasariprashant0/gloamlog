#!/bin/bash
# Runs Gloamlog with live reload: Vite serves the frontend, the Go app loads it from there.
# Edit anything in desktop/frontend/src and the window updates; edit Go and re-run this.
# Usage: bash desktop/dev.sh
set -e
cd "$(dirname "$0")"
(cd frontend && [ -d node_modules ] || npm install)
(cd frontend && npm run build >/dev/null)   # go:embed needs a dist folder to exist
(cd frontend && npm run dev) &
VITE=$!
trap 'kill $VITE' EXIT
sleep 1
FRONTEND_DEVSERVER_URL=http://localhost:9245 go run .
