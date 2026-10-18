// The one Milkdown editor of the main window. It mounts once and stays mounted; the day page swaps documents in it with
// editor.setMarkdown (see docs/EDITOR_CONTRACT.md). Bridge messages from the editor go to the handler given in onMessage.
import {useEffect, useRef} from 'react';
import {editor as engine, mountEditor, setHost} from './engine.js';

export type EditorMessage =
    | {type: 'ready'}
    | {type: 'change'; markdown: string}
    | {type: 'uploadImage'; id: string; name: string; mime: string; dataBase64: string}
    | {type: 'openLink'; url: string}
    | {type: 'rawFallback'; reason: string}
    | {type: 'appendResult'; id: string; result: 'appended' | 'alreadyPresent' | 'stale'; markdown: string | null}
    | {type: 'log'; level: string; message: string};

/** The editor's API (window.DailyLogEditor). Calls made before it is ready are queued by the engine. */
export const editor = engine as unknown as {
    setMarkdown(md: string, opts?: {cursor?: 'start' | 'end' | 'keep'; focus?: boolean}): void;
    getMarkdown(): string;
    isRawFallback(): boolean;
    setReadOnly(value: boolean): void;
    setPlaceholder(text: string): void;
    focus(): void;
    insertMarkdown(md: string, where?: 'start' | 'end'): void;
    appendToSection(callId: string, token: string, heading: string, markdown: string, opts?: {unlessPresent?: boolean}): void;
    _resolveUpload(id: string, path: string | null, err: string | null): void;
};

export function Editor({onMessage, label}: {onMessage: (m: EditorMessage) => void; label: string}) {
    const root = useRef<HTMLDivElement>(null);
    const handler = useRef(onMessage);
    handler.current = onMessage;
    useEffect(() => {
        setHost((m: EditorMessage) => handler.current(m));
        if (root.current) mountEditor(root.current);
    }, []);
    return <div ref={root} className="page-editor" aria-label={label}/>;
}
