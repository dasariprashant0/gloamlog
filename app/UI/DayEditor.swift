// DayEditor.swift - the state of ONE day's page while it is open (the page text itself lives in the web editor).
// Bound to the day it was opened for: a page opened as "today" stays that day after midnight.
//
// Data-loss rules (docs/EDITOR_CONTRACT.md, "Autosave"):
//  * `loaded` becomes true only after the web editor is READY and the initial setMarkdown for THIS day has completed.
//    Nothing is written before that, and change events are ignored until then.
//  * A day that was merely opened is never written: only a `change` event (a user edit, never a programmatic setMarkdown)
//    or a final read that differs from what was loaded marks the page as edited.
//  * A page with no file and no net change from what was loaded (the template untouched, or typed and deleted again) is
//    never created on disk. Typing even just a heading is a real edit and is saved.
//  * An empty page may delete an existing file only because the user emptied it; the core backs the old text up first.
//  * Quick capture (M2): a note reaches an open page only as ONE editor transaction (appendJot), never as a text replacement, so
//    what the user typed, the caret and the undo history are untouched. A page that shows the template above its jots (a day with
//    only jots) never writes that template back until the user writes something of their own.
import SwiftUI
import AppKit

final class DayEditor: ObservableObject {
    let day: String
    unowned let model: AppModel
    let token = UUID().uuidString
    let openedAsToday: Bool

    @Published private(set) var words = 0                // what the "logged" rule counts (jots only when Settings says so)
    @Published private(set) var jotCount = 0              // list items under the Jots heading
    @Published private(set) var isSkipped: Bool
    @Published private(set) var skipReason: String
    @Published var writingAnyway = false
    @Published private(set) var loaded = false
    @Published private(set) var loadFailed = false
    @Published private(set) var savedFlash = false
    @Published private(set) var latest = ""             // newest markdown known from the editor
    @Published var errorText: String?
    @Published var externalNotice = false
    @Published var backupNotice: String?
    @Published private(set) var rawTextOnly = false      // the editor could not parse the page without loss: raw text, read-only

    private(set) var initialBody: String
    private(set) var diskBody: String?                  // body on disk as a later load() would return it; nil = no log file
    private(set) var userEdited = false
    private var baseline = ""
    private var stable = true
    private var abandoned = false
    private var forceBackupNext = false                 // the file on disk is somebody else's version: back it up for sure
    private var saveWork: DispatchWorkItem?
    private var flashWork: DispatchWorkItem?

    init(day: String, model: AppModel) {
        self.day = day; self.model = model
        openedAsToday = day == model.today
        let doc = (try? model.store.load(day)) ?? nil
        isSkipped = doc?.isSkipped ?? false
        skipReason = doc?.skipReason ?? ""
        diskBody = (doc != nil && doc?.isSkipped == false) ? doc?.body : nil
        // A new (or empty) page starts from the template, in the editor only; a day with only jots shows it above them.
        initialBody = DayEditor.displayBody(for: diskBody, settings: model.settings)
        latest = initialBody
        recount(initialBody)
    }

    /// What the editor is given for a file body: the template for an empty page, the template above the jots for a page that
    /// holds nothing but jots (so the day can be written up), else the body itself. Editor-only: see `writable`.
    static func displayBody(for body: String?, settings: Settings) -> String {
        let b = body ?? ""
        if MarkdownBody.trimBody(b).isEmpty { return settings.template }
        let jots = settings.capture.jotsHeading
        let template = MarkdownBody.trimBody(settings.template)
        if !template.isEmpty, MarkdownBody.isJotsOnly(b, jotsHeading: jots) { return template + "\n\n" + MarkdownBody.trimBody(b) }
        return b
    }

    /// Words (per the logged rule) and jots of `md`.
    private func recount(_ md: String) {
        let prefs = model.settings.capture
        let c = MarkdownBody.counts(in: md, jotsHeading: prefs.jotsHeading)
        words = c.words + (prefs.jotsCountTowardLogged ? c.jotWords : 0)
        jotCount = c.jotCount
    }

    // MARK: derived
    var showsEditor: Bool { !isSkipped || writingAnyway }
    var baselineMarkdown: String { baseline }
    var minWords: Int { model.settings.minWords }
    var isLogged: Bool { words >= max(1, minWords) }

    static func diskForm(_ day: String, _ body: String) -> String {
        MarkdownFormat.parse(day: day, raw: MarkdownFormat.serialize(day: day, body: body)).body
    }

    /// What gets written for `md`: a page that is nothing but empty headings ("##", what select-all + delete leaves
    /// behind) is an emptied page, which the core turns into "no log" (it deletes the file after backing it up).
    static func writable(_ md: String) -> String {
        let lines = md.components(separatedBy: "\n").map { $0.dlTrimmed }.filter { !$0.isEmpty }
        let emptyHeading: (String) -> Bool = { $0.first == "#" && $0.allSatisfy { $0 == "#" } && $0.count <= 6 }
        return lines.allSatisfy(emptyHeading) ? "" : md
    }

    /// `writable` plus the jots rule: a day that shows the template above nothing but its jots writes only the jots, so the
    /// template headings reach the file together with the user's own first words, never before.
    static func writable(_ md: String, settings: Settings) -> String {
        MarkdownBody.stripUntouchedTemplate(writable(md), template: settings.template, jotsHeading: settings.capture.jotsHeading)
    }
    /// What a save writes now.
    var writableLatest: String { DayEditor.writable(latest, settings: model.settings) }

    /// True when a write would change the file (and is allowed to).
    var hasUnsavedEdits: Bool {
        guard loaded, userEdited, !abandoned else { return false }
        let body = writableLatest
        if diskBody == nil && MarkdownBody.trimBody(body).isEmpty { return false }       // no file and nothing to write: same thing
        if diskBody == DayEditor.diskForm(day, body) { return false }
        // No file yet and no net change from what was loaded (typed and deleted again): nothing to create.
        if diskBody == nil && MarkdownBody.trimBody(body) == MarkdownBody.trimBody(DayEditor.writable(baseline, settings: model.settings)) { return false }
        return true
    }

    /// A page that is exactly the template and has never been written: it only holds headings.
    var isPristineTemplate: Bool {
        guard loaded, !userEdited, diskBody == nil, showsEditor else { return false }
        let t = MarkdownBody.trimBody(model.settings.template)
        return !MarkdownBody.headings(inTemplate: t).isEmpty && MarkdownBody.trimBody(initialBody) == t
    }

    /// A day that holds only jots, shown with the template above them and not yet written to: like a pristine template page it
    /// needs a line to type into under each heading (an editor-only trick, not an edit).
    var isTemplateOverJots: Bool {
        guard loaded, !userEdited, showsEditor, let d = diskBody else { return false }
        return MarkdownBody.isJotsOnly(d, jotsHeading: model.settings.capture.jotsHeading) && !MarkdownBody.trimBody(model.settings.template).isEmpty
    }

    // MARK: lifecycle driven by the model
    func didLoad(markdown: String, stable: Bool) {
        baseline = markdown; latest = markdown; self.stable = stable
        userEdited = false; abandoned = false; loadFailed = false; loaded = true
        recount(markdown)
    }
    func markLoadFailed() { loaded = false; loadFailed = true }
    func showRawTextNotice() { if loaded || !rawTextOnly { rawTextOnly = true } }
    func markNotLoaded() { loaded = false }

    /// The page text changed because the USER edited it.
    func userChanged(_ md: String) {
        guard loaded, !abandoned, md != latest else { return }
        latest = md; userEdited = true
        recount(md)
        backupNotice = nil
        scheduleSave()
    }

    /// A change Swift made itself (carry-over, undo) that must still reach the file.
    func programmaticEdit(to md: String) {
        guard loaded, !abandoned else { return }
        latest = md; userEdited = true
        recount(md)
        scheduleSave()
    }

    /// The page is being replaced (day switch, quit, flush). `md` is its final text if the editor could tell.
    func finish(with md: String?) {
        saveWork?.cancel(); saveWork = nil
        guard loaded, !abandoned else { return }
        if let md = md, md != latest {
            // Typed within the page's 300 ms debounce. Trust it only if the serialiser is stable for this page.
            if userEdited || stable { latest = md; userEdited = true; recount(md) }
        }
        _ = write()
    }

    /// Drop everything unsaved (the page was restored/skipped/replaced from disk on purpose).
    func abandon() {
        saveWork?.cancel(); saveWork = nil
        abandoned = true; userEdited = false
    }

    /// The web process died: save what we last knew, then wait for a fresh page. If the text could not be written the
    /// editor stays "loaded" (it is kept as an orphan and retried).
    func processDied() {
        saveWork?.cancel(); saveWork = nil
        _ = write()
        if !hasUnsavedEdits { loaded = false }
    }

    // MARK: autosave
    private func scheduleSave() {
        saveWork?.cancel()
        let w = DispatchWorkItem { [weak self] in if let s = self { s.saveWork = nil; _ = s.write() } }
        saveWork = w
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.6, execute: w)
    }

    func retryIfNeeded() { if errorText != nil && hasUnsavedEdits { _ = write() } }

    @discardableResult
    func write() -> Bool {
        guard hasUnsavedEdits else { return true }
        // The file changed on disk since we last looked (another Mac, another editor): that version must survive. The core
        // throttles backups of ordinary autosaves (minBackupInterval), so a conflicting overwrite forces one.
        let before = readDisk()
        let conflict = before.body != diskBody || before.skipped != isSkipped
        let force = forceBackupNext || conflict
        if conflict { externalNotice = true }
        if force { model.store.minBackupInterval = 0 }
        defer { if force { model.store.minBackupInterval = AppModel.backupInterval } }
        do {
            let outcome = try model.store.save(day: day, body: writableLatest)
            forceBackupNext = false
            backupNotice = outcome.backupWarning
            let disk = readDisk()
            diskBody = disk.body; isSkipped = disk.skipped; skipReason = disk.reason
            if !disk.skipped { writingAnyway = false }
            errorText = nil; externalNotice = false
            flashSaved()
            model.didSave(day: day)
            model.captureSaved(day: day, body: diskBody ?? "")        // notes now in the file are done (acknowledged in the journal)
            return true
        } catch let e as LogError {
            fail(e); return false
        } catch {
            errorText = error.localizedDescription; return false
        }
    }

    private func fail(_ e: LogError) {
        switch e {
        case .folderMissing(let p): model.folderProblem = .missing(p); errorText = "The log folder can't be found."
        case .folderNotWritable(let p): model.folderProblem = .notWritable(p); errorText = "Gloamlog can't save to the log folder."
        case .io(let m): errorText = "Couldn't save. \(m)"
        default: errorText = "Couldn't save this page."
        }
        if model.live { UIAnnounce.say("Couldn't save. Your text is still on screen.", assertive: true) }
    }

    private func flashSaved() {
        savedFlash = true
        flashWork?.cancel()
        let w = DispatchWorkItem { [weak self] in if let s = self { s.savedFlash = false } }
        flashWork = w
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.2, execute: w)
    }

    // MARK: disk
    func readDisk() -> (body: String?, skipped: Bool, reason: String) {
        guard let doc = (try? model.store.load(day)) ?? nil else { return (nil, false, "") }
        return doc.isSkipped ? (nil, true, doc.skipReason) : (doc.body, false, "")
    }

    /// Called on the timer / app activation. Quiet reload when nothing is unsaved, a small notice otherwise.
    func checkExternalChange() {
        guard loaded, !abandoned else { return }
        let disk = readDisk()
        if disk.body == diskBody && disk.skipped == isSkipped { return }
        if hasUnsavedEdits {
            diskBody = disk.body                    // acknowledged: our version replaces theirs at the next save
            forceBackupNext = true                  // ... after the core has copied theirs aside
            externalNotice = true
            return
        }
        if disk.skipped != isSkipped { model.openDay(day, force: true); return }
        guard let b = model.bridgeIfCreated else { return }
        let newBody = DayEditor.displayBody(for: disk.body, settings: model.settings)
        let known = latest
        b.replace(token: token, expecting: known, markdown: newBody, cursor: .keep) { [weak self] normalized in
            guard let self = self else { return }
            self.diskBody = disk.body
            if let n = normalized {
                self.baseline = n; self.latest = n; self.userEdited = false
                self.recount(n)
            } else { self.externalNotice = true }   // the user typed while we looked: keep their version, tell them
        }
    }
}

// MARK: - quick capture
/// The page as the capture writer sees it (Core/Capture/CaptureTypes.swift). A note is added to the LIVE document in one editor
/// transaction, so the user's text, caret and undo history are untouched; the normal autosave then writes it and
/// `captureSaved` acknowledges it in the journal.
extension DayEditor: PageHandle {
    var isLoaded: Bool { loaded }

    /// True while this page can take a note through the editor: it is (or is about to be) loaded and editable. A skipped day, a
    /// page that failed to open and a page shown as raw text are written through the file instead.
    var acceptsJots: Bool { showsEditor && !loadFailed && !rawTextOnly && !abandoned && (loaded || model.editorProblem == nil) }

    func appendJot(_ item: CaptureItem, completion: @escaping (JotAppendResult) -> Void) {
        guard loaded, !abandoned else { completion(.notLoaded); return }
        let heading = model.settings.capture.jotsHeading
        // A page the user has left whose text could not be written (an orphan) is no longer in the web editor: the note joins
        // its text, and goes to the file with it once the folder is back.
        guard model.editor === self else {
            if CapturePlacement.contains(item, in: latest, jotsHeading: heading) { completion(.alreadyPresent); return }
            let next = CapturePlacement.apply(item, to: latest, jotsHeading: heading)
            programmaticEdit(to: next)
            completion(.appended(normalizedMarkdown: next)); return
        }
        guard let b = model.bridgeIfCreated, b.isReady else { completion(.notLoaded); return }
        b.appendToSection(token: token, heading: heading, markdown: CapturePlacement.render(item), unlessPresent: true) { [weak self] outcome in
            guard let self = self else { completion(.stale); return }
            switch outcome {
            case .appended(let md):
                self.programmaticEdit(to: md)             // the editor's own text is the truth; autosave writes it
                completion(.appended(normalizedMarkdown: md))
            case .alreadyPresent: completion(.alreadyPresent)
            case .stale: completion(.stale)
            case .failed: completion(.notLoaded)
            }
        }
    }
}

/// VoiceOver announcements (callers check `model.live` so the snapshot harness never touches NSApp).
enum UIAnnounce {
    static func say(_ text: String, assertive: Bool = false) {
        NSAccessibility.post(element: NSApp as Any, notification: .announcementRequested,
                             userInfo: [.announcement: text,
                                        .priority: (assertive ? NSAccessibilityPriorityLevel.high : NSAccessibilityPriorityLevel.low).rawValue])
    }
}
