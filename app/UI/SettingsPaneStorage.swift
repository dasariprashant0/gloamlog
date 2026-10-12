// SettingsPaneStorage.swift - Settings > Storage and backups: where the logs live and where the safety copies go.
// Changing the folder keeps the existing copy / move / use-existing flow (AppModel+Storage.swift): nothing is deleted.
import SwiftUI

struct StorageSettings: View {
    @ObservedObject var model: AppModel

    private var isDefaultFolder: Bool {
        model.settings.storageFolder.standardizedFileURL.path == Settings.defaultFolder.standardizedFileURL.path
    }
    private var backupsPath: String { (model.backupDir.path as NSString).abbreviatingWithTildeInPath }
    private var backupsExist: Bool { FileManager.default.fileExists(atPath: model.backupDir.path) }

    private func summaryLine(_ s: FolderSummary) -> String {
        var t = "\(Fmt.plural(s.logs, "log")) · \(s.skipped) skipped \(s.skipped == 1 ? "day" : "days")"
        if s.unrecognized > 0 { t += " · \(Fmt.plural(s.unrecognized, "file")) not recognised" }
        return t
    }

    var body: some View {
        SettingsPaneFrame(model: model, pane: .storage) {
            Section {
                SettingsRow("Folder", help: model.storageSummary.map(summaryLine)) {
                    Text(model.folderDisplay).font(.system(size: 12, design: .monospaced)).foregroundColor(Theme.textPrimary)
                        .lineLimit(1).truncationMode(.middle).textSelection(.enabled).help(model.settings.storageFolder.path)
                }
                HStack(spacing: Theme.s2) {
                    Spacer(minLength: 0)
                    Button("Show in Finder") { model.revealFolder() }
                    Button("Change…") { model.chooseFolder() }
                    Button("Use default") { model.useDefaultFolder() }.disabled(isDefaultFolder)
                }
                if let p = model.folderProblem { folderProblem(p) }
                if let r = model.storageResult {
                    SettingsStatus(symbol: "checkmark.circle", r.text) {
                        if !r.conflicts.isEmpty { Button("Show them") { model.reveal(r.conflicts) } }
                    }
                }
                SettingsHelp("Changing the folder never deletes a log: Gloamlog asks whether to copy or move them first. Pick a folder in iCloud Drive or Dropbox to keep your logs on every Mac.")
            } header: { Text("Log folder").font(Theme.font(13, .semibold)) }

            Section {
                SettingsRow("Location", help: backupsExist ? nil : "Created when a page is first overwritten.") {
                    HStack(spacing: Theme.s2) {
                        Text(backupsPath).font(.system(size: 12, design: .monospaced)).foregroundColor(Theme.textPrimary)
                            .lineLimit(1).truncationMode(.middle).textSelection(.enabled).help(model.backupDir.path)
                        Button("Open") { model.reveal([model.backupDir]) }.disabled(!backupsExist)
                    }
                }
                SettingsHelp("Before a page is overwritten, Gloamlog keeps up to \(LogStore.maxBackupsPerDay) earlier versions of it here, outside your log folder, so they never sync. To bring one back, open the page and choose Restore Previous Version… in the File menu, or in the page's … menu.")
            } header: { Text("Safety copies").font(Theme.font(13, .semibold)) }

            SyncStorageSection(model: model)                            // M5b: folder kind, sync status, "Resolve conflicts"
        }
        .onAppear { model.refreshSummary() }
    }

    @ViewBuilder private func folderProblem(_ p: FolderProblem) -> some View {
        switch p {
        case .missing:
            SettingsStatus(symbol: "exclamationmark.triangle", tint: Theme.danger, "This folder can't be found.") {
                Button("Create it again") { model.recreateFolder() }
            }
        case .notWritable:
            SettingsStatus(symbol: "exclamationmark.triangle", tint: Theme.danger, "Gloamlog can't write to this folder.") {
                Button("Try again") { model.retryFolder() }
            }
        }
    }
}
