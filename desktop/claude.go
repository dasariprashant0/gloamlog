package main

// "Connect Claude Code": registers the log_work helper that ships in the app bundle with Claude Code and adds the marked
// block to ~/.claude/CLAUDE.md, so Claude writes an entry when a piece of work is done. Same steps as the Swift ClaudeSetup
// (docs/port/PARITY.md 2.1). The user's confirmation dialog is the page's job (Settings > Sources); the calls here just do it.
//
// Never touches ~/.claude/settings.json (the Stop hook is only offered as text) and ~/.claude.json (`claude mcp add` writes that).
// The helper is Contents/Resources/claude/gloamlog-mcp.py of the app bundle; build.sh must copy desktop/resources/claude there.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// ClaudeState is where the connection stands.
type ClaudeState string

const (
	ClaudeConnected    ClaudeState = "connected"
	ClaudeMoved        ClaudeState = "moved" // registered, but the app was moved or renamed since: "Needs reconnecting"
	ClaudeNotConnected ClaudeState = "notConnected"
	ClaudeNoClaude     ClaudeState = "noClaude" // "Claude Code not found"
)

// ClaudeInfo is the state plus the one-line text for Settings.
type ClaudeInfo struct {
	State   ClaudeState `json:"state"`
	Message string      `json:"message"`
}

// ClaudeResult is what Connect and Disconnect report.
type ClaudeResult struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message"`
	State   ClaudeState `json:"state"`
}

// shellRun runs one command line in a login shell (an app started from the Dock has almost no PATH, and nvm, volta and npm
// prefixes live in the owner's profile) and returns its exit code and combined output.
type shellRun func(command string) (code int, out string)

func zshLogin(command string) (int, string) {
	cmd := exec.Command("/bin/zsh", "-lic", command)
	var buf strings.Builder
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return -1, err.Error()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0, buf.String()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), buf.String()
		}
		return -1, err.Error()
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		return -1, "timed out"
	}
}

// claudeSetup is everything the setup needs from the machine, so tests give it a fake shell and a temp home.
type claudeSetup struct {
	home   string
	script string   // the helper's absolute path
	system []string // places `claude` may be installed besides the home folder's
	run    shellRun
}

var systemClaudePaths = []string{"/opt/homebrew/bin/claude", "/usr/local/bin/claude"}

func (n *Notes) claude() *claudeSetup {
	return &claudeSetup{home: n.home, script: claudeScriptPath(), system: systemClaudePaths, run: zshLogin}
}

// claudeScriptPath is <bundle>/Contents/Resources/claude/gloamlog-mcp.py, derived from the running executable
// (.../Gloamlog.app/Contents/MacOS/Gloamlog). Outside a bundle (go run, tests) it is desktop/resources/claude next to the sources.
func claudeScriptPath() string {
	if exe, err := os.Executable(); err == nil {
		if resolved, e := filepath.EvalSymlinks(exe); e == nil {
			exe = resolved
		}
		if p, ok := bundleScript(exe); ok {
			return p
		}
	}
	if abs, err := filepath.Abs(filepath.Join("resources", claudeScriptRel)); err == nil {
		return abs
	}
	return filepath.Join("resources", claudeScriptRel)
}

const claudeScriptRel = "claude/gloamlog-mcp.py"

// bundleScript is the helper's path inside the app bundle that exe (…/Contents/MacOS/Gloamlog) belongs to.
func bundleScript(exe string) (string, bool) {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" {
		return "", false
	}
	return filepath.Join(contents, "Resources", claudeScriptRel), true
}

func (s *claudeSetup) claudeMdPath() string { return filepath.Join(s.home, ".claude", "CLAUDE.md") }

// quote is POSIX single quoting.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// find is the `claude` binary: the usual install places first, then whatever a login shell resolves.
func (s *claudeSetup) find() string {
	places := append([]string{filepath.Join(s.home, ".claude", "local", "claude"), filepath.Join(s.home, ".local", "bin", "claude")}, s.system...)
	for _, p := range places {
		if isExecutable(p) {
			return p
		}
	}
	_, out := s.run("command -v claude")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); strings.HasPrefix(l, "/") && isExecutable(l) {
			return l
		}
	}
	return ""
}

// Status asks Claude Code whether the helper is registered, and whether the registered path is this app's.
func (s *claudeSetup) Status() ClaudeInfo {
	claude := s.find()
	if claude == "" {
		return ClaudeInfo{ClaudeNoClaude, "Claude Code not found"}
	}
	code, out := s.run(quote(claude) + " mcp get gloamlog")
	switch {
	case code != 0 || !strings.Contains(out, "gloamlog"):
		return ClaudeInfo{ClaudeNotConnected, "Not connected"}
	case strings.Contains(out, s.script):
		return ClaudeInfo{ClaudeConnected, "Connected"}
	default:
		return ClaudeInfo{ClaudeMoved, "Needs reconnecting"}
	}
}

// Connect registers the helper (replacing a stale registration) and adds the CLAUDE.md block.
func (s *claudeSetup) Connect() ClaudeResult {
	claude := s.find()
	if claude == "" {
		return ClaudeResult{false, "Claude Code wasn't found on this Mac. Install it from claude.com/claude-code, then try again.", ClaudeNoClaude}
	}
	c := quote(claude)
	s.run(c + " mcp remove gloamlog -s user") // a stale registration (the app was moved) is replaced; absent is fine
	if code, out := s.run(c + " mcp add --scope user gloamlog -- python3 " + quote(s.script)); code != 0 {
		if len(out) > 200 {
			out = out[len(out)-200:]
		}
		return ClaudeResult{false, "Claude Code refused the setup: " + strings.TrimSpace(out), ClaudeNotConnected}
	}
	if err := s.editClaudeMd(core.ClaudeMdApply); err != "" {
		return ClaudeResult{false, err, ClaudeNotConnected}
	}
	return ClaudeResult{true, "Connected. Start a new Claude Code session; entries will appear under What I did.", ClaudeConnected}
}

// Disconnect removes the registration and our CLAUDE.md block (a hand-written section stays).
func (s *claudeSetup) Disconnect() ClaudeResult {
	text := "Disconnected."
	if claude := s.find(); claude != "" {
		if code, _ := s.run(quote(claude) + " mcp remove gloamlog -s user"); code != 0 {
			text = "Couldn't remove the helper from Claude Code. Run: claude mcp remove gloamlog -s user"
		}
	}
	if err := s.editClaudeMd(core.ClaudeMdRemove); err != "" {
		text = err
	}
	return ClaudeResult{text == "Disconnected.", text, ClaudeNotConnected}
}

// editClaudeMd reads ~/.claude/CLAUDE.md (absent is empty), applies change and writes it back only when it differs.
// It returns an error text, or "".
func (s *claudeSetup) editClaudeMd(change func(string) string) string {
	path := s.claudeMdPath()
	old, err := os.ReadFile(path)
	// Only a missing file counts as empty. Any other read failure (permissions, a cloud placeholder, a disk error) must leave
	// the user's CLAUDE.md alone: writing "the block" over a file we couldn't read would erase their own instructions.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "Couldn't read ~/.claude/CLAUDE.md, so it was left as it is: " + err.Error()
	}
	updated := change(string(old))
	if updated == string(old) {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "Couldn't update ~/.claude/CLAUDE.md: " + err.Error()
	}
	if err := core.AtomicWrite([]byte(updated), path, ""); err != nil {
		return "Couldn't update ~/.claude/CLAUDE.md: " + err.Error()
	}
	return ""
}

// CopyText is the text of one "Copy" button in Settings: "setup" (the command), "instructions" (the CLAUDE.md text) or "hook"
// (the Stop hook for ~/.claude/settings.json, which the app never edits).
func (s *claudeSetup) CopyText(kind string) string {
	switch kind {
	case "setup":
		return `claude mcp add --scope user gloamlog -- python3 "` + s.script + `"`
	case "instructions":
		return core.ClaudeMdInstruction
	case "hook":
		return `"hooks": { "Stop": [ { "hooks": [ { "type": "command", "command": "pgrep -x Gloamlog >/dev/null && open -g gloamlog://refresh || true" } ] } ] }`
	}
	return ""
}

// ---- what the Settings page calls ----

// ClaudeStatus is "connected", "moved", "notConnected" or "noClaude" with its text. It runs `claude`, so call it off the render path.
func (n *Notes) ClaudeStatus() ClaudeInfo { return n.claude().Status() }

// ConnectClaude does the three steps of "Connect Claude Code" (the page asks first): register the helper, add the CLAUDE.md
// block, and move "Jots go under" from its default "Jots" to "What I did" so Claude's entries land with the rest of the day.
func (n *Notes) ConnectClaude() (ClaudeResult, error) {
	r := n.claude().Connect()
	if r.OK {
		if s := n.Settings(); s.Capture.JotsHeading == "Jots" {
			s.Capture.JotsHeading = "What I did"
			if _, err := n.SaveSettings(s); err != nil {
				r.Message = "Connected, but couldn't change where jots go: " + err.Error()
			}
		}
	}
	return r, nil
}

// DisconnectClaude undoes steps 1 and 2.
func (n *Notes) DisconnectClaude() (ClaudeResult, error) { return n.claude().Disconnect(), nil }

// ClaudeCopyText is the text behind "Copy command" ("setup"), "Copy instructions" ("instructions") and "Copy hook" ("hook").
// The page puts it on the clipboard.
func (n *Notes) ClaudeCopyText(kind string) string { return n.claude().CopyText(kind) }
