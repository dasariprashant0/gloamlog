// HotKey.swift - the global Jot shortcut (M2, task U1). Carbon RegisterEventHotKey: it needs no Accessibility or Input
// Monitoring permission because it never observes keystrokes (docs/v1/ARCHITECTURE.md 5.D, DESIGN_V4 section 14).
//
// What the system really reports (probed on macOS 26/27 with two processes, see the M2 report):
//   * the same combination registered twice inside ONE process answers eventHotKeyExistsErr (-9878);
//   * a combination another PROCESS holds is NOT refused (both registrations return noErr), so "taken by another app" cannot be
//     read from the return code. What can be read is the list of macOS's own shortcuts (CopySymbolicHotKeys): `isSystemShortcut`.
// Both are surfaced as errors in Settings > Shortcuts; everything else about the registration is best effort and honest.
import AppKit
import Carbon.HIToolbox

final class HotKeyCenter {
    /// The app's one global shortcut. Harness code makes its own instances so it never touches the real combination.
    static let shared = HotKeyCenter()

    /// `eventHotKeyExistsErr`: the same combination is already registered (inside this process).
    static let existsStatus = OSStatus(eventHotKeyExistsErr)
    /// `eventHotKeyInvalidErr`: a combination the app refuses (see `isAllowed`).
    static let invalidStatus = OSStatus(eventHotKeyInvalidErr)

    /// Called on the main queue when the registered shortcut is pressed.
    var onPressed: (() -> Void)?
    /// The combination this center currently holds (nil: off, or a registration that failed).
    private(set) var registered: HotKeySpec?

    private var ref: EventHotKeyRef?
    private let id: UInt32

    private static let signature: OSType = 0x474C4D4A            // 'GLMJ'
    private static var nextID: UInt32 = 1
    private struct Slot { weak var center: HotKeyCenter? }
    private static var slots = [UInt32: Slot]()
    private static var handlerInstalled = false

    init() { id = HotKeyCenter.nextID; HotKeyCenter.nextID += 1 }
    deinit { if let r = ref { UnregisterEventHotKey(r) }; HotKeyCenter.slots[id] = nil }

    // MARK: rules

    /// A global shortcut needs control or command: option and option-shift alone clash with typing (and newer macOS releases
    /// restrict them), so they are refused. At least one modifier is always required.
    static func isAllowed(_ spec: HotKeySpec) -> Bool {
        spec.carbonModifiers & UInt32(controlKey | cmdKey) != 0
    }

    /// True when macOS itself owns the combination (Spotlight, Mission Control, input sources ...). Such a shortcut would be
    /// "registered" but the system handles it first, so it would never reach the app.
    static func isSystemShortcut(_ spec: HotKeySpec) -> Bool {
        var list: Unmanaged<CFArray>?
        guard CopySymbolicHotKeys(&list) == noErr, let items = list?.takeRetainedValue() as? [[String: Any]] else { return false }
        let mask = UInt32(controlKey | optionKey | shiftKey | cmdKey)
        let want = spec.carbonModifiers & mask
        for d in items {
            guard let code = d[kHISymbolicHotKeyCode as String] as? Int, UInt32(code) == spec.keyCode,
                  let mods = d[kHISymbolicHotKeyModifiers as String] as? Int, UInt32(truncatingIfNeeded: mods) & mask == want else { continue }
            let on = (d[kHISymbolicHotKeyEnabled as String] as? Bool) ?? ((d[kHISymbolicHotKeyEnabled as String] as? Int ?? 0) != 0)
            if on { return true }
        }
        return false
    }

    // MARK: registration

    /// Registers `spec`. noErr, or an OSStatus from Carbon (-9878 = already registered in this process). A previous
    /// registration of this center stays active until the new one has succeeded, so a refused combination never leaves the
    /// user without the shortcut they had.
    @discardableResult
    func register(_ spec: HotKeySpec) -> OSStatus {
        if registered == spec { return noErr }
        Self.installHandlerIfNeeded()
        var fresh: EventHotKeyRef?
        let status = RegisterEventHotKey(spec.keyCode, spec.carbonModifiers, EventHotKeyID(signature: Self.signature, id: id),
                                         GetApplicationEventTarget(), 0, &fresh)
        guard status == noErr, let new = fresh else { return status == noErr ? OSStatus(eventInternalErr) : status }
        if let old = ref { UnregisterEventHotKey(old) }
        ref = new; registered = spec
        Self.slots[id] = Slot(center: self)
        return noErr
    }

    /// Removes the shortcut (the user turned it off). Safe to call when nothing is registered.
    func unregister() {
        if let r = ref { UnregisterEventHotKey(r) }
        ref = nil; registered = nil
        Self.slots[id] = nil
    }

    // MARK: delivery

    private static func installHandlerIfNeeded() {
        guard !handlerInstalled else { return }
        var spec = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        // A C callback cannot capture: it reads the hot key's id and hops to the main queue, where the center is looked up.
        let handler: EventHandlerUPP = { _, event, _ in
            guard let event = event else { return OSStatus(eventNotHandledErr) }
            var hk = EventHotKeyID()
            let st = GetEventParameter(event, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID), nil,
                                       MemoryLayout<EventHotKeyID>.size, nil, &hk)
            guard st == noErr, hk.signature == HotKeyCenter.signature else { return OSStatus(eventNotHandledErr) }
            let id = hk.id
            DispatchQueue.main.async { HotKeyCenter.slots[id]?.center?.onPressed?() }
            return noErr
        }
        handlerInstalled = InstallEventHandler(GetApplicationEventTarget(), handler, 1, &spec, nil, nil) == noErr
    }

    /// For the check harness (a physical global key cannot be pressed there): sends this center's hot key event through the
    /// same Carbon dispatch a real key press uses, so the handler, the main-queue hop and `onPressed` all run for real.
    /// Returns false when nothing is registered or the event could not be sent.
    @discardableResult
    func deliverPressForTesting() -> Bool {
        guard registered != nil else { return false }
        var event: EventRef?
        guard CreateEvent(nil, OSType(kEventClassKeyboard), UInt32(kEventHotKeyPressed), 0, EventAttributes(kEventAttributeNone), &event) == noErr,
              let ev = event else { return false }
        defer { ReleaseEvent(ev) }
        var hk = EventHotKeyID(signature: Self.signature, id: id)
        guard SetEventParameter(ev, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID),
                                MemoryLayout<EventHotKeyID>.size, &hk) == noErr else { return false }
        return SendEventToEventTarget(ev, GetApplicationEventTarget()) == noErr
    }
}
