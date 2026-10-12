// ReviewCommands.swift - the menu commands of the Review and of copying (M3). Added to the scene with one line in GloamlogApp.swift.
//   File    Export… ⇧⌘E (the Export sheet) · Copy as ▸ Rich Text, Markdown, Plain Text (the review on screen, or the open page) ·
//           Save Review as Markdown… / as PDF… · Share Review…        (⇧⌘C, Copy Week/Page as Markdown, stays in AppCommands)
//   Review  Week · Month ⌘4 · Year ⌘5 · Range · Last Week · This Month · Include Skipped Days · Include Jots · Group By Day or Section
// Week is ⌘3 in the Go menu (AppCommands), so it carries no key here. Every item that needs the review is disabled elsewhere.
import SwiftUI

struct ReviewCommands: Commands {
    @ObservedObject var model: AppModel
    private var review: ReviewController { model.review }
    private var onReview: Bool { model.selection == .week }
    private var onPage: Bool { if case .day = model.selection { return model.editor?.showsEditor ?? false }; return false }

    private func copy(_ f: CopyFlavor) { if onReview { review.copy(f) } else if onPage { review.copyPage(f) } }

    var body: some Commands {
        CommandGroup(after: .importExport) {
            Divider()
            Button("Export…") { review.exportOpen = true }.keyboardShortcut("e", modifiers: [.command, .shift]).disabled(!onReview)
            Menu("Copy as") {
                Button("Rich Text") { copy(.rich) }
                Button("Markdown") { copy(.markdown) }
                Button("Plain Text") { copy(.plain) }
            }.disabled(!(onReview || onPage))
            Button("Save Review as Markdown…") { review.save(.markdown) }.disabled(!onReview)
            Button("Save Review as PDF…") { review.save(.pdf) }.disabled(!onReview)
            Button("Share Review…") { review.share() }.disabled(!onReview)
        }
        CommandMenu("Review") {
            Button("Week") { review.show(.week) }
            Button("Month") { review.show(.month) }.keyboardShortcut("4")
            Button("Year") { review.show(.year) }.keyboardShortcut("5")
            Button("Range") { review.show(.range) }
            Divider()
            Button("Last Week") { model.select(.week); review.lastWeek() }
            Button("This Month") { model.select(.week); review.thisMonth() }
            Divider()
            Toggle("Include Skipped Days", isOn: Binding(get: { review.includeSkipped }, set: { review.includeSkipped = $0 })).disabled(!onReview)
            Toggle("Include Jots", isOn: Binding(get: { review.includeJots }, set: { review.includeJots = $0 })).disabled(!onReview)
            Menu("Group By") {
                Button((review.grouping == .day ? "✓ " : "") + "Day") { review.groupingChoice = .day }
                Button((review.grouping == .section ? "✓ " : "") + "Section") { review.groupingChoice = .section }
            }.disabled(!onReview)
        }
    }
}
