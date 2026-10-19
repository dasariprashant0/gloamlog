import {Button, Display} from '@personal/ui/react';
import {Windows} from '../../api';
import {Panel, Soon, type PaneProps} from './shared';

export function AboutPane({info}: PaneProps) {
    const version = info.version && info.version !== 'dev' ? `Version ${info.version}` : 'Development build';
    return (
        <Panel>
            <div className="stack" style={{justifyItems: 'start'}}>
                <Display lines={['Gloamlog']} size="sm"/>
                <p className="muted">{version}</p>
                <p className="lede">A calm end-of-day log. One plain markdown file per day, on your Mac. No account, no telemetry; the only network request is the update check you ask for.</p>
                {/* The update dialog lives in the main window: this brings it forward and opens it there. */}
                <Button variant="line" size="sm" onClick={() => void Windows.CheckForUpdates()}>Check for updates…</Button>
                {/* Also in the main window: the setup sheet starts from the current settings there. */}
                <Button variant="line" size="sm" onClick={() => void Windows.RunSetup()}>Run setup again…</Button>
                <div className="row-gap">
                    <Button variant="line" size="sm" disabled title="Open the licences of the bundled editor">Third-party licences</Button>
                    <Soon/>
                </div>
            </div>
            <p className="small muted">Released under the MIT licence.</p>
        </Panel>
    );
}
