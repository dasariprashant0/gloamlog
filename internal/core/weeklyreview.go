// weeklyreview.go - one week's aggregate and "Copy week as markdown". Port of app/Core/WeeklyReview.swift.
//
// API:
//
//	SummarizeWeek(weekContaining, documents, states, settings, cal, now) WeekSummary
//	    documents = map[dayKey]DayDocument (LogStore.Documents(from, to)); states = LogStore.FileStates(minWords)
//	ShiftWeek(date, weeks, cal) time.Time          prev/next week (UI disables future)
//	WeekMarkdown(w, grouping, cal) string          grouping WeekByDay | WeekBySection
//	WeekItemMarkdown(label, text) string           one "- **Mon 5 Oct**: text" entry of a by-section list (also used by Review/ReviewFormats)
//	WeekSummary: WeekStart, WeekEnd, Days []WeekDay{Day, Status, Doc}, LoggedCount, WorkdayCount (scheduled days not in the
//	             future), SkippedCount, MissingDays, Sections []WeekSection{Title, IsOther, Items []WeekItem{Day, Text}},
//	             Section(titled) *WeekSection
//	Days always holds all 7 days of the week (weeks with no file included); statuses come from ResolveStatus with
//	since = EffectiveSince(settings.LogStartDate, states), so days before the log start are off and the current
//	week's later days are future. MissingDays = the week's scheduled days strictly before today, on or after the log start, that
//	are missed or partial (oldest first): exactly CatchUpMissing restricted to the week (same rule, IsMissingDay), and
//	empty while there is no log start and no page at all. The week's days a UI should list are the scheduled ones plus any day with a page.
//
// By section: for every heading of settings.Template (template order) the text under matching headings across the week
// (matched by normalised title; nested sub-headings stay inside, shown as bold labels), then "Other notes" = everything else
// (text before the first heading, headings not in the template). Empty groups are left out.
// By day: each non-empty page with headings re-levelled under "## Mon 5 Oct" (shallowest heading becomes ###) and empty
// sections removed. Skipped days print "_Skipped: reason_". Unlogged days are omitted (never listed as missed).
// Weeks start on cal.FirstWeekday.
package core

import (
	"sort"
	"strings"
	"time"
)

// WeekDay is one day of a week summary. Doc is nil when the day has no file.
type WeekDay struct {
	Day    string       `json:"day"`
	Status DayStatus    `json:"status"`
	Doc    *DayDocument `json:"doc"`
}

// WeekItem is the text a day wrote under one heading.
type WeekItem struct {
	Day  string `json:"day"`
	Text string `json:"text"`
}

// WeekSection is one heading of the template (or "Other notes") with what each day wrote under it.
type WeekSection struct {
	Title   string     `json:"title"`
	IsOther bool       `json:"isOther"`
	Items   []WeekItem `json:"items"`
}

// WeekSummary is one week's aggregate.
type WeekSummary struct {
	WeekStart    string        `json:"weekStart"`
	WeekEnd      string        `json:"weekEnd"`
	Days         []WeekDay     `json:"days"`
	Sections     []WeekSection `json:"sections"`
	LoggedCount  int           `json:"loggedCount"`
	WorkdayCount int           `json:"workdayCount"`
	SkippedCount int           `json:"skippedCount"`
	MissingDays  []string      `json:"missingDays"`
}

// Section is the first group whose title matches under NormalizeHeading ("finished" finds "✅ Finished"; "other notes" finds the rest).
func (w WeekSummary) Section(titled string) *WeekSection {
	n := NormalizeHeading(titled)
	for i := range w.Sections {
		if NormalizeHeading(w.Sections[i].Title) == n {
			return &w.Sections[i]
		}
	}
	return nil
}

// WeekGrouping chooses how WeekMarkdown lays the week out.
type WeekGrouping int

const (
	WeekByDay WeekGrouping = iota
	WeekBySection
)

// WeekOtherTitle is the title of the group that holds everything that is not under a template heading.
const WeekOtherTitle = "Other notes"

// ShiftWeek moves date by `weeks` weeks (negative = back), keeping the time of day.
func ShiftWeek(date time.Time, weeks int, cal Calendar) time.Time { return cal.ShiftWeeks(date, weeks) }

// SummarizeWeek aggregates the week that contains weekContaining.
func SummarizeWeek(weekContaining time.Time, documents map[string]DayDocument, states map[string]DayStatus,
	settings Settings, cal Calendar, now time.Time) WeekSummary {
	loc := cal.location()
	start := cal.StartOfWeek(weekContaining)
	today := DayKeyString(now, loc)
	since := EffectiveSince(settings.LogStartDate, states) // days before the log start are off, never missed
	var days []WeekDay
	var missing []string
	logged, work, skipped := 0, 0, 0
	for i := 0; i < 7; i++ { // all 7 days of the week, whatever the number of files (none is fine)
		d := addDays(start, i, loc)
		k := DayKeyString(d, loc)
		weekday := weekdayOf(d, loc)
		st := ResolveStatus(k, states[k], now, cal, settings.Weekdays, since)
		wd := WeekDay{Day: k, Status: st}
		if doc, ok := documents[k]; ok {
			docCopy := doc
			wd.Doc = &docCopy
		}
		days = append(days, wd)
		if st == StatusLogged {
			logged++
		}
		if st == StatusSkipped {
			skipped++
		}
		if st != StatusFuture && settings.Weekdays.Has(weekday) {
			work++
		}
		if IsMissingDay(k, weekday, st, today, since, settings.Weekdays) {
			missing = append(missing, k)
		}
	}

	template := TemplateHeadings(settings.Template)
	norm := make([]string, len(template))
	for i, t := range template {
		norm[i] = NormalizeHeading(t)
	}
	buckets := make([][]WeekItem, len(template))
	var other []WeekItem
	for _, wd := range days {
		if wd.Doc == nil || wd.Doc.IsSkipped {
			continue
		}
		matched, rest := PartitionBody(wd.Doc.Body, norm, SubheadingBoldLabel)
		per := map[int][]string{}
		for _, m := range matched {
			per[m.Index] = append(per[m.Index], m.Text)
		}
		var keys []int
		for k := range per {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, i := range keys {
			buckets[i] = append(buckets[i], WeekItem{Day: wd.Day, Text: strings.Join(per[i], "\n\n")})
		}
		if rest != "" {
			other = append(other, WeekItem{Day: wd.Day, Text: rest})
		}
	}
	var sections []WeekSection
	for i := range template {
		if len(buckets[i]) > 0 {
			sections = append(sections, WeekSection{Title: template[i], Items: buckets[i]})
		}
	}
	if len(other) > 0 {
		sections = append(sections, WeekSection{Title: WeekOtherTitle, IsOther: true, Items: other})
	}

	sum := WeekSummary{Days: days, Sections: sections, LoggedCount: logged, WorkdayCount: work, SkippedCount: skipped, MissingDays: missing}
	if len(days) > 0 {
		sum.WeekStart, sum.WeekEnd = days[0].Day, days[len(days)-1].Day
	}
	return sum
}

// WeekMarkdown is the "Copy week as markdown" text.
func WeekMarkdown(w WeekSummary, grouping WeekGrouping, cal Calendar) string {
	loc := cal.location()
	short := func(k string) string { return DayKeyFormat(k, "EEE d MMM", loc) }
	var out strings.Builder
	out.WriteString("# Week of " + DayKeyFormat(w.WeekStart, "d MMM", loc) + " – " + DayKeyFormat(w.WeekEnd, "d MMM yyyy", loc) + "\n\n")
	out.WriteString("Logged " + itoa(w.LoggedCount) + " of " + itoa(w.WorkdayCount) + " workdays")
	if w.SkippedCount > 0 {
		out.WriteString(" · " + itoa(w.SkippedCount) + " skipped")
	}
	out.WriteString("\n\n")
	switch grouping {
	case WeekByDay:
		for _, d := range w.Days {
			if d.Doc == nil {
				continue
			}
			if d.Doc.IsSkipped {
				out.WriteString("## " + short(d.Day) + "\n_Skipped")
				if d.Doc.SkipReason != "" {
					out.WriteString(": " + d.Doc.SkipReason)
				}
				out.WriteString("_\n\n")
				continue
			}
			body := ShiftHeadings(PruneEmptySections(d.Doc.Body), 3)
			if HasContent(body) {
				out.WriteString("## " + short(d.Day) + "\n\n" + body + "\n\n")
			}
		}
	case WeekBySection:
		for _, s := range w.Sections {
			out.WriteString("## " + s.Title + "\n\n")
			for _, it := range s.Items {
				out.WriteString(WeekItemMarkdown(short(it.Day), it.Text))
			}
			out.WriteString("\n")
		}
	}
	return trimmed(out.String()) + "\n"
}

// WeekItemMarkdown is one entry of a "by section" list: "- **Mon 5 Oct**: first line", then the other lines indented
// (also used by the Review screen's copies).
func WeekItemMarkdown(label, text string) string {
	lines := strings.Split(text, "\n")
	first := lines[0]
	firstText, _ := StripMarkers(first)
	plain := firstText == trimmed(first) && !strings.HasPrefix(first, "```") &&
		!(len(lines) > 1 && strings.HasPrefix(first, "**") && strings.HasSuffix(first, "**"))
	var out strings.Builder
	out.WriteString("- **" + label + "**")
	rest := lines
	if plain {
		out.WriteString(": " + first + "\n")
		rest = lines[1:]
	} else {
		out.WriteString("\n")
	}
	for _, l := range rest {
		if l == "" {
			out.WriteString("\n")
		} else {
			out.WriteString("  " + l + "\n")
		}
	}
	return out.String()
}
