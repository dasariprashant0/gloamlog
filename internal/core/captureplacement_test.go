package core

// Ported from tests/main.swift "K1 CapturePlacement" (render, escaping, apply, block boundaries, contains, open fences/comments, fuzz).

import (
	"fmt"
	"strings"
	"testing"
)

var jotT0 = mk(2026, 10, 7, 14, 32) // Wed 7 Oct 2026, 14:32 New York

// jotItem is Swift's jot(): a note with stamp "14:32" unless told otherwise.
func jotItem(text string) CaptureItem {
	return CaptureItem{ID: newUUID(), Ts: jotT0, Day: "2026-10-07", Text: text, Stamp: "14:32", Source: "hotkey"}
}

func jr(text string) string { return RenderJot(jotItem(text)) }
func jrStamp(text, stamp string, todo bool) string {
	it := jotItem(text)
	it.Stamp, it.Todo = stamp, todo
	return RenderJot(it)
}
func ja(text, body string) string { return ApplyJot(jotItem(text), body, "Jots") }
func jaWith(text, body, heading string) string {
	return ApplyJot(jotItem(text), body, heading)
}
func jhas(it CaptureItem, body string) bool { return JotPresent(it, body, "Jots") }

func TestCaptureRenderStampNoStampTodoMultiLine(t *testing.T) {
	cases := []struct{ got, want, why string }{
		{jr("call with Sam about pricing"), "- 14:32 call with Sam about pricing", ""},
		{jrStamp("x", "", false), "- x", "no stamp"},
		{jrStamp("call Sam", "", true), "- [ ] call Sam", ""},
		{jrStamp("call Sam", "14:32", true), "- [ ] call Sam", "a to-do never carries a time"},
		{jr("one\ntwo\n  three  "), "- 14:32 one\n  two\n  three", "continuation lines are trimmed and indented two spaces"},
		{jr("one\r\ntwo"), "- 14:32 one\n  two", "CRLF"},
		{jr("one\n\n\ntwo"), "- 14:32 one\n  two", "blank lines inside a note are dropped"},
		{jrStamp("one\ntwo", "", true), "- [ ] one\n  two", ""},
		{jr("  padded  "), "- 14:32 padded", ""},
		{jr("emoji 🎉 \"quotes\" and \\ backslash"), "- 14:32 emoji 🎉 \"quotes\" and \\ backslash", "only the start of a line is ever escaped"},
		{jr(""), "- 14:32", "an empty note keeps its stamp, without the trailing space"},
		{jrStamp("", "", false), "-", "an empty note with no stamp is a bare dash"},
		{jrStamp("", "", true), "- [ ]", "an empty to-do"},
	}
	for _, c := range cases {
		expect(t, c.got == c.want, "%s: got %q want %q", c.why, c.got, c.want)
	}
}

func TestCaptureRenderEscapesWhatWouldRewriteThePage(t *testing.T) {
	cases := []struct{ in, want, why string }{
		{"# x", "- 14:32 \\# x", ""}, {"## x", "- 14:32 \\## x", ""}, {"###### x", "- 14:32 \\###### x", ""}, {"#", "- 14:32 \\#", ""},
		{"- x", "- 14:32 \\- x", ""}, {"+ x", "- 14:32 \\+ x", ""}, {"* x", "- 14:32 \\* x", ""},
		{"1. x", "- 14:32 1\\. x", ""}, {"12) x", "- 14:32 12\\) x", ""}, {"1.", "- 14:32 1\\.", ""},
		{"> x", "- 14:32 \\> x", ""}, {">x", "- 14:32 \\>x", ""},
		// things that only look like markers stay as typed
		{"#tag x", "- 14:32 #tag x", ""}, {"####### x", "- 14:32 ####### x", ""},
		{"-5 degrees", "- 14:32 -5 degrees", ""}, {"*bold* word", "- 14:32 *bold* word", ""}, {"+1 yes", "- 14:32 +1 yes", ""},
		{"1.5x faster", "- 14:32 1.5x faster", ""}, {"2026-10-07 plan", "- 14:32 2026-10-07 plan", ""},
		{"call # x", "- 14:32 call # x", "only the start of a line matters"},
		// every line of a multi-line note
		{"a\n# b\n- c\n> d\n1. e", "- 14:32 a\n  \\# b\n  \\- c\n  \\> d\n  1\\. e", ""},
		// setext underlines, thematic breaks, fences and HTML comments would also rewrite the page structure
		{"a\n---", "- 14:32 a\n  \\---", ""}, {"a\n===", "- 14:32 a\n  \\===", ""}, {"***", "- 14:32 \\***", ""}, {"- - -", "- 14:32 \\- - -", ""},
		{"```", "- 14:32 \\```", ""}, {"~~~x", "- 14:32 \\~~~x", ""},
		{"see <!-- x", "- 14:32 see <\\!-- x", ""}, {"<!--", "- 14:32 <\\!--", ""},
	}
	for _, c := range cases {
		expect(t, jr(c.in) == c.want, "%q: got %q want %q", c.in, jr(c.in), c.want)
	}
	expect(t, jrStamp("# x", "", false) == "- \\# x" && jrStamp("1. x", "", true) == "- [ ] 1\\. x", "also without a time, where it matters most")
	// the escaped text still reads as the same words and can never produce a heading
	for _, text := range []string{"# x", "## y z", "- a", "1. b", "> c", "a\n# b", "```", "<!-- x"} {
		page := ja(text, "")
		var titles []string
		for _, s := range BodySections(page) {
			if s.Level > 0 {
				titles = append(titles, s.NormalizedTitle)
			}
		}
		expect(t, equal(titles, []string{"jots"}), "no extra heading for %q: %q", text, page)
	}
}

func TestCaptureApplyNoJotsBlockCreatesOneAtTheEnd(t *testing.T) {
	cases := []struct{ body, want, why string }{
		{"", "## Jots\n\n- 14:32 call Sam", ""},
		{"\n\n  \n", "## Jots\n\n- 14:32 call Sam", ""},
		{"Some text here.\n\n", "Some text here.\n\n## Jots\n\n- 14:32 call Sam", ""},
		{ClassicTemplate, ClassicTemplate + "\n\n## Jots\n\n- 14:32 call Sam", "a template page keeps its headings"},
		{"## Jotsy\n\ntext", "## Jotsy\n\ntext\n\n## Jots\n\n- 14:32 call Sam", "a similar heading is not the Jots heading"},
		{"```\n## Jots\n```", "```\n## Jots\n```\n\n## Jots\n\n- 14:32 call Sam", "a heading inside a code fence is not a heading"},
		{"<!--\n## Jots\n-->\ntext", "<!--\n## Jots\n-->\ntext\n\n## Jots\n\n- 14:32 call Sam", "nor one inside a comment"},
		{"a\r\nb", "a\nb\n\n## Jots\n\n- 14:32 call Sam", "CRLF becomes LF"},
	}
	for _, c := range cases {
		expect(t, ja("call Sam", c.body) == c.want, "%s: %q -> %q want %q", c.why, c.body, ja("call Sam", c.body), c.want)
	}
	todo := jotItem("x")
	todo.Stamp, todo.Todo = "", true
	expect(t, ApplyJot(todo, "text", "Jots") == "text\n\n## Jots\n\n- [ ] x")
}

func TestCaptureApplyExistingJotsBlockGetsTheLineAtItsEnd(t *testing.T) {
	cases := []struct{ body, want, why string }{
		{"text\n\n## Jots\n\n- 14:30 a", "text\n\n## Jots\n\n- 14:30 a\n- 14:32 b", "last block, ends in a list"},
		{"## Jots\n\n- 14:30 a\n\n## Finished\n\nstuff", "## Jots\n\n- 14:30 a\n- 14:32 b\n\n## Finished\n\nstuff", "block in the middle"},
		{"## Jots\n\n- 14:30 a\n\n\n", "## Jots\n\n- 14:30 a\n- 14:32 b", "trailing blank lines are not content"},
		{"## Jots\n\nremember this", "## Jots\n\nremember this\n\n- 14:32 b", "last content line a paragraph: one blank line"},
		{"## Jots\n\n- 14:30 a\n  more", "## Jots\n\n- 14:30 a\n  more\n- 14:32 b", "a multi-line item above is still a list"},
		{"## Jots\n\n- 14:30 a\nlazy", "## Jots\n\n- 14:30 a\nlazy\n- 14:32 b", "so is a lazy continuation"},
		{"## Jots\n\n- a\n\npara", "## Jots\n\n- a\n\npara\n\n- 14:32 b", "a paragraph after the list needs the blank line again"},
		{"## Jots\n\n1. a", "## Jots\n\n1. a\n- 14:32 b", "ordered items are list items"},
		{"## Jots\n- [ ] a", "## Jots\n- [ ] a\n- 14:32 b", "task items are list items"},
		{"## Jots\n\n```\n- code\n```", "## Jots\n\n```\n- code\n```\n\n- 14:32 b", "a list-looking line inside code is not a list"},
		// empty blocks
		{"## Jots", "## Jots\n\n- 14:32 b", ""},
		{"## Jots\n\n", "## Jots\n\n- 14:32 b", ""},
		{"## Jots\n\n## Next\n\nstuff", "## Jots\n\n- 14:32 b\n\n## Next\n\nstuff", ""},
		{"## Jots\n## Next", "## Jots\n\n- 14:32 b\n\n## Next", ""},
	}
	for _, c := range cases {
		expect(t, ja("b", c.body) == c.want, "%s: %q -> %q want %q", c.why, c.body, ja("b", c.body), c.want)
	}
	expect(t, ja("b\nc", "## Jots\n\n- 14:30 a") == "## Jots\n\n- 14:30 a\n- 14:32 b\n  c", "multi-line note")
	c3 := jotItem("c")
	c3.Stamp = "14:33"
	twice := ApplyJot(c3, ja("b", "## Jots\n\n- 14:30 a"), "Jots")
	expect(t, twice == "## Jots\n\n- 14:30 a\n- 14:32 b\n- 14:33 c", "two applies in a row build one list in order: %q", twice)
}

func TestCaptureApplyBlockBoundariesCaseEmojiAndConfiguredHeading(t *testing.T) {
	cases := []struct{ body, heading, want, why string }{
		{"### Jots\n- a\n\n## Next\ntext", "Jots", "### Jots\n- a\n- 14:32 b\n\n## Next\ntext", "a higher heading ends the block"},
		{"### Jots\n- a\n#### Sub\nmore", "Jots", "### Jots\n- a\n#### Sub\nmore\n\n- 14:32 b", "a deeper heading stays inside: the line goes after it"},
		{"# Jots\n- a\n## Sub\ntext\n# Next\nx", "Jots", "# Jots\n- a\n## Sub\ntext\n\n- 14:32 b\n\n# Next\nx", ""},
		{"## 📝 JOTS\n- a", "Jots", "## 📝 JOTS\n- a\n- 14:32 b", "case, emoji"},
		{"## jots!\n- a", "Jots", "## jots!\n- a\n- 14:32 b", "punctuation"},
		{"## Jots\n- a", "📝 Jots", "## Jots\n- a\n- 14:32 b", "matching is by normalised text"},
		{"text", "📝 Jots", "text\n\n## 📝 Jots\n\n- 14:32 b", "a new heading uses the configured text"},
		{"text", "", "text\n\n## Jots\n\n- 14:32 b", "an unusable heading falls back to Jots"},
		{"text", "🙂", "text\n\n## Jots\n\n- 14:32 b", "an emoji-only heading falls back to Jots"},
		{"text", "  Daily \n notes ", "text\n\n## Daily notes\n\n- 14:32 b", "whitespace collapsed"},
		{"## Notes\n- a", "Notes", "## Notes\n- a\n- 14:32 b", ""},
		{"## Jots\n- a\n\n## Other\n\n## Jots\n- z", "Jots", "## Jots\n- a\n- 14:32 b\n\n## Other\n\n## Jots\n- z", "the first Jots block is the target, like the editor"},
		{"## What I did\n\nwrote\n\n### 🗒️ Jots\n\n- 08:55 a\n\n#### Later\n\n- 09:30 z\n\n### Finished\n\nshipped", "Jots",
			"## What I did\n\nwrote\n\n### 🗒️ Jots\n\n- 08:55 a\n\n#### Later\n\n- 09:30 z\n- 14:32 b\n\n### Finished\n\nshipped", "the editor fixture shape"},
	}
	for _, c := range cases {
		got := jaWith("b", c.body, c.heading)
		expect(t, got == c.want, "%s: %q -> %q want %q", c.why, c.body, got, c.want)
	}
}

func TestCaptureContainsAfterApplyAndEditorRespellings(t *testing.T) {
	bodies := []string{"", "text", "## Jots\n\n- 14:30 a", "## Jots\n\n- 14:30 a\n\n## Finished\n\nstuff", ClassicTemplate, "## Jots", "### Jots\n- a\n## N\nz", "## Jots\n\nremember"}
	for _, text := range []string{"call Sam", "# x", "- y", "a\nb", "1. n", "café \U0001F389"} {
		for _, b := range bodies {
			it := jotItem(text)
			expect(t, !jhas(it, b), "not yet in %q", b)
			applied := ApplyJot(it, b, "Jots")
			expect(t, jhas(it, applied), "contains after apply: %q into %q -> %q", text, b, applied)
			other := jotItem(text)
			other.Stamp = "09:00"
			expect(t, !jhas(jotItem(text+" more"), applied) && !jhas(other, applied), "other text or time is another note")
		}
	}
	// what the editor may write back
	expect(t, jhas(jotItem("call Sam"), "## Jots\n\n* 14:32 call Sam") && jhas(jotItem("call Sam"), "## Jots\n\n+ 14:32   call   Sam  "), "marker style, spaces")
	expect(t, jhas(jotItem("# x"), "## Jots\n\n- 14:32 # x") && jhas(jotItem("2 * 3 = 6"), "## Jots\n\n- 14:32 2 \\* 3 = 6") && jhas(jotItem("a_b"), "## Jots\n\n- 14:32 a\\_b"), "escape differences")
	todo := jotItem("call Sam")
	todo.Stamp, todo.Todo = "", true
	expect(t, jhas(todo, "## Jots\n\n- [x] call Sam"), "a to-do ticked since is still the same note")
	expect(t, jhas(jotItem("a\nb"), "## Jots\n\n- 14:32 a\n  b") && jhas(jotItem("a\nb"), "## Jots\n\n- 14:32 a\\\n  b") && jhas(jotItem("a\nb"), "## Jots\n\n- 14:32 a b"), "soft and hard breaks")
	// not the same note
	expect(t, !jhas(jotItem("call Sam"), "## Jots\n\n- 14:33 call Sam") && !jhas(jotItem("call Sam"), "## Jots\n\n- 14:32 call Sam now") && !jhas(jotItem("call Sam"), "## Jots\n\n- 14:32 Call Sam"), "time, text and case matter")
	expect(t, !jhas(jotItem("call Sam"), "## Finished\n\n- 14:32 call Sam"), "outside the Jots block")
	expect(t, !jhas(jotItem("call Sam"), "## Jots\n\ntext\n\n## Finished\n\n- 14:32 call Sam"), "after the block ended")
	expect(t, jhas(jotItem("call Sam"), "## Jots\n\n- a\n\n## X\n\n## Jots\n\n- 14:32 call Sam"), "in any Jots block")
	expect(t, !jhas(jotItem("call Sam"), "## Jots\n\n```\n- 14:32 call Sam\n```"), "code is not a note")
	expect(t, !jhas(todo, "## Jots\n\n- 14:32 call Sam") && !jhas(jotItem("call Sam"), "## Jots\n\n- [ ] call Sam"), "a to-do is not a stamped note")
	noStamp := jotItem("x")
	noStamp.Stamp = ""
	expect(t, jhas(noStamp, "## Jots\n- x") && !jhas(noStamp, "## Jots\n- 14:32 x"), "no stamp")
	expect(t, JotPresent(jotItem("b"), "## Notes\n- 14:32 b", "Notes") && !jhas(jotItem("b"), "## Notes\n- 14:32 b"), "the configured heading")
}

func TestLastJotKeyIsTheLastTopLevelItemOfTheFirstJotsBlock(t *testing.T) {
	_, ok := LastJotKey("text", "Jots")
	expect(t, !ok, "no block")
	_, ok = LastJotKey("## Jots\n\ntext only", "Jots")
	expect(t, !ok, "no item")
	k, ok := LastJotKey("## Jots\n\n- 14:30 a\n  - nested\n- 14:32 <https://x.example> b\n\n## Jots\n- z", "Jots")
	expect(t, ok && k == "14:32 <https://x.example> b", "key %q", k)
}

func TestCaptureApplyPageEndingInsideAnOpenFenceOrCommentStillGetsAVisibleNote(t *testing.T) {
	cases := []struct{ body, want, why string }{
		{"```\ncode", "```\ncode\n```\n\n## Jots\n\n- 14:32 b", "no block, open fence"},
		{"~~~~\ncode", "~~~~\ncode\n~~~~\n\n## Jots\n\n- 14:32 b", "the closing fence copies the opener"},
		{"## Jots\n- a\n```\ncode", "## Jots\n- a\n```\ncode\n```\n\n- 14:32 b", "block, open fence"},
		{"text <!-- open", "text <!-- open\n-->\n\n## Jots\n\n- 14:32 b", "open comment"},
		{"## Jots\n- a\ntext <!-- open\nmore", "## Jots\n- a\ntext <!-- open\n-->\n\n- 14:32 b\n\nmore", "open comment inside the block"},
	}
	for _, c := range cases {
		expect(t, ja("b", c.body) == c.want, "%s: got %q want %q", c.why, ja("b", c.body), c.want)
	}
	for _, body := range []string{"```\ncode", "## Jots\n- a\n```\ncode", "text <!-- open", "~~~\n## Jots\n- a"} {
		it := jotItem("zz")
		out := ApplyJot(it, body, "Jots")
		expect(t, jhas(it, out) && BodyCounts(out, "Jots").JotCount >= 1, "%q", body)
		expect(t, ApplyJot(it, out, "Jots") != out, "apply is not its own guard: JotPresent is")
	}
}

// lcg is the Swift test's deterministic generator.
type lcg struct{ s uint64 }

func (g *lcg) next() int              { g.s = g.s*6364136223846793005 + 1442695040888963407; return int(g.s >> 33) }
func (g *lcg) pick(a []string) string { return a[g.next()%len(a)] }

func TestCaptureApplyFuzzPageLinesNeverLostReorderedOrEdited(t *testing.T) {
	g := lcg{20261007}
	safe := []string{"", "text words here", "## Jots", "### Jots", "# Top", "## Other", "- item", "- 14:30 note", "1. one", "  more", "> quote", "---",
		"- [ ] task", "para line", "## Carried over from Mon", "#### deep", "- 08:00 [x] looks like a task", "# Jots"}
	hazard := append(append([]string{}, safe...), "```", "~~~", "code line", "<!-- c", "-->", "<!-- all -->", "```swift")
	headingCount := func(s string) int {
		n := 0
		for _, l := range ScanBody(s) {
			if l.Kind == LineHeading {
				n++
			}
		}
		return n
	}
	for n := 0; n < 600; n++ {
		hazardous := n%2 == 1
		pool := safe
		if hazardous {
			pool = hazard
		}
		var lines []string
		for i, count := 0, g.next()%14; i < count; i++ {
			lines = append(lines, g.pick(pool))
		}
		body := strings.Join(lines, "\n")
		it := jotItem(fmt.Sprintf("zz%d note", n))
		expect(t, !jhas(it, body), "fresh note is not in %q", body)
		out := ApplyJot(it, body, "Jots")
		expect(t, jhas(it, out), "recognised after apply: %q -> %q", body, out)
		rest := strings.Split(out, "\n")
		kept := true
		for _, l := range lines {
			if trimmed(l) == "" {
				continue
			}
			i := indexOf(rest, l)
			if i < 0 {
				kept = false
				break
			}
			rest = rest[i+1:]
		}
		expect(t, kept, "own lines kept in order: %q -> %q", body, out)
		if !hazardous {
			expect(t, LoggedWords(out, "Jots", false) == LoggedWords(body, "Jots", false), "jots never change the logged words: %q", body)
			expect(t, BodyCounts(out, "Jots").JotCount == BodyCounts(body, "Jots").JotCount+1, "one more jot: %q -> %q", body, out)
			hadBlock := len(jotsBlocks(ScanBody(body), "Jots")) > 0
			add := 1
			if hadBlock {
				add = 0
			}
			expect(t, headingCount(out) == headingCount(body)+add, "only a missing Jots heading is ever added: %q -> %q", body, out)
		}
		// a second, different note joins the same block and the first is still recognised
		it2 := jotItem(fmt.Sprintf("yy%d other", n))
		it2.Stamp = "14:40"
		out2 := ApplyJot(it2, out, "Jots")
		expect(t, jhas(it, out2) && jhas(it2, out2), "two notes: %q", out2)
	}
}

func indexOf(a []string, s string) int {
	for i, x := range a {
		if x == s {
			return i
		}
	}
	return -1
}
