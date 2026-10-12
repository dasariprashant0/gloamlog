// EditorBridge.swift - owns the ONE WKWebView that hosts the bundled markdown editor (docs/EDITOR_CONTRACT.md) and speaks
// its bridge: JS -> Swift through the message handler `dl`, Swift -> JS through window.DailyLogEditor.
//
// Rules this file keeps:
//  * Values reach JS only as JSON literals made by JSONSerialization (JS.literal). User text is never concatenated into a script.
//  * Every document hand-over is tagged. Swift sets window.__dlDoc = <editor token> inside the same script that swaps the
//    markdown, so a read or replace that names a token can never touch another day's text (no cross-day writes).
//  * swapDocument() returns the OLD document's final markdown from the same script that loads the new one: nothing typed
//    in the last 300 ms is lost on a day switch, and no change event can be mistaken for the new day's.
//  * No network: persistent web data is off, navigation is blocked except the first load, links open in the default
//    browser only for http/https/mailto, and images come from the dlasset:// handler (AssetSchemeHandler).
import WebKit
import AppKit

final class EditorBridge: NSObject {
    /// Where the editor page lives: Contents/Resources/editor/index.html. Overridable for Tools/editor-check.
    static var htmlURL: () -> URL? = { Bundle.main.url(forResource: "index", withExtension: "html", subdirectory: "editor") }
    /// Opens an allowed external link. Overridable for Tools/editor-check (so the check never launches a browser).
    static var opener: (URL) -> Void = { NSWorkspace.shared.open($0) }
    /// Receives the page's warn/error log lines (Tools/editor-check collects them to catch CSP violations).
    var logHandler: ((String, String) -> Void)?

    let webView: EditorWKWebView
    weak var delegate: EditorBridgeDelegate?
    private(set) var isReady = false
    /// Set when a page was opened with focus before the web view had a window: the container takes focus on attach.
    var pendingFocus = false
    /// Swaps issued but not yet answered. Change events that arrive meanwhile are ambiguous and are dropped by the model
    /// (the old page's final text comes back inside the swap result).
    private(set) var swapsInFlight = 0
    /// appendToSection calls waiting for the page's `appendResult` (EditorBridge+Capture.swift), by call id.
    var pendingAppends = [String: (AppendOutcome) -> Void]()
    private var queue = [() -> Void]()
    var allowInitialLoad = false
    private let assetHandler = AssetSchemeHandler()
    private let proxy = WeakMessageProxy()
    private var readyTimer: DispatchWorkItem?

    init(delegate: EditorBridgeDelegate) {
        self.delegate = delegate
        let cfg = WKWebViewConfiguration()
        cfg.websiteDataStore = .nonPersistent()                  // nothing cached or stored on disk
        cfg.preferences.javaScriptCanOpenWindowsAutomatically = false
        cfg.setURLSchemeHandler(assetHandler, forURLScheme: AssetSchemeHandler.scheme)   // BEFORE the web view exists
        let ucc = WKUserContentController()
        cfg.userContentController = ucc
        webView = EditorWKWebView(frame: .zero, configuration: cfg)
        super.init()
        proxy.target = self
        ucc.add(proxy, name: "dl")                               // weak proxy: no retain cycle through the controller
        assetHandler.assets = { [weak delegate] in delegate?.editorAssetStore }
        webView.navigationDelegate = self
        webView.uiDelegate = self
        webView.allowsMagnification = false
        webView.allowsBackForwardNavigationGestures = false
        // Transparent page: the SwiftUI window paints the background (guarded: private key, harmless if missing).
        if webView.responds(to: NSSelectorFromString("_setDrawsBackground:")) { webView.setValue(false, forKey: "drawsBackground") }
        webView.appearanceChanged = { [weak self] in self?.pushTheme() }
        webView.setAccessibilityLabel("Daily log page")
        loadEditor()
    }

    deinit { webView.configuration.userContentController.removeScriptMessageHandler(forName: "dl") }

    // MARK: loading
    func loadEditor() {
        isReady = false; queue.removeAll(); swapsInFlight = 0
        guard let url = Self.htmlURL(),
              let html = try? String(contentsOf: url, encoding: .utf8) else {
            DispatchQueue.main.async { [weak self] in self?.delegate?.editorLoadFailed("The editor files are missing from the app. Reinstall Gloamlog.") }
            return
        }
        allowInitialLoad = true
        webView.loadHTMLString(html, baseURL: URL(string: "\(AssetSchemeHandler.scheme)://\(AssetSchemeHandler.host)/"))
        readyTimer?.cancel()
        let t = DispatchWorkItem { [weak self] in
            if let s = self, !s.isReady { s.delegate?.editorLoadFailed("The editor is taking a while to start.") }
        }
        readyTimer = t
        DispatchQueue.main.asyncAfter(deadline: .now() + 10, execute: t)
    }

    /// The web content process died: forget the page, load a fresh one FIRST (so the delegate's re-load request queues behind
    /// the new `ready`), then tell the delegate.
    func processTerminated() {
        isReady = false; swapsInFlight = 0
        failPendingAppends()
        loadEditor()
        delegate?.editorProcessDidTerminate()
    }

    func whenReady(_ block: @escaping () -> Void) {
        if isReady { block() } else { queue.append(block) }
    }

    func run(_ script: String, _ done: ((Any?) -> Void)? = nil) {
        webView.evaluateJavaScript(script) { result, error in
            if let e = error { NSLog("Gloamlog editor script failed: %@", e.localizedDescription) }
            done?(error == nil ? result : nil)
        }
    }

    // MARK: Swift -> JS
    /// Loads `markdown` for the editor identified by `token` and returns, from the same script, the PREVIOUS document's
    /// token and final markdown plus the new document's normalised markdown (read twice: the second read is the baseline,
    /// `stable` tells whether both reads agreed). completion gets nils if the script failed.
    func swapDocument(token: String, markdown: String, cursor: EditorCursor, focus: Bool,
                      completion: @escaping (_ prevToken: String?, _ prev: String?, _ loaded: String?, _ stable: Bool) -> Void) {
        swapsInFlight += 1
        whenReady { [weak self] in
            guard let self = self else { return }
            if focus {
                if let w = self.webView.window { w.makeFirstResponder(self.webView) } else { self.pendingFocus = true }
            }
            let script = """
            (function () {
              var E = window.DailyLogEditor;
              var prevToken = (typeof window.__dlDoc === 'string') ? window.__dlDoc : null;
              var prev = E.getMarkdown();
              E.setMarkdown(\(JS.literal(markdown)), { cursor: \(JS.literal(cursor.rawValue)), focus: \(focus ? "true" : "false") });
              E.setReadOnly(false);
              window.__dlDoc = \(JS.literal(token));
              var first = E.getMarkdown();
              var second = E.getMarkdown();
              return [prevToken, prev, second, first === second];
            })()
            """
            self.run(script) { result in
                self.swapsInFlight = max(0, self.swapsInFlight - 1)
                guard let a = result as? [Any], a.count == 4 else { completion(nil, nil, nil, false); return }
                completion(a[0] as? String, a[1] as? String, a[2] as? String, (a[3] as? Bool) ?? false)
            }
        }
    }

    /// The editor's markdown, but only while the page still holds the document of `token` (nil otherwise).
    func read(token: String, completion: @escaping (String?) -> Void) {
        guard isReady else { completion(nil); return }
        run("(window.__dlDoc === \(JS.literal(token))) ? DailyLogEditor.getMarkdown() : null") { completion($0 as? String) }
    }

    /// Replaces the page's markdown in place (cursor kept), only if the page still holds `token` and, when `expecting` is
    /// given, still has exactly that markdown (i.e. the user typed nothing in the meantime). Returns the new normalised
    /// markdown, or nil when it refused.
    func replace(token: String, expecting: String?, markdown: String, cursor: EditorCursor,
                 completion: @escaping (String?) -> Void) {
        guard isReady else { completion(nil); return }
        let script = """
        (function () {
          var E = window.DailyLogEditor;
          if (window.__dlDoc !== \(JS.literal(token))) return null;
          var expected = \(JS.literal(expecting));
          if (expected !== null && E.getMarkdown() !== expected) return null;
          E.setMarkdown(\(JS.literal(markdown)), { cursor: \(JS.literal(cursor.rawValue)), focus: false });
          return E.getMarkdown();
        })()
        """
        run(script) { completion($0 as? String) }
    }

    /// Inserts markdown at the caret / start / end of the page holding `token`. Like every Swift-initiated change it emits no
    /// change event: the caller must treat the returned markdown as an edit that still has to be saved.
    func insert(token: String, markdown: String, at place: EditorInsertPlace, completion: @escaping (String?) -> Void) {
        guard isReady else { completion(nil); return }
        let script = """
        (function () {
          var E = window.DailyLogEditor;
          if (window.__dlDoc !== \(JS.literal(token))) return null;
          E.insertMarkdown(\(JS.literal(markdown)), \(JS.literal(place.rawValue)));
          return E.getMarkdown();
        })()
        """
        run(script) { completion($0 as? String) }
    }

    /// An untouched template page is only headings, so there is nowhere to put the caret UNDER a heading: typing would edit the
    /// heading itself. This gives every heading that has nothing below it an empty paragraph (what pressing Enter at the end of
    /// each heading does). Empty paragraphs are not part of the markdown the editor writes out, so the page text, the change
    /// events and the file stay exactly as they were: this is not an edit. It works from the last heading to the first, so the
    /// caret finishes in the first empty line.
    func openTemplateLines(token: String, completion: @escaping (Bool) -> Void = { _ in }) {
        guard isReady else { completion(false); return }
        let script = """
        (function (token) {
          if (window.__dlDoc !== token) return false;
          var pm = document.querySelector('.ProseMirror'); if (!pm) return false;
          var isHead = function (n) { return !!n && /^H[1-6]$/.test(n.tagName); };
          var isWidget = function (n) { return !!n && n.classList && (n.classList.contains('ProseMirror-widget') || n.classList.contains('prosemirror-virtual-cursor')); };
          var headings = function () { return Array.prototype.filter.call(pm.children, isHead); };
          var below = function (h) { var n = h.nextElementSibling; while (isWidget(n)) n = n.nextElementSibling; return n; };
          var count = headings().length;
          pm.focus();
          for (var i = count - 1; i >= 0; i--) {
            var h = headings()[i]; if (!h) continue;
            var next = below(h);
            if (next && !isHead(next)) continue;
            var r = document.createRange(); r.selectNodeContents(h); r.collapse(false);
            var sel = window.getSelection(); sel.removeAllRanges(); sel.addRange(r);
            document.execCommand('insertParagraph');
          }
          return true;
        })(\(JS.literal(token)))
        """
        run(script) { completion(($0 as? Bool) == true) }
    }

    func focusEditor() {
        webView.window?.makeFirstResponder(webView)
        whenReady { [weak self] in self?.run("DailyLogEditor.focus()") }
    }

    func setReadOnly(_ on: Bool) { whenReady { [weak self] in self?.run("DailyLogEditor.setReadOnly(\(on ? "true" : "false"))") } }

    /// Scrolls the first occurrence of `text` into view (selection only; never edits). Used after opening a search hit.
    func reveal(text: String) {
        guard !text.dlTrimmed.isEmpty else { return }
        whenReady { [weak self] in
            self?.run("(function(){ var s = window.getSelection(); if (s) s.removeAllRanges(); return window.find(\(JS.literal(text)), false, false, true, false, false, false); })()")
        }
    }

    func pushTheme() {
        let dark = webView.effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
        whenReady { [weak self] in self?.run("DailyLogEditor.setTheme(\(JS.literal(dark ? "dark" : "light")))") }
    }

    private func resolveUpload(_ id: String, path: String?, error: String?) {
        whenReady { [weak self] in
            self?.run("DailyLogEditor._resolveUpload(\(JS.literal(id)), \(JS.literal(path)), \(JS.literal(error)))")
        }
    }

    // MARK: JS -> Swift
    func handle(_ body: Any) {
        guard let m = body as? [String: Any], let type = m["type"] as? String else { return }
        switch type {
        case "ready":
            // A second `ready` means the page reloaded: everything it held is gone and must be loaded again (contract).
            let reloaded = isReady
            isReady = true; readyTimer?.cancel()
            if reloaded { swapsInFlight = 0; failPendingAppends() }
            pushTheme()
            delegate?.editorBecameReady()
            if reloaded { delegate?.editorProcessDidTerminate() }
            let q = queue; queue = []
            q.forEach { $0() }
        case "change":
            if let md = m["markdown"] as? String { delegate?.editorDidChange(markdown: md) }
        case "appendResult":
            handleAppendResult(m)
        case "uploadImage":
            handleUpload(m)
        case "openLink":
            if let s = m["url"] as? String { Self.openExternal(s) }
        case "log":
            let level = (m["level"] as? String) ?? "info"
            let text = String(((m["message"] as? String) ?? "").prefix(400))
            if level == "warn" || level == "error" { NSLog("Gloamlog editor %@: %@", level, text) }
            logHandler?(level, text)
            if text.contains("could not be parsed without loss") { delegate?.editorShowsRawText() }
        default: break
        }
    }

    static func openExternal(_ s: String) {
        guard let url = URL(string: s.dlTrimmed), let scheme = url.scheme?.lowercased(),
              ["http", "https", "mailto"].contains(scheme) else { return }
        opener(url)
    }

    private func handleUpload(_ m: [String: Any]) {
        guard let id = m["id"] as? String else { return }
        guard let b64 = m["dataBase64"] as? String,
              let ext = AssetStore.fileExtension(mime: m["mime"] as? String, name: m["name"] as? String) else {
            resolveUpload(id, path: nil, error: "Only PNG, JPEG, GIF and WebP images can be added."); return
        }
        guard let store = delegate?.editorAssetStore, let ctx = delegate?.editorUploadContext else {
            resolveUpload(id, path: nil, error: "Open a day's page before adding an image."); return
        }
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            var path: String?, err: String?
            if let data = Data(base64Encoded: b64, options: .ignoreUnknownCharacters) {
                do { path = try store.save(data: data, ext: ext, day: ctx.day) } catch { err = EditorBridge.uploadMessage(error) }
            } else { err = "That image couldn't be read." }
            DispatchQueue.main.async {
                guard let self = self else { return }
                // The user may have moved to another day meanwhile: do not drop the picture into the wrong page.
                if path != nil, self.delegate?.editorUploadContext?.token != ctx.token {
                    self.resolveUpload(id, path: nil, error: "The page changed before the image was added. Try again."); return
                }
                self.resolveUpload(id, path: err == nil ? path : nil, error: err)
                if let e = err { self.delegate?.editorNotice(e) }
            }
        }
    }

    static func uploadMessage(_ error: Error) -> String {
        switch error as? LogError {
        case .some(.unsupportedAsset): return "Only PNG, JPEG, GIF and WebP images can be added."
        case .some(.assetTooLarge): return "Images can be up to 20 MB."
        case .some(.emptyAsset): return "That image is empty."
        case .some(.folderMissing), .some(.folderNotWritable): return "Gloamlog can't save to the log folder. Check Settings > Storage."
        case .some(.badPath): return "The assets folder is not inside the log folder."
        case .some(.io(let m)): return "Couldn't save the image. \(m)"
        default: return "Couldn't save the image."
        }
    }
}
