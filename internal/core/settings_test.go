package core

// Ported from tests/main.swift: "settings defaults, round trip, tolerant decode, normalize", "settings migration from v0.2 JSON",
// "settings M1 fields", "default storage folder". The Swift tests went through a MemoryStore/UserDefaults; here the same cases go
// through DecodeSettings (the JSON blob) and the settings file. Import from the old UserDefaults is new and tested at the end.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const home = "/Users/test"

func TestSettingsDefaultsRoundTripTolerantDecodeNormalize(t *testing.T) {
	d := DefaultSettings(home)
	expect(t, d.Mode == ModeStrict && d.MinWords == 20 && equal(d.CarryOverHeadings, []string{"To do next", "Pending / blocked"}) && d.ReminderMinutes == 16*60+55, "defaults")
	expect(t, d.Template == "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next", "default template")
	expect(t, d.SnoozeMinutes == 15 && equal(d.Weekdays, workdays) && d.LaunchAtLogin && !d.Onboarded && d.ClosedAppReminders && d.ShowMenuBar, "more defaults")
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	expect(t, equal(LoadSettings(path, home), DefaultSettings(home)), "defaults when the file is missing")
	s := DefaultSettings(home)
	s.Mode, s.ReminderMinutes, s.Weekdays, s.SnoozeMinutes = ModeGentle, 17*60, WeekdaySet{2, 4}, 30
	s.StorageFolder, s.Onboarded, s.LaunchAtLogin = "/tmp/x", true, false
	s.Template, s.CarryOverHeadings, s.MinWords = "## Wins\n\n## Next", []string{"Next"}, 50
	must(t, s.Save(path))
	expect(t, equal(LoadSettings(path, home), s), "round trip through the file")
	t2 := DecodeSettings([]byte(`{"mode":"gentle"}`), home)
	expect(t, t2.Mode == ModeGentle && t2.ReminderMinutes == 16*60+55 && equal(t2.Weekdays, workdays) && t2.Template == DefaultTemplate && t2.MinWords == 20, "missing keys fall back to defaults")
	expect(t, equal(DecodeSettings([]byte("garbage"), home), DefaultSettings(home)), "garbage gives the defaults")
	expect(t, equal(DecodeSettings([]byte("null"), home), DefaultSettings(home)) && equal(DecodeSettings([]byte("[]"), home), DefaultSettings(home)), "not an object")
	expect(t, equal(DecodeSettings([]byte(`{"minWords":"many"}`), home), DefaultSettings(home)), "a wrong type for a known key fails the whole decode")
	expect(t, equal(DecodeSettings([]byte(`{"mode":"loud"}`), home), DefaultSettings(home)), "an unknown mode fails the decode")

	bad := DefaultSettings(home)
	bad.Weekdays, bad.SnoozeMinutes, bad.ReminderMinutes, bad.MinWords = WeekdaySet{}, 7, 5000, 0
	bad.CarryOverHeadings = []string{"  Next  ", "", "   "}
	n := bad.Normalized()
	expect(t, equal(n.Weekdays, workdays) && n.SnoozeMinutes == 15 && n.ReminderMinutes == 1439 && n.MinWords == 1 && equal(n.CarryOverHeadings, []string{"Next"}), "normalized %+v", n)
	bad.MinWords = 9999
	expect(t, bad.Normalized().MinWords == 500, "max 500")
	bad.MinWords = -3
	expect(t, bad.Normalized().MinWords == 1, "min 1")
	bad.Weekdays = WeekdaySet{7, 0, 3, 3, 9}
	expect(t, equal(bad.Normalized().Weekdays, WeekdaySet{3, 7}), "weekdays outside 1...7 dropped, sorted, unique")

	empty := DefaultSettings(home)
	empty.Template, empty.CarryOverHeadings = "", []string{}
	must(t, empty.Save(path))
	e := LoadSettings(path, home)
	expect(t, e.Template == "" && len(e.CarryOverHeadings) == 0, "an empty template / no carry-over headings are allowed")
}

func TestSettingsMigrationFromV02JSON(t *testing.T) {
	old := `{"mode":"gentle","reminderMinutes":1020,"weekdays":[2,3],"snoozeMinutes":20,"launchAtLogin":false,"onboarded":true,
 "storageFolder":"file:///tmp/old-logs/","carryOverSectionID":"pending",
 "sections":[{"id":"did","title":"📝 What I did","hint":"h","required":true},{"id":"x","title":"  Wins  ","hint":"","required":false},
             {"id":"y","title":"   ","hint":"","required":true},{"id":"todo","title":"📌 To do next","hint":"","required":false}]}`
	s := DecodeSettings([]byte(old), home)
	expect(t, s.Template == "## 📝 What I did\n\n## Wins\n\n## 📌 To do next", "template %q", s.Template)
	expect(t, s.Mode == ModeGentle && s.ReminderMinutes == 1020 && equal(s.Weekdays, WeekdaySet{2, 3}) && s.SnoozeMinutes == 20 && s.Onboarded && !s.LaunchAtLogin, "carried over")
	expect(t, s.StorageFolder == "/tmp/old-logs" && s.MinWords == 20 && equal(s.CarryOverHeadings, DefaultCarryOverHeadings()), "folder %q", s.StorageFolder)
	var norms []string
	for _, h := range TemplateHeadings(s.Template) {
		norms = append(norms, NormalizeHeading(h))
	}
	expect(t, equal(norms, []string{"what i did", "wins", "to do next"}), "headings %v", norms)
	path := filepath.Join(t.TempDir(), "settings.json")
	must(t, s.Save(path))
	raw, err := os.ReadFile(path)
	must(t, err)
	data := string(raw)
	expect(t, !strings.Contains(data, `"sections"`) && !strings.Contains(data, "carryOverSectionID") && strings.Contains(data, `"template"`), "re-saved in the new shape")
	expect(t, equal(LoadSettings(path, home), s), "round trip")
	// an explicit template wins over old sections; empty/blank sections fall back to the default
	expect(t, DecodeSettings([]byte(`{"template":"## Mine","sections":[{"id":"a","title":"Old"}]}`), home).Template == "## Mine", "template wins")
	for _, in := range []string{`{"sections":[]}`, `{"sections":[{"id":"a","title":"  "}]}`, `{"sections":"nonsense"}`} {
		expect(t, DecodeSettings([]byte(in), home).Template == DefaultTemplate, "%s keeps the default template", in)
	}
}

func TestSettingsM1Fields(t *testing.T) {
	d := DefaultSettings(home)
	expect(t, d.LogStartDate == "" && d.Appearance == AppearanceSystem && d.WeekStart == 0 && d.CatchUpWindowDays == 30, "defaults")
	path := filepath.Join(t.TempDir(), "settings.json")
	s := DefaultSettings(home)
	s.LogStartDate, s.Appearance, s.WeekStart, s.CatchUpWindowDays = "2026-10-01", AppearanceDark, 2, 90
	must(t, s.Save(path))
	back := LoadSettings(path, home)
	expect(t, back.LogStartDate == "2026-10-01" && back.Appearance == AppearanceDark && back.WeekStart == 2 && back.CatchUpWindowDays == 90, "round trip")
	// a v0.3 save (none of the new keys) decodes to the defaults
	o := DecodeSettings([]byte(`{"reminderMinutes":1020,"minWords":25,"onboarded":true}`), home)
	expect(t, o.MinWords == 25 && o.Onboarded && o.LogStartDate == "" && o.Appearance == AppearanceSystem && o.CatchUpWindowDays == 30, "v0.3 save")
	// clamping and validation
	bad := DefaultSettings(home)
	bad.CatchUpWindowDays, bad.LogStartDate, bad.WeekStart = 3, "garbage", 9
	n := bad.Normalized()
	expect(t, n.CatchUpWindowDays == 7 && n.LogStartDate == "" && n.WeekStart == 0, "invalid values cleared: %+v", n)
	bad.CatchUpWindowDays, bad.WeekStart, bad.LogStartDate = 99999, 2, "2026-09-30"
	m := bad.Normalized()
	expect(t, m.CatchUpWindowDays == 365 && m.WeekStart == 2 && m.LogStartDate == "2026-09-30", "valid values kept: %+v", m)
}

func TestDefaultStorageFolderIsGloamlog(t *testing.T) {
	expect(t, filepath.Base(DefaultFolder(home)) == "Gloamlog" && DefaultSettings(home).StorageFolder == "/Users/test/Gloamlog", "default folder")
}

// ---- not in the Swift suite: importing the old UserDefaults ----

// A settings blob exactly as the Swift JSONEncoder wrote it (escaped slashes, URL folder, hotKey omitted because it was nil...).
const swiftBlob = `{"reminderMinutes":1020,"weekdays":[2,3,4,5,6],"mode":"gentle","snoozeMinutes":10,` +
	`"storageFolder":"file:\/\/\/Users\/ana\/Documents\/Logs\/","launchAtLogin":false,"template":"## Wins","carryOverHeadings":["Next"],` +
	`"minWords":30,"onboarded":true,"logStartDate":"2026-09-01","appearance":"dark","weekStart":2,"catchUpWindowDays":14,` +
	`"capture":{"timestamps":false,"todoShorthand":true,"jotsHeading":"Notes","jotsCountTowardLogged":true,"hotKey":{"keyCode":40,"carbonModifiers":6144}},` +
	`"closedAppReminders":false,"sources":{"claude":true,"git":false,"consented":true,"allowed":["\/Users\/ana\/proj"],"allProjects":false,"autoAdd":true,"gitRoots":[],"heading":"Done"}}`

func TestImportLegacyDefaults(t *testing.T) {
	// the plist <data> value arrives as []byte
	s, ok := ImportLegacyDefaults(map[string]any{LegacySettingsKey: []byte(swiftBlob), "showMenuBar": false, "unrelated": 1}, home)
	expect(t, ok, "found")
	expect(t, s.ReminderMinutes == 1020 && s.Mode == ModeGentle && s.SnoozeMinutes == 10 && s.StorageFolder == "/Users/ana/Documents/Logs", "basics: %+v", s)
	expect(t, !s.LaunchAtLogin && s.Template == "## Wins" && equal(s.CarryOverHeadings, []string{"Next"}) && s.MinWords == 30 && s.Onboarded, "more")
	expect(t, s.LogStartDate == "2026-09-01" && s.Appearance == AppearanceDark && s.WeekStart == 2 && s.CatchUpWindowDays == 14 && !s.ClosedAppReminders, "M1")
	expect(t, s.Capture.HotKey != nil && s.Capture.HotKey.KeyCode == 40 && !s.Capture.Timestamps && s.Capture.JotsHeading == "Notes" && s.Capture.JotsCountTowardLogged, "capture %+v", s.Capture)
	expect(t, s.Sources.Claude && !s.Sources.Git && s.Sources.Consented && s.Sources.AutoAdd && equal(s.Sources.Allowed, []string{"/Users/ana/proj"}) && s.Sources.Heading == "Done", "sources %+v", s.Sources)
	expect(t, !s.ShowMenuBar, "showMenuBar comes from its own key")
	// a string or an already decoded map works too
	s2, ok := ImportLegacyDefaults(map[string]any{LegacySettingsKey: swiftBlob}, home)
	expect(t, ok && equal(s2.Normalized(), func() Settings { x := s; x.ShowMenuBar = true; return x }()), "string blob")
	var m map[string]any
	must(t, json.Unmarshal([]byte(swiftBlob), &m))
	s3, ok := ImportLegacyDefaults(map[string]any{LegacySettingsKey: m}, home)
	expect(t, ok && s3.MinWords == 30, "decoded map")
	// the v0.1 key
	v1, ok := ImportLegacyDefaults(map[string]any{"remindMinutes": int64(1000)}, home)
	expect(t, ok && v1.ReminderMinutes == 1000 && v1.Onboarded && equal(v1.Weekdays, workdays), "v0.1 settings: %+v", v1)
	v1b, _ := ImportLegacyDefaults(map[string]any{"remindMinutes": 5000.0}, home)
	expect(t, v1b.ReminderMinutes == 1439, "normalized")
	// the blob wins over the v0.1 key; nothing found = defaults
	both, _ := ImportLegacyDefaults(map[string]any{LegacySettingsKey: []byte(`{"minWords":44}`), "remindMinutes": 1000}, home)
	expect(t, both.MinWords == 44 && both.ReminderMinutes == 16*60+55 && !both.Onboarded, "blob wins")
	none, ok := ImportLegacyDefaults(map[string]any{"unrelated": true}, home)
	expect(t, !ok && equal(none, DefaultSettings(home)), "fresh install")
	broken, ok := ImportLegacyDefaults(map[string]any{LegacySettingsKey: []byte("garbage")}, home)
	expect(t, ok && equal(broken, DefaultSettings(home)), "a damaged blob gives the defaults")
}

func TestHotKeyOffSurvivesASave(t *testing.T) {
	// Swift omitted a nil hotKey when encoding and read a missing one as "default shortcut", so turning the shortcut off was lost
	// on the next launch. Go writes an explicit null; reading still follows the Swift rule (missing = default, null = off).
	s := DefaultSettings(home)
	s.Capture.HotKey = nil
	path := filepath.Join(t.TempDir(), "settings.json")
	must(t, s.Save(path))
	expect(t, LoadSettings(path, home).Capture.HotKey == nil, "off stays off")
	expect(t, DecodeSettings([]byte(`{"capture":{"timestamps":false}}`), home).Capture.HotKey != nil, "a missing hotKey is the default shortcut")
	expect(t, DecodeSettings([]byte(`{"capture":{"hotKey":null}}`), home).Capture.HotKey == nil, "null is off")
	expect(t, equal(DecodeSettings([]byte(`{"capture":{"hotKey":{"keyCode":1}}}`), home), DefaultSettings(home)), "a malformed hotKey fails the decode")
}

func TestDamagedSourcesNeverCostTheOtherSettings(t *testing.T) {
	s := DecodeSettings([]byte(`{"minWords":44,"sources":"garbage"}`), home)
	expect(t, s.MinWords == 44 && equal(s.Sources, DefaultSourcesPrefs()), "sources reset only")
	s = DecodeSettings([]byte(`{"minWords":44,"sources":{"claude":true,"git":"yes","allowed":7}}`), home)
	expect(t, s.MinWords == 44 && s.Sources.Claude && !s.Sources.Git && len(s.Sources.Allowed) == 0, "bad fields keep their defaults, good ones are read: %+v", s.Sources)
	expect(t, DefaultSourcesPrefs().HeadingText() == "What I did" && (SourcesPrefs{Heading: "  "}).HeadingText() == "What I did" && !DefaultSourcesPrefs().AnyOn(), "helpers")
}
