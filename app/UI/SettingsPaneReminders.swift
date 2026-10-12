// SettingsPaneReminders.swift - Settings > Reminders: when, which days, Strict or Gentle, snooze, notification status.
import SwiftUI

/// Seven day buttons in the calendar's week order (so they follow "Week starts on"). At least one stays on.
struct WeekdayToggles: View {
    @Binding var days: Set<Int>
    let cal: Calendar
    var body: some View {
        HStack(spacing: 6) {
            ForEach(0..<7, id: \.self) { i in
                let wd = (cal.firstWeekday - 1 + i) % 7 + 1
                let on = days.contains(wd)
                let full = cal.weekdaySymbols[wd - 1]
                Toggle(String(cal.veryShortWeekdaySymbols[wd - 1]), isOn: Binding(
                    get: { on },
                    set: { v in if v { days.insert(wd) } else if days.count > 1 { days.remove(wd) } }))
                    .toggleStyle(.button).frame(minWidth: 30)
                    .disabled(on && days.count == 1)
                    .accessibilityLabel(full).accessibilityValue(on ? "on" : "off")
                    .help(on && days.count == 1 ? "Keep at least one day." : full)
            }
        }
    }
}

struct RemindersSettings: View {
    @ObservedObject var model: AppModel

    private var timeBinding: Binding<Date> {
        Binding(get: { model.cal.date(bySettingHour: model.settings.reminderMinutes / 60, minute: model.settings.reminderMinutes % 60, second: 0, of: model.now) ?? model.now },
                set: { d in let c = model.cal.dateComponents([.hour, .minute], from: d); model.settings.reminderMinutes = (c.hour ?? 16) * 60 + (c.minute ?? 55) })
    }

    private var styleHelp: String {
        let past = "Past days are never forced on you; they wait in Catch up."
        return model.settings.mode == .strict
            ? "Strict brings Gloamlog to the front, then reopens it every 5 minutes if you close it, until you save or skip today. Two snoozes a day. \(past)"
            : "Gentle sends one notification and nothing else. Snooze as often as you like. \(past)"
    }

    var body: some View {
        SettingsPaneFrame(model: model, pane: .reminders, resetSummary: "the reminder time, days, style, snooze length and closed-app reminders") {
            Section {
                SettingsRow("Remind me at", help: "Next reminder: \(model.nextReminderText)") {
                    DatePicker("Remind me at", selection: timeBinding, displayedComponents: .hourAndMinute).labelsHidden().fixedSize()
                }
                SettingsRow("On these days", help: "Catch up and your streak count the same days. Keep at least one.") {
                    WeekdayToggles(days: $model.settings.weekdays, cal: model.cal)
                }
            } header: { Text("End of day").font(Theme.font(13, .semibold)) }

            Section {
                SettingsRow("Style", help: styleHelp) {
                    Picker("Style", selection: $model.settings.mode) {
                        Text("Strict").tag(ReminderMode.strict)
                        Text("Gentle").tag(ReminderMode.gentle)
                    }.pickerStyle(.segmented).labelsHidden().fixedSize()
                }
                SettingsRow("Snooze for") {
                    Picker("Snooze for", selection: $model.settings.snoozeMinutes) {
                        ForEach(Settings.snoozeChoices, id: \.self) { Text("\($0) minutes").tag($0) }
                    }.labelsHidden().fixedSize()
                }
            } header: { Text("When it fires").font(Theme.font(13, .semibold)) }

            Section {
                notificationStatus
                SettingsRow("Remind me when Gloamlog is closed", help: closedHelp) {
                    Toggle("Remind me when Gloamlog is closed", isOn: $model.settings.closedAppReminders)
                        .labelsHidden().toggleStyle(.switch).disabled(model.notifState == .denied)
                }
            } header: { Text("Notifications").font(Theme.font(13, .semibold)) }
        }
        .onAppear { Notifier.shared.refreshState { s in model.notifState = s } }
    }

    /// The limits are stated here (PRD M5a): macOS holds 14 days of reminders, a sleeping Mac delivers late, a Focus can silence them.
    private var closedHelp: String {
        if model.notifState == .denied { return "Allow notifications for Gloamlog in System Settings first." }
        return model.settings.closedAppReminders
            ? "macOS holds your next 14 reminders, so one arrives even when Gloamlog is quit. Nothing arrives while the Mac sleeps (it comes when it wakes), and a Focus can silence it. Strict's window needs Gloamlog running. Open Gloamlog now and then to renew the reminders."
            : "Reminders arrive only while Gloamlog is running. Turn on Open at login (General) to keep it running after a restart."
    }

    @ViewBuilder private var notificationStatus: some View {
        switch model.notifState {
        case .allowed:
            SettingsStatus(symbol: "bell", "Notifications are on for Gloamlog.")
        case .denied:
            SettingsStatus(symbol: "bell.slash", tint: Theme.warning, "Notifications are off for Gloamlog. Reminders can't reach you while it's closed.") {
                Button("Open System Settings") { model.openNotificationSettings() }
            }
        case .unknown:
            SettingsStatus(symbol: "bell", "Gloamlog hasn't asked to send notifications yet.") {
                Button("Allow") { Notifier.shared.requestAuthorization { _ in Notifier.shared.refreshState { s in model.notifState = s } } }
            }
        }
    }
}
