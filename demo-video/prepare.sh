#!/bin/bash
# Makes the generated inputs the demo needs (not committed): the screenshots, the icon at 512 px and the twelve voice clips.
# Run once before rendering. The voice is Microsoft's neural "Andrew" through edge-tts (free, no account; the narration text is
# sent to that service). Without a network it falls back to macOS `say`, which sounds like a screen reader.
set -e
cd "$(dirname "$0")"
mkdir -p public/voice
cp ../docs/screenshots/today.png ../docs/screenshots/review.png public/
sips -Z 512 ../app/Resources/AppIcon-1024.png --out public/icon-512.png >/dev/null
# The other screenshots come from the app's snapshot harness: bash ../app/Tools/snapshot/run.sh <outdir>, then copy
# settings-sources.png, search.png, catch-up.png, jot-panel.png and settings-reminders.png into public/.
if [ ! -x .venv/bin/edge-tts ]; then python3 -m venv .venv && .venv/bin/pip install -q --disable-pip-version-check edge-tts; fi
i=0
while IFS= read -r line; do
  i=$((i+1))
  if .venv/bin/edge-tts --voice en-US-AndrewMultilingualNeural --rate=-4% --text "$line" --write-media "public/voice/s$i.mp3" 2>/dev/null; then :
  else say -v Samantha -r 168 -o "public/voice/s$i.wav" --data-format=LEI16@44100 "$line"; echo "s$i: edge-tts failed, used say (change the file name in src/Demo.tsx)"; fi
done <<'LINES'
Gloamlog is a free, open-source work journal for your Mac.
Each day is one page, saved as a plain markdown file in a folder you choose. Headings, lists, to-dos and images. No account, no cloud.
In Settings, one click connects Claude Code. It gives Claude a small helper, and tells it when to write.
Then, when a piece of work is done, Claude adds a short entry to today's page: what you were solving, what got done, and what is still open.
Turn on Git, and your commits and pull requests show up too.
Press Control, Option, J in any app to jot a line. It lands on today's page, with the time.
Missed a few days? Catch Up lists them and walks you through, and you can skip the ones that don't count.
Search every page you have ever written. It stays fast, even after years.
At the end of the week, review it, and export it as Markdown or PDF.
A gentle or strict end-of-day reminder keeps the habit, and the folder can live in iCloud Drive or Dropbox, so you can use two Macs.
Everything stays on your Mac. The only network request is the update check you click.
Install with one command or with Homebrew, or build it from source. Free, and open source on GitHub.
LINES
echo "ready: public/ has the icon, the screenshots and $i voice clips. If you change a line, update its length in src/Demo.tsx (ffprobe prints it)."
