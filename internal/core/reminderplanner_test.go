package core

// Ported from tests/main.swift: "reminder planner" (the DST case is in streak_test.go).

import (
	"encoding/json"
	"testing"
	"time"
)

const monDay = "2026-10-05"

func at(h, m int) time.Time       { return mk(2026, 10, 5, h, m) }
func atDay(d, h, m int) time.Time { return mk(2026, 10, d, h, m) }
func defaults() Settings          { return DefaultSettings("/home/x") }
func decide(now time.Time, s Settings, st DayStatus, state PlannerState, win bool) (ReminderAction, PlannerState) {
	return ReminderDecide(now, ny, s, st, state, win)
}

func TestPlannerNotDueIneligibleDays(t *testing.T) {
	s := defaults()
	fresh := PlannerState{Day: monDay}
	a := func(now time.Time, st DayStatus, state PlannerState) ReminderAction {
		r, _ := decide(now, s, st, state, true)
		return r
	}
	expect(t, a(at(16, 54), StatusMissed, fresh) == ActionNone, "before reminder")
	expect(t, a(at(17, 0), StatusLogged, fresh) == ActionNone, "already logged")
	expect(t, a(at(17, 0), StatusSkipped, fresh) == ActionNone, "skipped today")
	sat := PlannerState{Day: "2026-10-10"}
	expect(t, a(atDay(10, 17, 0), StatusOff, sat) == ActionNone, "Saturday")
	expect(t, a(atDay(10, 17, 0), StatusMissed, sat) == ActionNone, "Saturday even if status says unlogged")
	expect(t, a(at(17, 0), StatusFuture, fresh) == ActionNone, "future")
	expect(t, a(at(17, 0), StatusPartial, fresh) == ActionBringToFront(true), "partial still nags")
	tue := s
	tue.Weekdays = WeekdaySet{3}
	r, _ := decide(at(17, 0), tue, StatusMissed, fresh, true)
	expect(t, r == ActionNone, "Monday not scheduled")
}

func TestPlannerStrictFirstFireReopenSkipHintCutoff(t *testing.T) {
	s := defaults()
	a, st := decide(at(16, 55), s, StatusMissed, PlannerState{Day: monDay}, true)
	expect(t, a == ActionBringToFront(true) && st.FirstFired, "first fire")
	a, st = decide(at(16, 56), s, StatusMissed, st, true)
	expect(t, a == ActionNone, "visible: leave alone")
	a, st = decide(at(17, 0), s, StatusMissed, st, false)
	expect(t, a == ActionNone, "just closed: start 5 min wait")
	a, st = decide(at(17, 4), s, StatusMissed, st, false)
	expect(t, a == ActionNone, "still waiting")
	a, st = decide(at(17, 5), s, StatusMissed, st, false)
	expect(t, a == ActionReopen && st.ReopenCount == 1, "first re-open")
	a, st = decide(at(17, 6), s, StatusMissed, st, false)
	expect(t, a == ActionNone, "closed again, wait")
	a, st = decide(at(17, 11), s, StatusMissed, st, false)
	expect(t, a == ActionReopen && st.ReopenCount == 2 && !st.ShowSkipHint(), "second re-open")
	_, st = decide(at(17, 12), s, StatusMissed, st, false)
	a, st = decide(at(17, 17), s, StatusMissed, st, false)
	expect(t, a == ActionReopen && st.ReopenCount == 3 && st.ShowSkipHint(), "third re-open shows the hint")
	_, st = decide(at(22, 59), s, StatusMissed, st, false)
	a, st = decide(at(23, 0), s, StatusMissed, st, false)
	expect(t, a == ActionNone, "23:00 cutoff")
	a, _ = decide(at(23, 30), s, StatusMissed, st, false)
	expect(t, a == ActionNone, "after the cutoff")
	a, _ = decide(at(23, 5), s, StatusMissed, PlannerState{Day: monDay}, true)
	expect(t, a == ActionNone, "woke after cutoff: no first fire")
	a, _ = decide(at(22, 59), s, StatusMissed, PlannerState{Day: monDay}, true)
	expect(t, a == ActionBringToFront(true), "woke at 22:59 fires")
}

func TestPlannerStrictSnoozeMax2ThenRefireWithoutSecondNotification(t *testing.T) {
	s := defaults()
	_, st := decide(at(16, 55), s, StatusMissed, PlannerState{Day: monDay}, true)
	left, limited := ReminderSnoozesLeft(st, s)
	expect(t, limited && left == 2, "two snoozes at first")
	st, ok := ReminderSnooze(st, at(17, 0), s, ny)
	left, _ = ReminderSnoozesLeft(st, s)
	expect(t, ok && st.SnoozedUntil.Equal(at(17, 15)) && left == 1, "snoozed until 17:15")
	a, _ := decide(at(17, 10), s, StatusMissed, st, false)
	expect(t, a == ActionNone, "silent during snooze, even with window closed")
	a, st = decide(at(17, 15), s, StatusMissed, st, false)
	expect(t, a == ActionBringToFront(false), "re-fire quietly")
	st, _ = ReminderSnooze(st, at(17, 16), s, ny)
	left, _ = ReminderSnoozesLeft(st, s)
	expect(t, left == 0, "no snoozes left")
	_, ok = ReminderSnooze(st, at(17, 40), s, ny)
	expect(t, !ok, "third snooze refused")
	a, _ = decide(at(17, 31), s, StatusMissed, st, true)
	expect(t, a == ActionBringToFront(false), "re-fire after the second snooze")
	// snooze runs past 23:00 -> nothing
	late := PlannerState{Day: monDay, FirstFired: true, SnoozedUntil: at(23, 10)}
	a, _ = decide(at(23, 11), s, StatusMissed, late, true)
	expect(t, a == ActionNone, "past the cutoff")
	// snooze length follows settings
	s30 := s
	s30.SnoozeMinutes = 30
	sn, _ := ReminderSnooze(PlannerState{Day: monDay}, at(17, 0), s30, ny)
	expect(t, sn.SnoozedUntil.Equal(at(17, 30)), "30 minute snooze")
}

func TestPlannerGentleNeverForcesNotifiesOnceUnlimitedSnooze(t *testing.T) {
	g := defaults()
	g.Mode = ModeGentle
	a, st := decide(at(16, 55), g, StatusMissed, PlannerState{Day: monDay}, false)
	expect(t, a == ActionNotify, "gentle first fire")
	for _, m := range []int{0, 5, 30, 59} {
		r, _ := decide(at(17, m), g, StatusMissed, st, false)
		expect(t, r == ActionNone, "no repeats / re-opens at 17:%02d", m)
	}
	_, limited := ReminderSnoozesLeft(st, g)
	expect(t, !limited, "unlimited")
	for i := 0; i < 5; i++ {
		st, _ = ReminderSnooze(st, at(17, i), g, ny)
	}
	expect(t, st.SnoozesUsed == 0, "no counter in gentle")
	a, st = decide(at(17, 20), g, StatusMissed, st, true)
	expect(t, a == ActionNotify, "notification again after snooze")
	a, _ = decide(at(17, 21), g, StatusMissed, st, true)
	expect(t, a == ActionNone, "and then quiet")
}

func TestPlannerLateLaunchGraceRolloverSuppress(t *testing.T) {
	s := defaults()
	st := PlannerAtLaunch(nil, at(18, 0), ny)
	a, _ := decide(at(18, 0), s, StatusMissed, st, true)
	expect(t, a == ActionNone, "2 min grace after launch")
	a, _ = decide(at(18, 1), s, StatusMissed, st, true)
	expect(t, a == ActionNone, "still in the grace")
	a, st = decide(at(18, 2), s, StatusMissed, st, true)
	expect(t, a == ActionBringToFront(true), "fires once grace is over")
	// asleep through 16:55, wakes 20:10
	a, _ = decide(at(20, 10), s, StatusMissed, PlannerState{Day: monDay}, true)
	expect(t, a == ActionBringToFront(true), "woke late")
	// wakes after a snooze elapsed during sleep
	sn := PlannerState{Day: monDay, FirstFired: true, SnoozedUntil: at(17, 15)}
	a, _ = decide(at(19, 0), s, StatusMissed, sn, true)
	expect(t, a == ActionBringToFront(false), "snooze elapsed during sleep")
	// settings change / undo skip suppress
	sup := PlannerState{Day: monDay}.Suppressed(at(17, 1).Add(ReminderGraceAfterSettingsChange - 60*time.Second + 60*time.Second))
	a1, _ := decide(at(17, 0), s, StatusMissed, sup, true)
	a2, _ := decide(at(17, 2), s, StatusMissed, sup, true)
	expect(t, a1 == ActionNone && a2 != ActionNone, "suppressed until 17:01")
	expect(t, PlannerState{Day: monDay, GraceUntil: at(17, 30)}.Suppressed(at(17, 1)).GraceUntil.Equal(at(17, 30)), "a longer grace is kept")
	// snooze count survives relaunch the same day, not the next
	used := PlannerState{Day: monDay, SnoozesUsed: 2}
	expect(t, PlannerAtLaunch(&used, at(18, 0), ny).SnoozesUsed == 2, "same day")
	expect(t, PlannerAtLaunch(&used, atDay(6, 9, 0), ny).SnoozesUsed == 0, "next day")
	// yesterday's state resets on a new day
	old := PlannerState{Day: monDay, FirstFired: true, SnoozesUsed: 2, ReopenCount: 5}
	a2b, n := decide(atDay(6, 17, 0), s, StatusMissed, old, true)
	expect(t, a2b == ActionBringToFront(true) && n.SnoozesUsed == 0 && n.ReopenCount == 0 && n.Day == "2026-10-06", "new day")
}

func TestPlannerIsPendingAndStateIsJSON(t *testing.T) {
	s := defaults()
	expect(t, ReminderIsPending(at(17, 0), ny, s, StatusMissed), "pending at 17:00")
	expect(t, !ReminderIsPending(at(10, 0), ny, s, StatusMissed), "not before")
	expect(t, !ReminderIsPending(at(17, 0), ny, s, StatusLogged), "not when logged")
	expect(t, !ReminderIsPending(atDay(10, 17, 0), ny, s, StatusOff), "not on a day off")
	st := PlannerState{Day: monDay, SnoozedUntil: at(17, 15), ReopenCount: 2}
	data, err := json.Marshal(st)
	must(t, err)
	var back PlannerState
	must(t, json.Unmarshal(data, &back))
	expect(t, back.Day == st.Day && back.SnoozedUntil.Equal(st.SnoozedUntil) && back.ReopenCount == 2 && back.LastFire.IsZero(), "JSON round trip: %s", data)
}
