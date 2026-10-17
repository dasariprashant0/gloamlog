// reminderplanner.go - pure decision function for the nag. The UI owns a 30-60 s timer (and wake/launch hooks),
// calls ReminderDecide each tick, performs the action, and stores the new state (persist it so SnoozesUsed survives relaunch).
// Port of app/Core/ReminderPlanner.swift.
//
// API:
//
//	ReminderDecide(now, cal, settings, todayStatus, state, windowVisible) (ReminderAction, PlannerState)
//	ReminderSnooze(state, now, settings, cal) (PlannerState, bool)      false = Strict has no snoozes left
//	ReminderSnoozesLeft(state, settings) (n, limited)                   limited false = unlimited (Gentle)
//	ReminderIsPending(now, cal, settings, todayStatus) bool             banner / menu-bar "due, unlogged" (ignores snooze)
//	PlannerAtLaunch(previous, now, cal)   2-min grace after login/launch, keeps today's snooze count
//	(PlannerState).Suppressed(until) / ReminderGraceAfterSettingsChange (60 s) / ReminderGraceAfterUndoSkip (120 s)
//	(PlannerState).ShowSkipHint()         true once the 3rd re-open happened ("Not today? Skip day.")
//
// Actions:
//
//	ActionNotify                  post one notification only (Gentle first fire, Gentle after snooze)
//	ActionBringToFront(notify)    Strict: activate window + Dock bounce; notify=true on first fire, false after a snooze
//	ActionReopen                  Strict: orderFrontRegardless WITHOUT activating (window was closed >= 5 min)
//	ActionNone
//
// todayStatus: pass missed or partial for "unlogged"; logged/skipped/off/future never nag.
// Nag window: reminder time <= now < 23:00 local, scheduled weekday only. App launched late or Mac woken after the
// reminder time fires on the next tick (after the grace). Day change resets the state.
// Optional times in PlannerState are the zero time.Time when unset.
package core

import "time"

// ReminderKind is what the UI should do.
type ReminderKind string

const (
	KindNone         ReminderKind = "none"
	KindNotify       ReminderKind = "notify"
	KindBringToFront ReminderKind = "bringToFront"
	KindReopen       ReminderKind = "reopen"
)

// ReminderAction is the decision of one tick. Notify only matters for KindBringToFront.
type ReminderAction struct {
	Kind   ReminderKind `json:"kind"`
	Notify bool         `json:"notify"`
}

var (
	ActionNone   = ReminderAction{Kind: KindNone}
	ActionNotify = ReminderAction{Kind: KindNotify}
	ActionReopen = ReminderAction{Kind: KindReopen}
)

// ActionBringToFront is the Strict "activate the window" action; notify says whether a notification goes with it.
func ActionBringToFront(notify bool) ReminderAction {
	return ReminderAction{Kind: KindBringToFront, Notify: notify}
}

// PlannerState is what the planner remembers between ticks (persist it as JSON).
type PlannerState struct {
	Day          string    `json:"day"`
	SnoozesUsed  int       `json:"snoozesUsed"`
	SnoozedUntil time.Time `json:"snoozedUntil,omitzero"`
	FirstFired   bool      `json:"firstFired"`
	LastFire     time.Time `json:"lastFire,omitzero"`
	ReopenCount  int       `json:"reopenCount"`
	HiddenSince  time.Time `json:"hiddenSince,omitzero"`
	GraceUntil   time.Time `json:"graceUntil,omitzero"`
}

const (
	ReminderReopenInterval           = 5 * time.Minute
	ReminderMaxStrictSnoozes         = 2
	ReminderCutoffMinutes            = 23 * 60
	ReminderSkipHintAfterReopens     = 3
	ReminderGraceAfterLaunch         = 120 * time.Second
	ReminderGraceAfterUndoSkip       = 120 * time.Second
	ReminderGraceAfterSettingsChange = 60 * time.Second
)

// ShowSkipHint is true once the 3rd re-open happened.
func (s PlannerState) ShowSkipHint() bool { return s.ReopenCount >= ReminderSkipHintAfterReopens }

// Suppressed returns the state with the planner silenced until d (an earlier grace that lasts longer is kept).
func (s PlannerState) Suppressed(until time.Time) PlannerState {
	if s.GraceUntil.After(until) {
		return s
	}
	s.GraceUntil = until
	return s
}

// PlannerAtLaunch is the state to start a run with: a 2-minute grace, and today's snooze count kept from the previous run.
// previous nil = no saved state.
func PlannerAtLaunch(previous *PlannerState, now time.Time, cal Calendar) PlannerState {
	today := DayKeyString(now, cal.location())
	s := PlannerState{Day: today}
	if previous != nil && previous.Day == today {
		s.SnoozesUsed = previous.SnoozesUsed
	}
	s.GraceUntil = now.Add(ReminderGraceAfterLaunch)
	return s
}

func minutesOfDay(now time.Time, cal Calendar) int {
	t := now.In(cal.location())
	return t.Hour()*60 + t.Minute()
}

func needsLog(s DayStatus) bool { return s == StatusMissed || s == StatusPartial }

// ReminderIsPending is true when the banner / menu-bar "due, unlogged" should show (ignores snooze).
func ReminderIsPending(now time.Time, cal Calendar, settings Settings, todayStatus DayStatus) bool {
	if !needsLog(todayStatus) || !settings.Weekdays.Has(weekdayOf(now, cal.location())) {
		return false
	}
	return minutesOfDay(now, cal) >= settings.ReminderMinutes
}

// ReminderSnoozesLeft is how many snoozes Strict still allows; limited is false for Gentle (unlimited).
func ReminderSnoozesLeft(state PlannerState, settings Settings) (left int, limited bool) {
	if settings.Mode == ModeGentle {
		return 0, false
	}
	return max(0, ReminderMaxStrictSnoozes-state.SnoozesUsed), true
}

// ReminderSnooze snoozes the reminder. ok is false when Strict has no snoozes left.
func ReminderSnooze(state PlannerState, now time.Time, settings Settings, cal Calendar) (PlannerState, bool) {
	s := state
	if today := DayKeyString(now, cal.location()); s.Day != today {
		s = PlannerState{Day: today, GraceUntil: s.GraceUntil}
	}
	if settings.Mode == ModeStrict {
		if s.SnoozesUsed >= ReminderMaxStrictSnoozes {
			return state, false
		}
		s.SnoozesUsed++
	}
	s.SnoozedUntil = now.Add(time.Duration(settings.SnoozeMinutes) * time.Minute)
	s.HiddenSince = time.Time{}
	return s, true
}

// ReminderDecide is one tick of the planner: what to do now, and the state to keep.
func ReminderDecide(now time.Time, cal Calendar, settings Settings, todayStatus DayStatus,
	state PlannerState, windowVisible bool) (ReminderAction, PlannerState) {
	s := state
	today := DayKeyString(now, cal.location())
	if s.Day != today {
		s = PlannerState{Day: today, GraceUntil: s.GraceUntil}
	}

	if !needsLog(todayStatus) || !settings.Weekdays.Has(weekdayOf(now, cal.location())) {
		return ActionNone, s
	}
	m := minutesOfDay(now, cal)
	if m < settings.ReminderMinutes || m >= ReminderCutoffMinutes {
		return ActionNone, s
	}
	if !s.GraceUntil.IsZero() && now.Before(s.GraceUntil) {
		return ActionNone, s
	}
	if !s.SnoozedUntil.IsZero() && now.Before(s.SnoozedUntil) {
		return ActionNone, s
	}

	strict := settings.Mode == ModeStrict
	// Snooze ended: fire again (Strict: window only, no second notification; Gentle: notification again).
	if !s.SnoozedUntil.IsZero() {
		s.SnoozedUntil = time.Time{}
		s.FirstFired = true
		s.LastFire = now
		s.HiddenSince = time.Time{}
		if strict {
			return ActionBringToFront(false), s
		}
		return ActionNotify, s
	}
	// First fire of the day (also covers app launched / Mac woken after the reminder time).
	if !s.FirstFired {
		s.FirstFired = true
		s.LastFire = now
		s.HiddenSince = time.Time{}
		if strict {
			return ActionBringToFront(true), s
		}
		return ActionNotify, s
	}
	if !strict {
		return ActionNone, s
	}

	// Strict re-open: window closed/minimised for >= 5 min, and >= 5 min since the last fire.
	if windowVisible {
		s.HiddenSince = time.Time{}
		return ActionNone, s
	}
	if s.HiddenSince.IsZero() {
		s.HiddenSince = now
		return ActionNone, s
	}
	sinceLast := time.Duration(1<<62 - 1) // Swift: timeIntervalSince(lastFire ?? .distantPast)
	if !s.LastFire.IsZero() {
		sinceLast = now.Sub(s.LastFire)
	}
	if now.Sub(s.HiddenSince) >= ReminderReopenInterval && sinceLast >= ReminderReopenInterval {
		s.LastFire = now
		s.HiddenSince = time.Time{}
		s.ReopenCount++
		return ActionReopen, s
	}
	return ActionNone, s
}
