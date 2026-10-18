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
		{"logged", Overview{TodayStatus: core.StatusLogged}, "", menuView{iconLogged, "Logged today ✓"}},
		{"started only", Overview{TodayStatus: core.StatusPartial}, "", menuView{iconNotLogged, "Not logged yet"}},
		{"nothing yet", Overview{TodayStatus: core.StatusMissed}, "", menuView{iconNotLogged, "Not logged yet"}},
		{"skipped with a reason", Overview{TodayStatus: core.StatusSkipped}, "Holiday", menuView{iconLogged, "Skipped today (Holiday)"}},
		{"skipped", Overview{TodayStatus: core.StatusSkipped}, "", menuView{iconLogged, "Skipped today"}},
		{"not a workday", Overview{TodayStatus: core.StatusOff}, "", menuView{iconLogged, "Not a workday"}},
		{"folder problem wins", Overview{TodayStatus: core.StatusLogged, FolderError: "gone"}, "", menuView{iconProblem, "Saving problem: folder unavailable"}},
	} {
		if got := menuViewFor(c.o, c.reason); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
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
	if got := currentMenuView(n); got != (menuView{iconNotLogged, "Not logged yet"}) {
		t.Fatalf("empty folder: %+v", got)
	}
	// Twenty words make a day "logged" (the default minimum).
	body := "## What I did\n" + strings.Repeat("word ", 25)
	if _, err := n.Save(today, body); err != nil {
		t.Fatal(err)
	}
	if got := currentMenuView(n); got != (menuView{iconLogged, "Logged today ✓"}) {
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
