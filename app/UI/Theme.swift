// Theme.swift - design tokens (docs/DESIGN_SYSTEM.md section 10), button styles and small shared helpers.
// No macros: view-local state lives in ObservableObjects (see Box / HoverReader).
import SwiftUI
import AppKit

enum Theme {
    static func dyn(_ light: UInt32, _ dark: UInt32) -> Color {
        Color(nsColor: NSColor(name: nil) { a in
            let isDark = a.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
            let v = isDark ? dark : light
            return NSColor(srgbRed: CGFloat((v >> 16) & 0xFF) / 255, green: CGFloat((v >> 8) & 0xFF) / 255,
                           blue: CGFloat(v & 0xFF) / 255, alpha: 1)
        })
    }
    /// Like `dyn`, with an alpha per appearance (the focus ring is the accent at 30% / 40%).
    static func dynAlpha(_ light: UInt32, _ la: CGFloat, _ dark: UInt32, _ da: CGFloat) -> Color {
        Color(nsColor: NSColor(name: nil) { a in
            let isDark = a.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
            let v = isDark ? dark : light
            return NSColor(srgbRed: CGFloat((v >> 16) & 0xFF) / 255, green: CGFloat((v >> 8) & 0xFF) / 255,
                           blue: CGFloat(v & 0xFF) / 255, alpha: isDark ? da : la)
        })
    }
    static let bg = dyn(0xF9FAF9, 0x141716)
    static let surface = dyn(0xFFFFFF, 0x1C201E)
    static let sidebar = dyn(0xF0F2F0, 0x101312)
    static let border = dyn(0xDCE1DD, 0x2E3532)
    static let borderStrong = dyn(0xBCC5BF, 0x4A534F)
    static let hover = dyn(0xE9EDEA, 0x252B28)
    static let textPrimary = dyn(0x1B201E, 0xE8ECEA)
    static let textSecondary = dyn(0x566059, 0xA3ADA7)
    static let textTertiary = dyn(0x636D67, 0x8A948E)
    static let accent = dyn(0x0E7A5F, 0x45C29C)
    static let onAccent = dyn(0xFFFFFF, 0x06241C)
    static let accentHover = dyn(0x0B6A52, 0x5BD0AB)
    static let accentPressed = dyn(0x095A45, 0x7BE0BF)
    static let accentText = dyn(0x0B6A52, 0x45C29C)
    static let accentTint = dyn(0xDDF0E8, 0x1B3A31)
    static let warning = dyn(0x9A5B00, 0xE0A23A)
    static let warningTint = dyn(0xFBF0DC, 0x3A2E14)
    static let danger = dyn(0xB3261E, 0xF0847A)
    static let dangerTint = dyn(0xFBE9E7, 0x3D1F1C)
    static let heat: [Color] = [dyn(0xE6EBE8, 0x222826), dyn(0xBFE3D3, 0x17493B), dyn(0x7CC6A9, 0x1F7059),
                                dyn(0x2E9F7E, 0x2D9C7D), dyn(0x0E7A5F, 0x5FD6B2)]
    static let focusRing = dynAlpha(0x0E7A5F, 0.30, 0x45C29C, 0.40)

    // v4 deltas (DESIGN_V4 section 2): sizes only, no new colours.
    static let sidebarWash = 0.80                               // sidebar fill over the split view's material; 1.0 under Reduce Transparency
    static let calMin: CGFloat = 28, calMax: CGFloat = 34, calH: CGFloat = 30, calGap: CGFloat = 2, calMark: CGFloat = 6
    static let popCellW: CGFloat = 36, popCellH: CGFloat = 32
    static let weekTileH: CGFloat = 64, weekTileGap: CGFloat = 8
    static let catchRowH: CGFloat = 44, sessionBarH: CGFloat = 44, badgeH: CGFloat = 18

    static let s1: CGFloat = 4, s2: CGFloat = 8, s3: CGFloat = 12, s4: CGFloat = 16, s5: CGFloat = 20
    static let s6: CGFloat = 24, s8: CGFloat = 32, s10: CGFloat = 40, s12: CGFloat = 48
    static let radiusSm: CGFloat = 4, radiusMd: CGFloat = 6, radiusLg: CGFloat = 10
    static let columnMax: CGFloat = 720

    /// One place for Reduce Motion: pass the environment flag, get nil (instant) when set.
    static func animation(_ a: Animation?, reduce: Bool) -> Animation? { reduce ? nil : a }
    static let base = Animation.easeOut(duration: 0.2)
    static let fast = Animation.easeOut(duration: 0.12)

    static func font(_ size: CGFloat, _ weight: Font.Weight = .regular) -> Font { .system(size: size, weight: weight) }
    static func rect(_ r: CGFloat = radiusMd) -> RoundedRectangle { RoundedRectangle(cornerRadius: r, style: .continuous) }
}

/// Stand-in for @State (a macro in this SDK): a tiny observable box owned with @StateObject.
final class Box<T>: ObservableObject {
    @Published var value: T
    init(_ v: T) { value = v }
}

/// Hover tracking without @State.
struct HoverReader<Content: View>: View {
    @StateObject private var h = Box(false)
    let content: (Bool) -> Content
    init(@ViewBuilder _ c: @escaping (Bool) -> Content) { content = c }
    var body: some View { content(h.value).onHover { h.value = $0 } }
}

enum Fmt {
    private static let timeF: DateFormatter = {
        let f = DateFormatter(); f.dateStyle = .none; f.timeStyle = .short; return f
    }()
    static func time(_ d: Date) -> String { timeF.string(from: d).lowercased() }
    static func clock(minutes: Int, calendar: Calendar, now: Date = Date()) -> String {
        let d = calendar.date(bySettingHour: minutes / 60, minute: minutes % 60, second: 0, of: now) ?? now
        return time(d)
    }
    static func plural(_ n: Int, _ word: String) -> String {
        let es = ["ch", "sh", "s", "x", "z"].contains { word.hasSuffix($0) }     // match -> matches
        return "\(n) \(word)\(n == 1 ? "" : (es ? "es" : "s"))"
    }
}

/// Heading text for display: leading emoji/symbols and spaces dropped ("📌 To do next" -> "To do next").
/// Files keep the raw heading; only the chrome shows the cleaned one.
func cleanHeading(_ raw: String) -> String {
    var t = Substring(raw)
    while let f = t.first, !(f.isLetter || f.isNumber) { t = t.dropFirst() }
    let s = t.trimmingCharacters(in: .whitespaces)
    return s.isEmpty ? raw.dlTrimmed : s
}

// MARK: button styles

struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(Theme.font(13, .semibold))
            .foregroundColor(enabled ? Theme.onAccent : Theme.textTertiary)
            .padding(.horizontal, Theme.s4).frame(minWidth: 96, minHeight: 32)
            .background(Theme.rect().fill(enabled ? (configuration.isPressed ? Theme.accentPressed : Theme.accent) : Theme.hover))
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .contentShape(Rectangle())
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(Theme.font(13)).foregroundColor(enabled ? Theme.textPrimary : Theme.textTertiary)
            .padding(.horizontal, Theme.s3).frame(minHeight: 28)
            .background(Theme.rect().fill(configuration.isPressed ? Theme.hover : Theme.surface))
            .overlay(Theme.rect().stroke(Theme.borderStrong, lineWidth: 1))
            .contentShape(Rectangle())
    }
}

struct TextButtonStyle: ButtonStyle {
    var color: Color = Theme.accentText
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(Theme.font(13, .semibold)).foregroundColor(enabled ? color : Theme.textTertiary)
            .opacity(configuration.isPressed ? 0.6 : 1).contentShape(Rectangle())
    }
}

/// Snapshot harness flips this to draw static text instead of the WKWebView editor (WebKit does not render offscreen here).
struct SnapshotModeKey: EnvironmentKey { static let defaultValue = false }
extension EnvironmentValues {
    var snapshotMode: Bool { get { self[SnapshotModeKey.self] } set { self[SnapshotModeKey.self] = newValue } }
}
