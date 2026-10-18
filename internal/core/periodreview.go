// periodreview.go - one builder for Week, Month, Year and a custom Range. Pure: documents and states come in, a summary goes out.
// Port of app/Core/Review/PeriodReview.swift.
//
//	ReviewPeriodContaining(scale, anchor, cal)   the week (cal.FirstWeekday), month or year around a date
//	ReviewRange(from, to, cal)                   either order; at most ReviewMaxRangeDays (the end is kept)
//	ShiftReview(p, n, cal)                       the previous / next period of the same size; ok=false for a malformed period
//	BuildReview(period, documents, states, settings, options, cal, now) ReviewSummary
//	                                             documents = LogStore.Documents(from, to) (not needed for a Year); states = LogStore.FileStates
//	SplitJots(body, heading)                     the page without its Jots blocks, and what was in them
//
// Counts use the same rule as the week screen and Catch up (ResolveStatus + IsMissingDay): a day is "not logged" only when it is
// scheduled, strictly before today and on or after the log start. Skipped days are counted only while options.IncludeSkipped is on.
// Content: each page is cut by PartitionBody into the template headings (template order), then Jots (when included), then "Other
// notes", then the skipped days as one-line items. The page as shown by day has its Jots block split off first (unless included), its
// empty headings pruned, and is left out when nothing is written. Tasks come from the same text, so they follow the Jots option.
//
// Go vs Swift: the slices of a ReviewSummary are empty, never nil, so the JSON has [] where Swift had an empty array.
package core

import (
	"sort"
	"strings"
	"time"
)

// ReviewMaxRangeDays is the longest custom range (about ten years).
const ReviewMaxRangeDays = 3_700

// ---- periods ----

func civilKey(y int, m time.Month, d int) string {
	return DayKeyString(time.Date(y, m, d, 12, 0, 0, 0, time.UTC), time.UTC) // normalises an overflowing month or day
}

// ReviewPeriodContaining is the period of the given scale around anchor. A range has no natural size: it is the one day.
func ReviewPeriodContaining(scale ReviewScale, anchor time.Time, cal Calendar) ReviewPeriod {
	loc := cal.location()
	k := DayKeyString(anchor, loc)
	y, m, _ := anchor.In(loc).Date()
	switch scale {
	case ScaleWeek:
		start := DayKeyString(cal.StartOfWeek(anchor), loc)
		end, ok := DayKeyAdding(start, 6, loc)
		if !ok {
			end = start
		}
		return ReviewPeriod{Scale: scale, Start: start, End: end}
	case ScaleMonth:
		return ReviewPeriod{Scale: scale, Start: civilKey(y, m, 1), End: civilKey(y, m+1, 0)}
	case ScaleYear:
		return ReviewPeriod{Scale: scale, Start: civilKey(y, 1, 1), End: civilKey(y, 12, 31)}
	}
	return ReviewPeriod{Scale: scale, Start: k, End: k}
}

// ReviewRange is a custom range in either order, capped at ReviewMaxRangeDays (the end is kept).
func ReviewRange(from, to time.Time, cal Calendar) ReviewPeriod {
	loc := cal.location()
	if to.Before(from) {
		from, to = to, from
	}
	a, b := DayKeyString(from, loc), DayKeyString(to, loc)
	if capDay, ok := DayKeyAdding(b, -(ReviewMaxRangeDays - 1), loc); ok && a < capDay {
		a = capDay
	}
	return ReviewPeriod{Scale: ScaleRange, Start: a, End: b}
}

// ReviewDayCount is how many days the period spans (0 for a malformed one).
func ReviewDayCount(p ReviewPeriod, cal Calendar) int {
	loc := cal.location()
	s, ok1 := DayKeyDate(p.Start, loc)
	e, ok2 := DayKeyDate(p.End, loc)
	if !ok1 || !ok2 {
		return 0
	}
	// whole civil days between the two dates: independent of any 23 h or 25 h day in between
	sy, sm, sd := s.In(loc).Date()
	ey, em, ed := e.In(loc).Date()
	su := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)
	eu := time.Date(ey, em, ed, 0, 0, 0, 0, time.UTC)
	return int(eu.Sub(su).Hours()/24) + 1
}

// addMonths is Foundation's date(byAdding: .month): the same day of the month n months on, or the last day when that month is shorter.
func addMonths(t time.Time, n int, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	total := y*12 + int(m) - 1 + n
	ny, nm := total/12, total%12
	if nm < 0 {
		nm += 12
		ny--
	}
	last := time.Date(ny, time.Month(nm+2), 0, 0, 0, 0, 0, time.UTC).Day() // day 0 of the next month
	return startOfDay(ny, time.Month(nm+1), min(d, last), loc)
}

// ShiftReview is the previous (n = -1) or next (n = 1) period of the same size.
func ShiftReview(p ReviewPeriod, n int, cal Calendar) (ReviewPeriod, bool) {
	loc := cal.location()
	s, ok := DayKeyDate(p.Start, loc)
	if _, okEnd := DayKeyDate(p.End, loc); !ok || !okEnd {
		return ReviewPeriod{}, false
	}
	switch p.Scale {
	case ScaleWeek:
		return ReviewPeriodContaining(ScaleWeek, addDays(s, 7*n, loc), cal), true
	case ScaleMonth:
		return ReviewPeriodContaining(ScaleMonth, addMonths(s, n, loc), cal), true
	case ScaleYear:
		return ReviewPeriodContaining(ScaleYear, addMonths(s, 12*n, loc), cal), true
	case ScaleRange:
		length := ReviewDayCount(p, cal)
		a, ok1 := DayKeyAdding(p.Start, n*length, loc)
		b, ok2 := DayKeyAdding(p.End, n*length, loc)
		if !ok1 || !ok2 {
			return ReviewPeriod{}, false
		}
		return ReviewPeriod{Scale: ScaleRange, Start: a, End: b}, true
	}
	return ReviewPeriod{}, false
}

// ForEachDay calls fn with each day key from a to b (inclusive) and its start-of-day instant. Nothing when a > b.
func ForEachDay(a, b string, cal Calendar, fn func(key string, d time.Time)) {
	loc := cal.location()
	d, ok := DayKeyDate(a, loc)
	if a > b || !ok {
		return
	}
	k := a
	for k <= b {
		fn(k, d)
		n := addDays(d, 1, loc)
		nk := DayKeyString(n, loc)
		if nk <= k { // the calendar did not move on: never loop
			return
		}
		d, k = n, nk
	}
}

// ---- Jots ----

// SplitJots is the page without its Jots blocks, and the text inside them (their heading lines dropped).
func SplitJots(body, heading string) (rest, jots string) {
	lines := ScanBody(body)
	blocks := jotsBlocks(lines, heading)
	if len(blocks) == 0 {
		return body, ""
	}
	inBlock := map[int]bool{}
	heads := map[int]bool{}
	for _, b := range blocks {
		heads[b.heading] = true
		for i := b.heading; i < b.end; i++ {
			inBlock[i] = true
		}
	}
	text := func(keep func(i int) bool) string {
		var out []string
		for i, l := range lines {
			if keep(i) {
				out = append(out, l.Text)
			}
		}
		return TrimBody(strings.Join(out, "\n"))
	}
	return text(func(i int) bool { return !inBlock[i] }), text(func(i int) bool { return inBlock[i] && !heads[i] })
}

// ---- build ----

// The list page's review groups, and where the classic default headings go when the template is a list template.
var (
	listGroupTitles = []string{"Done", "Next", "Blocked"}
	classicGroupOf  = map[string]int{"what i did": 0, "finished": 0, "started": 0, "to do next": 1, "pending blocked": 2}
)

func markGroup(m LogMark) int {
	switch m {
	case MarkNext:
		return 1
	case MarkBlocked:
		return 2
	}
	return 0
}

// outsideLog is a list page without its Log section: what Before holds above the heading, and After.
func outsideLog(p LogPage) string {
	before := ""
	if i := strings.LastIndexByte(p.Before, '\n'); i >= 0 {
		before = p.Before[:i]
	}
	return TrimBody(strings.TrimSpace(before + "\n\n" + p.After))
}

// BuildReview turns the day files of a period into a summary (see the header). now decides which days are over.
func BuildReview(p ReviewPeriod, documents map[string]DayDocument, states map[string]DayStatus, settings Settings,
	options ReviewOptions, cal Calendar, now time.Time) ReviewSummary {
	loc := cal.location()
	today := DayKeyString(now, loc)
	since := EffectiveSince(settings.LogStartDate, states)
	summary := ReviewSummary{Period: p, Options: options, Days: []ReviewDay{}, Groups: []ReviewGroup{}, Months: []MonthRow{},
		Tasks: TaskStats{Completed: []ReviewItem{}, OpenAtEnd: []string{}}}

	count := func(a, b string) ReviewCounts {
		var c ReviewCounts
		ForEachDay(a, min(b, today), cal, func(k string, d time.Time) {
			st := ResolveStatus(k, states[k], now, cal, settings.Weekdays, since)
			switch st {
			case StatusLogged:
				c.Logged++
			case StatusSkipped:
				if options.IncludeSkipped {
					c.Skipped++
				}
			case StatusPartial, StatusMissed:
				if IsMissingDay(k, weekdayOf(d, loc), st, today, since, settings.Weekdays) {
					if st == StatusPartial {
						c.Started++
					} else {
						c.Unlogged++
					}
				}
			}
		})
		return c
	}

	if p.Scale == ScaleYear { // counts only: no page is read for a year
		m, ok := DayKeyDate(p.Start, loc)
		if !ok {
			return summary
		}
		for i := 0; i < 12; i++ {
			first := DayKeyString(m, loc)
			next := addMonths(m, 1, loc)
			last, ok := DayKeyAdding(DayKeyString(next, loc), -1, loc)
			if !ok {
				last = first
			}
			counts := count(first, last)
			summary.Months = append(summary.Months, MonthRow{Month: pfx(first, 7), Counts: counts, IsFuture: first > today})
			summary.Counts.add(counts)
			m = next
		}
		return summary
	}
	summary.Counts = count(p.Start, p.End)

	listMode := IsLogPage(settings.Template)
	template := TemplateHeadings(settings.Template)
	if listMode {
		// The five classic headings are always recognised (old pages), then any other heading of the template.
		template = append(TemplateHeadings(ClassicTemplate), template...)
	}
	norm := make([]string, len(template))
	for i, t := range template {
		norm[i] = NormalizeHeading(t)
	}
	// slot is where a template heading's text goes: its own index; in list mode a Done / Next / Blocked group (0-2) for a
	// classic heading and 3+ for any other heading of the template.
	slot := func(i int) int { return i }
	nSlots := len(template)
	var slotTitles []string
	if listMode {
		slotTitles = append([]string{}, listGroupTitles...)
		extra := map[string]int{}
		for _, t := range template {
			n := NormalizeHeading(t)
			if _, classic := classicGroupOf[n]; !classic && n != NormalizeHeading(LogHeading) && extra[n] == 0 {
				slotTitles = append(slotTitles, t)
				extra[n] = len(slotTitles) - 1 // 3 or more, so 0 means "not seen"
			}
		}
		slot = func(i int) int {
			if g, ok := classicGroupOf[norm[i]]; ok {
				return g
			}
			if s, ok := extra[norm[i]]; ok {
				return s
			}
			return -1 // the Log heading itself: never matched on a classic page
		}
		nSlots = len(slotTitles)
	}
	buckets := make([][]ReviewItem, nSlots)
	var other, jots, skipped []ReviewItem
	seenDone := map[string]bool{}
	lastLoggedBody, haveLogged := "", false

	keys := make([]string, 0, len(documents))
	for k := range documents {
		if k >= p.Start && k <= p.End {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		doc := documents[k]
		if doc.IsSkipped {
			if !options.IncludeSkipped {
				continue
			}
			summary.Days = append(summary.Days, ReviewDay{Day: k, Status: StatusSkipped, SkipReason: doc.SkipReason})
			reason := doc.SkipReason
			if reason == "" {
				reason = "Skipped"
			}
			skipped = append(skipped, ReviewItem{Day: k, Text: reason})
			continue
		}
		rest, jotText := SplitJots(doc.Body, settings.Capture.JotsHeading)
		shown := rest
		if options.IncludeJots {
			shown = doc.Body
		}
		if !HasContent(shown) {
			continue
		}
		status, ok := states[k]
		if !ok {
			status = doc.Status(settings.MinWords)
		}
		summary.Days = append(summary.Days, ReviewDay{Day: k, Status: status, Words: doc.Words(), Body: PruneEmptySections(shown)})
		if status == StatusLogged {
			lastLoggedBody, haveLogged = shown, true
		}

		var otherText string
		per := map[int][]string{}
		if page, ok := ParseLog(rest); ok && listMode {
			marked := map[int][]string{}
			for _, l := range page.Lines {
				g := markGroup(l.Mark)
				marked[g] = append(marked[g], l.Plain())
			}
			for g, texts := range marked {
				per[g] = []string{strings.Join(texts, "\n")} // one list per group per day
			}
			otherText = outsideLog(page)
		} else {
			var matched []PartitionMatch
			matched, otherText = PartitionBody(rest, norm, SubheadingBoldLabel)
			for _, m := range matched {
				if s := slot(m.Index); s >= 0 {
					per[s] = append(per[s], m.Text)
				}
			}
		}
		var idx []int
		for i := range per {
			idx = append(idx, i)
		}
		sort.Ints(idx)
		for _, i := range idx {
			buckets[i] = append(buckets[i], ReviewItem{Day: k, Text: strings.Join(per[i], "\n\n")})
		}
		if otherText != "" {
			other = append(other, ReviewItem{Day: k, Text: otherText})
		}
		if options.IncludeJots && HasContent(jotText) {
			jots = append(jots, ReviewItem{Day: k, Text: jotText})
		}

		for _, l := range ContentLines(shown) {
			if l.IsCode || l.Task != TaskDone {
				continue
			}
			text := Collapse(RemoveNoise(l.Text))
			if text == "" {
				continue
			}
			if key := ItemKey(text); !seenDone[key] {
				seenDone[key] = true
				summary.Tasks.Completed = append(summary.Tasks.Completed, ReviewItem{Day: k, Text: text})
			}
		}
	}
	if haveLogged {
		if items, _ := OpenItems(lastLoggedBody, settings.CarryOverHeadings); len(items) > 0 { // a list page: its open lines
			summary.Tasks.OpenAtEnd = items
		}
	}

	for i, items := range buckets {
		if len(items) == 0 {
			continue
		}
		title := template[i]
		if listMode {
			title = slotTitles[i]
		}
		summary.Groups = append(summary.Groups, ReviewGroup{Title: title, Kind: GroupHeading, Items: items})
	}
	if len(jots) > 0 {
		summary.Groups = append(summary.Groups, ReviewGroup{Title: jotsHeadingText(settings.Capture.JotsHeading), Kind: GroupJots, Items: jots})
	}
	if len(other) > 0 {
		summary.Groups = append(summary.Groups, ReviewGroup{Title: WeekOtherTitle, Kind: GroupOther, Items: other})
	}
	if len(skipped) > 0 {
		summary.Groups = append(summary.Groups, ReviewGroup{Title: "Skipped days", Kind: GroupSkipped, Items: skipped})
	}
	return summary
}
