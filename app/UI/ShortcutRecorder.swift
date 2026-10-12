// ShortcutRecorder.swift - how a global shortcut is written, spoken and recorded (M2, task U6): the key names, the
// recorder control for Settings > Shortcuts, and the plain key-down monitor behind it.
// Recording uses a LOCAL event monitor (only while Gloamlog's own Settings window is key), so it needs no permission, and it
// swallows the keys it records, so ⌘W or ⌘Q pressed while recording never reaches a menu.
import SwiftUI
import AppKit
import Carbon.HIToolbox

// MARK: - names

extension HotKeySpec {
    var hasControl: Bool { carbonModifiers & UInt32(controlKey) != 0 }
    var hasOption: Bool { carbonModifiers & UInt32(optionKey) != 0 }
    var hasShift: Bool { carbonModifiers & UInt32(shiftKey) != 0 }
    var hasCommand: Bool { carbonModifiers & UInt32(cmdKey) != 0 }

    /// "⌃⌥J": modifiers in the order macOS prints them (control, option, shift, command), then the key.
    var display: String { modifierGlyphs + KeyNames.name(for: keyCode) }
    var modifierGlyphs: String { (hasControl ? "⌃" : "") + (hasOption ? "⌥" : "") + (hasShift ? "⇧" : "") + (hasCommand ? "⌘" : "") }
    /// "Control Option J", for VoiceOver.
    var spoken: String {
        var w = [String]()
        if hasControl { w.append("Control") }; if hasOption { w.append("Option") }
        if hasShift { w.append("Shift") }; if hasCommand { w.append("Command") }
        w.append(KeyNames.spokenName(for: keyCode))
        return w.joined(separator: " ")
    }

    /// The combination as a key-down event describes it; nil for anything else.
    init?(event: NSEvent) {
        guard event.type == .keyDown else { return nil }
        let f = event.modifierFlags
        var m: UInt32 = 0
        if f.contains(.control) { m |= UInt32(controlKey) }
        if f.contains(.option) { m |= UInt32(optionKey) }
        if f.contains(.shift) { m |= UInt32(shiftKey) }
        if f.contains(.command) { m |= UInt32(cmdKey) }
        self.init(keyCode: UInt32(event.keyCode), carbonModifiers: m)
    }

    /// The same shortcut for a SwiftUI menu item, so the Page menu can show it beside "Jot…". nil when SwiftUI has no
    /// equivalent for the key (function keys), in which case the menu item simply shows no shortcut.
    var menuShortcut: (key: KeyEquivalent, modifiers: SwiftUI.EventModifiers)? {
        var mods = SwiftUI.EventModifiers()
        if hasControl { mods.insert(.control) }; if hasOption { mods.insert(.option) }
        if hasShift { mods.insert(.shift) }; if hasCommand { mods.insert(.command) }
        switch keyCode {
        case 36: return (.return, mods)
        case 48: return (.tab, mods)
        case 49: return (.space, mods)
        case 51: return (.delete, mods)
        case 53: return (.escape, mods)
        case 123: return (.leftArrow, mods)
        case 124: return (.rightArrow, mods)
        case 125: return (.downArrow, mods)
        case 126: return (.upArrow, mods)
        default:
            guard KeyNames.special[keyCode] == nil, let s = KeyNames.translate(keyCode)?.lowercased(), s.count == 1, let c = s.first else { return nil }
            return (KeyEquivalent(c), mods)
        }
    }
}

enum KeyNames {
    /// Keys that have a symbol or a name instead of a letter (ANSI key codes).
    static let special: [UInt32: String] = [
        36: "↩", 48: "⇥", 49: "Space", 51: "⌫", 53: "⎋", 71: "⌧", 76: "⌅", 114: "Help", 115: "↖", 116: "⇞", 117: "⌦", 119: "↘", 121: "⇟",
        123: "←", 124: "→", 125: "↓", 126: "↑",
        122: "F1", 120: "F2", 99: "F3", 118: "F4", 96: "F5", 97: "F6", 98: "F7", 100: "F8", 101: "F9", 109: "F10", 103: "F11", 111: "F12",
        105: "F13", 107: "F14", 113: "F15", 106: "F16", 64: "F17", 79: "F18", 80: "F19", 90: "F20",
    ]
    private static let spokenSpecial: [UInt32: String] = [
        36: "Return", 48: "Tab", 51: "Delete", 53: "Escape", 71: "Clear", 76: "Enter", 115: "Home", 116: "Page Up", 117: "Forward Delete",
        119: "End", 121: "Page Down", 123: "Left Arrow", 124: "Right Arrow", 125: "Down Arrow", 126: "Up Arrow",
    ]

    static func name(for keyCode: UInt32) -> String {
        if let s = special[keyCode] { return s }
        return translate(keyCode)?.uppercased() ?? "Key \(keyCode)"
    }
    static func spokenName(for keyCode: UInt32) -> String {
        if let s = spokenSpecial[keyCode] { return s }
        return name(for: keyCode)
    }

    /// The character a key makes on the current Latin keyboard layout (so ⌃⌥J reads right on Dvorak or AZERTY too).
    static func translate(_ keyCode: UInt32) -> String? {
        guard let source = TISCopyCurrentASCIICapableKeyboardInputSource()?.takeRetainedValue(),
              let raw = TISGetInputSourceProperty(source, kTISPropertyUnicodeKeyLayoutData) else { return nil }
        let data = Unmanaged<CFData>.fromOpaque(raw).takeUnretainedValue()
        guard let bytes = CFDataGetBytePtr(data) else { return nil }
        let layout = UnsafeRawPointer(bytes).assumingMemoryBound(to: UCKeyboardLayout.self)
        var dead: UInt32 = 0
        var length = 0
        var chars = [UniChar](repeating: 0, count: 4)
        let status = UCKeyTranslate(layout, UInt16(keyCode), UInt16(kUCKeyActionDisplay), 0, UInt32(LMGetKbdType()),
                                    OptionBits(kUCKeyTranslateNoDeadKeysBit), &dead, chars.count, &length, &chars)
        guard status == noErr, length > 0 else { return nil }
        let s = String(utf16CodeUnits: chars, count: length)
        return s.isEmpty || s.unicodeScalars.contains(where: { $0.value < 32 }) ? nil : s
    }
}

// MARK: - recording

/// One recording session: while `recording`, the next key press with control or command is the shortcut (Esc cancels).
/// `handle` is the whole behaviour, so a check can feed it key events without a physical keyboard.
final class ShortcutRecording: ObservableObject {
    @Published private(set) var recording = false
    @Published var hint: String?
    var onShortcut: ((HotKeySpec) -> Void)?
    private var monitor: Any?
    private var resign: NSObjectProtocol?

    deinit { stop() }

    func start() {
        guard !recording else { return }
        recording = true; hint = nil
        monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] e in self?.handle(e) ?? e }
        // Clicking away from the window ends the recording: nothing may stay armed in the background.
        resign = NotificationCenter.default.addObserver(forName: NSWindow.didResignKeyNotification, object: nil, queue: .main) { [weak self] _ in self?.stop() }
        UIAnnounce.say("Press the new shortcut. Escape cancels.")
    }

    func stop() {
        if let m = monitor { NSEvent.removeMonitor(m) }
        if let r = resign { NotificationCenter.default.removeObserver(r) }
        monitor = nil; resign = nil
        if recording { recording = false }
    }

    func toggle() { recording ? stop() : start() }

    /// Returns nil when the event was used up by the recording, the event itself when it is none of our business.
    func handle(_ event: NSEvent) -> NSEvent? {
        guard recording, event.type == .keyDown else { return event }
        if event.isARepeat { return nil }
        let plain = event.modifierFlags.intersection([.control, .option, .shift, .command]).isEmpty
        if event.keyCode == 53 && plain { stop(); return nil }                      // Esc cancels, changes nothing
        guard let spec = HotKeySpec(event: event) else { return nil }
        guard HotKeyCenter.isAllowed(spec) else {
            hint = "Include ⌃ or ⌘ so it can't clash with typing."
            return nil
        }
        stop()
        onShortcut?(spec)
        return nil
    }
}

/// The recorder row's control: a button that shows the shortcut and turns into "Press the new shortcut" while recording.
struct ShortcutRecorder: View {
    @ObservedObject var model: AppModel
    @ObservedObject var recording: ShortcutRecording

    private var spec: HotKeySpec? { model.settings.capture.hotKey }
    private var label: String {
        if recording.recording { return "Press the new shortcut" }
        return spec?.display ?? "Off"
    }

    var body: some View {
        Button { recording.toggle() } label: {
            Text(label)
                .font(recording.recording ? Theme.font(13) : .system(size: 14, weight: .medium, design: .rounded))
                .foregroundColor(recording.recording ? Theme.accentText : (spec == nil ? Theme.textSecondary : Theme.textPrimary))
                .frame(minWidth: 150, minHeight: 28)
                .padding(.horizontal, Theme.s2)
                .background(Theme.rect().fill(recording.recording ? Theme.accentTint : Theme.surface))
                .overlay(Theme.rect().stroke(recording.recording ? Theme.accent : Theme.borderStrong, lineWidth: recording.recording ? 1.5 : 1))
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help(recording.recording ? "Press the new shortcut. Escape cancels." : "Click, then press the shortcut you want")
        .accessibilityLabel("Jot shortcut")
        .accessibilityValue(recording.recording ? "Recording" : (spec?.spoken ?? "Off"))
        .accessibilityHint("Press the new shortcut. Escape cancels.")
    }
}
