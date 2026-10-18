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
	a := n.LogAppendLines(p, "- 14:32 note from Claude")
	if a.Result != "appended" || a.Rendered.Markdown != "## Log\n\n- 14:32 note from Claude" {
		t.Fatalf("%+v", a)
	}
	p.Lines = a.Lines
	again := n.LogAppendLines(p, "- 14:40 note from Claude")
	if again.Result != "alreadyPresent" || len(again.Lines) != 1 {
		t.Fatalf("the same text twice is added once: %+v", again)
	}
	two := n.LogAppendLines(p, "- one\n- two")
	if len(two.Lines) != 3 || two.Lines[2].Text != "two" || two.Lines[1].Blank != 0 {
		t.Fatalf("%+v", two.Lines)
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
	if a := n.LogAppendLines(empty, ""); a.Result != "alreadyPresent" || len(a.Lines) != 0 {
		t.Fatalf("nothing to add: %+v", a)
	}
	if a := n.LogAppendLines(empty, "A paragraph, not a bullet"); len(a.Lines) != 0 {
		t.Fatalf("only bullets are added: %+v", a.Lines)
	}
}
