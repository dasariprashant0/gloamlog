package sources

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	us = "\x1f"
	h1 = "1111111111111111111111111111111111111111"
)

func mkRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGitRepositoryDiscovery(t *testing.T) {
	root := t.TempDir()
	for _, r := range []string{"a", "group/b", "group/deep/x", "g/h/i/j", "node_modules/pkg", ".hidden/r", "Library/l", "vendor/v", "a/sub"} {
		mkRepo(t, filepath.Join(root, r))
	}
	if err := os.MkdirAll(filepath.Join(root, "target"), 0o755); err != nil { // not a repo
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link")); err != nil {
		t.Log("symlinks unavailable:", err)
	}
	// a plain file named like a repo child must not matter
	_ = os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o600)

	g := NewGitSource([]string{root, root + "/", strings.ToUpper(root)}, "git", testHome, func(context.Context, string, []string) (string, bool) { return "", false })
	var got []string
	for _, r := range g.Repositories(context.Background()) {
		got = append(got, strings.TrimPrefix(r, root+"/"))
	}
	// depth 3 is the limit; hidden, symlinked and build folders are skipped; a repo is not descended into; the same root twice is one
	if want := []string{"a", "group/b", "group/deep/x"}; !reflect.DeepEqual(got, want) && !(len(got) > 3 && reflect.DeepEqual(got[:3], want)) {
		t.Errorf("repos: %v", got)
	}
	for _, r := range got {
		if strings.Contains(r, "node_modules") || strings.Contains(r, ".hidden") || strings.Contains(r, "link") || strings.HasPrefix(r, "g/h") || strings.HasPrefix(r, "a/sub") {
			t.Errorf("should not be found: %s", r)
		}
	}
	// a root that is itself a repository is just itself
	one := NewGitSource([]string{filepath.Join(root, "a")}, "git", testHome, nil).Repositories(context.Background())
	if len(one) != 1 || one[0] != filepath.Join(root, "a") {
		t.Errorf("root repo: %v", one)
	}
	// cancelled: nothing
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := g.Repositories(ctx); len(r) != 0 {
		t.Errorf("cancelled: %v", r)
	}
	// the project list names them
	ps := NewGitSource([]string{filepath.Join(root, "group")}, "git", testHome, nil).Projects(context.Background())
	if len(ps) != 2 || ps[0].Name != "b" || !reflect.DeepEqual(ps[0].Sources, []string{"git"}) {
		t.Errorf("projects: %+v", ps)
	}
}

func TestGitRepositoryLimits(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 450; i++ {
		mkRepo(t, filepath.Join(root, "r"+pad3(i)))
	}
	if n := len(NewGitSource([]string{root}, "git", testHome, nil).Repositories(context.Background())); n != 400 {
		t.Errorf("repos = %d, want the 400 cap", n)
	}
}

func pad3(i int) string {
	return string([]byte{byte('0' + i/100), byte('0' + i/10%10), byte('0' + i%10)})
}

func TestGitActivityArgsAndFiltering(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "acme")
	other := filepath.Join(root, "other")
	mkRepo(t, repo)
	mkRepo(t, other)

	zone := time.FixedZone("X", 5*3600+1800) // +0530
	cal := utc
	cal.Loc = zone
	start := time.Date(2026, 10, 6, 0, 0, 0, 0, zone)
	ct := func(d time.Duration) string { return itoa(start.Add(d).Unix()) }

	var calls [][]string
	run := func(_ context.Context, git string, args []string) (string, bool) {
		if git != "/fake/git" {
			t.Errorf("git path %q", git)
		}
		calls = append(calls, args)
		if args[2] == "config" {
			return "  Me@X.com \n", true
		}
		return strings.Join([]string{
			h1 + us + "ME@x.COM" + us + ct(10*time.Hour) + us + "First subject",
			"2222222222222222222222222222222222222222" + us + "someone@else.com" + us + ct(11*time.Hour) + us + "Not mine",
			"3333333333333333333333333333333333333333" + us + "me@x.com" + us + ct(-time.Second) + us + "Day before",
			"4444444444444444444444444444444444444444" + us + "me@x.com" + us + ct(24*time.Hour) + us + "Day after (exactly the end)",
			"5555555555555555555555555555555555555555" + us + "me@x.com" + us + ct(0) + us + "Midnight counts",
			"6666666666666666666666666666666666666666" + us + "me@x.com" + us + ct(12*time.Hour) + us + "Has" + us + "unit separator",
			"7777777777777777777777777777777777777777" + us + "me@x.com" + us + "abc" + us + "Bad time",
			"8888888888888888888888888888888888888888" + us + "me@x.com" + us + ct(time.Hour),
			"",
		}, "\n"), true
	}
	g := NewGitSource([]string{root}, "/fake/git", testHome, run)
	policy := NewProjectPolicy([]string{repo}, false) // other is not ticked
	var progress [][2]int
	res := g.Activity(context.Background(), "2026-10-06", cal, policy, func(d, n int) { progress = append(progress, [2]int{d, n}) })

	wantCalls := [][]string{
		{"-C", repo, "config", "--get", "user.email"},
		{"-C", repo, "-c", "core.fsmonitor=false", "log", "--branches", "--no-merges", "-i", "--fixed-strings", "--author=Me@X.com",
			"--since=2026-10-04T00:00:00+0530", "--until=2026-10-08T00:00:00+0530", "--max-count=300", "--format=%H%x1f%ae%x1f%ct%x1f%s"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls:\n%q\nwant\n%q", calls, wantCalls)
	}
	if !reflect.DeepEqual(progress, [][2]int{{0, 1}}) {
		t.Errorf("progress %v", progress)
	}
	var titles []string
	for _, it := range res.Items {
		titles = append(titles, it.Title)
		if it.Kind != KindCommit || it.Root != repo || it.Project != "acme" || it.Detail != it.Evidence || len(it.Evidence) != 7 {
			t.Errorf("item: %+v", it)
		}
	}
	if want := []string{"First subject", "Midnight counts", "Has" + us + "unit separator"}; !reflect.DeepEqual(titles, want) {
		t.Errorf("titles %q want %q", titles, want)
	}
	if res.Items[0].Evidence != "1111111" || !res.Items[0].Time.Equal(start.Add(10*time.Hour)) {
		t.Errorf("first: %+v", res.Items[0])
	}
	if res.Stats.Files != 1 || res.Stats.Skipped != 0 || len(res.Notices) != 0 {
		t.Errorf("stats %+v notices %v", res.Stats, res.Notices)
	}
	// the commits become "Committed: ..." bullets with their short hash, oldest first (Swift ActivityDigest sorts by time,
	// so the 00:00 commit leads)
	bullets := Bullets(res.Items, testRed).Bullets
	if got := bullets[0].Text; got != "Committed: Midnight counts (acme 5555555)" {
		t.Errorf("first bullet %q", got)
	}
	if got := bullets[1].Text; got != "Committed: First subject (acme 1111111)" {
		t.Errorf("second bullet %q", got)
	}
}

func TestGitActivityNoticesAndGuards(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "acme")
	mkRepo(t, repo)
	policy := NewProjectPolicy([]string{repo}, false)
	never := func(context.Context, string, []string) (string, bool) {
		t.Error("git must not run")
		return "", false
	}
	ctx := context.Background()

	// no git: the notice, nothing run
	res := NewGitSource([]string{root}, "", testHome, never).Activity(ctx, "2026-10-06", utc, policy, nil)
	if !reflect.DeepEqual(res.Notices, []string{"Git isn't installed on this Mac, so commits can't be listed."}) || len(res.Items) != 0 {
		t.Errorf("no git: %+v", res)
	}
	// nothing ticked, no roots, a bad day: silent and nothing run
	for name, r := range map[string]SourceResult{
		"empty policy":                 NewGitSource([]string{root}, "/g", testHome, never).Activity(ctx, "2026-10-06", utc, NewProjectPolicy(nil, false), nil),
		"no roots":                     NewGitSource(nil, "/g", testHome, never).Activity(ctx, "2026-10-06", utc, policy, nil),
		"bad day":                      NewGitSource([]string{root}, "/g", testHome, never).Activity(ctx, "yesterday", utc, policy, nil),
		"unticked":                     NewGitSource([]string{root}, "/g", testHome, never).Activity(ctx, "2026-10-06", utc, NewProjectPolicy([]string{"/elsewhere"}, false), nil),
		"git missing but policy empty": NewGitSource([]string{root}, "", testHome, never).Activity(ctx, "2026-10-06", utc, NewProjectPolicy(nil, false), nil),
	} {
		if len(r.Items) != 0 || len(r.Notices) != 0 || r.Stats != (ScanStats{}) {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// no user.email (the command fails, or prints nothing): the notice, once
	for name, cfg := range map[string]func() (string, bool){"fails": func() (string, bool) { return "", false }, "blank": func() (string, bool) { return " \n", true }} {
		repo2 := filepath.Join(root, "b")
		mkRepo(t, repo2)
		g := NewGitSource([]string{root}, "/g", testHome, func(_ context.Context, _ string, args []string) (string, bool) {
			if args[2] != "config" {
				t.Errorf("%s: log must not run without an email", name)
			}
			return cfg()
		})
		res = g.Activity(ctx, "2026-10-06", utc, NewProjectPolicy([]string{repo, repo2}, false), nil)
		if !reflect.DeepEqual(res.Notices, []string{"Set your email with `git config --global user.email` to see your commits."}) || res.Stats.Files != 0 {
			t.Errorf("%s: %+v", name, res)
		}
	}
	// a failing git log is counted as a skipped record, not a crash, and no notice
	g := NewGitSource([]string{root}, "/g", testHome, func(_ context.Context, _ string, args []string) (string, bool) {
		if args[2] == "config" {
			return "me@x.com", true
		}
		return "", false
	})
	res = g.Activity(ctx, "2026-10-06", utc, policy, nil)
	if res.Stats.Skipped != 1 || res.Stats.Files != 0 || len(res.Notices) != 0 || len(res.Items) != 0 {
		t.Errorf("failed log: %+v", res)
	}
	// cancelled: the runner is not called
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if r := NewGitSource([]string{root}, "/g", testHome, never).Activity(cctx, "2026-10-06", utc, policy, nil); len(r.Items) != 0 {
		t.Errorf("cancelled: %+v", r)
	}
}

func TestLocateGit(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "git-exe")
	plain := filepath.Join(dir, "git-plain")
	_ = os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)
	_ = os.WriteFile(plain, []byte("x"), 0o644)
	if got := locateGitIn([]string{filepath.Join(dir, "missing"), dir, plain, exe}); got != exe {
		t.Errorf("got %q", got)
	}
	if locateGitIn([]string{plain}) != "" || locateGitIn(nil) != "" {
		t.Error("none")
	}
	for _, c := range gitCandidates {
		if c == "/usr/bin/git" {
			t.Error("/usr/bin/git is the install-prompt stub and must never be used")
		}
	}
	if !reflect.DeepEqual(gitCandidates, []string{"/Library/Developer/CommandLineTools/usr/bin/git", "/Applications/Xcode.app/Contents/Developer/usr/bin/git", "/opt/homebrew/bin/git", "/usr/local/bin/git"}) {
		t.Errorf("candidates: %v", gitCandidates)
	}
}

// ---- real git in tiny fixture repositories ------------------------------------------------------------------------

func realGit(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git binary")
	}
	return p
}

type fixtureGit struct {
	t    *testing.T
	git  string
	home string
}

func (f fixtureGit) run(dir string, env []string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command(f.git, append([]string{"-C", dir}, args...)...)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin", "HOME=" + f.home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// commit makes an empty commit at the given instant by author (name <email>); the committer is always the owner.
func (f fixtureGit) commit(dir, msg, at, author string, extra ...string) {
	f.t.Helper()
	env := []string{"GIT_AUTHOR_DATE=" + at, "GIT_COMMITTER_DATE=" + at, "GIT_COMMITTER_NAME=Me", "GIT_COMMITTER_EMAIL=me@x.com"}
	args := append([]string{"commit", "-q", "--allow-empty", "-m", msg, "--author=" + author}, extra...)
	f.run(dir, env, args...)
}

func (f fixtureGit) init(dir string, email string) {
	f.t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.run(dir, nil, "init", "-q", "-b", "main")
	if email != "" {
		f.run(dir, nil, "config", "user.email", email)
		f.run(dir, nil, "config", "user.name", "Me")
	}
}

func TestGitRealRepositories(t *testing.T) {
	gitBin := realGit(t)
	base := t.TempDir()
	f := fixtureGit{t, gitBin, filepath.Join(base, "home")}
	_ = os.MkdirAll(f.home, 0o755)
	work := filepath.Join(base, "work")
	acme, other, noMail := filepath.Join(work, "acme"), filepath.Join(work, "other"), filepath.Join(work, "nomail")

	f.init(acme, "Me@X.com") // the configured case differs from the author's: matching is case-insensitive
	f.commit(acme, "Day before", "2026-10-05T23:59:59+0000", "Me <me@x.com>")
	f.commit(acme, "Add pricing: table", "2026-10-06T10:00:00+0000", "Me <me@x.com>")
	f.commit(acme, "Someone else's work", "2026-10-06T11:00:00+0000", "Other <other@x.com>")
	f.commit(acme, "Next day starts", "2026-10-07T00:00:00+0000", "Me <me@x.com>")
	f.run(acme, nil, "checkout", "-q", "-b", "feat")
	f.commit(acme, "Feature branch work", "2026-10-06T12:00:00+0000", "Me <me@x.com>")
	f.run(acme, nil, "checkout", "-q", "main")
	f.run(acme, []string{"GIT_AUTHOR_DATE=2026-10-06T13:00:00+0000", "GIT_COMMITTER_DATE=2026-10-06T13:00:00+0000", "GIT_AUTHOR_NAME=Me", "GIT_AUTHOR_EMAIL=me@x.com", "GIT_COMMITTER_NAME=Me", "GIT_COMMITTER_EMAIL=me@x.com"},
		"merge", "-q", "--no-ff", "-m", "Merge feat", "feat")

	f.init(other, "me@x.com") // a ticked-out repo with a commit on the day
	f.commit(other, "Other repo commit", "2026-10-06T09:00:00+0000", "Me <me@x.com>")
	f.init(noMail, "") // no user.email anywhere (HOME is empty)
	f.commit(noMail, "Anonymous", "2026-10-06T09:00:00+0000", "Me <me@x.com>")

	g := NewGitSource([]string{work}, gitBin, f.home, nil)
	res := g.Activity(context.Background(), "2026-10-06", utc, NewProjectPolicy([]string{acme, noMail}, false), nil)
	var titles []string
	for _, it := range res.Items {
		titles = append(titles, it.Title)
		if it.Root != acme || len(it.Evidence) != 7 || it.Time.IsZero() {
			t.Errorf("item %+v", it)
		}
	}
	sort.Strings(titles)
	if want := []string{"Add pricing: table", "Feature branch work"}; !reflect.DeepEqual(titles, want) {
		t.Errorf("titles %q want %q (own commits only, that day only, all branches, no merges, ticked repos only)", titles, want)
	}
	if res.Stats.Files != 1 || !reflect.DeepEqual(res.Notices, []string{"Set your email with `git config --global user.email` to see your commits."}) {
		t.Errorf("stats %+v notices %v", res.Stats, res.Notices)
	}
	// the evidence is the real short hash
	want := strings.TrimSpace(f.run(acme, nil, "log", "-1", "--format=%h", "--grep=Add pricing"))
	for _, it := range res.Items {
		if it.Title == "Add pricing: table" && !strings.HasPrefix(want, it.Evidence[:7]) && it.Evidence[:7] != want[:7] {
			t.Errorf("sha %q vs %q", it.Evidence, want)
		}
	}
	// the other day sees the other commits
	res = g.Activity(context.Background(), "2026-10-07", utc, NewProjectPolicy([]string{acme}, false), nil)
	if len(res.Items) != 1 || res.Items[0].Title != "Next day starts" {
		t.Errorf("next day: %+v", res.Items)
	}
	// the default runner stops when the context is cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := DefaultGitRunner(f.home)(ctx, gitBin, []string{"--version"}); ok {
		t.Error("a cancelled context must not run git")
	}
	if out, ok := DefaultGitRunner(f.home)(context.Background(), gitBin, []string{"--version"}); !ok || !strings.HasPrefix(out, "git version") {
		t.Errorf("version: %q %v", out, ok)
	}
	if _, ok := DefaultGitRunner(f.home)(context.Background(), gitBin, []string{"-C", filepath.Join(base, "nope"), "log"}); ok {
		t.Error("a failing git is not ok")
	}
}

func TestCapBuffer(t *testing.T) {
	c := &capBuffer{limit: 5}
	n, _ := c.Write([]byte("abc"))
	m, _ := c.Write([]byte("defgh"))
	if n != 3 || m != 5 || c.buf.String() != "abcde" {
		t.Errorf("got %q %d %d", c.buf.String(), n, m)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
