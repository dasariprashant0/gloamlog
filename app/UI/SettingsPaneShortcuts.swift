// SettingsPaneShortcuts.swift - Settings > Shortcuts: the Jot shortcut (record, Off, Reset; takes effect at once), what a jot
// looks like, and a read-only list of every keyboard shortcut. Each of those is also a menu item.
// The list follows the menus in AppCommands.swift (keyboard map: docs/v1/UX_FLOWS.md 5.1); keep the two in step.
import SwiftUI

struct ShortcutsSettings: View {
    @ObservedObject var model: AppModel
    @StateObject private var recording = ShortcutRecording()

    struct ShortcutGroup: Identifiable {
        let title: String
        let items: [(name: String, keys: String)]
        var id: String { title }
    }

    static let groups: [ShortcutGroup] = [
        ShortcutGroup(title: "Go", items: [
            ("Today", "⌘T  ⌘1"), ("Catch Up", "⌘2"), ("Week Review", "⌘3"),
            ("Previous Day", "⌘["), ("Next Day", "⌘]"), ("Go to Date…", "⇧⌘T"),
            ("Next Unlogged Day", "⌘↩"), ("Previous Unlogged Day", "⌥⌘↑"),
            ("Month Review", "⌘4"), ("Year Review", "⌘5"), ("Export…", "⇧⌘E"),
        ]),
        ShortcutGroup(title: "Page", items: [
            ("Focus the Page", "⌘E"), ("Save Now", "⌘S"), ("Carry Over from Yesterday", "⇧⌘Y"),
            ("Skip Day…", "⇧⌘K"), ("Remind Me Later", "⇧⌘L"),
        ]),
        ShortcutGroup(title: "Search and copy", items: [
            ("Search Logs", "⌘F"), ("Next Search Result", "⌘G"), ("Previous Search Result", "⇧⌘G"),
            ("Copy as Markdown", "⇧⌘C"), ("Open Log Folder in Finder", "⇧⌘O"),
        ]),
        ShortcutGroup(title: "Settings", items: [
            ("Open Settings", "⌘,"), ("Switch pane", "← →"),
        ]),
    ]

    private var spec: HotKeySpec? { model.settings.capture.hotKey }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.s4) {
                jotSection
                HStack(alignment: .top, spacing: Theme.s5) {
                    VStack(alignment: .leading, spacing: Theme.s4) { card(ShortcutsSettings.groups[0]); card(ShortcutsSettings.groups[3]) }
                    VStack(alignment: .leading, spacing: Theme.s4) { card(ShortcutsSettings.groups[1]); card(ShortcutsSettings.groups[2]) }
                }
                SettingsHelp("Every shortcut here is also a menu item, with the shortcut shown beside it.")
            }
            .padding(Theme.s5)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .onAppear { let m = model; recording.onShortcut = { [weak m] s in m?.setJotHotKey(s) } }
        .onDisappear { recording.stop() }
    }

    // MARK: Jot

    private var jotSection: some View {
        VStack(alignment: .leading, spacing: Theme.s2) {
            Text("Jot from anywhere").font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
                .padding(.leading, Theme.s1).accessibilityAddTraits(.isHeader)
            VStack(spacing: 0) {
                recorderRow
                Divider().padding(.horizontal, Theme.s3)
                toggleRow("Time stamp", help: "Starts each jot with the time, like 14:32.", isOn: $model.settings.capture.timestamps)
                Divider().padding(.horizontal, Theme.s3)
                toggleRow("[] makes a to-do", help: "A jot that starts with [] becomes a task.", isOn: $model.settings.capture.todoShorthand)
                Divider().padding(.horizontal, Theme.s3)
                toggleRow("Jots count toward logging a day", help: "Off: a day with only jots still reads as started.",
                          isOn: $model.settings.capture.jotsCountTowardLogged)
            }
            .background(Theme.rect(Theme.radiusLg).fill(Color.primary.opacity(0.05)))
        }
    }

    private var recorderRow: some View {
        VStack(alignment: .leading, spacing: Theme.s2) {
            HStack(spacing: Theme.s2) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Shortcut").font(Theme.font(13)).foregroundColor(Theme.textPrimary)
                    Text(spec == nil ? "Off. Use the Jot field in the menu bar, or Page > Jot…"
                         : "Press it in any app to add a line to today's page.")
                        .font(Theme.font(12)).foregroundColor(Theme.textSecondary).lineLimit(1)
                }
                .accessibilityHidden(true)
                Spacer(minLength: Theme.s3)
                ShortcutRecorder(model: model, recording: recording)
                Button("Reset") { model.setJotHotKey(HotKeySpec.defaultJot) }
                    .disabled(spec == HotKeySpec.defaultJot && model.hotKeyFailure == nil)
                    .accessibilityHint("Goes back to \(HotKeySpec.defaultJot.display)")
                Button("Off") { model.setJotHotKey(nil) }.disabled(spec == nil)
                    .accessibilityHint("Turns the global shortcut off")
            }
            if let f = model.hotKeyFailure {
                HStack(spacing: Theme.s2) {
                    Image(systemName: "exclamationmark.triangle").font(Theme.font(13)).foregroundColor(Theme.warning).accessibilityHidden(true)
                    Text(f.message).font(Theme.font(13)).foregroundColor(Theme.textPrimary).fixedSize(horizontal: false, vertical: true)
                    Spacer(minLength: Theme.s2)
                    Button("Record another…") { recording.start() }
                    Button("Off") { model.setJotHotKey(nil) }.disabled(spec == nil)
                }
                .accessibilityElement(children: .contain)
            } else if let h = recording.hint {
                Text(h).font(Theme.font(12)).foregroundColor(Theme.textSecondary)
            }
        }
        .padding(.horizontal, Theme.s3).padding(.vertical, Theme.s2)
    }

    /// A switch with its explanation on the same line (the pane stays short enough to show every shortcut without scrolling).
    private func toggleRow(_ title: String, help: String, isOn: Binding<Bool>) -> some View {
        HStack(spacing: Theme.s3) {
            Text(title).font(Theme.font(13)).foregroundColor(Theme.textPrimary).accessibilityHidden(true)
            Text(help).font(Theme.font(12)).foregroundColor(Theme.textSecondary).lineLimit(1).accessibilityHidden(true)
            Spacer(minLength: Theme.s3)
            Toggle(title, isOn: isOn).labelsHidden().toggleStyle(.switch)
                .accessibilityLabel(title).accessibilityHint(help)
        }
        .frame(minHeight: SettingsTheme.rowMin + 6)
        .padding(.horizontal, Theme.s3)
    }

    // MARK: the list

    private func card(_ g: ShortcutGroup) -> some View {
        VStack(alignment: .leading, spacing: Theme.s2) {
            Text(g.title).font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
                .padding(.leading, Theme.s1).accessibilityAddTraits(.isHeader)
            VStack(spacing: 0) {
                ForEach(Array(g.items.enumerated()), id: \.offset) { i, item in
                    if i > 0 { Divider().padding(.horizontal, Theme.s3) }
                    HStack(spacing: Theme.s3) {
                        Text(item.name).font(Theme.font(13)).foregroundColor(Theme.textPrimary)
                        Spacer(minLength: Theme.s3)
                        Text(item.keys).font(.system(size: 13, design: .rounded)).foregroundColor(Theme.textSecondary)
                            .accessibilityLabel(spokenShortcut(item.keys))
                    }
                    .padding(.vertical, 5).padding(.horizontal, Theme.s3)
                    .accessibilityElement(children: .combine)
                }
            }
            .background(Theme.rect(Theme.radiusLg).fill(Color.primary.opacity(0.05)))
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
