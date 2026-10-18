package main

// The menu bar item: an icon that says whether today is logged, and a menu with today's state, Jot, Open Gloamlog, Today,
// "Copy this week's summary", Settings and Quit. A native menu replaces the Swift popover (no Jot field, streak line or week
// strip in it; the Jot panel is one click away). The menu re-reads the log every 30 s and when "refresh-menubar" arrives
// (capture.go emits it after every jot, delivery and page save).
//
// Events emitted: "open-today" {}   Today was chosen: the main window is shown, its page should open today.
// Needs from main.go: the showMain, showJot, showSettings variables, and the tray (see setupMenuBar).

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const menuRefresh = 30 * time.Second

// menuIcon is which tray icon to show.
type menuIcon string

const (
	iconNotLogged menuIcon = "notLogged" // today still to write
	iconLogged    menuIcon = "logged"    // nothing owed: logged, skipped or not a workday
	iconProblem   menuIcon = "problem"   // the notes folder is unavailable
)

// menuView is what the tray shows right now.
type menuView struct {
	Icon   menuIcon
	Status string // the first menu line
}

// menuViewFor reads today's state. skipReason is only used when the day is skipped.
func menuViewFor(o Overview, skipReason string) menuView {
	if o.FolderError != "" {
		return menuView{iconProblem, "Saving problem: folder unavailable"}
	}
	switch o.TodayStatus {
	case core.StatusLogged:
		return menuView{iconLogged, "Logged today ✓"}
	case core.StatusSkipped:
		if skipReason != "" {
			return menuView{iconLogged, "Skipped today (" + skipReason + ")"}
		}
		return menuView{iconLogged, "Skipped today"}
	case core.StatusOff:
		return menuView{iconLogged, "Not a workday"}
	}
	return menuView{iconNotLogged, "Not logged yet"}
}

// currentMenuView reads the overview (and the skip reason when needed) from n.
func currentMenuView(n *Notes) menuView {
	o, err := n.Overview()
	if err != nil && o.FolderError == "" {
		o.FolderError = err.Error() // the folder could not be read: same state as a missing one
	}
	reason := ""
	if o.TodayStatus == core.StatusSkipped {
		if p, err := n.Open(o.Today); err == nil {
			reason = p.SkipReason
		}
	}
	return menuViewFor(o, reason)
}

// ---- icons: template images drawn here (black on transparent, macOS tints them), 44 px for a 22 pt bar ----

var (
	iconMu    sync.Mutex
	iconCache = map[menuIcon][]byte{}
)

// menuIconPNG is the tray image for an icon state.
func menuIconPNG(k menuIcon) []byte {
	iconMu.Lock()
	defer iconMu.Unlock()
	if b, ok := iconCache[k]; ok {
		return b
	}
	b := drawMenuIcon(k)
	iconCache[k] = b
	return b
}

// segDist is the distance from (px,py) to the segment a-b.
func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// drawMenuIcon draws on a 0..1 square, 4x supersampled for soft edges.
func drawMenuIcon(k menuIcon) []byte {
	const size, ss = 44, 4
	const w = 0.07 // stroke half-width
	inside := func(x, y float64) bool {
		switch k {
		case iconProblem: // a triangle outline with an exclamation mark
			ax, ay, bx, by, cx, cy := 0.5, 0.12, 0.92, 0.84, 0.08, 0.84
			if segDist(x, y, ax, ay, bx, by) < w || segDist(x, y, bx, by, cx, cy) < w || segDist(x, y, cx, cy, ax, ay) < w {
				return true
			}
			return segDist(x, y, 0.5, 0.42, 0.5, 0.62) < w || math.Hypot(x-0.5, y-0.74) < w*1.15
		case iconLogged: // a ring with a tick
			if math.Abs(math.Hypot(x-0.5, y-0.5)-0.4) < w {
				return true
			}
			return segDist(x, y, 0.3, 0.52, 0.45, 0.67) < w || segDist(x, y, 0.45, 0.67, 0.72, 0.36) < w
		}
		return math.Abs(math.Hypot(x-0.5, y-0.5)-0.4) < w // a plain ring
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			n := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					if inside((float64(px)+(float64(sx)+0.5)/ss)/size, (float64(py)+(float64(sy)+0.5)/ss)/size) {
						n++
					}
				}
			}
			if n > 0 {
				img.SetNRGBA(px, py, color.NRGBA{0, 0, 0, uint8(255 * n / (ss * ss))})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // writing to memory cannot fail
	return buf.Bytes()
}

// ---- the menu ----

// setupMenuBar builds the tray menu on tray and keeps it current: every 30 s and on the "refresh-menubar" event.
// main.go makes the tray (app.SystemTray.New()), sets nothing on it, and calls this once after showMain, showJot and
// showSettings are assigned.
func setupMenuBar(app *application.App, n *Notes, tray *application.SystemTray) {
	menu := app.NewMenu()
	status := menu.Add("Not logged yet").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Jot…").OnClick(func(*application.Context) { showJot() })
	menu.Add("Open Gloamlog").OnClick(func(*application.Context) { showMain() })
	menu.Add("Today").OnClick(func(*application.Context) {
		showMain()
		app.Event.Emit("open-today")
	})
	menu.Add("Copy this week's summary").OnClick(func(*application.Context) {
		go func() {
			if w, err := n.Week(core.DayKeyString(n.now(), time.Local), 0, false); err == nil {
				app.Clipboard.SetText(w.Markdown)
			}
		}()
	})
	menu.AddSeparator()
	menu.Add("Settings…").OnClick(func(*application.Context) { showSettings() })
	menu.AddSeparator()
	menu.Add("Quit Gloamlog").OnClick(func(*application.Context) { app.Quit() })

	first := menuIconPNG(iconNotLogged)
	tray.SetTemplateIcon(first)
	tray.SetMenu(menu)

	var mu sync.Mutex
	shown := menuIcon("")
	refresh := func() {
		v := currentMenuView(n)
		mu.Lock()
		defer mu.Unlock()
		application.InvokeSync(func() {
			status.SetLabel(v.Status)
			if v.Icon != shown {
				shown = v.Icon
				tray.SetTemplateIcon(menuIconPNG(v.Icon))
			}
		})
	}
	go refresh()
	app.Event.On("refresh-menubar", func(*application.CustomEvent) { go refresh() })
	go func() {
		for range time.Tick(menuRefresh) {
			refresh()
		}
	}()
}
