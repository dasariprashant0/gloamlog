// ui.tsx - Gloamlog's screens rebuilt as plain React/CSS so the demo can animate them element by element. Metrics are the real
// app's (1000x760 window) scaled by S = 1.48 to fill a 1480x930 window in the 1920x1080 frame. Every animation value is a prop
// the scene computes (0..1 progress, or a character count); nothing here reads the frame.
import React from "react";
import { Img, staticFile } from "remotion";

export const P = { deep: "#0B2B26", green: "#1B4A3E", teal: "#2E8A74", accent: "#0E7A5F", mint: "#45C29C", peach: "#F6C48E", white: "#FFFFFF",
  pageBg: "#FAFAF9", sidebarBg: "#F1F3F1", text: "#1B201E", text2: "#66706C", text3: "#9AA39F", border: "#E4E8E5", selection: "#E2EEE8",
  amber: "#C58A3A", badgeBg: "#FBEBD3", badgeText: "#A8662A", mark: "#DDEFE6" };
export const FONT = '-apple-system, "SF Pro Display", "SF Pro Text", "Helvetica Neue", Helvetica, Arial, sans-serif';
export const MONO = '"SF Mono", Menlo, Monaco, Consolas, monospace';
const S = 1.48;
const px = (n: number) => Math.round(n * S);
type CSS = React.CSSProperties;

export type DotState = "logged" | "partial" | "missed" | "skipped" | "none";
export type PageItem = { id: string; text: string; time?: string; kind?: "bullet" | "todo" | "done"; typed?: number; highlight?: number; enter?: number; caret?: boolean };

// MARK: window and sidebar
export const AppWindow: React.FC<{ width: number; height: number; sidebar?: React.ReactNode; children?: React.ReactNode; style?: CSS }> = ({ width, height, sidebar, children, style }) => (
  <div style={{ width, height, borderRadius: 16, background: P.pageBg, boxShadow: "0 40px 100px rgba(0,0,0,0.5), 0 0 0 1px rgba(255,255,255,0.1)", overflow: "hidden",
                display: "flex", position: "relative", fontFamily: FONT, color: P.text, ...style }}>
    {sidebar && <div style={{ width: px(240), flexShrink: 0, background: P.sidebarBg, borderRight: `1px solid ${P.border}`, position: "relative", overflow: "hidden" }}>{sidebar}</div>}
    <div style={{ flex: 1, position: "relative", overflow: "hidden" }}>{children}</div>
    <div style={{ position: "absolute", left: 18, top: 16, display: "flex", gap: 10 }}>
      {["#FF5F57", "#FEBC2E", "#28C840"].map((c) => <div key={c} style={{ width: 14, height: 14, borderRadius: 7, background: c, boxShadow: "inset 0 0 0 1px rgba(0,0,0,0.12)" }} />)}
    </div>
  </div>
);

const Dot: React.FC<{ state: DotState; size?: number }> = ({ state, size = px(6) }) => {
  if (state === "none") return <div style={{ width: size, height: size }} />;
  const base: CSS = { width: size, height: size, borderRadius: size / 2 };
  if (state === "logged") return <div style={{ ...base, background: P.accent }} />;
  if (state === "partial") return <div style={{ ...base, background: `linear-gradient(90deg, ${P.accent} 50%, transparent 50%)`, border: `1.5px solid ${P.accent}`, boxSizing: "border-box" }} />;
  if (state === "missed") return <div style={{ ...base, border: `1.5px solid ${P.amber}`, boxSizing: "border-box" }} />;
  return <div style={{ ...base, background: P.text3 }} />;
};

const StateIcon: React.FC<{ state: DotState }> = ({ state }) => {
  const s = px(16);
  if (state === "logged") return <svg width={s} height={s} viewBox="0 0 16 16"><circle cx="8" cy="8" r="8" fill={P.accent} /><path d="M4.5 8.2l2.3 2.3 4.7-5" stroke="#fff" strokeWidth="1.8" fill="none" strokeLinecap="round" strokeLinejoin="round" /></svg>;
  if (state === "partial") return <svg width={s} height={s} viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" stroke={P.text} strokeWidth="1.5" fill="none" /><path d="M8 1a7 7 0 0 1 0 14z" fill={P.text} /></svg>;
  if (state === "skipped") return <svg width={s} height={s} viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" stroke={P.text2} strokeWidth="1.5" fill="none" /><path d="M5 8h6" stroke={P.text2} strokeWidth="1.5" strokeLinecap="round" /></svg>;
  return <svg width={s} height={s} viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" stroke={P.amber} strokeWidth="1.5" strokeDasharray="2.5 2" fill="none" /></svg>;
};

export const Sidebar: React.FC<{ selected?: "today" | "catchup" | "review" | "search"; badge?: number; searchText?: string; monthLabel?: string;
  days?: { n: number; state?: DotState; muted?: boolean; selected?: boolean; today?: boolean }[]; recent?: { label: string; state: DotState; note?: string }[]; streak?: string; style?: CSS }> =
  ({ selected = "today", badge, searchText, monthLabel = "October 2026", days = [], recent = [], streak = "1 day streak", style }) => {
  const row = (key: string, label: string, icon: React.ReactNode, extra?: React.ReactNode) => (
    <div key={key} style={{ display: "flex", alignItems: "center", gap: px(10), height: px(34), padding: `0 ${px(10)}px`, borderRadius: px(8), margin: `0 ${px(8)}px`,
                            background: selected === key ? P.selection : "transparent", fontWeight: 600, fontSize: px(13), color: P.text }}>
      {icon}<span style={{ flex: 1 }}>{label}</span>{extra}
    </div>
  );
  const badgeEl = badge ? <span style={{ background: P.badgeBg, color: P.badgeText, fontSize: px(11), fontWeight: 700, padding: `${px(2)}px ${px(7)}px`, borderRadius: px(9) }}>{badge}</span> : null;
  return (
    <div style={{ position: "absolute", inset: 0, paddingTop: px(30), display: "flex", flexDirection: "column", fontFamily: FONT, ...style }}>
      <div style={{ margin: `${px(6)}px ${px(12)}px ${px(10)}px`, height: px(32), borderRadius: px(8), background: "#fff", border: `1px solid ${searchText ? P.accent : P.border}`,
                    display: "flex", alignItems: "center", gap: px(8), padding: `0 ${px(10)}px`, fontSize: px(13), color: searchText ? P.text : P.text2 }}>
        <svg width={px(13)} height={px(13)} viewBox="0 0 16 16"><circle cx="7" cy="7" r="5" stroke={P.text2} strokeWidth="1.8" fill="none" /><path d="M11 11l3.5 3.5" stroke={P.text2} strokeWidth="1.8" strokeLinecap="round" /></svg>
        <span>{searchText || "Search your logs"}</span>
      </div>
      {row("today", "Today", <StateIcon state="partial" />)}
      {row("catchup", "Catch up", <svg width={px(16)} height={px(16)} viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11.5" rx="2" stroke={P.text} strokeWidth="1.4" fill="none" /><path d="M1.5 6h13" stroke={P.text} strokeWidth="1.4" /><circle cx="11.5" cy="11" r="2.4" fill={P.text} /></svg>, badgeEl)}
      {row("review", "Review", <svg width={px(16)} height={px(16)} viewBox="0 0 16 16"><rect x="2" y="1.5" width="12" height="13" rx="2" stroke={P.text} strokeWidth="1.4" fill="none" /><path d="M5 5h6M5 8h6M5 11h4" stroke={P.text} strokeWidth="1.3" strokeLinecap="round" /></svg>)}
      <div style={{ display: "flex", alignItems: "center", margin: `${px(14)}px ${px(14)}px ${px(6)}px`, fontSize: px(14), fontWeight: 700 }}>
        {monthLabel} <span style={{ color: P.text2, fontSize: px(10), marginLeft: px(6) }}>⌄</span><span style={{ flex: 1 }} />
        <span style={{ color: P.text2, fontSize: px(13), marginRight: px(18) }}>‹</span><span style={{ color: P.text2, fontSize: px(13) }}>›</span>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(7, 1fr)", margin: `0 ${px(10)}px`, rowGap: px(2) }}>
        {["S", "M", "T", "W", "T", "F", "S"].map((d, i) => <div key={i} style={{ textAlign: "center", fontSize: px(11), color: P.text2, height: px(16) }}>{d}</div>)}
        {days.map((d, i) => (
          <div key={i} style={{ display: "flex", flexDirection: "column", alignItems: "center", height: px(30) }}>
            <div style={{ width: px(26), height: px(22), borderRadius: px(6), display: "flex", alignItems: "center", justifyContent: "center", fontSize: px(13), fontWeight: d.selected || d.today ? 700 : 400,
                          color: d.selected ? "#fff" : d.muted ? P.text3 : P.text, background: d.selected ? P.accent : "transparent", boxShadow: d.today && !d.selected ? `inset 0 0 0 1.5px ${P.accent}` : "none" }}>{d.n}</div>
            <div style={{ marginTop: px(2) }}><Dot state={d.state ?? "none"} /></div>
          </div>
        ))}
      </div>
      <div style={{ margin: `${px(12)}px ${px(18)}px ${px(4)}px`, fontSize: px(13), fontWeight: 700, color: P.text2 }}>Recent</div>
      {recent.map((r) => (
        <div key={r.label} style={{ display: "flex", alignItems: "center", gap: px(10), height: px(29), padding: `0 ${px(10)}px`, margin: `0 ${px(8)}px`, fontSize: px(13), color: P.text }}>
          <StateIcon state={r.state} /><span style={{ flex: 1 }}>{r.label}</span>{r.note && <span style={{ color: P.text2, fontSize: px(12) }}>{r.note}</span>}
        </div>
      ))}
      <div style={{ flex: 1 }} />
      <div style={{ borderTop: `1px solid ${P.border}`, height: px(40), display: "flex", alignItems: "center", padding: `0 ${px(14)}px`, fontSize: px(13), fontWeight: 700 }}>
        <span style={{ flex: 1 }}>{streak}</span>
        <svg width={px(14)} height={px(14)} viewBox="0 0 16 16"><circle cx="8" cy="8" r="2.4" stroke={P.text2} strokeWidth="1.5" fill="none" /><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M3.4 12.6l1.4-1.4M11.2 4.8l1.4-1.4" stroke={P.text2} strokeWidth="1.5" strokeLinecap="round" /></svg>
      </div>
    </div>
  );
};

// MARK: the page
const Caret: React.FC = () => <span style={{ display: "inline-block", width: 2, height: "1.05em", background: P.accent, verticalAlign: "-0.15em", marginLeft: 1 }} />;

export const Page: React.FC<{ date: string; meta?: string; metaLogged?: boolean; sections: { heading: string; items: PageItem[] }[]; style?: CSS }> = ({ date, meta, metaLogged, sections, style }) => (
  <div style={{ padding: `${px(36)}px ${px(48)}px 0`, fontFamily: FONT, color: P.text, ...style }}>
    <div style={{ fontSize: px(32), fontWeight: 700, letterSpacing: -0.5, lineHeight: 1.1 }}>{date}</div>
    <div style={{ display: "flex", alignItems: "center", gap: px(12), marginTop: px(10), fontSize: px(13) }}>
      <span style={{ color: P.text2 }}>{meta}</span>
      {metaLogged && <span style={{ color: P.accent, fontWeight: 700 }}>✓ Logged, 115 words</span>}
    </div>
    <div style={{ height: 1, background: P.border, margin: `${px(16)}px 0 ${px(14)}px` }} />
    {sections.map((s) => (
      <div key={s.heading} style={{ marginBottom: px(16) }}>
        <div style={{ fontSize: px(21), fontWeight: 700, margin: `${px(10)}px 0 ${px(8)}px` }}>{s.heading}</div>
        {s.items.map((it) => {
          const e = it.enter ?? 1;
          const text = it.typed === undefined ? it.text : it.text.slice(0, it.typed);
          const kind = it.kind ?? "bullet";
          return (
            <div key={it.id} style={{ position: "relative", display: "flex", gap: px(10), padding: `${px(5)}px ${px(10)}px`, margin: `0 -${px(10)}px`, borderRadius: px(10), fontSize: px(15), lineHeight: 1.5,
                                      opacity: e, transform: `translateY(${(1 - e) * 16}px)`,
                                      boxShadow: it.highlight ? `0 0 0 3px rgba(69,194,156,${it.highlight}), 0 0 ${28 * it.highlight}px rgba(69,194,156,${0.45 * it.highlight})` : "none",
                                      background: it.highlight ? `rgba(69,194,156,${0.08 * it.highlight})` : "transparent" }}>
              {kind === "bullet" && <span style={{ color: P.text2, width: px(12), textAlign: "center", flexShrink: 0 }}>•</span>}
              {kind !== "bullet" && (
                <span style={{ width: px(14), height: px(14), borderRadius: px(3), marginTop: px(4), flexShrink: 0, border: `1.5px solid ${kind === "done" ? P.accent : P.text2}`, background: kind === "done" ? P.accent : "transparent", display: "flex", alignItems: "center", justifyContent: "center" }}>
                  {kind === "done" && <svg width={px(10)} height={px(10)} viewBox="0 0 10 10"><path d="M2 5.2l2.1 2.1L8 3" stroke="#fff" strokeWidth="1.6" fill="none" strokeLinecap="round" strokeLinejoin="round" /></svg>}
                </span>
              )}
              <span style={{ color: kind === "done" ? P.text2 : P.text }}>
                {it.time && <span style={{ color: P.text2, marginRight: px(6) }}>{it.time}</span>}{text}{it.caret && <Caret />}
              </span>
            </div>
          );
        })}
      </div>
    ))}
  </div>
);

// MARK: settings
export const SettingsPane: React.FC<{ heading: string; children?: React.ReactNode; width?: number; style?: CSS }> = ({ heading, children, width = 1100, style }) => (
  <div style={{ width, fontFamily: FONT, ...style }}>
    <div style={{ fontSize: 24, fontWeight: 700, color: "rgba(255,255,255,0.92)", marginBottom: 12 }}>{heading}</div>
    <div style={{ background: "#F4F6F4", borderRadius: 18, padding: "0 28px", boxShadow: "0 30px 80px rgba(0,0,0,0.45)", color: P.text }}>{children}</div>
  </div>
);
export const SettingsRow: React.FC<{ title: string; help?: string; control?: React.ReactNode; style?: CSS }> = ({ title, help, control, style }) => (
  <div style={{ padding: "22px 0", borderBottom: `1px solid ${P.border}`, ...style }}>
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 20 }}>
      <div style={{ fontSize: 24 }}>{title}</div>{control}
    </div>
    {help && <div style={{ fontSize: 19, color: P.text2, marginTop: 8, lineHeight: 1.4, maxWidth: "88%" }}>{help}</div>}
  </div>
);
export const Toggle: React.FC<{ on: number }> = ({ on }) => (
  <div style={{ width: 62, height: 36, borderRadius: 18, background: `rgba(14,122,95,${on})`, boxShadow: `inset 0 0 0 ${2 * (1 - on)}px #D4D9D6`, position: "relative", flexShrink: 0 }}>
    <div style={{ position: "absolute", top: 3, left: 3 + on * 26, width: 30, height: 30, borderRadius: 15, background: "#fff", boxShadow: "0 2px 6px rgba(0,0,0,0.3)" }} />
  </div>
);
export const PrimaryButton: React.FC<{ label: string; pressed?: number; done?: boolean; style?: CSS }> = ({ label, pressed = 0, done, style }) => (
  <div style={{ display: "inline-flex", alignItems: "center", gap: 10, padding: "14px 26px", borderRadius: 12, fontSize: 22, fontWeight: 600, whiteSpace: "nowrap",
                background: done ? "transparent" : `rgb(${14 - 6 * pressed}, ${122 - 30 * pressed}, ${95 - 24 * pressed})`, color: done ? P.accent : "#fff",
                boxShadow: done ? `inset 0 0 0 2px ${P.accent}` : "0 1px 2px rgba(0,0,0,0.2)", transform: `scale(${1 - 0.05 * pressed})`, ...style }}>
    {done && <svg width="22" height="22" viewBox="0 0 22 22"><circle cx="11" cy="11" r="10" fill={P.accent} /><path d="M6 11.5l3.2 3.2L16 8" stroke="#fff" strokeWidth="2.2" fill="none" strokeLinecap="round" strokeLinejoin="round" /></svg>}
    {done ? "Connected" : label}
  </div>
);

// MARK: quick capture
export const JotPanel: React.FC<{ text: string; typed?: number; time?: string; width?: number; style?: CSS }> = ({ text, typed, time = "14:32", width = 1100, style }) => (
  <div style={{ width, background: "#fff", borderRadius: 22, padding: "28px 34px 26px", boxShadow: "0 40px 100px rgba(0,0,0,0.5)", fontFamily: FONT, color: P.text, ...style }}>
    <div style={{ display: "flex", justifyContent: "space-between", fontSize: 26, fontWeight: 700 }}>Jot to Today <span style={{ color: P.text2, fontWeight: 400, fontSize: 24 }}>{time}</span></div>
    <div style={{ marginTop: 20, border: `2px solid ${P.accent}`, borderRadius: 14, padding: "18px 24px", minHeight: 120, fontSize: 30, lineHeight: 1.35, background: P.pageBg }}>
      {typed === undefined ? text : text.slice(0, typed)}{typed !== undefined && typed < text.length && <Caret />}
    </div>
    <div style={{ marginTop: 18, color: P.text2, fontSize: 22 }}>Return adds · ⇧Return new line · Esc cancels</div>
  </div>
);

// MARK: catch up, search, review
const Pill: React.FC<{ children: React.ReactNode; style?: CSS }> = ({ children, style }) => (
  <div style={{ display: "inline-flex", alignItems: "center", gap: px(8), background: "#EEF0EE", borderRadius: px(8), padding: `${px(7)}px ${px(12)}px`, fontSize: px(14), ...style }}>{children}<span style={{ color: P.text2, fontSize: px(10) }}>⌄</span></div>
);
export const CatchUpList: React.FC<{ days: { week: string; label: string; state: "missed" | "skipped" | "logged"; enter?: number }[]; count?: number; style?: CSS }> = ({ days, count, style }) => (
  <div style={{ padding: `${px(36)}px ${px(48)}px 0`, fontFamily: FONT, color: P.text, position: "relative", ...style }}>
    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
      <div style={{ fontSize: px(32), fontWeight: 700, letterSpacing: -0.5 }}>Catch up</div>
      <div style={{ background: P.accent, color: "#fff", fontWeight: 700, fontSize: px(14), padding: `${px(10)}px ${px(16)}px`, borderRadius: px(8) }}>Start catching up</div>
    </div>
    <div style={{ fontSize: px(14), color: P.text2, marginTop: px(8) }}>{count === 0 ? "You are up to date." : `You have ${count ?? days.length} unlogged day${(count ?? days.length) === 1 ? "" : "s"}.`}</div>
    <div style={{ display: "flex", justifyContent: "space-between", marginTop: px(18) }}><Pill>Last 30 days</Pill><Pill>Skip…</Pill></div>
    <div style={{ marginTop: px(20) }}>
      {days.map((d, i) => {
        const e = d.enter ?? 1; const showWeek = i === 0 || days[i - 1].week !== d.week;
        return (
          <div key={d.label} style={{ opacity: e, transform: `translateY(${(1 - e) * 14}px)` }}>
            {showWeek && <div style={{ display: "flex", justifyContent: "space-between", fontSize: px(14), fontWeight: 600, color: P.text2, margin: `${px(14)}px ${px(12)}px ${px(8)}px` }}>{d.week}<span>{d.state === "logged" ? "1 of 1 logged" : "0 of 1 logged"}</span></div>}
            <div style={{ display: "flex", alignItems: "center", gap: px(14), padding: `${px(8)}px ${px(12)}px`, height: px(52) }}>
              <StateIcon state={d.state === "missed" ? "missed" : d.state === "skipped" ? "skipped" : "logged"} />
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: px(15), fontWeight: 600, color: d.state === "skipped" ? P.text2 : P.text }}>{d.label}</div>
                <div style={{ fontSize: px(13), color: d.state === "logged" ? P.accent : P.text2 }}>{d.state === "missed" ? "Not started" : d.state === "skipped" ? "Skipped" : "Logged"}</div>
              </div>
              {d.state === "missed" && <>
                <div style={{ border: `1px solid ${P.border}`, background: "#fff", borderRadius: px(8), padding: `${px(8)}px ${px(16)}px`, fontSize: px(14), fontWeight: 600 }}>Write</div>
                <div style={{ color: P.text2, fontSize: px(14), fontWeight: 600, padding: `0 ${px(10)}px` }}>Skip</div>
              </>}
            </div>
          </div>
        );
      })}
    </div>
  </div>
);

export const SearchResults: React.FC<{ query: string; hits: { date: string; heading: string; snippet: string; match: string; enter?: number }[]; summary?: string; style?: CSS }> = ({ query, hits, summary, style }) => {
  const mark = (s: string, m: string) => {
    const i = s.toLowerCase().indexOf(m.toLowerCase());
    if (i < 0) return s;
    return <>{s.slice(0, i)}<span style={{ background: P.mark, fontWeight: 700, borderRadius: 3, padding: "0 2px" }}>{s.slice(i, i + m.length)}</span>{s.slice(i + m.length)}</>;
  };
  return (
    <div style={{ padding: `${px(36)}px ${px(48)}px 0`, fontFamily: FONT, color: P.text, ...style }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
        <div style={{ fontSize: px(32), fontWeight: 700, letterSpacing: -0.5 }}>Search: "{query}"<Caret /></div>
        <div style={{ fontSize: px(14), color: P.text2 }}>{summary}</div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: px(12), marginTop: px(14), fontSize: px(15) }}>Heading <Pill>All headings</Pill></div>
      <div style={{ height: 1, background: P.border, margin: `${px(18)}px 0 ${px(8)}px` }} />
      {hits.map((h, i) => {
        const e = h.enter ?? 1; const showDate = i === 0 || hits[i - 1].date !== h.date;
        return (
          <div key={i} style={{ opacity: e, transform: `translateY(${(1 - e) * 14}px)` }}>
            {showDate && <div style={{ fontSize: px(15), fontWeight: 700, margin: `${px(16)}px ${px(10)}px ${px(6)}px` }}>{h.date}</div>}
            <div style={{ display: "flex", gap: px(16), padding: `${px(8)}px ${px(10)}px`, fontSize: px(14), lineHeight: 1.45, borderRadius: px(8), background: i === 0 ? "#EEF4F1" : "transparent" }}>
              <div style={{ width: px(120), flexShrink: 0, color: P.text2, fontWeight: 600 }}>{h.heading}</div>
              <div>{mark(h.snippet, h.match)}</div>
            </div>
          </div>
        );
      })}
    </div>
  );
};

export const WeekTiles: React.FC<{ days: { label: string; n: number; words?: string; pop?: number; current?: boolean }[]; style?: CSS }> = ({ days, style }) => (
  <div style={{ display: "flex", gap: px(8), ...style }}>
    {days.map((d) => { const p = d.pop ?? 1; return (
      <div key={d.label} style={{ flex: 1, height: px(86), borderRadius: px(10), border: `${d.current ? 2 : 1}px solid ${d.current ? P.accent : P.border}`, background: "#fff", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center",
                                  opacity: p, transform: `scale(${0.6 + 0.4 * p})` }}>
        <div style={{ fontSize: px(12), color: P.text2 }}>{d.label}</div>
        <div style={{ fontSize: px(20), fontWeight: 700, color: d.words ? P.text : P.text3, display: "flex", alignItems: "center", gap: px(5) }}>{d.n}{d.words && <Dot state="logged" />}</div>
        <div style={{ fontSize: px(12), color: P.text2, height: px(16) }}>{d.words ?? ""}</div>
      </div>); })}
  </div>
);
export const ReviewCard: React.FC<{ date: string; heading: string; text: string; enter?: number; style?: CSS }> = ({ date, heading, text, enter = 1, style }) => (
  <div style={{ background: "#fff", border: `1px solid ${P.border}`, borderRadius: px(12), padding: `${px(16)}px ${px(20)}px`, opacity: enter, transform: `translateY(${(1 - enter) * 18}px)`, fontFamily: FONT, ...style }}>
    <div style={{ display: "flex", justifyContent: "space-between", fontSize: px(16), fontWeight: 700 }}>{date}<span style={{ color: P.accent, fontSize: px(14) }}>Open</span></div>
    <div style={{ fontSize: px(13), fontWeight: 600, color: P.text2, marginTop: px(10) }}>{heading}</div>
    <div style={{ fontSize: px(15), lineHeight: 1.45, marginTop: px(4) }}>{text}</div>
  </div>
);
export const ExportMenu: React.FC<{ items: string[]; enter?: number; style?: CSS }> = ({ items, enter = 1, style }) => (
  <div style={{ background: "rgba(255,255,255,0.98)", borderRadius: 12, padding: "8px 0", boxShadow: "0 16px 50px rgba(0,0,0,0.3), 0 0 0 1px rgba(0,0,0,0.08)", minWidth: 300, fontFamily: FONT,
                opacity: enter, transform: `scale(${0.9 + 0.1 * enter})`, transformOrigin: "100% 0", ...style }}>
    {items.map((it) => <div key={it} style={{ padding: "8px 18px", fontSize: 20, color: P.text }}>{it}</div>)}
  </div>
);

// MARK: chrome: cursor, notification, keycap, terminal, icon
export const Cursor: React.FC<{ x: number; y: number; pressed?: number; style?: CSS }> = ({ x, y, pressed = 0, style }) => (
  <svg width="34" height="40" viewBox="0 0 20 24" style={{ position: "absolute", left: x, top: y, transform: `scale(${1 - 0.12 * pressed})`, transformOrigin: "15% 10%", filter: "drop-shadow(0 2px 4px rgba(0,0,0,0.5))", ...style }}>
    <path d="M3 2 L3 22 L8.3 17.6 L11.6 25 L15.2 23.4 L12 16.3 L19 16.3 Z" fill="#000" stroke="#fff" strokeWidth="1.6" strokeLinejoin="round" />
  </svg>
);
export const Notification: React.FC<{ title: string; body: string; enter?: number; style?: CSS }> = ({ title, body, enter = 1, style }) => (
  <div style={{ width: 460, display: "flex", gap: 14, alignItems: "center", background: "rgba(242,245,243,0.97)", borderRadius: 18, padding: "14px 18px", boxShadow: "0 16px 50px rgba(0,0,0,0.35)",
                opacity: enter, transform: `translateX(${(1 - enter) * 80}px)`, fontFamily: FONT, ...style }}>
    <Img src={staticFile("icon-512.png")} style={{ width: 56, height: 56, borderRadius: 13 }} />
    <div><div style={{ fontSize: 20, fontWeight: 700, color: P.text }}>{title}</div><div style={{ fontSize: 18, color: P.text2, marginTop: 2 }}>{body}</div></div>
  </div>
);
export const Keycap: React.FC<{ label: string; pressed?: number; style?: CSS }> = ({ label, pressed = 0, style }) => (
  <div style={{ width: 104, height: 104, borderRadius: 20, background: "rgba(255,255,255,0.10)", border: "1px solid rgba(255,255,255,0.28)", color: "#fff", fontSize: 48, fontWeight: 600, fontFamily: FONT,
                display: "flex", alignItems: "center", justifyContent: "center", transform: `translateY(${6 * pressed}px)`, boxShadow: `0 ${8 - 6 * pressed}px 0 rgba(0,0,0,0.4)`, ...style }}>{label}</div>
);
export const Terminal: React.FC<{ title?: string; lines: { prompt?: boolean; text: string; typed?: number; dim?: boolean }[]; width?: number; style?: CSS }> = ({ title, lines, width = 1500, style }) => (
  <div style={{ width, background: "#07201B", borderRadius: 20, border: "1px solid rgba(255,255,255,0.12)", boxShadow: "0 24px 60px rgba(0,0,0,0.4)", padding: "18px 34px 30px", fontFamily: MONO, fontSize: 26, color: "#E8F3EE", lineHeight: 1.6, ...style }}>
    <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14, position: "relative" }}>
      {["#FF5F57", "#FEBC2E", "#28C840"].map((c) => <div key={c} style={{ width: 16, height: 16, borderRadius: 8, background: c }} />)}
      {title && <div style={{ position: "absolute", left: 0, right: 0, textAlign: "center", color: "rgba(232,243,238,0.5)", fontSize: 20, fontFamily: FONT }}>{title}</div>}
    </div>
    {lines.map((l, i) => {
      const t = l.typed === undefined ? l.text : l.text.slice(0, l.typed);
      const caret = l.typed !== undefined && l.typed < l.text.length;
      return <div key={i} style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", color: l.dim ? "rgba(232,243,238,0.6)" : l.text.startsWith("✓") || l.text.startsWith("Installed") ? P.mint : "#E8F3EE" }}>
        {l.prompt && <span style={{ color: P.mint }}>$ </span>}{t}{caret && <span style={{ color: P.mint }}>▍</span>}
      </div>;
    })}
  </div>
);
export const SunIcon: React.FC<{ size: number; rise?: number; lines?: number; style?: CSS }> = ({ size, rise = 1, lines = 1, style }) => {
  const r = 0.24 * size;                       // sun radius
  const horizon = 0.57 * size;
  const cy = horizon + r * 1.3 * (1 - rise);   // the sun comes up from below the horizon
  const seg = (i: number) => Math.max(0, Math.min(1, lines * 3 - i));
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={style}>
      <defs>
        <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#0B2B26" /><stop offset="0.42" stopColor="#1B4A3E" /><stop offset="0.78" stopColor="#2E8A74" /><stop offset="1" stopColor="#F6C48E" /></linearGradient>
        <linearGradient id="sun" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#FFE2B3" /><stop offset="1" stopColor="#F3A96A" /></linearGradient>
        <radialGradient id="glow"><stop offset="0" stopColor="#F6C48E" stopOpacity={0.55 * rise} /><stop offset="1" stopColor="#F6C48E" stopOpacity="0" /></radialGradient>
        <clipPath id="tile"><rect x="0" y="0" width={size} height={size} rx={size * 0.225} /></clipPath>
        <clipPath id="sky-clip"><rect x="0" y="0" width={size} height={horizon} /></clipPath>
      </defs>
      <g clipPath="url(#tile)">
        <rect width={size} height={horizon} fill="url(#sky)" />
        <circle cx={size / 2} cy={cy} r={r * 2.2} fill="url(#glow)" clipPath="url(#sky-clip)" />
        <circle cx={size / 2} cy={cy} r={r} fill="url(#sun)" clipPath="url(#sky-clip)" />
        <rect y={horizon} width={size} height={size - horizon} fill="#0E3A32" />
        {[0.44, 0.34, 0.24].map((w, i) => (
          <rect key={i} x={size / 2 - (w * size * seg(i)) / 2} y={horizon + size * (0.075 + 0.085 * i)} width={w * size * seg(i)} height={size * 0.045} rx={size * 0.0225} fill="#F2C78F" opacity={[0.95, 0.62, 0.34][i]} />
        ))}
      </g>
    </svg>
  );
};
