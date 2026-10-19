package core

import "testing"

func TestCommitsLine(t *testing.T) {
	expect(t, CommitsLine(1, "acme", "Fix `1a2b3c4`") == "1 commit in acme · latest: Fix `1a2b3c4`")
	expect(t, CommitsLine(48, "daily-log", "Add `5d6e7f8`") == "48 commits in daily-log · latest: Add `5d6e7f8`")
	expect(t, CommitsLineProject(CommitsLine(3, "my project", "x `abc1234`")) == "my project")
	expect(t, CommitsLineProject("Fixed 3 commits in acme") == "")
}

func TestATimedCommitsLineReplacesTheProjectsOldOne(t *testing.T) {
	item := func(text, stamp string) CaptureItem {
		return CaptureItem{Text: text, Stamp: stamp, Timed: true, List: ListPersonal}
	}
	body := "## Work\n\n- 09:00 review\n\n## Personal\n\n- 10:00 2 commits in acme · latest: A `1111111`\n- 11:00 run\n- 12:00 1 commit in other · latest: B `2222222`"
	got := ApplyJot(item("3 commits in acme · latest: C `3333333`", "13:00"), body, "Jots")
	want := "## Work\n\n- 09:00 review\n\n## Personal\n\n- 11:00 run\n- 12:00 1 commit in other · latest: B `2222222`\n- 13:00 3 commits in acme · latest: C `3333333`"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}
