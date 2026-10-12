// ReviewView.swift - the Review screen (M3, DESIGN_V4 section 5): Week, Month, Year and a custom Range in one place.
// Title and ‹ This week › on top, the counts line ("Logged 4 of 5 · 1 skipped · 2 not logged", never "missed"), then the scale control,
// [Catch up] and the Export menu; [Last week] [This month], the Skipped days and Jots checkboxes and Group by Day or Section.
// A week keeps its seven tiles. Month and Range show cards by Day or by Section (each item dated); a Year is 12 rows, one click opens a month.
// What is shown is exactly what Copy, Save, PDF and Share write (ReviewController builds one summary for all of them).
import SwiftUI

struct ReviewView: View {
    @ObservedObject var model: AppModel
    @ObservedObject var review: ReviewController

    init(model: AppModel) { self.model = model; self.review = model.review }

    /// The summary of the period on screen; nil while a long range is still being read.
    private var ready: ReviewSummary? {
        if let s = review.shown, s.period == review.period { return s }
        return nil
    }

    var body: some View {
        ScrollView {
            Column {
                header
                notices
                controls
                if review.scale == .week { tiles }
                content
            }
        }
        .background(Theme.bg)
        .onAppear { model.refreshWeek() }
        .sheet(isPresented: $review.exportOpen) { ExportSheet(model: model, review: review) }
    }

    // MARK: header
    private func countsLine(_ s: ReviewSummary) -> String {
        if let l = ReviewFormats.countsLine(s.counts) { return l }
        if s.period.scale == .week, model.weekTiles().allSatisfy({ $0.status == .off || $0.status == .future }) { return "No working days to count yet." }
        return "Nothing to count yet."
    }

    private var header: some View {
        let noun = review.scale == .range ? "period" : review.scale.rawValue
        return VStack(alignment: .leading, spacing: Theme.s2) {
            HStack(alignment: .firstTextBaseline) {
                Text(ReviewFormats.title(review.period, calendar: model.cal)).font(Theme.font(28, .semibold)).tracking(-0.4)
                    .foregroundColor(Theme.textPrimary).lineLimit(1).minimumScaleFactor(0.7).accessibilityAddTraits(.isHeader)
                Spacer()
                HStack(spacing: 4) {
                    Button { review.step(-1) } label: { Image(systemName: "chevron.left") }
                        .accessibilityLabel("Previous \(noun)").help("Previous \(noun) (⌘[)").disabled(!review.canStep(-1))
                    if review.scale != .range {
                        Button("This \(noun)") { review.goThis() }.disabled(review.isCurrent)
                    }
                    Button { review.step(1) } label: { Image(systemName: "chevron.right") }
                        .accessibilityLabel("Next \(noun)").help("Next \(noun) (⌘])").disabled(!review.canStep(1))
                }.buttonStyle(SecondaryButtonStyle())
            }
            if let s = ready {
                Text(countsLine(s)).font(Theme.font(13)).foregroundColor(Theme.textSecondary).monospacedDigit()
                if let t = ReviewFormats.tasksLine(s.tasks) { Text(t).font(Theme.font(13)).foregroundColor(Theme.textSecondary).monospacedDigit() }
            }
        }.padding(.bottom, Theme.s3)
    }

    // MARK: notices (saved, failed, working)
    @ViewBuilder private var notices: some View {
        if review.busy {
            HStack(spacing: Theme.s2) {
                ProgressView().controlSize(.small)
                Text("Making the PDF…").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
            }.padding(.bottom, Theme.s3)
        }
        if let f = review.failure {
            Banner(.error, f.text, detail: f.detail, onDismiss: { review.dismissResult() }) {
                Button("Try again") { review.retry() }.buttonStyle(TextButtonStyle())
                Button("Choose another place…") { review.chooseAnotherPlace() }.buttonStyle(TextButtonStyle())
            }.padding(.bottom, Theme.s3)
        }
        if let line = review.savedLine {
            Banner(.success, line, onDismiss: { review.dismissResult() }) {
                if review.savedURL != nil { Button("Show in Finder") { review.showInFinder() }.buttonStyle(TextButtonStyle()) }
            }.padding(.bottom, Theme.s3)
        }
    }

    // MARK: controls
    private var controls: some View {
        VStack(alignment: .leading, spacing: Theme.s3) {
            HStack(spacing: Theme.s3) {
                Picker("Review period", selection: Binding(get: { review.scale }, set: { review.setScale($0) })) {
                    ForEach(ReviewScale.allCases, id: \.self) { Text($0.title).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().fixedSize().accessibilityLabel("Review period")
                Spacer(minLength: Theme.s2)
                if let s = ready, !s.isEmpty, s.period.scale != .year, s.counts.notWritten > 0 {
                    Button(review.catchUpTitle) { review.catchUp() }.buttonStyle(SecondaryButtonStyle())
                }
                exportMenu
            }
            if review.scale == .range { rangePickers }
            HStack(spacing: Theme.s3) {
                Button("Last week") { review.lastWeek() }.buttonStyle(SecondaryButtonStyle())
                Button("This month") { review.thisMonth() }.buttonStyle(SecondaryButtonStyle())
                Spacer(minLength: Theme.s2)
                Toggle("Skipped days", isOn: $review.includeSkipped).toggleStyle(.checkbox)
                if review.scale != .year {
                    Toggle("Jots", isOn: $review.includeJots).toggleStyle(.checkbox)
                    Text("Group by").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
                    Picker("Group by", selection: Binding(get: { review.grouping }, set: { review.groupingChoice = $0 })) {
                        Text("Day").tag(WeekGrouping.day); Text("Section").tag(WeekGrouping.section)
                    }.pickerStyle(.segmented).labelsHidden().fixedSize().accessibilityLabel("Group by")
                }
            }
        }.padding(.bottom, Theme.s4)
    }

    private var rangePickers: some View {
        HStack(spacing: 6) {
            Text("From").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
            DatePicker("From", selection: Binding(get: { review.rangeFrom }, set: { review.setRange(from: $0) }), in: ...model.now, displayedComponents: .date)
                .datePickerStyle(.field).labelsHidden().accessibilityLabel("From")
            Text("to").font(Theme.font(13)).foregroundColor(Theme.textSecondary)
            DatePicker("To", selection: Binding(get: { review.rangeTo }, set: { review.setRange(to: $0) }), in: ...model.now, displayedComponents: .date)
                .datePickerStyle(.field).labelsHidden().accessibilityLabel("To")
        }
        .environment(\.calendar, model.cal).environment(\.timeZone, model.cal.timeZone)
    }

    private var exportMenu: some View {
        Menu {
            Button("Copy as Rich Text") { review.copy(.rich) }
            Button("Copy as Markdown") { review.copy(.markdown) }
            Button("Copy as Plain Text") { review.copy(.plain) }
            Divider()
            Button("Save as Markdown…") { review.save(.markdown) }
            Button("Save as PDF…") { review.save(.pdf) }
            Button("Share…") { review.share() }
            Divider()
            Button("More Export Options…") { review.exportOpen = true }
        } label: {
            HStack(spacing: 5) {
                Image(systemName: model.copiedFlash ? "checkmark" : "square.and.arrow.up").font(Theme.font(12))
                Text(model.copiedFlash ? "Copied ✓" : "Export").font(Theme.font(13))
                Image(systemName: "chevron.down").font(Theme.font(9, .semibold)).foregroundColor(Theme.textSecondary)
            }
            .foregroundColor(Theme.textPrimary)
            .padding(.horizontal, Theme.s3).frame(minHeight: 28)
            .background(Theme.rect().fill(Theme.surface))
            .overlay(Theme.rect().stroke(Theme.borderStrong, lineWidth: 1))
            .contentShape(Rectangle())
        }
        .menuStyle(.borderlessButton).menuIndicator(.hidden).fixedSize()
        .background(ShareAnchor(review: review))
        .help("Copy, save or share this review (⇧⌘E)")
        .accessibilityLabel("Export")
    }

    // MARK: week tiles
    /// Seven tiles, Monday (or the chosen first weekday) to Sunday. Each opens its day; days off are dimmed but still open.
    private var tiles: some View {
        HStack(spacing: Theme.weekTileGap) {
            ForEach(model.weekTiles()) { t in WeekTileView(model: model, t: t).frame(maxWidth: .infinity) }
        }
        .accessibilityElement(children: .contain).accessibilityLabel("Days of this week")
        .padding(.bottom, Theme.s6)
    }

    // MARK: content
    @ViewBuilder private var content: some View {
        if let s = ready {
            if s.isEmpty { empty(s) }
            else if s.period.scale == .year { YearTable(model: model, review: review, rows: s.months) }
            else if s.options.grouping == .day {
                LazyVStack(alignment: .leading, spacing: Theme.s4) { ForEach(s.days, id: \.day) { ReviewDayCard(model: model, d: $0) } }
            } else {
                LazyVStack(alignment: .leading, spacing: Theme.s4) {
                    ForEach(Array(s.groups.enumerated()), id: \.offset) { _, g in ReviewGroupCard(model: model, group: g) }
                }
            }
        } else {
            ProgressView().controlSize(.small).frame(maxWidth: .infinity).padding(.vertical, Theme.s8)
        }
    }

    private func emptyText(_ p: ReviewPeriod) -> String {
        switch p.scale {
        case .week: return "Nothing logged this week yet."
        case .month: return "Nothing logged in \(DayKey.format(p.start, "MMMM", model.cal)) yet."
        case .year: return "No pages in \(p.start.prefix(4)) yet."
        case .range: return "Nothing logged in this range yet."
        }
    }

    private func empty(_ s: ReviewSummary) -> some View {
        VStack(spacing: Theme.s3) {
            Text(emptyText(s.period)).font(Theme.font(15)).foregroundColor(Theme.textSecondary)
            HStack(spacing: Theme.s3) {
                if s.period.scale != .year, s.counts.notWritten > 0 { Button(review.catchUpTitle) { review.catchUp() }.buttonStyle(PrimaryButtonStyle()) }
                Button("Go to today") { model.openToday() }.buttonStyle(TextButtonStyle())
            }
        }.frame(maxWidth: .infinity).padding(.vertical, Theme.s8)
    }
}

// MARK: cards
/// A day's writing (or its skip), for Group by Day.
struct ReviewDayCard: View {
    @ObservedObject var model: AppModel
    let d: ReviewDay
    var body: some View {
        ReviewCard {
            HStack {
                Text(model.shortDate(d.day)).font(Theme.font(15, .semibold)).foregroundColor(Theme.textPrimary)
                Spacer()
                if d.isSkipped {
                    Text("Skipped" + (d.skipReason.isEmpty ? "" : " · \(d.skipReason)")).font(Theme.font(12)).foregroundColor(Theme.textSecondary)
                } else if d.status == .partial {
                    Text("\(d.words) of \(model.minWords) words").font(Theme.font(12)).foregroundColor(Theme.textSecondary).monospacedDigit()
                }
                Button("Open") { model.select(.day(d.day)) }.buttonStyle(TextButtonStyle()).accessibilityLabel("Open \(model.spokenDate(d.day))")
            }
            if !d.isSkipped { ClampedMarkdown(markdown: d.body, store: model.store.assets) }
        }
    }
}

/// One heading's roll-up across the period (or Jots, Other notes, Skipped days), every item dated, for Group by Section.
struct ReviewGroupCard: View {
    @ObservedObject var model: AppModel
    let group: ReviewGroup
    var body: some View {
        ReviewCard {
            Text(cleanHeading(group.title)).font(Theme.font(15, .semibold)).foregroundColor(Theme.textPrimary).accessibilityAddTraits(.isHeader)
            ForEach(Array(group.items.enumerated()), id: \.offset) { _, it in
                HStack(alignment: .top, spacing: Theme.s3) {
                    Text(DayKey.format(it.day, "EEE d", model.cal)).font(Theme.font(12)).monospacedDigit()
                        .foregroundColor(Theme.textSecondary).frame(width: 56, alignment: .leading)
                    if group.kind == .skipped { Text(it.text).font(Theme.font(14)).foregroundColor(Theme.textSecondary) }
                    else { ClampedMarkdown(markdown: it.text, store: model.store.assets) }
                }
            }
        }
    }
}

// MARK: year
/// 12 rows (header 28, rows 32): Month, Logged, Skipped, Not logged. A row opens that month; a month that has not begun is inert.
struct YearTable: View {
    @ObservedObject var model: AppModel
    @ObservedObject var review: ReviewController
    let rows: [MonthRow]

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                Text("Month").frame(width: 140, alignment: .leading)
                Spacer(minLength: 0)
                ForEach(["Logged", "Skipped", "Not logged"], id: \.self) { Text($0).frame(width: 90, alignment: .trailing) }
            }
            .font(Theme.font(12, .semibold)).foregroundColor(Theme.textSecondary)
            .padding(.horizontal, Theme.s4).frame(height: 28)
            .accessibilityHidden(true)
            ForEach(rows, id: \.month) { r in
                Divider().overlay(Theme.border)
                YearRowView(model: model, review: review, r: r)
            }
        }
        .background(Theme.rect(Theme.radiusLg).fill(Theme.surface))
        .overlay(Theme.rect(Theme.radiusLg).stroke(Theme.border, lineWidth: 1))
        .clipShape(Theme.rect(Theme.radiusLg))
        .accessibilityElement(children: .contain).accessibilityLabel("Months of \(rows.first?.month.prefix(4) ?? "the year")")
    }
}

private struct YearRowView: View {
    @ObservedObject var model: AppModel
    @ObservedObject var review: ReviewController
    let r: MonthRow

    private var name: String { DayKey.format(r.month + "-01", "MMMM", model.cal) }
    private var inert: Bool { r.isFuture || r.counts.total == 0 }
    private func cell(_ n: Int) -> some View {
        Text(inert ? "–" : "\(n)").font(Theme.font(13)).monospacedDigit()
            .foregroundColor(inert ? Theme.textTertiary : Theme.textPrimary).frame(width: 90, alignment: .trailing)
    }
    private var spoken: String {
        if r.isFuture { return "\(name), not started" }
        if r.counts.total == 0 { return "\(name), nothing to count" }
        return "\(name), \(r.counts.logged) logged, \(r.counts.skipped) skipped, \(r.counts.notWritten) not logged"
    }

    var body: some View {
        HoverReader { hovering in
            Button { review.openMonth(r.month) } label: {
                HStack(spacing: 0) {
                    Text(name).font(Theme.font(13)).foregroundColor(r.isFuture ? Theme.textTertiary : Theme.textPrimary).frame(width: 140, alignment: .leading)
                    Spacer(minLength: 0)
                    cell(r.counts.logged); cell(r.counts.skipped); cell(r.counts.notWritten)
                }
                .padding(.horizontal, Theme.s4).frame(height: 32)
                .background(hovering && !r.isFuture ? Theme.hover : Color.clear)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain).disabled(r.isFuture)
            .accessibilityLabel(spoken).accessibilityHint(r.isFuture ? "" : "Opens this month")
        }
    }
}
