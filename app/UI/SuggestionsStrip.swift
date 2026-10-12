// SuggestionsStrip.swift - "Suggested from your work" (M6): a collapsible strip under the day's header, the same pattern as
// "From yesterday". It lists what Claude Code and Git say you did that day (see ActivityDigest) as checkable lines; "Insert
// selected" adds the ticked ones to the page under the Sources heading ("Worked on" by default) as ordinary bullets you can edit,
// and "Not today" hides the strip for that day. It is not drawn at all unless a source is on and the consent was given.
import SwiftUI

struct SuggestionsStrip: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    @ObservedObject var sources: SourcesController
    @AppStorage("suggestionsExpanded") private var expanded = true

    init(model: AppModel, editor: DayEditor) {
        self.model = model; self.editor = editor
        _sources = ObservedObject(wrappedValue: model.sources)
    }

    var body: some View {
        if sources.undoDay == editor.day {
            SlimNotice(text: "Suggestions hidden for \(model.shortDate(editor.day)).") {
                Button("Undo") { sources.undoHide(editor.day) }.buttonStyle(TextButtonStyle())
            }.padding(.top, Theme.s3)
        } else if sources.isActive(for: editor.day) {
            strip
        }
    }

    private var ready: Bool { sources.loadedDay == editor.day }

    @ViewBuilder private var strip: some View {
        let items = ready ? sources.bullets : []
        VStack(alignment: .leading, spacing: Theme.s2) {
            header(items)
            if expanded { content(items) }
        }
        .padding(.vertical, 5).padding(.horizontal, Theme.s3)
        .background(Theme.rect(8).fill(Theme.sidebar))
        .padding(.top, Theme.s3)
        .accessibilityElement(children: .contain)
    }

    private func summary(_ items: [SuggestedBullet]) -> String {
        if sources.loading || !ready {
            return sources.progressTotal > 0 ? "reading \(sources.progressDone) of \(sources.progressTotal)" : "looking at your work"
        }
        return items.isEmpty ? "nothing found" : Fmt.plural(items.count, "item")
    }

    // MARK: header
    private func header(_ items: [SuggestedBullet]) -> some View {
        HStack(spacing: Theme.s3) {
            Button { expanded.toggle() } label: {
                HStack(spacing: 6) {
                    Image(systemName: expanded ? "chevron.down" : "chevron.right").font(Theme.font(10, .semibold)).frame(width: 10)
                    Text("Suggested from your work").font(Theme.font(13, .semibold))
                    Text("· \(summary(items))").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
                }
                .foregroundColor(Theme.textPrimary).contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Suggested from your work, \(summary(items)), \(expanded ? "expanded" : "collapsed")")
            Spacer()
            if sources.loading || !ready {
                ProgressView().controlSize(.small)
                Button("Stop") { sources.cancel() }.buttonStyle(TextButtonStyle(color: Theme.textSecondary))
            } else {
                Button { sources.refresh(force: true) } label: {
                    Image(systemName: "arrow.clockwise").font(Theme.font(12, .semibold)).foregroundColor(Theme.textSecondary).frame(width: 22, height: 22)
                }.buttonStyle(.plain).help("Look again").accessibilityLabel("Look again")
                Button("Not today") { sources.hideToday() }.buttonStyle(TextButtonStyle(color: Theme.textSecondary))
                    .help("Hide these suggestions for \(model.shortDate(editor.day))")
                if !items.isEmpty {
                    Button("Insert selected") { sources.insertSelected() }.buttonStyle(SecondaryButtonStyle())
                        .disabled(sources.selected.isEmpty || !editor.loaded || sources.inserting)
                        .accessibilityLabel("Insert \(Fmt.plural(sources.selected.count, "selected line")) into the page")
                }
            }
        }
    }

    // MARK: rows and notes
    @ViewBuilder private func content(_ items: [SuggestedBullet]) -> some View {
        let prefs = model.settings.sources
        VStack(alignment: .leading, spacing: 3) {
            if sources.loading || !ready {
                Text("Reading your work for \(model.shortDate(editor.day)). Nothing leaves this Mac.").font(Theme.font(12)).foregroundColor(Theme.textSecondary)
            } else if prefs.allowed.isEmpty {
                HStack(spacing: Theme.s2) {
                    Text("No projects are ticked yet, so nothing is read.").font(Theme.font(13)).foregroundColor(Theme.textPrimary)
                    Button("Choose projects…") { model.openSettings(pane: .sources) }.buttonStyle(TextButtonStyle())
                }
            } else if items.isEmpty {
                Text("No activity found for \(model.shortDate(editor.day)).").font(Theme.font(13)).foregroundColor(Theme.textPrimary)
            } else {
                rows(items)
                HStack {
                    Spacer()
                    Button(sources.selected.isSuperset(of: Set(items.map { $0.id })) ? "Clear" : "Select all") { sources.toggleAll(items) }
                        .buttonStyle(TextButtonStyle(color: Theme.textSecondary))
                }
            }
            ForEach(sources.notes, id: \.self) { n in
                Text(n).font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            }
            if !(sources.loading || !ready) {
                let hidden = sources.hiddenLine.map { " \($0)." } ?? ""
                Text(items.isEmpty ? "Read-only. Nothing leaves this Mac.\(hidden)"
                                   : "Lines go under \(prefs.headingText) as ordinary bullets you can edit. Read-only. Nothing leaves this Mac.\(hidden)")
                    .font(Theme.font(12)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.leading, 16).padding(.bottom, 4)
    }

    @ViewBuilder private func rows(_ items: [SuggestedBullet]) -> some View {
        let page = ActivityDigest.plainText(editor.latest)
        let list = VStack(alignment: .leading, spacing: 4) {
            ForEach(items) { b in row(b, added: ActivityDigest.isAdded(b, inPlainPage: page)) }
        }
        if items.count > 8 { ScrollView { list.frame(maxWidth: .infinity, alignment: .leading) }.frame(height: 240) } else { list }
    }

    private func row(_ b: SuggestedBullet, added: Bool) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Toggle(isOn: Binding(get: { sources.selected.contains(b.id) },
                                 set: { on in if on { sources.selected.insert(b.id) } else { sources.selected.remove(b.id) } })) { EmptyView() }
                .toggleStyle(.checkbox).labelsHidden().disabled(added).accessibilityLabel(b.text)
            VStack(alignment: .leading, spacing: 1) {
                Text(b.text).font(Theme.font(13)).foregroundColor(added ? Theme.textSecondary : Theme.textPrimary)
                    .lineLimit(2).fixedSize(horizontal: false, vertical: true)
                if !b.detail.isEmpty {
                    Text(b.detail).font(Theme.font(12)).foregroundColor(Theme.textSecondary).lineLimit(1).truncationMode(.middle)
                }
            }
            Spacer(minLength: 8)
            if added { Text("Added").font(Theme.font(12)).foregroundColor(Theme.accentText) }
        }
    }
}
