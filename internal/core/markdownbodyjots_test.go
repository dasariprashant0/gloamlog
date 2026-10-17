package core

// Ported from tests/main.swift: "K1 Jots rules in the words rule" (BodyCounts, LoggedWords, IsJotsOnly, StripUntouchedTemplate),
// plus a test of the Jots rules inside DayDocument.Words/Status that the Swift suite did not have.

import "testing"

func TestJotsCountsSplitThePageAtTheJotsBlock(t *testing.T) {
	cases := []struct {
		body    string
		heading string
		want    BodyCountsResult
		why     string
	}{
		{"", "Jots", BodyCountsResult{0, 0, 0}, ""},
		{lorem(5, "w"), "Jots", BodyCountsResult{5, 0, 0}, "no Jots block: everything is words"},
		{"intro words here\n\n## Jots\n\n- 14:32 call Sam\n- 14:40 email Ana now", "Jots", BodyCountsResult{3, 5, 2}, ""},
		{"## Jots\n- 14:32 meet at 15:00", "Jots", BodyCountsResult{0, 3, 1}, "only a LEADING time is dropped"},
		{"## Jots\n- [ ] call Sam\n- plain note here", "Jots", BodyCountsResult{0, 5, 2}, "a to-do and a note without a time"},
		{"## Jots\n- 9:05 a\n- 23:59 b\n- 24:00 c\n- 12:75 d\n- 7:5 e", "Jots", BodyCountsResult{0, 1 + 1 + 2 + 2 + 2, 5}, "only a real HH:mm is dropped"},
		{"## Jots\n- 14:32\n- 14:33 ", "Jots", BodyCountsResult{0, 0, 2}, "a bare time is a jot with no words"},
		{"## Jots\n14:32 typed by hand\n- 14:33 x", "Jots", BodyCountsResult{0, 4 + 1, 1}, "a time that starts a paragraph is the user's own text"},
		{"## Jots\n- 14:32 a b\n  c d\n  - nested e\n- 14:33 f", "Jots", BodyCountsResult{0, 7, 2}, "continuation and nested lines count as words, nested items are not jots"},
		{"## Jots\n- a b\n## Finished\nfour words right here", "Jots", BodyCountsResult{4, 2, 1}, "the next heading of the same level ends the block"},
		{"## Notes\nn1 n2\n### Jots\n- j1 j2\n## Other\no1", "Jots", BodyCountsResult{3, 2, 1}, "### Jots under ## Notes"},
		{"## Jots\n- a\n### sub\nmore words", "Jots", BodyCountsResult{0, 3, 1}, "a deeper heading stays inside the block"},
		{"# Jots\n- a\n## sub\nmore\n# Next\nx y", "Jots", BodyCountsResult{2, 2, 1}, ""},
		{"## Carried over from Mon\n- [ ] x y\n## Jots\n- j", "Jots", BodyCountsResult{0, 1, 1}, "carried tasks still never count"},
		{"## Jots\n- a\n```\ncode here\n```", "Jots", BodyCountsResult{0, 3, 1}, "code words count, fence lines do not"},
		{"## Jots\n<!-- hidden words -->\n- a", "Jots", BodyCountsResult{0, 1, 1}, "comments are not words"},
		{"## Jots\n- a\n## X\nw\n## Jots\n- b c", "Jots", BodyCountsResult{1, 3, 2}, "every Jots block counts as jots"},
		{"## JOTS!\n- a", "Jots", BodyCountsResult{0, 1, 1}, ""},
		{"## 📝 Jots\n- a", "Jots", BodyCountsResult{0, 1, 1}, ""},
		{"## Jotsy\n- a b", "Jots", BodyCountsResult{2, 0, 0}, "a similar heading is not a Jots heading"},
		{"##\n- a b", "Jots", BodyCountsResult{2, 0, 0}, "an empty heading is not a Jots heading"},
		{"## Notes\n- a b", "Notes", BodyCountsResult{0, 2, 1}, "configured heading"},
		{"## Jots\n- a b", "", BodyCountsResult{0, 2, 1}, "an unusable heading falls back to Jots"},
	}
	for _, c := range cases {
		got := BodyCounts(c.body, c.heading)
		if got != c.want {
			t.Errorf("BodyCounts(%q, %q) = %+v, want %+v (%s)", c.body, c.heading, got, c.want, c.why)
		}
	}
	expect(t, BodyCounts("```\n## Jots\n- a\n```", "Jots").JotCount == 0, "a heading inside a fence is no heading")
	// a page without a Jots block counts exactly like CountWords
	for _, b := range []string{DefaultTemplate, "## A\ntext here\n## Carried over from Mon\n- [ ] x\n- y", "![i](a.png) word", "```\ncode\n```", lorem(30, "w")} {
		expect(t, BodyCounts(b, "Jots") == BodyCountsResult{Words: W(b)}, "%q", b)
	}
}

func TestJotsLoggedWords(t *testing.T) {
	body := "intro words here\n\n## Jots\n\n- 14:32 call Sam\n- 14:40 email Ana now"
	expect(t, LoggedWords(body, "Jots", false) == 3, "jots not counted")
	expect(t, LoggedWords(body, "Jots", true) == 8, "the two times are still not words")
	expect(t, LoggedWords(body, "Notes", false) == 10, "no such block: everything counts")
	expect(t, LoggedWords(lorem(7, "w"), "Jots", false) == 7 && LoggedWords("", "Jots", false) == 0, "plain pages")
	expect(t, LoggedWords("## Jots\n- a b c", "Jots", false) == 0 && LoggedWords("## Jots\n- a b c", "Jots", true) == 3, "jots only")
}

func TestIsJotsOnly(t *testing.T) {
	expect(t, !IsJotsOnly("", "Jots") && !IsJotsOnly("\n \n", "Jots"), "an empty page is not jots-only")
	for _, b := range []string{"## Jots\n\n- 14:32 a", "\n\n## Jots\n- a\n\n", "## Jots", "### Jots\n- a", "# Jots\n- a\n## Sub\nx", "## Jots\n- a\n### sub\nx",
		"## Jots\n- a\n## Jots\n- b", "<!-- c -->\n## Jots\n- a", "## 📝 jots\n- a", "  \n## Jots\n- 14:32 a\n- 14:40 b\n"} {
		expect(t, IsJotsOnly(b, "Jots"), "%q should be jots-only", b)
	}
	for _, b := range []string{"text\n## Jots\n- a", "## Jots\n- a\n## Other\nx", DefaultTemplate + "\n\n## Jots\n- a", "- a", "```\n## Jots\n```", "## Jotsy\n- a", "## What I did\n\n## Jots\n- a"} {
		expect(t, !IsJotsOnly(b, "Jots"), "%q should not be jots-only", b)
	}
	expect(t, IsJotsOnly("## Notes\n- a", "Notes") && !IsJotsOnly("## Jots\n- a", "Notes"), "configured heading")
}

func TestStripUntouchedTemplate(t *testing.T) {
	tpl := DefaultTemplate
	jots := "## Jots\n\n- 14:32 call Sam\n- 14:40 email Ana"
	strip := func(md, template, heading string) string { return StripUntouchedTemplate(md, template, heading) }
	expect(t, strip(tpl+"\n\n"+jots, tpl, "Jots") == jots, "template + jots round trip")
	expect(t, IsJotsOnly(strip(tpl+"\n\n"+jots, tpl, "Jots"), "Jots"), "stripped result is jots-only")
	expect(t, strip(tpl, tpl, "Jots") == "" && strip("\n"+tpl+"\n\n", tpl, "Jots") == "", "the template alone has nothing to write")
	expect(t, strip(tpl+"\n"+jots, tpl, "Jots") == jots && strip("\n\n"+tpl+"\n\n\n"+jots+"\n\n", tpl, "Jots") == jots, "blank lines around do not matter")
	expect(t, strip("## What I did\n## Finished\n## Started\n## Pending / blocked\n## To do next\n\n"+jots, tpl, "Jots") == jots, "nor between template headings")
	expect(t, strip(tpl+"\n\n### Jots\n- a", tpl, "Jots") == "### Jots\n- a" && strip(tpl+"\n\n## Jots\n- a\n### sub\ntext", tpl, "Jots") == "## Jots\n- a\n### sub\ntext", "other levels")
	// anything the user wrote is left alone
	for _, md := range []string{tpl + "\n\nmy own text", tpl + "\n\n" + jots + "\n\n## Finished\nreal text", "## What I did\n\nwrote code\n\n## Finished\n\n" + jots,
		jots, "", "my own text", replaceHeading(tpl, "## Started", "## Begun") + "\n\n" + jots, "## What I did\n\n## Finished"} {
		expect(t, strip(md, tpl, "Jots") == md, "%q should be untouched", md)
	}
	expect(t, strip(tpl+"\n\n"+jots, "", "Jots") == tpl+"\n\n"+jots, "no template, nothing to strip")
	expect(t, strip("## A\n\n## B\n\n## Jots\n- a", "## A\n\n## B", "Jots") == "## Jots\n- a", "short template")
	expect(t, strip("## AB\n\n## Jots\n- a", "## A", "Jots") == "## AB\n\n## Jots\n- a", "a prefix inside a word is not the template")
	expect(t, strip(tpl+"\n\n## Notes\n- a", tpl, "Notes") == "## Notes\n- a" && strip(tpl+"\n\n## Jots\n- a", tpl, "Notes") == tpl+"\n\n## Jots\n- a", "configured heading")
}

func replaceHeading(s, old, repl string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + repl + s[i+len(old):]
		}
	}
	return s
}

// Not in the Swift suite: DayDocument follows the app-wide Jots rules (what LogStore.jotsRules did).
func TestDayDocumentFollowsJotsRules(t *testing.T) {
	defer SetJotsRules(JotsRules{Heading: DefaultJotsHeading})
	body := "intro words here\n\n## Jots\n- 14:32 call Sam\n- 14:40 email Ana now"
	doc := DayDocument{Day: "2026-10-07", Body: body}
	expect(t, doc.Words() == 3 && doc.JotCount() == 2, "jots are not words by default: %d words %d jots", doc.Words(), doc.JotCount())
	expect(t, doc.Status(8) == StatusPartial, "3 words of 8")
	SetJotsRules(JotsRules{Heading: "Jots", CountTowardLogged: true})
	expect(t, doc.Words() == 8 && doc.Status(8) == StatusLogged, "with the switch on the jot words count: %d", doc.Words())
	SetJotsRules(JotsRules{Heading: "Jots"})
	only := DayDocument{Day: "2026-10-07", Body: "## Jots\n- 14:32 a"}
	expect(t, only.Status(20) == StatusPartial, "a jots-only day is partial, so it stays in Catch up")
	expect(t, (DayDocument{Day: "2026-10-07", IsSkipped: true}).JotCount() == 0, "a skipped day has no jots")
}
