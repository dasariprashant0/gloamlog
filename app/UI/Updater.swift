// Updater.swift - "Check for Updates…": one click checks GitHub's latest release and, when it is newer, downloads it, checks it and
// swaps it in without leaving the app. The only network traffic is the click: one request to api.github.com and, on "Download and
// Install", the release's zip and checksum from github.com. Nothing is sent about you, nothing runs in the background.
//
// Install steps: download the zip (HTTPS, github.com only) -> SHA-256 must equal Gloamlog.zip.sha256 from the same release ->
// unpack -> bundle id and version must match, signature must verify -> a tiny shell script waits for this process to quit, moves
// the old app aside, copies the new one in (the old one is put back if that fails) and opens it. A downloaded file the app saves
// itself carries no quarantine flag, so no Gatekeeper dialog appears. Trust is GitHub plus the checksum; there is no developer
// signature to check until the app is notarised.
import AppKit
import CryptoKit

final class Updater: NSObject {
    static let shared = Updater()

    private var busy = false
    private var panel: NSPanel?
    private let bar = NSProgressIndicator()
    private let label = NSTextField(labelWithString: "")
    private var observation: NSKeyValueObservation?
    private let session = URLSession(configuration: .ephemeral)

    private var current: String { Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "0" }

    // MARK: check
    func check() {
        guard !busy else { return }
        busy = true
        var req = URLRequest(url: UpdateCheck.latestURL, timeoutInterval: 15)
        req.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        session.dataTask(with: req) { [weak self] data, resp, err in
            DispatchQueue.main.async { self?.handle(data, resp, err) }
        }.resume()
    }

    @discardableResult
    private func alert(_ title: String, _ text: String, buttons: [String] = ["OK"]) -> Int {
        let a = NSAlert()
        a.messageText = title; a.informativeText = text
        buttons.forEach { a.addButton(withTitle: $0) }
        NSApp.activate(ignoringOtherApps: true)
        return a.runModal().rawValue - NSApplication.ModalResponse.alertFirstButtonReturn.rawValue
    }

    private func handle(_ data: Data?, _ resp: URLResponse?, _ err: Error?) {
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        if status == 404 { busy = false; alert("No updates published yet", "Gloamlog \(current) is the only version so far."); return }
        guard err == nil, status == 200, let data = data, let info = UpdateCheck.parse(data) else {
            busy = false; alert("Couldn't check for updates", "Check your internet connection and try again."); return
        }
        guard UpdateCheck.isNewer(info.version, than: current) else {
            busy = false; alert("You're up to date", "Gloamlog \(current) is the latest version."); return
        }
        let notes = info.notes.trimmingCharacters(in: .whitespacesAndNewlines)
        let body = "You have \(current)." + (notes.isEmpty ? "" : "\n\n" + String(notes.prefix(600)))
        switch alert("Gloamlog \(info.version) is available", body, buttons: ["Download and Install", "Release Notes", "Later"]) {
        case 0: install(info)
        case 1: busy = false; NSWorkspace.shared.open(info.pageURL)
        default: busy = false
        }
    }

    // MARK: install
    private func install(_ info: ReleaseInfo) {
        let dest = Bundle.main.bundleURL
        guard UpdateCheck.isTrusted(info.zipURL), let shaURL = info.shaURL, UpdateCheck.isTrusted(shaURL) else {
            return fallback(info, "This release has no checksum file, so Gloamlog won't install it by itself.")
        }
        guard FileManager.default.isWritableFile(atPath: dest.deletingLastPathComponent().path) else {
            return fallback(info, "Gloamlog can't replace itself in \(dest.deletingLastPathComponent().path).")
        }
        showPanel("Downloading Gloamlog \(info.version)…")
        session.dataTask(with: shaURL) { [weak self] data, resp, _ in
            guard let self = self else { return }
            guard (resp as? HTTPURLResponse)?.statusCode == 200, let text = data.flatMap({ String(data: $0, encoding: .utf8) }),
                  let want = UpdateCheck.checksum(from: text) else { return self.fail("The checksum could not be read.") }
            let task = self.session.downloadTask(with: info.zipURL) { tmp, resp, _ in
                guard let tmp = tmp, (resp as? HTTPURLResponse)?.statusCode == 200 else { return self.fail("The download failed.") }
                self.prepare(zip: tmp, want: want, info: info, dest: dest)
            }
            self.observation = task.progress.observe(\.fractionCompleted) { p, _ in
                DispatchQueue.main.async { self.bar.doubleValue = p.fractionCompleted }
            }
            task.resume()
        }.resume()
    }

    /// Off the main thread: checksum, unpack, verify.
    private func prepare(zip tmp: URL, want: String, info: ReleaseInfo, dest: URL) {
        let fm = FileManager.default
        let work = fm.temporaryDirectory.appendingPathComponent("gloamlog-update-\(UUID().uuidString)")
        do {
            try fm.createDirectory(at: work, withIntermediateDirectories: true)
            let zip = work.appendingPathComponent("Gloamlog.zip")
            try fm.moveItem(at: tmp, to: zip)
            let got = SHA256.hash(data: try Data(contentsOf: zip)).map { String(format: "%02x", $0) }.joined()
            guard got == want else { return fail("The download did not match its checksum, so nothing was installed.") }
            let out = work.appendingPathComponent("out")
            guard run("/usr/bin/ditto", ["-x", "-k", zip.path, out.path]) == 0 else { return fail("The download could not be unpacked.") }
            let new = out.appendingPathComponent("Gloamlog.app")
            let plist = NSDictionary(contentsOf: new.appendingPathComponent("Contents/Info.plist"))
            guard plist?["CFBundleIdentifier"] as? String == Bundle.main.bundleIdentifier,
                  plist?["CFBundleShortVersionString"] as? String == info.version,
                  run("/usr/bin/codesign", ["--verify", "--deep", "--strict", new.path]) == 0 else {
                return fail("The download is not a valid Gloamlog \(info.version), so nothing was installed.")
            }
            DispatchQueue.main.async { self.swapAndQuit(new: new, dest: dest) }
        } catch { fail("The update could not be prepared: \(error.localizedDescription)") }
    }

    /// The script outlives this process. It gives up after 60 s if the app never quits (for example, a "save your edits?" prompt was cancelled).
    private func swapAndQuit(new: URL, dest: URL) {
        label.stringValue = "Restarting Gloamlog…"; bar.isIndeterminate = true; bar.startAnimation(nil)
        let script = """
        i=0; while kill -0 "$1" 2>/dev/null; do i=$((i+1)); [ $i -gt 300 ] && exit 1; sleep 0.2; done
        OLD="$3.old"; rm -rf "$OLD"; mv "$3" "$OLD" || exit 1
        if /usr/bin/ditto "$2" "$3"; then rm -rf "$OLD"; else rm -rf "$3"; mv "$OLD" "$3"; fi
        xattr -dr com.apple.quarantine "$3" 2>/dev/null
        open "$3"; rm -rf "$(dirname "$(dirname "$2")")"
        """
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/sh")
        p.arguments = ["-c", script, "sh", String(ProcessInfo.processInfo.processIdentifier), new.path, dest.path]
        p.standardOutput = FileHandle.nullDevice; p.standardError = FileHandle.nullDevice
        do { try p.run() } catch { return fail("The update could not be started.") }
        NSApp.terminate(nil)
    }

    // MARK: small helpers
    private func run(_ path: String, _ args: [String]) -> Int32 {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: path); p.arguments = args
        p.standardOutput = FileHandle.nullDevice; p.standardError = FileHandle.nullDevice
        do { try p.run() } catch { return -1 }
        p.waitUntilExit()
        return p.terminationStatus
    }

    private func fail(_ text: String) {
        DispatchQueue.main.async {
            self.closePanel(); self.busy = false
            self.alert("Update not installed", text + " You can still download it from the Gloamlog releases page.")
        }
    }

    private func fallback(_ info: ReleaseInfo, _ why: String) {
        busy = false
        if alert("Can't install automatically", why, buttons: ["Open Download Page", "Cancel"]) == 0 { NSWorkspace.shared.open(info.pageURL) }
    }

    private func showPanel(_ text: String) {
        label.stringValue = text
        bar.isIndeterminate = false; bar.minValue = 0; bar.maxValue = 1; bar.doubleValue = 0
        let p = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 340, height: 86), styleMask: [.titled], backing: .buffered, defer: false)
        p.title = "Updating Gloamlog"
        let stack = NSStackView(views: [label, bar])
        stack.orientation = .vertical; stack.alignment = .leading; stack.spacing = 10
        stack.edgeInsets = NSEdgeInsets(top: 14, left: 18, bottom: 14, right: 18)
        bar.widthAnchor.constraint(equalToConstant: 304).isActive = true
        p.contentView = stack
        p.center(); p.isReleasedWhenClosed = false
        panel = p
        NSApp.activate(ignoringOtherApps: true)
        p.makeKeyAndOrderFront(nil)
    }

    private func closePanel() { panel?.close(); panel = nil; observation = nil }
}
