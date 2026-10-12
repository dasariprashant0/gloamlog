// Integration check for the editor host (NOT shipped). Loads the REAL bundled editor (app/Resources/editor/index.html)
// into a WKWebView through EditorBridge + AssetSchemeHandler and exercises the contract: ready, swap/read/replace with
// document tokens, JSON-literal safety, dlasset:// images (and what it must refuse), user vs programmatic change events,
// paste-an-image upload round trip, navigation blocking, CSP violations.
// WKWebView needs a real WebKit: run it OUTSIDE the macOS sandbox (see run.sh). It never touches the network or your logs.
import AppKit
import WebKit

var failures = 0, passes = 0
func check(_ name: String, _ ok: Bool, _ detail: String = "") {
    if ok { passes += 1; print("  ok   \(name)") } else { failures += 1; print("  FAIL \(name)\(detail.isEmpty ? "" : "  -> \(detail)")") }
}
func spin(_ seconds: TimeInterval) { let end = Date().addingTimeInterval(seconds); while Date() < end { RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.01)) } }
@discardableResult
func wait(_ timeout: TimeInterval = 10, until cond: () -> Bool) -> Bool {
    let end = Date().addingTimeInterval(timeout)
    while !cond() && Date() < end { RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.01)) }
    return cond()
}
func blocking<T>(_ timeout: TimeInterval = 10, _ body: (@escaping (T) -> Void) -> Void) -> T? {
    var result: T?; var done = false
    body { result = $0; done = true }
    wait(timeout) { done }
    return result
}

// Screenshot mode: real UI + real editor, no checks.
if let i = CommandLine.arguments.firstIndex(of: "--screenshots"), i + 1 < CommandLine.arguments.count {
    _ = NSApplication.shared
    NSApp.setActivationPolicy(.accessory)
    let html = CommandLine.arguments.count > 1 && !CommandLine.arguments[1].hasPrefix("--") ? CommandLine.arguments[1] : "app/Resources/editor/index.html"
    EditorBridge.htmlURL = { URL(fileURLWithPath: html) }
    runScreenshots(CommandLine.arguments[i + 1])
    exit(0)
}

// ---- host ----
let base = ProcessInfo.processInfo.environment["EC_TMP"] ?? NSTemporaryDirectory()
let root = URL(fileURLWithPath: base).appendingPathComponent("dl-editor-check")
try? FileManager.default.removeItem(at: root)
let assetsDir = root.appendingPathComponent("assets")
try FileManager.default.createDirectory(at: assetsDir, withIntermediateDirectories: true)

let img = NSImage(size: NSSize(width: 64, height: 40))
img.lockFocus(); NSColor.systemGreen.setFill(); NSBezierPath(rect: NSRect(x: 0, y: 0, width: 64, height: 40)).fill(); img.unlockFocus()
let png = NSBitmapImageRep(data: img.tiffRepresentation!)!.representation(using: .png, properties: [:])!
try png.write(to: assetsDir.appendingPathComponent("x.png"))
try "not an image".write(to: assetsDir.appendingPathComponent("secret.txt"), atomically: false, encoding: .utf8)
try "outside the storage folder".write(to: URL(fileURLWithPath: base).appendingPathComponent("dl-outside.png"), atomically: false, encoding: .utf8)
try? FileManager.default.createSymbolicLink(at: assetsDir.appendingPathComponent("link.png"), withDestinationURL: URL(fileURLWithPath: base).appendingPathComponent("dl-outside.png"))

final class Host: NSObject, EditorBridgeDelegate {
    var ready = 0, changes = [String](), notices = [String](), terminated = 0, failed = [String](), rawTexts = 0
    var store: AssetStore? = AssetStore(dir: root)
    var context: (day: String, token: String)?
    func editorBecameReady() { ready += 1 }
    func editorShowsRawText() { rawTexts += 1 }
    func editorDidChange(markdown: String) { changes.append(markdown) }
    func editorProcessDidTerminate() { terminated += 1 }
    func editorLoadFailed(_ message: String) { failed.append(message) }
    func editorNotice(_ message: String) { notices.append(message) }
    var editorAssetStore: AssetStore? { store }
    var editorUploadContext: (day: String, token: String)? { context }
}

_ = NSApplication.shared
NSApp.setActivationPolicy(.accessory)
let htmlPath = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "app/Resources/editor/index.html"
EditorBridge.htmlURL = { URL(fileURLWithPath: htmlPath) }
var opened = [URL]()
EditorBridge.opener = { opened.append($0) }

let host = Host()
let bridge = EditorBridge(delegate: host)
var logs = [(String, String)]()
bridge.logHandler = { logs.append(($0, $1)) }
let win = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 900, height: 700), styleMask: [.titled, .resizable], backing: .buffered, defer: false)
win.contentView = bridge.webView
win.orderFrontRegardless()

func js(_ s: String) -> Any? { blocking { done in bridge.webView.evaluateJavaScript(s) { r, _ in done(r) } } ?? nil }
func jsAsync(_ s: String, _ args: [String: Any] = [:]) -> Any? {
    blocking(15) { done in bridge.webView.callAsyncJavaScript(s, arguments: args, in: nil, in: .page) { r in if case .success(let v) = r { done(v) } else { done(NSNull()) } } } ?? nil
}
func swap(_ token: String, _ md: String, cursor: EditorCursor = .start) -> (prevToken: String?, prev: String?, loaded: String?, stable: Bool)? {
    blocking { done in bridge.swapDocument(token: token, markdown: md, cursor: cursor, focus: false) { a, b, c, d in done((a, b, c, d)) } }
}
func read(_ token: String) -> String?? { blocking { done in bridge.read(token: token) { done($0) } } }

// ---- 1. start up ----
print("1. start-up")
check("editor page reports ready", wait(25) { bridge.isReady })
check("no load failure", host.failed.isEmpty, host.failed.joined(separator: "; "))
check("window.DailyLogEditor exists", (js("typeof window.DailyLogEditor === 'object'") as? Bool) == true)
check("page is transparent (html background)", (js("getComputedStyle(document.documentElement).backgroundColor") as? String) == "rgba(0, 0, 0, 0)")
check("web view does not draw its own background", (bridge.webView.value(forKey: "drawsBackground") as? Bool) == false)
guard bridge.isReady else { print("editor never became ready; stopping"); exit(1) }

// ---- 2. documents, tokens, round trip ----
print("2. swap / read / replace")
let doc1 = "## What I did\n\n- one\n- two\n\nSome *text* with **bold**.\n"
let r1 = swap("T1", doc1)
check("first swap answers", r1 != nil && r1?.loaded != nil)
check("first swap has no previous token", r1?.prevToken == nil)
check("serialiser is stable on first load", r1?.stable == true)
print("     normalised: \(String(reflecting: r1?.loaded ?? ""))")
let loaded1 = r1?.loaded ?? ""
let r2 = swap("T2", loaded1)
check("second swap returns the OLD token", r2?.prevToken == "T1")
check("second swap returns the old page's final text", r2?.prev == loaded1)
check("round trip is idempotent (serialize(parse(x)) == x)", r2?.loaded == loaded1, "\(String(reflecting: r2?.loaded ?? ""))")
check("read with the current token works", read("T2") == .some(loaded1))
check("read with a stale token refuses", read("T1") == .some(nil))
let rep = blocking { done in bridge.replace(token: "T2", expecting: loaded1, markdown: "replaced", cursor: .keep) { done($0) } } ?? nil
check("replace (right token, expected text) succeeds", rep == "replaced\n", String(describing: rep))
let repBad = blocking { done in bridge.replace(token: "T2", expecting: "something else", markdown: "nope", cursor: .keep) { done($0) } } ?? "x"
check("replace refuses when the page changed meanwhile", repBad == nil)
let repTok = blocking { done in bridge.replace(token: "T9", expecting: nil, markdown: "nope", cursor: .keep) { done($0) } } ?? "x"
check("replace refuses a foreign token", repTok == nil)
check("page still holds 'replaced'", read("T2") == .some("replaced\n"))
spin(0.9)
check("programmatic swap/replace emit NO change event", host.changes.isEmpty, "\(host.changes)")

let ins = blocking { (done: @escaping (String?) -> Void) in bridge.insert(token: "T2", markdown: "- appended item", at: .end) { done($0) } } ?? nil
check("insertMarkdown appends at the end and returns the page", ins?.contains("appended item") == true, String(describing: ins))
let insBad = blocking { (done: @escaping (String?) -> Void) in bridge.insert(token: "nope", markdown: "x", at: .end) { done($0) } } ?? "x"
check("insertMarkdown refuses a foreign token", insBad == nil)
spin(0.9)
check("insertMarkdown emits NO change event either", host.changes.isEmpty, "\(host.changes)")

// ---- 3. hostile text goes through as data ----
print("3. hostile text")
let evil = "He said \"hi\" and 'bye' \\ ${1+1} `tick` </script><script>window.__pwned=1</script> \u{2028}\u{2029} \\u0041 😀 \0x"
let r3 = swap("T3", evil.replacingOccurrences(of: "\0", with: ""))
check("swap with quotes/backticks/script/line-separators answers", r3?.loaded != nil)
check("no script injected", (js("window.__pwned === undefined") as? Bool) == true)
let r3b = swap("T3b", r3?.loaded ?? "")
check("hostile text round trip is stable", r3b?.loaded == r3?.loaded)
check("JS.literal escapes U+2028/2029", !JS.literal("a\u{2028}b").contains("\u{2028}") && JS.literal("a\"b\\n") == "\"a\\\"b\\\\n\"")

// ---- 4. images through dlasset:// ----
print("4. dlasset images")
_ = swap("T4", "Before\n\n![shot](assets/x.png)\n\nAfter\n")
spin(0.8)
let natural = js("(document.querySelector('.ProseMirror img') || {}).naturalWidth") as? Int ?? -1
check("assets/x.png loads through the scheme handler", natural > 0, "naturalWidth=\(natural)")
check("page maps assets/ to dlasset://local/", (js("(document.querySelector('.ProseMirror img') || {}).src") as? String)?.hasPrefix("dlasset://local/assets/") == true)
check("markdown keeps the RELATIVE path", read("T4") == .some("Before\n\n![shot](assets/x.png)\n\nAfter") || (read("T4") ?? "")?.contains("](assets/x.png)") == true)
func load(_ url: String) -> String {
    (jsAsync("return await new Promise(r => { const i = new Image(); i.onload = () => r('load'); i.onerror = () => r('error'); i.src = u; })", ["u": url]) as? String) ?? "timeout"
}
check("non-image file in assets/ is refused", load("dlasset://local/assets/secret.txt") == "error")
check("symlink leaving the folder is refused", load("dlasset://local/assets/link.png") == "error")
check("path traversal is refused", load("dlasset://local/assets/../../dl-outside.png") == "error")
check("other host is refused", load("dlasset://evil/assets/x.png") == "error")
check("missing file is a 404", load("dlasset://local/assets/nope.png") == "error")
check("the real image still loads", load("dlasset://local/assets/x.png") == "load")
_ = swap("T5", "![remote](https://example.com/pixel.png)\n")
spin(1.0)
check("remote image does not load (CSP)", (js("(document.querySelector('.ProseMirror img') || {complete:true, naturalWidth: 0}).naturalWidth") as? Int ?? 0) == 0)
check("remote request produced a CSP violation log", logs.contains { $1.contains("CSP blocked") }, "\(logs.map { $1 })")

// ---- 5. user edits produce change events ----
print("5. user edits")
host.changes.removeAll()
_ = swap("T6", "Hello world")
spin(0.3)
_ = js("DailyLogEditor.focus(); document.execCommand('insertText', false, ' typed'); 1")
var gotChange = wait(3) { !host.changes.isEmpty }
if !gotChange {   // no key window / focus in this headless run: a DOM edit goes through the same MutationObserver path
    _ = js("var p = document.querySelector('.ProseMirror p'); p.appendChild(document.createTextNode(' typed')); 1")
    gotChange = wait(3) { !host.changes.isEmpty }
}
check("a user edit emits ONE debounced change", gotChange && host.changes.count == 1, "\(host.changes)")
check("change carries the page markdown", host.changes.last?.contains("typed") == true, "\(host.changes)")
check("no swap is reported in flight afterwards", bridge.swapsInFlight == 0)

// ---- 6. paste an image: upload round trip ----
print("6. paste image")
host.changes.removeAll(); host.notices.removeAll()
host.context = (day: "2026-10-06", token: "T6")
let b64 = png.base64EncodedString()
_ = js("""
(function(){
  var bytes = Uint8Array.from(atob('\(b64)'), function(c){ return c.charCodeAt(0); });
  var file = new File([bytes], 'shot.png', {type: 'image/png'});
  var dt = new DataTransfer(); dt.items.add(file);
  var ev = new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true});
  document.querySelector('.ProseMirror').focus();
  document.querySelector('.ProseMirror').dispatchEvent(ev);
  return 1;
})()
""")
let uploaded = wait(8) { host.changes.contains { $0.contains("assets/2026-10-06-") } }
check("pasted image is saved by AssetStore and inserted by relative path", uploaded, "changes=\(host.changes) notices=\(host.notices) logs=\(logs.suffix(3).map { $1 })")
let saved = (try? FileManager.default.contentsOfDirectory(atPath: assetsDir.path))?.filter { $0.hasPrefix("2026-10-06-") } ?? []
check("an assets/<day>-<hash>.png file exists", saved.count == 1 && saved[0].hasSuffix(".png"), "\(saved)")

// ---- 7. theme, navigation, links ----
print("7. theme, navigation, links")
bridge.pushTheme(); spin(0.3)
check("theme is pushed to the page", ["light", "dark"].contains(js("document.documentElement.dataset.theme") as? String ?? ""))
win.appearance = NSAppearance(named: .darkAqua); spin(0.6)
check("dark appearance flips the page theme", js("document.documentElement.dataset.theme") as? String == "dark")
win.appearance = NSAppearance(named: .aqua); spin(0.6)
check("... and back to light", js("document.documentElement.dataset.theme") as? String == "light")
_ = swap("T7", "Alpha beta\n\n## Heading\n\nGamma delta epsilon\n")
bridge.reveal(text: "delta"); spin(0.5)
check("reveal(text:) selects the first match without editing", js("window.getSelection().toString()") as? String == "delta" && (read("T7") ?? "")?.contains("delta") == true)
let before = bridge.webView.url?.absoluteString ?? ""
_ = js("window.__marker = 42; setTimeout(function(){ location.href = 'https://example.com/'; }, 10); 1")
spin(1.0)
check("script navigation is blocked (page and state survive)", (js("window.__marker") as? Int) == 42 && (bridge.webView.url?.absoluteString ?? "") == before)
EditorBridge.openExternal("https://example.com/a"); EditorBridge.openExternal("mailto:a@b.c")
EditorBridge.openExternal("file:///etc/hosts"); EditorBridge.openExternal("javascript:alert(1)"); EditorBridge.openExternal("x-apple.systempreferences:")
check("only http/https/mailto links are opened", opened.map { $0.absoluteString } == ["https://example.com/a", "mailto:a@b.c"], "\(opened)")
check("page made no other CSP-relevant noise", !logs.contains { $0.0 == "error" }, "\(logs.filter { $0.0 == "error" }.map { $1 })")

// ---- 7b. no retain cycle ----
weak var weakBridge: EditorBridge?
autoreleasepool {
    let h = Host(); let b = EditorBridge(delegate: h); weakBridge = b
    spin(1.5)
}
spin(1.5)
check("an EditorBridge is released when dropped (weak message proxy, no cycle)", weakBridge == nil)

// ---- 7c. raw-text fallback notice ----
_ = js("window.webkit.messageHandlers.dl.postMessage({type: 'log', level: 'warn', message: 'markdown could not be parsed without loss; showing the raw text read-only'}); 1")
spin(0.4)
check("the page's 'could not be parsed without loss' warning reaches the host", host.rawTexts == 1, "\(host.rawTexts)")

// ---- 8. size ----
print("8. large page")
let big = (1...3000).map { "Line \($0) with some words to count and **bold** text" }.joined(separator: "\n\n")
let t0 = Date()
let rb = swap("T8", big)
let dt = Date().timeIntervalSince(t0)
check("3,000-paragraph page swaps in under 5 s", rb?.loaded != nil && dt < 5, String(format: "%.2fs", dt))
print(String(format: "     (took %.2fs)", dt))

// ---- 9. the app layer on top (AppModel + DayEditor + LogStore + this same editor) ----
if !CommandLine.arguments.contains("--bridge-only") { runAppScenarios(); runNavScenarios(); runSettingsScenarios(); runCaptureScenarios() }

print("\n\(passes) passed, \(failures) failed")
exit(failures == 0 ? 0 : 1)
