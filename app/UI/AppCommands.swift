// AppCommands.swift - the app's menu commands (File / Edit / Go / Page). Owned by the navigation stream; GloamlogApp.swift owns
// the scenes. Every toolbar and sidebar action has a command here (HIG), with the keyboard map of UX_FLOWS 5.1:
//   Go    Today ⌘T (⌘1 in the window)   Catch Up ⌘2   Week Review ⌘3   Previous / Next Day ⌘[ ⌘]   Go to Date… ⇧⌘T
//         Next Unlogged Day ⌘↩ (⌥⌘↓ in a session)   Previous Unlogged Day ⌥⌘↑
//   Page  Jot… (M2) shows the recorded global shortcut, ⌃⌥J by default; Settings > Shortcuts changes or removes it
// Collisions: the editor binds Mod-[ and Mod-] to outdent/indent inside a list item or a table cell, so ⌘[ ⌘] reach this
// menu everywhere else; Mod-Enter only inside a table. Nothing here uses a key the editor binds anywhere else.
import SwiftUI

struct AppCommands: Commands {
    @ObservedObject var model: AppModel
    private var onDay: Bool { if case .day = model.selection { return true }; return false }
    private var onWeek: Bool { model.selection == .week }
    private var canEdit: Bool { onDay && (model.editor?.showsEditor ?? false) }

    var body: some Commands {
        CommandGroup(replacing: .appInfo) { Button("About Gloamlog") { AppDelegate.showAbout() } }
        CommandGroup(replacing: .saveItem) {
            // Autosave is always on; ⌘S just flushes right now, as people expect.
            Button("Save Now") { model.flushEditor() }.keyboardShortcut("s").disabled(!canEdit)
            Divider()
            Button(onWeek ? (model.review.scale == .week ? "Copy Week as Markdown" : "Copy Review as Markdown") : "Copy Page as Markdown") {
                if onWeek { model.copyWeek() } else { model.copyPageMarkdown() }
            }
                .keyboardShortcut("c", modifiers: [.command, .shift]).disabled(!(onWeek || canEdit))
            Button("Open Log Folder in Finder") { model.revealFolder() }.keyboardShortcut("o", modifiers: [.command, .shift])
        }
        CommandGroup(replacing: .textEditing) {
            Button("Search Logs") { model.focusSearch() }.keyboardShortcut("f")
            Button("Next Search Result") { model.moveSearchCursor(1) }.keyboardShortcut("g")
            Button("Previous Search Result") { model.moveSearchCursor(-1) }.keyboardShortcut("g", modifiers: [.command, .shift])
        }
        CommandMenu("Go") {
            Button("Today") { model.openToday() }.keyboardShortcut("t")
            Button("Catch Up") { model.select(.catchUp) }.keyboardShortcut("2")
            Button("Week Review") { model.select(.week) }.keyboardShortcut("3")
            Divider()
            Button(onWeek ? "Previous \(model.review.noun)" : "Previous Day") { model.step(-1) }.keyboardShortcut("[")
                .disabled(!(onWeek ? model.review.canStep(-1) : model.canStep(-1)))
            Button(onWeek ? "Next \(model.review.noun)" : "Next Day") { model.step(1) }.keyboardShortcut("]")
                .disabled(!(onWeek ? model.canShowNextWeek : model.canStep(1)))
            Button("Go to Date…") { model.goToDateOpen = true }.keyboardShortcut("t", modifiers: [.command, .shift])
            Divider()
            Button("Next Unlogged Day") { model.nextUnloggedDay() }.keyboardShortcut(.return, modifiers: .command)
                .disabled(model.catchUpDays.isEmpty && model.session == nil)
            Button("Previous Unlogged Day") { model.previousSessionDay() }.keyboardShortcut(.upArrow, modifiers: [.command, .option])
                .disabled(model.session == nil)
        }
        CommandMenu("Page") {
            Button("Focus the Page") { model.focusEditor() }.keyboardShortcut("e").disabled(!canEdit)
            Button("Carry Over from Yesterday") { model.carryOver() }.keyboardShortcut("y", modifiers: [.command, .shift]).disabled(!model.carryAvailable)
            Divider()
            Button("Skip Day…") { model.requestSkip(day: onDay ? model.editor?.day : nil) }.keyboardShortcut("k", modifiers: [.command, .shift])
            Button("Remind Me Later") { model.snooze() }.keyboardShortcut("l", modifiers: [.command, .shift]).disabled(!model.isDue || !model.canSnooze)
            // The menu item shows the recorded global shortcut (none when it is Off or has no menu form).
            if let k = model.settings.capture.hotKey?.menuShortcut {
                Button("Jot…") { model.showJotPanel(source: "menu") }.keyboardShortcut(k.key, modifiers: k.modifiers)
            } else {
                Button("Jot…") { model.showJotPanel(source: "menu") }
            }
            Divider()
            Button("Restore Previous Version…") { if let d = model.editor?.day { model.sheet = .restore(d) } }.disabled(!onDay)
            Button("Settings…") { model.openSettings() }
        }
    }
}
