package core

import (
	"fmt"
	"regexp"
)

// A day's commits in one project are one line on a list page: "48 commits in daily-log · latest: Add CSV export `1a2b3c4`"
// (the owner's choice on 2026-10-19: a busy day has dozens of commits, and one line each flooded the page). The line is
// timed at the latest commit; a newer count replaces the older line wherever it is, so the page keeps one line per project.

var commitsLineRE = regexp.MustCompile("^\\d+ commits? in (.+?) · latest: ")

// CommitsLine is the line for count commits in project; latest is the newest commit as it reads (its subject and hash).
func CommitsLine(count int, project, latest string) string {
	word := "commits"
	if count == 1 {
		word = "commit"
	}
	return fmt.Sprintf("%d %s in %s · latest: %s", count, word, project, latest)
}

// CommitsLineProject is the project a commits line names; "" for any other text.
func CommitsLineProject(text string) string {
	if m := commitsLineRE.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// WithoutCommitsLines drops project's commits lines from any list. A dropped line's blank lines go to the next line of its
// list, so a heading keeps its gap.
func WithoutCommitsLines(lines []LogLine, project string) []LogLine {
	out := make([]LogLine, 0, len(lines))
	gap, gapSection := 0, -1
	for _, l := range lines {
		if !l.Other && CommitsLineProject(l.Text) == project {
			if l.Blank > gap || gapSection != l.Section {
				gap, gapSection = l.Blank, l.Section
			}
			continue
		}
		if gapSection == l.Section && gap > l.Blank {
			l.Blank = gap
		}
		gap, gapSection = 0, -1
		out = append(out, l)
	}
	return out
}
