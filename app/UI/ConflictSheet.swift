// ConflictSheet.swift - "two versions of a day": this Mac's page beside the other copy, the lines that only one side has marked, and
// three ways out. Keep mine and Take theirs choose a side; Keep both (the default) keeps this Mac's page and adds the other copy's
// extra lines under "From another copy". Whichever is chosen, the other copy is saved under Safety copies first: nothing is deleted.
import SwiftUI

final class ConflictForm: ObservableObject {
    struct Side {
        let title: String
        let lines: [(text: String, differs: Bool)]
    }
    @Published var items: [ConflictCopy] = []
    @Published var index = 0
    @Published var mine: Side?
    @Published var theirs: Side?
    @Published var mineExists = false
    @Published var theirsHasText = false
    @Published var error: String?
    @Published var busy = false

    var current: ConflictCopy? { items.indices.contains(index) ? items[index] : nil }

    private static let dayF: DateFormatter = { let f = DateFormatter(); f.dateFormat = "EEE d MMM"; return f }()
    private static func title(_ who: String, _ date: Date?, words: Int) -> String {
        let when = date.map { "\(dayF.string(from: $0)), \(Fmt.time($0))" } ?? "time unknown"
        return "\(who), \(when), \(Fmt.plural(words, "word"))"
    }

    /// Re-reads the list and the shown conflict. The open page is saved first, so "this Mac" includes what was typed a moment ago.
    func load(_ model: AppModel, done: (() -> Void)? = nil) {
        model.flushEditor { [weak self, weak model] in
            guard let self = self, let model = model else { return }
            self.items = model.sync.conflicts
            if self.index >= self.items.count { self.index = max(0, self.items.count - 1) }
            self.read(model)
            done?()
        }
    }

    func step(_ delta: Int, _ model: AppModel) {
        guard !items.isEmpty else { return }
        index = (index + delta + items.count) % items.count
        error = nil
        read(model)
    }

    private func read(_ model: AppModel) {
        guard let c = current else { mine = nil; theirs = nil; return }
        do {
            let s = try model.sync.readSides(c)
            let m = s.mineBody, t = s.theirs.body
            mineExists = s.mine != nil
            theirsHasText = MarkdownBody.hasContent(t)
            let mineWho = s.mine == nil ? "This Mac, no page" : (s.mine?.isSkipped == true ? "This Mac, skipped" : "This Mac")
            mine = Side(title: s.mine == nil ? mineWho : Self.title(mineWho, s.mineDate, words: MarkdownBody.words(in: m)),
                        lines: BothVersions.marked(m, against: t))
            theirs = Side(title: Self.title(s.theirs.isSkipped ? "Other copy, skipped" : "Other copy", s.theirsDate, words: MarkdownBody.words(in: t)),
                          lines: BothVersions.marked(t, against: m))
        } catch let p as SyncProblem {
            mine = nil; theirs = nil; error = p.message
        } catch {
            mine = nil; theirs = nil; self.error = error.localizedDescription
        }
    }
}

struct ConflictSheet: View {
    @ObservedObject var model: AppModel
    @StateObject private var form = ConflictForm()

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            header
            if form.current == nil {
                Text("No conflicts left.").font(Theme.font(13)).foregroundColor(Theme.textSecondary).padding(.vertical, Theme.s4)
                Spacer(minLength: 0)
            } else if let m = form.mine, let t = form.theirs {
                HStack(alignment: .top, spacing: Theme.s4) { column(m); column(t) }
                Text("Lines with a bar are only in that version. Keep both adds the other copy's extra lines under \"\(BothVersions.heading)\". The version you don't keep is saved under Safety copies, so nothing is deleted.")
                    .font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            } else {
                Spacer(minLength: 0)
            }
            if let e = form.error { Text(e).font(Theme.font(13)).foregroundColor(Theme.danger).fixedSize(horizontal: false, vertical: true) }
            HStack(spacing: Theme.s3) {
                Button("Close") { model.sheet = nil }.buttonStyle(SecondaryButtonStyle()).keyboardShortcut(.cancelAction)
                Spacer()
                Button("Keep mine") { choose(.keepMine) }.buttonStyle(SecondaryButtonStyle())
                    .disabled(form.busy || form.mine == nil || !form.mineExists)
                    .help(form.mineExists ? "Keep this Mac's page as it is" : "This Mac has no page for this day")
                Button("Take theirs") { choose(.takeTheirs) }.buttonStyle(SecondaryButtonStyle())
                    .disabled(form.busy || form.theirs == nil || !form.theirsHasText)
                    .help(form.theirsHasText ? "Replace this Mac's page with the other copy" : "The other copy has no text")
                Button("Keep both") { choose(.keepBoth) }.buttonStyle(PrimaryButtonStyle()).keyboardShortcut(.defaultAction)
                    .disabled(form.busy || form.mine == nil || form.theirs == nil)
                    .help("Keep this Mac's page and add the other copy's extra lines under \"\(BothVersions.heading)\"")
            }
        }
        .padding(Theme.s6).frame(width: 720, height: 520).background(Theme.surface)
        .onAppear { form.load(model) }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.s3) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Two versions of this day").font(Theme.font(22, .semibold)).foregroundColor(Theme.textPrimary).accessibilityAddTraits(.isHeader)
                if let c = form.current {
                    Text(model.longDate(c.day) + (form.items.count > 1 ? " · \(form.index + 1) of \(form.items.count)" : ""))
                        .font(Theme.font(13)).foregroundColor(Theme.textSecondary)
                }
            }
            Spacer()
            if form.items.count > 1 {
                Button { form.step(-1, model) } label: { Image(systemName: "chevron.left").frame(width: 24, height: 24) }
                    .buttonStyle(.plain).help("Previous conflict").accessibilityLabel("Previous conflict")
                Button { form.step(1, model) } label: { Image(systemName: "chevron.right").frame(width: 24, height: 24) }
                    .buttonStyle(.plain).help("Next conflict").accessibilityLabel("Next conflict")
            }
        }
    }

    private func column(_ side: ConflictForm.Side) -> some View {
        VStack(alignment: .leading, spacing: Theme.s2) {
            Text(side.title).font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary).lineLimit(1)
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(side.lines.enumerated()), id: \.offset) { _, line in
                        Text(line.text.isEmpty ? " " : line.text)
                            .font(.system(size: 12, design: .monospaced)).foregroundColor(Theme.textPrimary)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.leading, 8).padding(.trailing, 6).padding(.vertical, 1)
                            .background(line.differs ? Theme.warningTint : Color.clear)
                            .overlay(alignment: .leading) { if line.differs { Rectangle().fill(Theme.warning).frame(width: 2) } }
                            .accessibilityLabel(line.differs ? "Only in this version: \(line.text)" : line.text)
                    }
                }.padding(.vertical, Theme.s1)
            }
            .background(Theme.rect().fill(Theme.bg))
            .overlay(Theme.rect().stroke(Theme.border, lineWidth: 1))
            .accessibilityElement(children: .contain).accessibilityLabel(side.title)
        }
        .frame(maxWidth: .infinity)
    }

    private func choose(_ choice: ConflictChoice) {
        guard let c = form.current else { return }
        form.busy = true; form.error = nil
        model.sync.resolve(c, choice) { problem in
            form.busy = false
            if let p = problem { form.error = p; return }
            form.load(model) { if form.items.isEmpty { model.sheet = nil } }       // the next conflict, or done
        }
    }
}
