// SyncChip.swift - what sync shows: the sidebar footer chip, the conflict banner above a day page, the "Downloading from iCloud"
// page for a day that is not on this Mac yet, and the Sync section of Settings > Storage. Quiet by design: nothing shows while
// there is nothing to say. See SyncController.swift for the behaviour and Core/Sync for the rules.
import SwiftUI

/// One line above the footer: "2 conflicts to review" (click: the review sheet) or "3 days still downloading" (click: Settings).
struct SyncChip: View {
    @ObservedObject var model: AppModel
    var body: some View {
        let sync = model.sync
        let conflicts = sync.conflicts.count, pending = sync.pendingDays.count
        if conflicts > 0 || pending > 0 {
            let text = conflicts > 0 ? "\(Fmt.plural(conflicts, "conflict")) to review" : "\(Fmt.plural(pending, "day")) still downloading"
            Button {
                if conflicts > 0 { sync.review() } else { model.openSettings(pane: .storage) }
            } label: {
                HStack(spacing: Theme.s2) {
                    if conflicts > 0 {
                        Image(systemName: "externaldrive.badge.exclamationmark").font(Theme.font(12)).foregroundColor(Theme.warning)
                            .accessibilityHidden(true)
                    } else {
                        ProgressView().controlSize(.mini).accessibilityHidden(true)
                    }
                    Text(text).font(Theme.font(12)).foregroundColor(conflicts > 0 ? Theme.warning : Theme.textSecondary).lineLimit(1)
                    Spacer(minLength: Theme.s1)
                    if conflicts > 0 { Text("Review").font(Theme.font(12, .semibold)).foregroundColor(Theme.accentText) }
                }
                .padding(.horizontal, Theme.s3).frame(height: 28).contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help(conflicts > 0 ? "Your sync service saved a second version of a day. Nothing is deleted: you choose what to keep."
                                : "Waiting for these days to arrive from \(sync.sourceName). Gloamlog won't write over them.")
            .accessibilityLabel(conflicts > 0 ? "\(text). Review them" : text)
        }
    }
}

/// Above a day page while a conflict copy exists: closed for the session with the X, back for a new conflict.
struct SyncBanners: View {
    @ObservedObject var model: AppModel
    var body: some View {
        let sync = model.sync
        if sync.bannerVisible {
            Banner(.warning, "\(Fmt.plural(sync.conflicts.count, "conflict")) in \(model.folderDisplay).",
                   detail: "Your sync service saved another copy of a day. Nothing is deleted: choose which version to keep, or keep both.",
                   onDismiss: { sync.dismissBanner() }) {
                Button("Review…") { sync.review() }.buttonStyle(TextButtonStyle())
            }
            .padding(.bottom, Theme.s4)
        }
    }
}

/// The detail pane for a day whose page is still in the cloud. No editor, no template, nothing is written: the page is opened for real
/// as soon as it arrives (SyncController.apply), so the text that is coming is never replaced by a new empty page.
struct DownloadingPage: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    var body: some View {
        let sync = model.sync
        let sameYear = String(editor.day.prefix(4)) == String(model.today.prefix(4))
        VStack(spacing: 0) {
            Column(top: 28, bottom: 0) {
                VStack(alignment: .leading, spacing: Theme.s3) {
                    Text(DayKey.format(editor.day, sameYear ? "EEEE, d MMMM" : "EEEE, d MMMM yyyy", model.cal))
                        .font(Theme.font(32, .semibold)).tracking(-0.5).foregroundColor(Theme.textPrimary)
                        .lineLimit(1).minimumScaleFactor(0.6).accessibilityAddTraits(.isHeader)
                    HStack(spacing: Theme.s2) {
                        ProgressView().controlSize(.small).accessibilityHidden(true)
                        Text("Downloading from \(sync.sourceName)…").font(Theme.font(15, .semibold)).foregroundColor(Theme.textPrimary)
                    }
                    Text("This day is in your shared folder but not on this Mac yet. Gloamlog is waiting for it and won't start a new page here, so nothing you wrote on another Mac gets overwritten. It opens by itself when it arrives.")
                        .font(Theme.font(13)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
                    HStack(spacing: Theme.s3) {
                        Button("Try again") { sync.download(day: editor.day) }.buttonStyle(SecondaryButtonStyle())
                        Button("Open Storage settings") { model.openSettings(pane: .storage) }.buttonStyle(TextButtonStyle())
                    }.padding(.top, Theme.s1)
                    Rectangle().fill(Theme.border).frame(height: 1).padding(.top, Theme.s3)
                }
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Theme.bg)
        .accessibilityElement(children: .contain)
        .onAppear { sync.download(day: editor.day) }
    }
}

/// Settings > Storage: the folder's kind, what sync is waiting for, and the way into the conflict sheet.
struct SyncStorageSection: View {
    @ObservedObject var model: AppModel

    private var kindHelp: String {
        switch model.sync.kind {
        case .local: return "Only on this Mac. Pick a folder in iCloud Drive or Dropbox to use one log on two Macs."
        case .network: return "On a network or external drive. Gloamlog checks it every few seconds."
        default: return "Gloamlog doesn't run sync. \(model.sync.sourceName) moves the files and Gloamlog watches the folder."
        }
    }

    var body: some View {
        let sync = model.sync
        let conflicts = sync.conflicts.count, pending = sync.pendingDays.count
        Section {
            SettingsRow("Folder type", help: kindHelp) {
                Text(sync.kind.label).font(Theme.font(13)).foregroundColor(Theme.textPrimary)
            }
            if conflicts > 0 {
                SettingsStatus(symbol: "exclamationmark.triangle", tint: Theme.warning, "\(Fmt.plural(conflicts, "conflict")) to resolve.") {
                    Button("Resolve conflicts") { sync.review() }
                }
            }
            if pending > 0 {
                SettingsStatus(symbol: "icloud.and.arrow.down", "\(Fmt.plural(pending, "day")) still downloading from \(sync.sourceName).") {
                    Button("Download all") { sync.downloadAll() }
                }
            }
            if conflicts == 0 && pending == 0 {
                SettingsStatus(symbol: sync.kind == .local ? "internaldrive" : "checkmark.circle",
                               sync.kind == .local ? "This folder is only on this Mac." : "Every day in this folder is on this Mac.")
            }
            SettingsHelp("If two Macs change the same day, Gloamlog asks which version to keep (or keeps both) and saves the other one under Safety copies. Days that are still downloading are never written over.")
        } header: { Text("Sync across Macs").font(Theme.font(13, .semibold)) }
    }
}
