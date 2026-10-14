// Demo.tsx - the Gloamlog demo: real screenshots, a voiceover made with macOS `say`, captions for sound-off viewing.
// Scenes: Title -> Screen (page, Claude entries, Git line: one continuous camera over today.png) -> Review -> Privacy -> Install.
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

// The narration. The clips in public/voice were made from exactly these lines (see README.md); `seconds` is each clip's length.
type Line = { file: string; seconds: number; text: string };
const L: Record<string, Line> = {
  s1: { file: "voice/s1.wav", seconds: 3.37, text: "Gloamlog is a free, open-source work journal for your Mac." },
  s2: { file: "voice/s2.wav", seconds: 6.24, text: "One page a day, saved as a plain markdown file in a folder you choose. No account, no cloud." },
  s3: { file: "voice/s3.wav", seconds: 8.42, text: "Connect Claude Code, and when a piece of work is done, Claude adds a short entry: what you were solving, what got done, and what is still open." },
  s4: { file: "voice/s4.wav", seconds: 3.44, text: "Turn on Git, and your commits and pull requests show up too." },
  s5: { file: "voice/s5.wav", seconds: 4.57, text: "At the end of the week, review it, and export it as Markdown or PDF." },
  s6: { file: "voice/s6.wav", seconds: 4.87, text: "Everything stays on your Mac. The only network request is the update check you click." },
  s7: { file: "voice/s7.wav", seconds: 5.37, text: "Install with one command, or build it from source. Free, and open source on GitHub." },
};
const len = (line: Line, tail: number) => Math.round((line.seconds + tail) * FPS) + LEAD;

const TITLE = 150;
const SEG2 = len(L.s2, 0.9), SEG3 = len(L.s3, 0.9), SEG4 = len(L.s4, 1.0);
const SCREEN = SEG2 + SEG3 + SEG4;
const REVIEW = len(L.s5, 1.0), PRIVACY = len(L.s6, 1.0), INSTALL = len(L.s7, 2.2);
export const TOTAL_FRAMES = TITLE + SCREEN + REVIEW + PRIVACY + INSTALL - 4 * T;

// MARK: shared pieces
const Voice: React.FC<{ line: Line; at: number }> = ({ line, at }) => (
  <Sequence from={at} layout="none"><Audio src={staticFile(line.file)} /></Sequence>
);

/// The narration as a caption pill: words arrive one by one over the first 85% of the clip; `until` fades it out before the next one.
const Caption: React.FC<{ line: Line; at: number; until?: number }> = ({ line, at, until }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  if (frame < at - 6 || (until !== undefined && frame > until)) return null;
  const words = line.text.split(" ");
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

/// A screenshot (2000x1520) in a window frame with a camera: `k` is the zoom, (fx, fy) the source pixel that sits at the frame's focus point.
type Cam = { k: number; fx: number; fy: number };
const Window: React.FC<{ src: string; cam: Cam; rise: number; width: number; top: number; children?: React.ReactNode }> = ({ src, cam, rise, width, top, children }) => {
  const f = width / 2000, height = 1520 * f, left = (1920 - width) / 2;
  const P = { x: width / 2, y: 540 - top };                                   // the focus lands at the screen centre
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

/// A highlight box in source pixels of the screenshot.
const Box: React.FC<{ x1: number; y1: number; x2: number; y2: number; f: number; opacity: number }> = ({ x1, y1, x2, y2, f, opacity }) => (
  <div style={{ position: "absolute", left: x1 * f, top: y1 * f, width: (x2 - x1) * f, height: (y2 - y1) * f, borderRadius: 14, opacity,
                border: `4px solid ${C.mint}`, boxShadow: "0 0 0 8px rgba(69,194,156,0.22), 0 0 40px rgba(69,194,156,0.35)",
                transform: `scale(${0.985 + 0.015 * opacity})` }} />
);

// MARK: scenes
export const Title: React.FC = () => {
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
      <Voice line={L.s1} at={LEAD} />
      <Caption line={L.s1} at={LEAD} />
    </AbsoluteFill>
  );
};

const Screen: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const rise = spring({ frame, fps, config: { damping: 22, stiffness: 90 } });
  const a = SEG2, b = SEG2 + 48, c = SEG2 + SEG3, d = SEG2 + SEG3 + 40;
  const keys = [0, a, b, c, d];
  const k = interpolate(frame, keys, [1, 1.05, 1.45, 1.45, 1.45], { ...clamp, easing: ease });
  const fx = interpolate(frame, keys, [1000, 1000, 1237, 1237, 1237], { ...clamp, easing: ease });
  const fy = interpolate(frame, keys, [620, 600, 440, 440, 632], { ...clamp, easing: ease });
  const box1 = interpolate(frame, [a + 44, a + 58, c - 14, c], [0, 1, 1, 0], clamp);
  const box2 = interpolate(frame, [d - 8, d + 8], [0, 1], clamp);
  return (
    <AbsoluteFill style={{ background: `linear-gradient(180deg, ${C.green} 0%, ${C.deep} 100%)` }}>
      <Window src="today.png" cam={{ k, fx, fy }} rise={rise} width={1560} top={56}>
        <Box x1={560} y1={322} x2={1905} y2={482} f={0.78} opacity={box1} />
        <Box x1={560} y1={598} x2={1905} y2={668} f={0.78} opacity={box2} />
      </Window>
      <Voice line={L.s2} at={LEAD} />         <Caption line={L.s2} at={LEAD} until={a - 2} />
      <Voice line={L.s3} at={a + LEAD} />     <Caption line={L.s3} at={a + LEAD} until={c - 2} />
      <Voice line={L.s4} at={c + LEAD} />     <Caption line={L.s4} at={c + LEAD} />
    </AbsoluteFill>
  );
};

const Review: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const rise = spring({ frame, fps, config: { damping: 22, stiffness: 90 } });
  const k = interpolate(frame, [0, REVIEW], [1, 1.07], { ...clamp, easing: Easing.out(Easing.quad) });
  return (
    <AbsoluteFill style={{ background: `linear-gradient(180deg, ${C.green} 0%, ${C.deep} 100%)` }}>
      <Window src="review.png" cam={{ k, fx: 1000, fy: 640 }} rise={rise} width={1560} top={56} />
      <Voice line={L.s5} at={LEAD} />
      <Caption line={L.s5} at={LEAD} />
    </AbsoluteFill>
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
      <Voice line={L.s6} at={LEAD} />
      <Caption line={L.s6} at={LEAD} />
    </AbsoluteFill>
  );
};

const Install: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const cmd = "curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash";
  const typed = Math.floor(interpolate(frame, [18, 80], [0, cmd.length], clamp));
  const caret = Math.floor(frame / 16) % 2 === 0;
  const after = spring({ frame: Math.max(0, frame - 96), fps, config: { damping: 200 } });
  return (
    <AbsoluteFill style={{ background: C.deep, fontFamily: FONT, alignItems: "center", justifyContent: "center", paddingBottom: 150 }}>
      <div style={{ color: C.white, fontSize: 64, fontWeight: 700, marginBottom: 34, opacity: interpolate(frame, [0, 16], [0, 1], clamp) }}>Install in one line</div>
      <div style={{ width: 1500, background: "#07201B", borderRadius: 20, border: "1px solid rgba(255,255,255,0.12)", boxShadow: "0 24px 60px rgba(0,0,0,0.4)",
                    padding: "24px 34px 30px", fontFamily: MONO, fontSize: 26, color: "#E8F3EE", whiteSpace: "pre-wrap", overflowWrap: "anywhere", lineHeight: 1.5 }}>
        <div style={{ display: "flex", gap: 10, marginBottom: 16 }}>
          {["#FF5F57", "#FEBC2E", "#28C840"].map((c) => <div key={c} style={{ width: 16, height: 16, borderRadius: 8, background: c }} />)}
        </div>
        <span style={{ color: C.mint }}>$ </span>{cmd.slice(0, typed)}<span style={{ opacity: caret ? 1 : 0 }}>▍</span>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 18, marginTop: 40, color: "rgba(255,255,255,0.92)", fontSize: 38, opacity: after, transform: `translateY(${(1 - after) * 16}px)` }}>
        <Img src={staticFile("icon-512.png")} style={{ width: 64, height: 64 }} />
        <span>github.com/dasariprashant0/gloamlog</span><span style={{ color: C.mint }}>·</span><span>MIT licence</span><span style={{ color: C.mint }}>·</span><span>macOS 13+</span>
      </div>
      <Voice line={L.s7} at={LEAD} />
      <Caption line={L.s7} at={LEAD} />
    </AbsoluteFill>
  );
};

export const Demo: React.FC = () => (
  <TransitionSeries>
    <TransitionSeries.Sequence durationInFrames={TITLE}><Title /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={SCREEN}><Screen /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={REVIEW}><Review /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={PRIVACY}><Privacy /></TransitionSeries.Sequence>
    <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
    <TransitionSeries.Sequence durationInFrames={INSTALL}><Install /></TransitionSeries.Sequence>
  </TransitionSeries>
);
