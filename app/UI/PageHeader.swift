// PageHeader.swift - big date, then one quiet meta line: streak chip (opens the 12-week heatmap), live word progress,
// and a "Saved" indicator that fades. The "..." menu holds the rarely used actions.
import SwiftUI

struct PageHeader: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor

    var body: some View {
        let thisYear = String(model.today.prefix(4))
        let sameYear = String(editor.day.prefix(4)) == thisYear
        VStack(alignment: .leading, spacing: Theme.s2) {
            HStack(alignment: .firstTextBaseline, spacing: Theme.s3) {
                Text(DayKey.format(editor.day, sameYear ? "EEEE, d MMMM" : "EEEE, d MMMM yyyy", model.cal))
                    .font(Theme.font(32, .semibold)).tracking(-0.5).foregroundColor(Theme.textPrimary)
                    .lineLimit(1).minimumScaleFactor(0.6).accessibilityAddTraits(.isHeader)
                Spacer(minLength: Theme.s4)
                PageMenu(model: model, editor: editor)
            }
            HStack(spacing: Theme.s3) {
                if editor.day == model.today { StreakChip(model: model) }          // the streak belongs to Today, not every page
                if let rel = model.relativeLabel(for: editor.day) {
                    Text(rel).font(Theme.font(13)).foregroundColor(Theme.textSecondary).lineLimit(1)
                }
                WordProgress(model: model, editor: editor)
                Spacer(minLength: Theme.s3)
                SavedIndicator(editor: editor)
            }
            if model.isBeforeLogStart(editor.day) {
                Text("Before your log start. It won't appear in Catch up.").font(Theme.font(12)).foregroundColor(Theme.textSecondary)
            }
            Rectangle().fill(Theme.border).frame(height: 1).padding(.top, Theme.s3)
        }
    }
}

/// "12 day streak" - a button that opens the last 12 weeks. Always present, so the heatmap is always reachable.
struct StreakChip: View {
    @ObservedObject var model: AppModel
    @StateObject private var open = Box(false)

    var body: some View {
        let n = model.streak.current
        Button { open.value.toggle() } label: {
            Text(n == 0 ? "No streak yet" : "\(n) day streak")
                .font(Theme.font(12, .semibold)).monospacedDigit()
                .foregroundColor(n == 0 ? Theme.textSecondary : Theme.accentText)
                .padding(.horizontal, 10).frame(height: 24)
                .background(Capsule().fill(n == 0 ? Theme.hover : Theme.accentTint))
        }
        .buttonStyle(.plain)
        .help("Your streak and the last 12 weeks")
        .accessibilityLabel(n == 0 ? "No streak yet" : "\(n) day streak. Best \(model.streak.best).")
        .accessibilityHint("Shows the last 12 weeks")
        .popover(isPresented: Binding(get: { open.value }, set: { open.value = $0 }), arrowEdge: .bottom) {
            StreakPopover(model: model) { day in open.value = false; model.select(.day(day)) }
        }
    }
}

struct StreakPopover: View {
    @ObservedObject var model: AppModel
    let onSelect: (String) -> Void
    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s3) {
            HStack(alignment: .firstTextBaseline) {
                Text(model.streak.current == 0 ? "No streak yet" : "\(model.streak.current) day streak")
                    .font(Theme.font(15, .semibold)).foregroundColor(Theme.textPrimary).monospacedDigit()
                Spacer()
                if model.streak.best > 0 { Text("Best: \(model.streak.best)").font(Theme.font(12)).foregroundColor(Theme.textSecondary).monospacedDigit() }
            }
            HeatmapView(model: model, cell: 14, gap: 3, onSelect: onSelect)
            Text("A day counts after \(Fmt.plural(model.minWords, "word")). Skipped days don't break the streak.")
                .font(Theme.font(11)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
        }
        .padding(Theme.s4).frame(width: 264)
    }
}

/// "12 words. Counts as logged at 20." until the page has enough words, then "Logged, 63 words". No quota framing. Jots are
/// notes, not words (unless Settings says otherwise), so a day with only jots reads "0 words" and "3 jots": started, not logged.
struct WordProgress: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    private var jots: String? { editor.showsEditor && editor.jotCount > 0 ? Fmt.plural(editor.jotCount, "jot") : nil }
    var body: some View {
        let min = model.minWords
        HStack(spacing: 6) {
            Group {
                if !editor.showsEditor {
                    Text("Skipped" + (editor.skipReason.isEmpty ? "" : " · \(editor.skipReason)")).foregroundColor(Theme.textSecondary)
                } else if editor.isLogged {
                    HStack(spacing: 4) {
                        Image(systemName: "checkmark").font(Theme.font(11, .bold)).accessibilityHidden(true)
                        Text("Logged, \(Fmt.plural(editor.words, "word"))").fontWeight(.semibold)
                    }.foregroundColor(Theme.accentText)
                } else {
                    Text("\(Fmt.plural(editor.words, "word")). Counts as logged at \(min).").foregroundColor(Theme.textSecondary)
                }
            }
            if let j = jots { Text("· \(j)").foregroundColor(Theme.textSecondary) }
        }
        .font(Theme.font(13)).monospacedDigit()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel((!editor.showsEditor ? "Skipped" : editor.isLogged ? "Logged, \(Fmt.plural(editor.words, "word"))."
                             : "\(Fmt.plural(editor.words, "word")). Counts as logged at \(min).") + (jots.map { " \($0)." } ?? ""))
    }
}

/// A quiet "Saved" that fades out by itself. Reduce Motion: it simply appears and disappears.
struct SavedIndicator: View {
    @ObservedObject var editor: DayEditor
    @Environment(\.accessibilityReduceMotion) private var reduce
    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: "checkmark.circle").font(Theme.font(12))
            Text("Saved").font(Theme.font(12))
        }
        .foregroundColor(Theme.textTertiary)
        .opacity(editor.savedFlash ? 1 : 0)
        .animation(Theme.animation(.easeInOut(duration: 0.4), reduce: reduce), value: editor.savedFlash)
        .accessibilityHidden(true)
    }
}

struct PageMenu: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    var body: some View {
        Menu {
            Button("Skip day…") { model.requestSkip(day: editor.day) }.disabled(model.states[editor.day] == .logged)
            Button("Copy page as markdown") { model.copyPageMarkdown() }
            Button("Copy page as plain text") { model.review.copyPage(.plain) }
            Button("Copy page as rich text") { model.review.copyPage(.rich) }
            Button("Open folder") { model.revealPageInFinder() }
            Divider()
            Button("Go to date…") { model.goToDateOpen = true }
            Button("Catch up") { model.select(.catchUp) }
            Divider()
            Button("Restore previous version…") { model.sheet = .restore(editor.day) }
            if let n = editor.backupNotice {
                Divider()
                Text("Backups: \(n)")
            }
            Divider()
            Button("Settings…") { model.openSettings() }
        } label: {
            Image(systemName: "ellipsis").font(Theme.font(15, .semibold)).foregroundColor(Theme.textSecondary)
                .frame(width: 28, height: 28).contentShape(Rectangle())
        }
        .menuStyle(.borderlessButton).menuIndicator(.hidden).fixedSize()
        .help("More")
        .accessibilityLabel("Page options")
    }
}

/// Collapsible one-line strip: yesterday's open items and a "Carry over" button. Hidden when nothing is left to carry.
struct FromYesterdayStrip: View {
    @ObservedObject var model: AppModel
    @ObservedObject var editor: DayEditor
    @AppStorage("yesterdayExpanded") private var expanded = false

    var body: some View {
        if let c = model.carryCard, model.carryAvailable {
            let h = CarryOver.heading(for: c, today: model.today, calendar: model.cal)
            VStack(alignment: .leading, spacing: Theme.s2) {
                HStack(spacing: Theme.s3) {
                    Button { expanded.toggle() } label: {
                        HStack(spacing: 6) {
                            Image(systemName: expanded ? "chevron.down" : "chevron.right").font(Theme.font(10, .semibold)).frame(width: 10)
                            Text(h == "Yesterday" ? "From yesterday" : "From \(h)").font(Theme.font(13, .semibold))
                            Text("\(c.sourceLabel) · \(Fmt.plural(c.items.count, "item"))").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
                        }
                        .foregroundColor(Theme.textPrimary).contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("\(h == "Yesterday" ? "From yesterday" : "From \(h)"), \(Fmt.plural(c.items.count, "item")), \(expanded ? "expanded" : "collapsed")")
                    Spacer()
                    Button("Carry over") { model.carryOver() }.buttonStyle(SecondaryButtonStyle())
                        .disabled(!editor.loaded).accessibilityLabel("Carry over \(Fmt.plural(c.items.count, "item")) to today")
                }
                if expanded {
                    VStack(alignment: .leading, spacing: 3) {
                        ForEach(Array(c.items.prefix(8).enumerated()), id: \.offset) { _, item in
                            HStack(alignment: .firstTextBaseline, spacing: 6) {
                                Text("•").foregroundColor(Theme.textTertiary)
                                Text(item).font(Theme.font(13)).foregroundColor(Theme.textPrimary).lineLimit(2)
                            }
                        }
                        if c.items.count > 8 { Text("and \(c.items.count - 8) more").font(Theme.font(12)).foregroundColor(Theme.textSecondary) }
                    }.padding(.leading, 16).padding(.bottom, 4)
                }
            }
            .padding(.vertical, 5).padding(.horizontal, Theme.s3)
            .background(Theme.rect(8).fill(Theme.sidebar))
            .padding(.top, Theme.s3)
        } else if model.carryUndoBody != nil {
            SlimNotice(text: "Carried over from yesterday.") {
                Button("Undo") { model.undoCarry() }.buttonStyle(TextButtonStyle())
            }.padding(.top, Theme.s3)
        }
    }
}
