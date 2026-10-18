package sources

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

var testRed = NewRedactor("/Users/me")

func texts(bs []SuggestedBullet) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Text
	}
	return out
}

func TestBulletsExactTexts(t *testing.T) {
	at := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	root := "/work/acme"
	items := []ActivityItem{
		{Kind: KindSession, Root: root, Project: "acme", Title: "Claude Code", Count: 3, Seconds: 175 * 60, Time: at},
		{Kind: KindFile, Root: root, Project: "acme", Title: "src/a.go", Count: 4},
		{Kind: KindFile, Root: root, Project: "acme", Title: "b.go", Count: 1},
		{Kind: KindFile, Root: root, Project: "acme", Title: "c.go", Count: 2},
		{Kind: KindFile, Root: root, Project: "acme", Title: "d.go", Count: 1},
		{Kind: KindFile, Root: root, Project: "acme", Title: "e.go", Count: 1},
		{Kind: KindFile, Root: root, Project: "acme", Title: "f.go", Count: 1},
		{Kind: KindPrompt, Root: root, Project: "acme", Title: "Add a pricing table to the landing page", Time: at},
		{Kind: KindCommit, Root: root, Project: "acme", Title: "Add pricing table", Detail: "3f2a9c1", Evidence: "3f2a9c1", Time: at.Add(time.Hour)},
		{Kind: KindPullRequest, Root: root, Project: "acme", Title: "https://github.com/acme/site/pull/42", Evidence: "https://github.com/acme/site/pull/42"},
	}
	d := Bullets(items, testRed)
	want := []string{
		"Worked in acme with Claude Code for about 2h 55m (3 sessions, 6 files edited)",
		"Worked on acme: Add a pricing table to the landing page",
		"Committed: Add pricing table (acme 3f2a9c1)",
		"Pull request: https://github.com/acme/site/pull/42 (acme)",
	}
	if got := texts(d.Bullets); !reflect.DeepEqual(got, want) {
		t.Fatalf("texts:\n%q\nwant\n%q", got, want)
	}
	if len(d.Hidden) != 0 {
		t.Errorf("hidden = %v", d.Hidden)
	}
	s := d.Bullets[0]
	if s.ID != "session|/work/acme" || s.Kind != KindSession || s.Project != "acme" || s.Evidence != "Worked in acme with Claude Code" || !s.Time.Equal(at) {
		t.Errorf("session bullet: %+v", s)
	}
	if s.Detail != "src/a.go (4), b.go, c.go (2), +3 more" {
		t.Errorf("detail = %q", s.Detail)
	}
	if b := d.Bullets[1]; b.ID != "prompt|/work/acme|Add a pricing table to the landing page" || b.Evidence != "Add a pricing table to the landing page" {
		t.Errorf("prompt bullet: %+v", b)
	}
	if b := d.Bullets[2]; b.ID != "commit|/work/acme|3f2a9c1" || b.Evidence != "acme 3f2a9c1" {
		t.Errorf("commit bullet: %+v", b)
	}
	if b := d.Bullets[3]; b.ID != "pr|/work/acme|https://github.com/acme/site/pull/42" || b.Evidence != "https://github.com/acme/site/pull/42" {
		t.Errorf("pr bullet: %+v", b)
	}
	// singular forms and a short session have no duration
	one := Bullets([]ActivityItem{{Kind: KindSession, Root: root, Project: "acme", Count: 1, Seconds: 30},
		{Kind: KindFile, Root: root, Project: "acme", Title: "x.go", Count: 1}}, testRed)
	if got := one.Bullets[0].Text; got != "Worked in acme with Claude Code (1 session, 1 file edited)" {
		t.Errorf("got %q", got)
	}
	noFiles := Bullets([]ActivityItem{{Kind: KindSession, Root: root, Project: "acme", Count: 2, Seconds: 60}}, testRed)
	if got := noFiles.Bullets[0].Text; got != "Worked in acme with Claude Code for about 1 min (2 sessions)" {
		t.Errorf("got %q", got)
	}
}

func TestDuration(t *testing.T) {
	cases := map[int]string{0: "under a minute", 59: "under a minute", 60: "1 min", 540: "9 min", 600: "10 min", 12 * 60: "10 min", 13 * 60: "15 min",
		58 * 60: "1h", 3600: "1h", 3660: "1h", 125 * 60: "2h 5m", 175 * 60: "2h 55m", 24 * 3600: "24h"}
	for sec, want := range cases {
		if got := Duration(sec); got != want {
			t.Errorf("Duration(%d) = %q, want %q", sec, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" || truncate("exactly10!", 10) != "exactly10!" {
		t.Error("no cut at or under the limit")
	}
	if got := truncate("one two three four five", 14); got != "one two three…" { // cut at the space near the end
		t.Errorf("got %q", got)
	}
	if got := truncate("abcdefghijklmnopqrstuvwxyz", 10); got != "abcdefghij…" {
		t.Errorf("no space: %q", got)
	}
	if got := truncate("ab cdefghijklmnopqrstuv", 12); got != "ab cdefghijk…" { // the only space is in the first half: cut at the limit
		t.Errorf("early space: %q", got)
	}
	if got := truncate("fix: the thing, - and more words", 16); got != "fix: the thing…" {
		t.Errorf("trailing punctuation: %q", got)
	}
	if got := truncate("ééééé ééééé ééééé", 8); got != "ééééé…" {
		t.Errorf("counts characters, not bytes: %q", got)
	}
}

func TestBulletsLimitsAndRedaction(t *testing.T) {
	root := "/work/acme"
	words := strings.TrimSpace(strings.Repeat("alpha ", 80))
	d := Bullets([]ActivityItem{
		{Kind: KindPrompt, Root: root, Project: strings.Repeat("n", 90), Title: words},
		{Kind: KindCommit, Root: root, Project: strings.Repeat("n", 90), Title: words, Evidence: "abcdef1"},
		{Kind: KindPullRequest, Root: root, Project: "p", Title: "https://github.com/a/" + strings.Repeat("z", 250) + "/pull/1", Evidence: "x"},
	}, testRed)
	if len(d.Bullets) != 3 {
		t.Fatalf("bullets: %d", len(d.Bullets))
	}
	name := d.Bullets[0].Project
	if len([]rune(name)) != 61 || !strings.HasSuffix(name, "…") { // 60 + ellipsis
		t.Errorf("name %d %q", len([]rune(name)), name)
	}
	if line := strings.TrimPrefix(d.Bullets[0].Text, "Worked on "+name+": "); len([]rune(line)) > 141 || !strings.HasSuffix(line, "…") {
		t.Errorf("prompt line %d", len([]rune(line)))
	}
	if sub := strings.TrimSuffix(strings.TrimPrefix(d.Bullets[1].Text, "Committed: "), " ("+name+" abcdef1)"); len([]rune(sub)) > 121 || !strings.HasSuffix(sub, "…") {
		t.Errorf("subject %d", len([]rune(sub)))
	}
	if u := strings.TrimSuffix(strings.TrimPrefix(d.Bullets[2].Text, "Pull request: "), " ("+name+")"); len([]rune(u)) > 201 || !strings.HasSuffix(u, "…") {
		t.Errorf("url %d", len([]rune(u)))
	}

	// redaction happens before the cut: a secret is never left half-visible
	prose := strings.TrimSpace(strings.Repeat("word ", 27)) // 134 characters
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789"
	r := Bullets([]ActivityItem{{Kind: KindPrompt, Root: root, Project: "acme", Title: prose + " " + secret + " tail words"}}, testRed)
	if got := r.Bullets[0].Text; strings.Contains(got, "sk-") || strings.Contains(got, "sk") && strings.Contains(got, "abcd") {
		t.Errorf("secret visible: %q", got)
	}
	r = Bullets([]ActivityItem{
		{Kind: KindPrompt, Root: root, Project: "acme", Title: "Deploy with token=abc123 to me@example.com today"},
		{Kind: KindPrompt, Root: root, Project: "acme", Title: "token=abc123"}, // under 8 characters once the secret is gone
	}, testRed)
	if got := texts(r.Bullets); !reflect.DeepEqual(got, []string{"Worked on acme: Deploy with token=[redacted] to [email] today"}) {
		t.Errorf("texts: %q", got)
	}
	if r.Hidden[RedactToken] != 2 || r.Hidden[RedactEmail] != 1 { // the skipped prompt's secret still counts, like Swift
		t.Errorf("hidden = %v", r.Hidden)
	}
	if got := RedactSummary(r.Hidden); got != "3 values hidden (1 email, 2 tokens)" {
		t.Errorf("summary %q", got)
	}
	// short or empty
	e := Bullets([]ActivityItem{{Kind: KindPrompt, Root: root, Project: "acme", Title: "short"}, {Kind: KindCommit, Root: root, Project: "acme", Title: "   "}}, testRed)
	if len(e.Bullets) != 0 {
		t.Errorf("got %q", texts(e.Bullets))
	}
	// the project name is redacted and folded too
	h := Bullets([]ActivityItem{{Kind: KindPrompt, Root: "/Users/me/p", Project: "/Users/me/p", Title: "Do the thing properly now"}}, testRed)
	if got := h.Bullets[0].Text; got != "Worked on ~/p: Do the thing properly now" {
		t.Errorf("got %q", got)
	}
}

func TestBulletsDedupe(t *testing.T) {
	root := "/work/acme"
	c := func(title, sha string) ActivityItem {
		return ActivityItem{Kind: KindCommit, Root: root, Project: "acme", Title: title, Evidence: sha, Detail: sha}
	}
	d := Bullets([]ActivityItem{
		c("Add pricing", "3f2a9c1"),
		c("Add pricing", "3f2a9c1"), // the same commit from two sources: one
		c("ADD PRICING", ""),        // a hashless copy of that commit (case differs): that commit
		c("Something else", ""),     // hashless with no twin: kept, evidence is the subject
		c("Something else", ""),     // twice: one
		{Kind: KindPrompt, Root: root, Project: "acme", Title: "Fix the sidebar layout now"},
		{Kind: KindPrompt, Root: root, Project: "acme", Title: "Fix the sidebar layout now"},
		{Kind: KindPrompt, Root: "/work/other", Project: "other", Title: "Fix the sidebar layout now"}, // another project: separate
		{Kind: KindPullRequest, Root: root, Project: "acme", Title: "https://github.com/a/b/pull/1", Evidence: "https://github.com/a/b/pull/1"},
		{Kind: KindPullRequest, Root: root, Project: "acme", Title: "https://github.com/a/b/pull/1", Evidence: "https://github.com/a/b/pull/1"},
	}, testRed)
	got := texts(d.Bullets)
	want := []string{
		"Worked on acme: Fix the sidebar layout now",
		"Committed: Add pricing (acme 3f2a9c1)",
		"Committed: Something else (acme)",
		"Pull request: https://github.com/a/b/pull/1 (acme)",
		"Worked on other: Fix the sidebar layout now",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if ev := d.Bullets[2].Evidence; ev != "Something else" {
		t.Errorf("hashless evidence = %q", ev)
	}
	// a hashless commit in a different project is not the same commit
	d2 := Bullets([]ActivityItem{c("Add pricing", "3f2a9c1"), {Kind: KindCommit, Root: "/work/b", Project: "b", Title: "Add pricing"}}, testRed)
	if len(d2.Bullets) != 2 {
		t.Errorf("got %q", texts(d2.Bullets))
	}
}

func TestBulletsOrder(t *testing.T) {
	mk := func(root, name string, secs, commits int) []ActivityItem {
		var out []ActivityItem
		if secs > 0 {
			out = append(out, ActivityItem{Kind: KindSession, Root: root, Project: name, Count: 1, Seconds: secs})
		}
		for i := 0; i < commits; i++ {
			out = append(out, ActivityItem{Kind: KindCommit, Root: root, Project: name, Title: "c" + string(rune('a'+i)), Evidence: "abcdef" + string(rune('0'+i))})
		}
		return out
	}
	var items []ActivityItem
	items = append(items, mk("/w/zeta", "zeta", 0, 1)...)
	items = append(items, mk("/w/alpha", "Alpha", 0, 1)...) // same commits as zeta: name decides
	items = append(items, mk("/w/busy", "busy", 0, 3)...)   // more commits: first of the sessionless
	items = append(items, mk("/w/long", "long", 7200, 0)...)
	items = append(items, mk("/w/short", "short", 120, 5)...)
	var projects []string
	for _, b := range Bullets(items, testRed).Bullets {
		if len(projects) == 0 || projects[len(projects)-1] != b.Project {
			projects = append(projects, b.Project)
		}
	}
	if want := []string{"long", "short", "busy", "Alpha", "zeta"}; !reflect.DeepEqual(projects, want) {
		t.Errorf("order %v want %v", projects, want)
	}
	// commits inside a project go oldest first
	t0 := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	d := Bullets([]ActivityItem{
		{Kind: KindCommit, Root: "/w/a", Project: "a", Title: "later", Evidence: "bbbbbbb", Time: t0.Add(time.Hour)},
		{Kind: KindCommit, Root: "/w/a", Project: "a", Title: "earlier", Evidence: "aaaaaaa", Time: t0},
	}, testRed)
	if got := texts(d.Bullets); !reflect.DeepEqual(got, []string{"Committed: earlier (a aaaaaaa)", "Committed: later (a bbbbbbb)"}) {
		t.Errorf("got %q", got)
	}
}

func TestMarkdownAndAlreadyOnPage(t *testing.T) {
	bs := []SuggestedBullet{
		{Text: "Committed: fix `x` and *y* [z] <b> ~ | & # (acme abc1234)", Evidence: "acme abc1234"},
		{Text: "plain", Evidence: ""},
	}
	md := BulletsMarkdown(bs)
	if want := "- Committed: fix \\`x\\` and \\*y\\* \\[z\\] \\<b\\> \\~ \\| \\& \\# (acme abc1234)\n- plain"; md != want {
		t.Errorf("markdown:\n%q\n%q", md, want)
	}
	if got := EscapeMarkdown(`a\b_c`); got != `a\\b\_c` {
		t.Errorf("escape %q", got)
	}
	page := PlainText("# 2026-10-06\n\n## What I did\n\n" + md + "\n")
	if !strings.Contains(page, "fix `x` and *y* [z] <b> ~ | & # (acme abc1234)") {
		t.Errorf("plain text: %q", page)
	}
	if !IsAdded(bs[0], page) {
		t.Error("a bullet whose evidence is on the page is already added")
	}
	if IsAdded(SuggestedBullet{Evidence: "acme def5678"}, page) || IsAdded(bs[1], page) {
		t.Error("absent or empty evidence is not added")
	}
	if !IsAdded(SuggestedBullet{Evidence: "acme   abc1234"}, page) {
		t.Error("whitespace is collapsed on both sides")
	}
}
