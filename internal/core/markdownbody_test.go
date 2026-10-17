package core

// Ported from tests/main.swift: "word counting", "headings and sections", plus the Swift tests of the carried-over exclusion that
// only exercise MarkdownBody (the ones that need the store or CarryOver are in carryover_test.go).

import (
	"strings"
	"testing"
)

type wordCase struct {
	in   string
	want int
	why  string
}

func runWordCases(t *testing.T, cases []wordCase) {
	t.Helper()
	for _, c := range cases {
		if got := W(c.in); got != c.want {
			t.Errorf("W(%q) = %d, want %d %s", c.in, got, c.want, c.why)
		}
	}
}

func TestWordsBasics(t *testing.T) {
	runWordCases(t, []wordCase{
		{"", 0, ""}, {"  \n\n ", 0, ""},
		{"Hello world", 2, ""},
		{"# Title\n## What I did\nwrote code", 2, "heading lines are ignored"},
		{"#tag stays", 2, "#tag is not a heading"},
		{"####### seven", 1, "7 hashes is not a heading; the hashes are not a word"},
		{"## Done ##\n###### six\nreal words", 2, "closing hashes, level 6"},
		{"   ### three spaces\ntext", 1, "up to 3 leading spaces is still a heading"},
		{"    ### four spaces is not a heading", 6, ""},
		{"-\n- \n* [ ]\n- [x]\n1.\n- [ ] \n+ [X]\n2) [ ]\n[ ]", 0, "empty list/task markers"},
		{"- [ ] Call Ana", 2, ""}, {"- [x] done thing", 2, ""}, {"  - [ ] sub task", 2, ""},
		{"1. First thing\n2) second thing", 4, ""}, {"* star item", 2, ""},
		{"**bold** text", 2, "not a list marker"}, {"-5 degrees", 2, "not a list marker"},
		{"> quoted words here", 3, ""}, {"> - [ ] task", 1, ""}, {">> nested quote", 2, ""},
		{"a b\tc", 3, "NBSP, tab"}, {"one two\r\nthree", 3, "CRLF"},
		{"don't can't", 2, ""}, {"https://example.com/a/b", 1, ""}, {"v1.2.3 shipped", 2, ""}, {"2026-10-06", 1, ""},
		{"---\n***\n___\n— – -", 0, "rules and dashes are not words"},
		{"| a | b |\n|---|---|\n| c | d |", 4, "tables"},
		{"[two words](http://x.com)", 2, ""}, {"<https://example.com>", 1, ""},
	})
}

func TestWordsCodeFences(t *testing.T) {
	runWordCases(t, []wordCase{
		{"```swift\nlet x = 1\n```", 3, "fence lines ignored, code between them counts"},
		{"~~~\ncode here\n~~~\nafter", 3, ""},
		{"```\n# not heading\n```", 2, "headings do not exist inside code"},
		{"````\n```\ntext\n````", 1, "a shorter fence does not close a longer one"},
		{"```\none two", 2, "unclosed fence runs to the end"},
		{"`a b`", 2, ""}, {"```\n```", 0, ""},
	})
}

func TestWordsImagesHTMLComments(t *testing.T) {
	runWordCases(t, []wordCase{
		{"![diagram](assets/2026-10-06-1a2b3c4d.png)", 0, ""},
		{"see ![a b](x.png) here", 2, ""}, {"![a](x.png \"title text\")", 0, ""},
		{"![ref image][r1] words", 1, ""}, {"- ![](a.png)", 0, ""},
		{"<img src=\"a.png\" alt=\"x y\">", 0, ""}, {"<br>", 0, ""}, {"text<br>more", 2, ""},
		{"<!-- hidden words here -->", 0, ""}, {"visible <!-- hidden --> text", 2, ""},
		{"start\n<!--\nsecret one two\n-->\nend", 2, "multi-line comment"},
		{"a b <!-- c d", 2, "unterminated comment hides the rest"},
		{"<!-- a\nb --> tail words", 2, ""}, {"## Title <!-- note -->\nbody", 1, ""},
		{"```\n<!-- x y -->\n```", 2, "comments are literal inside code"},
	})
}

func TestWordsEmojiAndNonLatin(t *testing.T) {
	runWordCases(t, []wordCase{
		{"Done ✅ 🎉", 1, ""}, {"🎉🎉🎉", 0, ""}, {"✅", 0, ""}, {"Shipped🚀", 1, ""}, {"👍 great job 👍", 2, ""},
		{"👨‍👩‍👧", 0, "ZWJ sequence is not a word"},
		{"Сегодня я закончил отчёт", 4, "Cyrillic"},
		{"اليوم أنهيت التقرير", 3, "Arabic"}, {"היום סיימתי את הדוח", 4, "Hebrew"},
		{"आज मैंने रिपोर्ट पूरी की", 5, "Devanagari"},
		{"오늘 보고서를 완료했다", 3, "Hangul uses spaces"},
		{"今天完成了报告", 4, "CJK run of 7 chars = 4 words (ceil(n/2))"},
		{"完成 报告", 2, ""}, {"Q4完成", 2, ""}, {"hello世界", 2, ""}, {"完成，报告。", 2, ""},
		{"ありがとう", 3, ""}, {"コーヒーを飲んだ", 4, ""},
		{"วันนี้ทำงานเสร็จ", 1, "Thai: one word per space-delimited chunk (documented limit)"},
	})
}

func TestHasContentAndStatusBoundaries(t *testing.T) {
	for _, s := range []string{"", "## A\n\n## B", "- [ ]\n", "<!-- c -->", "```\n```"} {
		expect(t, !HasContent(s), "HasContent(%q) should be false", s)
	}
	expect(t, HasContent("![](assets/x.png)"), "an image alone is content")
	expect(t, HasContent("- [ ] x") && HasContent("---"), "a task and a rule are content")
	doc := func(body string) DayDocument { return DayDocument{Day: "2026-10-05", Body: body} }
	expect(t, doc(lorem(19, "w")).Status(20) == StatusPartial, "19 words is partial")
	expect(t, doc(lorem(20, "w")).Status(20) == StatusLogged, "20 words is logged")
	expect(t, doc(DefaultTemplate).Status(20) == StatusMissed, "template alone is no log")
	expect(t, doc(DefaultTemplate+"\n\n"+lorem(3, "w")).Words() == 3, "template plus 3 words")
	expect(t, doc("![](a.png)").Status(1) == StatusMissed, "an image is content but not words")
	expect(t, DayDocument{Day: "2026-10-05", IsSkipped: true, SkipReason: "x"}.Status(1) == StatusSkipped, "skipped")
	expect(t, doc(lorem(1, "w")).Status(0) == StatusLogged, "minWords below 1 behaves as 1")
}

func TestNormalizeHeading(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"📝 What I did", "what i did"}, {"⏳ Pending / blocked", "pending blocked"},
		{"Pending / blocked", "pending blocked"}, {"📌 To do next", "to do next"}, {"To do next", "to do next"},
		{"  To-Do   Next!! ", "to do next"}, {"Pending/blocked", "pending blocked"},
		{"Q&A", "q a"}, {"Café ☕", "café"}, {"Привет, мир!", "привет мир"},
		{"1️⃣ First", "1 first"}, {"हिंदी", "हिंदी"}, {"🙂", ""}, {"", ""},
	} {
		if got := NormalizeHeading(c.in); got != c.want {
			t.Errorf("NormalizeHeading(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBodySections(t *testing.T) {
	body := "intro line\n\n## 📝 What I did\ntext A\n#tag line\n\n### Sub\nsub text\n\n```\n## not heading\n```\n\n## Finished\n"
	s := BodySections(body)
	var levels []int
	var titles, norms []string
	for _, x := range s {
		levels = append(levels, x.Level)
		titles = append(titles, x.Title)
		norms = append(norms, x.NormalizedTitle)
	}
	expect(t, equal(levels, []int{0, 2, 3, 2}), "levels %v", levels)
	expect(t, equal(titles, []string{"", "📝 What I did", "Sub", "Finished"}), "titles %q", titles)
	expect(t, equal(norms, []string{"", "what i did", "sub", "finished"}), "normalized %q", norms)
	expect(t, s[0].Text == "intro line" && s[1].Text == "text A\n#tag line", "texts")
	expect(t, s[2].Text == "sub text\n\n```\n## not heading\n```" && s[3].Text == "", "texts 2")

	e := BodySections("## Title ##\n##\n   ## three\n    ## four spaces\n## C#\n####### x\nSetext\n======")
	var et []string
	for _, x := range e {
		et = append(et, x.Title)
	}
	expect(t, equal(et, []string{"Title", "", "three", "C#"}), "edge titles %q", et)
	expect(t, e[2].Text == "## four spaces", "4 leading spaces is text, not a heading")
	expect(t, e[3].Text == "####### x\nSetext\n======", "7 hashes and setext underlines are plain text")
	expect(t, len(BodySections("")) == 0 && len(BodySections("\n\n")) == 0, "empty")
	expect(t, equal(TemplateHeadings(DefaultTemplate), []string{"What I did", "Finished", "Started", "Pending / blocked", "To do next"}), "template headings")
	expect(t, equal(TemplateHeadings("## A\n### a\n# 🅰 A\n## B"), []string{"A", "B"}), "de-duplicated by normalised title")
}

func TestTrimShiftPrunePartition(t *testing.T) {
	expect(t, TrimBody("\n\n  indented\nline  \n\n") == "  indented\nline", "trim keeps first-line indentation")
	expect(t, TrimBody("a\r\nb\r\n") == "a\nb", "CRLF")
	for _, c := range []struct {
		in    string
		level int
		want  string
		why   string
	}{
		{"# Big\n## Small\ntext", 3, "### Big\n#### Small\ntext", ""},
		{"### a\n#### b", 3, "### a\n#### b", ""},
		{"# a\n###### b", 3, "### a\n###### b", "capped at 6"},
		{"no headings <!-- c -->", 3, "no headings", ""},
	} {
		if got := ShiftHeadings(c.in, c.level); got != c.want {
			t.Errorf("ShiftHeadings(%q, %d) = %q, want %q %s", c.in, c.level, got, c.want, c.why)
		}
	}
	expect(t, PruneEmptySections("## A\n\n## B\ntext\n\n## C\n- [ ]\n\n## D\n### E\n### F\nstuff") == "## B\ntext\n\n## D\n### F\nstuff", "prune")
	expect(t, PruneEmptySections(DefaultTemplate) == "", "the template alone prunes to nothing")

	body := "pre text\n## Finished\nA\n### Sub\nB\n## Misc\nC\n## finished\nD\n#### Deep\nE"
	matched, other := PartitionBody(body, []string{"finished"}, SubheadingBoldLabel)
	expect(t, len(matched) == 2 && matched[0].Index == 0 && matched[1].Index == 0, "two matches of heading 0")
	expect(t, matched[0].Text == "A\n\n**Sub**\nB" && matched[1].Text == "D\n\n**Deep**\nE", "matched texts: %q", matched)
	expect(t, other == "pre text\n\n**Misc**\nC", "other: %q", other)
	dropped, _ := PartitionBody(body, []string{"finished"}, SubheadingDrop)
	expect(t, dropped[0].Text == "A\n\nB", "sub-headings dropped: %q", dropped[0].Text)
}

// ---- the carried-over exclusion (words rule) ----

func TestCarriedOverTasksNeverCount(t *testing.T) {
	carried := func(label string, items, per int) string {
		s := "## Carried over from " + label + "\n\n"
		for i := 1; i <= items; i++ {
			if i > 1 {
				s += "\n"
			}
			s += "- [ ] " + lorem(per, "i"+string(rune('0'+i))+"w")
		}
		return s
	}
	block := carried("Fri 2 Oct", 4, 8)
	expect(t, HasContent(block) && W(block) == 0, "32 task words, none counted")
	d := DayDocument{Day: "2026-10-06", Body: block}
	expect(t, d.Status(20) == StatusMissed && d.Status(1) == StatusMissed, "a carried block alone is no log")

	// real words anywhere count, whatever surrounds the block
	onTemplate := InsertCarryOver(DefaultTemplate, "Carried over from Fri 2 Oct", []string{lorem(10, "t"), lorem(10, "u")})
	for _, c := range []struct {
		n    int
		want DayStatus
	}{{0, StatusMissed}, {1, StatusPartial}, {19, StatusPartial}, {20, StatusLogged}, {50, StatusLogged}} {
		real := lorem(c.n, "real")
		var bullets string
		for i, w := range strings.Fields(real) {
			if i > 0 {
				bullets += "\n"
			}
			bullets += "- " + w
		}
		bodies := []string{
			strings.Replace(onTemplate, "## What I did", "## What I did\n"+real, 1), // under another heading
			block + "\n\n" + real, // headingless paragraph right below the block
			block + "\n\n" + real + "\n\n## Finished\n",
			block + "\n\n" + bullets, // plain bullets
		}
		for _, b := range bodies {
			doc := DayDocument{Day: "2026-10-06", Body: b}
			if doc.Words() != c.n || doc.Status(20) != c.want {
				t.Errorf("n=%d got %d %s in %.60q", c.n, doc.Words(), doc.Status(20), b)
			}
		}
	}
}

func TestCarriedOverExclusionDetails(t *testing.T) {
	task := "- [ ] " + lorem(4, "t") // 4 words
	for _, h := range []string{"## Carried over from Mon 5 Oct", "# Carried over from Mon", "### carried OVER FROM friday", "#### **Carried over from Mon**",
		"## 🔁 Carried over from Mon", "## Carried over from"} {
		expect(t, W(h+"\n"+task) == 0 && HasContent(h+"\n"+task), "%s", h)
	}
	// look-alike headings are the user's own and count like any other
	for _, h := range []string{"## Carried over", "## Carried over today", "## Not carried over from Mon", "## Carried overfrom Mon", "## Carry over from Mon", "## Carried from Mon"} {
		expect(t, W(h+"\n"+task) == 4, "%s", h)
	}
	expect(t, W("Carried over from Mon\n"+task) == 8, "the phrase as plain text is not a heading (4 + 4)")
	expect(t, W("```\n## Carried over from Mon\n```\n"+task) == 8, "a heading inside a code fence is not a heading (4 code words + 4 task words)")
	// extent: up to the next heading of the same or a higher level
	runWordCases(t, []wordCase{
		{"## Carried over from Mon\n" + task + "\n\n## What I did\nreal words here", 3, ""},
		{"### Carried over from Mon\n" + task + "\n## Next\n" + task, 4, "a higher-level heading ends the block"},
		{"## Carried over from Mon\n" + task + "\n### Details\n" + task + "\n## Next\nreal", 1, "deeper sub-headings stay inside"},
		{"# Carried over from Mon\n" + task + "\n## Notes\n" + task + "\n# Real\nyes words", 2, ""},
		{"## What I did\nmine here\n### Carried over from Mon\n" + task + "\n## Next\nafter text", 4, "a block nested in a normal section: 2 + 2"},
		// multiple and nested blocks
		{"## Carried over from Fri 2 Oct\n" + task + "\n\n## Carried over from Mon 5 Oct\n" + task + "\n\n## What I did\nreal words here", 3, ""},
		{"## Carried over from Fri\n" + task + "\n### Carried over from Mon\n" + task + "\n## Next\nreal one", 2, ""},
		{"## Carried over from Fri\n" + task + "\n## Carried over from Mon\n" + task + "\n### Carried over from Sun\n" + task + "\n# Top\nreal one", 2, ""},
	})
	// every kind of task line is skipped, ticked or rewritten, nested or not; other text under the block is the user's own
	b := "## Carried over from Mon\n- [x] done thing here\n- [ ] rewritten into a much longer sentence than before\n* [ ] star task words\n1. [ ] numbered task words\n  - [ ] nested sub task words\n+ [X] plus task words"
	expect(t, W(b) == 0 && HasContent(b), "all task lines skipped")
	runWordCases(t, []wordCase{
		{b + "\n\nmy own note under the block", 6, "a paragraph typed under the block counts"},
		{b + "\n- plain bullet two\n- plain bullet words", 6, "so do plain bullets"},
		{b + "\n\n```\ncode words\n```", 2, "and code"},
		{"## Carried over from Mon\n<!-- c -->\n![](a.png)\n- [ ]\n" + task, 0, ""},
		// outside a carried block nothing changed
		{"## To do next\n- [ ] one two three\n- [x] four five", 5, ""}, {"- [ ] loose task words", 3, ""},
		{"## Carried over from Mon\n\n## Real\n" + task, 4, "an empty block ends at the next heading"},
	})
	expect(t, HasContent("## Carried over from Mon\n![](a.png)"), "an image under a carried heading is still content")
}
