// Go -> React events (app.Event.Emit in desktop/*.go). In the browser mock there is no Go side, so subscribing does nothing.
import {Events} from '@wailsio/runtime';

export function on<T = unknown>(name: string, handler: (data: T) => void): () => void {
    try {
        return Events.On(name, ev => handler(ev.data as T));
    } catch { // no Wails runtime (browser mock): nothing will ever be emitted
        return () => {};
    }
}

// React -> Go (app.Event.On in desktop/*.go). The browser mock has no Go side: nothing to tell.
export function emit(name: string, data?: unknown): void {
    try {
        void Events.Emit(name, data);
    } catch { /* no Wails runtime */ }
}
