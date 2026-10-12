// AppModel+Editor.swift - which page the shared web editor holds, flushing it safely, and the EditorBridge delegate.
//
// One WKWebView serves every day. Opening another day is ONE script (EditorBridge.swapDocument): it returns the old
// page's final text and loads the new page, so the old page is saved from exactly what the user saw and a late change
// event can never be attributed to the wrong day.
import SwiftUI
import AppKit

extension AppModel {
    /// The shared editor host; created on first use (never in the snapshot harness).
    var bridge: EditorBridge {
        if let b = bridgeStorage { return b }
        let b = EditorBridge(delegate: self)
        bridgeStorage = b
        return b
    }
    var bridgeIfCreated: EditorBridge? { bridgeStorage }

    // MARK: opening a day
    /// Opens any date from 2000-01-01 to today, whether or not it has a file. Opening writes nothing: DayEditor creates the
    /// file only after the first real edit. `force` reloads the same day (after a skip, restore or template change).
    func openDay(_ k: String, focus: Bool = false, force: Bool = false) {
        guard force || canOpen(k) else { return }
        if !force, let e = editor, e.day == k, selection == .day(k) { if focus { focusEditor() }; return }
        let old = editor
        let new = DayEditor(day: k, model: self)
        editor = new
        selection = .day(k)
        shownMonth = YearMonth(day: k); stripDay = k                // the sidebar calendar follows the page
        if let s = session, !s.days.contains(k) { session = nil }   // any other route ends a catch-up session
        rolloverDismissed = false
        carryUndoBody = nil
        refreshCarry()
        activate(new, replacing: old, focus: focus)
    }

    /// Loads `e` into the web editor, saving `old` from its final text. Calls queue until the editor is ready.
    func activate(_ e: DayEditor, replacing old: DayEditor?, focus: Bool) {
        guard live || bridgeStorage != nil else { old?.abandon(); return }
        let md = e.showsEditor ? e.initialBody : ""
        let cursor: EditorCursor = (focus && MarkdownBody.hasContent(md)) ? .end : .start
        bridge.swapDocument(token: e.token, markdown: md, cursor: cursor, focus: focus && e.showsEditor) { [weak self] prevToken, prev, loaded, stable in
            guard let self = self else { return }
            if let o = old { o.finish(with: prevToken == o.token ? prev : nil); self.keepIfUnsaved(o) }
            self.retiredMarkdown = prev
            guard let loaded = loaded else { if e.showsEditor { e.markLoadFailed() }; return }
            if e.showsEditor { e.didLoad(markdown: loaded, stable: stable) } else { e.markNotLoaded() }
            // A page that is only the template's headings (or the template above a day's jots) gets an empty line under each
            // heading to type into (not an edit).
            if e.isPristineTemplate || e.isTemplateOverJots { self.bridge.openTemplateLines(token: e.token) }
            self.captureEditorLoaded(e)                 // notes that were waiting for this page can go in now
        }
    }

    /// "Write a log anyway" on a skipped page: the page becomes editable, with the template.
    func startWritingAnyway() {
        guard let e = editor, e.isSkipped, !e.writingAnyway else { return }
        e.writingAnyway = true
        activate(e, replacing: nil, focus: true)
    }

    func focusEditor() {
        guard let e = editor, e.showsEditor else { return }
        bridgeStorage?.focusEditor()
    }

    // MARK: flushing
    /// Reads the page's final text and saves it if it was edited. `done` runs after the write (or at once if nothing is open).
    func flushEditor(_ done: (() -> Void)? = nil) {
        guard let e = editor, e.loaded, let b = bridgeStorage, b.isReady else { done?(); return }
        b.read(token: e.token) { md in e.finish(with: md); done?() }
    }

    /// Quit: flush with a 1.5 s ceiling so a stuck web process can never hold the app open. `done(true)` = nothing unsaved remains.
    func flushForQuit(_ done: @escaping (Bool) -> Void) {
        retryOrphans()
        guard let e = editor, e.loaded, let b = bridgeStorage, b.isReady else { done(!(editor?.hasUnsavedEdits ?? false) && orphans.isEmpty); return }
        var finished = false
        let end: () -> Void = { if !finished { finished = true; self.retryOrphans(); done(!e.hasUnsavedEdits && self.orphans.isEmpty) } }
        b.read(token: e.token) { md in e.finish(with: md); end() }
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { if !finished { e.finish(with: nil); end() } }
    }
    var editorNeedsQuitFlush: Bool { ((editor?.loaded ?? false) && (bridgeStorage?.isReady ?? false)) || !orphans.isEmpty }

    /// Quit with edits that could not be written: let the user take the text with them.
    func confirmQuitWithUnsavedEdits() -> Bool {
        let a = NSAlert()
        a.messageText = "Your latest edits couldn't be saved."
        a.informativeText = "Gloamlog can't write to the log folder right now. Copy the text, then quit, or go back and fix the folder first."
        a.addButton(withTitle: "Copy text and quit")
        a.addButton(withTitle: "Don't quit")
        guard a.runModal() == .alertFirstButtonReturn else { return false }
        copyUnsavedText()
        return true
    }

    /// Pages that could not be written, as markdown with a "# day" heading each (for "Copy my text").
    func copyUnsavedText() {
        var parts = [String]()
        for e in ([editor].compactMap { $0 } + orphans) where e.hasUnsavedEdits { parts.append("# \(e.day)\n\n\(e.writableLatest)") }
        if parts.isEmpty { editor?.copyLatest(); return }
        NSPasteboard.general.clearContents(); NSPasteboard.general.setString(parts.joined(separator: "\n\n"), forType: .string)
    }

    // MARK: pages that could not be saved when they were left
    func keepIfUnsaved(_ o: DayEditor) {
        guard o.hasUnsavedEdits, !orphans.contains(where: { $0 === o }) else { return }
        orphans.append(o)
    }
    func retryOrphans() {
        guard !orphans.isEmpty else { return }
        for o in orphans { o.retryIfNeeded() }
        orphans.removeAll { !$0.hasUnsavedEdits }
    }

    func copyPageMarkdown() {
        guard let e = editor else { return }
        let put: (String) -> Void = { NSPasteboard.general.clearContents(); NSPasteboard.general.setString($0, forType: .string) }
        if e.loaded, let b = bridgeStorage { b.read(token: e.token) { put($0 ?? e.latest) } } else { put(e.latest) }
    }

    func revealPageInFinder() {
        let url = store.url(for: editor?.day ?? today)
        if FileManager.default.fileExists(atPath: url.path) { reveal([url]) } else { revealFolder() }
    }
}

extension DayEditor {
    func copyLatest() {
        NSPasteboard.general.clearContents(); NSPasteboard.general.setString(latest, forType: .string)
    }
}

// MARK: - EditorBridgeDelegate
extension AppModel: EditorBridgeDelegate {
    func editorBecameReady() { editorProblem = nil }
    func editorShowsRawText() { editor?.showRawTextNotice() }

    func editorDidChange(markdown md: String) {
        guard let b = bridgeStorage, b.swapsInFlight == 0, let e = editor, e.loaded else { return }
        if let r = retiredMarkdown, md == r, md != e.baselineMarkdown { return }   // the old page's text, not an edit of this one
        e.userChanged(md)
    }

    func editorProcessDidTerminate() {
        // The page is gone. Save what we last knew of it, then load the same day again once the new page is ready.
        guard let e = editor else { return }
        e.processDied()
        keepIfUnsaved(e)
        let fresh = DayEditor(day: e.day, model: self)
        editor = fresh
        activate(fresh, replacing: nil, focus: false)
    }

    func editorLoadFailed(_ message: String) { editorProblem = message }
    func editorNotice(_ message: String) {
        pageNotice = message
        DispatchQueue.main.asyncAfter(deadline: .now() + 6) { [weak self] in if self?.pageNotice == message { self?.pageNotice = nil } }
    }

    var editorAssetStore: AssetStore? { store.assets }
    var editorUploadContext: (day: String, token: String)? {
        guard let e = editor, e.loaded else { return nil }
        return (e.day, e.token)
    }
}
