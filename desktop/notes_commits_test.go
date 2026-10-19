package main

import (
	"testing"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/dasariprashant0/gloamlog/internal/sources"
)

const commitDay = "2026-10-14" // captureNotes' today

// commitNotes is captureNotes with Git on for every project, and a scan that finds these bullets on any day.
func commitNotes(t *testing.T, bullets ...sources.SuggestedBullet) (*Notes, *recorder) {
	t.Helper()
	n, _, _, events := captureNotes(t, true)
	s := n.Settings()
	s.Sources.Git, s.Sources.Consented, s.Sources.AllProjects = true, true, true
	s.Lists = core.ListsPrefs{Work: []string{"/w"}, Personal: []string{"/p"}, Jots: core.ListWork}
	n.apply(s)
	old := scanCommits
	scanCommits = func(_, day, _ string, _ core.Calendar, _ core.SourcesPrefs) sources.ScanResult {
		return sources.ScanResult{Day: day, Bullets: bullets}
	}
	t.Cleanup(func() { scanCommits = old })
	return n, events
}

func commitAt(h, m int, subject, sha string) sources.SuggestedBullet {
	return sources.SuggestedBullet{Kind: sources.KindCommit, Subject: subject, Sha: sha, Time: time.Date(2026, 10, 14, h, m, 0, 0, time.Local)}
}

func dayBody(t *testing.T, n *Notes, day string) string {
	t.Helper()
	d, err := n.store.Load(day)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		return ""
	}
	return d.Body
}

func savePage(t *testing.T, n *Notes, day, body string) {
	t.Helper()
	if _, err := n.store.Save(day, body); err != nil {
		t.Fatal(err)
	}
}

func TestAddCommitsPutsEachAtItsTimeOnce(t *testing.T) {
	n, _ := commitNotes(t,
		commitAt(9, 15, "Add CSV export", "1a2b3c4"),
		commitAt(11, 0, "Fix *the* build", "5d6e7f8"),
		commitAt(9, 30, "a commit without a hash", ""),
		sources.SuggestedBullet{Kind: sources.KindPrompt, Text: "Worked on acme: a prompt", Time: time.Date(2026, 10, 14, 9, 40, 0, 0, time.Local)})
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started\n- 10:00 standup")
	added, err := n.AddCommits(commitDay)
	want := "## Log\n\n- 09:00 started\n- 09:15 Add CSV export `1a2b3c4`\n- 10:00 standup\n- 11:00 Fix \\*the\\* build `5d6e7f8`"
	if body := dayBody(t, n, commitDay); err != nil || added != 2 || body != want {
		t.Fatalf("added %d, err %v, page:\n%s", added, err, body)
	}
	if again, _ := n.AddCommits(commitDay); again != 0 {
		t.Fatalf("a hash already on the page is not added again (%d)", again)
	}
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started") // the person deleted both lines
	if again, _ := n.AddCommits(commitDay); again != 0 || dayBody(t, n, commitDay) != "## Log\n\n- 09:00 started" {
		t.Fatalf("a commit line deleted this session stays deleted: %d, %q", again, dayBody(t, n, commitDay))
	}
}

func TestAddCommitsRoutesEachToItsFoldersList(t *testing.T) {
	work := commitAt(9, 15, "Fix the demo form", "1a2b3c4")
	work.Root = "/w/site"
	home := commitAt(21, 5, "Standup view", "5d6e7f8")
	home.Root = "/p/gloamlog"
	n, _ := commitNotes(t, work, home)
	savePage(t, n, commitDay, "## Work\n\n- 10:00 review\n\n## Personal\n\n- 20:00 run")
	if added, err := n.AddCommits(commitDay); err != nil || added != 2 {
		t.Fatalf("added %d, err %v", added, err)
	}
	want := "## Work\n\n- 09:15 Fix the demo form `1a2b3c4`\n- 10:00 review\n\n## Personal\n\n- 20:00 run\n- 21:05 Standup view `5d6e7f8`"
	if body := dayBody(t, n, commitDay); body != want {
		t.Fatalf("each in its list at its time:\n%s", body)
	}
}

func TestAddCommitsKnowsLinesTheStripInserted(t *testing.T) {
	n, _ := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	savePage(t, n, commitDay, "## Log\n\n- Committed: Add CSV export (acme 1a2b3c4)")
	if added, _ := n.AddCommits(commitDay); added != 0 {
		t.Fatalf("the hash is on the page already (%d)", added)
	}
}

func TestAddCommitsGoThroughAnOpenPageAsTimedJots(t *testing.T) {
	n, events := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started")
	n.EditorOpened(commitDay)
	if added, err := n.AddCommits(commitDay); err != nil || added != 1 {
		t.Fatalf("added %d, err %v", added, err)
	}
	got := events.named("jot-into-page")
	if len(got) != 1 {
		t.Fatalf("jot-into-page events = %v", got)
	}
	ev := got[0].(map[string]any)
	if ev["day"] != commitDay || ev["heading"] != core.LogHeading || ev["markdown"] != "- 09:15 Add CSV export `1a2b3c4`" || ev["timed"] != true {
		t.Errorf("event = %v", ev)
	}
	if body := dayBody(t, n, commitDay); body != "## Log\n\n- 09:00 started" {
		t.Errorf("the file was written under an open page: %q", body)
	}
}

func TestTodaysCommitsArriveByThemselvesOnlyWithAddCommitsForMe(t *testing.T) {
	n, _ := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	savePage(t, n, commitDay, "## Log\n\n- 09:00 started")
	if got := n.autoAddCommits(); got != 0 {
		t.Fatalf("Add commits for me is off by default, yet %d were added", got)
	}
	s := n.Settings()
	s.Sources.AutoAdd = true
	n.apply(s)
	if got := n.autoAddCommits(); got != 1 {
		t.Fatalf("added %d, want 1", got)
	}
}

func TestAddCommitsLeavesClassicPagesAndNeedsTheSources(t *testing.T) {
	n, _ := commitNotes(t, commitAt(9, 15, "Add CSV export", "1a2b3c4"))
	classic := "## What I did\n\nwords"
	savePage(t, n, "2026-10-13", classic)
	if added, err := n.AddCommits("2026-10-13"); err != nil || added != 0 || dayBody(t, n, "2026-10-13") != classic {
		t.Fatalf("a classic page keeps the strip: %d, %v, %q", added, err, dayBody(t, n, "2026-10-13"))
	}
	if _, err := n.AddCommits("2026-10-15"); err == nil {
		t.Fatal("a day after today has no commits to add")
	}
	s := n.Settings()
	s.Sources.Consented = false
	n.apply(s)
	if _, err := n.AddCommits(commitDay); err == nil {
		t.Fatal("with the sources off the button says so")
	}
}
