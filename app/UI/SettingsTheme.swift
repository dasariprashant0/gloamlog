// SettingsTheme.swift - sizes, pane metadata and the small building blocks every Settings pane shares.
// Controls stay native (grouped Form, system accent); only text colours and spacing come from Theme.
import SwiftUI
import AppKit

enum SettingsTheme {
    /// Window content width. DESIGN_V4 allows 840 for eleven panes; six panes fit a 640 column plus 20 pt gutters.
    static let width: CGFloat = 680
    static let rowMin: CGFloat = 28
    /// Panes never grow past this (they scroll instead); also capped to the screen at run time.
    static let heightCap: CGFloat = 660
}

extension SettingsPane {
    /// Window title and toolbar label. The window title follows the pane (HIG).
    var title: String {
        switch self {
        case .general: return "General"
        case .reminders: return "Reminders"
        case .page: return "Page"
        case .storage: return "Storage and backups"
        case .shortcuts: return "Shortcuts"
        case .about: return "About"
        }
    }
    var symbol: String {
        switch self {
        case .general: return "gearshape"
        case .reminders: return "alarm"
        case .page: return "doc.text"
        case .storage: return "externaldrive"
        case .shortcuts: return "keyboard"
        case .about: return "info.circle"
        }
    }
    /// Window content height for the pane (DESIGN_V4: 300 to 560, scrolls above). Tuned from real screenshots.
    var contentHeight: CGFloat {
        switch self {
        case .general: return 460
        case .reminders: return 600
        case .page: return 640
        case .storage: return 440
        case .shortcuts: return 640
        case .about: return 380
        }
    }
}

/// Helper text under a row: 12/400 secondary.
struct SettingsHelp: View {
    let text: String
    init(_ text: String) { self.text = text }
    var body: some View {
        Text(text).font(Theme.font(12)).foregroundColor(Theme.textSecondary)
            .fixedSize(horizontal: false, vertical: true).frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// One row of a grouped form: label left, control right, plain-language help underneath. The label and the help
/// become the control's accessibility label and hint, so VoiceOver reads one element per row.
struct SettingsRow<Control: View>: View {
    let title: String
    let help: String?
    let control: Control
    init(_ title: String, help: String? = nil, @ViewBuilder control: () -> Control) {
        self.title = title; self.help = help; self.control = control()
    }
    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s1) {
            HStack(spacing: Theme.s3) {
                Text(title).font(Theme.font(13)).foregroundColor(Theme.textPrimary).accessibilityHidden(true)
                Spacer(minLength: Theme.s3)
                control.accessibilityLabel(title).accessibilityHint(help ?? "")
            }.frame(minHeight: SettingsTheme.rowMin)
            if let h = help { SettingsHelp(h).accessibilityHidden(true) }
        }
        .padding(.vertical, 2)
    }
}

/// A status line with a symbol, a sentence and optional trailing buttons (notifications, folder problems).
struct SettingsStatus<Actions: View>: View {
    let symbol: String
    let tint: Color
    let text: String
    let actions: Actions
    init(symbol: String, tint: Color = Theme.textSecondary, _ text: String, @ViewBuilder actions: () -> Actions) {
        self.symbol = symbol; self.tint = tint; self.text = text; self.actions = actions()
    }
    var body: some View {
        HStack(spacing: Theme.s3) {
            Image(systemName: symbol).font(Theme.font(14)).foregroundColor(tint).frame(width: 18).accessibilityHidden(true)
            Text(text).font(Theme.font(13)).foregroundColor(Theme.textPrimary).fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: Theme.s3)
            actions
        }.frame(minHeight: SettingsTheme.rowMin).accessibilityElement(children: .contain)
    }
}
extension SettingsStatus where Actions == EmptyView {
    init(symbol: String, tint: Color = Theme.textSecondary, _ text: String) { self.init(symbol: symbol, tint: tint, text) { EmptyView() } }
}

/// "Reset to defaults…" at the foot of a pane. Asks first; pages are never touched. `summary` is a noun phrase
/// ("appearance, week start and the menu bar item") that completes "Resets …".
struct SettingsReset: View {
    @ObservedObject var model: AppModel
    let pane: SettingsPane
    let summary: String
    var body: some View {
        HStack(spacing: Theme.s3) {
            Button("Reset to defaults…") { confirm() }.accessibilityLabel("Reset \(pane.title) settings to defaults")
            Text("Resets \(summary).").font(Theme.font(12)).foregroundColor(Theme.textSecondary)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
    }

    private func confirm() {
        let a = NSAlert()
        a.messageText = "Reset \(pane.title) settings?"
        a.informativeText = "This resets \(summary). Your pages are not touched."
        a.addButton(withTitle: "Reset"); a.addButton(withTitle: "Cancel")
        let done: (NSApplication.ModalResponse) -> Void = { r in if r == .alertFirstButtonReturn { model.resetSettings(pane) } }
        if let w = SettingsWindowController.shared.window, w.isVisible { a.beginSheetModal(for: w, completionHandler: done) }
        else { done(a.runModal()) }
    }
}

/// Spoken form of a shortcut such as "⇧⌘T" ("Shift Command T") for VoiceOver.
func spokenShortcut(_ keys: String) -> String {
    let names: [Character: String] = ["⌘": "Command", "⇧": "Shift", "⌥": "Option", "⌃": "Control", "↩": "Return", "⌫": "Delete",
                                      "←": "Left arrow", "→": "Right arrow", "↑": "Up arrow", "↓": "Down arrow", "⎋": "Escape"]
    return keys.map { names[$0] ?? String($0) }.joined(separator: " ")
}
