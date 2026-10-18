// Sidebar: search, Today / Catch up / Review, the month, the newest pages, and the streak + settings footer.
import type {ReactNode, RefObject} from 'react';
import {Badge, Brand, NavItem, ThemeSwitch, useTheme} from '@personal/ui/react';
import {Windows, shortDay, type Overview} from '../api';
import {MonthCalendar} from './MonthCalendar';

export type Route = {kind: 'day'; day: string} | {kind: 'catchup'} | {kind: 'review'} | {kind: 'search'};

const TODAY_STATE: Record<string, string> = {logged: 'Logged', skipped: 'Skipped', off: 'Not a workday', partial: 'Started'};

export function Sidebar({overview, route, query, searchRef, version, onQuery, onGo, foot}: {
    overview: Overview | null; route: Route; query: string; searchRef: RefObject<HTMLInputElement | null>; version: number;
    onQuery: (q: string) => void; onGo: (r: Route) => void; foot?: ReactNode; // the sync chip
}) {
    const [theme, setTheme] = useTheme();
    const today = overview?.today ?? '';
    const openDay = route.kind === 'day' ? route.day : null;
    const n = overview?.catchUp?.length ?? 0;
    const streak = overview?.streak;

    return <>
        <Brand name="Gloamlog"/>
        <label className="sr-only" htmlFor="search">Search your logs</label>
        <input id="search" ref={searchRef} className="input input-sm search" type="search" placeholder="Search your logs" value={query}
               onChange={e => onQuery(e.target.value)} onKeyDown={e => e.key === 'Escape' && onQuery('')}/>
        <nav className="nav" aria-label="Pages">
            <NavItem current={openDay === today} meta={TODAY_STATE[overview?.todayStatus ?? ''] ?? 'Not logged'} onClick={() => onGo({kind: 'day', day: today})}>Today</NavItem>
            <NavItem current={route.kind === 'catchup'} meta={n > 0 ? <Badge>{n > 99 ? '99+' : n}</Badge> : undefined} onClick={() => onGo({kind: 'catchup'})}>Catch up</NavItem>
            <NavItem current={route.kind === 'review'} onClick={() => onGo({kind: 'review'})}>Review</NavItem>
        </nav>
        {today && <MonthCalendar selected={openDay} today={today} version={version} onPick={day => onGo({kind: 'day', day})}/>}
        <p className="label side-label">Recent</p>
        {overview && overview.recent?.length === 0 && <p className="small muted side-note">Pages you write appear here.</p>}
        <nav className="nav" aria-label="Recent pages">
            {overview?.recent?.map(d => <NavItem key={d} current={openDay === d} onClick={() => onGo({kind: 'day', day: d})}>{shortDay(d)}</NavItem>)}
        </nav>
        <div className="side-foot">
            {foot}
            <p className="small">{streak && streak.current > 0 ? <>{streak.current}-day streak <span className="muted">· best {streak.best}</span></> : <span className="muted">No streak yet</span>}</p>
            <button className="btn btn-line btn-sm" onClick={() => Windows.ShowSettings()} title="⌘,">Settings</button>
            <ThemeSwitch theme={theme} onChange={setTheme}/>
        </div>
    </>;
}
