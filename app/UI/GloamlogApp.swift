// GloamlogApp.swift - @main: main Window, MenuBarExtra, menu commands. Settings is not a scene: it is an AppKit window
// opened by AppModel.openSettings(pane:) (SettingsWindow.swift), and ⌘, is the "Settings…" command below.
import SwiftUI

@main
struct GloamlogApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model = AppModel.shared
    @AppStorage("showMenuBar") private var showMenuBar = true

    init() {
        // Appearance (System, Light, Dark) and the other settings effects are live from the first frame.
        AppModel.shared.installSettingsEffects()
        // The Settings window is built hidden a moment after launch, so opening it is instant (cold: about half a second).
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { SettingsWindowController.shared.prewarm(model: AppModel.shared) }
    }

    var body: some Scene {
        Window("Gloamlog", id: "main") {
            MainView(model: model)
        }
        .defaultSize(width: 1000, height: 760)
        .windowResizability(.contentMinSize)
        .commands { AppCommands(model: model) }
        .commands { SettingsCommands(model: model) }
        .commands { ReviewCommands(model: model) }

        MenuBarExtra(isInserted: $showMenuBar) {
            MenuBarView(model: model)
        } label: {
            Image(systemName: model.menuIcon.symbol).accessibilityLabel(model.menuIcon.title)
        }
        .menuBarExtraStyle(.window)
    }
}
