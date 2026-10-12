// PageView.swift - the day page: one centred column, no cards, no save bar. Slim notices, a header (date, streak chip,
// word progress, "..." menu), an optional "From yesterday" strip, then the editor filling the rest of the pane.
import SwiftUI

/// Centred reading column: 720pt of content with the 48pt page padding the editor page also uses (816 total).
struct Column<Content: View>: View {
    var top: CGFloat = Theme.s10
    var bottom: CGFloat = Theme.s6
    @ViewBuilder let content: () -> Content
    var body: some View {
        VStack(alignment: .leading, spacing: 0, content: content)
            .frame(maxWidth: Theme.columnMax, alignment: .leading)
            .padding(.horizontal, Theme.s12).padding(.top, top).padding(.bottom, bottom)
            .frame(maxWidth: .infinity)
    }
}

struct DayPage: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    @Environment(\.snapshotMode) private var snapshot

    var body: some View {
        VStack(spacing: 0) {
            Column(top: 28, bottom: 0) {
                if let s = model.session { SessionBar(model: model, session: s).padding(.bottom, Theme.s4) }
                SyncBanners(model: model)                               // M5b: "1 conflict in ~/Gloamlog" (nothing while there is none)
                PageNotices(model: model, editor: editor)
                PageHeader(model: model, editor: editor)
                if editor.showsEditor { FromYesterdayStrip(model: model, editor: editor) }
                if editor.showsEditor { SuggestionsStrip(model: model, editor: editor) }
            }
            if editor.showsEditor {
                if snapshot {
                    SnapshotEditorStub(markdown: editor.initialBody, store: model.store.assets)
                } else {
                    EditorWebView(bridge: model.bridge).padding(.top, Theme.s2)
                        .accessibilityLabel("Page for \(model.spokenDate(editor.day))")
                }
            } else {
                Column(top: Theme.s6, bottom: Theme.s6) { SkippedBody(model: model, editor: editor) }
                Spacer(minLength: 0)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Theme.bg)
    }
}

/// Stand-in for the web editor in snapshots only: static blocks drawn from the page's markdown.
struct SnapshotEditorStub: View {
    let markdown: String
    let store: AssetStore
    var body: some View {
        ScrollView {
            MarkdownBlocksView(blocks: MDParse.blocks(markdown), store: store, size: 16)
                .frame(maxWidth: Theme.columnMax, alignment: .leading)
                .padding(.horizontal, Theme.s12).padding(.top, Theme.s2)
                .frame(maxWidth: .infinity)
        }
    }
}

// MARK: - notices

/// A single quiet line: text on the left, text buttons on the right, optional dismiss. Fill only, no border, no icon.
struct SlimNotice<Actions: View>: View {
    let text: String
    var tone: Color = Theme.textSecondary
    var onDismiss: (() -> Void)?
    @ViewBuilder let actions: () -> Actions
    var body: some View {
        HStack(spacing: Theme.s3) {
            Text(text).font(Theme.font(13)).foregroundColor(tone).lineLimit(2).fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: Theme.s2)
            actions()
            if let d = onDismiss {
                Button(action: d) {
                    Image(systemName: "xmark").font(Theme.font(10, .semibold)).foregroundColor(Theme.textTertiary).frame(width: 22, height: 22)
                }.buttonStyle(.plain).help("Dismiss").accessibilityLabel("Dismiss")
            }
        }
        .padding(.vertical, 5).padding(.leading, Theme.s3).padding(.trailing, d(onDismiss))
        .frame(minHeight: 32)
        .background(Theme.rect(8).fill(Theme.sidebar))
        .accessibilityElement(children: .contain)
    }
    private func d(_ f: (() -> Void)?) -> CGFloat { f == nil ? Theme.s3 : 6 }
}

struct PageNotices: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    private var showNag: Bool {
        editor.day == model.today && editor.showsEditor && model.isDue && model.nagDismissedDay != model.today
    }
    /// Once a day on Today: earlier days are unlogged. Quiet, and never while the reminder or a catch-up session shows.
    private var showCatch: Bool {
        editor.day == model.today && editor.showsEditor && model.catchNoticeVisible && !showNag && model.session == nil
    }
    var body: some View {
        let any = model.folderProblem != nil || editor.errorText != nil || !model.orphans.isEmpty || model.editorProblem != nil || editor.loadFailed || editor.rawTextOnly || showNag || showCatch || rollover
            || editor.externalNotice || editor.backupNotice != nil || model.pageNotice != nil
        VStack(spacing: Theme.s2) {
            if let p = model.folderProblem { FolderBanner(model: model, problem: p, editor: editor) }
            else if let e = editor.errorText {
                Banner(.error, "Couldn't save this page.", detail: "\(e) Your text is still here. Try again, or choose another folder.") {
                    Button("Try again") { _ = editor.write() }.buttonStyle(TextButtonStyle())
                    Button("Copy my text") { editor.copyLatest() }.buttonStyle(TextButtonStyle())
                }
            }
            if !model.orphans.isEmpty {
                let days = model.orphans.map { model.shortDate($0.day) }.joined(separator: ", ")
                Banner(.error, "A page you left couldn't be saved yet.",
                       detail: "\(days). Gloamlog keeps its text and tries again. Copy it to be safe.") {
                    Button("Try again") { model.retryOrphans() }.buttonStyle(TextButtonStyle())
                    Button("Copy my text") { model.copyUnsavedText() }.buttonStyle(TextButtonStyle())
                }
            }
            if let p = model.editorProblem {
                Banner(.error, "The editor isn't available.", detail: p)
            }
            if editor.rawTextOnly {
                Banner(.warning, "This page can't be edited safely.",
                       detail: "Part of it uses markdown the editor can't round-trip, so its raw text is shown read-only. The file is untouched.") {
                    Button("Show in Finder") { model.revealPageInFinder() }.buttonStyle(TextButtonStyle())
                }
            }
            if editor.loadFailed && editor.showsEditor {
                Banner(.error, "This page couldn't be opened for editing.", detail: "Your file is untouched. Try again, or quit and reopen Gloamlog.") {
                    Button("Try again") { model.openDay(editor.day, force: true) }.buttonStyle(TextButtonStyle())
                }
            }
            if showNag {
                let strict = model.settings.mode == .strict
                SlimNotice(text: strict ? "It's \(Fmt.time(model.now)). Time to write up today." : "Today isn't logged yet.",
                           onDismiss: { model.nagDismissedDay = model.today }) {
                    if strict && model.planner.showSkipHint { Text("Not today?").font(Theme.font(13)).foregroundColor(Theme.textSecondary) }
                    Button(model.snoozeLabel) { model.snooze() }.buttonStyle(TextButtonStyle()).disabled(!model.canSnooze)
                    Button("Skip day") { model.requestSkip(day: model.today) }.buttonStyle(TextButtonStyle())
                }.accessibilityLabel("Reminder")
            }
            if showCatch {
                if let d = model.singleUnloggedYesterday {
                    SlimNotice(text: "You haven't logged \(model.shortDate(d)).", onDismiss: { model.dismissCatchNotice() }) {
                        Button("Write it") { model.select(.day(d)) }.buttonStyle(TextButtonStyle())
                        Button("Skip day") { model.requestSkip(day: d) }.buttonStyle(TextButtonStyle())
                    }.accessibilityLabel("Catch up")
                } else {
                    SlimNotice(text: model.catchUpLabel, onDismiss: { model.dismissCatchNotice() }) {
                        Button("Catch up") { model.select(.catchUp) }.buttonStyle(TextButtonStyle())
                        Button("Later") { model.dismissCatchNotice() }.buttonStyle(TextButtonStyle(color: Theme.textSecondary))
                    }.accessibilityLabel("Catch up")
                }
            }
            if rollover {
                SlimNotice(text: "It's now \(DayKey.format(model.today, "EEEE, d MMMM", model.cal)). You're still writing \(DayKey.format(editor.day, "EEEE, d MMMM", model.cal)).",
                           onDismiss: { model.rolloverDismissed = true }) {
                    Button("Start today's page") { model.openToday() }.buttonStyle(TextButtonStyle())
                }
            }
            if editor.externalNotice {
                SlimNotice(text: "This page changed elsewhere. Your version here replaces it when it saves; the other one stays under Restore previous version.",
                           onDismiss: { editor.externalNotice = false }) { EmptyView() }
            }
            if let b = editor.backupNotice {
                SlimNotice(text: "Earlier versions couldn't be kept: \(b) This page was saved.", onDismiss: { editor.backupNotice = nil }) { EmptyView() }
            }
            if let n = model.pageNotice { SlimNotice(text: n, onDismiss: { model.pageNotice = nil }) { EmptyView() } }
        }
        .padding(.bottom, any ? Theme.s4 : 0)
    }
    private var rollover: Bool { editor.openedAsToday && editor.day != model.today && !model.rolloverDismissed }
}

struct FolderBanner: View {
    @ObservedObject var model: AppModel
    let problem: FolderProblem
    @ObservedObject var editor: DayEditor
    var body: some View {
        switch problem {
        case .missing(let p):
            Banner(.warning, "Your log folder can't be found.",
                   detail: "\"\((p as NSString).abbreviatingWithTildeInPath)\" isn't there any more. It may have been moved or an external drive is disconnected. What you type stays on screen.") {
                Button("Choose folder…") { model.chooseFolder() }.buttonStyle(TextButtonStyle())
                Button("Try again") { model.retryFolder() }.buttonStyle(TextButtonStyle())
                Button("Create it again") { model.recreateFolder() }.buttonStyle(TextButtonStyle())
            }
        case .notWritable(let p):
            Banner(.error, "Gloamlog can't save to this folder.",
                   detail: "You may not have permission to write to \"\((p as NSString).lastPathComponent)\". What you type stays on screen.") {
                Button("Choose folder…") { model.chooseFolder() }.buttonStyle(TextButtonStyle())
                Button("Try again") { model.retryFolder() }.buttonStyle(TextButtonStyle())
                Button("Copy my text") { editor.copyLatest() }.buttonStyle(TextButtonStyle())
            }
        }
    }
}

struct SkippedBody: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            Text("You skipped this day. It doesn't affect your streak.").font(Theme.font(15)).foregroundColor(Theme.textPrimary)
            HStack(spacing: Theme.s3) {
                Button("Write a log anyway") { model.startWritingAnyway() }.buttonStyle(SecondaryButtonStyle())
                Button("Undo skip") { model.unskip(editor.day) }.buttonStyle(SecondaryButtonStyle())
            }
        }
    }
}
