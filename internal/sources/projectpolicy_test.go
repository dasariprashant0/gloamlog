package sources

import "testing"

func TestProjectPolicy(t *testing.T) {
	p := NewProjectPolicy([]string{"/work/Acme/", "/work/b"}, false)
	if r, ok := p.Root("/work/acme"); !ok || r != "/work/Acme" {
		t.Errorf("case-insensitive exact match: %q %v", r, ok)
	}
	if r, ok := p.Root("/work/acme/.claude/worktrees/feat/x"); !ok || r != "/work/Acme" {
		t.Errorf("worktree folds: %q %v", r, ok)
	}
	if _, ok := p.Root("/work/acme/sub"); ok {
		t.Error("a folder inside a ticked project is not ticked (never a prefix)")
	}
	if _, ok := p.Root("/work/acm"); ok {
		t.Error("prefix")
	}
	if !NewProjectPolicy(nil, false).IsEmpty() || NewProjectPolicy(nil, true).IsEmpty() || p.IsEmpty() {
		t.Error("IsEmpty")
	}
	if _, ok := NewProjectPolicy(nil, false).Root("/work/a"); ok {
		t.Error("ticking nothing reads nothing")
	}
	all := NewProjectPolicy([]string{"/work/b"}, true)
	if r, ok := all.Root("/work/a"); !ok || r != "/work/a" {
		t.Errorf("all mode reads everything: %q %v", r, ok)
	}
	if _, ok := all.Root("/work/B"); ok {
		t.Error("all mode skips the listed roots")
	}
	if r, _ := all.Root("/work/a/.claude/worktrees/x"); r != "/work/a" {
		t.Errorf("all mode folds worktrees: %q", r)
	}
	if NewProjectPolicy([]string{"/b", "/A"}, false).Key() != "/a\n/b" || all.Key() != "all\n/work/b" {
		t.Errorf("keys: %q %q", NewProjectPolicy([]string{"/b", "/A"}, false).Key(), all.Key())
	}
}

func TestSourcePaths(t *testing.T) {
	if NormalizePath("/a/b/../c/./d/") != "/a/c/d" || NormalizePath("/") != "/" || NormalizePath("") != "" {
		t.Error("NormalizePath")
	}
	if CollapseWorktree("/w/x/.claude/worktrees/y/z") != "/w/x" || CollapseWorktree("/w/x") != "/w/x" {
		t.Error("CollapseWorktree")
	}
	if ProjectName("/work/acme", "/Users/me") != "acme" || ProjectName("/users/ME", "/Users/me") != "Home" || ProjectName("/", "/Users/me") != "/" {
		t.Error("ProjectName")
	}
	if !PathContains("/w/x", "/W/x/y") || !PathContains("/w/x", "/w/x") || PathContains("/w/x", "/w/xy") || !PathContains("/", "/any") {
		t.Error("PathContains")
	}
	if RelativePath("/w/x/src/a.go", "/w/x") != "src/a.go" ||
		RelativePath("/w/x/.claude/worktrees/feat/src/a.go", "/w/x") != "src/a.go" ||
		RelativePath("/elsewhere/b.go", "/w/x") != "b.go" {
		t.Error("RelativePath")
	}
}
