// Go -> React events (app.Event.Emit in desktop/*.go). In the browser mock there is no Go side, so subscribing does nothing.
import {Events} from '@wailsio/runtime';

export function on<T = unknown>(name: string, handler: (data: T) => void): () => void {
    try {
        return Events.On(name, ev => handler(ev.data as T));
    } catch {
        return () => {};
    }
}
