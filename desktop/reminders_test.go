package main

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// fakeOS records what the engine asked of the machine; no real notification is ever sent.
type fakeOS struct {
	available, visible bool
	allowed            bool
	posts              []notifications.NotificationOptions
	removed            []string
	raised             []bool
	hidden             atomic.Int32 // HideMain runs from a timer
	events             []string
	notices            []ReminderNoticeState
	categories         []int
}

func (f *fakeOS) Available() bool { return f.available }
func (f *fakeOS) Post(o notifications.NotificationOptions) error {
	f.posts = append(f.posts, o)
	return nil
}
func (f *fakeOS) Remove(id string)             { f.removed = append(f.removed, id) }
func (f *fakeOS) Check() (bool, error)         { return f.allowed, nil }
func (f *fakeOS) Request() (bool, error)       { return f.allowed, nil }
func (f *fakeOS) RegisterCategory(m int) error { f.categories = append(f.categories, m); return nil }
func (f *fakeOS) RaiseMain(activate bool)      { f.raised = append(f.raised, activate) }
func (f *fakeOS) MainVisible() bool            { return f.visible }
func (f *fakeOS) HideMain()                    { f.hidden.Add(1) }
func (f *fakeOS) Emit(name string, data any) {
	f.events = append(f.events, name)
	if n, ok := data.(ReminderNoticeState); ok {
		f.notices = append(f.notices, n)
	}
}
func (f *fakeOS) clear() { f.posts, f.removed, f.raised, f.events = nil, nil, nil, nil }
func (f *fakeOS) scheduled() (n int) {
	for _, p := range f.posts {
		if p.Schedule != nil {
			n++
		}
	}
	return n
}

type rig struct {
	t       *testing.T
	e       *reminderEngine
	os      *fakeOS
	now     time.Time
	in      reminderInput
	skipped []string
}

// Wednesday 14 Oct 2026: a scheduled day with the default 16:55 reminder.
func newRig(t *testing.T, mode core.ReminderMode) *rig {
	r := &rig{t: t, os: &fakeOS{available: true, allowed: true}, now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)}
	s := core.DefaultSettings(testHome)
	s.Mode = mode
	r.in = reminderInput{Settings: s, Cal: core.NewCalendar(time.Local, 2), States: map[string]core.DayStatus{}}
	r.e = &reminderEngine{
		snap: func() (reminderInput, error) { return r.in, nil },
		skip: func(day string) error {
			r.skipped = append(r.skipped, day)
			r.in.States[day] = core.StatusSkipped
			return nil
		},
		now:   func() time.Time { return r.now },
		use24: func() bool { return false },
		os:    r.os,
		path:  filepath.Join(t.TempDir(), "planner.json"),
		force: true,
	}
	r.e.allowed = true
	r.e.load(r.in.Cal) // launch at 09:00: grace until 09:02
	return r
}

func (r *rig) at(h, m int) { r.now = time.Date(2026, 10, 14, h, m, 0, 0, time.Local) }
func (r *rig) tick()       { r.e.tick() }

func TestStrictFirstFireRaisesWindowWithOneNotification(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.in.Settings.ClosedAppReminders = false // with it on, the Mac's own request stands (TestDeliveredRequestStands...)
	r.at(16, 54)
	r.tick()
	if len(r.os.raised) != 0 || len(r.os.posts) != 0 && r.os.posts[len(r.os.posts)-1].ID == inAppReminderID {
		t.Fatalf("nothing should fire before 16:55: raised=%v", r.os.raised)
	}
	r.os.clear()
	r.at(16, 56)
	r.tick()
	if len(r.os.raised) != 1 || !r.os.raised[0] {
		t.Fatalf("strict first fire raises the window with focus: %v", r.os.raised)
	}
	var inApp []notifications.NotificationOptions
	for _, p := range r.os.posts {
		if p.ID == inAppReminderID {
			inApp = append(inApp, p)
		}
	}
	if len(inApp) != 1 || inApp[0].Title != "Time to write up your day" || inApp[0].Data["day"] != "2026-10-14" {
		t.Fatalf("one in-app notification, within 5 minutes of the time: %+v", inApp)
	}
	last := r.os.notices[len(r.os.notices)-1]
	if !last.Visible || !last.Strict || last.Time != "4:56 pm" || last.SnoozeLabel != "Remind me in 15 min · 2 left" || !last.CanSnooze {
		t.Fatalf("notice: %+v", last)
	}
	// Nothing again on the next tick (window now visible).
	r.os.clear()
	r.os.visible = true
	r.at(16, 57)
	r.tick()
	if len(r.os.raised) != 0 {
		t.Fatalf("no second raise: %v", r.os.raised)
	}
}

func TestLateStartCopyAndOldReminder(t *testing.T) {
	r := newRig(t, core.ModeGentle)
	r.at(18, 30)
	r.tick()
	var inApp *notifications.NotificationOptions
	for i, p := range r.os.posts {
		if p.ID == inAppReminderID {
			inApp = &r.os.posts[i]
		}
	}
	if inApp == nil || inApp.Title != "Today isn't logged yet" || inApp.Body != "It's 6:30 pm. Open Gloamlog whenever you're ready." {
		t.Fatalf("late copy: %+v", inApp)
	}
	if len(r.os.raised) != 0 {
		t.Fatalf("gentle never raises the window: %v", r.os.raised)
	}
}

func TestStrictSnoozeTwiceThenNoMore(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.at(17, 0)
	r.tick() // first fire
	r.os.clear()
	for i := 1; i <= 2; i++ {
		r.e.mu.Lock()
		ok := r.e.snooze(r.now, r.in, false)
		r.e.mu.Unlock()
		if !ok {
			t.Fatalf("snooze %d refused", i)
		}
		r.at(r.now.Hour(), r.now.Minute()+10) // still snoozed: nothing happens
		r.tick()
		if len(r.os.raised) != 0 {
			t.Fatalf("raised while snoozed")
		}
		r.at(r.now.Hour(), r.now.Minute()+6) // snooze over: window again, no second notification
		before := len(r.os.posts)
		r.tick()
		if len(r.os.raised) != 1 {
			t.Fatalf("snooze %d: window should come back: %v", i, r.os.raised)
		}
		for _, p := range r.os.posts[before:] {
			if p.ID == inAppReminderID {
				t.Fatalf("no second notification after a strict snooze")
			}
		}
		r.os.clear()
		r.os.visible = true
	}
	n := r.e.noticeState(r.now, r.in, "2026-10-14", core.StatusMissed)
	if n.CanSnooze || n.SnoozeLabel != "No snoozes left today" {
		t.Fatalf("after two snoozes: %+v", n)
	}
	r.e.mu.Lock()
	ok := r.e.snooze(r.now, r.in, false)
	r.e.mu.Unlock()
	if ok {
		t.Fatal("third strict snooze must be refused")
	}
}

func TestGentleSnoozeIsUnlimitedAndLabelHasNoCount(t *testing.T) {
	r := newRig(t, core.ModeGentle)
	r.at(17, 0)
	r.tick()
	for i := 0; i < 5; i++ {
		r.e.mu.Lock()
		ok := r.e.snooze(r.now, r.in, false)
		r.e.mu.Unlock()
		if !ok {
			t.Fatalf("gentle snooze %d refused", i)
		}
	}
	if n := r.e.noticeState(r.now, r.in, "2026-10-14", core.StatusMissed); n.SnoozeLabel != "Remind me in 15 min" || n.Visible {
		t.Fatalf("gentle notice while snoozed: %+v", n)
	}
}

func TestUndoSkipHasGrace(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.in.States["2026-10-14"] = core.StatusSkipped
	r.at(17, 0)
	r.tick() // skipped: nothing
	r.in.States["2026-10-14"] = core.StatusMissed
	r.os.clear()
	r.at(17, 1)
	r.tick()
	if len(r.os.raised) != 0 {
		t.Fatalf("120 s grace after undo skip: %v", r.os.raised)
	}
	r.at(17, 4)
	r.tick()
	if len(r.os.raised) != 1 {
		t.Fatalf("fires after the grace: %v", r.os.raised)
	}
}

func TestSettingsChangeHasGrace(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.at(17, 0)
	r.in.Settings.SnoozeMinutes = 10
	r.in.Settings.ReminderMinutes = 16*60 + 30
	r.tick() // first tick sees the settings for the first time: no grace
	if len(r.os.raised) != 1 {
		t.Fatalf("setup: %v", r.os.raised)
	}
	r2 := newRig(t, core.ModeStrict)
	r2.at(17, 0)
	r2.e.tick() // learns the settings, first fire
	r2.os.clear()
	r2.in.Settings.Mode = core.ModeGentle
	r2.e.f.Planner.FirstFired = false
	r2.at(17, 0)
	r2.tick() // mode changed now: 60 s grace
	if len(r2.os.posts) != 0 && func() bool {
		for _, p := range r2.os.posts {
			if p.ID == inAppReminderID {
				return true
			}
		}
		return false
	}() {
		t.Fatal("settings change must silence the planner for a minute")
	}
}

func TestSleepingScreensDoNotNag(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.e.asleep = true
	r.at(17, 0)
	r.tick()
	if len(r.os.raised) != 0 {
		t.Fatal("screens asleep: no reminder")
	}
	r.e.asleep = false
	r.tick()
	if len(r.os.raised) != 1 {
		t.Fatal("awake again: the next tick fires")
	}
}

func TestClosedAppPlanIsAppliedOnceAndFollowsTheDay(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.at(9, 5)
	r.tick()
	if got := r.os.scheduled(); got != core.ReminderHorizon {
		t.Fatalf("14 scheduled requests, got %d", got)
	}
	first := r.os.posts[0]
	if first.ID != "gloamlog.reminder.2026-10-14" || first.Title != "Today isn't logged yet" || first.Body != "It's 4:55 pm. Open Gloamlog when you're ready." ||
		first.Schedule.At != time.Date(2026, 10, 14, 16, 55, 0, 0, time.Local).Unix() || first.CategoryID != core.ReminderCategory {
		t.Fatalf("first request: %+v", first)
	}
	// A plan that has been applied diffs to nothing.
	r.os.clear()
	r.at(9, 6)
	r.tick()
	if len(r.os.posts) != 0 || len(r.os.removed) != 0 {
		t.Fatalf("same plan again must do nothing: %v %v", len(r.os.posts), r.os.removed)
	}
	// Logging today silences its request at once; the other 13 stay.
	r.in.States["2026-10-14"] = core.StatusLogged
	r.at(9, 7)
	r.tick()
	if !contains(r.os.removed, "gloamlog.reminder.2026-10-14") || !contains(r.os.removed, "gloamlog.snooze.2026-10-14") {
		t.Fatalf("today's requests removed: %v", r.os.removed)
	}
	if len(r.e.f.Scheduled) != core.ReminderHorizon-1 {
		t.Fatalf("record keeps the rest: %d", len(r.e.f.Scheduled))
	}
	// Switching the closed-app reminders off removes everything we scheduled.
	r.os.clear()
	r.in.Settings.ClosedAppReminders = false
	r.at(9, 8)
	r.tick()
	if len(r.os.removed) < core.ReminderHorizon-1 || len(r.e.f.Scheduled) != 0 {
		t.Fatalf("switch off: removed %d, left %d", len(r.os.removed), len(r.e.f.Scheduled))
	}
}

func TestNothingScheduledWithoutPermission(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.e.allowed = false
	r.at(9, 5)
	r.tick()
	if r.os.scheduled() != 0 {
		t.Fatal("no permission: nothing is handed to the Mac")
	}
	r.e.allowed = true // permission arrives; the plan was kept pending
	r.e.force = true
	r.at(9, 6)
	r.tick()
	if r.os.scheduled() != core.ReminderHorizon {
		t.Fatalf("after permission: %d", r.os.scheduled())
	}
}

func TestDeliveredRequestStandsAgainstInAppNotification(t *testing.T) {
	r := newRig(t, core.ModeGentle)
	r.at(9, 5)
	r.tick() // plan applied
	r.os.clear()
	r.at(16, 55) // the Mac delivers its request now; the tick comes 30 s later
	r.now = r.now.Add(30 * time.Second)
	r.e.f.Planner.GraceUntil = time.Time{}
	r.tick()
	for _, p := range r.os.posts {
		if p.ID == inAppReminderID {
			t.Fatalf("the delivered request stands, no second notification: %+v", p)
		}
	}
	// An hour later (app just launched): the old one is replaced by the in-app copy.
	r2 := newRig(t, core.ModeGentle)
	r2.at(9, 5)
	r2.tick()
	r2.os.clear()
	r2.at(18, 0)
	r2.e.f.Planner.GraceUntil = time.Time{}
	r2.tick()
	found := false
	for _, p := range r2.os.posts {
		found = found || p.ID == inAppReminderID
	}
	if !found || !contains(r2.os.removed, "gloamlog.reminder.2026-10-14") {
		t.Fatalf("a late start posts its own and clears the old one: removed=%v", r2.os.removed)
	}
}

func TestSnoozeFromBannerSchedulesOneOff(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.at(17, 0)
	r.tick()
	r.os.clear()
	r.e.handle(notifications.NotificationResponse{ActionIdentifier: "snooze", UserInfo: map[string]any{"day": "2026-10-14"}})
	var snooze *notifications.NotificationOptions
	for i, p := range r.os.posts {
		if p.ID == "gloamlog.snooze.2026-10-14" {
			snooze = &r.os.posts[i]
		}
	}
	if snooze == nil || snooze.Schedule.At != time.Date(2026, 10, 14, 17, 15, 0, 0, time.Local).Unix() || snooze.Body != "It's 5:15 pm. Open Gloamlog when you're ready." {
		t.Fatalf("one-off snooze: %+v", snooze)
	}
	if !contains(r.os.removed, inAppReminderID) {
		t.Fatalf("snooze clears the in-app banner: %v", r.os.removed)
	}
	// Near the cutoff no one-off is scheduled.
	r.os.clear()
	r.at(22, 50)
	r.e.f.Planner = core.PlannerState{Day: "2026-10-14"}
	r.e.handle(notifications.NotificationResponse{ActionIdentifier: "snooze"})
	for _, p := range r.os.posts {
		if p.ID == "gloamlog.snooze.2026-10-14" {
			t.Fatal("a snooze past 23:00 is not scheduled")
		}
	}
}

func TestBannerButtons(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.at(17, 0)
	r.tick()
	// Skip today from the banner: skip marker, requests gone.
	r.os.clear()
	r.e.handle(notifications.NotificationResponse{ActionIdentifier: "skip", UserInfo: map[string]any{"day": "2026-10-14"}})
	if len(r.skipped) != 1 || r.skipped[0] != "2026-10-14" || !contains(r.os.removed, "gloamlog.reminder.2026-10-14") || len(r.os.raised) != 0 {
		t.Fatalf("skip: %v removed=%v raised=%v", r.skipped, r.os.removed, r.os.raised)
	}
	// An old banner opens the day it is about.
	r.os.clear()
	r.e.handle(notifications.NotificationResponse{ActionIdentifier: "DEFAULT_ACTION", UserInfo: map[string]any{"day": "2026-10-09"}})
	if len(r.os.raised) != 1 || !contains(r.os.events, "reminder-open-day") {
		t.Fatalf("old banner: %v %v", r.os.raised, r.os.events)
	}
	// Launched by Snooze: the window that opened is put away again.
	r2 := newRig(t, core.ModeStrict)
	r2.at(9, 1)
	r2.e.started = r2.now.Add(-time.Second)
	r2.e.hideIfLaunched(r2.now)
	time.Sleep(300 * time.Millisecond)
	if r2.os.hidden.Load() != 1 {
		t.Fatalf("hidden after launch action: %d", r2.os.hidden.Load())
	}
}

func TestPlannerSurvivesRestart(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.at(17, 0)
	r.tick()
	r.e.mu.Lock()
	r.e.snooze(r.now, r.in, false)
	r.e.mu.Unlock()
	e2 := &reminderEngine{snap: r.e.snap, now: r.e.now, use24: r.e.use24, os: r.os, path: r.e.path}
	e2.load(r.in.Cal)
	if e2.f.Planner.SnoozesUsed != 1 || e2.f.Planner.GraceUntil.IsZero() || len(e2.f.Scheduled) != core.ReminderHorizon-1 { // today's 16:55 request has fired by 17:00
		t.Fatalf("restart keeps today's snooze count, adds grace, keeps the record: %+v", e2.f.Planner)
	}
}

func TestInAppCopyBoundary(t *testing.T) {
	cal := core.NewCalendar(time.Local, 2)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 14, h, m, 0, 0, time.Local) }
	if title, _ := inAppCopy(at(17, 0), 16*60+55, cal, false); title != "Time to write up your day" {
		t.Fatalf("5 minutes after: %q", title)
	}
	if title, body := inAppCopy(at(17, 1), 16*60+55, cal, true); title != "Today isn't logged yet" || body != "It's 17:01. Open Gloamlog whenever you're ready." {
		t.Fatalf("6 minutes after: %q %q", title, body)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// A banner with no day, or a day that is not a day key (never a path), means today.
func TestBannerWithoutUsableDayMeansToday(t *testing.T) {
	for _, info := range []map[string]any{nil, {}, {"day": ""}, {"day": "../../etc/passwd"}, {"day": 7}} {
		r := newRig(t, core.ModeStrict)
		r.at(17, 0)
		r.tick()
		r.os.clear()
		r.e.handle(notifications.NotificationResponse{ActionIdentifier: "skip", UserInfo: info})
		if len(r.skipped) != 1 || r.skipped[0] != "2026-10-14" {
			t.Fatalf("UserInfo %v: skipped %v", info, r.skipped)
		}
	}
}

// A page with writing refuses the skip: the banner's Skip opens the page instead of hiding anything.
func TestSkipRefusedOpensThePage(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.e.skip = func(string) error { return core.ErrHasContent }
	r.at(17, 0)
	r.tick()
	r.os.clear()
	r.e.handle(notifications.NotificationResponse{ActionIdentifier: "skip"})
	if len(r.os.raised) != 1 || !contains(r.os.events, "reminder-open-day") {
		t.Fatalf("raised=%v events=%v", r.os.raised, r.os.events)
	}
}

// Without a notification service (go run) the planner still raises the window and fills the notice, and nothing is posted.
func TestNoNotificationServiceStillRemindsInTheWindow(t *testing.T) {
	r := newRig(t, core.ModeStrict)
	r.os.available = false
	r.at(17, 0)
	r.tick()
	if len(r.os.raised) != 1 || len(r.os.posts) != 0 || !r.os.notices[len(r.os.notices)-1].Visible {
		t.Fatalf("raised=%v posts=%d", r.os.raised, len(r.os.posts))
	}
}
