package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

func TestCarryOfferOnAListPageDropsDismissedAndPresentLines(t *testing.T) {
	n, _, _, _ := captureNotes(t, true) // today is 2026-10-14
	n.mu.Lock()
	n.settings.MinWords = 1
	n.mu.Unlock()
	save := func(day, body string) {
		if _, err := n.store.Save(day, body); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-10-13", "## Log\n\n- 09:00 shipped it\n- 10:00 write the test #next\n- 11:00 keys from ops #blocked\n- 12:00 call Sam #next")
	save("2026-10-14", "## Log\n\n- 09:00 call Sam")
	p, err := n.Open("2026-10-14")
	if err != nil || p.Carry == nil || strings.Join(p.Carry.Items, "|") != "write the test|keys from ops" {
		t.Fatalf("a line already on today's page is not offered: %+v %v", p.Carry, err)
	}
	if err := n.CarryDismiss("2026-10-14", "keys from ops"); err != nil {
		t.Fatal(err)
	}
	p, _ = n.Open("2026-10-14")
	if p.Carry == nil || len(p.Carry.Items) != 1 || p.Carry.Items[0] != "write the test" || p.Carry.Marks[0] != core.MarkNext {
		t.Fatalf("a dropped line is not offered again: %+v", p.Carry)
	}
	if err := n.CarryDismiss("2026-10-14", "write the test"); err != nil {
		t.Fatal(err)
	}
	if p, _ = n.Open("2026-10-14"); p.Carry != nil {
		t.Fatalf("nothing left, no card: %+v", p.Carry)
	}
	// dropping the same line twice keeps one entry
	if err := n.CarryDismiss("2026-10-14", "write the test"); err != nil || len(n.loadCarryDismissed()["2026-10-14"]) != 2 {
		t.Fatalf("%v %v", err, n.loadCarryDismissed())
	}
}

func TestCarryDismissedFileIsPrunedAndSurvivesDamage(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	if len(n.loadCarryDismissed()) != 0 {
		t.Fatal("no file yet: nothing dropped")
	}
	path := n.carryDismissedPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// today is 2026-10-14: 2026-09-14 is 30 days back (kept), 2026-09-13 is 31 (goes)
	if err := os.WriteFile(path, []byte(`{"2026-09-01":["old"],"2026-09-13":["31"],"2026-09-14":["30"],"2026-10-13":["kept"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := n.CarryDismiss("2026-10-14", "x"); err != nil {
		t.Fatal(err)
	}
	d := n.loadCarryDismissed()
	_, old := d["2026-09-01"]
	_, day31 := d["2026-09-13"]
	if old || day31 || len(d["2026-09-14"]) != 1 || len(d["2026-10-13"]) != 1 || len(d["2026-10-14"]) != 1 {
		t.Fatalf("older than 30 days goes, the rest stays: %v", d)
	}
	for _, bad := range []string{"{broken", "null", ""} {
		if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if len(n.loadCarryDismissed()) != 0 {
			t.Fatalf("a damaged file (%q) offers everything again", bad)
		}
	}
}
