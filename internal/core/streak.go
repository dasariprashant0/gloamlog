// streak.go - day status resolution, streak and 12-week heatmap. Pure; input is LogStore.FileStates(minWords)
// (logged = words >= minWords, partial = 1...minWords-1 words, skipped = marker file; empty pages are absent).
// Port of app/Core/Streak.swift.
//
// API:
//
//	ResolveStatus(day, fileState, now, cal, weekdays, since) DayStatus
//	ComputeStreak(states, now, cal, weekdays, since) StreakResult{Current, Best}
//	HeatmapWeeks(states, now, cal, weekdays, count, since) [][]HeatCell
//	    columns = weeks oldest first; each column = 7 cells in cal.FirstWeekday order.
//
// Rules (UX_SPEC 3.3): logged = +1 (scheduled or not); skipped = neutral; unlogged today = neutral ("not yet");
// unlogged/partial PAST scheduled day = break; unlogged non-scheduled day = neutral. Days before the first file
// ever written are neutral/off (a new user has no "missed" history). Weekday changes re-evaluate all history.
// Day-key arguments that are optional in Swift (String?) are "" here: fileState "" = no file, since "" = no log start.
package core

import "time"

// ResolveStatus is the status of one day. fileState = logged/partial/skipped if the day has a file, else "".
// since = the earliest day that counts (see EffectiveSince), "" for none.
func ResolveStatus(day string, fileState DayStatus, now time.Time, cal Calendar, weekdays WeekdaySet, since string) DayStatus {
	today := DayKeyString(now, cal.location())
	if day > today {
		return StatusFuture
	}
	if fileState != "" {
		return fileState
	}
	// No log start and no page yet (a brand-new install): nothing before today is due, so there is no wall of "missed" days.
	if since == "" {
		since = today
	}
	if day < since {
		return StatusOff
	}
	if weekdays.Has(DayKeyWeekday(day, cal.location())) {
		return StatusMissed
	}
	return StatusOff
}

// EffectiveSince is the day before which nothing counts as missed: the user's log start date, else the earliest day with a file.
// Pinning this (Settings.LogStartDate) is what lets someone backfill an older day without creating a wall of "missed" days.
// "" when there is neither.
func EffectiveSince(setting string, states map[string]DayStatus) string {
	if setting != "" {
		return setting
	}
	earliest := ""
	for k := range states {
		if earliest == "" || k < earliest {
			earliest = k
		}
	}
	return earliest
}

// StreakResult is the current and the best streak.
type StreakResult struct {
	Current int `json:"current"`
	Best    int `json:"best"`
}

// ComputeStreak counts from `since` (see EffectiveSince) on, so pages written before it neither extend nor break the streak.
// "" keeps the old rule: count from the earliest day with a file.
func ComputeStreak(states map[string]DayStatus, now time.Time, cal Calendar, weekdays WeekdaySet, since string) StreakResult {
	loc := cal.location()
	today := DayKeyString(now, loc)
	first := since
	if first == "" {
		for k := range states {
			if k <= today && (first == "" || k < first) {
				first = k
			}
		}
	}
	if first == "" || first > today {
		return StreakResult{}
	}
	date, ok := DayKeyDate(first, loc)
	if !ok {
		return StreakResult{}
	}
	cur := first
	run, best := 0, 0
	for steps := 0; cur <= today && steps < 40000; steps++ {
		switch states[cur] {
		case StatusLogged:
			run++
			best = max(best, run)
		case StatusSkipped:
			// neutral
		default:
			if cur != today && weekdays.Has(weekdayOf(date, loc)) {
				run = 0
			}
		}
		date = addDays(date, 1, loc)
		cur = DayKeyString(date, loc)
	}
	return StreakResult{Current: run, Best: best}
}

// HeatCell is one square of the heatmap.
type HeatCell struct {
	Day     string    `json:"day"`
	Status  DayStatus `json:"status"`
	IsToday bool      `json:"isToday"`
}

// HeatmapWeeks is the last `count` weeks (the UI uses 12), oldest column first; logStart "" = none.
func HeatmapWeeks(states map[string]DayStatus, now time.Time, cal Calendar, weekdays WeekdaySet, count int, logStart string) [][]HeatCell {
	loc := cal.location()
	thisWeek := cal.StartOfWeek(now)
	today := DayKeyString(now, loc)
	since := EffectiveSince(logStart, states)
	var cols [][]HeatCell
	for w := -(count - 1); w <= 0; w++ {
		ws := addDays(thisWeek, 7*w, loc)
		var col []HeatCell
		for i := 0; i < 7; i++ {
			k := DayKeyString(addDays(ws, i, loc), loc)
			col = append(col, HeatCell{Day: k, Status: ResolveStatus(k, states[k], now, cal, weekdays, since), IsToday: k == today})
		}
		cols = append(cols, col)
	}
	return cols
}
