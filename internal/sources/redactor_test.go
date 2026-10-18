package sources

import (
	"strings"
	"testing"
)

func TestRedactorRules(t *testing.T) {
	r := NewRedactor("/Users/me")
	long40 := "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name, in, want string
		kind           RedactKind
	}{
		{"1 private key block", "key -----BEGIN RSA PRIVATE KEY-----\nabc\ndef\n-----END RSA PRIVATE KEY----- tail", "key [redacted] tail", RedactToken},
		{"2 private key header only", "x -----BEGIN PRIVATE KEY-----\nabcdef", "x [redacted]", RedactToken},
		{"3 scheme user pass", "clone https://bob:hunter2@example.com/x.git", "clone https://[redacted]@example.com/x.git", RedactToken},
		{"4 aws id", "id AKIAABCDEFGHIJKLMNOP end", "id [redacted] end", RedactToken},
		{"5a github ghp", "t ghp_abcdefghijklmnopqrstuvwx end", "t [redacted] end", RedactToken},
		{"5b github pat", "t github_pat_abcdefghijklmnopqrstuv end", "t [redacted] end", RedactToken},
		{"6 slack", "t xoxb-1234567890-abcdef end", "t [redacted] end", RedactToken},
		{"7 sk-", "t sk-ant-api03-abcdefghijkl end", "t [redacted] end", RedactToken},
		{"8 stripe", "t sk_live_abcdefghij12 end", "t [redacted] end", RedactToken},
		{"9 google", "t AIza" + strings.Repeat("a", 35) + " end", "t [redacted] end", RedactToken},
		{"10 jwt", "t eyJhbGciOiJI.eyJzdWIiOiIx.abcdefghij end", "t [redacted] end", RedactToken},
		{"11a bearer", "Authorization: Bearer abcdefghijklmnop1234", "Authorization: Bearer [redacted]", RedactToken},
		{"11b basic any case", "authorization: basic dXNlcjpwYXNzd29yZDEyMw==", "authorization: basic [redacted]", RedactToken},
		{"12a NAME=value", "PASSWORD=hunter2 next", "PASSWORD=[redacted] next", RedactToken},
		{"12b quoted value", `api_key = "abc def" next`, "api_key = [redacted] next", RedactToken},
		{"13a name: value that looks secret", "token: abc123 next", "token: [redacted] next", RedactToken},
		{"13b prose is left alone", "Fix Token: handle expiry", "Fix Token: handle expiry", ""},
		{"14a password flag", "run --password hunter2 now", "run --password [redacted] now", RedactToken},
		{"14b token flag with =", "run --token=abc now", "run --token=[redacted] now", RedactToken},
		{"15 email", "mail me@example.com now", "mail [email] now", RedactEmail},
		{"16a long hex", "h abcdef0123456789abcdef0123456789 end", "h [redacted] end", RedactToken},
		{"16b 40-char git hash stays", "commit " + long40, "commit " + long40, ""},
		{"17a long base64-ish", "b Zm9vYmFyQmF6MTIzNDU2Nzg5MEFiQ2RFZgxx end", "b [redacted] end", RedactToken},
		{"17b a long word with no digit stays", "w supercalifragilisticexpialidociousandmore end", "w supercalifragilisticexpialidociousandmore end", ""},
		{"17c a path stays", "p src/components/ui/buttons/PrimaryButton1/index.tsx", "p src/components/ui/buttons/PrimaryButton1/index.tsx", ""},
	}
	for _, c := range cases {
		got, hidden := r.Apply(c.in)
		if got != c.want {
			t.Errorf("%s:\n in  %q\n got %q\n want %q", c.name, c.in, got, c.want)
		}
		total := 0
		for _, n := range hidden {
			total += n
		}
		if c.kind == "" && total != 0 {
			t.Errorf("%s: nothing should be counted, got %v", c.name, hidden)
		}
		if c.kind != "" && hidden[c.kind] != 1 {
			t.Errorf("%s: want one %s counted, got %v", c.name, c.kind, hidden)
		}
		// idempotent: a second pass changes nothing and counts nothing
		again, h2 := r.Apply(got)
		if again != got || len(h2) != 0 && (h2[RedactToken]+h2[RedactEmail] != 0) {
			t.Errorf("%s: not idempotent: %q -> %q (%v)", c.name, got, again, h2)
		}
	}
	if n := r.RuleCount(); n != 17 {
		t.Errorf("rule count = %d, want 17", n)
	}
}

func TestRedactorHomeFolding(t *testing.T) {
	r := NewRedactor("/Users/me")
	cases := map[string]string{
		"/Users/me/proj/a.go":         "~/proj/a.go",
		"open /Users/me now":          "open ~ now",
		"/Users/me":                   "~",
		`"/Users/me"`:                 `"~"`,
		"(/Users/me)":                 "(~)",
		"/Users/mex/y":                "~/y", // not the home folder, but still somebody's
		"/Users/other/file.txt":       "~/file.txt",
		"/home/bob/a":                 "~/a",
		"cd /Users/me/a && /Users/me": "cd ~/a && ~",
	}
	for in, want := range cases {
		if got := r.ShortenPaths(in); got != want {
			t.Errorf("ShortenPaths(%q) = %q, want %q", in, got, want)
		}
	}
	if got, _ := NewRedactor("").Apply("/Users/me/x"); got != "~/x" {
		t.Errorf("without a home the other-homes rule still applies, got %q", got)
	}
}

func TestRedactorIdempotentOnMixedText(t *testing.T) {
	r := NewRedactor("/Users/me")
	in := "mail me@x.com token=abc123 /Users/me/a Bearer abcdefghijklmnop1234 sk-abcdefghijklmnopqr https://u:p@h/x --password=zz Zm9vYmFyQmF6MTIzNDU2Nzg5MEFiQ2RFZgxx"
	once, hidden := r.Apply(in)
	twice, _ := r.Apply(once)
	if once != twice {
		t.Errorf("not idempotent:\n%q\n%q", once, twice)
	}
	if strings.Contains(once, "hunter") || strings.Contains(once, "me@x.com") || strings.Contains(once, "sk-abc") {
		t.Errorf("a secret survived: %q", once)
	}
	if hidden[RedactEmail] != 1 || hidden[RedactToken] < 5 {
		t.Errorf("counts: %v", hidden)
	}
}

func TestRedactSummary(t *testing.T) {
	if got := RedactSummary(map[RedactKind]int{RedactEmail: 2, RedactToken: 2}); got != "4 values hidden (2 emails, 2 tokens)" {
		t.Errorf("got %q", got)
	}
	if got := RedactSummary(map[RedactKind]int{RedactToken: 1}); got != "1 value hidden (1 token)" {
		t.Errorf("got %q", got)
	}
	if got := RedactSummary(map[RedactKind]int{RedactEmail: 1}); got != "1 value hidden (1 email)" {
		t.Errorf("got %q", got)
	}
	if got := RedactSummary(nil); got != "" {
		t.Errorf("got %q", got)
	}
}
