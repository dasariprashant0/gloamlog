// Notifier.swift - UserNotifications wrapper. Every call degrades silently: an ad-hoc-signed app may be denied or
// unable to register, and the in-window banner + menu bar icon remain the fallback signal.
import Foundation
import UserNotifications
import AppKit

final class Notifier: NSObject, UNUserNotificationCenterDelegate {
    static let shared = Notifier()
    private weak var model: AppModel?
    private let id = "dailylog.reminder"       // one notification, replaced not stacked
    /// nil in a bare binary (no bundle): every call below degrades to nothing. Also used by ClosedAppReminders.
    var center: UNUserNotificationCenter? {
        Bundle.main.bundleIdentifier == nil || Bundle.main.bundlePath.hasSuffix(".app") == false ? nil : UNUserNotificationCenter.current()
    }
    /// The last request for permission was answered with no (or could not be asked at all, as an ad-hoc build may find).
    private var refused = false

    func setup(model: AppModel) {
        self.model = model
        guard let c = center else { return }
        c.delegate = self
        let open = UNNotificationAction(identifier: "open", title: "Open", options: [.foreground])
        let snooze = UNNotificationAction(identifier: "snooze", title: "Snooze \(model.settings.snoozeMinutes) min", options: [])
        let skip = UNNotificationAction(identifier: "skip", title: "Skip today", options: [])
        c.setNotificationCategories([UNNotificationCategory(identifier: ReminderScheduler.category, actions: [open, snooze, skip],
                                                            intentIdentifiers: [], options: [])])
    }

    func requestAuthorization(_ done: @escaping (Bool) -> Void = { _ in }) {
        guard let c = center else { done(false); return }
        c.requestAuthorization(options: [.alert]) { [weak self] ok, _ in DispatchQueue.main.async { self?.refused = !ok; done(ok) } }
    }

    func refreshState(_ done: @escaping (NotifState) -> Void) {
        guard let c = center else { done(.unknown); return }
        c.getNotificationSettings { [weak self] s in
            DispatchQueue.main.async {
                switch s.authorizationStatus {
                case .denied: done(.denied)
                case .authorized, .provisional: done(.allowed)
                default: done(self?.refused == true ? .denied : .unknown)       // asked, and macOS (or an ad-hoc build) said no
                }
            }
        }
    }

    /// Content never includes log text, streak numbers or anything personal.
    func post(title: String, body: String, snoozeMinutes: Int) {
        guard let c = center else { return }
        let content = UNMutableNotificationContent()
        content.title = title; content.body = body; content.categoryIdentifier = ReminderScheduler.category
        // One notification a day: if macOS just delivered the scheduled one, that one stands (ClosedAppReminders.postInApp).
        ClosedAppReminders.shared.postInApp(UNNotificationRequest(identifier: id, content: content, trigger: nil), center: c)
    }

    func cancelAll() {
        guard let c = center else { return }
        c.removePendingNotificationRequests(withIdentifiers: [id]); c.removeDeliveredNotifications(withIdentifiers: [id])
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                withCompletionHandler done: @escaping (UNNotificationPresentationOptions) -> Void) { done([.banner]) }

    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                withCompletionHandler done: @escaping () -> Void) {
        let action = response.actionIdentifier, info = response.notification.request.content.userInfo
        DispatchQueue.main.async { [weak self] in
            // Open, Snooze, Skip today and a click on the banner (also the one that launched the app): ClosedAppReminders.handle.
            if let m = self?.model { ClosedAppReminders.shared.handle(action: action, userInfo: info, model: m) }
            done()
        }
    }
}
