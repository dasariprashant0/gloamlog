// catchup.go - which scheduled days still need writing, and batch skipping. Port of app/Core/Calendar/CatchUp.swift.
// CONTRACT:
//
//	CatchUpMissing(...)  scheduled days in the last windowDays days BEFORE today, oldest first, whose state is missed or partial.
//	              Skipped, logged, off, future and today are never listed; days before the effective log start are never listed.
//	              windowDays is clamped to CatchUpWindowMin...CatchUpWindowMax (7...365): a day exactly windowDays back is in, one further is out.
//	              With no log start and no page at all there is nothing to catch up on (a new user's days before the first page don't count).
//	CatchUpNext(after, list)  first entry strictly after `after` (the first entry when after is ""); ok false when there is none.
//	LogStore.SkipDays / UnskipDays: one skip marker per day; both return the days actually changed, in the order given, each day once.
//
// SkipDays rules (the existing LogStore.Skip rules, applied per day):
//   - a day with writing is left alone, not an error; a day that is already a skip marker is left alone too (any reason), so the
//     returned list is exactly what Undo may remove and Undo can never delete a marker this call did not create;
//   - a page with no writing (headings only) is replaced by the marker; undo then leaves no file for that day (it carried nothing);
//   - all malformed days are rejected (ErrBadDay) before anything is written; if a write fails midway the markers this call already
//     wrote are removed again and the error is returned, so a failed call changes nothing.
//
// UnskipDays removes a marker only while the file is still a pure skip marker (a day the user wrote into since, or a marker someone
// added notes to, is left alone). A skip followed by undo leaves the folder byte-identical for days that had no page.
package core

import (
	"errors"
	"time"
)

// IsMissingDay is the one rule behind the Catch up list, the sidebar count and WeekSummary.MissingDays: a scheduled day, strictly
// before today and on or after the log start, that is unwritten (missed) or only started (partial). status is the day's resolved
// status; since "" = no log start.
func IsMissingDay(day string, weekday int, status DayStatus, today, since string, weekdays WeekdaySet) bool {
	if since == "" || day < since || day >= today || !weekdays.Has(weekday) {
		return false
	}
	return status == StatusMissed || status == StatusPartial
}

// CatchUpMissing lists the days that still need writing, oldest first. logStart "" = none.
func CatchUpMissing(states map[string]DayStatus, now time.Time, cal Calendar, weekdays WeekdaySet, logStart string, windowDays int) []string {
	loc := cal.location()
	today := DayKeyString(now, loc)
	since := EffectiveSince(logStart, states)
	date, ok := DayKeyDate(today, loc)
	if since == "" || !ok {
		return nil
	}
	window := clampInt(windowDays, CatchUpWindowMin, CatchUpWindowMax)
	var newestFirst []string
	for i := 0; i < window; i++ { // yesterday, then one day further back each step
		date = addDays(date, -1, loc)
		key := DayKeyString(date, loc)
		if key < since { // nothing earlier counts
			break
		}
		status := ResolveStatus(key, states[key], now, cal, weekdays, since)
		if IsMissingDay(key, weekdayOf(date, loc), status, today, since, weekdays) {
			newestFirst = append(newestFirst, key)
		}
	}
	for i, j := 0, len(newestFirst)-1; i < j; i, j = i+1, j-1 {
		newestFirst[i], newestFirst[j] = newestFirst[j], newestFirst[i]
	}
	return newestFirst
}

// CatchUpNext is the first entry of list strictly after `after`; after "" gives the first entry.
func CatchUpNext(after string, list []string) (string, bool) {
	if after == "" {
		if len(list) == 0 {
			return "", false
		}
		return list[0], true
	}
	for _, d := range list {
		if d > after {
			return d, true
		}
	}
	return "", false
}

// SkipDays writes one skip marker per day and returns the days it changed (see the header for the rules).
func (s *LogStore) SkipDays(days []string, reason string) ([]string, error) {
	list, err := uniqueDays(days)
	if err != nil {
		return nil, err
	}
	var done []string
	rollback := func(err error) ([]string, error) {
		for _, d := range done {
			_, _ = s.Unskip(d) // a failed call changes nothing
		}
		return nil, err
	}
	for _, d := range list {
		page, err := s.Load(d)
		if err != nil {
			return rollback(err)
		}
		if page != nil && page.IsSkipped {
			continue
		}
		if err := s.Skip(d, reason); err != nil {
			if errors.Is(err, ErrHasContent) {
				continue
			}
			return rollback(err)
		}
		done = append(done, d)
	}
	return done, nil
}

// UnskipDays removes the skip markers of the given days (only pure markers) and returns the days it restored.
func (s *LogStore) UnskipDays(days []string) ([]string, error) {
	list, err := uniqueDays(days)
	if err != nil {
		return nil, err
	}
	var restored []string
	for _, d := range list {
		ok, err := s.Unskip(d)
		if err != nil {
			return restored, err
		}
		if ok {
			restored = append(restored, d)
		}
	}
	return restored, nil
}

// uniqueDays is days without repeats, in order; it fails with ErrBadDay for the first malformed entry before anything is touched.
func uniqueDays(days []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, d := range days {
		if !DayKeyIsWellFormed(d) {
			return nil, logErr(ErrBadDay, d)
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}
