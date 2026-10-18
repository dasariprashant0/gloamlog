package main

// Reminders: the 30 s planner tick, the notifications, the "reminder-notice" events and the reminders the Mac delivers while
// Gloamlog is quit. The rules live in internal/core (reminderplanner.go, reminderscheduler.go); this file only performs them.
// Port of AppModel+Reminders.swift, Notifier.swift and ClosedAppReminders.swift.
//
// main.go (after `notes.OnSettings = registerJot`, before `app.Run()`):
//
//	startReminders(app, notes)
//
// Events (Go -> React, data is the payload):
//
//	"reminder-notice"    ReminderNoticeState  the slim notice on today's page changed (also read once with Notes.CurrentReminderNotice)
//	"reminder-open-day"  string (day key)     the planner or a notification wants this day open in the main window
//
// Methods on *Notes for the UI: CurrentReminderNotice, SnoozeReminder, SkipToday, NotificationStatus, RequestNotifications,
// NextReminder.
//
// Closed-app reminders: the Wails notifications service schedules natively on macOS (UNCalendarNotificationTrigger), so the Mac
// delivers them while the app is quit. It cannot list pending or delivered requests, so what was handed over is kept in
// planner.json and diffed against the plan. Without a .app bundle (go run) there is no notification service and everything
// notification-related does nothing; the in-page notice and the window still work.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

const (
	inAppReminderID  = "dailylog.reminder" // one in-app notification, replaced not stacked
	reminderThreadID = "gloamlog.reminders"
	reminderTick     = 30 * time.Second
	deliveredWindow  = 120 * time.Second // a request the Mac delivered this recently stands: no second notification
	launchedWindow   = 8 * time.Second   // an action in the first seconds of the process is what launched it
	plannerFileName  = "planner.json"
	legacyPlannerKey = "dailylog.planner.v1"
)

// ReminderNoticeState is what the slim notice on today's page needs. The page adds "viewing today" and "not dismissed".
type ReminderNoticeState struct {
	Visible      bool   `json:"visible"` // due, unlogged and not snoozed
	Strict       bool   `json:"strict"`
	Day          string `json:"day"`
	Time         string `json:"time"`        // "4:55 pm", the clock now
	SnoozeLabel  string `json:"snoozeLabel"` // "Remind me in 15 min · 2 left", "No snoozes left today"
	CanSnooze    bool   `json:"canSnooze"`
	ShowSkipHint bool   `json:"showSkipHint"` // Strict, after the 3rd re-open: "Not today?"
}

// reminderFile is planner.json.
type reminderFile struct {
	Planner   core.PlannerState      `json:"planner"`
	Scheduled []core.PlannedReminder `json:"scheduled"` // requests handed to the Mac, still in the future when last seen
	Snooze    snoozeRecord           `json:"snooze"`    // the one-off snooze request, if any
	Refused   bool                   `json:"refused"`   // the last permission request was answered no
}

type snoozeRecord struct {
	Day string    `json:"day"`
	At  time.Time `json:"at,omitzero"`
}

// reminderInput is what one tick reads from Notes.
type reminderInput struct {
	Settings core.Settings
	Cal      core.Calendar
	States   map[string]core.DayStatus
	Since    string
}

// reminderOS is everything the engine asks of the machine; tests fake it.
type reminderOS interface {
	Available() bool // a notification service exists (a .app bundle on macOS)
	Post(o notifications.NotificationOptions) error
	Remove(id string) // pending and delivered
	Check() (bool, error)
	Request() (bool, error)
	RegisterCategory(snoozeMinutes int) error
	RaiseMain(activate bool)
	MainVisible() bool
	HideMain()
	Emit(name string, data any)
}

type reminderEngine struct {
	mu     sync.Mutex
	snap   func() (reminderInput, error)
	skip   func(day string) error
	now    func() time.Time
	use24  func() bool
	os     reminderOS
	path   string
	legacy func() *core.PlannerState // the Swift app's planner, read once when planner.json is missing

	f            reminderFile
	allowed      bool
	asleep       bool
	force        bool // re-read what the Mac holds at the next tick
	started      time.Time
	catMinutes   int
	lastNotice   ReminderNoticeState
	lastStatus   core.DayStatus
	lastSettings string
	lastSig      string
}

// reminders is the running engine, nil before startReminders (the Notes methods then answer with nothing).
var reminders *reminderEngine

func startReminders(app *application.App, n *Notes) {
	w := &wailsOS{app: app}
	if bundled() {
		w.ns = notifications.New()
		app.RegisterService(application.NewService(w.ns)) // started by app.Run; a bare binary has no bundle id and would fail
	}
	support := filepath.Dir(n.settingsPath)
	e := &reminderEngine{
		snap:   n.reminderInput,
		skip:   func(day string) error { return n.Skip(day, "") },
		now:    n.now,
		use24:  macUse24Hour,
		os:     w,
		path:   filepath.Join(support, plannerFileName),
		legacy: legacyPlanner,
		force:  true,
	}
	reminders = e
	if w.ns != nil {
		w.ns.OnNotificationResponse(func(r notifications.NotificationResult) {
			if r.Error == nil {
				go e.handle(r.Response) // the Mac calls on the main thread, and window calls wait for it
			}
		})
	}
	prev := n.OnSettings
	n.OnSettings = func(s core.Settings) {
		if prev != nil {
			prev(s)
		}
		go e.tick()
	}
	ev := app.Event
	ev.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { go e.run() })
	ev.OnApplicationEvent(events.Mac.ApplicationDidBecomeActive, func(*application.ApplicationEvent) { go e.refreshAndTick() })
	ev.OnApplicationEvent(events.Mac.ApplicationDidResignActive, func(*application.ApplicationEvent) { go e.tick() })
	ev.OnApplicationEvent(events.Mac.ApplicationDidWake, func(*application.ApplicationEvent) {
		e.setAsleep(false)
		e.setForce()
		time.AfterFunc(15*time.Second, e.refreshAndTick) // let the unlock settle
	})
	ev.OnApplicationEvent(events.Mac.ApplicationScreensDidSleep, func(*application.ApplicationEvent) { e.setAsleep(true) })
	ev.OnApplicationEvent(events.Mac.ApplicationScreensDidWake, func(*application.ApplicationEvent) { e.setAsleep(false) })
	app.OnShutdown(e.tick) // logging or skipping today right before Quit still cancels what the Mac holds
}

// run is the app's start: grace after launch, the notification buttons, permission, then the 30 s tick.
func (e *reminderEngine) run() {
	e.refreshPermission()
	cal := core.NewCalendar(time.Local, 1) // PlannerAtLaunch only needs the zone
	if in, err := e.snap(); err == nil {
		cal = in.Cal
	}
	e.mu.Lock()
	e.load(cal)
	e.mu.Unlock()
	e.tick()
	for range time.Tick(reminderTick) {
		e.tick()
	}
}

func (e *reminderEngine) load(cal core.Calendar) {
	now := e.now()
	var prev *core.PlannerState
	if b, err := os.ReadFile(e.path); err == nil {
		var f reminderFile
		if json.Unmarshal(b, &f) == nil {
			e.f = f
			prev = &f.Planner
		}
	} else if errors.Is(err, os.ErrNotExist) && e.legacy != nil {
		prev = e.legacy()
	}
	e.f.Planner = core.PlannerAtLaunch(prev, now, cal)
	e.started = now
	e.save()
}

func (e *reminderEngine) setAsleep(v bool) { e.mu.Lock(); e.asleep = v; e.mu.Unlock() }
func (e *reminderEngine) setForce()        { e.mu.Lock(); e.force = true; e.mu.Unlock() }

func (e *reminderEngine) refreshAndTick() {
	e.refreshPermission()
	e.setForce()
	e.tick()
}

// refreshPermission asks the Mac (outside the lock: it can take a moment).
func (e *reminderEngine) refreshPermission() {
	if !e.os.Available() {
		return
	}
	ok, err := e.os.Check()
	if err != nil {
		return
	}
	e.mu.Lock()
	e.allowed = ok
	if ok && e.f.Refused {
		e.f.Refused = false
		e.save()
	}
	e.mu.Unlock()
}

// ---- the tick ----

func (e *reminderEngine) tick() {
	in, err := e.snap()
	if err != nil {
		return // the folder can't be read: nothing is known about today, so nothing nags
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.step(in)
}

// step is one tick: observe changes, decide, perform, tell the page, keep the Mac's requests equal to the plan.
func (e *reminderEngine) step(in reminderInput) {
	now := e.now()
	today := core.DayKeyString(now, time.Local)
	status := core.ResolveStatus(today, in.States[today], now, in.Cal, in.Settings.Weekdays, in.Since)
	before := e.f.Planner

	e.observe(now, in.Settings, status)
	if !e.asleep {
		action, next := core.ReminderDecide(now, in.Cal, in.Settings, status, e.f.Planner, e.os.MainVisible())
		e.f.Planner = next
		e.perform(action, now, in, today)
	}
	if e.f.Planner != before {
		e.save()
	}
	e.emitNotice(now, in, today, status)
	e.replan(now, in, today, status)
}

// observe turns a settings change and an undone skip into the planner's graces, and keeps the Snooze button's title current.
func (e *reminderEngine) observe(now time.Time, s core.Settings, status core.DayStatus) {
	key := fmt.Sprint(s.Mode, s.SnoozeMinutes, s.ReminderMinutes, s.Weekdays, s.ClosedAppReminders, s.LogStartDate)
	if e.lastSettings != "" && key != e.lastSettings {
		e.f.Planner = e.f.Planner.Suppressed(now.Add(core.ReminderGraceAfterSettingsChange))
		e.force = true
	}
	e.lastSettings = key
	if e.lastStatus == core.StatusSkipped && (status == core.StatusMissed || status == core.StatusPartial) {
		e.f.Planner = e.f.Planner.Suppressed(now.Add(core.ReminderGraceAfterUndoSkip))
	}
	e.lastStatus = status
	if e.os.Available() && e.catMinutes != s.SnoozeMinutes {
		if e.os.RegisterCategory(s.SnoozeMinutes) == nil {
			e.catMinutes = s.SnoozeMinutes
		}
	}
}

func (e *reminderEngine) perform(a core.ReminderAction, now time.Time, in reminderInput, today string) {
	switch a.Kind {
	case core.KindNotify:
		e.post(now, in, today)
	case core.KindBringToFront:
		if a.Notify {
			e.post(now, in, today)
		}
		e.os.RaiseMain(true)
		e.os.Emit("reminder-open-day", today)
		// ponytail: no Dock bounce (Wails has no requestUserAttention); add it when the lead exposes one.
	case core.KindReopen:
		e.os.RaiseMain(false)
	}
}

// post is the running app's own notification. A request the Mac delivered in the last two minutes stands; otherwise the
// scheduled ones for today are cancelled and this one replaces anything older. Content never holds log text.
func (e *reminderEngine) post(now time.Time, in reminderInput, today string) {
	if !e.os.Available() || e.deliveredRecently(now, today) {
		return
	}
	e.dropToday(today)
	title, body := inAppCopy(now, in.Settings.ReminderMinutes, in.Cal, e.use24())
	err := e.os.Post(notifications.NotificationOptions{
		ID: inAppReminderID, Title: title, Body: body, CategoryID: core.ReminderCategory,
		Data: map[string]any{"day": today}, ThreadID: reminderThreadID, Sound: &notifications.NotificationSound{Silent: true},
	})
	if err != nil { // the window and the page notice still remind; the log says why the banner is missing
		log.Printf("reminders: the notification for %s was not posted: %v", today, err)
	}
}

// inAppCopy: within 5 minutes of the reminder time "Time to write up your day", later "Today isn't logged yet".
func inAppCopy(now time.Time, reminderMinutes int, cal core.Calendar, use24 bool) (title, body string) {
	t := now.In(time.Local)
	if t.Hour()*60+t.Minute()-reminderMinutes > 5 {
		return "Today isn't logged yet", "It's " + core.ReminderTimeText(now, cal, use24) + ". Open Gloamlog whenever you're ready."
	}
	return "Time to write up your day", "It takes a few minutes. Gloamlog is ready when you are."
}

func (e *reminderEngine) deliveredRecently(now time.Time, today string) bool {
	recent := func(at time.Time) bool { return !at.After(now) && now.Sub(at) < deliveredWindow }
	for _, r := range e.f.Scheduled {
		if r.Day == today && recent(fireTime(r.Fire)) {
			return true
		}
	}
	return e.f.Snooze.Day == today && !e.f.Snooze.At.IsZero() && recent(e.f.Snooze.At)
}

// dropToday cancels today's end-of-day and snooze requests, pending and delivered.
func (e *reminderEngine) dropToday(today string) {
	for _, id := range []string{core.ReminderID(today), core.ReminderSnoozeID(today)} {
		e.os.Remove(id)
	}
	kept := e.f.Scheduled[:0:0]
	for _, r := range e.f.Scheduled {
		if r.Day != today {
			kept = append(kept, r)
		}
	}
	e.f.Scheduled = kept
	if e.f.Snooze.Day == today {
		e.f.Snooze = snoozeRecord{}
	}
	e.save()
}

func fireTime(f core.ReminderFire) time.Time {
	return time.Date(f.Year, time.Month(f.Month), f.Day, f.Hour, f.Minute, 0, 0, time.Local)
}

// ---- the page's notice ----

// noticeState is the notice for today, from the planner state of today.
func (e *reminderEngine) noticeState(now time.Time, in reminderInput, today string, status core.DayStatus) ReminderNoticeState {
	p := e.f.Planner
	if p.Day != today {
		p = core.PlannerState{}
	}
	s := in.Settings
	snoozed := !p.SnoozedUntil.IsZero() && now.Before(p.SnoozedUntil)
	n := ReminderNoticeState{
		Visible: core.ReminderIsPending(now, in.Cal, s, status) && !snoozed,
		Strict:  s.Mode == core.ModeStrict, Day: today, Time: core.ReminderTimeText(now, in.Cal, e.use24()),
		ShowSkipHint: s.Mode == core.ModeStrict && p.ShowSkipHint(),
	}
	label := fmt.Sprintf("Remind me in %d min", s.SnoozeMinutes)
	left, limited := core.ReminderSnoozesLeft(p, s)
	n.CanSnooze = !limited || left > 0
	switch {
	case limited && left == 0:
		label = "No snoozes left today"
	case limited:
		label += fmt.Sprintf(" · %d left", left)
	}
	n.SnoozeLabel = label
	return n
}

func (e *reminderEngine) emitNotice(now time.Time, in reminderInput, today string, status core.DayStatus) {
	n := e.noticeState(now, in, today, status)
	if n != e.lastNotice {
		e.lastNotice = n
		e.os.Emit("reminder-notice", n)
	}
}

// ---- closed-app plan ----

// replan keeps what the Mac holds equal to core.ReminderPlan. Silent when notifications aren't allowed: the running app reminds.
func (e *reminderEngine) replan(now time.Time, in reminderInput, today string, status core.DayStatus) {
	force := e.force
	e.force = false
	dayDone := status == core.StatusLogged || status == core.StatusSkipped
	if dayDone && e.hasToday(today) {
		e.dropToday(today) // logging or skipping today silences its request at once
	}
	if !e.os.Available() || !e.allowed {
		e.force = e.force || force // try again when permission arrives
		return
	}
	s := in.Settings
	planned := core.ReminderPlan(now, in.Cal, s, in.States, core.ReminderHorizon, e.use24())
	var sig strings.Builder
	fmt.Fprint(&sig, dayDone)
	for _, p := range planned {
		fmt.Fprintf(&sig, ";%s|%v|%s|%s", p.ID, p.Fire, p.Title, p.Body)
	}
	if !force && sig.String() == e.lastSig {
		return
	}
	var pending []core.PendingReminder
	kept := e.f.Scheduled[:0:0]
	for _, r := range e.f.Scheduled {
		if fireTime(r.Fire).After(now) { // a request in the past has fired
			f := r.Fire
			kept = append(kept, r)
			pending = append(pending, core.PendingReminder{ID: r.ID, Fire: &f, Title: r.Title, Body: r.Body, Category: r.Category})
		}
	}
	e.f.Scheduled = kept
	add, remove := core.ReminderDiff(planned, pending)
	for _, id := range remove {
		e.os.Remove(id)
		e.f.Scheduled = without(e.f.Scheduled, id)
	}
	for _, p := range add {
		err := e.os.Post(notifications.NotificationOptions{
			ID: p.ID, Title: p.Title, Body: p.Body, CategoryID: p.Category, Data: map[string]any{"day": p.Day},
			ThreadID: reminderThreadID, Sound: &notifications.NotificationSound{Silent: true},
			Schedule: &notifications.NotificationSchedule{At: fireTime(p.Fire).Unix()},
		})
		if err != nil {
			log.Printf("reminders: could not hand %s to the Mac: %v", p.ID, err)
			continue
		}
		e.f.Scheduled = append(without(e.f.Scheduled, p.ID), p)
	}
	if dayDone || !s.ClosedAppReminders { // today needs nothing left over: banner or snooze
		e.dropToday(today)
	}
	e.lastSig = sig.String()
	e.save()
}

func (e *reminderEngine) hasToday(today string) bool {
	for _, r := range e.f.Scheduled {
		if r.Day == today {
			return true
		}
	}
	return e.f.Snooze.Day == today
}

func without(list []core.PlannedReminder, id string) []core.PlannedReminder {
	out := list[:0:0]
	for _, r := range list {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return out
}

// ---- snooze, skip, notification buttons ----

// snooze holds the reminder; fromBanner also asks the Mac for a one-off request, which brings it back if Gloamlog is closed by then.
func (e *reminderEngine) snooze(now time.Time, in reminderInput, fromBanner bool) bool {
	next, ok := core.ReminderSnooze(e.f.Planner, now, in.Settings, in.Cal)
	if !ok {
		return false
	}
	e.f.Planner = next
	e.os.Remove(inAppReminderID)
	today := core.DayKeyString(now, time.Local)
	if until := next.SnoozedUntil; fromBanner && e.os.Available() && e.allowed && in.Settings.ClosedAppReminders && until.After(now) &&
		core.DayKeyString(until, time.Local) == today && until.In(time.Local).Hour()*60+until.In(time.Local).Minute() < core.ReminderCutoffMinutes {
		title, body := core.ReminderCopy(until, in.Cal, e.use24())
		err := e.os.Post(notifications.NotificationOptions{
			ID: core.ReminderSnoozeID(today), Title: title, Body: body, CategoryID: core.ReminderCategory,
			Data: map[string]any{"day": today}, ThreadID: reminderThreadID, Sound: &notifications.NotificationSound{Silent: true},
			Schedule: &notifications.NotificationSchedule{At: until.Unix()},
		})
		if err != nil {
			log.Printf("reminders: could not schedule the snooze for %s: %v", today, err)
		} else {
			e.f.Snooze = snoozeRecord{Day: today, At: until}
		}
	}
	e.save()
	e.emitNotice(now, in, today, e.lastStatus)
	return true
}

// handle answers a click on the banner or one of its buttons (Open, Snooze, Skip today).
func (e *reminderEngine) handle(r notifications.NotificationResponse) {
	in, err := e.snap()
	if err != nil {
		e.os.RaiseMain(true)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	today := core.DayKeyString(now, time.Local)
	day, _ := r.UserInfo["day"].(string)
	if !core.DayKeyIsWellFormed(day) {
		day = today
	}
	open := func() {
		e.os.RaiseMain(true)
		e.os.Emit("reminder-open-day", day)
	}
	if day != today { // an old banner: show the day it is about
		open()
		return
	}
	if e.f.Planner.Day != today {
		e.f.Planner = core.PlannerState{Day: today, GraceUntil: e.f.Planner.GraceUntil}
	}
	switch r.ActionIdentifier {
	case "snooze":
		if e.snooze(now, in, true) {
			e.hideIfLaunched(now)
			return
		}
	case "skip":
		if err := e.skip(day); err != nil { // writing exists: open the page instead
			open()
			return
		}
		e.dropToday(today)
		e.emitNotice(now, in, today, core.StatusSkipped)
		e.lastStatus = core.StatusSkipped
		e.hideIfLaunched(now)
		return
	}
	if !e.f.Planner.FirstFired { // they have seen it: no second one two minutes after a launch
		e.f.Planner.FirstFired = true
		e.f.Planner.LastFire = now
		e.save()
	}
	open()
}

// hideIfLaunched: Snooze and Skip today are not foreground actions, so a window that opened because of one is put away again.
func (e *reminderEngine) hideIfLaunched(now time.Time) {
	if now.Sub(e.started) >= launchedWindow {
		return
	}
	for _, d := range []time.Duration{200 * time.Millisecond, time.Second, 2500 * time.Millisecond} {
		time.AfterFunc(d, e.os.HideMain)
	}
}

// save writes planner.json atomically. A failure is logged (the state then lives on in memory, and the next change tries again);
// the temp file never stays behind.
func (e *reminderEngine) save() {
	if e.path == "" {
		return
	}
	if err := e.write(); err != nil {
		log.Printf("reminders: could not save %s: %v", e.path, err)
	}
}

func (e *reminderEngine) write() error {
	b, err := json.MarshalIndent(e.f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, e.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ---- the methods the UI calls ----

// reminderInput reads settings and every day's status.
func (n *Notes) reminderInput() (reminderInput, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	states, since, err := n.states()
	if err != nil {
		return reminderInput{}, err
	}
	return reminderInput{Settings: n.settings, Cal: n.cal(), States: states, Since: since}, nil
}

// CurrentReminderNotice is the slim notice now (the "reminder-notice" event carries every later change).
func (n *Notes) CurrentReminderNotice() ReminderNoticeState {
	e := reminders
	in, err := n.reminderInput()
	if e == nil || err != nil {
		return ReminderNoticeState{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	today := core.DayKeyString(now, time.Local)
	return e.noticeState(now, in, today, core.ResolveStatus(today, in.States[today], now, in.Cal, in.Settings.Weekdays, in.Since))
}

// SnoozeReminder is the notice's snooze button (⇧⌘L, the menu bar). It returns the notice as it is afterwards; Strict allows 2 a day.
func (n *Notes) SnoozeReminder() ReminderNoticeState {
	if e := reminders; e != nil {
		if in, err := n.reminderInput(); err == nil {
			e.mu.Lock()
			e.snooze(e.now(), in, false)
			e.mu.Unlock()
		}
	}
	return n.CurrentReminderNotice()
}

// SkipToday writes today's skip marker (no reason) and cancels what the Mac holds for today. Refused when today has writing.
func (n *Notes) SkipToday() error {
	now := time.Now()
	if reminders != nil {
		now = reminders.now()
	}
	if err := n.Skip(core.DayKeyString(now, time.Local), ""); err != nil {
		return err
	}
	if reminders != nil {
		go reminders.tick()
	}
	return nil
}

// NotificationStatus is "allowed", "denied" or "unknown" (not asked yet, or no notification service in this build).
func (n *Notes) NotificationStatus() string {
	e := reminders
	if e == nil || !e.os.Available() {
		return "unknown"
	}
	e.refreshPermission()
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case e.allowed:
		return "allowed"
	case e.f.Refused:
		return "denied"
	}
	return "unknown"
}

// RequestNotifications asks macOS for permission (it shows the system prompt once) and returns the new status.
func (n *Notes) RequestNotifications() string {
	e := reminders
	if e == nil || !e.os.Available() {
		return "unknown"
	}
	ok, err := e.os.Request()
	e.mu.Lock()
	e.allowed = ok
	e.f.Refused = !ok && err == nil
	e.force = true
	e.save()
	e.mu.Unlock()
	go e.tick()
	return n.NotificationStatus()
}

// NextReminder is "Thu 8 Oct, 4:55 pm", the first reminder of the plan the Mac holds, or "none planned".
func (n *Notes) NextReminder() string {
	in, err := n.reminderInput()
	if err != nil {
		return "none planned"
	}
	now := time.Now()
	if reminders != nil {
		now = reminders.now()
	}
	use24 := macUse24Hour()
	plan := core.ReminderUpcoming(now, in.Cal, in.Settings, in.States, core.ReminderHorizon, use24)
	if at, ok := core.ReminderNext(plan, now, in.Cal); ok {
		return core.ReminderLabel(at, in.Cal, use24)
	}
	return "none planned"
}

// ---- the machine ----

type wailsOS struct {
	app *application.App
	ns  *notifications.NotificationService // nil without a .app bundle
}

func (w *wailsOS) Available() bool { return w.ns != nil }

func (w *wailsOS) Post(o notifications.NotificationOptions) error {
	if o.CategoryID != "" {
		return w.ns.SendNotificationWithActions(o)
	}
	return w.ns.SendNotification(o)
}

func (w *wailsOS) Remove(id string) {
	if w.ns != nil {
		// Best effort: a request that is not there (already fired or never added) is the normal case, and a failed
		// removal at worst leaves one banner that the next re-plan removes again.
		_ = w.ns.RemovePendingNotification(id)
		_ = w.ns.RemoveDeliveredNotification(id)
	}
}

func (w *wailsOS) Check() (bool, error)   { return w.ns.CheckNotificationAuthorization() }
func (w *wailsOS) Request() (bool, error) { return w.ns.RequestNotificationAuthorization() }

func (w *wailsOS) RegisterCategory(snoozeMinutes int) error {
	return w.ns.RegisterNotificationCategory(notifications.NotificationCategory{ID: core.ReminderCategory, Actions: []notifications.NotificationAction{
		{ID: "open", Title: "Open"},
		{ID: "snooze", Title: fmt.Sprintf("Snooze %d min", snoozeMinutes)},
		{ID: "skip", Title: "Skip today"},
	}})
}

// RaiseMain shows the main window. activate=false only orders it in, so another app keeps the keyboard.
func (w *wailsOS) RaiseMain(activate bool) {
	win, ok := w.app.Window.GetByName("main")
	if !ok {
		return
	}
	if win.IsMinimised() {
		win.UnMinimise()
	}
	win.Show()
	if activate {
		win.Focus()
	}
}

func (w *wailsOS) MainVisible() bool {
	win, ok := w.app.Window.GetByName("main")
	return ok && win.IsVisible() && !win.IsMinimised()
}

func (w *wailsOS) HideMain() {
	if win, ok := w.app.Window.GetByName("main"); ok {
		win.Hide()
	}
}

func (w *wailsOS) Emit(name string, data any) { w.app.Event.Emit(name, data) }

// bundled is true when running from Gloamlog.app: macOS only has a notification center for a bundled app.
func bundled() bool {
	exe, err := os.Executable()
	return runtime.GOOS == "darwin" && err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

var (
	use24Mu   sync.Mutex
	use24At   time.Time
	use24Last bool
)

// macUse24Hour reads the Mac's clock style (System Settings > General > Date & Time > 24-hour time); read at most every 5 minutes.
func macUse24Hour() bool {
	use24Mu.Lock()
	defer use24Mu.Unlock()
	if time.Since(use24At) < 5*time.Minute {
		return use24Last
	}
	// The key is absent on most Macs (defaults then exits non-zero): that means "follow the region", read here as 12-hour.
	out, _ := exec.Command("/usr/bin/defaults", "read", "-g", "AppleICUForce24HourTime").Output()
	use24Last = strings.TrimSpace(string(out)) == "1"
	use24At = time.Now()
	return use24Last
}

// legacyPlanner reads the Swift app's dailylog.planner.v1 value: only the day and the snooze count matter, which is all
// PlannerAtLaunch keeps. (Its dates are Swift's seconds-since-2001 and are not read.)
func legacyPlanner() *core.PlannerState {
	data, ok := readLegacyDefaults(bundleID)[legacyPlannerKey].([]byte)
	if !ok {
		return nil
	}
	var p struct {
		Day         string `json:"day"`
		SnoozesUsed int    `json:"snoozesUsed"`
	}
	if json.Unmarshal(data, &p) != nil || p.Day == "" {
		return nil
	}
	return &core.PlannerState{Day: p.Day, SnoozesUsed: p.SnoozesUsed}
}
