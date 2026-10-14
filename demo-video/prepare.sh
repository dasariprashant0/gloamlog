#!/bin/bash
# Makes the generated inputs the demo needs (not committed): the two screenshots, the icon at 512 px and the seven voice clips.
# Run once before rendering. The voice uses macOS text-to-speech (`say`), so this is a Mac-only step.
set -e
cd "$(dirname "$0")"
mkdir -p public/voice
cp ../docs/screenshots/today.png ../docs/screenshots/review.png public/
sips -Z 512 ../app/Resources/AppIcon-1024.png --out public/icon-512.png >/dev/null
i=0
while IFS= read -r line; do
  i=$((i+1))
  say -v Samantha -r 168 -o "public/voice/s$i.wav" --data-format=LEI16@44100 "$line"
done <<'LINES'
Gloamlog is a free, open-source work journal for your Mac.
One page a day, saved as a plain markdown file in a folder you choose. No account, no cloud.
Connect Claude Code, and when a piece of work is done, Claude adds a short entry: what you were solving, what got done, and what is still open.
Turn on Git, and your commits and pull requests show up too.
At the end of the week, review it, and export it as Markdown or PDF.
Everything stays on your Mac. The only network request is the update check you click.
Install with one command, or build it from source. Free, and open source on GitHub.
LINES
echo "ready: public/ has the screenshots, the icon and $i voice clips. If you change a line, update its length in src/Demo.tsx (ffprobe prints it)."
