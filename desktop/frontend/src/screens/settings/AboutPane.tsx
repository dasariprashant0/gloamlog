import {Button, Display} from '@personal/ui/react';
import {Panel, Soon, type PaneProps} from './shared';

export function AboutPane({info}: PaneProps) {
    const version = info.version && info.version !== 'dev' ? `Version ${info.version}` : 'Development build';
    return (
        <Panel>
            <div className="stack" style={{justifyItems: 'start'}}>
                <Display lines={['Gloamlog']} size="sm"/>
                <p className="muted">{version}</p>
                <p className="lede">A calm end-of-day log. One plain markdown file per day, on your Mac. No network requests, no account, no telemetry.</p>
                <div className="row-gap">
                    <Button variant="line" size="sm" disabled title="Walk through the setup steps again">Run setup again…</Button>
                    <Button variant="line" size="sm" disabled title="Open the licences of the bundled editor">Third-party licences</Button>
                    <Soon/>
                </div>
            </div>
            <p className="small muted">Released under the MIT licence.</p>
        </Panel>
    );
}
