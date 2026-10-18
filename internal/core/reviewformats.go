// reviewformats.go - the words of a review: titles, file names, the counts line, and the Markdown / Plain text / HTML writers.
// Port of app/Core/Review/ReviewFormats.swift.
//
//	ReviewTitle(p, cal, dash)    "Week of 5 to 11 Oct 2026", "September 2026", "2026", "5 Sep to 12 Oct 2026"
//	                             (dash: the Markdown heading of a week, "Week of 5 Oct – 11 Oct 2026", as v0.3 copied it)
//	ReviewFileName(p, cal)       "Gloamlog 2026-W41", "Gloamlog 2026-10", "Gloamlog 2026", "Gloamlog 2026-09-05 to 2026-10-12"
//	ReviewCountsLine(c)          "Logged 4 of 5 · 1 skipped · 2 not logged" (never "missed"); ok=false when nothing counts
//	ReviewTasksLine(t)           "12 tasks done · 3 still open"; ok=false when there are none
//	ReviewRowLine(r)             "20 logged · 1 skipped · 2 not logged", or a dash for a month that has not started
//	ReviewMarkdown(s, cal)       by Day: "## Mon 5 Oct" + the page; by Section: "## Finished" + "- **Mon 5 Oct**: text"; a Year: one
//	                             bullet per month. The week's Markdown is the v0.3 copy plus the options.
//	ReviewPlain(s, cal)          the same without markup, for chat boxes (the "Copy as plain text" text)
//	ReviewHTML(s, cal, images)   a printable page (PDF); images answers a data: URI for an image path, or ok=false for alt text
//	ReviewRichHTML(s, cal)       a page for the pasteboard (alt text for images)
//
// The pasteboard itself (RTF, .html and plain flavours of a rich copy) is the desktop shell's job; these only produce the text.
package core

import (
	"fmt"
	"strings"
)

// pfx is Swift's String.prefix(n): the whole string when it is shorter.
func pfx(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// Plural is "1 task" / "2 tasks".
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func reviewSpan(a, b string, cal Calendar) string {
	loc := cal.location()
	if a == b {
		return DayKeyFormat(a, "d MMM yyyy", loc)
	}
	sameMonth, sameYear := pfx(a, 7) == pfx(b, 7), pfx(a, 4) == pfx(b, 4)
	pattern := "d MMM yyyy"
	switch {
	case sameMonth:
		pattern = "d"
	case sameYear:
		pattern = "d MMM"
	}
	return DayKeyFormat(a, pattern, loc) + " to " + DayKeyFormat(b, "d MMM yyyy", loc)
}

// ReviewTitle is the period's name. dash selects the Markdown heading of a week ("Week of 5 Oct – 11 Oct 2026").
func ReviewTitle(p ReviewPeriod, cal Calendar, dash bool) string {
	loc := cal.location()
	switch p.Scale {
	case ScaleWeek:
		if dash {
			return "Week of " + DayKeyFormat(p.Start, "d MMM", loc) + " – " + DayKeyFormat(p.End, "d MMM yyyy", loc)
		}
		return "Week of " + reviewSpan(p.Start, p.End, cal)
	case ScaleMonth:
		return DayKeyFormat(p.Start, "MMMM yyyy", loc)
	case ScaleYear:
		return pfx(p.Start, 4)
	}
	return reviewSpan(p.Start, p.End, cal)
}

// ReviewFileName is the file or folder name (without ".md") an export of the period gets.
func ReviewFileName(p ReviewPeriod, cal Calendar) string {
	loc := cal.location()
	switch p.Scale {
	case ScaleWeek:
		// the week number is the ISO one whatever day the week starts on; the middle day of the week decides which week it is
		mid, ok := DayKeyAdding(p.Start, 3, loc)
		d, ok2 := DayKeyDate(mid, loc)
		if !ok || !ok2 {
			return "Gloamlog " + p.Start
		}
		y, w := d.In(loc).ISOWeek()
		return fmt.Sprintf("Gloamlog %04d-W%02d", y, w)
	case ScaleMonth:
		return "Gloamlog " + pfx(p.Start, 7)
	case ScaleYear:
		return "Gloamlog " + pfx(p.Start, 4)
	}
	return "Gloamlog " + p.Start + " to " + p.End
}

// ReviewCountsLine is "Logged 4 of 5 · 1 skipped · 2 not logged"; ok is false when no day counts.
func ReviewCountsLine(c ReviewCounts) (string, bool) {
	if c.Total() <= 0 {
		return "", false
	}
	s := fmt.Sprintf("Logged %d of %d", c.Logged, c.Total())
	if c.Skipped > 0 {
		s += fmt.Sprintf(" · %d skipped", c.Skipped)
	}
	if c.NotWritten() > 0 {
		s += fmt.Sprintf(" · %d not logged", c.NotWritten())
	}
	return s, true
}

// ReviewTasksLine is "12 tasks done · 3 still open"; ok is false when there are no tasks.
func ReviewTasksLine(t TaskStats) (string, bool) {
	if len(t.Completed) == 0 && len(t.OpenAtEnd) == 0 {
		return "", false
	}
	var parts []string
	if len(t.Completed) > 0 {
		parts = append(parts, Plural(len(t.Completed), "task")+" done")
	}
	if len(t.OpenAtEnd) > 0 {
		parts = append(parts, fmt.Sprintf("%d still open", len(t.OpenAtEnd)))
	}
	return strings.Join(parts, " · "), true
}

// ReviewRowLine is a month row as one line: "20 logged · 1 skipped · 2 not logged", or a dash for a month that has not started.
func ReviewRowLine(r MonthRow) string {
	if r.IsFuture {
		return "–"
	}
	c := r.Counts
	parts := []string{fmt.Sprintf("%d logged", c.Logged)}
	if c.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", c.Skipped))
	}
	if c.NotWritten() > 0 {
		parts = append(parts, fmt.Sprintf("%d not logged", c.NotWritten()))
	}
	return strings.Join(parts, " · ")
}

// ReviewMarkdown is the review as markdown: what "Copy" and "Save as Markdown" write, and what the other formats start from.
func ReviewMarkdown(s ReviewSummary, cal Calendar) string {
	loc := cal.location()
	short := func(k string) string { return DayKeyFormat(k, "EEE d MMM", loc) }
	out := "# " + ReviewTitle(s.Period, cal, true) + "\n\n"
	if c, ok := ReviewCountsLine(s.Counts); ok {
		out += c + "\n\n"
	}
	if t, ok := ReviewTasksLine(s.Tasks); ok {
		out += t + "\n\n"
	}
	if s.Period.Scale == ScaleYear {
		n := 0
		for _, r := range s.Months {
			if !r.IsFuture && r.Counts.Total() > 0 {
				out += "- **" + DayKeyFormat(r.Month+"-01", "MMMM", loc) + "**: " + ReviewRowLine(r) + "\n"
				n++
			}
		}
		if n == 0 {
			return out + "_Nothing written in this period._\n"
		}
		return trimmed(out) + "\n"
	}
	if len(s.Days) == 0 {
		return out + "_Nothing written in this period._\n"
	}
	switch s.Options.Grouping {
	case WeekByDay:
		for _, d := range s.Days {
			if d.IsSkipped() {
				out += "## " + short(d.Day) + "\n_Skipped"
				if d.SkipReason != "" {
					out += ": " + d.SkipReason
				}
				out += "_\n\n"
			} else {
				out += "## " + short(d.Day) + "\n\n" + ShiftHeadings(d.Body, 3) + "\n\n"
			}
		}
	case WeekBySection:
		for _, g := range s.Groups {
			out += "## " + g.Title + "\n\n"
			for _, it := range g.Items {
				out += WeekItemMarkdown(short(it.Day), it.Text)
			}
			out += "\n"
		}
	}
	return trimmed(out) + "\n"
}

// ReviewPlain is the review without markup, for chat boxes.
func ReviewPlain(s ReviewSummary, cal Calendar) string { return MDPlain(ReviewMarkdown(s, cal)) }

// ReviewHTML is a printable page (the PDF). images may be nil.
func ReviewHTML(s ReviewSummary, cal Calendar, images MDImages) string {
	return MDDocument(ReviewTitle(s.Period, cal, false), ReviewMarkdown(s, cal), images)
}

// ReviewRichHTML is the page for the pasteboard (alt text for images).
func ReviewRichHTML(s ReviewSummary, cal Calendar) string { return MDFragment(ReviewMarkdown(s, cal)) }
