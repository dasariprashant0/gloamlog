// Demo.tsx - the Gloamlog demo: real app screenshots, a neural text-to-speech voiceover (edge-tts, "Andrew") and captions for
// sound-off viewing. Twelve scenes, each as long as its narration plus a short tail, cross-faded with TransitionSeries.
import React from "react";
import { AbsoluteFill, Audio, Easing, Img, Sequence, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { TransitionSeries, linearTiming } from "@remotion/transitions";
import { fade } from "@remotion/transitions/fade";

export const FPS = 30;
const T = 15;     // frames of cross-fade between scenes
const LEAD = 12;  // frames before a scene's voice starts

const C = { deep: "#0B2B26", green: "#1B4A3E", teal: "#2E8A74", accent: "#0E7A5F", mint: "#45C29C", peach: "#F6C48E", white: "#FFFFFF" };
const FONT = '-apple-system, "SF Pro Display", "SF Pro Text", "Helvetica Neue", Helvetica, Arial, sans-serif';
const MONO = '"SF Mono", Menlo, Monaco, Consolas, monospace';
const clamp = { extrapolateLeft: "clamp" as const, extrapolateRight: "clamp" as const };
const ease = Easing.inOut(Easing.cubic);

// The narration. prepare.sh makes the clips in public/voice from exactly these lines; `seconds` is each clip's length (ffprobe).
type Line = { file: string; seconds: number; text: string; caption?: string };
const L: Record<string, Line> = {
  s1: { file: "voice/s1.mp3", seconds: 3.89, text: "Gloamlog is a free, open-source work journal for your Mac." },
  s2: { file: "voice/s2.mp3", seconds: 11.02, text: "Each day is one page, saved as a plain markdown file in a folder you choose. Headings, lists, to-dos and images. No account, no cloud." },
  s3: { file: "voice/s3.mp3", seconds: 7.06, text: "In Settings, one click connects Claude Code. It gives Claude a small helper, and tells it when to write." },
  s4: { file: "voice/s4.mp3", seconds: 9.22, text: "Then, when a piece of work is done, Claude adds a short entry to today's page: what you were solving, what got done, and what is still open." },
  s5: { file: "voice/s5.mp3", seconds: 4.42, text: "Turn on Git, and your commits and pull requests show up too." },
  s6: { file: "voice/s6.mp3", seconds: 7.37, text: "Press Control, Option, J in any app to jot a line. It lands on today's page, with the time.",
        caption: "Press Control-Option-J (⌃⌥J) in any app to jot a line. It lands on today's page, with the time." },
  s7: { file: "voice/s7.mp3", seconds: 5.95, text: "Missed a few days? Catch Up lists them and walks you through, and you can skip the ones that don't count." },
  s8: { file: "voice/s8.mp3", seconds: 5.66, text: "Search every page you have ever written. It stays fast, even after years." },
  s9: { file: "voice/s9.mp3", seconds: 5.54, text: "At the end of the week, review it, and export it as Markdown or PDF." },
  s10: { file: "voice/s10.mp3", seconds: 8.90, text: "A gentle or strict end-of-day reminder keeps the habit, and the folder can live in iCloud Drive or Dropbox, so you can use two Macs." },
  s11: { file: "voice/s11.mp3", seconds: 5.81, text: "Everything stays on your Mac. The only network request is the update check you click." },
  s12: { file: "voice/s12.mp3", seconds: 7.15, text: "Install with one command or with Homebrew, or build it from source. Free, and open source on GitHub." },
};
const len = (line: Line, tail: number) => Math.round((line.seconds + tail) * FPS) + LEAD;

const TITLE = len(L.s1, 0.9), PAGE = len(L.s2, 0.8), CONNECT = len(L.s3, 0.9), CLAUDE = len(L.s4, 0.9), GIT = len(L.s5, 1.0);
const JOT = len(L.s6, 1.0), CATCH = len(L.s7, 0.9), SEARCH = len(L.s8, 0.9), REVIEW = len(L.s9, 1.0), REMIND = len(L.s10, 0.9);
const PRIVACY = len(L.s11, 1.0), INSTALL = len(L.s12, 2.2);
const SCENES = [TITLE, PAGE, CONNECT, CLAUDE + GIT, JOT, CATCH, SEARCH, REVIEW, REMIND, PRIVACY, INSTALL];
export const TOTAL_FRAMES = SCENES.reduce((a, b) => a + b, 0) - T * (SCENES.length - 1);

// MARK: shared pieces
const Voice: React.FC<{ line: Line; at: number }> = ({ line, at }) => (
  <Sequence from={at} layout="none"><Audio src={staticFile(line.file)} /></Sequence>
);

/// The narration as a caption pill: words arrive one by one over the first 85% of the clip; `until` fades it out before the next one.
const Caption: React.FC<{ line: Line; at: number; until?: number }> = ({ line, at, until }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  if (frame < at - 6 || (until !== undefined && frame > until)) return null;
  const words = (line.caption ?? line.text).split(" ");
  const per = (line.seconds * fps * 0.85) / words.length;
  const boxIn = interpolate(frame, [at - 6, at + 4], [0, 1], clamp);
  const boxOut = until === undefined ? 1 : interpolate(frame, [until - 8, until], [1, 0], clamp);
  return (
    <div style={{ position: "absolute", left: 0, right: 0, bottom: 72, display: "flex", justifyContent: "center", opacity: boxIn * boxOut }}>
      <div style={{ background: "rgba(11,43,38,0.86)", color: C.white, padding: "20px 34px", borderRadius: 24, fontFamily: FONT, fontSize: 40,
                    lineHeight: 1.3, maxWidth: 1440, textAlign: "center", boxShadow: "0 12px 40px rgba(0,0,0,0.25)" }}>
        {words.map((w, i) => {
          const s = spring({ frame: Math.max(0, frame - at - i * per), fps, config: { damping: 200, stiffness: 260 } });
          return <span key={i} style={{ display: "inline-block", opacity: s, transform: `translateY(${(1 - s) * 12}px)`, marginRight: 11 }}>{w}</span>;
        })}
      </div>
    </div>
  );
};

/// A screenshot (iw x ih pixels) in a window frame `width` wide at `top`, with a camera: `k` is the zoom and (fx, fy) the image
/// pixel that sits at the frame's focus point (its centre, or the screen centre when the frame is taller than the screen).
/// Keep (fx, fy) at the frame's own centre while k is 1, or the image slides off the frame.
type Cam = { k: number; fx: number; fy: number };
const Window: React.FC<{ src: string; iw: number; ih: number; width: number; top: number; cam: Cam; rise: number; children?: React.ReactNode }> =
  ({ src, iw, ih, width, top, cam, rise, children }) => {
  const f = width / iw, height = ih * f, left = (1920 - width) / 2;
  const P = { x: width / 2, y: Math.min(height / 2, 540 - top) };
  const F = { x: cam.fx * f, y: cam.fy * f };
  return (
    <div style={{ position: "absolute", left, top: top + (1 - rise) * 90, width, height, borderRadius: 18, overflow: "hidden", opacity: rise,
                  boxShadow: "0 30px 80px rgba(0,0,0,0.45)", border: "1px solid rgba(255,255,255,0.14)" }}>
      <div style={{ position: "absolute", left: 0, top: 0, width, height, transformOrigin: "0 0",
                    transform: `translate(${P.x - cam.k * F.x}px, ${P.y - cam.k * F.y}px) scale(${cam.k})` }}>
        <Img src={staticFile(src)} style={{ width, height, display: "block" }} />
        {children}
      </div>
    </div>
  );
};

/// A highlight box in image pixels (f = displayed width / image width).
const Box: React.FC<{ x1: number; y1: number; x2: number; y2: number; f: number; opacity: number }> = ({ x1, y1, x2, y2, f, opacity }) => (
  <div style={{ position: "absolute", left: x1 * f, top: y1 * f, width: (x2 - x1) * f, height: (y2 - y1) * f, borderRadius: 14, opacity,
                border: `4px solid ${C.mint}`, boxShadow: "0 0 0 8px rgba(69,194,156,0.22), 0 0 40px rgba(69,194,156,0.35)",
                transform: `scale(${0.985 + 0.015 * opacity})` }} />
);

const useRise = () => { const frame = useCurrentFrame(); const { fps } = useVideoConfig(); return spring({ frame, fps, config: { damping: 22, stiffness: 90 } }); };
const Backdrop: React.FC<{ children?: React.ReactNode }> = ({ children }) => (
  <AbsoluteFill style={{ background: `linear-gradient(180deg, ${C.green} 0%, ${C.deep} 100%)`, fontFamily: FONT }}>{children}</AbsoluteFill>
);

// MARK: scenes
const Title: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const pop = spring({ frame, fps, config: { damping: 13, stiffness: 110, mass: 0.9 } });
  const rise = spring({ frame: Math.max(0, frame - 12), fps, config: { damping: 200, stiffness: 120 } });
  return (
    <AbsoluteFill style={{ background: `linear-gradient(180deg, ${C.deep} 0%, ${C.green} 42%, ${C.teal} 70%, ${C.peach} 100%)`, fontFamily: FONT }}>
      <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", paddingBottom: 150 }}>
        <Img src={staticFile("icon-512.png")} style={{ width: 320, height: 320, transform: `scale(${pop})`, filter: "drop-shadow(0 28px 44px rgba(0,0,0,0.35))" }} />
        <div style={{ color: C.white, fontSize: 118, fontWeight: 700, letterSpacing: -3, marginTop: 30, opacity: rise, transform: `translateY(${(1 - rise) * 28}px)` }}>Gloamlog</div>
        <div style={{ color: "rgba(255,255,255,0.9)", fontSize: 40, marginTop: 8, opacity: interpolate(frame, [34, 54], [0, 1], clamp) }}>The work journal that can write itself</div>
      </AbsoluteFill>
      <Voice line={L.s1} at={LEAD} /><Caption line={L.s1} at={LEAD} />
    </AbsoluteFill>
  );
};

/// today.png (2000x1520): the whole page, drifting down a little.
const Page: React.FC = () => {
  const frame = useCurrentFrame(); const rise = useRise();
  const k = interpolate(frame, [0, PAGE], [1, 1.04], { ...clamp, easing: Easing.out(Easing.quad) });
  const fy = interpolate(frame, [0, PAGE], [620, 660], { ...clamp, easing: Easing.out(Easing.quad) });
  return (
    <Backdrop>
      <Window src="today.png" iw={2000} ih={1520} width={1560} top={56} cam={{ k, fx: 1000, fy }} rise={rise} />
      <Voice line={L.s2} at={LEAD} /><Caption line={L.s2} at={LEAD} />
    </Backdrop>
  );
};

/// settings-sources.png (1120x1280): zoom to the "Claude writes my log" row and its Connect button.
const Connect: React.FC = () => {
  const frame = useCurrentFrame(); const rise = useRise();
  const k = interpolate(frame, [20, 75], [1, 1.12], { ...clamp, easing: ease });
  const fx = interpolate(frame, [20, 75], [560, 560], clamp);
  const fy = interpolate(frame, [20, 75], [627, 780], { ...clamp, easing: ease });
  const box = interpolate(frame, [70, 85], [0, 1], clamp);
  return (
    <Backdrop>
      <Window src="settings-sources.png" iw={1120} ih={1280} width={1100} top={36} cam={{ k, fx, fy }} rise={rise}>
        <Box x1={50} y1={742} x2={1070} y2={932} f={1100 / 1120} opacity={box} />
      </Window>
      <Voice line={L.s3} at={LEAD} /><Caption line={L.s3} at={LEAD} />
    </Backdrop>
  );
};

/// today.png zoomed on the first Claude-written entry, then panning to the commit line.
const ClaudeGit: React.FC = () => {
  const frame = useCurrentFrame();
  const g = CLAUDE;                                                            // where the Git segment starts
  const fy = interpolate(frame, [g - 6, g + 34], [440, 632], { ...clamp, easing: ease });
  const box1 = interpolate(frame, [14, 28, g - 16, g - 4], [0, 1, 1, 0], clamp);
  const box2 = interpolate(frame, [g + 26, g + 42], [0, 1], clamp);
  return (
    <Backdrop>
      <Window src="today.png" iw={2000} ih={1520} width={1560} top={56} cam={{ k: 1.45, fx: 1237, fy }} rise={1}>
        <Box x1={560} y1={322} x2={1905} y2={482} f={0.78} opacity={box1} />
        <Box x1={560} y1={598} x2={1905} y2={668} f={0.78} opacity={box2} />
      </Window>
      <Voice line={L.s4} at={LEAD} />      <Caption line={L.s4} at={LEAD} until={g - 2} />
      <Voice line={L.s5} at={g + LEAD} />  <Caption line={L.s5} at={g + LEAD} />
    </Backdrop>
  );
};

/// The quick-capture panel: the shortcut's keycaps press, then the panel pops in.
const Jot: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const press = spring({ frame: Math.max(0, frame - 10), fps, config: { damping: 14, stiffness: 170 } });
  const bump = Math.sin(Math.min(1, press) * Math.PI);                         // down and back up
  const pop = spring({ frame: Math.max(0, frame - 30), fps, config: { damping: 15, stiffness: 120 } });
  return (
    <AbsoluteFill style={{ background: `radial-gradient(1300px 760px at 50% 40%, ${C.green} 0%, ${C.deep} 72%)`, fontFamily: FONT }}>
      <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", paddingBottom: 170 }}>
        <div style={{ display: "flex", gap: 16, marginBottom: 48 }}>
          {["⌃", "⌥", "J"].map((key) => (
            <div key={key} style={{ width: 104, height: 104, borderRadius: 20, background: "rgba(255,255,255,0.10)", border: "1px solid rgba(255,255,255,0.28)",
                                    color: C.white, fontSize: 48, fontWeight: 600, display: "flex", alignItems: "center", justifyContent: "center",
                                    transform: `translateY(${bump * 6}px) scale(${1 - 0.06 * bump})`, boxShadow: `0 ${8 - 6 * bump}px 0 rgba(0,0,0,0.4)` }}>{key}</div>
          ))}
        </div>
        <Img src={staticFile("jot-panel.png")} style={{ width: 1100, height: 1100 * 296 / 1040, borderRadius: 22, display: "block",
                                                         boxShadow: "0 30px 80px rgba(0,0,0,0.5)", transform: `scale(${0.9 + 0.1 * pop})`, opacity: pop }} />
      </AbsoluteFill>
      <Voice line={L.s6} at={LEAD} /><Caption line={L.s6} at={LEAD} />
    </AbsoluteFill>
  );
};

/// catch-up.png (2000x1520): zoom towards the list of unlogged days.
const CatchUp: React.FC = () => {
  const frame = useCurrentFrame(); const rise = useRise();
  const k = interpolate(frame, [0, CATCH], [1, 1.18], { ...clamp, easing: Easing.out(Easing.quad) });
  const fx = interpolate(frame, [0, CATCH], [1000, 1120], { ...clamp, easing: Easing.out(Easing.quad) });
  const fy = interpolate(frame, [0, CATCH], [620, 590], { ...clamp, easing: Easing.out(Easing.quad) });
  return (
    <Backdrop>
      <Window src="catch-up.png" iw={2000} ih={1520} width={1560} top={56} cam={{ k, fx, fy }} rise={rise} />
      <Voice line={L.s7} at={LEAD} /><Caption line={L.s7} at={LEAD} />
    </Backdrop>
  );
};

/// search.png (2000x1040): fits the screen, slight zoom.
const SearchScene: React.FC = () => {
  const frame = useCurrentFrame(); const rise = useRise();
  const k = interpolate(frame, [0, SEARCH], [1, 1.1], { ...clamp, easing: Easing.out(Easing.quad) });
  const fy = interpolate(frame, [0, SEARCH], [520, 500], { ...clamp, easing: Easing.out(Easing.quad) });
  return (
    <Backdrop>
      <Window src="search.png" iw={2000} ih={1040} width={1560} top={114} cam={{ k, fx: 1000, fy }} rise={rise} />
      <Voice line={L.s8} at={LEAD} /><Caption line={L.s8} at={LEAD} />
    </Backdrop>
  );
};

const Review: React.FC = () => {
  const frame = useCurrentFrame(); const rise = useRise();
  const k = interpolate(frame, [0, REVIEW], [1, 1.07], { ...clamp, easing: Easing.out(Easing.quad) });
  const fy = interpolate(frame, [0, REVIEW], [620, 640], { ...clamp, easing: Easing.out(Easing.quad) });
  return (
    <Backdrop>
      <Window src="review.png" iw={2000} ih={1520} width={1560} top={56} cam={{ k, fx: 1000, fy }} rise={rise} />
      <Voice line={L.s9} at={LEAD} /><Caption line={L.s9} at={LEAD} />
    </Backdrop>
  );
};

/// settings-reminders.png (1040x1120) as a card, with the storage note beside it.
const Reminders: React.FC = () => {
  const frame = useCurrentFrame(); const rise = useRise();
  const { fps } = useVideoConfig();
  const k = interpolate(frame, [0, REMIND], [1, 1.1], { ...clamp, easing: Easing.out(Easing.quad) });
  const fy = interpolate(frame, [0, REMIND], [560, 540], { ...clamp, easing: Easing.out(Easing.quad) });
  const side = spring({ frame: Math.max(0, frame - Math.round(REMIND * 0.45)), fps, config: { damping: 18, stiffness: 120 } });
  return (
    <Backdrop>
      <div style={{ position: "absolute", left: 0, top: 0, width: 1920, height: 1080, transform: "translateX(-300px)" }}>
        <Window src="settings-reminders.png" iw={1040} ih={1120} width={760} top={100} cam={{ k, fx: 520, fy }} rise={rise} />
      </div>
      <div style={{ position: "absolute", left: 1090, top: 330, width: 640, color: C.white, opacity: side, transform: `translateX(${(1 - side) * 40}px)` }}>
        <div style={{ fontSize: 30, color: C.mint, letterSpacing: 6, textTransform: "uppercase" }}>Two Macs</div>
        <div style={{ fontSize: 50, fontWeight: 700, lineHeight: 1.15, marginTop: 12 }}>Keep the folder in iCloud Drive or Dropbox</div>
        <div style={{ fontSize: 30, lineHeight: 1.4, marginTop: 18, color: "rgba(255,255,255,0.85)" }}>Gloamlog watches it, never treats a file that is not downloaded yet as empty, and keeps both copies when they conflict.</div>
      </div>
      <Voice line={L.s10} at={LEAD} /><Caption line={L.s10} at={LEAD} />
    </Backdrop>
  );
};

const Check: React.FC = () => (
  <svg width="52" height="52" viewBox="0 0 52 52"><circle cx="26" cy="26" r="24" fill={C.accent} />
    <path d="M15 27l8 8 14-16" stroke="#fff" strokeWidth="5" fill="none" strokeLinecap="round" strokeLinejoin="round" /></svg>
);

const Privacy: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const rows = ["No account, no sign-in", "No analytics, no telemetry", "One network request: the update check you click"];
  return (
    <AbsoluteFill style={{ background: `radial-gradient(1300px 760px at 50% 28%, ${C.green} 0%, ${C.deep} 72%)`, fontFamily: FONT, alignItems: "center", justifyContent: "center", paddingBottom: 150 }}>
      <div style={{ color: C.mint, fontSize: 28, letterSpacing: 7, textTransform: "uppercase", opacity: interpolate(frame, [0, 16], [0, 1], clamp) }}>Private by design</div>
      <div style={{ color: C.white, fontSize: 74, fontWeight: 700, letterSpacing: -1, marginTop: 10, marginBottom: 44, opacity: interpolate(frame, [6, 24], [0, 1], clamp) }}>Your notes stay on your Mac</div>
      <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-start", gap: 22 }}>
        {rows.map((r, i) => {
          const s = spring({ frame: Math.max(0, frame - (28 + i * 30)), fps, config: { damping: 18, stiffness: 140 } });
          return (
            <div key={r} style={{ display: "flex", alignItems: "center", gap: 22, fontSize: 44, color: C.white, opacity: s, transform: `translateX(${(1 - s) * 40}px)` }}>
              <Check /><span>{r}</span>
            </div>
          );
        })}
      </div>
      <Voice line={L.s11} at={LEAD} /><Caption line={L.s11} at={LEAD} />
    </AbsoluteFill>
  );
};

const Install: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const cmd1 = "curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash";
  const cmd2 = "brew install --cask --no-quarantine dasariprashant0/gloamlog/gloamlog";
  const typed1 = Math.floor(interpolate(frame, [18, 78], [0, cmd1.length], clamp));
  const typed2 = Math.floor(interpolate(frame, [96, 140], [0, cmd2.length], clamp));
  const caret = Math.floor(frame / 16) % 2 === 0;
  const after = spring({ frame: Math.max(0, frame - 150), fps, config: { damping: 200 } });
  return (
    <AbsoluteFill style={{ background: C.deep, fontFamily: FONT, alignItems: "center", justifyContent: "center", paddingBottom: 150 }}>
      <div style={{ color: C.white, fontSize: 64, fontWeight: 700, marginBottom: 34, opacity: interpolate(frame, [0, 16], [0, 1], clamp) }}>Install in one line</div>
      <div style={{ width: 1500, background: "#07201B", borderRadius: 20, border: "1px solid rgba(255,255,255,0.12)", boxShadow: "0 24px 60px rgba(0,0,0,0.4)",
                    padding: "24px 34px 30px", fontFamily: MONO, fontSize: 26, color: "#E8F3EE", whiteSpace: "pre-wrap", overflowWrap: "anywhere", lineHeight: 1.6 }}>
        <div style={{ display: "flex", gap: 10, marginBottom: 16 }}>
          {["#FF5F57", "#FEBC2E", "#28C840"].map((c) => <div key={c} style={{ width: 16, height: 16, borderRadius: 8, background: c }} />)}
        </div>
        <span style={{ color: C.mint }}>$ </span>{cmd1.slice(0, typed1)}{frame < 92 ? <span style={{ opacity: caret ? 1 : 0 }}>▍</span> : null}
        {frame >= 92 ? <><br /><span style={{ color: "rgba(232,243,238,0.5)" }}># or, with Homebrew</span><br /><span style={{ color: C.mint }}>$ </span>{cmd2.slice(0, typed2)}<span style={{ opacity: caret ? 1 : 0 }}>▍</span></> : null}
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 18, marginTop: 40, color: "rgba(255,255,255,0.92)", fontSize: 38, opacity: after, transform: `translateY(${(1 - after) * 16}px)` }}>
        <Img src={staticFile("icon-512.png")} style={{ width: 64, height: 64 }} />
        <span>github.com/dasariprashant0/gloamlog</span><span style={{ color: C.mint }}>·</span><span>MIT licence</span><span style={{ color: C.mint }}>·</span><span>macOS 13+</span>
      </div>
      <Voice line={L.s12} at={LEAD} /><Caption line={L.s12} at={LEAD} />
    </AbsoluteFill>
  );
};

export const Demo: React.FC = () => (
  <TransitionSeries>
    <TransitionSeries.Sequence durationInFrames={TITLE}><Title /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={PAGE}><Page /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={CONNECT}><Connect /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={CLAUDE + GIT}><ClaudeGit /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={JOT}><Jot /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={CATCH}><CatchUp /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={SEARCH}><SearchScene /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={REVIEW}><Review /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={REMIND}><Reminders /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={PRIVACY}><Privacy /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={INSTALL}><Install /></TransitionSeries.Sequence>
  </TransitionSeries>
);
