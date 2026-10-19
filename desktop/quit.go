package main

// Quitting with edits that couldn't be saved (the Swift app's "Your latest edits couldn't be saved" alert). The day page says
// whether it holds text it couldn't write ("unsaved-edits", true / false: a failed save or a page left behind unsaved). While
// it does, a quit is held: the main window comes forward and the page asks ("quit-blocked": Try again / Copy my text / Quit
// anyway). "quit-anyway" from the page quits for real.
//
// Every quit path asks shouldQuit: macOS's own (⌘Q, the Dock, logging out) through application.Options.ShouldQuit, and the
// menu bar's Quit item through requestQuit (app.Quit itself would not ask).

import (
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var unsavedEdits, quitAnyway atomic.Bool

// shouldQuit is false while the page holds unsaved text; it then shows the main window and asks there.
func shouldQuit() bool {
	if quitAnyway.Load() || !unsavedEdits.Load() {
		return true
	}
	if showMain != nil {
		showMain()
	}
	if app := application.Get(); app != nil {
		app.Event.Emit("quit-blocked")
	}
	return false
}

// requestQuit quits unless shouldQuit holds it (the menu bar's Quit item).
func requestQuit(app *application.App) {
	if shouldQuit() {
		app.Quit()
	}
}

// watchQuit listens to the page: its unsaved state, and "Quit anyway".
func watchQuit(app *application.App) {
	app.Event.On("unsaved-edits", func(e *application.CustomEvent) {
		b, _ := e.Data.(bool)
		unsavedEdits.Store(b)
	})
	app.Event.On("quit-anyway", func(*application.CustomEvent) {
		quitAnyway.Store(true)
		app.Quit()
	})
}
