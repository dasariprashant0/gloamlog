package sources

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// Redactor hides secrets and personal values in text that is about to be shown or inserted.
//
// Transcripts are plain text and can hold anything a command printed, so every string that can reach the screen or a page goes
// through Apply (Bullets does it for all of them): API keys and tokens (ghp_ gho_ sk- AKIA ...), `Bearer` values, private key
// blocks, `password=` style assignments, emails and long hex or base64 strings become "[redacted]" / "[email]", and the home
// folder becomes "~". It is best effort, which is why only short first lines are ever read; it is idempotent (running it twice
// changes nothing) and leaves ordinary prose and 40-character git hashes alone.
//
// ponytail: Go's regexp (RE2) has ASCII-only \b and \w, ICU's are Unicode; a secret glued to a non-ASCII letter may be treated
// differently. Same 17 patterns otherwise.

// RedactKind is what the "N values hidden" line counts. Paths are shortened, not counted.
type RedactKind string

const (
	RedactToken RedactKind = "token"
	RedactEmail RedactKind = "email"
)

type redactRule struct {
	kind     RedactKind
	re       *regexp.Regexp
	template string
	accept   func(s string, m []int) bool
}

var (
	nameWords = `(?:secret|token|password|passwd|api[_-]?key|apikey|private[_-]?key|credentials?)`
	namePat   = `\b((?:[A-Za-z0-9]+[_-])*` + nameWords + `(?:[_-][A-Za-z0-9]+)*)`
	valuePat  = `("[^"\n]*"|'[^'\n]*'|[^\s"',;]+)`

	redactRules = buildRules()
	otherHomes  = regexp.MustCompile(`/(?:Users|home)/[^/\s"']+/`)
)

func buildRules() []redactRule {
	// ICU "(?i)" flag is [.caseInsensitive]; the others have none.
	ci := "(?i)"
	notAlready := func(s string, m []int) bool {
		if len(m) > 7 && m[6] >= 0 {
			return s[m[6]:m[7]] != "[redacted]"
		}
		return true
	}
	// A "name: value" pair is prose as often as a secret ("Fix Token: handle expiry"), so the value must look like one.
	looksSecret := func(s string, m []int) bool {
		v := strings.Trim(s[m[6]:m[7]], `"'`)
		if v == "[redacted]" {
			return false
		}
		return len([]rune(v)) >= 20 || strings.ContainsAny(v, "0123456789-_+/=@!$%^&*#")
	}
	r := func(kind RedactKind, pattern, template string, accept func(string, []int) bool) redactRule {
		return redactRule{kind: kind, re: regexp.MustCompile(pattern), template: template, accept: accept}
	}
	return []redactRule{
		r(RedactToken, `-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`, "[redacted]", nil),
		r(RedactToken, `-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*`, "[redacted]", nil),                  // header only, the body was cut off
		r(RedactToken, `([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@`, "${1}[redacted]@", nil),          // scheme://user:password@host
		r(RedactToken, `\b(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA|AIPA)[A-Z0-9]{16}\b`, "[redacted]", nil),   // AWS access key ids
		r(RedactToken, `\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`, "[redacted]", nil), // GitHub
		r(RedactToken, `\bxox[abprs]-[A-Za-z0-9-]{10,}`, "[redacted]", nil),                                // Slack
		r(RedactToken, `\bsk-[A-Za-z0-9_-]{16,}`, "[redacted]", nil),                                       // sk-, sk-ant-, sk-proj-
		r(RedactToken, `\b[sr]k_(?:live|test)_[A-Za-z0-9]{10,}`, "[redacted]", nil),                        // Stripe
		r(RedactToken, `\bAIza[0-9A-Za-z_-]{30,}`, "[redacted]", nil),                                      // Google
		r(RedactToken, `\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`, "[redacted]", nil),  // JWT
		r(RedactToken, ci+`\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{16,}`, "${1} [redacted]", nil),
		r(RedactToken, ci+namePat+`(\s*=\s*)`+valuePat, "${1}${2}[redacted]", func(s string, m []int) bool { // PASSWORD=..., api_key = "..."
			return s[m[6]:m[7]] != "[redacted]"
		}),
		r(RedactToken, ci+namePat+`(\s*:\s*)`+valuePat, "${1}${2}[redacted]", looksSecret),
		r(RedactToken, ci+`(--?(?:password|passwd|token|secret|api[_-]?key))(\s+|=)([^\s"']+)`, "${1}${2}[redacted]", notAlready),
		r(RedactEmail, `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`, "[email]", nil),
		r(RedactToken, `\b[0-9A-Fa-f]{32,}\b`, "[redacted]", func(s string, m []int) bool { return m[1]-m[0] != 40 }), // long hex; a 40-character git hash stays
		r(RedactToken, `[A-Za-z0-9+/_-]{32,}={0,2}`, "[redacted]", func(s string, m []int) bool { // long base64-ish
			t := s[m[0]:m[1]]
			var up, low, dig bool
			for _, c := range t {
				up = up || unicode.IsUpper(c)
				low = low || unicode.IsLower(c)
				dig = dig || unicode.IsNumber(c)
			}
			return up && low && dig && strings.Count(t, "/") <= 1
		}),
	}
}

// Redactor is the 17 rules plus the home folder to fold into "~".
type Redactor struct {
	home string
}

// RuleCount is the number of regex rules (17).
func (Redactor) RuleCount() int { return len(redactRules) }

// NewRedactor folds home into "~" (a home of "" or "/" is not folded).
func NewRedactor(home string) Redactor { return Redactor{home: home} }

// StandardRedactor uses the real home folder, like Swift's Redactor.standard.
func StandardRedactor() Redactor {
	h, _ := os.UserHomeDir()
	return NewRedactor(h)
}

// Apply returns s with every secret and personal value hidden, and how many of each kind were hidden.
func (r Redactor) Apply(s string) (string, map[RedactKind]int) {
	out := s
	hidden := map[RedactKind]int{}
	for _, rule := range redactRules {
		ms := rule.re.FindAllStringSubmatchIndex(out, -1)
		if len(ms) == 0 {
			continue
		}
		var b strings.Builder
		last, n := 0, 0
		for _, m := range ms {
			if rule.accept != nil && !rule.accept(out, m) {
				continue
			}
			b.WriteString(out[last:m[0]])
			b.Write(rule.re.ExpandString(nil, rule.template, out, m))
			last = m[1]
			n++
		}
		if n > 0 {
			b.WriteString(out[last:])
			out = b.String()
			hidden[rule.kind] += n
		}
	}
	return r.ShortenPaths(out), hidden
}

// Display is for text that is only displayed (paths and names in Settings): the same hiding, without the counts.
func (r Redactor) Display(s string) string { t, _ := r.Apply(s); return t }

// ShortenPaths: the home folder becomes `~`; any other `/Users/<name>/` or `/home/<name>/` becomes `~/`.
func (r Redactor) ShortenPaths(s string) string {
	out := s
	if len([]rune(r.home)) > 1 {
		out = foldHome(out, r.home)
	}
	return otherHomes.ReplaceAllLiteralString(out, "~/")
}

// foldHome replaces home where it ends the text or is followed by whitespace or one of " ' ) , : ; / (ICU used a lookahead).
func foldHome(s, home string) string {
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(s[i:], home)
		if j < 0 {
			break
		}
		j += i
		end := j + len(home)
		ok := end == len(s)
		if !ok {
			c, _ := firstRune(s[end:])
			ok = unicode.IsSpace(c) || strings.ContainsRune(`"'),:;/`, c)
		}
		if ok {
			b.WriteString(s[i:j])
			b.WriteByte('~')
			i = end
		} else {
			b.WriteString(s[i : j+1])
			i = j + 1
		}
	}
	b.WriteString(s[i:])
	return b.String()
}

func firstRune(s string) (rune, int) {
	for _, c := range s {
		return c, len(string(c))
	}
	return 0, 0
}

// RedactSummary is "4 values hidden (2 emails, 2 tokens)", or "" when nothing was hidden.
func RedactSummary(hidden map[RedactKind]int) string {
	n := 0
	for _, v := range hidden {
		n += v
	}
	if n == 0 {
		return ""
	}
	plural := func(c int, w string) string {
		if c == 1 {
			return fmt.Sprintf("%d %s", c, w)
		}
		return fmt.Sprintf("%d %ss", c, w)
	}
	var parts []string
	if e := hidden[RedactEmail]; e > 0 {
		parts = append(parts, plural(e, "email"))
	}
	if t := hidden[RedactToken]; t > 0 {
		parts = append(parts, plural(t, "token"))
	}
	val := "values"
	if n == 1 {
		val = "value"
	}
	return fmt.Sprintf("%d %s hidden (%s)", n, val, strings.Join(parts, ", "))
}
