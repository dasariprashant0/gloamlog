import {Brand, Empty, NavItem, Shell, ThemeSwitch, useTheme} from '@personal/ui/react';

export function App() {
    const [theme, setTheme] = useTheme();
    const side = <>
        <Brand name="Gloamlog"/>
        <nav className="nav"><NavItem current>Today</NavItem><NavItem>Catch up</NavItem><NavItem>Review</NavItem></nav>
        <div className="side-foot"><ThemeSwitch theme={theme} onChange={setTheme}/></div>
    </>;
    return <Shell side={side}><Empty title={['Gloamlog', 'on Wails']}>The port is wiring up.</Empty></Shell>;
}
