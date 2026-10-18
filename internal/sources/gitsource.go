package sources

// Your own commits on a day, from the repositories you ticked.
//
// READ-ONLY and local: it runs `git log` (and `git config --get user.email`), which never writes and never touches the network,
// without a shell, with a minimal environment, no terminal and no pager, and a timeout. It only runs in repositories the owner
// ticked, and only shows commits whose author email is the owner's own git `user.email`; merge commits are left out.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

const (
	GitTimeout        = 10 * time.Second
	maxRepos          = 400
	maxVisited        = 5000
	maxCommitsPerRepo = 300
	maxGitOutput      = 1_000_000
)

// GitRunner runs git and returns its output (at most 1 MB), or ok=false when it failed or took too long.
// Tests pass their own; production uses DefaultGitRunner.
type GitRunner func(ctx context.Context, git string, args []string) (out string, ok bool)

// GitSource finds the owner's commits in the repositories under Roots.
type GitSource struct {
	Roots   []string  // folders the owner added: a repository itself, or a folder that holds repositories (looked at 3 levels deep)
	GitPath string    // "" = git is not installed
	Home    string    // the home folder, for the name "Home"
	Run     GitRunner // nil = DefaultGitRunner(Home)
}

// NewGitSource normalises roots.
func NewGitSource(roots []string, gitPath, home string, run GitRunner) *GitSource {
	norm := make([]string, len(roots))
	for i, r := range roots {
		norm[i] = NormalizePath(r)
	}
	if run == nil {
		run = DefaultGitRunner(home)
	}
	return &GitSource{Roots: norm, GitPath: gitPath, Home: home, Run: run}
}

// gitCandidates are the git binaries looked for, in order. /usr/bin/git is never used: without the developer tools it is a
// stub that opens an install dialog.
var gitCandidates = []string{"/Library/Developer/CommandLineTools/usr/bin/git", "/Applications/Xcode.app/Contents/Developer/usr/bin/git",
	"/opt/homebrew/bin/git", "/usr/local/bin/git"}

// LocateGit is the first executable of the known real git binaries, or "".
func LocateGit() string { return locateGitIn(gitCandidates) }

func locateGitIn(candidates []string) string {
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return c
		}
	}
	return ""
}

var skipDirs = map[string]bool{"node_modules": true, "Library": true, "Pods": true, "venv": true, "target": true, "build": true, "dist": true, "vendor": true}

// Repositories are the repositories under the added folders (a folder with `.git` is one; what is inside it belongs to it).
// Symbolic links, hidden folders and the usual build folders are skipped, and the walk is bounded, so adding a huge folder
// cannot hang the app.
func (g *GitSource) Repositories(ctx context.Context) []string {
	var found []string
	visited := 0
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if visited >= maxVisited || len(found) >= maxRepos || cancelled(ctx) {
			return
		}
		visited++
		if _, err := os.Stat(dir + "/.git"); err == nil {
			found = append(found, dir)
			return
		}
		if depth >= 3 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, k := range names {
			if strings.HasPrefix(k, ".") || skipDirs[k] {
				continue
			}
			p := dir + "/" + k
			if st, err := os.Lstat(p); err != nil || !st.IsDir() { // a symlink is not a directory to Lstat
				continue
			}
			walk(p, depth+1)
		}
	}
	for _, r := range g.Roots {
		walk(r, 0)
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range found {
		if k := strings.ToLower(r); !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}

// Projects are the repositories this source can see (names only), for the project list in Settings.
func (g *GitSource) Projects(ctx context.Context) []SourceProject {
	var out []SourceProject
	for _, r := range g.Repositories(ctx) {
		out = append(out, SourceProject{Root: r, Name: ProjectName(r, g.Home), Sources: []string{"git"}})
	}
	return out
}

// Activity is the owner's commits on day ("yyyy-MM-dd" in cal's zone), only in repositories policy allows.
func (g *GitSource) Activity(ctx context.Context, day string, cal core.Calendar, policy ProjectPolicy, progress Progress) SourceResult {
	var res SourceResult
	start, end, ok := dayBounds(day, cal)
	if policy.IsEmpty() || len(g.Roots) == 0 || !ok {
		return res
	}
	if g.GitPath == "" {
		res.Notices = append(res.Notices, "Git isn't installed on this Mac, so commits can't be listed.")
		return res
	}
	loc := start.Location()
	const layout = "2006-01-02T15:04:05-0700" // DateFormatter's yyyy-MM-dd'T'HH:mm:ssZZZ
	// git stops walking at the first commit older than --since, so a history whose dates are not strictly in order (a clock that
	// was off, a rebase) would hide later commits: ask for a wider window and cut to the day here, by commit time.
	since := start.In(loc).AddDate(0, 0, -2).Format(layout)
	until := end.In(loc).AddDate(0, 0, 1).Format(layout)
	lo, hi := float64(start.Unix()), float64(end.Unix())

	type repo struct{ path, root string }
	var repos []repo
	for _, r := range g.Repositories(ctx) {
		if root, ok := policy.Root(r); ok {
			repos = append(repos, repo{r, root})
		}
	}
	noEmail := 0
	for i, r := range repos {
		if cancelled(ctx) {
			break
		}
		progress.report(i, len(repos))
		out, ok := g.Run(ctx, g.GitPath, []string{"-C", r.path, "config", "--get", "user.email"})
		email := trim(out)
		if !ok || email == "" {
			noEmail++
			continue
		}
		args := []string{"-C", r.path, "-c", "core.fsmonitor=false", "log", "--branches", "--no-merges", "-i", "--fixed-strings", "--author=" + email,
			"--since=" + since, "--until=" + until, "--max-count=" + strconv.Itoa(maxCommitsPerRepo), "--format=%H%x1f%ae%x1f%ct%x1f%s"}
		log, ok := g.Run(ctx, g.GitPath, args)
		if !ok {
			res.Stats.Skipped++
			continue
		}
		res.Stats.Files++
		name := ProjectName(r.root, g.Home)
		for _, line := range strings.Split(log, "\n") {
			if line == "" {
				continue
			}
			p := strings.Split(line, "\x1f")
			if len(p) < 4 || !strings.EqualFold(p[1], email) {
				continue
			}
			ct, err := strconv.ParseFloat(p[2], 64)
			if err != nil || ct < lo || ct >= hi {
				continue
			}
			sha := p[0]
			if len(sha) > 7 {
				sha = sha[:7]
			}
			res.Items = append(res.Items, ActivityItem{Kind: KindCommit, Root: r.root, Project: name, Title: strings.Join(p[3:], "\x1f"),
				Detail: sha, Time: unixTime(ct), Evidence: sha, Count: 1})
		}
	}
	if noEmail > 0 {
		res.Notices = append(res.Notices, "Set your email with `git config --global user.email` to see your commits.")
	}
	return res
}

// capBuffer keeps the first limit bytes and drops the rest (git is still read to its end, like Swift's prefix(1_000_000)).
type capBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// DefaultGitRunner runs git with no shell, no terminal, no pager, no prompts and no repository locks; the environment is only
// what git needs to find its own configuration. Output is capped at 1 MB; git is killed after GitTimeout.
func DefaultGitRunner(home string) GitRunner {
	env := []string{"PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin", "HOME=" + home, "GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat", "GIT_CONFIG_NOSYSTEM=1"}
	return func(ctx context.Context, git string, args []string) (string, bool) {
		ctx, cancel := context.WithTimeout(ctx, GitTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Env = env
		cmd.WaitDelay = 2 * time.Second  // a killed git whose child still holds the pipe must not hang us
		cmd.Stdin, cmd.Stderr = nil, nil // /dev/null
		out := &capBuffer{limit: maxGitOutput}
		cmd.Stdout = out
		if err := cmd.Run(); err != nil {
			return "", false
		}
		return strings.ToValidUTF8(out.buf.String(), "�"), true
	}
}
