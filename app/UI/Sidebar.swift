// Sidebar.swift - search, Today / Catch up (N) / Review, the month calendar, the 7 newest pages, and a footer with the
// streak and the Settings gear. The 12-week heatmap lives in the streak popover (footer, and the chip on Today).
// Under about 580 pt of usable height (a 600 pt window: the title bar takes the rest) the calendar is a one-week strip.
import SwiftUI

struct SidebarView: View {
    @ObservedObject var model: AppModel
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    var body: some View {
        GeometryReader { geo in
            VStack(spacing: 0) {
                SearchField(model: model).padding(.horizontal, Theme.s3).padding(.top, Theme.s3).padding(.bottom, Theme.s2)
                ScrollView {
                    VStack(alignment: .leading, spacing: 1) {
                        TodayRow(model: model)
                        CatchUpRow(model: model)
                        ReviewRow(model: model)
                        SidebarCalendar(model: model, compact: geo.size.height - geo.safeAreaInsets.top < 580)
                            .padding(.horizontal, Theme.s1).padding(.top, Theme.s3)
                        RecentList(model: model)
                    }.padding(.horizontal, Theme.s2).padding(.bottom, Theme.s3)
                }
                SidebarFooter(model: model)
            }
        }
        // The split view's own sidebar material shows through the wash; Reduce Transparency makes it solid.
        .background(Theme.sidebar.opacity(reduceTransparency ? 1 : Theme.sidebarWash).ignoresSafeArea())
    }
}

struct SearchField: View {
    @ObservedObject var model: AppModel
    @FocusState private var focused: Bool
    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "magnifyingglass").font(Theme.font(12)).foregroundColor(Theme.textSecondary).accessibilityHidden(true)
            TextField("Search your logs", text: $model.searchText)
                .textFieldStyle(.plain).font(Theme.font(13)).focused($focused)
                .onExitCommand { model.clearSearch() }
                .accessibilityLabel("Search your logs")
            if !model.searchText.isEmpty {
                Button { model.clearSearch() } label: {
                    Image(systemName: "xmark.circle.fill").font(Theme.font(12)).foregroundColor(Theme.textSecondary)
                }.buttonStyle(.plain).accessibilityLabel("Clear search").help("Clear search")
            }
        }
        .padding(.horizontal, Theme.s2).frame(height: 28)
        .background(Theme.rect().fill(Theme.surface))
        .overlay(Theme.rect().stroke(focused ? Theme.accent : Theme.border, lineWidth: focused ? 1.5 : 1))
        .onChange(of: model.searchFocusTick) { _ in focused = true }
    }
}

struct SidebarRow: View {
    let symbol: String
    let color: Color
    let title: String
    var trailing: String?
    var badge: Int?
    var badgeHelp: String?
    var weight: Font.Weight = .regular
    var height: CGFloat = 30
    let selected: Bool
    let label: String
    let action: () -> Void
    var body: some View {
        HoverReader { hovering in
            Button(action: action) {
                HStack(spacing: Theme.s2) {
                    StatusGlyph(symbol: symbol, color: color)
                    Text(title).font(Theme.font(13, selected ? .medium : weight)).foregroundColor(Theme.textPrimary).lineLimit(1)
                    Spacer(minLength: 4)
                    if let t = trailing { Text(t).font(Theme.font(11)).foregroundColor(Theme.textSecondary).monospacedDigit() }
                    if let n = badge, n > 0 { CountBadge(count: n).help(badgeHelp ?? "") }
                }
                .padding(.horizontal, 10).frame(height: height)
                .background(Theme.rect().fill(selected ? Theme.accentTint : (hovering ? Theme.hover : Color.clear)))
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(label).accessibilityAddTraits(selected ? .isSelected : [])
        }
    }
}

/// The only number in the sidebar that asks for action: amber on its tint, hidden at 0, "99+" at most.
struct CountBadge: View {
    let count: Int
    var body: some View {
        Text(count > 99 ? "99+" : "\(count)")
            .font(Theme.font(11, .semibold)).monospacedDigit().foregroundColor(Theme.warning)
            .padding(.horizontal, 6).frame(minWidth: Theme.badgeH, minHeight: Theme.badgeH)
            .background(Capsule().fill(Theme.warningTint))
            .accessibilityHidden(true)
    }
}

struct TodayRow: View {
    @ObservedObject var model: AppModel
    var body: some View {
        let st = model.status(of: model.today)
        let (sym, col): (String, Color) = {
            switch st {
            case .logged: return ("checkmark.circle.fill", Theme.accent)
            case .skipped: return ("minus.circle", Theme.textSecondary)
            case .off: return ("moon.zzz", Theme.textSecondary)
            case .partial: return ("circle.lefthalf.filled", Theme.textSecondary)
            default: return ("circle", Theme.textSecondary)
            }
        }()
        let state = st == .logged ? "logged" : st == .skipped ? "skipped" : st == .off ? "not a workday" : st == .partial ? "started, not logged yet" : "not logged"
        SidebarRow(symbol: sym, color: col, title: "Today", weight: .semibold,
                   selected: model.selection == .day(model.today), label: "Today, \(state)") { model.openToday() }
            .help("Today (⌘T)")
    }
}

struct CatchUpRow: View {
    @ObservedObject var model: AppModel
    var body: some View {
        let n = model.catchUpDays.count
        SidebarRow(symbol: "calendar.badge.exclamationmark", color: Theme.textSecondary, title: "Catch up", badge: n,
                   badgeHelp: "\(Fmt.plural(n, "unlogged day")) need a page",
                   selected: model.selection == .catchUp || model.session != nil,
                   label: n == 0 ? "Catch up, nothing to catch up" : "Catch up, \(Fmt.plural(n, "unlogged day"))") { model.select(.catchUp) }
            .help(n == 0 ? "Catch up (⌘2)" : "\(Fmt.plural(n, "unlogged day")) (⌘2)")
    }
}

struct ReviewRow: View {
    @ObservedObject var model: AppModel
    var body: some View {
        SidebarRow(symbol: "doc.richtext", color: Theme.textSecondary, title: "Review", selected: model.selection == .week,
                   label: "Review, weekly review") { model.select(.week) }
            .help("Review: week, month, year or range (⌘3)")
    }
}

/// The 7 newest pages (not today). The calendar above reaches everything older.
struct RecentList: View {
    @ObservedObject var model: AppModel
    var body: some View {
        let days = Array(model.historyDays.prefix(7))
        VStack(alignment: .leading, spacing: 1) {
            Text("Recent").font(Theme.font(12, .semibold)).foregroundColor(Theme.textSecondary)
                .padding(.horizontal, 10).frame(height: 28, alignment: .leading).accessibilityAddTraits(.isHeader)
            if days.isEmpty {
                Text("Pages you write appear here.").font(Theme.font(12)).foregroundColor(Theme.textSecondary)
                    .padding(.horizontal, 10).padding(.vertical, Theme.s1)
            } else {
                ForEach(days, id: \.self) { d in HistoryRow(model: model, day: d) }
            }
        }.padding(.top, Theme.s2)
    }
}

struct HistoryRow: View {
    @ObservedObject var model: AppModel
    let day: String
    var body: some View {
        let st = model.states[day]
        let (sym, col, word): (String, Color, String) = {
            switch st {
            case .some(.logged): return ("checkmark.circle.fill", Theme.accent, "logged")
            case .some(.skipped): return ("minus.circle", Theme.textSecondary, "skipped")
            default: return ("circle.lefthalf.filled", Theme.textSecondary, "started, not logged")
            }
        }()
        let reason = model.skipReasons[day] ?? ""
        SidebarRow(symbol: sym, color: col, title: model.shortDate(day),
                   trailing: st == .skipped ? "skipped" : nil,
                   height: 28,
                   selected: model.selection == .day(day),
                   label: "\(model.spokenDate(day)), \(word)\(st == .skipped && !reason.isEmpty ? ", \(reason)" : "")") {
            model.select(.day(day))
        }
        .contextMenu {
            Button("Show in Finder") { model.reveal([model.store.url(for: day)]) }
            if st != .some(.skipped) {
                Button("Copy as Markdown") {
                    if let doc = (try? model.store.load(day)) ?? nil {
                        NSPasteboard.general.clearContents(); NSPasteboard.general.setString(doc.body, forType: .string)
                    }
                }
            }
            if st != .some(.logged) && st != .some(.skipped) { Button("Skip this day") { model.requestSkip(day: day) } }
        }
    }
}

/// The streak (click for the 12 weeks) and the always-visible Settings gear: the main window's entry to Settings.
struct SidebarFooter: View {
    @ObservedObject var model: AppModel
    @StateObject private var open = Box(false)
    var body: some View {
        let n = model.streak.current
        VStack(spacing: 0) {
            if model.notesWaiting > 0 { JotsWaitingRow(model: model) }
            HStack(spacing: Theme.s2) {
                Button { open.value.toggle() } label: {
                    Text(n == 0 ? "No streak yet" : "\(n) day streak")
                        .font(Theme.font(13, .semibold)).monospacedDigit().foregroundColor(Theme.textPrimary)
                        .frame(height: 28, alignment: .leading).contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help("Your streak and the last 12 weeks")
                .accessibilityLabel(n == 0 ? "No streak yet" : "\(n) day streak. Best \(model.streak.best).")
                .accessibilityHint("Shows the last 12 weeks")
                .popover(isPresented: Binding(get: { open.value }, set: { open.value = $0 }), arrowEdge: .top) {
                    StreakPopover(model: model) { day in open.value = false; model.select(.day(day)) }
                }
                Spacer()
                Button { model.openSettings() } label: {
                    Image(systemName: "gearshape").font(Theme.font(14)).foregroundColor(Theme.textSecondary)
                        .frame(width: 28, height: 28).contentShape(Rectangle())
                }
                .buttonStyle(.plain).help("Settings (⌘,)").accessibilityLabel("Settings")
            }
            .padding(.horizontal, Theme.s3).frame(height: 40)
        }
        .overlay(Rectangle().fill(Theme.border).frame(height: 1), alignment: .top)
    }
}

/// "2 jots waiting": notes that are safe on this Mac (the capture journal) but not in a page yet, because the log folder was
/// unavailable. Quiet by design (one line, no colour); it goes away by itself when the folder returns and the notes land.
struct JotsWaitingRow: View {
    @ObservedObject var model: AppModel
    var body: some View {
        HStack(spacing: Theme.s2) {
            Image(systemName: "tray.full").font(Theme.font(12)).foregroundColor(Theme.textSecondary).accessibilityHidden(true)
            Text(model.notesWaitingLabel).font(Theme.font(12)).foregroundColor(Theme.textSecondary).lineLimit(1)
            Spacer(minLength: Theme.s1)
            Button("Try now") { model.replayCapture(); model.retryFolder() }.buttonStyle(TextButtonStyle())
                .accessibilityLabel("Try adding the waiting jots now")
        }
        .padding(.horizontal, Theme.s3).frame(height: 28)
        .help("Saved on this Mac. They are added to today's page when your log folder is back.")
        .accessibilityElement(children: .contain)
    }
}

/// One quiet line for the menu bar popover: streak and best.
struct StreakLine: View {
    @ObservedObject var model: AppModel
    var body: some View {
        let s = model.streak
        HStack(alignment: .firstTextBaseline) {
            Text(s.current == 0 ? "No streak yet" : "\(s.current) day streak")
                .font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary).monospacedDigit()
            Spacer()
            if s.best > 0 { Text("Best: \(s.best)").font(Theme.font(11)).foregroundColor(Theme.textSecondary).monospacedDigit() }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(s.current == 0 ? "No streak yet" : "\(s.current) day streak. Best \(s.best).")
    }
}
