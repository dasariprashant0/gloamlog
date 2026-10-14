// Headless snapshot harness (NOT shipped in the app). Renders UI views with sample data to PNG.
// Build+run: app/Tools/snapshot/run.sh [outdir]
// WKWebView does not render offscreen in this sandbox, so the editor area is a STAND-IN: snapshotMode draws the page's
// markdown as static blocks (MarkdownBlocksView) in place of the web editor. Everything around it is the real SwiftUI.
import SwiftUI
import AppKit

let base = ProcessInfo.processInfo.environment["TMPDIR"] ?? NSTemporaryDirectory()
let outDir = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : base + "/dl-snap"
try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)

// ---- sample data ----
let cal = Calendar.current
let tmp = URL(fileURLWithPath: base).appendingPathComponent("dl-snap-logs")
let backups = URL(fileURLWithPath: base).appendingPathComponent("dl-snap-backups")
for u in [tmp, backups] { try? FileManager.default.removeItem(at: u); try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true) }

let comps = DateComponents(year: 2026, month: 10, day: 6, hour: 17, minute: 5)
let nowDate = cal.date(from: comps)!
let ds = UserDefaults(suiteName: "dailylog.snapshot")!
ds.removePersistentDomain(forName: "dailylog.snapshot")
UserDefaults.standard.set(CommandLine.arguments.contains("--expanded"), forKey: "yesterdayExpanded")
var s = Settings(); s.storageFolder = tmp; s.onboarded = true
s.save(to: ds)

// The sandbox forbids Foundation's atomic-write temp file, so sample files are written non-atomically here only.
func put(_ day: String, _ text: String) { try? text.write(to: tmp.appendingPathComponent(day + ".md"), atomically: false, encoding: .utf8) }
func page(_ day: String, _ body: String) { put(day, MarkdownFormat.serialize(day: day, body: body)) }

// a tiny picture for the image path (a gradient-free flat swatch with a border)
let assets = tmp.appendingPathComponent("assets"); try? FileManager.default.createDirectory(at: assets, withIntermediateDirectories: true)
let img = NSImage(size: NSSize(width: 480, height: 270))
img.lockFocus(); NSColor(srgbRed: 0.86, green: 0.93, blue: 0.9, alpha: 1).setFill(); NSBezierPath(rect: NSRect(x: 0, y: 0, width: 480, height: 270)).fill()
NSColor(srgbRed: 0.05, green: 0.48, blue: 0.37, alpha: 1).setFill(); NSBezierPath(roundedRect: NSRect(x: 40, y: 40, width: 220, height: 120), xRadius: 12, yRadius: 12).fill()
img.unlockFocus()
if let tiff = img.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff), let png = rep.representation(using: .png, properties: [:]) {
    try? png.write(to: assets.appendingPathComponent("2026-10-05-1a2b3c4d.png"))
}

let did = ["Drafted the pricing page copy and reviewed the signup form with Ana.", "Webinar landing page wireframes; fixed the tracking on the demo form.",
           "Reviewed the Q3 campaign report and wrote the summary for Friday's sync.", "Interviewed two candidates for the content role."]
for back in 1...80 {
    guard let k = DayKey.adding("2026-10-06", -back, cal) else { continue }
    let wd = DayKey.weekday(k, cal)
    if wd == 1 || wd == 7 { continue }
    if back % 11 == 4 { continue }                       // missed
    if back % 9 == 3 { put(k, MarkdownFormat.serializeSkip(day: k, reason: back % 2 == 0 ? "Holiday" : "Leave")); continue }
    if back == 1 {
        page(k, """
        ## What I did

        \(did[1]) Synced with **Ana** about the signup form and left comments on the hero copy.

        ![Wireframe](assets/2026-10-05-1a2b3c4d.png)

        ## Finished

        - Pricing table update shipped
        - Form fix merged

        ## Pending / blocked

        - Waiting on legal for the DPA
        - Ping Ana about the form

        ## To do next

        - [ ] Review the pricing page copy with Ana
        - [ ] Ship the webinar page
        - [x] Send the deck
        """)
    } else {
        page(k, "## 📝 What I did\n\(did[back % did.count])\n\n## ✅ Finished\nPricing table update shipped.\nForm fix merged.\n\n## ⏳ Pending / blocked\nWaiting on design review\n\n## 📌 To do next\nFollow up on open items")
    }
}
// today: a few sentences typed so far (12 words), one heading
page("2026-10-06", "## What I did\n\nReviewed the new pricing page and left comments on the hero copy. Synced with Ana.")

// previous versions for the restore sheet (names are UTC time stamps)
let bdir = backups.appendingPathComponent("2026-10-05"); try? FileManager.default.createDirectory(at: bdir, withIntermediateDirectories: true)
for (name, text) in [("20261005-150812", "# 2026-10-05\n\n## What I did\n\nDrafted the pricing page copy.\n"),
                     ("20261005-163044", "# 2026-10-05\n\n## What I did\n\nDrafted the pricing page copy and reviewed the signup form with Ana. Fixed tracking on the demo form.\n\n## Finished\n\n- Pricing table update shipped\n")] {
    try? text.write(to: bdir.appendingPathComponent(name + ".md"), atomically: false, encoding: .utf8)
}

let model = AppModel(defaults: ds, backupDir: backups, live: false, clock: { nowDate })

// ---- rendering ----
_ = NSApplication.shared
func snap<V: View>(_ v: V, _ w: CGFloat, _ h: CGFloat, _ name: String, dark: Bool = false) {
    let root = v.frame(width: w, height: h).environment(\.snapshotMode, true).environment(\.colorScheme, dark ? .dark : .light)
    let host = NSHostingView(rootView: root)
    host.frame = NSRect(x: 0, y: 0, width: w, height: h)
    let win = NSWindow(contentRect: host.frame, styleMask: [.titled], backing: .buffered, defer: false)
    win.appearance = NSAppearance(named: dark ? .darkAqua : .aqua)
    win.contentView = host
    host.layoutSubtreeIfNeeded()
    RunLoop.current.run(until: Date().addingTimeInterval(0.4))
    host.layoutSubtreeIfNeeded()
    guard let rep = host.bitmapImageRepForCachingDisplay(in: host.bounds) else { print("no rep \(name)"); return }
    host.cacheDisplay(in: host.bounds, to: rep)
    if let d = rep.representation(using: .png, properties: [:]) { try? d.write(to: URL(fileURLWithPath: "\(outDir)/\(name).png")); print("wrote \(name)") }
}

func composite(_ detail: AnyView) -> some View {
    HStack(spacing: 0) { SidebarView(model: model).frame(width: 240); detail.frame(maxWidth: .infinity) }
}

let only = CommandLine.arguments.dropFirst().filter { !$0.hasPrefix("-") && !$0.hasPrefix("/") }
func want(_ n: String) -> Bool { only.isEmpty || only.contains(n) }

for dark in [false, true] {
    let t = dark ? "-dark" : ""
    snap(composite(AnyView(DayPage(model: model, editor: model.editor))), 1000, 760, "main-today\(t)", dark: dark)
    snap(StreakPopover(model: model) { _ in }, 264, 230, "streak-popover\(t)", dark: dark)
    snap(MenuBarView(model: model), 300, 400, "menubar\(t)", dark: dark)
    model.select(.week)
    snap(composite(AnyView(WeeklyReviewView(model: model))), 1000, 760, "weekly\(t)", dark: dark)
    model.weekGrouping = .section
    snap(composite(AnyView(WeeklyReviewView(model: model))), 1000, 760, "weekly-sections\(t)", dark: dark)
    model.weekGrouping = .day
    model.select(.day(model.today))
    snap(GeneralSettings(model: model), 520, 560, "settings-general\(t)", dark: dark)
    snap(PageSettings(model: model), 520, 640, "settings-page\(t)", dark: dark)
    snap(StorageSettings(model: model), 520, 300, "settings-storage\(t)", dark: dark)
}
// a past, logged day with an image and tasks (the editor stand-in shows the page markdown)
model.select(.day("2026-10-05"))
snap(composite(AnyView(DayPage(model: model, editor: model.editor))), 1000, 860, "main-past-day")
model.select(.day(model.today))
model.searchText = "pricing"; model.runSearch("pricing", nil)
snap(composite(AnyView(SearchView(model: model))), 1000, 520, "search")
model.searchText = ""
snap(SkipSheet(model: model, day: model.today), 500, 380, "skip-sheet")
snap(RestoreVersionSheet(model: model, day: "2026-10-05"), 520, 380, "restore-sheet")
snap(OnboardingView(model: model, startStep: 4), 600, 520, "onboarding-claude")
snap(OnboardingView(model: model, startStep: 4), 600, 520, "onboarding-claude-dark", dark: true)
for step in 1...3 { snap(OnboardingView(model: model, startStep: step), 600, 520, "onboarding-\(step)") }
