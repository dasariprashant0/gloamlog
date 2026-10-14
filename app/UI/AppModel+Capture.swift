// AppModel+Capture.swift - quick capture ("Jot") wired into the app (M2, task U3): the journal and the single writer, which
// open page a note goes to, when waiting notes are retried, the global shortcut, and the panel.
//
// The rule that makes capture safe: a note is durable in the journal (fsynced) BEFORE anything else happens to it. After that it
// is routed by what is open (Core/Capture/DayWriter.swift): the live editor when today's page is open, the file when it is
// not, and it simply waits when neither can take it. The journal is retried when the folder returns, when the app comes
// forward, after a wake, after a folder change, and when a page finishes loading, so a note is never lost and never added twice.
import SwiftUI
import AppKit
import Carbon.HIToolbox

/// Why the Jot shortcut is not working (shown in Settings > Shortcuts).
struct HotKeyFailure: Equatable {
    enum Reason: Equatable { case taken, system, notAllowed, other(OSStatus) }
    var spec: HotKeySpec
    var reason: Reason
    var message: String {
        switch reason {
        case .taken: return "\(spec.display) is already used by another app."
        case .system: return "\(spec.display) is already used by macOS."
        case .notAllowed: return "\(spec.display) needs ⌃ or ⌘, so it can't clash with typing."
        case .other(let s): return "\(spec.display) couldn't be set up (error \(s))."
        }
    }
}

// MARK: - which page takes a note

extension AppModel: EditorLookup {
    /// The page open for `day`: the current editor (even while it is still loading), or a page the user left whose text could
    /// not be written yet (an orphan), so a note never goes to the file underneath a page that holds unsaved text. nil sends the
    /// note to the file.
    func page(for day: String) -> PageHandle? {
        if let e = editor, e.day == day, e.acceptsJots { return e }
        return orphans.first { $0.day == day && $0.acceptsJots }
    }
}

extension AppModel {
    // MARK: wiring

    /// The writer, built over the current store. Called once from init and again whenever the store is replaced.
    func installCapture() {
        rebuildWriter()
        applyJotsRules()
    }

    func rebuildWriter() {
        writer = DayWriter(store: store, journal: journal, pages: self,
                           prefs: { [weak self] in self?.settings.capture ?? CapturePrefs() },
                           clock: { [weak self] in self?.clock() ?? Date() },
                           calendar: { [weak self] in self?.cal ?? Calendar.current })
    }

    /// `store` was replaced (another log folder): the writer follows, and the notes that were waiting try the new folder.
    func storeReplaced() {
        guard writer != nil else { return }                  // the very first assignment is part of init
        rebuildWriter()
        applyJotsRules()
        replayCaptureSoon()
        DispatchQueue.global(qos: .utility).async { [store = self.store] in try? store.warmSearch() }   // the new folder's search cache
    }

    /// The Jots heading and whether jots count toward "logged" are process-wide rules in the core (LogStore.jotsRules).
    func applyJotsRules() {
        LogStore.jotsRules = (heading: settings.capture.jotsHeading, countTowardLogged: settings.capture.jotsCountTowardLogged)
    }

    /// Launch: pick up notes a previous run left behind (the writer compacts the journal on its first replay), watch for the moments
    /// to retry, and (the real app only) register the global shortcut. A harness never registers the owner's real shortcut: it passes `realApp: false`.
    func startCapture(realApp: Bool) {
        refreshNotesWaiting()
        let nc = NotificationCenter.default, ws = NSWorkspace.shared.notificationCenter
        nc.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { [weak self] _ in self?.replayCapture(); self?.drainInbox() }
        ws.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { [weak self] _ in
            DispatchQueue.main.asyncAfter(deadline: .now() + 5) { self?.replayCapture() }       // let the disk or network volume settle
        }
        if realApp {
            installHotKeys(HotKeyCenter.shared)
            Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in self?.drainInbox() }
        }
        replayCaptureSoon()
        DispatchQueue.main.async { [weak self] in self?.drainInbox() }
    }

    /// Entries other programs left in the inbox (Claude Code's `log_work`) become jots, then their files are deleted. The jot is durable
    /// (journal first) before the file goes, so an entry is never lost between the two.
    func drainInbox() {
        guard live else { return }
        for e in Inbox.pending() {
            _ = jot(text: e.text, source: "claude")
            try? FileManager.default.removeItem(at: e.url)
        }
    }

    /// Makes `center` the model's global shortcut: its press opens the panel, and it holds the shortcut from Settings. The real
    /// app passes the shared center; a check passes its own, so it never touches the owner's real combination.
    func installHotKeys(_ center: HotKeyCenter) {
        hotKeys = center
        center.onPressed = { [weak self] in self?.showJotPanel(source: "hotkey") }
        applyHotKey()
    }

    // MARK: capturing

    /// Adds a note to today's page (any app, any state of the page). Returns after the note is durable in the journal; nil when
    /// nothing is left of the text (empty, only to-do brackets) or the journal itself could not be written.
    @discardableResult
    func jot(text: String, source: String) -> CaptureReceipt? {
        let receipt = writer.capture(text: text, source: source)
        refreshNotesWaiting()
        if receipt != nil {
            let day = DayKey.string(clock(), cal)
            if page(for: day) == nil { fileChangedUnderneath(day) }          // nobody has the day open: the note went (or tried to go) to the file
        }
        return receipt
    }

    /// A note was written straight into a day's file: that day's status follows at once, and a page that shows the day (a skipped day
    /// that the note just turned into a page, a raw-text page) reloads instead of waiting for the next 30 s check.
    func fileChangedUnderneath(_ day: String) {
        didSave(day: day)
        if let e = editor, e.day == day { e.checkExternalChange() }
    }

    /// `jot`, summarised for the panel: where the note stands now.
    func jotOutcome(text: String, source: String) -> JotOutcome {
        guard let r = jot(text: text, source: source) else { return .failed }
        if r.waiting == 0 { return .added }
        // Waiting is normal while an open page autosaves (under a second). It is "kept on this Mac" only when the folder is
        // unavailable, or no page is open to take the note and the file did not.
        if (try? store.checkFolder()) == nil { return .kept }
        return page(for: DayKey.string(clock(), cal)) != nil ? .added : .kept
    }

    func refreshNotesWaiting() {
        guard writer != nil else { return }
        let n = writer.waiting
        if n != notesWaiting { notesWaiting = n }
    }

    // MARK: retrying

    /// Delivers whatever is waiting. Safe to call at any time: a note already in a file is acknowledged, never added twice.
    func replayCapture() {
        guard writer != nil else { return }
        refreshNotesWaiting()
        guard notesWaiting > 0 else { return }
        let days = Set(journal.pending().map { $0.day })
        if let e = editor, e.loaded { writer.acknowledgeSaved(day: e.day, body: e.diskBody ?? "") }   // lines the open page's file already holds
        writer.replayJournal()
        refreshNotesWaiting()
        for d in days where page(for: d) == nil { fileChangedUnderneath(d) }        // notes written straight to a file change that day
    }

    func replayCaptureSoon() { DispatchQueue.main.async { [weak self] in self?.replayCapture() } }

    /// A page finished loading: notes that were waiting for it can go in now.
    func captureEditorLoaded(_ e: DayEditor) {
        guard live, writer != nil else { return }
        replayCaptureSoon()
    }

    /// Called after every successful page save: notes whose line is now in the file are done.
    func captureSaved(day: String, body: String) {
        guard writer != nil, notesWaiting > 0 else { return }
        writer.acknowledgeSaved(day: day, body: body)
        refreshNotesWaiting()
    }

    var notesWaitingLabel: String { "\(Fmt.plural(notesWaiting, "jot")) waiting" }

    // MARK: the panel

    func showJotPanel(source: String) { jotPanel.show(source: source) }

    // MARK: the global shortcut

    /// Registers (or removes) the shortcut from Settings. Called at launch and whenever it changes.
    func applyHotKey() {
        guard let hk = hotKeys else { return }
        guard let spec = settings.capture.hotKey else { hk.unregister(); hotKeyFailure = nil; return }
        if hk.registered == spec { hotKeyFailure = nil; return }
        if let f = hotKeyProblem(spec) { hotKeyFailure = f; return }
        let status = hk.register(spec)
        hotKeyFailure = status == noErr ? nil : HotKeyFailure(spec: spec, reason: status == HotKeyCenter.existsStatus ? .taken : .other(status))
    }

    private func hotKeyProblem(_ spec: HotKeySpec) -> HotKeyFailure? {
        if !HotKeyCenter.isAllowed(spec) { return HotKeyFailure(spec: spec, reason: .notAllowed) }
        if HotKeyCenter.isSystemShortcut(spec) { return HotKeyFailure(spec: spec, reason: .system) }
        return nil
    }

    /// The recorder's door: nil = Off. A shortcut is stored only after the system accepted it, so a refused combination never
    /// replaces the one that works; the reason is published for Settings to show. Returns the failure, nil on success.
    @discardableResult
    func setJotHotKey(_ spec: HotKeySpec?) -> HotKeyFailure? {
        guard let spec = spec else { settings.capture.hotKey = nil; hotKeyFailure = nil; return nil }      // Off: settingsChanged unregisters it
        if let hk = hotKeys {
            if let f = hotKeyProblem(spec) { hotKeyFailure = f; return f }
            let status = hk.register(spec)                                  // the old registration stays until this one succeeds
            if status != noErr {
                let f = HotKeyFailure(spec: spec, reason: status == HotKeyCenter.existsStatus ? .taken : .other(status))
                hotKeyFailure = f; return f
            }
        }
        hotKeyFailure = nil
        settings.capture.hotKey = spec
        return nil
    }
}
