// MainView.swift - NavigationSplitView: sidebar + detail routing, the unified toolbar (previous day, Today, next day, Go to
// date), sheets.
import SwiftUI

struct MainView: View {
    @ObservedObject var model: AppModel
    @Environment(\.openWindow) private var openWindow

    private var onDay: Bool { if case .day = model.selection { return true }; return false }

    var body: some View {
        NavigationSplitView {
            SidebarView(model: model)
                .navigationSplitViewColumnWidth(min: 220, ideal: 240, max: 280)
        } detail: {
            DetailRouter(model: model)
                .background(Theme.bg.ignoresSafeArea())
                .toolbar {
                    ToolbarItemGroup(placement: .navigation) {
                        if onDay {
                            Button { model.step(-1) } label: { Image(systemName: "chevron.left") }
                                .help("Previous day (⌘[)").accessibilityLabel("Previous day").disabled(!model.canStep(-1))
                            Button("Today") { model.openToday() }
                                .help("Go to today (⌘T)").accessibilityLabel("Today").disabled(model.selection == .day(model.today))
                            Button { model.step(1) } label: { Image(systemName: "chevron.right") }
                                .help("Next day (⌘])").accessibilityLabel("Next day").disabled(!model.canStep(1))
                        }
                        GoToDateButton(model: model)
                    }
                }
        }
        .frame(minWidth: 860, minHeight: 600)
        .background(WindowChrome())
        .background(Button("") { model.openToday() }.keyboardShortcut("1").opacity(0).accessibilityHidden(true))   // ⌘1: Today, as in the sidebar
        .onAppear { model.openWindowAction = { openWindow(id: "main") } }
        .sheet(item: $model.sheet) { sheet in
            switch sheet {
            case .skip(let day): SkipSheet(model: model, day: day)
            case .skipMany(let days): SkipSheet(model: model, day: days.first ?? model.today, days: days)
            case .onboarding: OnboardingView(model: model)
            case .whatsNew: WhatsNewSheet(model: model)
            case .restore(let day): RestoreVersionSheet(model: model, day: day)
            case .conflicts: ConflictSheet(model: model)
            }
        }
    }
}

/// Hides the window title next to the toolbar (the page shows its own date). The title string stays "Gloamlog", which is
/// how the app finds its window. Same effect as `.windowToolbarStyle(.unified(showsTitle: false))` on the scene.
struct WindowChrome: NSViewRepresentable {
    func makeNSView(context: Context) -> NSView { let v = NSView(); DispatchQueue.main.async { v.window?.titleVisibility = .hidden }; return v }
    func updateNSView(_ v: NSView, context: Context) { DispatchQueue.main.async { v.window?.titleVisibility = .hidden } }
}

struct DetailRouter: View {
    @ObservedObject var model: AppModel
    var body: some View {
        switch model.selection {
        case .day:
            if model.editor.downloading { DownloadingPage(model: model, editor: model.editor) }     // M5b: the day is still in the cloud
            else { DayPage(model: model, editor: model.editor) }
        case .catchUp: CatchUpView(model: model)
        case .week: WeeklyReviewView(model: model)
        case .search: SearchView(model: model)
        }
    }
}

struct WhatsNewSheet: View {
    @ObservedObject var model: AppModel
    /// What changed since 0.3, in the app's own plain voice. The Jot line names the shortcut the person really has.
    private static let moreLines = ["Review any week, month, year or range, then copy it, save a PDF or export it with its images (⇧⌘E).",
                                    "Reminders still arrive when Gloamlog is closed (Settings > Reminders)."]
    private var lines: [String] {
        let jot = model.settings.capture.hotKey.map { "Jot a note from any app with \($0.display). It lands under Jots on today's page." }
            ?? "Jot a note from the menu bar. It lands under Jots on today's page."
        return ["Open any day from the calendar, or press ⇧⌘T and type a date.",
                "Catch up lists the days you haven't logged. Skip the ones you didn't work.",
                jot] + WhatsNewSheet.moreLines + [
                "Settings (⌘,) holds your reminders, page template, storage and shortcuts."]
    }
    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            Text("What's new in 0.6").font(Theme.font(22, .semibold)).foregroundColor(Theme.textPrimary).accessibilityAddTraits(.isHeader)
            VStack(alignment: .leading, spacing: Theme.s2) {
                ForEach(lines, id: \.self) { s in
                    HStack(alignment: .firstTextBaseline, spacing: Theme.s2) {
                        Image(systemName: "checkmark").font(Theme.font(11, .bold)).foregroundColor(Theme.accent).accessibilityHidden(true)
                        Text(s).font(Theme.font(13)).foregroundColor(Theme.textPrimary).fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
            HStack {
                Spacer()
                Button("Take a look at Settings") { done(); model.openSettings() }
                    .buttonStyle(SecondaryButtonStyle())
                Button("Done") { done() }.buttonStyle(PrimaryButtonStyle()).keyboardShortcut(.defaultAction)
            }
        }
        .padding(Theme.s8).frame(width: 460).background(Theme.surface)
    }
    private func done() { model.defaults.set(true, forKey: AppModel.whatsNewKey); model.sheet = nil }
}
