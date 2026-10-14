# Demo video

Renders the 95-second Gloamlog demo (`gloamlog-demo.mp4` on the release page, and the GIF in `docs/demo/`) from the real screenshots in `docs/screenshots/` and a neural voiceover (Microsoft's Andrew voice through edge-tts; macOS `say` as the offline fallback). It is a tooling folder, not part of the app. It needs Node and a Chrome install (Remotion renders with a browser).

```bash
cd demo-video
bash prepare.sh                       # copies the screenshots and icon, and makes the voice clips (edge-tts; Mac only for the fallback)
npm install
npx remotion render src/index.ts GloamlogDemo out/gloamlog-demo.mp4 --codec h264 --crf 19
```

Remotion downloads its own headless Chrome on first use (a full Chrome build produced tiled frames). The narration lines live in `prepare.sh` and, with each clip's length in seconds, in `src/Demo.tsx`; the scene lengths follow from them. `npx remotion studio src/index.ts` opens an interactive preview.

Finishing steps used for the published file: loudness normalised to -14 LUFS and an English subtitle track muxed in from `docs/demo/gloamlog-demo.srt`; the README GIF is a short cut (the title and the Claude entries) at 480 px, 12 fps. Remotion is free for individuals and small teams (remotion.dev/license).
