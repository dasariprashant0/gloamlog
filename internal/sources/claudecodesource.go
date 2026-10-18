package sources

// What you did with Claude Code on a day, read from its local session files.
//
// READ-ONLY. It opens `<config>/projects/<project>/<session>.jsonl` for reading and nothing else: no writes, no network, no
// credentials, and no file outside those folders. The config folder is `~/.claude`, plus `$CLAUDE_CONFIG_DIR` when set.
//
// Privacy: a session file is read in full only when its project (the working folder recorded in it) is ticked. The first
// records of any other file are only asked for the working folder name, from the raw bytes, without decoding the record.
// Records of a ticked file whose own folder is not ticked are dropped before any text is taken from them.
//
// The format is Claude Code's private one and changes between versions, so the parser asks for almost nothing: a line is a
// JSON object with a `type`; user and assistant records need a `timestamp` and a `message`; everything else, and every field
// that is missing or has another type, is ignored. A line that is not JSON is counted, never fatal. Assumed shapes:
//
//	record  {type: "user"|"assistant", timestamp: "2026-10-06T09:12:03.120Z", sessionId, cwd, isSidechain, isMeta,
//	         isCompactSummary, uuid, message: {role, content: String | [block]}}
//	blocks  {type:"text", text} · {type:"tool_use", id, name, input} · {type:"tool_result", tool_use_id, content: String | [{text}]}
//	tools   Write / Edit / MultiEdit {input.file_path} · NotebookEdit {input.notebook_path} · Bash {input.command}
//	commit  a Bash command with `git commit`; its result holds "[branch 1a2b3c4] subject"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// ---- time -------------------------------------------------------------------------------------------------------------

// ParseClaudeTime reads "2026-10-06T09:12:03.120Z" into seconds since 1970 (a hand parser for the hot path, like Swift's),
// falling back to RFC 3339 with an offset.
func ParseClaudeTime(s string) (float64, bool) {
	if t, ok := fastTime(s); ok {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return float64(t.UnixNano()) / 1e9, true
	}
	return 0, false
}

func fastTime(s string) (float64, bool) {
	u := []byte(s)
	if len(u) < 20 || u[4] != '-' || u[7] != '-' || u[10] != 'T' || u[13] != ':' || u[16] != ':' {
		return 0, false
	}
	num := func(a, n int) (int, bool) {
		v := 0
		for k := a; k < a+n; k++ {
			if u[k] < '0' || u[k] > '9' {
				return 0, false
			}
			v = v*10 + int(u[k]-'0')
		}
		return v, true
	}
	y, ok1 := num(0, 4)
	mo, ok2 := num(5, 2)
	d, ok3 := num(8, 2)
	h, ok4 := num(11, 2)
	mi, ok5 := num(14, 2)
	se, ok6 := num(17, 2)
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6) || mo < 1 || mo > 12 || d < 1 || d > 31 || h >= 24 || mi >= 60 || se >= 61 {
		return 0, false
	}
	frac, idx := 0.0, 19
	if u[idx] == '.' {
		idx++
		scale := 0.1
		for idx < len(u) && u[idx] >= '0' && u[idx] <= '9' {
			frac += float64(u[idx]-'0') * scale
			scale /= 10
			idx++
		}
	}
	if idx >= len(u) || u[idx] != 'Z' { // must end in Z (UTC)
		return 0, false
	}
	return float64(daysFromCivil(y, mo, d)*86400+h*3600+mi*60+se) + frac, true
}

func daysFromCivil(y0, m, d int) int { // Hinnant's algorithm
	y := y0
	if m <= 2 {
		y = y0 - 1
	}
	era := y / 400
	if y < 0 {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mp := m - 3
	if m <= 2 {
		mp = m + 9
	}
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// ActiveSeconds: sort the record times; a gap longer than gap ends a period; add up the periods.
func ActiveSeconds(times []float64, gap float64) int {
	if len(times) == 0 {
		return 0
	}
	t := append([]float64(nil), times...)
	sort.Float64s(t)
	start, prev, total := t[0], t[0], 0.0
	for _, x := range t[1:] {
		if x-prev > gap {
			total += prev - start
			start = x
		}
		prev = x
	}
	return int(math.Round(total + prev - start))
}

// ---- text -------------------------------------------------------------------------------------------------------------

// Tag names whose content is system or command noise, never something the owner wrote.
var wrapperTags = []string{"system-reminder", "local-command-caveat", "local-command-stdout", "local-command-stderr", "command-name",
	"command-message", "command-args", "command-contents", "bash-input", "bash-stdout", "bash-stderr", "ide_opened_file", "ide_selection",
	"ide_diagnostics", "user-prompt-submit-hook", "task-notification", "function_results", "persisted-output"}

var (
	wrapperOpen    = regexp.MustCompile(`(?i)<(` + strings.Join(wrapperTags, "|") + `)\b[^>]*>`)
	wrapperClosers = func() map[string]*regexp.Regexp {
		m := map[string]*regexp.Regexp{}
		for _, t := range wrapperTags {
			m[t] = regexp.MustCompile(`(?i)</` + regexp.QuoteMeta(t) + `>`)
		}
		return m
	}()
	unclosedRe    = regexp.MustCompile(`(?i)<(?:system-reminder|local-command-caveat|ide_opened_file|ide_selection|user-prompt-submit-hook)\b[^>]*>[\s\S]*$`)
	placeholderRe = regexp.MustCompile(`(?i)\[(?:Pasted text|Pasted image|Image|Attachment)[^\]]*\]`)
	noisePrefixes = []string{"Caveat:", "[Request interrupted", "This session is being continued", "API Error", "Below is a conversation log from", "The app was quit while you were working"}
	gitCommitRe   = regexp.MustCompile(`\bgit\s+(?:(?:-C|-c)\s+\S+\s+|--[a-z-]+\s+)*commit(?:$|[^\p{L}\p{N}_-])`)
	heredocOpenRe = regexp.MustCompile(`<<-?\s*['"]?(\w+)['"]?[^\n]*\n`)
	messageArgRe  = regexp.MustCompile(`(?:^|\s)(?:-[A-Za-z]*m|--message)(?:\s+|=)(?:"((?:[^"\\]|\\.)*)"|'([^']*)')`)
	commitLineRe  = regexp.MustCompile(`(?m)^\[[^\]\n]+? ([0-9a-f]{7,40})\] (.+)$`)
	prCreateRe    = regexp.MustCompile(`\b(?:gh\s+pr\s+create|glab\s+mr\s+create)\b`)
	prLinkRe      = regexp.MustCompile(`https?://(?:github\.com/[\w.-]+/[\w.-]+/pull/\d+|gitlab\.com/[\w./-]+?/-/merge_requests/\d+|bitbucket\.org/[\w.-]+/[\w.-]+/pull-requests/\d+)`)
)

// stripWrapped removes `<tag ...>...</tag>` for the noise tags. ICU did this with a backreference (`</\1>`), which RE2 lacks:
// find an opening tag, look for its own closer, and when there is none move on from the next character.
func stripWrapped(s string) string {
	var b strings.Builder
	pos, last := 0, 0
	for pos < len(s) {
		m := wrapperOpen.FindStringSubmatchIndex(s[pos:])
		if m == nil {
			break
		}
		start, openEnd := pos+m[0], pos+m[1]
		tag := strings.ToLower(s[pos+m[2] : pos+m[3]])
		cm := wrapperClosers[tag].FindStringIndex(s[openEnd:])
		if cm == nil {
			pos = start + 1
			continue
		}
		b.WriteString(s[last:start])
		last = openEnd + cm[1]
		pos = last
	}
	b.WriteString(s[last:])
	return b.String()
}

func isWordRune(c rune) bool { return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) }

// heredocBody finds `<<DELIM ... \n body \n DELIM` (backreference in the ICU pattern) and returns the first match's body.
func heredocBody(cmd string) (string, bool) {
	pos := 0
	for pos < len(cmd) {
		m := heredocOpenRe.FindStringSubmatchIndex(cmd[pos:])
		if m == nil {
			return "", false
		}
		delim := cmd[pos+m[2] : pos+m[3]]
		bodyStart := pos + m[1]
		for k := bodyStart; k < len(cmd); k++ {
			if cmd[k] != '\n' {
				continue
			}
			j := k + 1
			for j < len(cmd) {
				c, n := firstRune(cmd[j:])
				if !unicode.IsSpace(c) {
					break
				}
				j += n
			}
			if strings.HasPrefix(cmd[j:], delim) {
				after := cmd[j+len(delim):]
				if c, _ := firstRune(after); after == "" || !isWordRune(c) {
					return cmd[bodyStart:k], true
				}
			}
		}
		pos += m[0] + 1
	}
	return "", false
}

func isNewline(c rune) bool {
	return c == '\n' || c == '\r' || c == '\u0085' || c == '\u2028' || c == '\u2029' || c == '\v' || c == '\f'
}

// firstNonEmptyLine is the first line that has something on it, trimmed.
func firstNonEmptyLine(s string) (string, bool) {
	for _, l := range strings.FieldsFunc(s, isNewline) {
		if t := trim(l); t != "" {
			return t, true
		}
	}
	return "", false
}

// PromptLine is the first line of something the owner typed, with system, command and tool wrappers removed; ok=false when
// there is none (a slash command, an interrupt marker, a compaction summary, "yes").
func PromptLine(raw string) (string, bool) {
	s := trim(placeholderRe.ReplaceAllString(unclosedRe.ReplaceAllString(stripWrapped(raw), ""), ""))
	if s == "" {
		return "", false
	}
	for _, p := range noisePrefixes {
		if strings.HasPrefix(s, p) {
			return "", false
		}
	}
	if strings.HasPrefix(s, "/") && !strings.Contains(s, " ") { // a bare slash command
		return "", false
	}
	first, ok := firstNonEmptyLine(s)
	if !ok {
		return "", false
	}
	line := collapse(first)
	if len(strings.Split(line, " ")) < 3 { // "ok", "yes do it": not worth a bullet
		return "", false
	}
	if r := []rune(line); len(r) > 400 {
		line = string(r[:400])
	}
	return line, true
}

// IsGitCommit: the command runs `git commit` (also `git -C dir commit`, `git --no-pager commit`).
func IsGitCommit(command string) bool { return gitCommitRe.MatchString(command) }

// CommitSubject is the commit message's first line, from `-m "..."`, `-m '...'`, `-am` or a heredoc; ok=false when the command
// does not carry one.
func CommitSubject(whole string) (string, bool) {
	// Only the text from `git commit` on: an earlier heredoc (a script piped into python, say) is not the message.
	command := whole
	if loc := gitCommitRe.FindStringIndex(whole); loc != nil {
		command = whole[loc[0]:]
	}
	firstLine := func(s string) (string, bool) {
		l, ok := firstNonEmptyLine(s)
		return collapse(l), ok
	}
	if body, ok := heredocBody(command); ok {
		if l, ok := firstLine(body); ok {
			return l, true
		}
	}
	m := messageArgRe.FindStringSubmatchIndex(command)
	if m == nil {
		return "", false
	}
	if m[2] >= 0 {
		raw := command[m[2]:m[3]]
		if strings.HasPrefix(raw, "$(") || strings.HasPrefix(raw, "`") { // a shell substitution we could not read
			return "", false
		}
		return firstLine(strings.ReplaceAll(strings.ReplaceAll(raw, `\"`, `"`), `\\`, `\`))
	}
	if m[4] >= 0 {
		return firstLine(command[m[4]:m[5]])
	}
	return "", false
}

// CommitResult reads "[feature/pricing 1a2b3c4] Add pricing table" from a command's output: the short hash and the subject.
func CommitResult(text string) (sha, subject string, ok bool) {
	m := commitLineRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	sha = m[1]
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return sha, collapse(m[2]), true
}

// IsPRCreate: the command is `gh pr create` or `glab mr create`.
func IsPRCreate(command string) bool { return prCreateRe.MatchString(command) }

// PullRequestURLs are the github / gitlab / bitbucket pull or merge request links in text, in order, without repeats.
func PullRequestURLs(text string) []string {
	if !strings.Contains(text, "/pull/") && !strings.Contains(text, "/merge_requests/") && !strings.Contains(text, "/pull-requests/") {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, u := range prLinkRe.FindAllString(text, -1) {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

func prefixRunes(s string, n int) string {
	if len(s) <= n { // fewer bytes than the limit means fewer characters too
		return s
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// resultText is a tool result's text: the string, or the `text` of its blocks (at most 40,000 characters).
func resultText(content any) string {
	if s, ok := content.(string); ok {
		return prefixRunes(s, 40_000)
	}
	blocks, ok := content.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if m, ok := b.(map[string]any); ok {
			if t, ok := m["text"].(string); ok {
				parts = append(parts, t)
			}
		}
	}
	return prefixRunes(strings.Join(parts, "\n"), 40_000)
}

// ---- one transcript ---------------------------------------------------------------------------------------------------

// editTools are the tools whose input names a file they changed. A table, so a renamed tool is a one-word fix.
var editTools = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true}

const (
	idleGap    = 15 * 60 // a pause longer than this ends a period of activity (15 minutes)
	maxPrompts = 3
)

type tickKey struct{ root, session string }
type promptRec struct {
	t                   float64
	root, session, text string
}
type editRec struct {
	t              float64
	root, path, id string
}
type commitRec struct {
	t                      float64
	root, id, subject, sha string
	failed                 bool
}
type prRec struct {
	t         float64
	root, url string
}

// fileDigest is what one session file holds for the ticked projects, across all days (the cache keeps it; a day is cut from it afterwards).
type fileDigest struct {
	ticks   map[tickKey][]float64 // the time of every record, per project and session: the active-time estimate
	prompts []promptRec
	edits   []editRec
	commits []commitRec
	prs     []prRec
	stats   ScanStats
}

var (
	userMark      = []byte(`"user"`)
	assistantMark = []byte(`"assistant"`)
)

// transcriptParser reads one transcript line by line; handle never panics.
type transcriptParser struct {
	policy          ProjectPolicy
	anchor          string // the ticked project this file started in: a `cd` into one of its folders stays in it ("" = none)
	fallbackSession string
	digest          fileDigest
	cwd             string
	seenTools       map[string]bool
	pendingCommit   map[string]int // tool_use id -> index in digest.commits, until its result arrives
	pendingPR       map[string]bool
}

func newTranscriptParser(policy ProjectPolicy, anchor, fallbackSession string) *transcriptParser {
	return &transcriptParser{policy: policy, anchor: anchor, fallbackSession: fallbackSession,
		digest:    fileDigest{ticks: map[tickKey][]float64{}},
		seenTools: map[string]bool{}, pendingCommit: map[string]int{}, pendingPR: map[string]bool{}}
}

func (p *transcriptParser) handle(line []byte) {
	p.digest.stats.Lines++
	// Cheap superset check: a user or assistant record always carries one of these words, whatever the spacing.
	if !bytes.Contains(line, userMark) && !bytes.Contains(line, assistantMark) {
		return
	}
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil || rec == nil {
		p.digest.stats.Skipped++
		return
	}
	typ, _ := rec["type"].(string)
	if typ != "user" && typ != "assistant" { // titles, snapshots, newer kinds
		return
	}
	p.digest.stats.Records++
	if c, ok := rec["cwd"].(string); ok && c != "" {
		p.cwd = c
	}
	ts, _ := rec["timestamp"].(string)
	t, okT := ParseClaudeTime(ts)
	msg, okM := rec["message"].(map[string]any)
	if !okT || !okM {
		return
	}
	p.digest.stats.Usable++
	// The gate: nothing is taken from a record whose project is not ticked.
	if p.cwd == "" {
		return
	}
	root, ok := p.policy.Root(p.cwd)
	if !ok {
		if p.anchor == "" || !PathContains(p.anchor, p.cwd) {
			return
		}
		root = p.anchor
	}
	session, _ := rec["sessionId"].(string)
	if session == "" {
		session = p.fallbackSession
	}
	k := tickKey{root, session}
	p.digest.ticks[k] = append(p.digest.ticks[k], t)
	if typ == "user" {
		p.user(rec, msg["content"], t, root, session)
	} else {
		p.assistant(msg["content"], t, root)
	}
}

func boolField(rec map[string]any, k string) bool { b, _ := rec[k].(bool); return b }

func (p *transcriptParser) user(rec map[string]any, content any, t float64, root, session string) {
	synthetic := boolField(rec, "isMeta") || boolField(rec, "isCompactSummary") || boolField(rec, "isVisibleInTranscriptOnly")
	sidechain := boolField(rec, "isSidechain") // a subagent's prompt is Claude's, not the owner's
	var texts []string
	if s, ok := content.(string); ok {
		texts = append(texts, s)
	} else if blocks, ok := content.([]any); ok {
		for _, b := range blocks {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			switch bt, _ := bm["type"].(string); bt {
			case "text":
				if s, ok := bm["text"].(string); ok {
					texts = append(texts, s)
				}
			case "tool_result":
				p.toolResult(bm, t, root)
			}
		}
	}
	const open, closeTag = "<bash-input>", "</bash-input>"
	for _, s := range texts {
		a, z := strings.Index(s, open), strings.Index(s, closeTag)
		if a >= 0 && z >= 0 && a+len(open) <= z { // a "!" command
			cmd := s[a+len(open) : z]
			if IsGitCommit(cmd) {
				subj, _ := CommitSubject(cmd)
				p.digest.commits = append(p.digest.commits, commitRec{t: t, root: root, subject: subj})
			}
			continue
		}
		if synthetic || sidechain {
			continue
		}
		if line, ok := PromptLine(s); ok {
			p.digest.prompts = append(p.digest.prompts, promptRec{t, root, session, line})
		}
	}
}

func (p *transcriptParser) assistant(content any, t float64, root string) {
	blocks, ok := content.([]any)
	if !ok {
		return
	}
	for _, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if bt, _ := bm["type"].(string); bt != "tool_use" {
			continue
		}
		name, ok := bm["name"].(string)
		if !ok {
			continue
		}
		id, _ := bm["id"].(string)
		if id != "" {
			if p.seenTools[id] { // a response streamed as several lines repeats its tool calls
				continue
			}
			p.seenTools[id] = true
		}
		input, _ := bm["input"].(map[string]any)
		if editTools[name] {
			path, ok := input["file_path"].(string)
			if !ok {
				path, _ = input["notebook_path"].(string)
			}
			if path != "" {
				p.digest.edits = append(p.digest.edits, editRec{t, root, RelativePath(path, root), id})
			}
		} else if name == "Bash" {
			cmd, ok := input["command"].(string)
			if !ok {
				continue
			}
			if IsGitCommit(cmd) {
				if id != "" {
					p.pendingCommit[id] = len(p.digest.commits)
				}
				subj, _ := CommitSubject(cmd)
				p.digest.commits = append(p.digest.commits, commitRec{t: t, root: root, id: id, subject: subj})
			}
			if id != "" && IsPRCreate(cmd) {
				p.pendingPR[id] = true
			}
		}
	}
}

// toolResult looks only at the results of a commit or `gh pr create` call; every other tool output is ignored unread.
func (p *transcriptParser) toolResult(b map[string]any, t float64, root string) {
	id, ok := b["tool_use_id"].(string)
	if !ok {
		return
	}
	ci, isCommit := p.pendingCommit[id]
	isPR := p.pendingPR[id]
	if !isCommit && !isPR {
		return
	}
	text := resultText(b["content"])
	if isCommit {
		delete(p.pendingCommit, id)
		if sha, subj, ok := CommitResult(text); ok {
			p.digest.commits[ci].sha = sha
			if subj != "" {
				p.digest.commits[ci].subject = subj
			}
		} else {
			p.digest.commits[ci].failed = true // the commit did not happen (hook failed, nothing to commit)
		}
	}
	if isPR {
		delete(p.pendingPR, id)
		for _, u := range PullRequestURLs(text) {
			p.digest.prs = append(p.digest.prs, prRec{t, root, u})
		}
	}
}

// ---- the source -------------------------------------------------------------------------------------------------------

// ClaudeCodeSource reads Claude Code's session files. Keep one around: it caches per file.
type ClaudeCodeSource struct {
	Roots []string // Claude config folders (each holds `projects/`)
	Home  string   // the home folder, for the name "Home"
	cache *fileCache
}

// NewClaudeCodeSource reads from roots (see DefaultClaudeRoots).
func NewClaudeCodeSource(roots []string, home string) *ClaudeCodeSource {
	return &ClaudeCodeSource{Roots: roots, Home: home, cache: &fileCache{m: map[string]cacheEntry{}}}
}

// DefaultClaudeRoots are `$CLAUDE_CONFIG_DIR` when set, and `<home>/.claude`. getenv is os.Getenv in production.
func DefaultClaudeRoots(getenv func(string) string, home string) []string {
	var paths []string
	if e := trim(getenv("CLAUDE_CONFIG_DIR")); e != "" {
		paths = append(paths, NormalizePath(e))
	}
	paths = append(paths, NormalizePath(home+"/.claude"))
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		if k := strings.ToLower(p); !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

type sessionFile struct {
	path  string
	mtime time.Time
	size  int64
}

// sessionFiles: every `*.jsonl` directly inside a project folder. Symbolic links are never followed, and the files Claude
// Code sets aside (`.orphaned-`, `.superseded-`) are ignored, or their copies would be counted twice.
func (s *ClaudeCodeSource) sessionFiles() []sessionFile {
	var out []sessionFile
	for _, root := range s.Roots {
		projects := filepath.Join(root, "projects")
		dirs, err := os.ReadDir(projects)
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if strings.HasPrefix(d.Name(), ".") || !d.IsDir() { // DirEntry.IsDir is false for a symlink
				continue
			}
			dir := filepath.Join(projects, d.Name())
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, f := range files {
				name := f.Name()
				if strings.HasPrefix(name, ".") || filepath.Ext(name) != ".jsonl" || !f.Type().IsRegular() {
					continue
				}
				if strings.Contains(name, ".orphaned-") || strings.Contains(name, ".superseded-") {
					continue
				}
				info, err := f.Info()
				if err != nil {
					continue
				}
				out = append(out, sessionFile{filepath.Join(dir, name), info.ModTime(), info.Size()})
			}
		}
	}
	return out
}

var cwdKeys = [][]byte{[]byte(`"cwd":"`), []byte(`"cwd": "`)}

// cwdIn is the working folder named by a line, read from its raw bytes: nothing else in the record is decoded.
func cwdIn(line []byte) (string, bool) {
	for _, key := range cwdKeys {
		i := bytes.Index(line, key)
		if i < 0 {
			continue
		}
		for j := i + len(key); j < len(line); j++ {
			switch line[j] {
			case '"':
				return string(line[i+len(key) : j]), true
			case '\\': // an escape in the path: let JSON decode that one
				var rec map[string]any
				if json.Unmarshal(line, &rec) != nil {
					return "", false
				}
				c, ok := rec["cwd"].(string)
				return c, ok
			}
		}
	}
	return "", false
}

// firstCwd is the working folder of the first record that names one, looking at no more than the first 512 KB.
func firstCwd(path string) string {
	found := ""
	_, _ = JSONLReader{Path: path}.ForEachLine(0, 512<<10, func(line []byte) bool {
		if c, ok := cwdIn(line); ok && c != "" {
			found = c
			return false
		}
		return true
	})
	return found
}

// Projects are the projects this source can see (names only), for the project list in Settings. Reads as little as it can.
func (s *ClaudeCodeSource) Projects(ctx context.Context, progress Progress) []SourceProject {
	byRoot := map[string]SourceProject{}
	groups := map[string][]sessionFile{}
	for _, f := range s.sessionFiles() {
		d := filepath.Dir(f.path)
		groups[d] = append(groups[d], f)
	}
	dirs := make([]string, 0, len(groups))
	for d := range groups {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for i, d := range dirs {
		if cancelled(ctx) {
			break
		}
		progress.report(i, len(dirs))
		newest := groups[d]
		sort.SliceStable(newest, func(a, b int) bool { return newest[a].mtime.After(newest[b].mtime) })
		for _, f := range newest[:min(4, len(newest))] {
			c := firstCwd(f.path)
			if c == "" {
				continue
			}
			root := CollapseWorktree(c)
			p, ok := byRoot[strings.ToLower(root)]
			if !ok {
				p = SourceProject{Root: root, Name: ProjectName(root, s.Home), Sources: []string{"claude"}}
			}
			if m := newest[0].mtime; m.After(p.LastActive) {
				p.LastActive = m
			}
			byRoot[strings.ToLower(root)] = p
			break
		}
	}
	out := make([]SourceProject, 0, len(byRoot))
	for _, p := range byRoot {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Root) < strings.ToLower(out[j].Root) })
	return out
}

// Activity is the Claude Code items for day ("yyyy-MM-dd" in cal's zone), only for projects policy allows.
func (s *ClaudeCodeSource) Activity(ctx context.Context, day string, cal core.Calendar, policy ProjectPolicy, progress Progress) SourceResult {
	var res SourceResult
	start, end, ok := dayBounds(day, cal)
	if policy.IsEmpty() || !ok {
		return res
	}
	lo, hi := float64(start.UnixNano())/1e9, float64(end.UnixNano())/1e9
	// A file's modified time is its newest record, so an older file cannot hold anything from this day.
	var files []sessionFile
	for _, f := range s.sessionFiles() {
		if !f.mtime.Before(start) {
			files = append(files, f)
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		if !files[i].mtime.Equal(files[j].mtime) {
			return files[i].mtime.Before(files[j].mtime)
		}
		return files[i].path < files[j].path
	})

	ms := map[string]map[string]map[int64]bool{} // project -> session -> record times (ms): copies of a record collapse
	firstPrompt := map[string]map[string]promptRec{}
	edits := map[string]map[string]int{}
	editSeen := map[string]bool{}
	commits := map[string][]commitRec{}
	commitSeen := map[string]bool{}
	prs := map[string][]string{}
	prSeen := map[string]bool{}
	unreadable := 0

	for i, f := range files {
		if cancelled(ctx) {
			break
		}
		progress.report(i, len(files))
		e, ok := s.entry(ctx, f, policy)
		if !ok {
			if !cancelled(ctx) {
				unreadable++
			}
			continue
		}
		d := e.digest
		if d == nil { // skipped: its project is not ticked
			continue
		}
		res.Stats.Add(d.stats)
		for k, ts := range d.ticks {
			for _, t := range ts {
				if t >= lo && t < hi {
					if ms[k.root] == nil {
						ms[k.root] = map[string]map[int64]bool{}
					}
					if ms[k.root][k.session] == nil {
						ms[k.root][k.session] = map[int64]bool{}
					}
					ms[k.root][k.session][int64(t*1000)] = true
				}
			}
		}
		for _, p := range d.prompts {
			if p.t >= lo && p.t < hi {
				if old, ok := firstPrompt[p.root][p.session]; ok && old.t <= p.t {
					continue
				}
				if firstPrompt[p.root] == nil {
					firstPrompt[p.root] = map[string]promptRec{}
				}
				firstPrompt[p.root][p.session] = p
			}
		}
		for _, ed := range d.edits {
			if ed.t >= lo && ed.t < hi && (ed.id == "" || !editSeen[ed.id]) {
				if ed.id != "" {
					editSeen[ed.id] = true
				}
				if edits[ed.root] == nil {
					edits[ed.root] = map[string]int{}
				}
				edits[ed.root][ed.path]++
			}
		}
		for _, c := range d.commits {
			if c.t >= lo && c.t < hi && !c.failed {
				key := c.id
				if key == "" {
					key = fmt.Sprintf("%s|%v", c.root, c.t)
				}
				if !commitSeen[key] {
					commitSeen[key] = true
					commits[c.root] = append(commits[c.root], c)
				}
			}
		}
		for _, p := range d.prs {
			if p.t >= lo && p.t < hi && !prSeen[p.url] {
				prSeen[p.url] = true
				prs[p.root] = append(prs[p.root], p.url)
			}
		}
	}

	roots := make([]string, 0, len(ms))
	for r := range ms {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	for _, root := range roots {
		sessions := ms[root]
		name := ProjectName(root, s.Home)
		var times []float64
		for _, set := range sessions {
			for v := range set {
				times = append(times, float64(v)/1000)
			}
		}
		first := lo
		if len(times) > 0 {
			first = times[0]
			for _, t := range times {
				first = math.Min(first, t)
			}
		}
		res.Items = append(res.Items, ActivityItem{Kind: KindSession, Root: root, Project: name, Title: "Claude Code",
			Time: unixTime(first), Count: len(sessions), Seconds: ActiveSeconds(times, idleGap)})

		var chosen []promptRec
		for _, p := range firstPrompt[root] {
			chosen = append(chosen, p)
		}
		sort.Slice(chosen, func(i, j int) bool {
			if chosen[i].t != chosen[j].t {
				return chosen[i].t < chosen[j].t
			}
			return chosen[i].session < chosen[j].session
		})
		for _, p := range chosen[:min(maxPrompts, len(chosen))] {
			res.Items = append(res.Items, ActivityItem{Kind: KindPrompt, Root: root, Project: name, Title: p.text, Time: unixTime(p.t), Count: 1})
		}

		type pathN struct {
			path string
			n    int
		}
		var ps []pathN
		for p, n := range edits[root] {
			ps = append(ps, pathN{p, n})
		}
		sort.Slice(ps, func(i, j int) bool {
			if ps[i].n != ps[j].n {
				return ps[i].n > ps[j].n
			}
			return ps[i].path < ps[j].path
		})
		for _, p := range ps[:min(60, len(ps))] {
			res.Items = append(res.Items, ActivityItem{Kind: KindFile, Root: root, Project: name, Title: p.path, Count: p.n})
		}

		cs := commits[root]
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].t < cs[j].t })
		for _, c := range cs[:min(12, len(cs))] {
			if c.subject == "" {
				continue
			}
			res.Items = append(res.Items, ActivityItem{Kind: KindCommit, Root: root, Project: name, Title: c.subject, Detail: c.sha,
				Time: unixTime(c.t), Evidence: c.sha, Count: 1})
		}
		for _, u := range prs[root][:min(5, len(prs[root]))] {
			res.Items = append(res.Items, ActivityItem{Kind: KindPullRequest, Root: root, Project: name, Title: u, Evidence: u, Count: 1})
		}
	}
	if unreadable > 0 && res.Stats.Files == 0 {
		res.Notices = append(res.Notices, "Gloamlog can't read Claude Code's files. Allow access in System Settings > Privacy & Security > Files & Folders.")
	}
	return res
}

func unixTime(sec float64) time.Time {
	whole := math.Floor(sec)
	return time.Unix(int64(whole), int64((sec-whole)*1e9))
}

// ---- cache (path + modified time + size + the ticked projects) --------------------------------------------------------

type cacheStamp struct {
	mtime  time.Time
	size   int64
	policy string
}

type cacheEntry struct {
	stamp  cacheStamp
	digest *fileDigest // nil = skipped
}

type fileCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}

func (s *ClaudeCodeSource) entry(ctx context.Context, f sessionFile, policy ProjectPolicy) (cacheEntry, bool) {
	stamp := cacheStamp{f.mtime, f.size, policy.Key()}
	s.cache.mu.Lock()
	hit, ok := s.cache.m[f.path]
	s.cache.mu.Unlock()
	if ok && hit.stamp == stamp {
		return hit, true
	}
	made := cacheEntry{stamp: stamp}
	first := firstCwd(f.path) // a file whose first working folder is not ticked is never read further
	if _, ticked := policy.Root(first); first != "" && !ticked {
		// skipped: nil digest
	} else {
		anchor := ""
		if first != "" {
			anchor, _ = policy.Root(first)
		}
		base := filepath.Base(f.path)
		parser := newTranscriptParser(policy, anchor, strings.TrimSuffix(base, filepath.Ext(base)))
		out, err := JSONLReader{Path: f.path}.ForEachLine(0, -1, func(line []byte) bool {
			parser.handle(line)
			return !cancelled(ctx)
		})
		if err != nil || cancelled(ctx) {
			return cacheEntry{}, false
		}
		parser.digest.stats.Skipped += out.SkippedLong
		parser.digest.stats.Files = 1
		made.digest = &parser.digest
	}
	s.cache.mu.Lock()
	if len(s.cache.m) > 600 {
		s.cache.m = map[string]cacheEntry{}
	}
	s.cache.m[f.path] = made
	s.cache.mu.Unlock()
	return made, true
}

// ClearCache forgets every cached file.
func (s *ClaudeCodeSource) ClearCache() {
	s.cache.mu.Lock()
	s.cache.m = map[string]cacheEntry{}
	s.cache.mu.Unlock()
}
