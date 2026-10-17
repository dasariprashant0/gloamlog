// Gloamlog desktop app: one Go process (Wails v3) with a main window, a
// quick-jot panel and a menu bar item. All note logic lives in
// internal/core; this package only wires windows, menus and services.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

// The built frontend (npm run build in desktop/frontend).
//
//go:embed all:frontend/dist
var assets embed.FS

// Same id as the Swift app, so macOS treats it as the same app
// (settings import, notifications, the gloamlog:// URL scheme).
const bundleID = "io.github.dasariprashant0.gloamlog"

// Set by build.sh (-ldflags -X main.version=...); "dev" under go run.
var version = "dev"

// Set in main once the windows exist; menus, shortcuts and services call them.
var showMain, showJot, showSettings, hideJot func()

// Windows lets any page open or close the app's other windows.
type Windows struct{}

func (Windows) ShowMain()     { showMain() }
func (Windows) ShowJot()      { showJot() }
func (Windows) HideJot()      { hideJot() }
func (Windows) ShowSettings() { showSettings() }

func main() {
	notes, err := NewNotes()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name:        "Gloamlog",
		Description: "A work journal: one markdown file per day.",
		Services:    []application.Service{application.NewService(notes), application.NewService(Windows{})},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: notes.serveAssets, // the editor's images: /dlasset/assets/<file>
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			// Closing the window keeps the app in the menu bar, for reminders and Jot.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               bundleID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { showMain() },
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Gloamlog",
		Width:     1080,
		Height:    760,
		MinWidth:  860,
		MinHeight: 600,
		URL:       "/",
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 44,
		},
	})
	// Closing hides: the app keeps running in the menu bar.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})

	jot := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "jot",
		Title:           "Jot",
		Width:           560,
		Height:          240,
		URL:             "/jot.html",
		Hidden:          true,
		Frameless:       true,
		AlwaysOnTop:     true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		InitialPosition: application.WindowCentered,
		Mac: application.MacWindow{
			WindowClass:        application.MacWindowClassPanel,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces | application.MacWindowCollectionBehaviorFullScreenAuxiliary,
			WindowLevel:        application.MacWindowLevelFloating,
		},
	})

	settings := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "settings",
		Title:     "Settings",
		Width:     720,
		Height:    560,
		MinWidth:  640,
		MinHeight: 480,
		URL:       "/settings.html",
		Hidden:    true,
		Mac:       application.MacWindow{TitleBar: application.MacTitleBarHiddenInset, InvisibleTitleBarHeight: 44},
	})
	settings.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		settings.Hide()
		e.Cancel()
	})

	showMain = func() { win.Show(); win.Focus() }
	showJot = func() { jot.Show(); jot.Focus() }
	showSettings = func() { settings.Show(); settings.Focus() }
	hideJot = func() { jot.Hide() }

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(icons.SystrayMacTemplate)
	menu := app.NewMenu()
	menu.Add("Open Gloamlog").OnClick(func(*application.Context) { showMain() })
	menu.Add("Jot…").OnClick(func(*application.Context) { showJot() })
	menu.Add("Settings…").OnClick(func(*application.Context) { showSettings() })
	menu.AddSeparator()
	menu.Add("Quit Gloamlog").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
