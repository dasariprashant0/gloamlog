# Demo video

Renders the 45-second Gloamlog demo (`gloamlog-demo.mp4` on the release page, and the GIF in `docs/demo/`) from the real screenshots in `docs/screenshots/` and a voiceover made with macOS text-to-speech. It is a tooling folder, not part of the app. It needs Node and a Chrome install (Remotion renders with a browser).

```bash
cd demo-video
bash prepare.sh                       # copies the screenshots and icon, and makes the voice clips with `say` (Mac only)
npm install
npx remotion render src/index.ts GloamlogDemo out/gloamlog-demo.mp4 --codec h264 --crf 19 \
  --browser-executable "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
```

Leave out `--browser-executable` to let Remotion download its own headless Chrome. The narration lines live in `prepare.sh` and, with each clip's length in seconds, in `src/Demo.tsx`; the scene lengths follow from them. `npx remotion studio src/index.ts` opens an interactive preview.

Finishing steps used for the published file: loudness normalised to -14 LUFS and an English subtitle track muxed in from `docs/demo/gloamlog-demo.srt`; the README GIF is the first 21.5 seconds at 480 px, 12 fps. Remotion is free for individuals and small teams (remotion.dev/license).
