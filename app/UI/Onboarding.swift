// Onboarding.swift - first-run sheet, 4 steps (the last, Claude Code, is optional). Everything is pre-filled; the login item is opt-in (unticked).
//   1 Welcome + the page template   2 Reminder + words to log a day   3 Folder + log start (existing logs) + login item
// "Run setup again" (Settings > About) opens the same sheet. It starts from the current settings and changes only what is
// edited: a folder switch goes through the usual copy / move dialog, a template of your own stays unless you pick a card.
import SwiftUI
import AppKit

final class OnboardingState: ObservableObject {
    enum FolderChoice: Hashable { case local, icloud, custom }
    /// Day pages already in the chosen folder (shape-checked file names only; nothing is read or changed).
    struct FoundLogs: Equatable { var count: Int; var first: String; var last: String }

    @Published var step = 1
    @Published var template: TemplatePreset? = .dailyReview          // nil: the user's own template, kept on a re-run
    @Published var templateTouched = false
    @Published var minutes: Int
    @Published var days: Set<Int>
    @Published var mode: ReminderMode
    @Published var minWords: Int
    @Published var choice = FolderChoice.local
    @Published var customURL: URL?
    @Published var login = false
    @Published var logStart: Date?                                   // nil: follow the first log
    @Published var found: FoundLogs?
    var rerun = false

    init(_ s: Settings) { minutes = s.reminderMinutes; days = s.weekdays; mode = s.mode; minWords = s.minWords }

    static var iCloudFolder: URL? {
        let base = URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent("Library/Mobile Documents/com~apple~CloudDocs")
        return FileManager.default.fileExists(atPath: base.path) ? base.appendingPathComponent("Gloamlog") : nil
    }

    /// Shows the current settings. On a re-run (already set up) the folder, the template card and the login item start
    /// where they are, so nothing changes until a step is edited.
    func configure(from s: Settings, loginEnabled: Bool) {
        minutes = s.reminderMinutes; days = s.weekdays; mode = s.mode; minWords = s.minWords
        guard s.onboarded else { return }
        rerun = true
        template = TemplatePreset.matching(template: s.template)
        login = loginEnabled
        let here = s.storageFolder.standardizedFileURL.path
        if here == Settings.defaultFolder.standardizedFileURL.path { choice = .local }
        else if let i = OnboardingState.iCloudFolder, i.standardizedFileURL.path == here { choice = .icloud }
        else { choice = .custom; customURL = s.storageFolder }
    }

    var chosenFolder: URL {
        switch choice {
        case .local: return Settings.defaultFolder
        case .icloud: return OnboardingState.iCloudFolder ?? Settings.defaultFolder
        case .custom: return customURL ?? Settings.defaultFolder
        }
    }

    /// Looks for day pages in the chosen folder, so a person with existing logs can say where the log starts.
    func refreshFound() {
        let names = ((try? FileManager.default.contentsOfDirectory(atPath: chosenFolder.path)) ?? [])
            .filter { $0.hasSuffix(".md") }.map { String($0.dropLast(3)) }.filter { DayKey.isWellFormed($0) }.sorted()
        if let f = names.first, let l = names.last { found = FoundLogs(count: names.count, first: f, last: l) }
        else { found = nil; logStart = nil }
    }

    /// The settings after this run. First run: every answer applies. Run again: only what was edited (the folder is
    /// switched afterwards through `changeFolder`, which asks whether to copy or move the logs).
    func applied(to current: Settings, useDefaults: Bool, calendar: Calendar) -> Settings {
        var s = current
        if let t = template, !rerun || templateTouched { s.template = t.markdown; s.carryOverHeadings = t.carryOverHeadings }
        if !useDefaults {
            s.reminderMinutes = minutes; s.weekdays = days.isEmpty ? [2, 3, 4, 5, 6] : days; s.mode = mode
            s.minWords = minWords
            if !rerun { s.storageFolder = chosenFolder }
            if let d = logStart, found != nil { s.logStartDate = DayKey.string(d, calendar) }
        }
        s.onboarded = true
        return s
    }
}

struct OnboardingView: View {
    @ObservedObject var model: AppModel
    @StateObject private var st: OnboardingState
    @ObservedObject private var claudeSetup: ClaudeSetup
    @Environment(\.accessibilityReduceMotion) private var reduce

    /// `state` lets a test or screenshot start from prepared answers (a folder other than the real one).
    init(model: AppModel, startStep: Int = 1, state: OnboardingState? = nil) {
        self.model = model
        _st = StateObject(wrappedValue: { let s = state ?? OnboardingState(Settings()); s.step = startStep; return s }())
        _claudeSetup = ObservedObject(wrappedValue: model.claudeSetup)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                HStack(spacing: 6) {
                    ForEach(1...4, id: \.self) { i in Capsule().fill(i <= st.step ? Theme.accent : Theme.border).frame(width: 28, height: 4) }
                }.accessibilityElement(children: .ignore).accessibilityLabel("Step \(st.step) of 4")
                Spacer()
                if st.step == 1 {
                    Button(st.rerun ? "Cancel" : "Skip setup") { if st.rerun { model.sheet = nil } else { finish(useDefaults: true) } }
                        .buttonStyle(TextButtonStyle(color: Theme.textSecondary))
                }
            }
            Group {
                switch st.step {
                case 1: welcome
                case 2: reminder
                case 3: folder
                default: claude
                }
            }.padding(.top, Theme.s6).frame(maxWidth: .infinity, alignment: .leading)
            Spacer(minLength: Theme.s4)
            HStack {
                if st.step > 1 { Button("Back") { go(st.step - 1) }.buttonStyle(TextButtonStyle(color: Theme.textSecondary)) }
                Spacer()
                Button(st.step == 4 ? (st.rerun ? "Done" : "Start writing") : "Continue") { next() }
                    .buttonStyle(PrimaryButtonStyle()).keyboardShortcut(.defaultAction)
            }
        }
        .padding(Theme.s10).frame(width: 600, height: 520).background(Theme.surface)
        .interactiveDismissDisabled()
        .onAppear { st.configure(from: model.settings, loginEnabled: model.loginEnabled); st.refreshFound(); if model.live { claudeSetup.refresh() } }
    }

    private func go(_ n: Int) { withAnimation(Theme.animation(.easeInOut(duration: 0.2), reduce: reduce)) { st.step = n } }
    private func next() {
        if st.step == 2 { Notifier.shared.requestAuthorization { _ in Notifier.shared.refreshState { s in model.notifState = s } } }
        if st.step < 4 { go(st.step + 1) } else { finish(useDefaults: false) }
    }

    private func headline(_ t: String) -> some View {
        Text(t).font(Theme.font(28, .semibold)).tracking(-0.4).foregroundColor(Theme.textPrimary).accessibilityAddTraits(.isHeader)
    }
    private func note(_ t: String) -> some View {
        Text(t).font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
    }

    // MARK: 1 welcome + template
    private var welcome: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            headline(st.rerun ? "Your page" : "Write up your day, every day.")
            Text("Each day gets one page. Write freely, with headings, lists, to-dos and images, or start from a few prompts.")
                .font(Theme.font(15)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            Text("Start your page with").font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary).padding(.top, Theme.s2)
            HStack(alignment: .top, spacing: Theme.s3) {
                ForEach(TemplatePreset.allCases) { p in templateCard(p) }
            }
            note(st.rerun
                 ? (st.template == nil ? "Your own template stays as it is unless you pick one of these. Pages you already wrote are never changed."
                                       : "Pages you already wrote are never changed. The template only fills new days.")
                 : "Your pages are plain markdown files you own. Nothing leaves your Mac. You can change the template any time in Settings (⌘,).")
        }
    }

    private func templateCard(_ p: TemplatePreset) -> some View {
        let on = st.template == p
        return Button { st.template = p; st.templateTouched = true } label: {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Image(systemName: on ? "largecircle.fill.circle" : "circle").foregroundColor(on ? Theme.accent : Theme.textSecondary)
                    Text(p.title).font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
                }
                Text(p.detail).font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 2)
                if p.preview.isEmpty {
                    Text("(empty)").font(.system(size: 11, design: .monospaced)).foregroundColor(Theme.textTertiary)
                } else {
                    VStack(alignment: .leading, spacing: 1) {
                        ForEach(p.preview.prefix(5), id: \.self) { l in
                            Text("## \(l)").font(.system(size: 11, design: .monospaced)).foregroundColor(Theme.textSecondary).lineLimit(1)
                        }
                    }
                }
            }
            .padding(Theme.s3).frame(maxWidth: .infinity, minHeight: 150, alignment: .topLeading)
            .background(Theme.rect(Theme.radiusLg).fill(on ? Theme.accentTint : Theme.surface))
            .overlay(Theme.rect(Theme.radiusLg).stroke(on ? Theme.accent : Theme.borderStrong, lineWidth: on ? 1.5 : 1))
        }
        .buttonStyle(.plain).accessibilityLabel("\(p.title). \(p.detail)").accessibilityAddTraits(on ? .isSelected : [])
    }

    // MARK: 2 reminder
    private var reminder: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            headline("When should we ask?")
            HStack(spacing: Theme.s4) {
                DatePicker("Time", selection: Binding(
                    get: { model.cal.date(bySettingHour: st.minutes / 60, minute: st.minutes % 60, second: 0, of: model.now) ?? model.now },
                    set: { d in let c = model.cal.dateComponents([.hour, .minute], from: d); st.minutes = (c.hour ?? 16) * 60 + (c.minute ?? 55) }),
                           displayedComponents: .hourAndMinute).fixedSize()
                WeekdayToggles(days: Binding(get: { st.days }, set: { st.days = $0 }), cal: model.cal)
            }
            Text("How firm?").font(Theme.font(15, .semibold)).foregroundColor(Theme.textPrimary).padding(.top, Theme.s2)
            HStack(spacing: Theme.s3) {
                modeCard(.strict, "Strict", "Comes to the front until you save or skip the day.")
                modeCard(.gentle, "Gentle", "One notification. You decide when.")
            }
            WordsRuleRow(minWords: $st.minWords).padding(.top, Theme.s2)
            note("A day counts once the page has this many words. You can always skip a day or snooze. Change all of this later in Settings (⌘,).")
        }
    }

    private func modeCard(_ m: ReminderMode, _ title: String, _ body: String) -> some View {
        let on = st.mode == m
        return Button { st.mode = m } label: {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Image(systemName: on ? "largecircle.fill.circle" : "circle").foregroundColor(on ? Theme.accent : Theme.textSecondary)
                    Text(title).font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
                }
                Text(body).font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            }
            .padding(Theme.s3).frame(maxWidth: .infinity, minHeight: 72, alignment: .topLeading)
            .background(Theme.rect(Theme.radiusLg).fill(on ? Theme.accentTint : Theme.surface))
            .overlay(Theme.rect(Theme.radiusLg).stroke(on ? Theme.accent : Theme.borderStrong, lineWidth: on ? 1.5 : 1))
        }
        .buttonStyle(.plain).accessibilityLabel("\(title). \(body)").accessibilityAddTraits(on ? .isSelected : [])
    }

    // MARK: 3 folder
    private var folder: some View {
        VStack(alignment: .leading, spacing: Theme.s3) {
            headline("Where should logs live?")
            VStack(alignment: .leading, spacing: Theme.s2) {
                Picker("", selection: $st.choice) {
                    Text("On this Mac: ~/Gloamlog").tag(OnboardingState.FolderChoice.local)
                    if OnboardingState.iCloudFolder != nil { Text("iCloud Drive: Gloamlog").tag(OnboardingState.FolderChoice.icloud) }
                    Text(st.customURL.map { "Another folder: " + ($0.path as NSString).abbreviatingWithTildeInPath } ?? "Another folder…").tag(OnboardingState.FolderChoice.custom)
                }.pickerStyle(.radioGroup).labelsHidden()
                .onChange(of: st.choice) { c in if c == .custom && st.customURL == nil { chooseCustom() }; st.refreshFound() }
                if st.choice == .custom { Button("Choose…") { chooseCustom() }.buttonStyle(SecondaryButtonStyle()) }
            }
            if let f = st.found { existingLogs(f) }
            else { note("Pick iCloud Drive or Dropbox to keep logs on every Mac. Images you add live next to your pages in an assets folder.") }
            Toggle("Open Gloamlog at login", isOn: $st.login).toggleStyle(.checkbox)
            note("So the reminder can fire after a restart. Everything in this setup lives in Settings (⌘,), and the gear at the bottom of the sidebar opens it.")
        }
    }

    /// A folder that already has day pages: nothing is touched, and the log can start later than the first page.
    private func existingLogs(_ f: OnboardingState.FoundLogs) -> some View {
        let first = DayKey.date(f.first, model.cal) ?? model.now
        let start = Binding<Date>(get: { st.logStart ?? first }, set: { st.logStart = $0 })
        return VStack(alignment: .leading, spacing: Theme.s2) {
            Text("Found \(Fmt.plural(f.count, "log")), \(DayKey.format(f.first, "d MMM yyyy", model.cal)) to \(DayKey.format(f.last, "d MMM yyyy", model.cal)). Nothing is moved or rewritten.")
                .font(Theme.font(13)).foregroundColor(Theme.textPrimary).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: Theme.s3) {
                Text("Log starts").font(Theme.font(13)).foregroundColor(Theme.textPrimary)
                DatePicker("Log starts", selection: start, in: first...max(model.now, first), displayedComponents: .date)
                    .labelsHidden().datePickerStyle(.field).fixedSize()
                Text("Days before it never count as unlogged.").font(Theme.font(12)).foregroundColor(Theme.textSecondary)
            }
        }
        .padding(Theme.s3).frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.rect(Theme.radiusMd).fill(Theme.sidebar))
        .accessibilityElement(children: .contain)
    }

    private func chooseCustom() {
        let p = NSOpenPanel(); p.canChooseDirectories = true; p.canChooseFiles = false; p.canCreateDirectories = true; p.prompt = "Choose"
        if p.runModal() == .OK, let u = p.url { st.customURL = u; st.refreshFound() } else if st.customURL == nil { st.choice = .local }
    }

    // MARK: 4 claude (optional)
    private var claude: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            headline("Let Claude write your log")
            Text("If you use Claude Code, Gloamlog can give it a small helper. When a piece of work is done, Claude adds a short entry to today's page: the problem, what it did, what is still open. Your Git commits and pull requests can be added too.")
                .font(Theme.font(15)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: Theme.s3) { claudeControl }.padding(.top, Theme.s2)
            if let m = claudeSetup.message { note(m) }
            note("Optional. The button shows exactly what it changes before it does anything, nothing leaves your Mac, and Settings > Sources can undo it. You can skip this and connect later.")
        }
    }

    @ViewBuilder private var claudeControl: some View {
        switch claudeSetup.state {
        case .unknown, .checking:
            ProgressView().controlSize(.small)
        case .working:
            ProgressView().controlSize(.small)
            Text("Connecting…").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
        case .connected:
            Label("Connected to Claude Code", systemImage: "checkmark.circle.fill").font(Theme.font(13)).foregroundColor(Theme.accentText)
        case .moved:
            Button("Reconnect Claude Code") { claudeSetup.connect() }.buttonStyle(SecondaryButtonStyle())
        case .notConnected:
            Button("Connect Claude Code") { claudeSetup.connect() }.buttonStyle(SecondaryButtonStyle())
        case .noClaude:
            Text("Claude Code was not found on this Mac. You can connect it later in Settings > Sources.")
                .font(Theme.font(13)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
        }
    }

    private func finish(useDefaults: Bool) {
        let before = model.settings
        let s = st.applied(to: before, useDefaults: useDefaults, calendar: model.cal)
        if !st.rerun, s.storageFolder != before.storageFolder { model.store = AppModel.makeStore(dir: s.storageFolder, backupDir: model.backupDir) }
        model.settings = s
        if !st.rerun { try? model.store.ensureFolder() }
        if !useDefaults && st.login != (st.rerun ? model.loginEnabled : false) { model.setLogin(st.login) }
        if useDefaults && !st.rerun { Notifier.shared.requestAuthorization { _ in Notifier.shared.refreshState { s in model.notifState = s } } }
        model.defaults.set(true, forKey: AppModel.whatsNewKey)
        model.planner = model.planner.suppressed(until: model.clock().addingTimeInterval(ReminderPlanner.graceAfterLaunch))
        model.sheet = nil
        if st.rerun {
            // The page that is open stays open. A different folder asks whether to copy or move the logs.
            if !useDefaults, st.chosenFolder.standardizedFileURL.path != before.storageFolder.standardizedFileURL.path { model.changeFolder(to: st.chosenFolder) }
        } else {
            model.reload()
            model.openDay(model.today, focus: true, force: true)     // a fresh page with the chosen template, cursor in it
        }
    }
}
