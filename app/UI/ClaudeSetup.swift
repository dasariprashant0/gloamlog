// ClaudeSetup.swift - the "Connect Claude Code" button (Settings > Sources). One click, after a confirmation that lists exactly what changes:
//   1. registers the helper that ships inside the app (Resources/claude/gloamlog-mcp.py) with Claude Code for all projects:
//      `claude mcp add --scope user gloamlog -- python3 <path>`
//   2. adds a short, marked section to ~/.claude/CLAUDE.md telling Claude when to call `log_work` (ClaudeMd; a hand-written one is kept)
//   3. sets "Jots go under" to "What I did" when it still has its default, so entries land with the rest of the day.
// Disconnect undoes 1 and 2. The `claude` command is found through a login shell, because an app started from the Dock has almost no PATH.
// Everything runs off the main thread; nothing here touches the network. Main thread API.
import AppKit

final class ClaudeSetup: ObservableObject {
    enum State: Equatable { case unknown, checking, connected, moved, notConnected, noClaude, working }

    @Published private(set) var state: State = .unknown
    @Published private(set) var message: String?
    unowned let model: AppModel
    private let queue = DispatchQueue(label: "gloamlog.claude-setup", qos: .utility)

    init(model: AppModel) { self.model = model }

    static var scriptPath: String { Bundle.main.bundlePath + "/Contents/Resources/claude/gloamlog-mcp.py" }
    static var claudeMdURL: URL { FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".claude/CLAUDE.md") }

    // MARK: status
    func refresh() {
        guard state != .working else { return }
        state = .checking
        queue.async { [weak self] in
            let s = ClaudeSetup.currentState()
            DispatchQueue.main.async { self?.state = s }
        }
    }

    private static func currentState() -> State {
        guard let claude = findClaude() else { return .noClaude }
        let r = shell("\(q(claude)) mcp get gloamlog")
        guard r.code == 0, r.out.contains("gloamlog") else { return .notConnected }
        return r.out.contains(scriptPath) ? .connected : .moved          // .moved: the app was moved or renamed since it was connected
    }

    // MARK: connect / disconnect
    func connect() {
        guard state != .working else { return }
        let a = NSAlert()
        a.messageText = "Connect Claude Code to Gloamlog?"
        a.informativeText = """
        Gloamlog will:
        1. Register its small helper with Claude Code for all your projects (it runs claude mcp add).
        2. Add a short section to ~/.claude/CLAUDE.md that tells Claude when to write a log entry.
        3. Make quick notes and Claude's entries go under "What I did".

        Nothing is sent anywhere, and Disconnect undoes steps 1 and 2. Start a new Claude Code session afterwards.
        """
        a.addButton(withTitle: "Connect"); a.addButton(withTitle: "Cancel")
        NSApp.activate(ignoringOtherApps: true)
        guard a.runModal() == .alertFirstButtonReturn else { return }
        state = .working; message = nil
        queue.async { [weak self] in
            let result = ClaudeSetup.doConnect()
            DispatchQueue.main.async {
                guard let self = self else { return }
                if result.ok, self.model.settings.capture.jotsHeading == "Jots" { self.model.settings.capture.jotsHeading = "What I did" }
                self.message = result.text
                self.state = .unknown
                self.refresh()
            }
        }
    }

    func disconnect() {
        guard state != .working else { return }
        state = .working; message = nil
        queue.async { [weak self] in
            var text = "Disconnected."
            if let claude = ClaudeSetup.findClaude(), ClaudeSetup.shell("\(ClaudeSetup.q(claude)) mcp remove gloamlog -s user").code != 0 {
                text = "Couldn't remove the helper from Claude Code. Run: claude mcp remove gloamlog -s user"
            }
            if let err = ClaudeSetup.editClaudeMd(ClaudeMd.remove(from:)) { text = err }
            DispatchQueue.main.async { self?.message = text; self?.state = .unknown; self?.refresh() }
        }
    }

    private static func doConnect() -> (ok: Bool, text: String) {
        guard let claude = findClaude() else { return (false, "Claude Code wasn't found on this Mac. Install it from claude.com/claude-code, then try again.") }
        let c = q(claude)
        _ = shell("\(c) mcp remove gloamlog -s user")                      // a stale registration (the app was moved) is replaced; absent is fine
        let add = shell("\(c) mcp add --scope user gloamlog -- python3 \(q(scriptPath))")
        guard add.code == 0 else { return (false, "Claude Code refused the setup: " + String(add.out.suffix(200)).trimmingCharacters(in: .whitespacesAndNewlines)) }
        if let err = editClaudeMd(ClaudeMd.apply(to:)) { return (false, err) }
        return (true, "Connected. Start a new Claude Code session; entries will appear under What I did.")
    }

    /// Reads ~/.claude/CLAUDE.md (absent is empty), applies `change`, writes it back only when it differs. Returns an error text or nil.
    private static func editClaudeMd(_ change: (String) -> String) -> String? {
        let url = claudeMdURL, fm = FileManager.default
        let old = (try? String(contentsOf: url, encoding: .utf8)) ?? ""
        let new = change(old)
        guard new != old else { return nil }
        do {
            try fm.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try new.write(to: url, atomically: true, encoding: .utf8)
            return nil
        } catch { return "Couldn't update ~/.claude/CLAUDE.md: \(error.localizedDescription)" }
    }

    // MARK: finding and running `claude`
    private static func findClaude() -> String? {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        for p in [home + "/.claude/local/claude", home + "/.local/bin/claude", "/opt/homebrew/bin/claude", "/usr/local/bin/claude"]
        where FileManager.default.isExecutableFile(atPath: p) { return p }
        let r = shell("command -v claude")                                   // a login shell: nvm, volta and npm prefixes live in the owner's profile
        return r.out.split(separator: "\n").map(String.init).last(where: { $0.hasPrefix("/") && FileManager.default.isExecutableFile(atPath: $0) })
    }

    private static func q(_ s: String) -> String { "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'" }

    private static func shell(_ command: String) -> (code: Int32, out: String) {
        let p = Process(), pipe = Pipe()
        p.executableURL = URL(fileURLWithPath: "/bin/zsh"); p.arguments = ["-lic", command]
        p.standardOutput = pipe; p.standardError = pipe; p.standardInput = FileHandle.nullDevice
        do { try p.run() } catch { return (-1, error.localizedDescription) }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        return (p.terminationStatus, String(decoding: data, as: UTF8.self))
    }
}
