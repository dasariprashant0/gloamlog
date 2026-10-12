// SettingsView.swift - the content of the Settings window: the pane that the toolbar tabs (SettingsWindow.swift) selected.
// Every change applies at once and persists (AppModel.settings saves itself); there is no Save button.
import SwiftUI

struct SettingsView: View {
    @ObservedObject var model: AppModel
    @ObservedObject var nav: SettingsNav

    var body: some View {
        Group {
            switch nav.pane {
            case .general: GeneralSettings(model: model)
            case .reminders: RemindersSettings(model: model)
            case .page: PageSettings(model: model)
            case .storage: StorageSettings(model: model)
            case .shortcuts: ShortcutsSettings(model: model)
            case .about: AboutSettings(model: model)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// A pane: a grouped form that scrolls, with an optional "Reset to defaults…" bar pinned under it.
struct SettingsPaneFrame<Content: View>: View {
    @ObservedObject var model: AppModel
    let pane: SettingsPane
    var resetSummary: String?
    let content: Content
    init(model: AppModel, pane: SettingsPane, resetSummary: String? = nil, @ViewBuilder content: () -> Content) {
        self.model = model; self.pane = pane; self.resetSummary = resetSummary; self.content = content()
    }
    var body: some View {
        VStack(spacing: 0) {
            Form { content }.formStyle(.grouped)
            if let s = resetSummary {
                Divider()
                HStack { SettingsReset(model: model, pane: pane, summary: s); }
                    .padding(.horizontal, Theme.s5).frame(height: 48)
            }
        }
    }
}
