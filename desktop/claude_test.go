package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// fakeShell records every command and answers by prefix. Nothing here runs the real claude.
type fakeShell struct {
	commands []string
	answers  map[string]shellAnswer // command suffix or prefix -> answer
}
type shellAnswer struct {
	code int
	out  string
}

func (f *fakeShell) run(command string) (int, string) {
	f.commands = append(f.commands, command)
	for k, a := range f.answers {
		if strings.Contains(command, k) {
			return a.code, a.out
		}
	}
	return 0, ""
}

const testScript = "/Applications/Gloamlog.app/Contents/Resources/claude/gloamlog-mcp.py"

// setup makes a claudeSetup over a temp home with an executable "claude" in ~/.claude/local.
func newClaudeSetup(t *testing.T, withClaude bool, answers map[string]shellAnswer) (*claudeSetup, *fakeShell, string) {
	t.Helper()
	home := t.TempDir()
	claude := filepath.Join(home, ".claude", "local", "claude")
	if withClaude {
		if err := os.MkdirAll(filepath.Dir(claude), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sh := &fakeShell{answers: answers}
	return &claudeSetup{home: home, script: testScript, run: sh.run}, sh, claude
}

func TestClaudeStatus(t *testing.T) {
	for _, c := range []struct {
		name       string
		withClaude bool
		answers    map[string]shellAnswer
		want       ClaudeState
	}{
		{"no claude", false, nil, ClaudeNoClaude},
		{"not registered", true, map[string]shellAnswer{"mcp get": {1, "No MCP server found"}}, ClaudeNotConnected},
		{"registered here", true, map[string]shellAnswer{"mcp get": {0, "gloamlog:\n  Command: python3 " + testScript}}, ClaudeConnected},
		{"app moved", true, map[string]shellAnswer{"mcp get": {0, "gloamlog:\n  Command: python3 /old/Gloamlog.app/x.py"}}, ClaudeMoved},
		{"exit 0 but not ours", true, map[string]shellAnswer{"mcp get": {0, "nothing relevant"}}, ClaudeNotConnected},
	} {
		s, _, _ := newClaudeSetup(t, c.withClaude, c.answers)
		if got := s.Status(); got.State != c.want {
			t.Errorf("%s: state %q, want %q", c.name, got.State, c.want)
		}
	}
}

func TestClaudeFoundThroughALoginShell(t *testing.T) {
	s, sh, _ := newClaudeSetup(t, false, nil)
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sh.answers = map[string]shellAnswer{"command -v claude": {0, "Welcome to zsh\n/not/executable/claude\n" + bin + "\n"}}
	if got := s.find(); got != bin {
		t.Errorf("find = %q, want %q", got, bin)
	}
}

func TestConnectRegistersTheHelperAndAddsTheBlock(t *testing.T) {
	s, sh, claude := newClaudeSetup(t, true, nil)
	md := filepath.Join(s.home, ".claude", "CLAUDE.md")
	r := s.Connect()
	if !r.OK || r.State != ClaudeConnected {
		t.Fatalf("Connect = %+v", r)
	}
	want := []string{
		quote(claude) + " mcp remove gloamlog -s user",
		quote(claude) + " mcp add --scope user gloamlog -- python3 '" + testScript + "'",
	}
	if strings.Join(sh.commands, "\n") != strings.Join(want, "\n") {
		t.Errorf("commands = %q, want %q", sh.commands, want)
	}
	if got := readFile(t, md); got != core.ClaudeMdBlock+"\n" {
		t.Errorf("CLAUDE.md = %q", got)
	}
	if _, err := os.Stat(filepath.Join(s.home, ".claude", "settings.json")); err == nil {
		t.Error("settings.json was created")
	}
}

func TestConnectKeepsHandWrittenTextAndDisconnectRemovesOnlyOurBlock(t *testing.T) {
	s, sh, claude := newClaudeSetup(t, true, nil)
	md := filepath.Join(s.home, ".claude", "CLAUDE.md")
	os.MkdirAll(filepath.Dir(md), 0o755)
	os.WriteFile(md, []byte("# Mine\n\nBe brief.\n"), 0o644)

	s.Connect()
	got := readFile(t, md)
	if !strings.HasPrefix(got, "# Mine\n\nBe brief.\n\n"+core.ClaudeMdBegin) {
		t.Fatalf("CLAUDE.md = %q", got)
	}
	s.Connect() // again: replaced in place, not added twice
	if strings.Count(readFile(t, md), core.ClaudeMdBegin) != 1 {
		t.Fatal("the block was added twice")
	}

	sh.commands = nil
	r := s.Disconnect()
	if !r.OK || r.Message != "Disconnected." {
		t.Fatalf("Disconnect = %+v", r)
	}
	if got := readFile(t, md); got != "# Mine\n\nBe brief.\n" {
		t.Errorf("CLAUDE.md after disconnect = %q", got)
	}
	if len(sh.commands) != 1 || sh.commands[0] != quote(claude)+" mcp remove gloamlog -s user" {
		t.Errorf("commands = %q", sh.commands)
	}
}

func TestAHandWrittenLogWorkSectionCountsAsSetUp(t *testing.T) {
	s, _, _ := newClaudeSetup(t, true, nil)
	md := filepath.Join(s.home, ".claude", "CLAUDE.md")
	os.MkdirAll(filepath.Dir(md), 0o755)
	mine := "## Work log\nCall log_work when done.\n"
	os.WriteFile(md, []byte(mine), 0o644)
	s.Connect()
	if got := readFile(t, md); got != mine {
		t.Errorf("CLAUDE.md = %q, want it untouched", got)
	}
}

func TestConnectRefusedByClaudeLeavesClaudeMdAlone(t *testing.T) {
	s, _, _ := newClaudeSetup(t, true, map[string]shellAnswer{"mcp add": {1, "boom"}})
	r := s.Connect()
	if r.OK || !strings.Contains(r.Message, "Claude Code refused the setup: boom") {
		t.Fatalf("Connect = %+v", r)
	}
	if _, err := os.Stat(filepath.Join(s.home, ".claude", "CLAUDE.md")); err == nil {
		t.Error("CLAUDE.md was written although the registration failed")
	}
}

func TestConnectWithoutClaude(t *testing.T) {
	s, sh, _ := newClaudeSetup(t, false, nil)
	r := s.Connect()
	if r.OK || r.State != ClaudeNoClaude || !strings.Contains(r.Message, "wasn't found") {
		t.Fatalf("Connect = %+v", r)
	}
	for _, c := range sh.commands {
		if strings.Contains(c, "mcp add") {
			t.Errorf("ran %q", c)
		}
	}
}

func TestDisconnectWhenRemoveFails(t *testing.T) {
	s, _, _ := newClaudeSetup(t, true, map[string]shellAnswer{"mcp remove": {1, ""}})
	r := s.Disconnect()
	if r.OK || r.Message != "Couldn't remove the helper from Claude Code. Run: claude mcp remove gloamlog -s user" {
		t.Fatalf("Disconnect = %+v", r)
	}
}

func TestQuoteSurvivesApostrophes(t *testing.T) {
	if got := quote("/Users/o'neil/x"); got != `'/Users/o'\''neil/x'` {
		t.Errorf("quote = %s", got)
	}
}

func TestBundleScriptPath(t *testing.T) {
	got, ok := bundleScript("/Applications/Gloamlog.app/Contents/MacOS/Gloamlog")
	if !ok || got != testScript {
		t.Errorf("bundleScript = %q, %v", got, ok)
	}
	if _, ok := bundleScript("/tmp/go-build123/exe/desktop"); ok {
		t.Error("a go run binary is not a bundle")
	}
	// Outside a bundle the helper is the copy next to the sources, which must exist and be the same file as the Swift app's.
	if _, err := os.Stat(claudeScriptPath()); err != nil {
		t.Errorf("claudeScriptPath() = %s: %v", claudeScriptPath(), err)
	}
}

func TestCopyTexts(t *testing.T) {
	s := &claudeSetup{script: testScript}
	if got := s.CopyText("setup"); got != `claude mcp add --scope user gloamlog -- python3 "`+testScript+`"` {
		t.Errorf("setup = %s", got)
	}
	if got := s.CopyText("instructions"); !strings.HasPrefix(got, "## Gloamlog work log\n") || !strings.Contains(got, "`log_work`") {
		t.Errorf("instructions = %s", got)
	}
	if got := s.CopyText("hook"); !strings.Contains(got, `pgrep -x Gloamlog >/dev/null && open -g gloamlog://refresh || true`) {
		t.Errorf("hook = %s", got)
	}
	if s.CopyText("other") != "" {
		t.Error("unknown kind should be empty")
	}
}

// A CLAUDE.md that exists but can't be read is never overwritten: writing the block over it would erase the user's own
// instructions.
func TestUnreadableClaudeMdIsLeftAlone(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "# My rules\n\nNever touch production.\n"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o644)
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running with permission to read anything (root?): can't make the file unreadable")
	}
	s := &claudeSetup{home: home, script: testScript}
	msg := s.editClaudeMd(func(old string) string { return old + "\n<!-- gloamlog:begin -->\nblock\n<!-- gloamlog:end -->\n" })
	if !strings.Contains(msg, "left as it is") {
		t.Errorf("message = %q", msg)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != mine {
		t.Errorf("CLAUDE.md changed: %q", got)
	}
}
