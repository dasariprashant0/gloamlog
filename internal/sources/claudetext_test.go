package sources

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPromptLine(t *testing.T) {
	long := strings.Repeat("word ", 120)
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Add a pricing table to the landing page", "Add a pricing table to the landing page", true},
		{"  first line here ok\nsecond line", "first line here ok", true},
		{"\n\n  skip blank lines first\n", "skip blank lines first", true},
		{"<system-reminder>be nice</system-reminder>Fix the sidebar layout now", "Fix the sidebar layout now", true},
		{"<SYSTEM-REMINDER>x</system-reminder>Fix the sidebar layout now", "Fix the sidebar layout now", true},
		{"<command-name>/model</command-name><command-args>opus</command-args>", "", false},
		{"<local-command-stdout>ok</local-command-stdout>", "", false},
		{"<ide_opened_file>a.go</ide_opened_file>Look at this function please", "Look at this function please", true},
		{"unclosed <system-reminder> never ends and runs to the end of the text", "unclosed", false}, // cut, then under 3 words
		{"Refactor this [Pasted text #1 +20 lines] into helpers", "Refactor this into helpers", true},
		{"[Image #1] what is wrong here", "what is wrong here", true},
		{"Caveat: the messages below were generated", "", false},
		{"[Request interrupted by user]", "", false},
		{"This session is being continued from a previous conversation", "", false},
		{"API Error: 500 something went wrong here", "", false},
		{"Below is a conversation log from a Claude Code coding session.", "", false},
		{"/clear", "", false},
		{"/review", "", false},
		{"/review the last three commits", "/review the last three commits", true}, // not bare: has a space
		{"yes do", "", false}, // under 3 words; "yes do it" would pass, as in Swift
		{"ok", "", false},
		{"", "", false},
		{"   \n  ", "", false},
		{long, strings.Repeat("word ", 80), true}, // cut to exactly 400 characters (no trimming, like Swift)
	}
	for _, c := range cases {
		got, ok := PromptLine(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("PromptLine(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
	if got, _ := PromptLine(long); len([]rune(got)) != 400 {
		t.Errorf("long prompt is %d runes, want 400", len([]rune(got)))
	}
	// an unterminated wrapper in the middle drops the rest, keeps the lead
	if got, ok := PromptLine("Please fix the build <system-reminder>secret instructions"); !ok || got != "Please fix the build" {
		t.Errorf("unclosed wrapper: %q %v", got, ok)
	}
}

func TestIsGitCommit(t *testing.T) {
	yes := []string{
		`git commit -m "x"`, `git add -A && git commit -am "x"`, `git -C /repo commit -m x`, `git -c user.name=a commit -m x`,
		`git --no-pager commit -m x`, `cd a; git commit`, `git   commit`, "git commit\n",
	}
	no := []string{`git commit-tree abc`, `git status`, `echo git committed`, `git log --oneline`, `gitcommit`, `git commit-msg`, `git push origin commit`}
	for _, c := range yes {
		if !IsGitCommit(c) {
			t.Errorf("should be a commit: %q", c)
		}
	}
	for _, c := range no {
		if IsGitCommit(c) {
			t.Errorf("should not be a commit: %q", c)
		}
	}
}

func TestCommitSubject(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{`git commit -m "fix: thing"`, "fix: thing", true},
		{`git commit -am 'feat: single quoted'`, "feat: single quoted", true},
		{`git commit --message "long form"`, "long form", true},
		{`git commit --message="eq form"`, "eq form", true},
		{`git commit -m "line one" -m "line two"`, "line one", true},
		{`git commit -m "say \"hi\" now"`, `say "hi" now`, true},
		{"git commit -m \"multi\nline body\"", "multi", true},
		{"git commit -F - <<'EOF'\nfeat: from heredoc\n\nbody\nEOF", "feat: from heredoc", true},
		{"git commit -m \"$(cat <<'EOF'\nfeat: hd in subshell\n\nbody\nEOF\n)\"", "feat: hd in subshell", true},
		{`git commit -m "$(cat msg.txt)"`, "", false},
		{"git commit -m \"`date`\"", "", false},
		{`git commit`, "", false},
		{`git commit -m ""`, "", false},
		// an earlier heredoc (a script) is not the message
		{"python3 - <<'EOF'\np=\"CHANGELOG.md\"\nEOF\ngit add -A && git commit -q -m \"fix: real subject\" && git push", "fix: real subject", true},
		{"git commit -F - <<-EOT\n\tindented end\n\tEOT\n", "indented end", true},
	}
	for _, c := range cases {
		got, ok := CommitSubject(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("CommitSubject(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestCommitResult(t *testing.T) {
	sha, subj, ok := CommitResult("[main 1a2b3c4] Add   pricing table\n 2 files changed")
	if !ok || sha != "1a2b3c4" || subj != "Add pricing table" {
		t.Errorf("got %q %q %v", sha, subj, ok)
	}
	sha, subj, ok = CommitResult("hook output\n[feature/pricing abcdef0123456789abcdef0123456789abcdef01] Long hash\n")
	if !ok || sha != "abcdef0" || subj != "Long hash" {
		t.Errorf("long hash: %q %q %v", sha, subj, ok)
	}
	sha, _, ok = CommitResult("[detached HEAD 1a2b3c4] Root commit")
	if !ok || sha != "1a2b3c4" {
		t.Errorf("branch names with a space: %q %v", sha, ok)
	}
	for _, bad := range []string{"nothing to commit, working tree clean", "error: pre-commit hook failed", "[main abc] too short", ""} {
		if _, _, ok := CommitResult(bad); ok {
			t.Errorf("should not read a commit from %q", bad)
		}
	}
}

func TestPullRequests(t *testing.T) {
	if !IsPRCreate("gh pr create --title x") || !IsPRCreate("glab mr create -f") || IsPRCreate("gh pr view 4") || IsPRCreate("gh pr list") {
		t.Error("IsPRCreate")
	}
	text := "Creating pull request\nhttps://github.com/acme/site/pull/42\nand https://gitlab.com/g/sub/proj/-/merge_requests/7 " +
		"and https://bitbucket.org/w/r/pull-requests/3 and https://github.com/acme/site/pull/42 again and https://github.com/acme/site/issues/9 " +
		"and https://example.com/a/b/pull/1"
	want := []string{"https://github.com/acme/site/pull/42", "https://gitlab.com/g/sub/proj/-/merge_requests/7", "https://bitbucket.org/w/r/pull-requests/3"}
	if got := PullRequestURLs(text); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	if got := PullRequestURLs("no links https://github.com/a/b"); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestClaudeTime(t *testing.T) {
	a, ok := ParseClaudeTime("2026-10-06T09:12:03.120Z")
	b, _ := ParseClaudeTime("2026-10-06T09:12:03Z")
	c, ok3 := ParseClaudeTime("2026-10-06T10:12:03+01:00")
	if !ok || !ok3 || a-b < 0.119 || a-b > 0.121 || b != c {
		t.Errorf("times: %v %v %v", a, b, c)
	}
	if want := float64(time.Date(2026, 10, 6, 9, 12, 3, 0, time.UTC).Unix()); b != want {
		t.Errorf("epoch = %v, want %v", b, want)
	}
	for _, bad := range []string{"", "yesterday", "2026-13-06T09:12:03Z", "2026-10-06 09:12:03Z"} {
		if _, ok := ParseClaudeTime(bad); ok {
			t.Errorf("should not parse %q", bad)
		}
	}
	// gaps over 15 minutes end a period
	if got := ActiveSeconds([]float64{0, 60, 120, 120 + 901, 120 + 901 + 30}, 900); got != 120+30 {
		t.Errorf("active = %d", got)
	}
	if ActiveSeconds(nil, 900) != 0 || ActiveSeconds([]float64{5}, 900) != 0 {
		t.Error("empty and single")
	}
}

func TestResultText(t *testing.T) {
	if resultText("abc") != "abc" || resultText(42) != "" {
		t.Error("string / other")
	}
	blocks := []any{map[string]any{"type": "text", "text": "a"}, "junk", map[string]any{"text": "b"}, map[string]any{"type": "image"}}
	if got := resultText(blocks); got != "a\nb" {
		t.Errorf("got %q", got)
	}
	if got := resultText(strings.Repeat("é", 50_000)); len([]rune(got)) != 40_000 {
		t.Errorf("cap = %d", len([]rune(got)))
	}
}

func TestJSONLReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jsonl")
	long := strings.Repeat("x", 100)
	content := "one\n" + "\n" + "two-two\n" + long + "\n" + "after-long\n" + "straddles-a-chunk-boundary-line\n" + "last-no-newline"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []int{7, 16, 64, 4 << 20} {
		var got []string
		out, err := JSONLReader{Path: path, MaxLine: 50, ChunkSize: chunk}.ForEachLine(0, -1, func(l []byte) bool { got = append(got, string(l)); return true })
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"one", "two-two", "after-long", "straddles-a-chunk-boundary-line"}
		if !reflect.DeepEqual(got, want) || out.SkippedLong != 1 || out.EndOffset != int64(len(content)-len("last-no-newline")) {
			t.Errorf("chunk %d: lines %v skipped %d end %d", chunk, got, out.SkippedLong, out.EndOffset)
		}
	}
	// a limit leaves the line it cuts unread, and stopping early reports where it stopped
	var got []string
	out, _ := JSONLReader{Path: path}.ForEachLine(0, 10, func(l []byte) bool { got = append(got, string(l)); return true })
	if !reflect.DeepEqual(got, []string{"one"}) || out.EndOffset != 5 { // the blank line at 4 is complete; "two-tw" is cut by the limit
		t.Errorf("limit: %v %d", got, out.EndOffset)
	}
	got = nil
	out, _ = JSONLReader{Path: path}.ForEachLine(0, -1, func(l []byte) bool { got = append(got, string(l)); return false })
	if !reflect.DeepEqual(got, []string{"one"}) || out.EndOffset != 4 {
		t.Errorf("stop: %v %d", got, out.EndOffset)
	}
	// resume from an offset
	got = nil
	_, _ = JSONLReader{Path: path}.ForEachLine(4, -1, func(l []byte) bool { got = append(got, string(l)); return true })
	if len(got) == 0 || got[0] != "two-two" {
		t.Errorf("offset: %v", got)
	}
	if _, err := (JSONLReader{Path: filepath.Join(dir, "missing")}).ForEachLine(0, -1, nil); err == nil {
		t.Error("a missing file is an error")
	}
}
