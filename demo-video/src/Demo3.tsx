// Demo3.tsx - the Gloamlog demo, motion edition: the app's screens are rebuilt as animatable HTML (src/ui.tsx) so the UI itself
// moves (typing, a cursor that clicks, toggles, counters, highlights) over one continuous dusk background. Narration: edge-tts.
import React from "react";
import { AbsoluteFill, Audio, Easing, Sequence, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { TransitionSeries, linearTiming } from "@remotion/transitions";
import { fade } from "@remotion/transitions/fade";
import { AppWindow, Sidebar, Page, PageItem, DotState, SettingsPane, SettingsRow, Toggle, PrimaryButton, JotPanel, CatchUpList, SearchResults,
         WeekTiles, ReviewCard, Cursor, Notification, Keycap, Terminal, ExportMenu, SunIcon, P, FONT, MONO } from "./ui";

export const FPS = 30;
const T = 15, LEAD = 12;
const clamp = { extrapolateLeft: "clamp" as const, extrapolateRight: "clamp" as const };
const C = { deep: "#0B2B26", green: "#1B4A3E", teal: "#2E8A74", accent: "#0E7A5F", mint: "#45C29C", peach: "#F6C48E", white: "#FFFFFF" };

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

// MARK: helpers
const sp = (fps: number, f: number, c: { damping?: number; stiffness?: number; mass?: number } = {}) =>
  spring({ frame: Math.max(0, f), fps, config: { damping: 18, stiffness: 120, ...c } });
const ip = (f: number, from: number[], to: number[], e?: (t: number) => number) => interpolate(f, from, to, { ...clamp, ...(e ? { easing: e } : {}) });
const typedN = (f: number, start: number, total: number, cps = 55) => Math.max(0, Math.min(total, Math.floor(((f - start) / FPS) * cps)));
const eo = Easing.out(Easing.cubic);

const Voice: React.FC<{ line: Line; at: number }> = ({ line, at }) => (
  <Sequence from={at} layout="none"><Audio src={staticFile(line.file)} /></Sequence>
);
const Sfx: React.FC<{ name: "pop" | "click" | "whoosh" | "chime"; at: number; volume?: number }> = ({ name, at, volume = 0.45 }) => (
  <Sequence from={at} layout="none"><Audio src={staticFile(`audio/${name}.wav`)} volume={volume} /></Sequence>
);

const Caption: React.FC<{ line: Line; at: number; until?: number }> = ({ line, at, until }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  if (frame < at - 6 || (until !== undefined && frame > until)) return null;
  const words = (line.caption ?? line.text).split(" ");
  const per = (line.seconds * fps * 0.85) / words.length;
  const boxIn = ip(frame, [at - 6, at + 4], [0, 1]);
  const boxOut = until === undefined ? 1 : ip(frame, [until - 8, until], [1, 0]);
  return (
    <div style={{ position: "absolute", left: 0, right: 0, bottom: 64, display: "flex", justifyContent: "center", opacity: boxIn * boxOut }}>
      <div style={{ background: "rgba(8,30,26,0.82)", color: C.white, padding: "18px 32px", borderRadius: 22, fontFamily: FONT, fontSize: 38, lineHeight: 1.3,
                    maxWidth: 1480, textAlign: "center", boxShadow: "0 12px 40px rgba(0,0,0,0.3)", border: "1px solid rgba(255,255,255,0.08)" }}>
        {words.map((w, i) => {
          const s = spring({ frame: Math.max(0, frame - at - i * per), fps, config: { damping: 200, stiffness: 260 } });
          return <span key={i} style={{ display: "inline-block", opacity: s, transform: `translateY(${(1 - s) * 12}px)`, marginRight: 10 }}>{w}</span>;
        })}
      </div>
    </div>
  );
};

/// One continuous dusk behind every scene: two slow drifting glows over the deep green.
const Dusk: React.FC = () => {
  const frame = useCurrentFrame();
  const t = frame / FPS;
  const x1 = 50 + 14 * Math.sin(t * 0.21), y1 = 32 + 10 * Math.cos(t * 0.17);
  const x2 = 68 + 16 * Math.cos(t * 0.13), y2 = 78 + 8 * Math.sin(t * 0.19);
  return (
    <AbsoluteFill style={{ background: `radial-gradient(1000px 680px at ${x1}% ${y1}%, rgba(46,138,116,0.5) 0%, rgba(46,138,116,0) 70%),
                                         radial-gradient(1100px 760px at ${x2}% ${y2}%, rgba(246,196,142,0.16) 0%, rgba(246,196,142,0) 70%),
                                         linear-gradient(180deg, #0B2B26 0%, #113B33 55%, #0A2622 100%)` }} />
  );
};

/// Slides and scales content in like a spring, optionally with a 3D tilt settling flat.
const Rise: React.FC<{ at?: number; tilt?: boolean; children?: React.ReactNode; style?: React.CSSProperties }> = ({ at = 0, tilt, children, style }) => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const s = sp(fps, frame - at, { damping: 20, stiffness: 80 });
  return (
    <div style={{ ...style, opacity: s, transform: `perspective(1800px) translateY(${(1 - s) * 70}px) scale(${0.96 + 0.04 * s})${tilt ? ` rotateX(${(1 - s) * 7}deg)` : ""}`, transformOrigin: "50% 60%" }}>
      {children}
    </div>
  );
};

/// A cursor that glides between waypoints [frame, x, y] with a spring per leg, and presses at `clicks` frames.
const Pointer: React.FC<{ path: [number, number, number][]; clicks?: number[] }> = ({ path, clicks = [] }) => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  let x = path[0][1], y = path[0][2];
  for (let i = 1; i < path.length; i++) {
    const g = sp(fps, frame - path[i][0], { damping: 24, stiffness: 70 });
    x += (path[i][1] - x) * g; y += (path[i][2] - y) * g;
  }
  const pressed = clicks.reduce((m, c) => Math.max(m, ip(frame, [c - 2, c + 1, c + 6], [0, 1, 0])), 0);
  const vis = ip(frame, [path[0][0] - 4, path[0][0] + 6], [0, 1]);
  return <div style={{ opacity: vis }}><Cursor x={x} y={y} pressed={pressed} /></div>;
};

// MARK: shared data
const OCT: { n: number; state?: DotState; muted?: boolean; selected?: boolean; today?: boolean }[] = [
  ...[27, 28, 29, 30].map((n) => ({ n, muted: true })), { n: 1 }, { n: 2 }, { n: 3 },
  { n: 4 }, { n: 5 }, { n: 6 }, { n: 7 }, { n: 8 }, { n: 9 }, { n: 10 }, { n: 11 }, { n: 12 }, { n: 13 }, { n: 14 }, { n: 15 }, { n: 16 }, { n: 17 },
  { n: 18 }, { n: 19 }, { n: 20 }, { n: 21 }, { n: 22 }, { n: 23 }, { n: 24 }, { n: 25 }, { n: 26 }, { n: 27 }, { n: 28 }, { n: 29 }, { n: 30 }, { n: 31 },
  ...[1, 2, 3, 4, 5, 6, 7].map((n) => ({ n, muted: true })),
];
const DOTS: Record<number, DotState> = { 1: "logged", 2: "logged", 3: "partial", 5: "logged", 6: "missed", 8: "logged", 9: "partial", 10: "logged" };
const days = (frame: number, at: number, selected = 11, catchup = false) =>
  OCT.map((d, i) => ({ ...d, selected: !d.muted && d.n === selected, today: !d.muted && d.n === 11,
                       state: (!d.muted && DOTS[d.n] && frame > at + (i % 14) * 3 ? DOTS[d.n] : "none") as DotState }));
const RECENT = [{ label: "Sat 10 Oct", state: "logged" as DotState }, { label: "Fri 9 Oct", state: "partial" as DotState }, { label: "Thu 8 Oct", state: "logged" as DotState },
                { label: "Tue 6 Oct", state: "missed" as DotState }];
const WIN = { width: 1480, height: 850, left: 220, top: 44 };

// MARK: scenes
const Title: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const rise = sp(fps, frame - 4, { damping: 16, stiffness: 55 });
  const lines = ip(frame, [26, 64], [0, 1], eo);
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", paddingBottom: 150, fontFamily: FONT }}>
      <div style={{ filter: "drop-shadow(0 30px 50px rgba(0,0,0,0.4))" }}><SunIcon size={340} rise={rise} lines={lines} /></div>
      <div style={{ display: "flex", marginTop: 34 }}>
        {"Gloamlog".split("").map((ch, i) => { const s = sp(fps, frame - 34 - i * 3, { damping: 14, stiffness: 140 });
          return <span key={i} style={{ color: C.white, fontSize: 118, fontWeight: 700, letterSpacing: -3, opacity: s, transform: `translateY(${(1 - s) * 30}px)`, display: "inline-block" }}>{ch}</span>; })}
      </div>
      <div style={{ color: "rgba(255,255,255,0.9)", fontSize: 40, marginTop: 8, opacity: ip(frame, [66, 86], [0, 1]) }}>The work journal that can write itself</div>
      <Voice line={L.s1} at={LEAD} /><Caption line={L.s1} at={LEAD} />
    </AbsoluteFill>
  );
};

const I1 = "Fixed the invoice export showing yesterday's date. The cause was a time-zone off-by-one in the date formatter; added a test for it.";
const I2 = "Wrote the migration plan for the settings page and got it reviewed. Decided to ship it in two steps.";
const I3 = "Tracked down why the nightly job was slow: one query was missing an index. Added it; the job went from 40 minutes to 3.";

/// The page typing itself under the sidebar, dots popping into the calendar, a to-do getting ticked.
const PageScene: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const t1 = typedN(frame, 48, I1.length, 62), t2 = typedN(frame, 150, I2.length, 62);
  const items: PageItem[] = [
    { id: "a", text: I1, time: "10:12", typed: t1, caret: frame >= 48 && t1 < I1.length, enter: sp(fps, frame - 44) },
    { id: "b", text: I2, time: "11:40", typed: t2, caret: frame >= 150 && t2 < I2.length, enter: sp(fps, frame - 146) },
  ].filter((it) => (it.id === "b" ? frame >= 146 : frame >= 44));
  const fin: PageItem[] = [
    { id: "f1", text: "Invoice export date fix", kind: "done", enter: sp(fps, frame - 236) },
    { id: "f2", text: "Settings page migration plan", kind: "done", enter: sp(fps, frame - 254) },
  ].filter((it) => frame >= (it.id === "f1" ? 236 : 254));
  const todo: PageItem[] = frame >= 286 ? [{ id: "t1", text: "Check the CSV import", kind: frame >= 326 ? "done" : "todo", enter: sp(fps, frame - 286), highlight: ip(frame, [326, 334, 352, 366], [0, 1, 1, 0]) }] : [];
  const sections = [{ heading: "What I did", items }, ...(frame >= 230 ? [{ heading: "Finished", items: fin }] : []), ...(frame >= 280 ? [{ heading: "To do next", items: todo }] : [])];
  const logged = frame >= 310;
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise tilt style={{ position: "absolute", left: WIN.left, top: WIN.top }}>
        <AppWindow width={WIN.width} height={WIN.height}
          sidebar={<Sidebar selected="today" badge={2} monthLabel="October 2026" days={days(frame, 30)} recent={RECENT} streak="4 day streak" />}>
          <Page date="Sunday, 11 October" meta={logged ? "Today" : "Today"} metaLogged={logged} sections={sections} />
        </AppWindow>
      </Rise>
      <Sfx name="pop" at={44} /><Sfx name="pop" at={146} volume={0.35} /><Sfx name="pop" at={236} volume={0.35} /><Sfx name="chime" at={326} volume={0.35} />
      <Voice line={L.s2} at={LEAD} /><Caption line={L.s2} at={LEAD} />
    </AbsoluteFill>
  );
};

/// Settings > Sources: the cursor glides to Connect Claude Code, clicks, the button becomes Connected and a toast confirms.
const Connect: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const press = ip(frame, [80, 84, 90], [0, 1, 0]);
  const done = frame >= 92;
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise style={{ position: "absolute", left: 410, top: 120 }}>
        <SettingsPane heading="Fill in my log from my work" width={1100}>
          <SettingsRow title="Claude Code" help="Reads the session files Claude Code keeps in ~/.claude, only for the projects you tick." control={<Toggle on={sp(fps, frame - 28, { damping: 16 })} />} />
          <SettingsRow title="Every session and tab" help="Reads all your Claude Code sessions, in every folder and tab, without ticking projects." control={<Toggle on={sp(fps, frame - 42, { damping: 16 })} />} />
          <SettingsRow title="Claude writes my log" help="Lets Claude Code add a short entry (the problem, what it did, what is left) to today's page when a piece of work is done."
                       control={<PrimaryButton label="Connect Claude Code" pressed={press} done={done} />} />
        </SettingsPane>
      </Rise>
      {done && (
        <Rise at={96} style={{ position: "absolute", left: 410, top: 600 }}>
          <div style={{ display: "inline-flex", alignItems: "center", gap: 14, background: "rgba(69,194,156,0.14)", border: "1px solid rgba(69,194,156,0.5)", color: C.white, borderRadius: 16, padding: "14px 22px", fontSize: 26 }}>
            <span style={{ color: C.mint, fontWeight: 700 }}>✓</span> Helper registered with Claude Code <span style={{ color: C.mint }}>·</span> ~/.claude/CLAUDE.md updated
          </div>
        </Rise>
      )}
      <Pointer path={[[18, 1560, 960], [40, 1385, 447]]} clicks={[84]} />
      <Sfx name="click" at={84} /><Sfx name="chime" at={94} volume={0.4} />
      <Voice line={L.s3} at={LEAD} /><Caption line={L.s3} at={LEAD} />
    </AbsoluteFill>
  );
};

/// A Claude Code session on the left; its entry appears on the page on the right. Then Git: a toggle, and a commit line lands.
const ClaudeGit: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const g = CLAUDE;
  const q = "fix the invoice export date bug";
  const r = "Found a time-zone off-by-one in the date formatter. Fixed it and added a test.";
  const term = [
    { prompt: true, text: q, typed: typedN(frame, 10, q.length, 40) },
    ...(frame >= 72 ? [{ text: r, typed: typedN(frame, 72, r.length, 70), dim: true }] : []),
    ...(frame >= 150 ? [{ text: "✓ log_work · entry added to Gloamlog", typed: typedN(frame, 150, 40, 80) }] : []),
  ];
  const entry = "Fixed the invoice export showing yesterday's date. The cause was a time-zone off-by-one in the date formatter; added a test for it. Still open: the CSV import has the same pattern.";
  const te = typedN(frame, 174, entry.length, 80);
  const commit = "Committed: Add retry with backoff to the sync worker (sync-service 3f2a9c1)";
  const items: PageItem[] = [
    ...(frame >= 172 ? [{ id: "c", text: entry, time: "10:12", typed: te, caret: te < entry.length, enter: sp(fps, frame - 172), highlight: ip(frame, [232, 246, 300, 316], [0, 1, 1, 0]) }] : []),
    { id: "b", text: I2, time: "11:40", enter: 1 },
    ...(frame >= g + 44 ? [{ id: "g", text: commit, time: "14:05", enter: sp(fps, frame - (g + 44)), highlight: ip(frame, [g + 52, g + 66, g + 140, g + 160], [0, 1, 1, 0]) }] : []),
  ];
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise style={{ position: "absolute", left: 90, top: 110 }}>
        <Terminal title="Claude Code" lines={term} width={820} />
      </Rise>
      {frame >= g && (
        <Rise at={g} style={{ position: "absolute", left: 90, top: 640 }}>
          <SettingsPane heading="Git" width={820}>
            <SettingsRow title="Add commits for me" help="Writes today's commits and pull requests into your page as you work." control={<Toggle on={sp(fps, frame - (g + 14), { damping: 16 })} />} />
          </SettingsPane>
        </Rise>
      )}
      <Rise at={6} tilt style={{ position: "absolute", left: 970, top: 90 }}>
        <AppWindow width={870} height={910}>
          <Page date="Sunday, 11 October" meta="Today" metaLogged sections={[{ heading: "What I did", items }]} />
        </AppWindow>
      </Rise>
      <Sfx name="pop" at={150} volume={0.35} /><Sfx name="whoosh" at={172} volume={0.3} /><Sfx name="pop" at={g + 44} />
      <Voice line={L.s4} at={LEAD} /><Caption line={L.s4} at={LEAD} until={g - 2} />
      <Voice line={L.s5} at={g + LEAD} /><Caption line={L.s5} at={g + LEAD} />
    </AbsoluteFill>
  );
};

/// Quick capture: keycaps press, the panel springs up, a line is typed, Return, and it lands on the dimmed page behind.
const Jot: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const press = ip(frame, [14, 18, 26], [0, 1, 0]);
  const ret = ip(frame, [128, 132, 140], [0, 1, 0]);
  const text = "Fixed the invoice export showing yesterday's date; added a test for it";
  const tt = typedN(frame, 44, text.length, 48);
  const panel = sp(fps, frame - 28, { damping: 15, stiffness: 110 });
  const fly = ip(frame, [138, 160], [0, 1], Easing.in(Easing.cubic));
  const bright = ip(frame, [146, 170], [0.45, 1]);
  const items: PageItem[] = [
    { id: "a", text: I1, time: "10:12", enter: 1 }, { id: "b", text: I2, time: "11:40", enter: 1 },
    ...(frame >= 152 ? [{ id: "j", text: text, time: "14:32", enter: sp(fps, frame - 152), highlight: ip(frame, [158, 170, 230, 250], [0, 1, 1, 0]) }] : []),
  ];
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <div style={{ position: "absolute", left: WIN.left, top: 150, opacity: bright, transform: `scale(${0.9 + 0.1 * ip(frame, [146, 170], [0, 1], eo)})`, transformOrigin: "50% 100%" }}>
        <AppWindow width={WIN.width} height={WIN.height}
          sidebar={<Sidebar selected="today" badge={2} monthLabel="October 2026" days={days(frame, -100)} recent={RECENT} streak="4 day streak" />}>
          <Page date="Sunday, 11 October" meta="Today" metaLogged sections={[{ heading: "What I did", items }]} />
        </AppWindow>
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 30, display: "flex", justifyContent: "center", gap: 18, opacity: ip(frame, [140, 156], [1, 0]) }}>
        {["⌃", "⌥", "J"].map((k) => <Keycap key={k} label={k} pressed={press} />)}
        <div style={{ width: 40 }} /><Keycap label="↩" pressed={ret} style={{ opacity: ip(frame, [110, 124], [0, 1]) }} />
      </div>
      <div style={{ position: "absolute", left: 410, top: 330, opacity: panel * (1 - fly), transform: `translateY(${(1 - panel) * 40 + fly * 420}px) scale(${(0.92 + 0.08 * panel) * (1 - 0.45 * fly)})`, transformOrigin: "50% 100%" }}>
        <JotPanel text={text} typed={tt} time="14:32" width={1100} />
      </div>
      <Sfx name="click" at={16} /><Sfx name="whoosh" at={28} volume={0.35} /><Sfx name="click" at={130} /><Sfx name="pop" at={152} />
      <Voice line={L.s6} at={LEAD} /><Caption line={L.s6} at={LEAD} />
    </AbsoluteFill>
  );
};

/// Catch Up: the cursor skips one day and logs another; the badge counts down.
const CatchUp: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const skipped = frame >= 70, logged = frame >= 130;
  const count = 3 - (skipped ? 1 : 0) - (logged ? 1 : 0);
  const rows = [
    { week: "Week of 27 Sep", label: "Tue 29 Sep", state: (skipped ? "skipped" : "missed") as "missed" | "skipped" | "logged", enter: sp(fps, frame - 20) },
    { week: "Week of 4 Oct", label: "Tue 6 Oct", state: (logged ? "logged" : "missed") as "missed" | "skipped" | "logged", enter: sp(fps, frame - 28) },
    { week: "Week of 4 Oct", label: "Fri 9 Oct", state: "missed" as const, enter: sp(fps, frame - 36) },
  ];
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise tilt style={{ position: "absolute", left: WIN.left, top: WIN.top }}>
        <AppWindow width={WIN.width} height={WIN.height}
          sidebar={<Sidebar selected="catchup" badge={count} monthLabel="October 2026" days={days(frame, -100)} recent={RECENT} streak="4 day streak" />}>
          <CatchUpList days={rows} count={count} />
        </AppWindow>
      </Rise>
      <Pointer path={[[26, 1700, 980], [44, 1612, 400], [90, 1482, 548]]} clicks={[68, 126]} />
      <Sfx name="click" at={68} /><Sfx name="pop" at={70} volume={0.3} /><Sfx name="click" at={126} /><Sfx name="chime" at={132} volume={0.4} />
      <Voice line={L.s7} at={LEAD} /><Caption line={L.s7} at={LEAD} />
    </AbsoluteFill>
  );
};

/// Search: the query types in the sidebar, results stagger in, the count rolls up.
const SearchScene: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const q = "pricing"; const tq = typedN(frame, 8, q.length, 9);
  const hitsAll = [
    { date: "Tuesday, 6 October 2026", heading: "What I did", snippet: "Reviewed the new pricing page and left comments on the hero copy. Synced with Ana.", match: "pricing" },
    { date: "Monday, 5 October 2026", heading: "Finished", snippet: "Pricing table update shipped. Form fix merged.", match: "Pricing" },
    { date: "Monday, 5 October 2026", heading: "To do next", snippet: "Review the pricing page copy with Ana. Ship the webinar page.", match: "pricing" },
    { date: "Thursday, 1 October 2026", heading: "Finished", snippet: "Pricing table update shipped.", match: "Pricing" },
  ];
  const hits = hitsAll.map((h, i) => ({ ...h, enter: sp(fps, frame - 52 - i * 12) })).filter((_, i) => frame >= 52 + i * 12);
  const n = Math.round(ip(frame, [52, 118], [0, 59], eo));
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise tilt style={{ position: "absolute", left: WIN.left, top: WIN.top }}>
        <AppWindow width={WIN.width} height={WIN.height}
          sidebar={<Sidebar selected="search" searchText={q.slice(0, tq)} monthLabel="October 2026" days={days(frame, -100)} recent={RECENT} streak="4 day streak" />}>
          <SearchResults query={q.slice(0, tq)} hits={hits} summary={frame >= 52 ? `${n} matches in ${Math.round(ip(frame, [52, 118], [0, 47], eo))} days` : ""} />
        </AppWindow>
      </Rise>
      <Sfx name="pop" at={52} volume={0.3} />
      <Voice line={L.s8} at={LEAD} /><Caption line={L.s8} at={LEAD} />
    </AbsoluteFill>
  );
};

/// Review: week tiles pop in, cards enter, the cursor opens Export.
const Review: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const tiles = ["Sun 4", "Mon 5", "Tue 6", "Wed 7", "Thu 8", "Fri 9", "Sat 10"].map((l, i) => ({ label: l.split(" ")[0], n: Number(l.split(" ")[1]), words: [undefined, "57 words", "15 words", undefined, "88 words", "41 words", "102 words"][i], current: i === 6, pop: sp(fps, frame - 14 - i * 5, { damping: 14, stiffness: 150 }) }));
  const open = frame >= 134;
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise tilt style={{ position: "absolute", left: WIN.left, top: WIN.top }}>
        <AppWindow width={WIN.width} height={WIN.height}
          sidebar={<Sidebar selected="review" monthLabel="October 2026" days={days(frame, -100)} recent={RECENT} streak="4 day streak" />}>
          <div style={{ padding: "44px 48px 0", position: "relative" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <div style={{ fontSize: 42, fontWeight: 700, color: P.text, letterSpacing: -0.5 }}>Week of 4 to 10 Oct 2026</div>
              <div style={{ border: `1px solid ${P.border}`, borderRadius: 10, padding: "8px 16px", fontSize: 18, color: P.text, background: open ? "#E9EFEC" : "#fff" }}>⬆ Export</div>
            </div>
            <div style={{ fontSize: 18, color: P.text2, marginTop: 10 }}>Logged 5 of 5 · 3 tasks done · 4 still open</div>
            <div style={{ marginTop: 26 }}><WeekTiles days={tiles} /></div>
            <div style={{ marginTop: 28 }}>
              {frame >= 64 && <ReviewCard date="Mon 5 Oct" heading="What I did" text="Webinar landing page wireframes; fixed the tracking on the demo form. Synced with Ana about the signup form and left comments on the hero copy." enter={sp(fps, frame - 64)} />}
              {frame >= 92 && <ReviewCard date="Thu 8 Oct" heading="What I did" text="Reviewed the new pricing page, shipped the pricing table update and merged the form fix." enter={sp(fps, frame - 92)} style={{ marginTop: 16 }} />}
            </div>
            {open && <div style={{ position: "absolute", right: 48, top: 96 }}><ExportMenu items={["Copy as Markdown", "Copy as plain text", "Copy as rich text", "Save as PDF…", "Export Markdown with images…"]} enter={sp(fps, frame - 134, { damping: 16, stiffness: 160 })} /></div>}
          </div>
        </AppWindow>
      </Rise>
      <Pointer path={[[96, 1700, 980], [104, 1598, 170]]} clicks={[132]} />
      <Sfx name="pop" at={14} volume={0.3} /><Sfx name="click" at={132} /><Sfx name="whoosh" at={134} volume={0.25} />
      <Voice line={L.s9} at={LEAD} /><Caption line={L.s9} at={LEAD} />
    </AbsoluteFill>
  );
};

/// A reminder arrives at 4:55; then two Macs share one folder with a pulse travelling between them.
const Remind: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const second = ip(frame, [118, 140], [0, 1], eo);
  const MacShape: React.FC<{ x: number; label: string }> = ({ x, label }) => (
    <div style={{ position: "absolute", left: x, top: 400, width: 300, textAlign: "center" }}>
      <div style={{ width: 300, height: 190, borderRadius: 16, background: "#0F1F1B", border: "2px solid rgba(255,255,255,0.22)", boxShadow: "0 20px 50px rgba(0,0,0,0.4)", padding: 14 }}>
        <div style={{ width: "100%", height: "100%", background: P.pageBg, borderRadius: 6, padding: 12, textAlign: "left" }}>
          <div style={{ fontSize: 13, fontWeight: 700, color: P.text }}>Sunday, 11 October</div>
          <div style={{ fontSize: 10, color: P.text2, marginTop: 6, lineHeight: 1.5 }}>What I did<br />• 10:12 Fixed the invoice export…<br />• 11:40 Wrote the migration plan…</div>
        </div>
      </div>
      <div style={{ width: 120, height: 10, background: "rgba(255,255,255,0.25)", margin: "0 auto", borderRadius: "0 0 10px 10px" }} />
      <div style={{ color: "rgba(255,255,255,0.85)", fontSize: 22, marginTop: 14 }}>{label}</div>
    </div>
  );
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <Rise style={{ position: "absolute", left: 110, top: 150 }}>
        <div style={{ width: 880, height: 560, borderRadius: 18, background: "rgba(8,30,26,0.6)", border: "1px solid rgba(255,255,255,0.14)", overflow: "hidden", position: "relative" }}>
          <div style={{ height: 36, background: "rgba(255,255,255,0.08)", display: "flex", alignItems: "center", justifyContent: "flex-end", padding: "0 18px", color: "rgba(255,255,255,0.85)", fontSize: 17 }}>Sun 11 Oct  4:55 PM</div>
          <div style={{ position: "absolute", right: 18, top: 50 }}><Notification title="Gloamlog" body="Write up your day. Two minutes is enough." enter={sp(fps, frame - 22, { damping: 16, stiffness: 110 })} /></div>
          <div style={{ position: "absolute", left: 36, bottom: 40, color: "rgba(255,255,255,0.9)", fontSize: 26, lineHeight: 1.4, opacity: ip(frame, [60, 80], [0, 1]) }}>
            <div style={{ color: C.mint, fontSize: 18, letterSpacing: 5, textTransform: "uppercase", marginBottom: 8 }}>End of day</div>
            4:55 pm on the days you pick.<br />Gentle, or strict until you write.
          </div>
        </div>
      </Rise>
      <div style={{ position: "absolute", left: 1040, top: 0, width: 880, height: 1080, opacity: second, transform: `translateX(${(1 - second) * 40}px)` }}>
        <div style={{ position: "absolute", left: 300, top: 300, color: C.white, textAlign: "center", width: 300 }}>
          <div style={{ display: "inline-flex", alignItems: "center", gap: 10, background: "rgba(255,255,255,0.1)", border: "1px solid rgba(255,255,255,0.25)", borderRadius: 14, padding: "10px 18px", fontSize: 22 }}>📁 iCloud Drive / Dropbox</div>
        </div>
        <MacShape x={60} label="Work Mac" /><MacShape x={520} label="Home Mac" />
        <div style={{ position: "absolute", left: 360, top: 495, width: 160, height: 4, background: "rgba(255,255,255,0.18)", borderRadius: 2 }} />
        {[0, 1, 2].map((k) => { const p = ((frame * 0.018) + k / 3) % 1; const dir = k % 2 === 0; const x = 360 + (dir ? p : 1 - p) * 150;
          return <div key={k} style={{ position: "absolute", left: x, top: 491, width: 12, height: 12, borderRadius: 6, background: C.mint, boxShadow: `0 0 16px ${C.mint}`, opacity: Math.sin(p * Math.PI) }} />; })}
        <div style={{ position: "absolute", left: 60, top: 690, width: 760, textAlign: "center", color: "rgba(255,255,255,0.85)", fontSize: 26, lineHeight: 1.4, opacity: ip(frame, [190, 210], [0, 1]) }}>
          Gloamlog watches the folder, never treats a file that is not downloaded yet as empty, and keeps both copies when they conflict.
        </div>
      </div>
      <Sfx name="whoosh" at={22} volume={0.3} /><Sfx name="pop" at={120} volume={0.3} />
      <Voice line={L.s10} at={LEAD} /><Caption line={L.s10} at={LEAD} />
    </AbsoluteFill>
  );
};

const Check: React.FC = () => (
  <svg width="52" height="52" viewBox="0 0 52 52"><circle cx="26" cy="26" r="24" fill={C.accent} />
    <path d="M15 27l8 8 14-16" stroke="#fff" strokeWidth="5" fill="none" strokeLinecap="round" strokeLinejoin="round" /></svg>
);

/// Privacy: three facts, and a map where nothing leaves the Mac except one dashed line when the update check is clicked.
const Privacy: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const rows = ["No account, no sign-in", "No analytics, no telemetry", "One network request: the update check you click"];
  const nodes = [["Cloud sync", -210, -170], ["Analytics", 230, -150], ["Accounts", -250, 150], ["Ad networks", 240, 170]] as [string, number, number][];
  const on = ip(frame, [150, 168], [0, 1]);
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <div style={{ position: "absolute", left: 150, top: 230 }}>
        <div style={{ color: C.mint, fontSize: 26, letterSpacing: 7, textTransform: "uppercase", opacity: ip(frame, [0, 16], [0, 1]) }}>Private by design</div>
        <div style={{ color: C.white, fontSize: 66, fontWeight: 700, letterSpacing: -1, marginTop: 10, marginBottom: 40, opacity: ip(frame, [6, 24], [0, 1]), maxWidth: 960, lineHeight: 1.1 }}>Your notes stay on your Mac</div>
        {rows.map((r, i) => { const s = sp(fps, frame - (26 + i * 28), { damping: 18, stiffness: 140 });
          return <div key={r} style={{ display: "flex", alignItems: "center", gap: 20, fontSize: 36, color: C.white, opacity: s, transform: `translateX(${(1 - s) * 40}px)`, marginBottom: 18 }}><Check /><span>{r}</span></div>; })}
      </div>
      <div style={{ position: "absolute", left: 1330, top: 440 }}>
        {nodes.map(([n, dx, dy], i) => { const s = sp(fps, frame - 20 - i * 10); const ang = Math.atan2(dy, dx) * 180 / Math.PI, lenL = Math.hypot(dx, dy);
          return (<React.Fragment key={n}>
            <div style={{ position: "absolute", left: 0, top: 0, width: lenL, height: 2, background: "rgba(255,255,255,0.14)", transformOrigin: "0 50%", transform: `rotate(${ang}deg)`, opacity: s }} />
            <div style={{ position: "absolute", left: dx - 70, top: dy - 22, width: 140, textAlign: "center", padding: "8px 0", borderRadius: 12, border: "1px solid rgba(255,255,255,0.18)", color: "rgba(255,255,255,0.4)", fontSize: 18, textDecoration: "line-through", opacity: s }}>{n}</div>
          </React.Fragment>); })}
        <div style={{ position: "absolute", left: 0, top: 0, width: 300, height: 2, transformOrigin: "0 50%", transform: "rotate(-90deg)", borderTop: `2px dashed ${on > 0 ? C.mint : "rgba(255,255,255,0.3)"}`, opacity: 0.5 + 0.5 * on }} />
        <div style={{ position: "absolute", left: -80, top: -330, width: 160, textAlign: "center", padding: "8px 0", borderRadius: 12, border: `1px solid ${on > 0 ? C.mint : "rgba(255,255,255,0.3)"}`, color: on > 0 ? C.mint : "rgba(255,255,255,0.6)", fontSize: 18, boxShadow: on > 0 ? `0 0 ${24 * on}px rgba(69,194,156,0.6)` : "none" }}>github.com</div>
        <div style={{ position: "absolute", left: -70, top: -30, width: 140, padding: "12px 0", textAlign: "center", borderRadius: 16, background: "rgba(255,255,255,0.1)", border: "1px solid rgba(255,255,255,0.4)", color: C.white, fontSize: 20, fontWeight: 600 }}>Your Mac</div>
        <div style={{ position: "absolute", left: -86, top: 230, width: 300, opacity: ip(frame, [120, 134], [0, 1]) }}>
          <div style={{ background: "rgba(255,255,255,0.96)", color: P.text, borderRadius: 10, padding: "8px 0", fontSize: 18, boxShadow: "0 10px 30px rgba(0,0,0,0.35)" }}>
            <div style={{ padding: "6px 16px" }}>About Gloamlog</div>
            <div style={{ padding: "6px 16px", background: on > 0 ? C.accent : "transparent", color: on > 0 ? "#fff" : P.text }}>Check for Updates…</div>
          </div>
        </div>
      </div>
      <Pointer path={[[118, 1700, 980], [130, 1332, 724]]} clicks={[150]} />
      <Sfx name="click" at={150} /><Sfx name="pop" at={156} volume={0.3} />
      <Voice line={L.s11} at={LEAD} /><Caption line={L.s11} at={LEAD} />
    </AbsoluteFill>
  );
};

/// Install: the command runs with real progress, then the end card.
const Install: React.FC = () => {
  const frame = useCurrentFrame(); const { fps } = useVideoConfig();
  const cmd1 = "curl -fsSL https://raw.githubusercontent.com/dasariprashant0/gloamlog/main/install.sh | bash";
  const pct = Math.round(ip(frame, [96, 150], [0, 100], eo));
  const bar = "▓".repeat(Math.round(pct / 4)) + "░".repeat(25 - Math.round(pct / 4));
  const lines = [
    { prompt: true, text: cmd1, typed: typedN(frame, 14, cmd1.length, 60) },
    ...(frame >= 74 ? [{ text: "Looking up the release…", dim: true }] : []),
    ...(frame >= 92 ? [{ text: `Downloading Gloamlog 0.6.6…  ${bar} ${pct}%`, dim: true }] : []),
    ...(frame >= 156 ? [{ text: "Checksum OK.", dim: true }] : []),
    ...(frame >= 168 ? [{ text: "Installed Gloamlog 0.6.6 in /Applications" }] : []),
    ...(frame >= 186 ? [{ prompt: true, text: "brew install --cask --no-quarantine dasariprashant0/gloamlog/gloamlog", typed: typedN(frame, 186, 68, 70), dim: true }] : []),
  ];
  const out = ip(frame, [214, 232], [0, 1], Easing.in(Easing.cubic));
  const card = sp(fps, frame - 226, { damping: 16, stiffness: 90 });
  return (
    <AbsoluteFill style={{ fontFamily: FONT }}>
      <div style={{ position: "absolute", left: 210, top: 180, opacity: 1 - out, transform: `scale(${1 - 0.1 * out}) translateY(${-40 * out}px)` }}>
        <Rise><Terminal title="Terminal" lines={lines} width={1500} /></Rise>
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 0, bottom: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", paddingBottom: 150, opacity: card, transform: `scale(${0.9 + 0.1 * card})` }}>
        <div style={{ filter: "drop-shadow(0 30px 50px rgba(0,0,0,0.4))" }}><SunIcon size={220} rise={1} lines={1} /></div>
        <div style={{ color: C.white, fontSize: 96, fontWeight: 700, letterSpacing: -2, marginTop: 24 }}>Gloamlog</div>
        <div style={{ color: "rgba(255,255,255,0.9)", fontSize: 34, marginTop: 6 }}>github.com/dasariprashant0/gloamlog</div>
        <div style={{ color: C.mint, fontSize: 26, marginTop: 18, letterSpacing: 3, textTransform: "uppercase" }}>Free · Open source · macOS 13+</div>
      </div>
      <Sfx name="chime" at={168} volume={0.4} /><Sfx name="whoosh" at={226} volume={0.3} />
      <Voice line={L.s12} at={LEAD} /><Caption line={L.s12} at={LEAD} />
    </AbsoluteFill>
  );
};

export const Demo3: React.FC = () => (
  <AbsoluteFill>
    <Dusk />
    <TransitionSeries>
      <TransitionSeries.Sequence durationInFrames={TITLE}><Title /></TransitionSeries.Sequence>
      <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
      <TransitionSeries.Sequence durationInFrames={PAGE}><PageScene /></TransitionSeries.Sequence>
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
      <TransitionSeries.Sequence durationInFrames={REMIND}><Remind /></TransitionSeries.Sequence>
      <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
      <TransitionSeries.Sequence durationInFrames={PRIVACY}><Privacy /></TransitionSeries.Sequence>
      <TransitionSeries.Transition presentation={fade()} timing={linearTiming({ durationInFrames: T })} />
      <TransitionSeries.Sequence durationInFrames={INSTALL}><Install /></TransitionSeries.Sequence>
    </TransitionSeries>
  </AbsoluteFill>
);
