// CaptureTypes.swift - quick capture ("Jot") shared types. Foundation only. FROZEN CONTRACT for M2 (see docs/v1/M2_BUILD_PLAN.md).
// Owned by the core stream; the UI stream compiles against these exactly.
import Foundation

/// One captured note. Written to the CaptureJournal before anything else happens, so it can never be lost.
struct CaptureItem: Codable, Equatable {
    var id: String          // UUID string
    var ts: Date
    var day: String         // the effective "today" when captured, yyyy-MM-dd
    var text: String        // trimmed, control characters removed (newlines kept), at most 4,000 characters
    var stamp: String?      // "14:32", nil when timestamps are off or the note is a to-do
    var todo: Bool          // the text began with "[]" and todoShorthand is on; the "[]" is already removed from `text`
    var source: String      // "hotkey", "menubar", "menu"
}

/// A global shortcut as Carbon understands it (kVK_ANSI_J = 38; controlKey 0x1000 | optionKey 0x0800 = 0x1800).
struct HotKeySpec: Codable, Equatable {
    var keyCode: UInt32
    var carbonModifiers: UInt32
    static let defaultJot = HotKeySpec(keyCode: 38, carbonModifiers: 0x1800)   // control-option-J
}

struct CapturePrefs: Codable, Equatable {
    var hotKey: HotKeySpec? = HotKeySpec.defaultJot   // nil = off
    var timestamps = true
    var todoShorthand = true
    var jotsHeading = "Jots"
    var jotsCountTowardLogged = false                 // jots never make a day "logged" unless the user turns this on

    init() {}
    enum CodingKeys: String, CodingKey { case hotKey, timestamps, todoShorthand, jotsHeading, jotsCountTowardLogged }
    init(from d: Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        hotKey = c.contains(.hotKey) ? try c.decodeIfPresent(HotKeySpec.self, forKey: .hotKey) : HotKeySpec.defaultJot
        timestamps = try c.decodeIfPresent(Bool.self, forKey: .timestamps) ?? timestamps
        todoShorthand = try c.decodeIfPresent(Bool.self, forKey: .todoShorthand) ?? todoShorthand
        jotsHeading = try c.decodeIfPresent(String.self, forKey: .jotsHeading) ?? jotsHeading
        jotsCountTowardLogged = try c.decodeIfPresent(Bool.self, forKey: .jotsCountTowardLogged) ?? jotsCountTowardLogged
    }
}

/// What the live editor did with an append request.
enum JotAppendResult: Equatable {
    case appended(normalizedMarkdown: String)   // the page now holds the note; the editor's normalised markdown after the change
    case alreadyPresent                         // a replay of a line the page already contains
    case notLoaded                              // the page is not ready yet: the writer keeps the note and retries after load
    case stale                                  // the page was swapped while the call was in flight: re-route once
}

/// The open page for a day. DayEditor (UI) implements it; the Core never imports the editor.
protocol PageHandle: AnyObject {
    var day: String { get }
    var isLoaded: Bool { get }
    /// Appends the rendered note under the Jots heading in the LIVE document (one editor transaction: the user's caret, text and undo history are untouched).
    func appendJot(_ item: CaptureItem, completion: @escaping (JotAppendResult) -> Void)
}
protocol EditorLookup: AnyObject {
    /// The page currently open for `day` (including a hidden window's page and pages that could not be saved), or nil.
    func page(for day: String) -> PageHandle?
}

struct CaptureReceipt: Equatable {
    var id: String
    var waiting: Int        // journal items not yet acknowledged, including this one; 0 means it is already in the file
}
