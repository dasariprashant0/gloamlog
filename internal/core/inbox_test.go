package core

// Ported from tests/main.swift "Inbox: entries are read oldest first; junk is skipped; an emoji heading takes jots", plus the
// reader limits and the ts rule that Swift did not test.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func inboxTexts(es []InboxEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Text)
	}
	return out
}

func TestInboxEntriesReadOldestFirstJunkSkippedEmojiHeadingTakesJots(t *testing.T) {
	d := t.TempDir()
	write := func(name, content string) {
		must(t, os.WriteFile(filepath.Join(d, name), []byte(content), 0o644))
	}
	write("1.json", `{"text":"  Fixed the cards.  "}`)
	write("2.json", "not json")
	write("3.json", `{"text":"  "}`)
	write(".4.json", `{"text":"hidden"}`)
	write("5.txt", `{"text":"wrong extension"}`)
	write("6.json", `["text"]`)
	write("7.json", `{"text":42}`)
	expect(t, equal(inboxTexts(InboxPending(d)), []string{"Fixed the cards."}), "one good entry, trimmed: %v", inboxTexts(InboxPending(d)))
	expect(t, len(InboxPending(filepath.Join(d, "nope"))) == 0, "no folder, no entries")

	item := CaptureItem{ID: "x", Ts: mk(2026, 10, 14, 12, 0), Day: "2026-10-14", Text: "Fixed the cards.", Stamp: "14:35", Source: "claude"}
	page := "## \U0001F4DD What I did\n\n- earlier\n\n## Finished"
	out := ApplyJot(item, page, "What I did")
	expect(t, out == "## \U0001F4DD What I did\n\n- earlier\n- 14:35 Fixed the cards.\n\n## Finished", "added under the emoji heading, no new Jots section: %q", out)
}

func TestInboxOldestModificationTimeFirstNameBreaksTies(t *testing.T) {
	d := t.TempDir()
	base := mk(2026, 10, 14, 9, 0)
	for name, mod := range map[string]time.Time{"b.json": base, "a.json": base, "z.json": base.Add(-time.Hour), "m.json": base.Add(time.Hour)} {
		p := filepath.Join(d, name)
		must(t, os.WriteFile(p, []byte(`{"text":"`+strings.TrimSuffix(name, ".json")+`"}`), 0o644))
		setMTime(t, p, mod)
	}
	expect(t, equal(inboxTexts(InboxPending(d)), []string{"z", "a", "b", "m"}), "%v", inboxTexts(InboxPending(d)))
	es := InboxPending(d)
	expect(t, filepath.Base(es[0].Path) == "z.json" && filepath.Dir(es[0].Path) == d, "the path of the file is kept for deleting it after")
}

func TestInboxReaderLimits(t *testing.T) {
	d := t.TempDir()
	// a file over 64 KiB is skipped
	big := `{"text":"` + strings.Repeat("x", InboxMaxFileBytes) + `"}`
	must(t, os.WriteFile(filepath.Join(d, "big.json"), []byte(big), 0o644))
	small := `{"text":"` + strings.Repeat("y", 1000) + `"}`
	must(t, os.WriteFile(filepath.Join(d, "small.json"), []byte(small), 0o644))
	es := InboxPending(d)
	expect(t, len(es) == 1 && filepath.Base(es[0].Path) == "small.json", "over 64 KiB is skipped, never deleted here")
	_, err := os.Stat(filepath.Join(d, "big.json"))
	expect(t, err == nil)

	// 60 files: at most 50 per look, and junk counts toward the 50
	e := t.TempDir()
	base := mk(2026, 10, 14, 9, 0)
	for i := 0; i < 60; i++ {
		name := string(rune('a'+i/26)) + string(rune('a'+i%26)) + ".json"
		content := `{"text":"n` + itoa(i) + `"}`
		if i < 5 {
			content = "junk"
		}
		p := filepath.Join(e, name)
		must(t, os.WriteFile(p, []byte(content), 0o644))
		setMTime(t, p, base.Add(time.Duration(i)*time.Second))
	}
	got := InboxPending(e)
	expect(t, len(got) == InboxMaxEntriesPerLook-5 && got[0].Text == "n5" && got[len(got)-1].Text == "n49", "%d entries, first %s", len(got), got[0].Text)
}

func TestInboxTimestampAndCaptureTime(t *testing.T) {
	d := t.TempDir()
	must(t, os.WriteFile(filepath.Join(d, "a.json"), []byte(`{"text":"x","ts":"2026-10-14T14:35:00Z"}`), 0o644))
	must(t, os.WriteFile(filepath.Join(d, "b.json"), []byte(`{"text":"y","ts":"yesterday"}`), 0o644))
	must(t, os.WriteFile(filepath.Join(d, "c.json"), []byte(`{"text":"z"}`), 0o644))
	setMTime(t, filepath.Join(d, "a.json"), mk(2026, 10, 14, 9, 0))
	setMTime(t, filepath.Join(d, "b.json"), mk(2026, 10, 14, 9, 1))
	setMTime(t, filepath.Join(d, "c.json"), mk(2026, 10, 14, 9, 2))
	es := InboxPending(d)
	want, _ := time.Parse(time.RFC3339, "2026-10-14T14:35:00Z")
	expect(t, es[0].Ts.Equal(want) && es[1].Ts.IsZero() && es[2].Ts.IsZero(), "ts parsed; a bad or missing ts is zero")
	drain := mk(2026, 10, 14, 16, 0)
	expect(t, es[0].CaptureTime(drain, InboxUseFileTime).Equal(want), "the file's ts")
	expect(t, es[0].CaptureTime(drain, InboxUseDrainTime).Equal(drain), "Swift's behaviour")
	expect(t, es[1].CaptureTime(drain, InboxUseFileTime).Equal(drain) && es[2].CaptureTime(drain, InboxUseFileTime).Equal(drain), "no usable ts: the drain time")
	expect(t, es[0].CaptureTime(mk(2026, 10, 14, 9, 0), InboxUseFileTime).Equal(mk(2026, 10, 14, 9, 0)), "a ts after the drain time is not believed")
}

func TestDefaultDirs(t *testing.T) {
	expect(t, DefaultInboxDir("/home/u") == "/home/u/Library/Application Support/Gloamlog/inbox")
	expect(t, DefaultCaptureDir("/home/u") == "/home/u/Library/Application Support/Gloamlog/capture")
}
