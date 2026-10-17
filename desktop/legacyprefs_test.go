package main

import (
	"encoding/base64"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// A synthetic `defaults export` of the Swift app: the settings JSON as <data>, plus a bool.
func TestLegacyPlistImport(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString([]byte(`{"reminderMinutes":1080,"weekdays":[2,3,4],"minWords":35,"onboarded":true,` +
		`"storageFolder":"file:\/\/\/Users\/someone\/Notes\/","capture":{"jotsHeading":"Notes"}}`))
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>dailylog.settings.v2</key>
	<data>
	` + blob[:20] + `
	` + blob[20:] + `
	</data>
	<key>showMenuBar</key>
	<false/>
	<key>review.scale</key>
	<string>month</string>
	<key>dailylog.sources.hiddenDays</key>
	<array><string>2026-10-01</string></array>
	<key>remindMinutes</key>
	<integer>900</integer>
</dict>
</plist>`
	m, ok := parsePlist([]byte(plist)).(map[string]any)
	if !ok {
		t.Fatal("not a dict")
	}
	if m["review.scale"] != "month" || m["showMenuBar"] != false || m["remindMinutes"] != int64(900) {
		t.Fatalf("values: %#v", m)
	}
	if a, _ := m["dailylog.sources.hiddenDays"].([]any); len(a) != 1 || a[0] != "2026-10-01" {
		t.Fatalf("array: %#v", m["dailylog.sources.hiddenDays"])
	}
	s, imported := core.ImportLegacyDefaults(m, testHome)
	if !imported {
		t.Fatal("nothing imported")
	}
	if s.ReminderMinutes != 1080 || s.MinWords != 35 || !s.Onboarded || s.ShowMenuBar || s.Capture.JotsHeading != "Notes" {
		t.Fatalf("settings: %+v", s)
	}
	if s.StorageFolder != "/Users/someone/Notes" {
		t.Fatalf("folder: %q", s.StorageFolder)
	}
}

// No old app, or output we can't read: nothing is imported and the defaults stand.
func TestLegacyPlistEmptyOrBroken(t *testing.T) {
	for _, in := range []string{"", "not xml at all", "<plist><dict><key>x</key>", `<plist version="1.0"><dict/></plist>`} {
		m, _ := parsePlist([]byte(in)).(map[string]any)
		s, imported := core.ImportLegacyDefaults(m, testHome)
		if imported || s.ReminderMinutes != core.DefaultSettings(testHome).ReminderMinutes {
			t.Fatalf("%q: imported=%v settings=%+v", in, imported, s)
		}
	}
}
