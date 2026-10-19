package main

// A time-zone change while the app runs (travel, or a change in System Settings). Go reads the zone once at start, so day
// keys and times would stay in the old zone; the Swift app re-read it on NSSystemTimeZoneDidChange. Every minute the zone
// macOS uses (/etc/localtime) is compared with the one in use; on a change the app's zone (appZone, read wherever the app
// needs local time) becomes the new one and the windows reload. time.Local itself is never written: other goroutines read it.

import (
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const zoneTick = time.Minute

var zone atomic.Pointer[time.Location] // set on a zone change; nil until then

// appZone is the zone the app uses for days and times: the Mac's zone at start, then the newest one watchTimeZone saw.
func appZone() *time.Location {
	if l := zone.Load(); l != nil {
		return l
	}
	return time.Local
}

// zoneName is the IANA name a localtime link points to ("Asia/Kolkata"), or "" when it can't be read.
func zoneName(link string) string {
	target, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	if i := strings.Index(target, "zoneinfo/"); i >= 0 {
		return target[i+len("zoneinfo/"):]
	}
	return ""
}

// watchTimeZone follows the Mac's time zone while the app runs. main.go calls it once.
func watchTimeZone(app *application.App) {
	current := zoneName("/etc/localtime")
	go func() {
		for range time.Tick(zoneTick) {
			name := zoneName("/etc/localtime")
			if name == "" || name == current {
				continue
			}
			loc, err := time.LoadLocation(name)
			if err != nil {
				continue
			}
			current = name
			zone.Store(loc)
			app.Event.Emit("folder-changed")  // the main window reads today, the calendar and the page again
			app.Event.Emit("refresh-menubar") // and the menu bar its state
		}
	}()
}
