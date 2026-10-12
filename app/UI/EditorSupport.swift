// EditorSupport.swift - small pieces shared by the editor host: JSON literals for JS, the delegate protocol, the web view
// subclass (appearance + context menu) and the weak message-handler proxy.
import WebKit
import AppKit

enum EditorCursor: String { case start, end, keep }
enum EditorInsertPlace: String { case cursor, start, end }

enum JS {
    /// A JSON literal for a JSON-compatible value (String, NSNumber/Bool, NSNull/nil, Array, Dictionary).
    static func literal(_ value: Any?) -> String {
        let v: Any = value ?? NSNull()
        guard JSONSerialization.isValidJSONObject([v]),
              let data = try? JSONSerialization.data(withJSONObject: [v], options: []),
              var s = String(data: data, encoding: .utf8) else { return "null" }
        s.removeFirst(); s.removeLast()                      // strip the wrapping [ ]
        // U+2028/2029 are legal in JSON but ended string literals in older JS engines: escape them anyway.
        return s.replacingOccurrences(of: "\u{2028}", with: "\\u2028").replacingOccurrences(of: "\u{2029}", with: "\\u2029")
    }
}

protocol EditorBridgeDelegate: AnyObject {
    func editorBecameReady()
    /// The page cannot be edited safely: the editor shows the raw text read-only and the file stays untouched.
    func editorShowsRawText()
    /// A USER edit (debounced 300 ms in the page). Never fired for Swift-initiated setMarkdown.
    func editorDidChange(markdown: String)
    func editorProcessDidTerminate()
    func editorLoadFailed(_ message: String)
    func editorNotice(_ message: String)
    var editorAssetStore: AssetStore? { get }
    /// The day (and its token) an upload belongs to, or nil when no editable page is open.
    var editorUploadContext: (day: String, token: String)? { get }
}

final class EditorWKWebView: WKWebView {
    var appearanceChanged: (() -> Void)?
    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        appearanceChanged?()
    }

    /// ⌘[ and ⌘] are the Go menu's Previous Day and Next Day. The page binds the same two keys to outdent and indent inside a
    /// list item, a table cell and a code block, and WebKit hands it the key first, so there the day never changed. These two
    /// key equivalents are offered to the main menu BEFORE the page. When the menu declines (a disabled item, such as ⌘] on
    /// today) the page gets the key as it always did. Nothing else is touched: every other shortcut still goes to the page first.
    override func performKeyEquivalent(with event: NSEvent) -> Bool {
        if event.type == .keyDown,
           event.modifierFlags.intersection(.deviceIndependentFlagsMask).subtracting([.capsLock, .numericPad, .function]) == .command,
           let c = event.charactersIgnoringModifiers, c == "[" || c == "]",
           NSApp.mainMenu?.performKeyEquivalent(with: event) == true { return true }
        return super.performKeyEquivalent(with: event)
    }

    /// The default context menu stays (copy, paste, spelling, look up), minus what would navigate or download: a reload
    /// would drop the page's unsaved state, and there is nowhere to download to.
    private static let removed: Set<String> = ["WKMenuItemIdentifierReload", "WKMenuItemIdentifierGoBack", "WKMenuItemIdentifierGoForward",
        "WKMenuItemIdentifierInspectElement", "WKMenuItemIdentifierOpenFrameInNewWindow", "WKMenuItemIdentifierDownloadImage",
        "WKMenuItemIdentifierDownloadLinkedFile", "WKMenuItemIdentifierOpenImageInNewWindow", "WKMenuItemIdentifierOpenMediaInNewWindow",
        "WKMenuItemIdentifierDownloadMedia"]
    override func willOpenMenu(_ menu: NSMenu, with event: NSEvent) {
        super.willOpenMenu(menu, with: event)
        for item in menu.items where Self.removed.contains(item.identifier?.rawValue ?? "") { menu.removeItem(item) }
    }
}

final class WeakMessageProxy: NSObject, WKScriptMessageHandler {
    weak var target: EditorBridge?
    func userContentController(_ c: WKUserContentController, didReceive message: WKScriptMessage) {
        guard message.frameInfo.isMainFrame else { return }
        target?.handle(message.body)
    }
}

