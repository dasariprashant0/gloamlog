package main

import (
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

func TestLogParseAndRenderRoundTrip(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	body := "## Log\n\n*  09:40  Fixed it\n- 10:00 next one #next\n"
	p := n.LogParse(body)
	if !p.OK || len(p.Page.Lines) != 2 {
		t.Fatalf("parse: %+v", p)
	}
	r := n.LogRender(p.Page)
	if r.Markdown != body {
		t.Fatalf("an untouched page comes back byte for byte: %q", r.Markdown)
	}
	if r.Words != 4 || len(r.HTML) != 2 || !strings.Contains(r.HTML[0], "Fixed it") {
		t.Fatalf("words %d, html %q", r.Words, r.HTML)
	}
	if n.LogParse("## What I did\n- x").OK {
		t.Fatal("a classic page is not a list page")
	}
}

func TestLogRenderEscapesHTML(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	r := n.LogRender(core.LogPage{Before: "## Log", Lines: []core.LogLine{{Text: "<script>x</script> **bold**", Blank: 1}}})
	if strings.Contains(r.HTML[0], "<script>") || !strings.Contains(r.HTML[0], "<strong>bold</strong>") {
		t.Fatalf("%q", r.HTML[0])
	}
}

func TestLogAppendLinesAddsOnceAtTheEnd(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	p := n.LogParse("## Log").Page
	a := n.LogAppendLines(p, "- 14:32 note from Claude", core.ListWork, false)
	if a.Result != "appended" || a.Rendered.Markdown != "## Log\n\n- 14:32 note from Claude" {
		t.Fatalf("%+v", a)
	}
	p.Lines = a.Lines
	again := n.LogAppendLines(p, "- 14:40 note from Claude", core.ListWork, false)
	if again.Result != "alreadyPresent" || len(again.Lines) != 1 {
		t.Fatalf("the same text twice is added once: %+v", again)
	}
	two := n.LogAppendLines(p, "- one\n- two", core.ListWork, false)
	if len(two.Lines) != 3 || two.Lines[2].Text != "two" || two.Lines[1].Blank != 0 {
		t.Fatalf("%+v", two.Lines)
	}
}

func TestLogAppendLinesGoToTheirList(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	p := n.LogParse("## Work\n\n- 09:00 standup").Page
	a := n.LogAppendLines(p, "- 21:00 gloamlog standup view", core.ListPersonal, false)
	if a.Result != "appended" || a.Rendered.Markdown != "## Work\n\n- 09:00 standup\n\n## Personal\n\n- 21:00 gloamlog standup view" {
		t.Fatalf("the missing Personal list is written with its first line: %q", a.Rendered.Markdown)
	}
	p.Lines = a.Lines
	if again := n.LogAppendLines(p, "- 22:00 Gloamlog standup view", core.ListWork, false); again.Result != "alreadyPresent" {
		t.Fatalf("a line in either list counts as there: %+v", again)
	}
	w := n.LogAppendLines(p, "- 10:00 review", core.ListWork, false)
	if len(w.Lines) != 3 || w.Lines[1].Text != "review" || w.Lines[1].Section != 0 || w.Lines[1].Blank != 0 {
		t.Fatalf("Work lines stay in Work, the lists in page order: %+v", w.Lines)
	}
}

func TestLogAppendLinesTimedGoesInTimeOrder(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	p := n.LogParse("## Log\n\n- 09:00 a\n- 15:00 c").Page
	a := n.LogAppendLines(p, "- 13:02 b `1a2b3c4`", core.ListWork, true)
	if a.Result != "appended" || a.Rendered.Markdown != "## Log\n\n- 09:00 a\n- 13:02 b `1a2b3c4`\n- 15:00 c" {
		t.Fatalf("%s: %q", a.Result, a.Rendered.Markdown)
	}
	if again := n.LogAppendLines(n.LogParse(a.Rendered.Markdown).Page, "- 13:02 b `1a2b3c4`", core.ListWork, true); again.Result != "alreadyPresent" {
		t.Fatalf("a commit already on the page is not added again: %s", again.Result)
	}
	if first := n.LogAppendLines(n.LogParse("## Log").Page, "- 13:02 b", core.ListWork, true); first.Rendered.Markdown != "## Log\n\n- 13:02 b" {
		t.Fatalf("the first line of an empty Log: %q", first.Rendered.Markdown)
	}
	split := n.LogParse("## Work\n\n- 14:00 w\n\n## Personal\n\n- 09:00 p\n- 18:00 q").Page
	if s := n.LogAppendLines(split, "- 13:02 home `5d6e7f8`", core.ListPersonal, true); s.Rendered.Markdown != "## Work\n\n- 14:00 w\n\n## Personal\n\n- 09:00 p\n- 13:02 home `5d6e7f8`\n- 18:00 q" {
		t.Fatalf("by time in its own list: %q", s.Rendered.Markdown)
	}
}

func TestLogEditorEdgeCases(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	if n.LogParse("").OK {
		t.Fatal("an empty body is not a list page")
	}
	empty := n.LogParse("## Log").Page
	if r := n.LogRender(empty); r.Markdown != "## Log" || r.Words != 0 || len(r.HTML) != 0 {
		t.Fatalf("an empty Log renders as just its heading: %+v", r)
	}
	if a := n.LogAppendLines(empty, "", core.ListWork, false); a.Result != "alreadyPresent" || len(a.Lines) != 0 {
		t.Fatalf("nothing to add: %+v", a)
	}
	if a := n.LogAppendLines(empty, "A paragraph, not a bullet", core.ListWork, false); len(a.Lines) != 0 {
		t.Fatalf("only bullets are added: %+v", a.Lines)
	}
}
