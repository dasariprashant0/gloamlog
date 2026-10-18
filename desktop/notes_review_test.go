package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// reviewNotes is a Notes over a temp folder, with "now" fixed on Thu 15 Oct 2026 (the week of Mon 12 Oct).
func reviewNotes(t *testing.T, pages map[string]string) *Notes {
	t.Helper()
	dir := t.TempDir()
	for day, body := range pages {
		if err := os.WriteFile(filepath.Join(dir, day+".md"), []byte("# "+day+"\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := core.DefaultSettings(testHome)
	s.StorageFolder, s.MinWords, s.WeekStart = dir, 3, 2 // Monday first
	s.LogStartDate = "2026-10-12"
	n := &Notes{home: testHome, backupDir: t.TempDir(), now: func() time.Time { return time.Date(2026, 10, 15, 10, 0, 0, 0, time.Local) }}
	n.apply(s)
	return n
}

const logged = "## What I did\n- shipped the export sheet today\n\n## To do next\n- [ ] tidy the sidebar\n"

func TestReviewWeekTilesCountsAndCatchUp(t *testing.T) {
	n := reviewNotes(t, map[string]string{
		"2026-10-12": logged,
		"2026-10-13": "## What I did\n- one two\n", // started: 2 of 3 words
	})
	if err := n.store.Skip("2026-10-14", "Holiday"); err != nil {
		t.Fatal(err)
	}
	d, err := n.Review(ReviewRequest{Scale: "week", Day: "2026-10-15", IncludeSkipped: true})
	if err != nil {
		t.Fatal(err)
	}
	if d.Period.Start != "2026-10-12" || d.Period.End != "2026-10-18" || d.Title != "Week of 12 to 18 Oct 2026" {
		t.Fatalf("period %+v title %q", d.Period, d.Title)
	}
	if d.CountsLine != "Logged 1 of 3 · 1 skipped · 1 not logged" {
		t.Errorf("counts line %q", d.CountsLine)
	}
	if len(d.Tiles) != 7 || !strings.HasSuffix(d.Tiles[0].Bottom, " words") || d.Tiles[2].Bottom != "Holiday" || d.Tiles[3].Bottom != "Write" || !d.Tiles[3].IsToday {
		t.Errorf("tiles %+v", d.Tiles)
	}
	if d.Tiles[4].Opens || d.Tiles[3].Opens != true {
		t.Errorf("only days that have happened open: %+v", d.Tiles)
	}
	// Tue is started (not logged); Thu is today, unlogged but still going: only Tue needs catching up.
	if len(d.MissingDays) != 1 || d.MissingDays[0] != "2026-10-13" {
		t.Errorf("missing days %v", d.MissingDays)
	}
	if d.FileName != "Gloamlog 2026-W42" || d.CatchUpLabel != "Catch up this week" || d.CanNext || !d.CanPrev || !d.IsCurrent {
		t.Errorf("file %q label %q next %v prev %v current %v", d.FileName, d.CatchUpLabel, d.CanNext, d.CanPrev, d.IsCurrent)
	}

	off, _ := n.Review(ReviewRequest{Scale: "week", Day: "2026-10-15", IncludeSkipped: false})
	if off.CountsLine != "Logged 1 of 2 · 1 not logged" {
		t.Errorf("without skipped days: %q", off.CountsLine)
	}
}

func TestReviewStepStaysInsideTheDaysThatExist(t *testing.T) {
	n := reviewNotes(t, nil)
	next, _ := n.Review(ReviewRequest{Scale: "week", Day: "2026-10-15", Shift: 1})
	if next.Period.Start != "2026-10-12" {
		t.Errorf("stepping into the future moved to %v", next.Period)
	}
	prev, _ := n.Review(ReviewRequest{Scale: "month", Day: "2026-10-15", Shift: -1})
	if prev.Period.Start != "2026-09-01" || prev.Period.End != "2026-09-30" || !prev.CanNext {
		t.Errorf("previous month %+v next %v", prev.Period, prev.CanNext)
	}
	y, _ := n.Review(ReviewRequest{Scale: "year", Day: "2026-10-15"})
	if len(y.Months) != 12 || y.Months[0].Line != "–" || y.Months[11].Line != "–" || !y.Months[11].IsFuture {
		t.Errorf("year rows %+v", y.Months)
	}
	if y.Months[9].Name != "October" || y.Months[9].IsFuture {
		t.Errorf("october row %+v", y.Months[9])
	}
}

func TestReviewRangeEitherOrderNeverPastToday(t *testing.T) {
	n := reviewNotes(t, map[string]string{"2026-10-12": logged})
	r, _ := n.Review(ReviewRequest{Scale: "range", From: "2026-12-01", To: "2026-10-12", IncludeSkipped: true})
	if r.Period.Start != "2026-10-12" || r.Period.End != "2026-10-15" { // swapped, then cut at today
		t.Errorf("range %+v", r.Period)
	}
	if r.CatchUpLabel != "Catch up this range" || len(r.Days) != 1 {
		t.Errorf("label %q days %d", r.CatchUpLabel, len(r.Days))
	}
	step, _ := n.Review(ReviewRequest{Scale: "range", From: "2026-10-12", To: "2026-10-15", Shift: -1})
	if step.Period.Start != "2026-10-08" || step.Period.End != "2026-10-11" { // four days back
		t.Errorf("stepped range %+v", step.Period)
	}
}

func TestReviewGroupsAndHTML(t *testing.T) {
	n := reviewNotes(t, map[string]string{"2026-10-12": "## ✅ Finished\n- shipped **it** today\n"})
	s := n.settings
	s.Template = "## ✅ Finished\n\n## To do next"
	n.apply(s)
	d, err := n.Review(ReviewRequest{Scale: "week", Day: "2026-10-12", BySection: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Groups) != 1 || d.Groups[0].Title != "Finished" || !strings.Contains(d.Groups[0].Items[0].HTML, "<strong>it</strong>") || d.Groups[0].Items[0].Label != "Mon 12" {
		t.Errorf("groups %+v", d.Groups)
	}
	if d.Empty || d.EmptyText != "" {
		t.Errorf("a week with writing is not empty")
	}
	empty := reviewNotes(t, nil)
	e, _ := empty.Review(ReviewRequest{Scale: "month", Day: "2026-10-15"})
	if !e.Empty || e.EmptyText != "Nothing logged in October yet." {
		t.Errorf("empty %v %q", e.Empty, e.EmptyText)
	}
}

func TestReviewTextFlavorsAndExportRoundTrip(t *testing.T) {
	n := reviewNotes(t, map[string]string{"2026-10-12": logged})
	req := ReviewRequest{Scale: "week", Day: "2026-10-12", IncludeSkipped: true}
	md, err := n.ReviewText(req, "markdown")
	if err != nil || !strings.HasPrefix(md.Text, "# Week of 12 Oct – 18 Oct 2026\n\nLogged 1 of 3") || !strings.HasSuffix(md.Text, "\n") {
		t.Fatalf("markdown %q %v", md.Text, err)
	}
	plain, _ := n.ReviewText(req, "plain")
	if strings.Contains(plain.Text, "#") || strings.Contains(plain.Text, "**") {
		t.Errorf("plain text still has marks: %q", plain.Text)
	}
	rich, _ := n.ReviewText(req, "rich")
	if !strings.Contains(rich.HTML, "<h1>") || rich.Text != md.Text {
		t.Errorf("rich %q", rich.HTML)
	}
	if _, err := n.ReviewText(req, "pdf"); err == nil {
		t.Errorf("an unknown flavor should be refused")
	}

	info, _ := n.ReviewExportInfo(req)
	if info.Name != "Gloamlog 2026-W42" || info.Days != 1 || info.Detail != "1 day. Skipped days included. Jots left out." {
		t.Errorf("info %+v", info)
	}
	out := t.TempDir()
	saved, err := n.ReviewWrite(req, "folder", out)
	if err != nil || saved.Line != "Saved 1 day." || saved.Path != filepath.Join(out, "Gloamlog 2026-W42") {
		t.Fatalf("folder export %+v %v", saved, err)
	}
	if _, err := os.Stat(filepath.Join(saved.Path, "Gloamlog 2026-W42.md")); err != nil {
		t.Error(err)
	}
	file := filepath.Join(out, "my review.md")
	if saved, err = n.ReviewWrite(req, "markdown", file); err != nil || saved.Path != file {
		t.Fatalf("file export %+v %v", saved, err)
	}
	if _, err := n.ReviewWrite(req, "pdf", file); err == nil {
		t.Errorf("an unknown kind should be refused")
	}
}

func TestCleanHeading(t *testing.T) {
	for in, want := range map[string]string{"✅ Finished": "Finished", "Other notes": "Other notes", "🚧": "🚧", "## 2 Do": "2 Do"} {
		if got := cleanHeading(in); got != want {
			t.Errorf("cleanHeading(%q) = %q, want %q", in, got, want)
		}
	}
}
