# Demo video

Renders the 94-second Gloamlog demo (`gloamlog-demo.mp4` on the release page, and the GIF in `docs/demo/`). It is a tooling folder, not part of the app. It needs Node; Remotion downloads its own headless Chrome on first use (a full Chrome build produced tiled frames).

Two editions are registered in `src/Root.tsx`:

- `GloamlogDemo3` (`src/Demo3.tsx`, the published one): the app is rebuilt as animated components in `src/ui.tsx`, so every scene moves: typing, a cursor that clicks, panels that spring in, calendar dots, a sync pulse, a progress bar. The sound effects and the music bed come from `audio/make-audio.py`, a standard-library synth.
- `GloamlogDemo` (`src/Demo.tsx`): the earlier edition, a zoom camera over the real screenshots in `docs/screenshots/`.

```bash
cd demo-video
bash prepare.sh                       # copies the screenshots and icon, and makes the voice clips (edge-tts, the Andrew voice; macOS `say` as the offline fallback)
python3 audio/make-audio.py           # bed.wav and the pop/click/whoosh/chime sounds into public/audio/ (pure Python, about a minute)
npm install
npx remotion render src/index.ts GloamlogDemo3 out/gloamlog-demo-v3.mp4 --codec h264 --crf 19
```

The narration lines live in `prepare.sh` and, with each clip's length in seconds, in `src/Demo3.tsx`; the scene lengths follow from them, so a re-recorded line only needs its new length. `npx remotion studio src/index.ts` opens an interactive preview.

Finishing steps used for the published file (ffmpeg-skill scripts): the music bed mixed in at -10 dB with auto-ducking under the voice and a 3 s fade-out, loudness normalised to -14 LUFS / -1.5 dBTP, an English subtitle track muxed in from `docs/demo/gloamlog-demo.srt` (its cue times are the voice start times, derived from the scene lengths), and a YouTube spec check. The README GIF is a short cut (the title and the Claude entry) from the GIF preset. Remotion is free for individuals and small teams (remotion.dev/license).
