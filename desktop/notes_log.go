package main

// The list page's editor (frontend/src/log) asks Go for every rule about the file (internal/core/loglist.go): a page is
// parsed and written here, never in TypeScript, so a line on disk is always what core writes. The screen holds the lines
// while they are edited and sends them back to be written out.

import "github.com/dasariprashant0/gloamlog/internal/core"

// LogParsed is a body split around its Log section; OK is false for a classic page.
type LogParsed struct {
	Page core.LogPage `json:"page"`
	OK   bool         `json:"ok"`
}

// LogParse splits body for the list editor.
func (n *Notes) LogParse(body string) LogParsed {
	p, ok := core.ParseLog(body)
	return LogParsed{Page: p, OK: ok}
}

// LogRendered is a list page written out: the markdown to save, its words (what "logged" counts), and each line's text as
// HTML for display (escaped; images through /dlasset/, like the review).
type LogRendered struct {
	Markdown string   `json:"markdown"`
	Words    int      `json:"words"`
	HTML     []string `json:"html"`
}

// LogRender writes page out.
func (n *Notes) LogRender(page core.LogPage) LogRendered {
	n.mu.Lock()
	images := n.assetImages()
	n.mu.Unlock()
	md := page.Markdown()
	out := LogRendered{Markdown: md, Words: core.DayDocument{Body: md}.Words(), HTML: make([]string, len(page.Lines))}
	for i, l := range page.Lines {
		out.HTML[i] = core.MDBody(l.Text, images)
	}
	return out
}

// LogAppended is the page after LogAppendLines: its lines, the page written out, and "appended", or "alreadyPresent" when
// every line was there already (the same words the Milkdown editor answers with).
type LogAppended struct {
	Lines    []core.LogLine `json:"lines"`
	Rendered LogRendered    `json:"rendered"`
	Result   string         `json:"result"`
}

// LogAppendLines adds the bullets of markdown (a jot as capture renders it, or the suggestions strip's lines) to list
// (core.ListWork / core.ListPersonal; a one-list page takes them in its list): at its end, or each by its time when timed (a
// commit: core.InsertTimed). A line whose text is already a line in any list is skipped. Used while the list editor has the
// page open. The lines come back list by list, in page order.
func (n *Notes) LogAppendLines(page core.LogPage, markdown, list string, timed bool) LogAppended {
	add, _ := core.ParseLog("## " + core.LogHeading + "\n" + markdown)
	if timed {
		for _, l := range add.Lines {
			if project := core.CommitsLineProject(l.Text); project != "" {
				page.Lines = core.WithoutCommitsLines(page.Lines, project) // a newer commit count replaces the line (core/commitsline.go)
			}
		}
	}
	section := page.Target(list)
	have := map[string]bool{}
	var mine []core.LogLine // the target list's lines
	for _, l := range page.Lines {
		have[core.ItemKey(l.Text)] = true
		if l.Section == section {
			mine = append(mine, l)
		}
	}
	added := 0
	for _, l := range add.Lines {
		key := core.ItemKey(l.Text)
		if l.Other || key == "" || have[key] {
			continue
		}
		l.Blank, l.Section = 0, section
		switch {
		case timed:
			mine = core.InsertTimed(mine, l)
		case len(mine) == 0:
			l.Blank = 1 // a blank line between the heading and the first line
			mine = append(mine, l)
		default:
			mine = append(mine, l)
		}
		have[key] = true
		added++
	}
	lists := max(len(page.Sections), 1)
	var lines []core.LogLine
	for s := 0; s < lists; s++ {
		if s == section {
			lines = append(lines, mine...)
			continue
		}
		for _, l := range page.Lines {
			if l.Section == s {
				lines = append(lines, l)
			}
		}
	}
	if lines == nil {
		lines = []core.LogLine{}
	}
	page.Lines = lines
	res := LogAppended{Lines: lines, Rendered: n.LogRender(page), Result: "appended"}
	if added == 0 {
		res.Result = "alreadyPresent"
	}
	return res
}
