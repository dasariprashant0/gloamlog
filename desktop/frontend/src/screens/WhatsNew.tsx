// "What's new in 0.7": shown once to people who already use Gloamlog. Port of WhatsNewSheet in app/UI/MainView.swift
// (docs/port/PARITY.md 3.N). Closing it, either way, remembers that it was shown (Notes.MarkWhatsNewSeen).
// The text belongs to one release: when it changes, bump whatsNewRelease in desktop/notes_onboarding.go.
import {useEffect, useState} from 'react';
import {Button, Dialog} from '@personal/ui/react';
import {Notes} from '../api';
import {hotKeyLabel} from './settings/keys';

type Props = {
    open: boolean;
    onClose: () => void;
    /** "Take a look at Settings": the sheet closes itself first. */
    onSettings: () => void;
};

export function WhatsNew({open, onClose, onSettings}: Props) {
    const [jot, setJot] = useState('Jot a note from the menu bar. It lands under Jots on today\'s page.');

    // The Jot line names the shortcut the person really has.
    useEffect(() => {
        if (!open) return;
        let live = true;
        Notes.Settings().then(s => {
            const k = s.capture.hotKey;
            const heading = s.capture.jotsHeading || 'Jots';
            if (live) setJot(k ? `Jot a note from any app with ${hotKeyLabel(k)}. It lands under ${heading} on today's page.`
                : `Jot a note from the menu bar. It lands under ${heading} on today's page.`);
        }).catch(() => undefined);
        return () => { live = false; };
    }, [open]);

    const lines = [
        'A new look, and the app is rebuilt from the ground up. Your notes are the same plain markdown files in the same folder.',
        'Open any day from the calendar, or press ⇧⌘T and type a date.',
        "Catch up lists the days you haven't logged. Skip the ones you didn't work.",
        jot,
        'Review any week, month, year or range, then copy it, save a PDF or export it with its images (⇧⌘E).',
        'Reminders still arrive when Gloamlog is closed (Settings > Reminders).',
        'Settings (⌘,) holds your reminders, page template, storage and shortcuts.',
    ];

    // Remembering is best effort: if the file can't be written the sheet may show once more, which loses nothing.
    const done = () => { Notes.MarkWhatsNewSeen().catch(() => undefined); onClose(); };

    return (
        <Dialog open={open} onClose={done} title="What's new in 0.7" actions={<>
            <Button variant="line" onClick={() => { done(); onSettings(); }}>Take a look at Settings</Button>
            <Button onClick={done}>Done</Button>
        </>}>
            <ul className="list">
                {lines.map(l => <li key={l} className="row"><span className="row-body">{l}</span></li>)}
            </ul>
        </Dialog>
    );
}
