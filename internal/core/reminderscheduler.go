// reminderscheduler.go - the plan for end-of-day reminders that the OS itself delivers while Gloamlog is closed. Pure: the same
// inputs give the same plan, and a plan that has been applied diffs to nothing. ReminderDecide (reminderplanner.go) stays the
// authority while the app runs; the desktop shell applies this plan to the notification center. Port of
// app/Core/Schedule/ReminderScheduler.swift.
//
//	ReminderUpcoming(now, cal, settings, states, horizon, use24Hour)   what the next 14 scheduled days need
//	ReminderPlan(...)                                                  same, but nothing while Settings.ClosedAppReminders is off
//	ReminderDiff(planned, pending) (add, remove)                       the least change; only ids under "gloamlog.reminder." are touched
//	ReminderNext(plan, now, cal)                                       the first fire time after now
//	ReminderLabel(date, cal, use24Hour)                                "Thu 8 Oct, 4:55 pm"
//	ReminderCopy(at, cal, use24Hour)                                   title and body: never page text, a streak or a name
//	ReminderID(day) / ReminderSnoozeID(day)                            "gloamlog.reminder.2026-10-09" / "gloamlog.snooze.2026-10-09"
//
// Rules: from today on, the next 14 days whose weekday is in Settings.Weekdays. A day gets no reminder when it is logged or skipped
// (a skip range may reach future days), is before the log start date, or is today with its time already past. A started day (a few
// words, or only jots) is not logged, so it keeps its reminder.
//
// Go vs Swift: Swift read the Mac's 12 or 24 hour setting through DateFormatter. Go has no such setting, so the caller says it
// (use24Hour) and the 12 hour form is "4:55 pm" with a plain space (macOS 14+ puts a narrow no-break space before am/pm).
package core

import (
	"fmt"
	"strings"
	"time"
)

const (
	// ReminderHorizon is how many scheduled days ahead the plan reaches.
	ReminderHorizon = 14
	// ReminderIDPrefix starts the id of every end-of-day request this plan owns.
	ReminderIDPrefix = "gloamlog.reminder."
	// ReminderSnoozePrefix starts the id of a snooze request (not part of this plan).
	ReminderSnoozePrefix = "gloamlog.snooze."
	// ReminderCategory is the notification category with the Open, Snooze and Skip today buttons.
	ReminderCategory = "reminder"
)

// ReminderFire is the year, month, day, hour and minute of a request, which the system resolves in the Mac's zone.
type ReminderFire struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

func (f ReminderFire) key() string {
	return fmt.Sprintf("%d-%d-%d %d:%d", f.Year, f.Month, f.Day, f.Hour, f.Minute)
}

func (f ReminderFire) date(loc *time.Location) time.Time {
	return time.Date(f.Year, time.Month(f.Month), f.Day, f.Hour, f.Minute, 0, 0, loc)
}

// PlannedReminder is one end-of-day request for the OS.
type PlannedReminder struct {
	ID       string       `json:"id"`
	Day      string       `json:"day"`
	Fire     ReminderFire `json:"fire"`
	Title    string       `json:"title"`
	Body     string       `json:"body"`
	Category string       `json:"category"` // the Open, Snooze and Skip today buttons
}

// PendingReminder is what the notification center holds for us (a request flattened so the diff stays pure).
type PendingReminder struct {
	ID       string        `json:"id"`
	Fire     *ReminderFire `json:"fire"` // nil for a request with no calendar trigger
	Title    string        `json:"title"`
	Body     string        `json:"body"`
	Category string        `json:"category"`
}

// ReminderID is the request id of a day's end-of-day reminder.
func ReminderID(day string) string { return ReminderIDPrefix + day }

// ReminderSnoozeID is the request id of a day's snooze.
func ReminderSnoozeID(day string) string { return ReminderSnoozePrefix + day }

// ReminderUpcoming is what the next `horizon` scheduled days need, whether or not the OS is asked to hold them.
func ReminderUpcoming(now time.Time, cal Calendar, settings Settings, states map[string]DayStatus, horizon int, use24Hour bool) []PlannedReminder {
	loc := cal.location()
	out := []PlannedReminder{}
	start := dayStart(now, loc)
	scheduled := 0
	for offset := 0; offset < horizon*7+7; offset++ { // a scheduled weekday comes round every week, so this always reaches `horizon`
		if scheduled == horizon {
			break
		}
		date := addDays(start, offset, loc)
		if !settings.Weekdays.Has(weekdayOf(date, loc)) {
			continue
		}
		scheduled++
		day := DayKeyString(date, loc)
		y, m, d := date.In(loc).Date()
		fire := ReminderFire{Year: y, Month: int(m), Day: d, Hour: settings.ReminderMinutes / 60, Minute: settings.ReminderMinutes % 60}
		at := fire.date(loc)
		if !at.After(now) { // today, and its time has passed
			continue
		}
		if s := states[day]; s == StatusLogged || s == StatusSkipped {
			continue
		}
		if settings.LogStartDate != "" && day < settings.LogStartDate {
			continue
		}
		title, body := ReminderCopy(at, cal, use24Hour)
		out = append(out, PlannedReminder{ID: ReminderID(day), Day: day, Fire: fire, Title: title, Body: body, Category: ReminderCategory})
	}
	return out
}

// ReminderPlan is what the OS should be holding: ReminderUpcoming, or nothing while "Remind me when Gloamlog is closed" is off.
func ReminderPlan(now time.Time, cal Calendar, settings Settings, states map[string]DayStatus, horizon int, use24Hour bool) []PlannedReminder {
	if !settings.ClosedAppReminders {
		return []PlannedReminder{}
	}
	return ReminderUpcoming(now, cal, settings, states, horizon, use24Hour)
}

// ReminderDiff is the least change that makes pending equal planned. A request with the same id is replaced when its time or text
// differs. Pending requests that are not ours (no "gloamlog.reminder." prefix) are never touched.
func ReminderDiff(planned []PlannedReminder, pending []PendingReminder) (add []PlannedReminder, remove []string) {
	have := map[string]PendingReminder{}
	var mine []string
	for _, p := range pending {
		if strings.HasPrefix(p.ID, ReminderIDPrefix) {
			have[p.ID] = p
			mine = append(mine, p.ID)
		}
	}
	wanted := map[string]bool{}
	for _, p := range planned {
		wanted[p.ID] = true
	}
	add = []PlannedReminder{}
	for _, p := range planned {
		h, ok := have[p.ID]
		if !ok || h.Title != p.Title || h.Body != p.Body || h.Category != p.Category || h.Fire == nil || h.Fire.key() != p.Fire.key() {
			add = append(add, p)
		}
	}
	remove = []string{}
	for _, id := range mine {
		if !wanted[id] {
			remove = append(remove, id)
		}
	}
	return add, remove
}

// ReminderNext is the first planned fire time after now; ok is false when there is none.
func ReminderNext(plan []PlannedReminder, now time.Time, cal Calendar) (time.Time, bool) {
	loc := cal.location()
	var best time.Time
	for _, p := range plan {
		if t := p.Fire.date(loc); t.After(now) && (best.IsZero() || t.Before(best)) {
			best = t
		}
	}
	return best, !best.IsZero()
}

// ReminderLabel is "Thu 8 Oct, 4:55 pm".
func ReminderLabel(date time.Time, cal Calendar, use24Hour bool) string {
	loc := cal.location()
	return DayKeyFormat(DayKeyString(date, loc), "EEE d MMM", loc) + ", " + ReminderTimeText(date, cal, use24Hour)
}

// ReminderCopy is the notification text: informational, never an order.
func ReminderCopy(at time.Time, cal Calendar, use24Hour bool) (title, body string) {
	return "Today isn't logged yet", "It's " + ReminderTimeText(at, cal, use24Hour) + ". Open Gloamlog when you're ready."
}

// ReminderTimeText is the time as the Mac's short style prints it, lower-cased: "4:55 pm", or "16:55" with a 24 hour clock.
func ReminderTimeText(t time.Time, cal Calendar, use24Hour bool) string {
	if use24Hour {
		return t.In(cal.location()).Format("15:04")
	}
	return t.In(cal.location()).Format("3:04 pm")
}
