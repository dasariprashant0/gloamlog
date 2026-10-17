package core

// Ported from tests/main.swift: "day file format and old files".

import (
	"strings"
	"testing"
)

func TestOldFilesLoadAsPlainMarkdownAndResaveIsStable(t *testing.T) {
	store := newStore(t)
	v02 := "# 2026-10-05\n\n"
	for _, p := range [][2]string{{"📝 What I did", "Wrote code"}, {"✅ Finished", "Shipped it"}, {"🚀 Started", "Docs"},
		{"⏳ Pending / blocked", "Waiting on Ana"}, {"📌 To do next", "Review PR"}} {
		v02 += "## " + p[0] + "\n" + p[1] + "\n\n"
	}
	writeRaw(t, store.Dir, "2026-10-05", v02)
	d := load(t, store, "2026-10-05")
	expect(t, strings.HasPrefix(d.Body, "## 📝 What I did\nWrote code\n\n## ✅ Finished") && strings.HasSuffix(d.Body, "## 📌 To do next\nReview PR"), "body: %q", d.Body)
	expect(t, !strings.Contains(d.Body, "# 2026-10-05") && !d.IsSkipped, "title stripped")
	expect(t, d.Words() == 10 && d.Status(20) == StatusPartial && d.Status(10) == StatusLogged, "words %d", d.Words())
	save(t, store, "2026-10-05", d.Body)
	once := fileText(t, store.Dir, "2026-10-05")
	expect(t, strings.HasPrefix(once, "# 2026-10-05\n\n## 📝 What I did\nWrote code") && strings.HasSuffix(once, "Review PR\n"), "once: %q", once)
	save(t, store, "2026-10-05", load(t, store, "2026-10-05").Body)
	expect(t, fileText(t, store.Dir, "2026-10-05") == once, "save(load(x)) is a fixed point")
	var norms []string
	for _, s := range BodySections(d.Body) {
		norms = append(norms, s.NormalizedTitle)
	}
	expect(t, equal(norms, []string{"what i did", "finished", "started", "pending blocked", "to do next"}), "norms %q", norms)
}

func TestOldBackslashEscapesUnescapeOnLoad(t *testing.T) {
	store := newStore(t)
	writeRaw(t, store.Dir, "2026-10-05", "# 2026-10-05\n\n## 📝 What I did\nline\n\\## fake\n\\\\## two\n  \\## indented stays\n\n```\n\\## code stays\n```\n")
	d := load(t, store, "2026-10-05")
	lines := strings.Split(d.Body, "\n")
	has := func(s string) bool {
		for _, l := range lines {
			if l == s {
				return true
			}
		}
		return false
	}
	expect(t, has("## fake") && has("\\## two") && has("  \\## indented stays") && has("\\## code stays"), "%q", lines)
}

func TestFileFormatTitleBOMCRLFSkipMarkers(t *testing.T) {
	store := newStore(t)
	loadRaw := func(s string) DayDocument {
		writeRaw(t, store.Dir, "2026-10-05", s)
		return *load(t, store, "2026-10-05")
	}
	for _, c := range []struct{ in, body, why string }{
		{"# 2026-10-05\n\nbody", "body", ""},
		{"body only", "body only", ""},
		{"\n\n# 2026-10-05\n\n\nbody\n\n\n", "body", ""},
		{"# 2026-01-01\n\nbody", "body", "any date title is stripped"},
		{"# 2026-10-05 retro\n\nbody", "# 2026-10-05 retro\n\nbody", "a title with text is content"},
		{"# My plan\n\nbody", "# My plan\n\nbody", ""},
		{"\xEF\xBB\xBF# 2026-10-05\r\n\r\nline one\r\nline two\r\n", "line one\nline two", "BOM and CRLF"},
	} {
		if got := loadRaw(c.in).Body; got != c.body {
			t.Errorf("load(%q).Body = %q, want %q %s", c.in, got, c.body, c.why)
		}
	}
	d := loadRaw("# 2026-10-05\n\n> Skipped: Holiday\n")
	expect(t, d.IsSkipped && d.SkipReason == "Holiday" && d.Body == "", "skip with reason: %+v", d)
	d = loadRaw("# 2026-10-05\n\n> Skipped\n\n\n")
	expect(t, d.IsSkipped && d.SkipReason == "", "skip without reason")
	expect(t, !loadRaw("# 2026-10-05\n\n> Skipped the meeting\n").IsSkipped, "no colon: ordinary quote")
	expect(t, !loadRaw("# 2026-10-05\n\n> Skipped: x\nmore text\n").IsSkipped, "marker must be the only content")
	expect(t, SerializeDay("2026-10-05", "\n\nhello\n\n") == "# 2026-10-05\n\nhello\n", "serialize")
	expect(t, SerializeSkip("2026-10-05", " a\nb ") == "# 2026-10-05\n\n> Skipped: a b\n", "serializeSkip joins lines")
	expect(t, SerializeSkip("2026-10-05", "") == "# 2026-10-05\n\n> Skipped\n", "serializeSkip without reason")
}
