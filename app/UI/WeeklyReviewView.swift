// WeeklyReviewView.swift - the pieces the Review screen is made of that were born with the week review: the day tile, the card frame
// and the clamped markdown. The screen itself is ReviewView (M3); WeeklyReviewView stays as its old name so the snapshot and window
// harnesses, which build it directly, keep compiling.
import SwiftUI

struct WeeklyReviewView: View {
    @ObservedObject var model: AppModel
    var body: some View { ReviewView(model: model) }
}

struct WeekTileView: View {
    @ObservedObject var model: AppModel
    let t: WeekTile

    var body: some View {
        HoverReader { hovering in
            Button { if t.opens { model.select(.day(t.day)) } } label: {
                VStack(spacing: 3) {
                    Text(t.weekday).font(Theme.font(11)).foregroundColor(Theme.textSecondary)
                    HStack(spacing: 4) {
                        Text("\(t.number)").font(Theme.font(15, .semibold)).monospacedDigit().foregroundColor(Theme.textPrimary)
                        DayMark(status: t.status, size: 8, showsMissed: !t.isToday)
                    }
                    Text(t.bottom).font(Theme.font(11, t.status == .missed ? .semibold : .regular)).lineLimit(1)
                        .foregroundColor(t.status == .missed ? Theme.accentText : Theme.textSecondary)
                        .frame(height: 14)
                }
                .frame(maxWidth: .infinity).frame(height: Theme.weekTileH)
                .background(Theme.rect(Theme.radiusLg).fill(hovering && t.opens ? Theme.hover : Theme.surface))
                .overlay(Theme.rect(Theme.radiusLg).stroke(t.isToday ? Theme.accent : Theme.border, lineWidth: t.isToday ? 1.5 : 1))
                .opacity(t.status == .off ? 0.65 : (t.opens ? 1 : 0.5))
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help(t.opens ? "\(model.shortDate(t.day)): \(model.statusWord(t.day))" : "Upcoming")
            .accessibilityLabel(model.spokenStatus(t.day))
            .accessibilityHint(t.opens ? "Opens this day" : "")
        }
    }
}

struct ReviewCard<Content: View>: View {
    @ViewBuilder let content: () -> Content
    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s3, content: content)
            .padding(Theme.s4).frame(maxWidth: .infinity, alignment: .leading)
            .background(Theme.rect(Theme.radiusLg).fill(Theme.surface))
            .overlay(Theme.rect(Theme.radiusLg).stroke(Theme.border, lineWidth: 1))
            .accessibilityElement(children: .contain)
    }
}

/// A page's markdown, cut at a block boundary with Show more when long (never mid-line).
struct ClampedMarkdown: View {
    let markdown: String
    let store: AssetStore
    var maxLines: Double = 9
    @StateObject private var open = Box(false)

    private func clamp(_ all: [MDBlock]) -> (shown: [MDBlock], long: Bool) {
        var used = 0.0, cut = all.count
        for (i, b) in all.enumerated() {
            used += b.estimatedLines
            if used > maxLines && i > 0 { cut = i; break }
        }
        var shown = Array(all.prefix(cut))
        while case .heading? = shown.last, cut < all.count { shown.removeLast() }       // no heading left hanging
        return (shown, cut < all.count)
    }

    var body: some View {
        let all = MDParse.blocks(markdown)
        let c = clamp(all)
        VStack(alignment: .leading, spacing: 4) {
            MarkdownBlocksView(blocks: open.value ? all : c.shown, store: store, size: 14, compact: true)
            if c.long { Button(open.value ? "Show less" : "Show more") { open.value.toggle() }.buttonStyle(TextButtonStyle()) }
        }
    }
}
