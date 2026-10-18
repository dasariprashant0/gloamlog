// Global shortcuts are stored the way macOS (Carbon) wants them: a virtual key code and a modifier mask.
// This maps what the browser reports (KeyboardEvent.code) to those, and back to the symbols shown in the window.

const KEYS: Record<string, [code: number, name: string]> = {
    KeyA: [0, 'A'], KeyS: [1, 'S'], KeyD: [2, 'D'], KeyF: [3, 'F'], KeyH: [4, 'H'], KeyG: [5, 'G'], KeyZ: [6, 'Z'], KeyX: [7, 'X'],
    KeyC: [8, 'C'], KeyV: [9, 'V'], KeyB: [11, 'B'], KeyQ: [12, 'Q'], KeyW: [13, 'W'], KeyE: [14, 'E'], KeyR: [15, 'R'], KeyY: [16, 'Y'],
    KeyT: [17, 'T'], Digit1: [18, '1'], Digit2: [19, '2'], Digit3: [20, '3'], Digit4: [21, '4'], Digit6: [22, '6'], Digit5: [23, '5'],
    Equal: [24, '='], Digit9: [25, '9'], Digit7: [26, '7'], Minus: [27, '-'], Digit8: [28, '8'], Digit0: [29, '0'],
    BracketRight: [30, ']'], KeyO: [31, 'O'], KeyU: [32, 'U'], BracketLeft: [33, '['], KeyI: [34, 'I'], KeyP: [35, 'P'],
    Enter: [36, '↩'], KeyL: [37, 'L'], KeyJ: [38, 'J'], Quote: [39, "'"], KeyK: [40, 'K'], Semicolon: [41, ';'], Backslash: [42, '\\'],
    Comma: [43, ','], Slash: [44, '/'], KeyN: [45, 'N'], KeyM: [46, 'M'], Period: [47, '.'], Tab: [48, '⇥'], Space: [49, 'Space'],
    Backquote: [50, '`'], Backspace: [51, '⌫'],
    F1: [122, 'F1'], F2: [120, 'F2'], F3: [99, 'F3'], F4: [118, 'F4'], F5: [96, 'F5'], F6: [97, 'F6'], F7: [98, 'F7'], F8: [100, 'F8'],
    F9: [101, 'F9'], F10: [109, 'F10'], F11: [103, 'F11'], F12: [111, 'F12'],
    ArrowLeft: [123, '←'], ArrowRight: [124, '→'], ArrowDown: [125, '↓'], ArrowUp: [126, '↑'],
};

const NAME_OF = new Map(Object.values(KEYS).map(([code, name]) => [code, name]));

const CONTROL = 0x1000, OPTION = 0x0800, SHIFT = 0x0200, COMMAND = 0x0100;

export type HotKey = {keyCode: number; carbonModifiers: number};

/** The shortcut a key press makes, or null when the key has no macOS key code here. */
export function hotKeyOf(e: KeyboardEvent): HotKey | null {
    const key = KEYS[e.code];
    if (!key) return null;
    const mods = (e.ctrlKey ? CONTROL : 0) | (e.altKey ? OPTION : 0) | (e.shiftKey ? SHIFT : 0) | (e.metaKey ? COMMAND : 0);
    return {keyCode: key[0], carbonModifiers: mods};
}

/** Needs ⌃ or ⌘, so it can't clash with typing. */
export const hotKeyAllowed = (k: HotKey) => (k.carbonModifiers & (CONTROL | COMMAND)) !== 0;

/** "⌃⌥J" */
export function hotKeyLabel(k: HotKey): string {
    const m = k.carbonModifiers;
    return (m & CONTROL ? '⌃' : '') + (m & OPTION ? '⌥' : '') + (m & SHIFT ? '⇧' : '') + (m & COMMAND ? '⌘' : '') + (NAME_OF.get(k.keyCode) ?? `Key ${k.keyCode}`);
}

export const DEFAULT_HOT_KEY: HotKey = {keyCode: 38, carbonModifiers: CONTROL | OPTION};
