package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

const testHome = "/nonexistent-home"

var utc = core.NewCalendar(time.UTC, 2)

// jl builds JSONL lines for a fixture session file.
type jl struct{ lines []string }

func (j *jl) add(typ, ts, cwd, session string, content any, extra map[string]any) {
	rec := map[string]any{"type": typ, "timestamp": ts, "cwd": cwd, "sessionId": session, "message": map[string]any{"role": typ, "content": content}}
	for k, v := range extra {
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	j.lines = append(j.lines, string(b))
}
func (j *jl) raw(s string)                       { j.lines = append(j.lines, s) }
func (j *jl) user(ts, cwd, session, text string) { j.add("user", ts, cwd, session, text, nil) }
func (j *jl) tool(ts, cwd, session, id, name string, input map[string]any) {
	j.add("assistant", ts, cwd, session, []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}, nil)
}
func (j *jl) result(ts, cwd, session, id string, content any) {
	j.add("user", ts, cwd, session, []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": content}}, nil)
}

// write puts the lines in <root>/projects/<project>/<name> and sets the modified time.
func (j *jl) write(t *testing.T, root, project, name string, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(root, "projects", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(strings.Join(j.lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

func ts(h, m, s int) string { return fmt.Sprintf("2026-10-06T%02d:%02d:%02d.000Z", h, m, s) }

var dayEnd = time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)

func byKind(items []ActivityItem, k ActivityKind) []ActivityItem {
	var out []ActivityItem
	for _, it := range items {
		if it.Kind == k {
			out = append(out, it)
		}
	}
	return out
}

func claudeFixture(t *testing.T) (*ClaudeCodeSource, string) {
	root := t.TempDir()
	acme, secret := "/work/acme", "/work/secret"

	s1 := &jl{}
	s1.user(ts(9, 0, 0), acme, "s1", "Add a pricing table to the landing page")
	s1.tool(ts(9, 5, 0), acme, "s1", "t1", "Edit", map[string]any{"file_path": "/work/acme/src/a.go"})
	s1.tool(ts(9, 6, 0), acme, "s1", "t2", "Edit", map[string]any{"file_path": "/work/acme/src/a.go"})
	s1.tool(ts(9, 8, 0), acme, "s1", "t3", "Write", map[string]any{"file_path": "/work/acme/b.go"})
	s1.tool(ts(9, 10, 0), acme, "s1", "t4", "Bash", map[string]any{"command": `git add -A && git commit -m "Add pricing"`})
	s1.result(ts(9, 11, 0), acme, "s1", "t4", "[main abc1234] Add pricing\n 1 file changed")
	s1.tool(ts(9, 12, 0), acme, "s1", "t5", "Bash", map[string]any{"command": `git commit -m "will fail"`}) // failed: no commit line in the result
	s1.result(ts(9, 12, 30), acme, "s1", "t5", []any{map[string]any{"type": "text", "text": "nothing to commit, working tree clean"}})
	s1.tool(ts(9, 13, 0), acme, "s1", "t6", "Bash", map[string]any{"command": `gh pr create --title x --body y`})
	s1.result(ts(9, 14, 0), acme, "s1", "t6", "https://github.com/acme/site/pull/42\n")
	s1.user(ts(9, 20, 0), acme, "s1", "A second prompt that must not count") // only the session's first prompt per day
	s1.add("user", ts(9, 21, 0), acme, "s1", "Subagent prompt that is not the owner's", map[string]any{"isSidechain": true})
	s1.add("user", ts(9, 22, 0), acme, "s1", "Synthetic meta prompt not typed", map[string]any{"isMeta": true})
	s1.raw(`{"type":"summary","summary":"a title line mentioning "user" is not JSON-valid but is cheap-checked"}`) // not valid JSON: counted as skipped
	s1.raw(`{"type":"file-history-snapshot","note":"assistant"}`)                                                  // valid, another type: ignored
	s1.write(t, root, "-work-acme", "s1.jsonl", dayEnd)

	s2 := &jl{} // another session, run in a worktree of the same project
	s2.user(ts(14, 0, 0), acme+"/.claude/worktrees/feat", "s2", "<system-reminder>x</system-reminder>Fix the broken sidebar layout today")
	s2.user(ts(14, 5, 0), acme+"/.claude/worktrees/feat", "s2", "<command-name>/clear</command-name>")
	s2.write(t, root, "-work-acme-worktree", "s2.jsonl", dayEnd)

	sec := &jl{} // an unticked project; the garbage line proves the file is never parsed (a parse would count it as skipped)
	sec.user(ts(10, 0, 0), secret, "x1", "Prompt about something private entirely")
	sec.tool(ts(10, 1, 0), secret, "x1", "z1", "Write", map[string]any{"file_path": "/work/secret/plan.md"})
	sec.raw(`this is not json but says "user"`)
	sec.write(t, root, "-work-secret", "x1.jsonl", dayEnd)

	old := &jl{} // modified before the day starts: cannot hold it
	old.user(ts(9, 0, 0), acme, "old", "Prompt from a file that is too old to count")
	old.write(t, root, "-work-acme", "old.jsonl", time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC))

	orph := &jl{}
	orph.user(ts(9, 30, 0), acme, "orph", "Prompt in an orphaned copy that is ignored")
	orph.write(t, root, "-work-acme", "s1.jsonl.orphaned-123.jsonl", dayEnd)

	if err := os.Symlink(filepath.Join(root, "projects", "-work-secret"), filepath.Join(root, "projects", "-link")); err != nil {
		t.Log("symlinks unavailable:", err)
	}
	return NewClaudeCodeSource([]string{root}, testHome), root
}

func TestClaudeActivityDay(t *testing.T) {
	src, _ := claudeFixture(t)
	policy := NewProjectPolicy([]string{"/work/acme"}, false)
	res := src.Activity(context.Background(), "2026-10-06", utc, policy, nil)

	sessions := byKind(res.Items, KindSession)
	if len(sessions) != 1 {
		t.Fatalf("items: %+v", res.Items)
	}
	s := sessions[0]
	if s.Root != "/work/acme" || s.Project != "acme" || s.Count != 2 || s.Seconds != 27*60 /* sidechain and meta records still count as activity */ || !s.Time.Equal(time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("session item: %+v", s)
	}
	prompts := byKind(res.Items, KindPrompt)
	if len(prompts) != 2 || prompts[0].Title != "Add a pricing table to the landing page" || prompts[1].Title != "Fix the broken sidebar layout today" {
		t.Errorf("prompts: %+v", prompts)
	}
	files := byKind(res.Items, KindFile)
	if len(files) != 2 || files[0].Title != "src/a.go" || files[0].Count != 2 || files[1].Title != "b.go" || files[1].Count != 1 {
		t.Errorf("files: %+v", files)
	}
	commits := byKind(res.Items, KindCommit)
	if len(commits) != 1 || commits[0].Title != "Add pricing" || commits[0].Evidence != "abc1234" || commits[0].Detail != "abc1234" {
		t.Errorf("commits (the failed one must be dropped): %+v", commits)
	}
	prs := byKind(res.Items, KindPullRequest)
	if len(prs) != 1 || prs[0].Title != "https://github.com/acme/site/pull/42" || prs[0].Evidence != prs[0].Title {
		t.Errorf("prs: %+v", prs)
	}
	for _, it := range res.Items {
		if it.Root != "/work/acme" {
			t.Errorf("an item from another project: %+v", it)
		}
	}
	// the privacy gate: the unticked file is neither read nor counted; the 2 ticked files are
	if res.Stats.Files != 2 || res.Stats.Skipped != 1 {
		t.Errorf("stats: %+v (the unticked file's garbage line must not have been parsed)", res.Stats)
	}
	if len(res.Notices) != 0 {
		t.Errorf("notices: %v", res.Notices)
	}

	// bullets for the whole day, as the strip shows them
	d := Bullets(res.Items, testRed)
	want := []string{
		"Worked in acme with Claude Code for about 25 min (2 sessions, 2 files edited)",
		"Worked on acme: Add a pricing table to the landing page",
		"Worked on acme: Fix the broken sidebar layout today",
		"Committed: Add pricing (acme abc1234)",
		"Pull request: https://github.com/acme/site/pull/42 (acme)",
	}
	if got := texts(d.Bullets); !reflect.DeepEqual(got, want) {
		t.Errorf("bullets:\n%q\nwant\n%q", got, want)
	}
	if d.Bullets[0].Detail != "src/a.go (2), b.go" {
		t.Errorf("detail %q", d.Bullets[0].Detail)
	}
}

func TestClaudePrivacyGate(t *testing.T) {
	src, _ := claudeFixture(t)
	// nothing ticked reads nothing, not even to count
	res := src.Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy(nil, false), nil)
	if len(res.Items) != 0 || res.Stats != (ScanStats{}) {
		t.Errorf("empty policy: %+v", res)
	}
	// only the secret project ticked: the acme files are skipped unread, the secret one is read
	res = src.Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/secret"}, false), nil)
	if len(res.Items) == 0 || res.Stats.Files != 1 || res.Stats.Skipped != 1 {
		t.Errorf("secret only: %+v", res.Stats)
	}
	for _, it := range res.Items {
		if it.Root != "/work/secret" {
			t.Errorf("leak: %+v", it)
		}
	}
	// all-projects mode with acme in the skip list
	res = src.Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme"}, true), nil)
	for _, it := range res.Items {
		if it.Root == "/work/acme" {
			t.Errorf("skipped project leaked: %+v", it)
		}
	}
	if len(res.Items) == 0 {
		t.Error("all-projects mode should read the other project")
	}
	// a bad day key reads nothing
	if r := src.Activity(context.Background(), "nope", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil); len(r.Items) != 0 {
		t.Error("bad day")
	}
	// another day: the records are all on the 6th
	if r := src.Activity(context.Background(), "2026-10-07", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil); len(r.Items) != 0 {
		t.Errorf("wrong day: %+v", r.Items)
	}
}

func TestClaudeRecordsOutsideTickedFolderAreDropped(t *testing.T) {
	// A ticked file whose own folder later changes to an unticked one: records there are dropped before any text is taken.
	root := t.TempDir()
	j := &jl{}
	j.user(ts(9, 0, 0), "/work/acme", "s", "First prompt inside the ticked project")
	j.user(ts(9, 30, 0), "/work/other", "s", "A prompt typed after moving into another project")
	j.tool(ts(9, 31, 0), "/work/other", "s", "e1", "Edit", map[string]any{"file_path": "/work/other/x.go"})
	j.write(t, root, "-p", "s.jsonl", dayEnd)
	src := NewClaudeCodeSource([]string{root}, testHome)
	res := src.Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil)
	for _, it := range res.Items {
		if strings.Contains(it.Title, "another project") || it.Title == "x.go" {
			t.Errorf("leak: %+v", it)
		}
	}
	if p := byKind(res.Items, KindPrompt); len(p) != 1 || p[0].Title != "First prompt inside the ticked project" {
		t.Errorf("prompts: %+v", p)
	}
	// but a cd into a folder inside the ticked project stays in it
	root2 := t.TempDir()
	j2 := &jl{}
	j2.user(ts(9, 0, 0), "/work/acme", "s", "First prompt inside the ticked project")
	j2.tool(ts(9, 5, 0), "/work/acme/web", "s", "e1", "Edit", map[string]any{"file_path": "/work/acme/web/x.ts"})
	j2.write(t, root2, "-p", "s.jsonl", dayEnd)
	res = NewClaudeCodeSource([]string{root2}, testHome).Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil)
	if f := byKind(res.Items, KindFile); len(f) != 1 || f[0].Title != "web/x.ts" || f[0].Root != "/work/acme" {
		t.Errorf("files: %+v", f)
	}
}

func TestClaudePerDayLimits(t *testing.T) {
	root := t.TempDir()
	acme := "/work/acme"
	j := &jl{}
	for i := 0; i < 5; i++ { // 5 sessions, each with a first prompt: only 3 are kept (the earliest)
		j.user(ts(8, i, 0), acme, fmt.Sprintf("s%d", i), fmt.Sprintf("Prompt number %d in its own session", i))
	}
	for i := 0; i < 70; i++ { // 70 files; file i is edited 1 to 3 times (i%3+1), every tool_use id unique
		for k := 0; k <= i%3; k++ {
			j.tool(ts(9, i%50, k), acme, "s0", fmt.Sprintf("e%d-%d", i, k), "Edit", map[string]any{"file_path": fmt.Sprintf("%s/f%02d.go", acme, i)})
		}
	}
	j.tool(ts(9, 55, 0), acme, "s0", "nb", "NotebookEdit", map[string]any{"notebook_path": acme + "/n.ipynb"})
	j.tool(ts(9, 55, 1), acme, "s0", "nb", "NotebookEdit", map[string]any{"notebook_path": acme + "/n.ipynb"}) // a repeated tool_use id counts once
	for i := 0; i < 15; i++ {
		id := fmt.Sprintf("c%d", i)
		j.tool(ts(10, i, 0), acme, "s0", id, "Bash", map[string]any{"command": fmt.Sprintf(`git commit -m "commit %d"`, i)})
		j.result(ts(10, i, 1), acme, "s0", id, fmt.Sprintf("[main a%06d] commit %d", i, i))
	}
	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("p%d", i)
		j.tool(ts(11, i, 0), acme, "s0", id, "Bash", map[string]any{"command": "gh pr create --fill"})
		j.result(ts(11, i, 1), acme, "s0", id, fmt.Sprintf("https://github.com/acme/site/pull/%d", 100+i))
	}
	j.write(t, root, "-p", "s.jsonl", dayEnd)
	res := NewClaudeCodeSource([]string{root}, testHome).Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{acme}, false), nil)

	if n := len(byKind(res.Items, KindPrompt)); n != 3 {
		t.Errorf("prompts = %d, want 3", n)
	}
	if p := byKind(res.Items, KindPrompt); p[0].Title != "Prompt number 0 in its own session" || p[2].Title != "Prompt number 2 in its own session" {
		t.Errorf("the earliest prompts are kept: %+v", p)
	}
	files := byKind(res.Items, KindFile)
	if len(files) != 60 {
		t.Errorf("files = %d, want 60", len(files))
	}
	// most-edited first, then by path: f02.go (3 edits) leads
	if files[0].Count != 3 || files[0].Title != "f02.go" {
		t.Errorf("file order: %+v", files[:3])
	}
	for i := 1; i < len(files); i++ {
		if files[i].Count > files[i-1].Count {
			t.Fatalf("not sorted by edit count at %d", i)
		}
	}
	commits := byKind(res.Items, KindCommit)
	if len(commits) != 12 || commits[0].Title != "commit 0" || commits[11].Title != "commit 11" {
		t.Errorf("commits = %d: %+v", len(commits), commits)
	}
	if n := len(byKind(res.Items, KindPullRequest)); n != 5 {
		t.Errorf("prs = %d, want 5", n)
	}
	if byKind(res.Items, KindSession)[0].Count != 5 {
		t.Errorf("sessions: %+v", byKind(res.Items, KindSession))
	}
	for _, f := range files { // the notebook was seen twice with one id: one edit
		if f.Title == "n.ipynb" && f.Count != 1 {
			t.Errorf("repeated tool_use id counted %d times", f.Count)
		}
	}
}

func TestClaudeCacheAndCancel(t *testing.T) {
	src, root := claudeFixture(t)
	policy := NewProjectPolicy([]string{"/work/acme"}, false)
	first := src.Activity(context.Background(), "2026-10-06", utc, policy, nil)
	if n := len(src.cache.m); n != 3 { // two ticked, one skipped (cached as skipped); the old file is filtered by mtime, the orphan by name
		t.Errorf("cache entries = %d", n)
	}
	second := src.Activity(context.Background(), "2026-10-06", utc, policy, nil)
	if !reflect.DeepEqual(first, second) {
		t.Error("a cached scan must give the same answer")
	}
	// a changed file (new size) is read again
	p := filepath.Join(root, "projects", "-work-acme", "s1.jsonl")
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	extra := &jl{}
	extra.tool(ts(9, 25, 0), "/work/acme", "s1", "t9", "Write", map[string]any{"file_path": "/work/acme/new.go"})
	_, _ = f.WriteString(extra.lines[0] + "\n")
	f.Close()
	_ = os.Chtimes(p, dayEnd, dayEnd)
	third := src.Activity(context.Background(), "2026-10-06", utc, policy, nil)
	if len(byKind(third.Items, KindFile)) != 3 {
		t.Errorf("a grown file must be re-read: %+v", byKind(third.Items, KindFile))
	}
	// a different policy re-reads: a new key
	src.Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme", "/work/secret"}, false), nil)
	// cancellation: nothing is read, nothing cached as unreadable, no notice
	src.ClearCache()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := src.Activity(ctx, "2026-10-06", utc, policy, nil)
	if len(res.Items) != 0 || len(res.Notices) != 0 || len(src.cache.m) != 0 {
		t.Errorf("cancelled: %+v cache %d", res, len(src.cache.m))
	}
	// progress is reported
	var calls [][2]int
	src.Activity(context.Background(), "2026-10-06", utc, policy, func(d, n int) { calls = append(calls, [2]int{d, n}) })
	if len(calls) != 3 || calls[0] != [2]int{0, 3} {
		t.Errorf("progress calls: %v", calls)
	}
}

func TestClaudeUnreadableNotice(t *testing.T) {
	root2 := t.TempDir()
	j := &jl{}
	j.user(ts(9, 0, 0), "/work/acme", "s", "Prompt that nobody can read here")
	q := j.write(t, root2, "-p", "s.jsonl", dayEnd)
	if err := os.Chmod(q, 0); err != nil {
		t.Skip("chmod unavailable")
	}
	defer os.Chmod(q, 0o600)
	if f, err := os.Open(q); err == nil { // running as root: permissions do not apply
		f.Close()
		t.Skip("file is readable regardless of mode")
	}
	res := NewClaudeCodeSource([]string{root2}, testHome).Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil)
	if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "can't read Claude Code's files") {
		t.Errorf("notices: %v", res.Notices)
	}
}

func TestClaudeFormatDrift(t *testing.T) {
	if (ScanStats{Records: 30, Usable: 24}).FormatLooksChanged() { // 24*5 < 30*4 is false: exactly 80% is fine
		t.Error("80% usable is fine")
	}
	if !(ScanStats{Records: 30, Usable: 23}).FormatLooksChanged() {
		t.Error("under 80% usable looks changed")
	}
	if (ScanStats{Records: 29, Usable: 0}).FormatLooksChanged() {
		t.Error("too few records to tell")
	}
	// end to end: 40 user records with no timestamp
	root := t.TempDir()
	j := &jl{}
	for i := 0; i < 40; i++ {
		j.add("user", "", "/work/acme", "s", "Some prompt that has no usable time", nil)
	}
	j.write(t, root, "-p", "s.jsonl", dayEnd)
	res := NewClaudeCodeSource([]string{root}, testHome).Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil)
	if res.Stats.Records != 40 || res.Stats.Usable != 0 || !res.Stats.FormatLooksChanged() {
		t.Errorf("stats: %+v", res.Stats)
	}
}

func TestClaudeBashInputCommit(t *testing.T) {
	root := t.TempDir()
	j := &jl{}
	j.user(ts(9, 0, 0), "/work/acme", "s", "<bash-input>git commit -m \"feat: from a bang command\"</bash-input>")
	j.write(t, root, "-p", "s.jsonl", dayEnd)
	res := NewClaudeCodeSource([]string{root}, testHome).Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{"/work/acme"}, false), nil)
	c := byKind(res.Items, KindCommit)
	if len(c) != 1 || c[0].Title != "feat: from a bang command" || c[0].Evidence != "" {
		t.Errorf("commits: %+v", c)
	}
	if len(byKind(res.Items, KindPrompt)) != 0 {
		t.Error("a bang command is not a prompt")
	}
	// the hashless one is replaced by a hashed twin in the digest
	d := Bullets(append(res.Items, ActivityItem{Kind: KindCommit, Root: "/work/acme", Project: "acme", Title: "feat: from a bang command", Evidence: "1234567"}), testRed)
	if got := texts(d.Bullets); len(got) == 0 || got[len(got)-1] != "Committed: feat: from a bang command (acme 1234567)" || len(byKind(res.Items, KindCommit)) != 1 {
		t.Errorf("bullets: %q", got)
	}
}

func TestClaudeProjectsAndRoots(t *testing.T) {
	src, _ := claudeFixture(t)
	ps := src.Projects(context.Background(), nil)
	var got []string
	for _, p := range ps {
		got = append(got, p.Name+"="+p.Root)
		if !reflect.DeepEqual(p.Sources, []string{"claude"}) || p.LastActive.IsZero() {
			t.Errorf("project: %+v", p)
		}
	}
	if !reflect.DeepEqual(got, []string{"acme=/work/acme", "secret=/work/secret"}) {
		t.Errorf("projects: %v", got)
	}
	env := map[string]string{"CLAUDE_CONFIG_DIR": " /cfg/claude/ "}
	roots := DefaultClaudeRoots(func(k string) string { return env[k] }, "/Users/me")
	if !reflect.DeepEqual(roots, []string{"/cfg/claude", "/Users/me/.claude"}) {
		t.Errorf("roots: %v", roots)
	}
	env["CLAUDE_CONFIG_DIR"] = "/USERS/me/.claude"
	if r := DefaultClaudeRoots(func(k string) string { return env[k] }, "/Users/me"); len(r) != 1 {
		t.Errorf("duplicate root (case-insensitive) kept: %v", r)
	}
	if r := DefaultClaudeRoots(func(string) string { return "" }, "/Users/me"); !reflect.DeepEqual(r, []string{"/Users/me/.claude"}) {
		t.Errorf("roots: %v", r)
	}
}

func TestCwdIn(t *testing.T) {
	cases := map[string]string{
		`{"cwd":"/a/b","x":1}`:         "/a/b",
		`{"cwd": "/a/b"}`:              "/a/b",
		`{"cwd":"/a/\"q\"/b"}`:         `/a/"q"/b`,
		`{"cwd":"/café/x"}`:            "/café/x",
		`{"message":"no folder here"}`: "",
		`{"cwd":"/unterminated`:        "",
	}
	for in, want := range cases {
		got, ok := cwdIn([]byte(in))
		if got != want || (want != "") != ok {
			t.Errorf("cwdIn(%s) = %q,%v want %q", in, got, ok, want)
		}
	}
}
