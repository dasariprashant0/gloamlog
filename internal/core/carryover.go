// carryover.go - "Yesterday" card and carry-over into today's page. Port of app/Core/CarryOver.swift.
//
// API:
//
//	CarryCardBefore(today, cal, settings, load func(day) *DayDocument) *CarryCard
//	    Source = the most recent previous LOGGED day (status logged under settings.MinWords; skipped, missing and
//	    partial days are walked past, 14-day lookback). Items = non-empty lines under that day's
//	    settings.CarryOverHeadings (matched with NormalizeHeading; deeper sub-headings stay included),
//	    list/checkbox markers stripped, ticked tasks ("- [x]") and code/images skipped, de-duplicated.
//	    nil when that day has nothing to carry.
//	    On a list page the items are its open lines (OpenLines) and Marks says which is next or blocked.
//	CarryHeading(card, today, cal) -> "Yesterday" | "Friday" | "2 Oct 2026"
//	ApplyCarryOver(card, body) string
//	    Inserts "## Carried over from Fri 2 Oct" + "- [ ] item" lines at the TOP of `body`. Idempotent: returns `body`
//	    unchanged if that heading (any level) is already there or every item already appears as a line.
//	IsCarryApplied(card, body) bool     true when ApplyCarryOver would change nothing (hide the card)
//	CarryItems(body, headings) []string
//
// The UI keeps the previous body to implement "Undo". Pass the editor's current markdown (template included) to ApplyCarryOver.
package core

import "time"

const (
	// CarryLookbackDays is how many days back the card looks for a logged day.
	CarryLookbackDays = 14
	// CarryHeadingPrefix starts the heading ApplyCarryOver inserts.
	CarryHeadingPrefix = "Carried over from "
)

// CarryCard is the "Yesterday" card: the items of the last logged day that are still open.
type CarryCard struct {
	SourceDay   string    `json:"sourceDay"`
	SourceLabel string    `json:"sourceLabel"` // "Mon 5 Oct"
	Items       []string  `json:"items"`
	Marks       []LogMark `json:"marks"` // list-page source only: Items[i] is Marks[i]; nil for a classic source
}

// HeadingText is the heading ApplyCarryOver inserts: "Carried over from Mon 5 Oct".
func (c CarryCard) HeadingText() string { return CarryHeadingPrefix + c.SourceLabel }

// CarryItems lists the items under the given template headings of a body.
func CarryItems(body string, headings []string) []string {
	var norm []string
	for _, h := range headings {
		if n := NormalizeHeading(h); n != "" {
			norm = append(norm, n)
		}
	}
	if len(norm) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	matched, _ := PartitionBody(body, norm, SubheadingDrop)
	for _, m := range matched {
		for _, l := range ContentLines(m.Text) {
			if l.IsCode || l.Task == TaskDone {
				continue
			}
			text := Collapse(RemoveNoise(l.Text))
			key := ItemKey(text)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, text)
		}
	}
	return out
}

// CarryCardBefore builds the card for `today` from the most recent logged day. load returns nil for a day without a page.
func CarryCardBefore(today string, cal Calendar, settings Settings, load func(day string) *DayDocument) *CarryCard {
	d := today
	for i := 0; i < CarryLookbackDays; i++ {
		p, ok := DayKeyAdding(d, -1, cal.location())
		if !ok {
			return nil
		}
		d = p
		doc := load(d)
		if doc == nil || doc.IsSkipped || doc.Status(settings.MinWords) != StatusLogged {
			continue
		}
		var items []string
		var marks []LogMark
		if IsLogPage(doc.Body) {
			for _, l := range OpenLines(doc.Body) {
				items, marks = append(items, Collapse(l.Text)), append(marks, l.Mark)
			}
		} else {
			items = CarryItems(doc.Body, settings.CarryOverHeadings)
		}
		if len(items) == 0 {
			return nil
		}
		return &CarryCard{SourceDay: d, SourceLabel: DayKeyFormat(d, "EEE d MMM", cal.location()), Items: items, Marks: marks}
	}
	return nil
}

// CarryHeading is the title the UI shows over the card: "Yesterday", the weekday name within a week, else the date.
func CarryHeading(c CarryCard, today string, cal Calendar) string {
	loc := cal.location()
	a, okA := DayKeyDate(c.SourceDay, loc)
	b, okB := DayKeyDate(today, loc)
	if !okA || !okB {
		return c.SourceDay
	}
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	n := int(time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC).Sub(time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)).Hours() / 24) // whole calendar days
	switch {
	case n == 1:
		return "Yesterday"
	case n <= 6:
		return DayKeyFormat(c.SourceDay, "EEEE", loc)
	}
	return DayKeyFormat(c.SourceDay, "d MMM yyyy", loc)
}

// ApplyCarryOver puts the card's items at the top of body under "Carried over from ...".
func ApplyCarryOver(c CarryCard, body string) string {
	return InsertCarryOver(body, c.HeadingText(), c.Items)
}

// IsCarryApplied is true when ApplyCarryOver would change nothing.
func IsCarryApplied(c CarryCard, body string) bool { return ApplyCarryOver(c, body) == body }
