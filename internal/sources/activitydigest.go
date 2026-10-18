package sources

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// SuggestedBullet is one line the owner can insert.
type SuggestedBullet struct {
	ID       string       `json:"id"`
	Kind     ActivityKind `json:"kind"`
	Project  string       `json:"project"`
	Text     string       `json:"text"`     // the bullet as it reads on the page, redacted, one line
	Detail   string       `json:"detail"`   // a second line for the strip only (the files touched)
	Evidence string       `json:"evidence"` // the part of Text that shows the bullet is already on the page
	Time     time.Time    `json:"time"`     // zero = unknown
}

// DigestResult is the bullets and what the redactor hid, for "4 values hidden (2 emails, 2 tokens)".
type DigestResult struct {
	Bullets []SuggestedBullet
	Hidden  map[RedactKind]int
}

const (
	maxPrompt  = 140
	maxSubject = 120
	maxName    = 60
)

// Bullets turns what the sources found into the short bullets the owner can insert. This is the ONE place text leaves the
// sources: every string that can reach the screen or a page is redacted here, cut to a short length AFTER redaction (so a
// secret is never cut in half and left half-visible), and put in one line.
func Bullets(raw []ActivityItem, red Redactor) DigestResult {
	hidden := map[RedactKind]int{}
	clean := func(s string, limit int) string {
		t, h := red.Apply(s)
		for k, n := range h {
			hidden[k] += n
		}
		return truncate(collapse(t), limit)
	}

	// The same item twice (two sources, or a session and its copy) is one item; a commit without a hash that another item
	// already shows with one is that commit.
	seen := map[string]bool{}
	var items []ActivityItem
	for _, it := range raw {
		if !seen[it.ID()] {
			seen[it.ID()] = true
			items = append(items, it)
		}
	}
	known := map[string]bool{}
	for _, it := range items {
		if it.Kind == KindCommit && it.Evidence != "" {
			known[it.Root+"|"+strings.ToLower(it.Title)] = true
		}
	}
	kept := items[:0:0]
	for _, it := range items {
		if it.Kind == KindCommit && it.Evidence == "" && known[it.Root+"|"+strings.ToLower(it.Title)] {
			continue
		}
		kept = append(kept, it)
	}
	items = kept

	// group by root, keeping first-seen order inside a group
	groups := map[string][]ActivityItem{}
	var roots []string
	for _, it := range items {
		if _, ok := groups[it.Root]; !ok {
			roots = append(roots, it.Root)
		}
		groups[it.Root] = append(groups[it.Root], it)
	}
	type rankKey struct {
		secs, commits int
		name, root    string
	}
	rank := func(root string) rankKey {
		g := groups[root]
		k := rankKey{root: root, name: strings.ToLower(g[0].Project)}
		for _, it := range g {
			if it.Kind == KindSession {
				k.secs = -it.Seconds
				break
			}
		}
		for _, it := range g {
			if it.Kind == KindCommit {
				k.commits--
			}
		}
		return k
	}
	sort.SliceStable(roots, func(i, j int) bool {
		a, b := rank(roots[i]), rank(roots[j])
		if a.secs != b.secs {
			return a.secs < b.secs
		}
		if a.commits != b.commits {
			return a.commits < b.commits
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.root < b.root // ponytail: Swift left ties to dictionary order; a stable tie-break is kinder
	})

	var out []SuggestedBullet
	for _, root := range roots {
		g := groups[root]
		first := g[0]
		name := clean(first.Project, maxName)

		var session *ActivityItem
		for i := range g {
			if g[i].Kind == KindSession {
				session = &g[i]
				break
			}
		}
		if session != nil {
			s := *session
			var files []ActivityItem
			for _, it := range g {
				if it.Kind == KindFile {
					files = append(files, it)
				}
			}
			parts := []string{plural(s.Count, "session")}
			if len(files) > 0 {
				parts = append(parts, plural(len(files), "file")+" edited")
			}
			lead := "Worked in " + name + " with Claude Code"
			span := ""
			if s.Seconds >= 60 {
				span = " for about " + Duration(s.Seconds)
			}
			var dets []string
			for _, f := range files[:min(3, len(files))] {
				d := clean(f.Title, 60)
				if f.Count > 1 {
					d += fmt.Sprintf(" (%d)", f.Count)
				}
				dets = append(dets, d)
			}
			detail := strings.Join(dets, ", ")
			if len(files) > 3 {
				detail += fmt.Sprintf(", +%d more", len(files)-3)
			}
			out = append(out, SuggestedBullet{ID: "session|" + first.Root, Kind: KindSession, Project: name,
				Text: lead + span + " (" + strings.Join(parts, ", ") + ")", Detail: detail, Evidence: lead, Time: s.Time})
		}
		for _, p := range g {
			if p.Kind != KindPrompt {
				continue
			}
			line := clean(p.Title, maxPrompt)
			if runeLen(trim(strings.ReplaceAll(line, "[redacted]", ""))) < 8 {
				continue
			}
			out = append(out, SuggestedBullet{ID: "prompt|" + first.Root + "|" + line, Kind: KindPrompt, Project: name,
				Text: "Worked on " + name + ": " + line, Evidence: line, Time: p.Time})
		}
		var commits []ActivityItem
		for _, it := range g {
			if it.Kind == KindCommit {
				commits = append(commits, it)
			}
		}
		sort.SliceStable(commits, func(i, j int) bool { return commits[i].Time.Before(commits[j].Time) })
		for _, c := range commits {
			subject := clean(c.Title, maxSubject)
			if subject == "" {
				continue
			}
			sha := clean(c.Evidence, 12)
			id := sha
			if id == "" {
				id = subject
			}
			text := "Committed: " + subject + " (" + name
			ev := subject
			if sha != "" {
				text += " " + sha
				ev = name + " " + sha
			}
			out = append(out, SuggestedBullet{ID: "commit|" + first.Root + "|" + id, Kind: KindCommit, Project: name,
				Text: text + ")", Evidence: ev, Time: c.Time})
		}
		for _, p := range g {
			if p.Kind != KindPullRequest {
				continue
			}
			url := clean(p.Title, 200)
			out = append(out, SuggestedBullet{ID: "pr|" + first.Root + "|" + url, Kind: KindPullRequest, Project: name,
				Text: "Pull request: " + url + " (" + name + ")", Evidence: url, Time: p.Time})
		}
	}
	return DigestResult{Bullets: out, Hidden: hidden}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// BulletsMarkdown is the bullets as markdown list items. Everything that could mean something to markdown is
// backslash-escaped, so what was typed in a commit message or a prompt can never become a heading, a link, a table or raw
// HTML on the page.
func BulletsMarkdown(bullets []SuggestedBullet) string {
	lines := make([]string, len(bullets))
	for i, b := range bullets {
		lines[i] = "- " + EscapeMarkdown(b.Text)
	}
	return strings.Join(lines, "\n")
}

// EscapeMarkdown backslash-escapes \ ` * _ [ ] < > ~ | & #.
func EscapeMarkdown(s string) string {
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune("\\`*_[]<>~|&#", c) {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

var mdEscapes = regexp.MustCompile("\\\\([!-/:-@\\[-`{-~])")

// PlainText is a page's markdown with escapes undone and whitespace collapsed: what a reader sees, for comparing with Evidence.
func PlainText(markdown string) string {
	return collapse(mdEscapes.ReplaceAllString(markdown, "$1"))
}

// IsAdded: is the bullet already on the page (its evidence is a substring of the page's plain text)?
func IsAdded(b SuggestedBullet, plainPage string) bool {
	e := collapse(b.Evidence)
	return e != "" && strings.Contains(plainPage, e)
}

// Duration is the "about" time: under a minute, minutes, then hours and minutes; rounded to 5 minutes from 10 minutes up.
func Duration(seconds int) string {
	if seconds < 60 {
		return "under a minute"
	}
	m := seconds / 60
	if m >= 10 {
		m = int(math.Round(float64(m)/5)) * 5
	}
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

// truncate is at most limit characters, cut at a word when one is near the end, with an ellipsis.
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	cut := r[:limit]
	for i := len(cut) - 1; i >= 0; i-- {
		if cut[i] == ' ' {
			if i > limit/2 {
				cut = cut[:i]
			}
			break
		}
	}
	for len(cut) > 0 && strings.ContainsRune(" ,;:.-–—", cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	return string(cut) + "…"
}

// collapse puts all runs of whitespace into single spaces and trims the ends.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func trim(s string) string { return strings.TrimFunc(s, unicode.IsSpace) }

func runeLen(s string) int { return len([]rune(s)) }
