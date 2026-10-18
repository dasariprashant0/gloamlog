package sources

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// The rule that matters: NOTHING is read, counted or shown for a project until the owner ticks it. A project is its root
// folder (a Claude Code session's working folder with git worktrees folded into their main folder, or a Git repository's top
// folder); matching is exact on that folder, case-insensitive (the default APFS volume is), never a prefix, so ticking a
// repository can never switch on whatever happens to sit inside it, and ticking nothing reads nothing.

const worktreeMark = "/.claude/worktrees/"

// NormalizePath is absolute-looking, `~` expanded, no `.` / `..` and no trailing slash.
// ponytail: NSString.standardizingPath also drops a leading /private when the shorter path exists; not reproduced.
func NormalizePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			p = h + p[1:]
		}
	}
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// CollapseWorktree: `<root>/.claude/worktrees/<name>[/...]` is the same project as `<root>`.
func CollapseWorktree(p string) string {
	s := NormalizePath(p)
	if i := strings.Index(s, worktreeMark); i >= 0 {
		return s[:i]
	}
	return s
}

// ProjectName is what a project is called: its folder name, or "Home" for the home folder itself.
func ProjectName(root, home string) string {
	if strings.EqualFold(NormalizePath(root), NormalizePath(home)) {
		return "Home" // not the account name
	}
	t := strings.TrimRight(root, "/")
	if i := strings.LastIndex(t, "/"); i >= 0 {
		t = t[i+1:]
	}
	if t == "" {
		return root
	}
	return t
}

// PathContains is true when path is root itself or inside it (whole path components, case-insensitive).
func PathContains(root, path string) bool {
	r, p := strings.ToLower(NormalizePath(root)), strings.ToLower(NormalizePath(path))
	if p == r {
		return true
	}
	if r == "/" {
		return strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, r+"/")
}

// RelativePath is path as seen from the project: relative to the root (or to its worktree), else just the file name.
func RelativePath(path, root string) string {
	p, base := NormalizePath(path), NormalizePath(root)
	lower := strings.ToLower(p)
	if i := strings.Index(p, worktreeMark); i >= 0 && strings.HasPrefix(lower, strings.ToLower(base)+"/") {
		rest := p[i+len(worktreeMark):]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[slash+1:]
		}
	}
	if strings.HasPrefix(lower, strings.ToLower(base)+"/") {
		return p[len(base)+1:]
	}
	if j := strings.LastIndex(p, "/"); j >= 0 && j < len(p)-1 {
		return p[j+1:]
	}
	return p
}

// ProjectPolicy says which projects Sources may read.
type ProjectPolicy struct {
	Allowed []string // the ticked project roots, as Settings hold them
	// All reads every project (each Claude Code session and tab, wherever it ran) except the ones in the table, which then holds the skipped roots.
	All   bool
	table map[string]string // lower-cased normalised root -> normalised root (ticked, or skipped when All)
}

// NewProjectPolicy builds a policy from ticked (or, with all, skipped) roots.
func NewProjectPolicy(allowed []string, all bool) ProjectPolicy {
	t := map[string]string{}
	for _, a := range allowed {
		n := NormalizePath(a)
		t[strings.ToLower(n)] = n
	}
	return ProjectPolicy{Allowed: allowed, All: all, table: t}
}

// PolicyFromPrefs is the policy Settings > Sources describes.
func PolicyFromPrefs(p core.SourcesPrefs) ProjectPolicy {
	return NewProjectPolicy(p.Allowed, p.AllProjects)
}

// IsEmpty is true when nothing at all may be read.
func (p ProjectPolicy) IsEmpty() bool { return !p.All && len(p.table) == 0 }

// Key is a short stable text for "which projects were ticked", used to key caches.
func (p ProjectPolicy) Key() string {
	keys := make([]string, 0, len(p.table))
	for k := range p.table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	k := strings.Join(keys, "\n")
	if p.All {
		return "all\n" + k
	}
	return k
}

// Root is the root that is this project (worktrees folded), or ok=false: then nothing about it may be read or shown.
func (p ProjectPolicy) Root(path string) (string, bool) {
	n := CollapseWorktree(path)
	_, listed := p.table[strings.ToLower(n)]
	if p.All {
		return n, !listed
	}
	r, ok := p.table[strings.ToLower(n)]
	return r, ok
}
