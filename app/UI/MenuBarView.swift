// MenuBarView.swift - MenuBarExtra popover (.window style): state, next reminder, a one-line Jot field, streak, last 7 days,
// actions, Settings. The Jot field adds a note to today's page like the global shortcut does (M2, docs/v1/DESIGN_V4.md section 8).
import SwiftUI

extension AppModel {
    var menuIcon: (symbol: String, title: String) {
        if folderProblem != nil { return ("exclamationmark.triangle", "Gloamlog: can't save, folder unavailable") }
        switch todayStatus {
        case .logged: return ("checkmark.circle", "Gloamlog: today logged")
        case .skipped: return ("moon.zzz", "Gloamlog: skipped today")
        case .off: return ("circle.dashed", "Gloamlog: no log needed today")
        default: return isDue ? ("pencil.circle.fill", "Gloamlog: time to write up today") : ("book.closed", "Gloamlog: today not yet logged")
        }
    }
    var menuStateLine: String {
        if folderProblem != nil { return "Saving problem: folder unavailable" }
        switch todayStatus {
        case .logged: return "Logged today ✓"
        case .skipped: let r = skipReasons[today] ?? ""; return "Skipped today" + (r.isEmpty ? "" : " (\(r))")
        case .off: return "Not a workday"
        default: return isDue ? "Due now: \(reminderLabel)" : "Not logged yet"
        }
    }
    /// The line under the state in the popover: when the next reminder is due, and how firm it is.
    var menuReminderLine: String { "Next reminder: \(nextReminderLabel) · \(settings.mode == .strict ? "Strict" : "Gentle")" }
}

struct MenuBarView: View {
    @ObservedObject var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: Theme.s3) {
                VStack(alignment: .leading, spacing: 3) {
                    Text("Today · \(DayKey.format(model.today, "EEE d MMM", model.cal))").font(Theme.font(11)).foregroundColor(Theme.textSecondary)
                    HStack(spacing: 6) {
                        Image(systemName: model.menuIcon.symbol).font(Theme.font(14)).foregroundColor(model.folderProblem != nil ? Theme.danger : (model.todayStatus == .logged ? Theme.accent : Theme.textPrimary))
                            .accessibilityHidden(true)
                        Text(model.menuStateLine).font(Theme.font(14, .semibold)).foregroundColor(Theme.textPrimary)
                    }
                    Text(model.menuReminderLine)
                        .font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
                }
                .accessibilityElement(children: .combine)
                PopoverJotField(model: model)
            }
            .padding(Theme.s3)
            divider
            VStack(alignment: .leading, spacing: Theme.s2) {
                StreakLine(model: model)
                WeekStrip(model: model)
            }.padding(Theme.s3)
            divider
            VStack(spacing: 2) {
                MenuButton(title: "Open Gloamlog", shortcut: "⌘O", primary: true) { open() }.keyboardShortcut("o")
                MenuButton(title: "Jot…", shortcut: model.settings.capture.hotKey?.display) { model.showJotPanel(source: "menu") }
                MenuButton(title: "Copy this week's summary") { model.review.copyThisWeekSummary() }
                if model.isDue { MenuButton(title: model.snoozeLabel, disabled: !model.canSnooze) { model.snooze() } }
                MenuButton(title: model.canSkipToday ? "Skip today…" : "Today is already logged", disabled: !model.canSkipToday) {
                    open(); model.requestSkip(day: model.today)
                }
            }.padding(Theme.s2)
            divider
            VStack(spacing: 2) {
                MenuButton(title: "Settings…", shortcut: "⌘,") { model.openSettings() }
                MenuButton(title: "Quit Gloamlog", shortcut: "⌘Q") { NSApp.terminate(nil) }
            }.padding(Theme.s2)
        }
        .frame(width: 300).background(Theme.surface)
        .onAppear { model.openWindowAction = { openWindow(id: "main") }; model.refreshClock() }
    }

    private var divider: some View { Rectangle().fill(Theme.border).frame(height: 1) }
    private func open() {
        if model.mainWindow == nil { openWindow(id: "main") }
        model.showMainWindow(activate: true); model.openToday()
    }
}

struct MenuButton: View {
    let title: String
    var shortcut: String?
    var primary = false
    var disabled = false
    let action: () -> Void
    var body: some View {
        HoverReader { hovering in
            Button(action: action) {
                HStack {
                    Text(title).font(Theme.font(13, primary ? .semibold : .regular))
                    Spacer()
                    if let s = shortcut { Text(s).font(Theme.font(12)).foregroundColor(primary ? Theme.onAccent.opacity(0.85) : Theme.textSecondary) }
                }
                .foregroundColor(disabled ? Theme.textTertiary : (primary ? Theme.onAccent : Theme.textPrimary))
                .padding(.horizontal, 10).frame(height: 30)
                .background(Theme.rect().fill(primary && !disabled ? Theme.accent : (hovering && !disabled ? Theme.hover : Color.clear)))
                .contentShape(Rectangle())
            }.buttonStyle(.plain).disabled(disabled)
        }
    }
}

// MARK: - the Jot field

/// What the popover's Jot field is doing: its text, and the short confirmation after Return.
final class PopoverJotState: ObservableObject {
    enum Flash: Equatable { case none, added, kept, failed }
    @Published var text = ""
    @Published private(set) var flash = Flash.none
    private var work: DispatchWorkItem?

    func show(_ f: Flash, for seconds: TimeInterval) {
        work?.cancel()
        flash = f
        let w = DispatchWorkItem { [weak self] in self?.flash = .none }
        work = w
        DispatchQueue.main.asyncAfter(deadline: .now() + seconds, execute: w)
    }
}

/// A 32 pt line at the top of the popover: type, Return adds a note to today's page, "Added" shows for 600 ms and the field is
/// ready for the next one. It is the keyboard route to Jot when the global shortcut is off or taken.
struct PopoverJotField: View {
    @ObservedObject var model: AppModel
    @StateObject private var state = PopoverJotState()
    @FocusState private var focused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                TextField(JotCopy.placeholder, text: $state.text)
                    .textFieldStyle(.plain).font(Theme.font(13)).focused($focused)
                    .onSubmit { submit() }
                    .accessibilityLabel("Jot a line for today")
                    .accessibilityHint("Press Return to add it to today's page")
                trailing
            }
            .padding(.horizontal, 10).frame(height: 32)
            .background(Theme.rect().fill(Theme.surface))
            .overlay(Theme.rect().stroke(focused ? Theme.accent : Theme.border, lineWidth: focused ? 1.5 : 1))
            if model.notesWaiting > 0 { waiting }
        }
        .onAppear { DispatchQueue.main.async { focused = true } }
    }

    @ViewBuilder private var trailing: some View {
        switch state.flash {
        case .added:
            HStack(spacing: 4) {
                Image(systemName: "checkmark").font(Theme.font(11, .bold)).accessibilityHidden(true)
                Text("Added").font(Theme.font(12, .semibold))
            }.foregroundColor(Theme.accentText).accessibilityLabel(JotCopy.addedSpoken)
        case .kept:
            HStack(spacing: 4) {
                Image(systemName: "info.circle").font(Theme.font(12)).accessibilityHidden(true)
                Text("Kept on this Mac").font(Theme.font(12))
            }.foregroundColor(Theme.textSecondary).accessibilityLabel(JotCopy.keptSpoken)
        case .failed:
            Text("Not saved").font(Theme.font(12, .semibold)).foregroundColor(Theme.danger).accessibilityLabel(JotCopy.failed)
        case .none:
            if !state.text.isEmpty {
                Image(systemName: "return").font(Theme.font(12)).foregroundColor(Theme.textSecondary).accessibilityHidden(true)
            }
        }
    }

    private var waiting: some View {
        HStack(spacing: 6) {
            Image(systemName: "tray.full").font(Theme.font(12)).accessibilityHidden(true)
            Text(model.notesWaitingLabel).font(Theme.font(12))
            Spacer(minLength: Theme.s2)
            Button("Try now") { model.replayCapture(); model.retryFolder() }.buttonStyle(TextButtonStyle())
                .font(Theme.font(12, .semibold))
        }
        .foregroundColor(Theme.textSecondary)
        .help("Saved on this Mac. \(Fmt.plural(model.notesWaiting, "jot")) will be added to your page when your log folder is back.")
    }

    private func submit() {
        let t = state.text.trimmingCharacters(in: .whitespacesAndNewlines)
        if t.isEmpty || t == "[]" { return }
        switch model.jotOutcome(text: state.text, source: "menubar") {
        case .added:
            state.text = ""; state.show(.added, for: JotMetrics.addedHold)
            if model.live { UIAnnounce.say(JotCopy.addedSpoken) }
        case .kept:
            state.text = ""; state.show(.kept, for: JotMetrics.keptHold)
            if model.live { UIAnnounce.say(JotCopy.keptSpoken) }
        case .failed:
            state.show(.failed, for: 3)                         // the text stays in the field
            if model.live { UIAnnounce.say(JotCopy.failed, assertive: true) }
        }
        focused = true
    }
}
