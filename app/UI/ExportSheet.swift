// ExportSheet.swift - ⇧⌘E on the Review: every way out of a review in one small sheet (the Export menu has the common ones).
// Copy as Rich text, Markdown or Plain text; Save as a Markdown file (with an assets folder beside it), a folder, a zip or a PDF; Share.
// It says what would be written ("5 days, 2 images. Skipped days included. Jots left out.") before anything is.
import SwiftUI

struct ExportSheet: View {
    @ObservedObject var model: AppModel
    @ObservedObject var review: ReviewController
    @StateObject private var kind = Box(SaveKind.markdown)
    @StateObject private var info = Box<ExportPlan?>(nil)

    private var detail: String {
        guard let p = info.value else { return "" }
        let days = p.dayCount == 1 ? "1 day" : "\(p.dayCount) days"
        let images = p.assets.isEmpty ? "" : (p.assets.count == 1 ? ", 1 image" : ", \(p.assets.count) images")
        return "\(days)\(images). Skipped days \(review.includeSkipped ? "included" : "left out"). Jots \(review.includeJots ? "included" : "left out")."
    }

    private func copy(_ f: CopyFlavor) { review.exportOpen = false; review.copy(f) }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Export \(ReviewFormats.title(review.period, calendar: model.cal))").font(Theme.font(17, .semibold))
                    .foregroundColor(Theme.textPrimary).accessibilityAddTraits(.isHeader)
                Text(detail).font(Theme.font(13)).foregroundColor(Theme.textSecondary)
            }
            VStack(alignment: .leading, spacing: Theme.s2) {
                Text("Copy").font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
                HStack(spacing: Theme.s2) {
                    Button("Rich text") { copy(.rich) }.help("Pastes formatted into Notes, Mail and chat; Markdown where only text fits")
                    Button("Markdown") { copy(.markdown) }
                    Button("Plain text") { copy(.plain) }.help("No # or ** marks: for a chat box")
                }.buttonStyle(SecondaryButtonStyle())
            }
            VStack(alignment: .leading, spacing: Theme.s2) {
                Text("Save as").font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
                Picker("Save as", selection: $kind.value) {
                    ForEach(SaveKind.allCases, id: \.self) { Text($0.title).tag($0) }
                }.pickerStyle(.radioGroup).labelsHidden()
            }
            HStack(spacing: Theme.s3) {
                Button("Share…") { review.exportOpen = false; DispatchQueue.main.async { review.share() } }.buttonStyle(SecondaryButtonStyle())
                    .help("Opens the system share sheet. Nothing is sent until you choose a target there.")
                Spacer()
                Button("Cancel") { review.exportOpen = false }.buttonStyle(SecondaryButtonStyle()).keyboardShortcut(.cancelAction)
                Button("Save…") { review.exportOpen = false; review.save(kind.value) }.buttonStyle(PrimaryButtonStyle()).keyboardShortcut(.defaultAction)
            }
        }
        .padding(Theme.s6).frame(width: 480).background(Theme.surface)
        .onAppear { info.value = review.previewPlan() }
    }
}
