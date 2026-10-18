import {useState} from 'react';
import {Banner, Button, Setting, Switch, ThemeSwitch, type Theme} from '@personal/ui/react';
import {Notes} from '../../api';
import {Block, DAY_NAMES, errText, Panel, ResetBar, systemWeekStart, type PaneProps} from './shared';

export function GeneralPane({s, set, info, reload, theme, setTheme}: PaneProps & {theme: Theme; setTheme: (t: Theme) => void}) {
    const [loginMessage, setLoginMessage] = useState('');
    const needsApproval = /approv/i.test(loginMessage);

    async function setLogin(on: boolean) {
        setLoginMessage('');
        try {
            await Notes.SetLoginItem(on);
        } catch (e) {
            const text = errText(e);
            setLoginMessage(/approv/i.test(text) ? 'Approve Gloamlog in System Settings > General > Login Items.' : `Couldn't change the login item: ${text}`);
        }
        reload();
    }

    return (
        <Panel>
            <Block title="Display">
                <Setting title="Theme" hint="Warm is light paper, dark is ink, midnight is darker still.">
                    <ThemeSwitch theme={theme} onChange={setTheme}/>
                </Setting>
                <Setting title="Week starts on" hint={`Used by the calendar and the week review. System follows your Mac, which starts the week on ${DAY_NAMES[systemWeekStart() - 1]}.`}>
                    <select className="select" aria-label="Week starts on" style={{width: 'auto'}} value={s.weekStart ?? 0}
                        onChange={e => set({weekStart: Number(e.target.value)})}>
                        <option value={0}>System</option>
                        <option value={2}>Monday</option>
                        <option value={1}>Sunday</option>
                        <option value={7}>Saturday</option>
                    </select>
                </Setting>
            </Block>
            <Block title="Startup">
                <Setting title="Open Gloamlog at login" hint="Starts Gloamlog hidden, so your reminder still works after a restart.">
                    <Switch checked={info.loginEnabled} onChange={setLogin} label={<span className="muted small">{info.loginEnabled ? 'On' : 'Off'}</span>}/>
                </Setting>
                {loginMessage && (
                    <Banner tone="warning" action={needsApproval ? <Button size="sm" variant="line" onClick={() => Notes.OpenLoginItems()}>Open Login Items</Button> : undefined}>
                        {loginMessage}
                    </Banner>
                )}
                <Setting title="Show in the menu bar" hint="The menu bar item shows today's status and opens Settings. Reminders work without it.">
                    <Switch checked={s.showMenuBar} onChange={v => set({showMenuBar: v})} label={<span className="muted small">{s.showMenuBar ? 'On' : 'Off'}</span>}/>
                </Setting>
            </Block>
            <ResetBar pane="General" summary="the week start and the menu bar item" onReset={() => { set({weekStart: 0, showMenuBar: true}); if (info.loginEnabled) setLogin(false); }}/>
        </Panel>
    );
}
