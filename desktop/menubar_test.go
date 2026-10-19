package main

import (
	"bytes"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

func TestMenuViewFor(t *testing.T) {
	for _, c := range []struct {
		name   string
		o      Overview
		reason string
		want   menuView
	}{
		{"logged", Overview{TodayStatus: core.StatusLogged}, "", menuView{Icon: iconLogged, Status: "Logged today ✓"}},
		{"started only", Overview{TodayStatus: core.StatusPartial}, "", menuView{Icon: iconNotLogged, Status: "Not logged yet"}},
		{"nothing yet", Overview{TodayStatus: core.StatusMissed}, "", menuView{Icon: iconNotLogged, Status: "Not logged yet"}},
		{"skipped with a reason", Overview{TodayStatus: core.StatusSkipped}, "Holiday", menuView{Icon: iconLogged, Status: "Skipped today (Holiday)"}},
		{"skipped", Overview{TodayStatus: core.StatusSkipped}, "", menuView{Icon: iconLogged, Status: "Skipped today"}},
		{"not a workday", Overview{TodayStatus: core.StatusOff}, "", menuView{Icon: iconLogged, Status: "Not a workday"}},
		{"folder problem wins", Overview{TodayStatus: core.StatusLogged, FolderError: "gone"}, "", menuView{Icon: iconProblem, Status: "Saving problem: folder unavailable"}},
	} {
		if got := menuViewFor(c.o, c.reason); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestStreakAndWeekLines(t *testing.T) {
	if got := streakLine(core.StreakResult{}); got != "No streak yet" {
		t.Errorf("no streak: %q", got)
	}
	if got := streakLine(core.StreakResult{Current: 5, Best: 14}); got != "5-day streak · best 14" {
		t.Errorf("streak: %q", got)
	}
	week := []core.HeatCell{ // Sun 18 Oct 2026 .. Sat 24 Oct: weekends off
		{Day: "2026-10-18", Status: core.StatusOff}, {Day: "2026-10-19", Status: core.StatusLogged},
		{Day: "2026-10-20", Status: core.StatusPartial}, {Day: "2026-10-21", Status: core.StatusSkipped},
		{Day: "2026-10-22", Status: core.StatusMissed}, {Day: "2026-10-23", Status: core.StatusFuture},
		{Day: "2026-10-24", Status: core.StatusOff},
	}
	if got := weekLine(week); got != "This week  M ●  T ◐  W –  T ○  F ·" {
		t.Errorf("week: %q", got)
	}
	if got := weekLine(nil); got != "" {
		t.Errorf("no week: %q", got)
	}
}

func TestCurrentMenuViewReadsTheFolder(t *testing.T) {
	n, _, folder, _ := captureNotes(t, false)
	if got := currentMenuView(n); got.Icon != iconProblem {
		t.Fatalf("missing folder: %+v", got)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := currentMenuView(n); got.Icon != iconNotLogged || got.Status != "Not logged yet" || got.Streak != "No streak yet" {
		t.Fatalf("empty folder: %+v", got)
	}
	// Twenty words make a day "logged" (the default minimum).
	body := "## What I did\n" + strings.Repeat("word ", 25)
	if _, err := n.Save(today, body); err != nil {
		t.Fatal(err)
	}
	if got := currentMenuView(n); got.Icon != iconLogged || got.Status != "Logged today ✓" || !strings.HasPrefix(got.Week, "This week") {
		t.Fatalf("logged day: %+v", got)
	}
}

// A 44 px icon with a thin outline inks a few hundred to about a thousand pixels; far outside that the drawing is broken.
const minInked, maxInked = 60, 1400

func TestMenuIconsAreTemplateImages(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range []menuIcon{iconNotLogged, iconLogged, iconProblem} {
		b := menuIconPNG(k)
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if r := img.Bounds(); r.Dx() != 44 || r.Dy() != 44 {
			t.Errorf("%s: %v, want 44x44", k, r)
		}
		inked := 0
		for y := 0; y < 44; y++ {
			for x := 0; x < 44; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				if a > 0 {
					inked++
					if r|g|bl != 0 {
						t.Fatalf("%s: a template image must be black, got a coloured pixel", k)
					}
				}
			}
		}
		if inked < minInked || inked > maxInked {
			t.Errorf("%s: %d inked pixels looks wrong", k, inked)
		}
		seen[string(b)] = true
	}
	if len(seen) != 3 {
		t.Error("the three icons should differ")
	}
}
