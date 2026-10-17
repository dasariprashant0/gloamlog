// settings.go - persisted preferences. Port of app/Core/Settings.swift.
//
// The Swift app kept Settings as one JSON blob inside UserDefaults (key "dailylog.settings.v2"). Here Settings lives in a JSON file
// whose path the caller passes in (the app uses ~/Library/Application Support/Gloamlog/settings.json). The JSON field names are the
// Swift Codable keys, so a blob read out of the old UserDefaults plist decodes with the very same code (see ImportLegacyDefaults).
//
// API:
//
//	DefaultSettings(home)             the defaults (Strict, 16:55, Mon-Fri, 15 min snooze, 20 words, default template, ~/Gloamlog)
//	DecodeSettings(data, home)        tolerant decode + Normalized; defaults on garbage (Swift: Settings.load(from:))
//	LoadSettings(path, home)          same, from the settings file; a missing or unreadable file gives the defaults
//	(Settings).Save(path)             writes the normalized settings atomically (Swift: save(to:))
//	(Settings).Normalized()           clamps invalid values (call after editing; Decode/Load already do)
//	ImportLegacyDefaults(old, home)   settings from a map read out of the old UserDefaults plist, so existing users keep theirs
//	(Settings).IsWorkday(weekday)     weekdays use Calendar numbering: 1 = Sunday ... 7 = Saturday
//
// v0.3 fields (replace the v0.2 `sections` array):
//
//	Template            markdown pre-filled into a NEW day in the editor only (never written until the user edits)
//	CarryOverHeadings   headings whose lines are carried to the next day (matched by NormalizeHeading)
//	MinWords            logged rule: words(body) >= MinWords (1...500, default 20); carried-over task lines do not count
//
// Old settings JSON decodes tolerantly: an old `sections` array with no `template` becomes "## <title>" blocks;
// `carryOverSectionID` and other unknown keys are ignored. A wrong type for a known key makes the whole decode fail, and the
// caller then gets the defaults (the Swift rule); only `sources` is forgiving, so a damaged value never costs the other settings.
//
// What the Swift UI did on top of Settings and is NOT here: `if !onboarded { launchAtLogin = false }` (the login item is opt-in) and
// "follow the system calendar when WeekStart is 0" (the UI builds its Calendar from WeekStart).
package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReminderMode is how hard the end-of-day reminder nags. Swift enum ReminderMode.
type ReminderMode string

const (
	ModeStrict ReminderMode = "strict"
	ModeGentle ReminderMode = "gentle"
)

// AppAppearance is the colour scheme choice. Swift enum AppAppearance.
type AppAppearance string

const (
	AppearanceSystem AppAppearance = "system"
	AppearanceLight  AppAppearance = "light"
	AppearanceDark   AppAppearance = "dark"
)

const (
	// LegacySettingsKey is the UserDefaults key of the old settings blob ("unchanged on purpose: v0.2 settings are found and migrated").
	LegacySettingsKey = "dailylog.settings.v2"

	DefaultMinWords   = 20
	MinWordsMin       = 1 // minWordsRange = 1...500
	MinWordsMax       = 500
	CatchUpWindowMin  = 7 // catchUpWindowRange = 7...365
	CatchUpWindowMax  = 365
	DefaultTemplate   = "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next"
	DefaultSourcesHdg = "What I did" // SourcesPrefs.defaultHeading: exists in the default template, so a click adds bullets there
)

// SnoozeChoices are the snooze lengths (minutes) the UI offers; anything else normalizes to 15.
var SnoozeChoices = []int{5, 10, 15, 20, 30}

// DefaultCarryOverHeadings returns a fresh copy of the default carry-over headings.
func DefaultCarryOverHeadings() []string { return []string{"To do next", "Pending / blocked"} }

// DefaultFolder is where the day files live unless the user picks another folder: <home>/Gloamlog.
func DefaultFolder(home string) string { return filepath.Join(home, "Gloamlog") }

// HotKeySpec is a global shortcut as Carbon understands it (kVK_ANSI_J = 38; controlKey 0x1000 | optionKey 0x0800 = 0x1800).
// Swift: HotKeySpec in Core/Capture/CaptureTypes.swift (ported here only because Settings holds one).
type HotKeySpec struct {
	KeyCode         uint32 `json:"keyCode"`
	CarbonModifiers uint32 `json:"carbonModifiers"`
}

// DefaultJotHotKey is control-option-J.
var DefaultJotHotKey = HotKeySpec{KeyCode: 38, CarbonModifiers: 0x1800}

// CapturePrefs is quick capture ("Jot"). Swift: CapturePrefs in Core/Capture/CaptureTypes.swift (Settings holds one).
type CapturePrefs struct {
	HotKey                *HotKeySpec `json:"hotKey"` // nil = off. Written as an explicit null so "off" survives a save (see note below)
	Timestamps            bool        `json:"timestamps"`
	TodoShorthand         bool        `json:"todoShorthand"`
	JotsHeading           string      `json:"jotsHeading"`
	JotsCountTowardLogged bool        `json:"jotsCountTowardLogged"` // jots never make a day "logged" unless the user turns this on
}

// DefaultCapturePrefs are the quick-capture defaults.
func DefaultCapturePrefs() CapturePrefs {
	hk := DefaultJotHotKey
	return CapturePrefs{HotKey: &hk, Timestamps: true, TodoShorthand: true, JotsHeading: "Jots"}
}

// SourcesPrefs is "Suggested from your work" (Claude Code, Git); everything off until ticked. This Mac only.
// Swift: SourcesPrefs in Core/Sources/ProjectPolicy.swift (Settings holds one); only its stored fields and two tiny helpers are ported.
type SourcesPrefs struct {
	Claude      bool     `json:"claude"`      // read Claude Code's session files (only for ticked projects)
	Git         bool     `json:"git"`         // run `git log` in ticked repositories
	Consented   bool     `json:"consented"`   // the first-enable sheet was accepted
	Allowed     []string `json:"allowed"`     // ticked project roots (absolute paths); with AllProjects it holds the SKIPPED roots instead
	AllProjects bool     `json:"allProjects"` // read every Claude Code session and tab, not only ticked projects
	AutoAdd     bool     `json:"autoAdd"`     // write today's lines into the page by themselves, as the work happens
	GitRoots    []string `json:"gitRoots"`    // folders added with "Add folder...": a repository, or a folder that holds repositories
	Heading     string   `json:"heading"`
}

// DefaultSourcesPrefs are the "Suggested from your work" defaults: everything off.
func DefaultSourcesPrefs() SourcesPrefs {
	return SourcesPrefs{Allowed: []string{}, GitRoots: []string{}, Heading: DefaultSourcesHdg}
}

// AnyOn is true when at least one source is switched on.
func (p SourcesPrefs) AnyOn() bool { return p.Claude || p.Git }

// HeadingText is the heading the suggestions are added under ("What I did" when the setting is blank).
func (p SourcesPrefs) HeadingText() string {
	if h := trimmed(p.Heading); h != "" {
		return h
	}
	return DefaultSourcesHdg
}

// Settings is everything the user can set. Each field notes its Swift property name (the JSON key is the same).
type Settings struct {
	ReminderMinutes    int           `json:"reminderMinutes"`        // reminderMinutes: minutes after midnight, local time (default 16:55)
	Weekdays           WeekdaySet    `json:"weekdays"`               // weekdays: Calendar numbering, 1 = Sunday (default Mon-Fri = 2...6)
	Mode               ReminderMode  `json:"mode"`                   // mode: strict | gentle (default strict)
	SnoozeMinutes      int           `json:"snoozeMinutes"`          // snoozeMinutes: one of SnoozeChoices (default 15)
	StorageFolder      string        `json:"storageFolder"`          // storageFolder: a path (Swift stored a file:// URL; both decode)
	LaunchAtLogin      bool          `json:"launchAtLogin"`          // launchAtLogin (default true)
	Template           string        `json:"template"`               // template: markdown pre-filled into a NEW day in the editor only
	CarryOverHeadings  []string      `json:"carryOverHeadings"`      // carryOverHeadings (default "To do next", "Pending / blocked")
	MinWords           int           `json:"minWords"`               // minWords: 1...500, default 20
	Onboarded          bool          `json:"onboarded"`              // onboarded (default false)
	LogStartDate       string        `json:"logStartDate,omitempty"` // logStartDate: "yyyy-MM-dd", "" = nil = the first log in the folder
	Appearance         AppAppearance `json:"appearance"`             // appearance: system | light | dark
	WeekStart          int           `json:"weekStart,omitempty"`    // weekStart: Calendar weekday 1...7 (1 = Sunday), 0 = nil = follow the system
	CatchUpWindowDays  int           `json:"catchUpWindowDays"`      // catchUpWindowDays: how far back "Catch up" looks, 7...365, default 30
	Capture            CapturePrefs  `json:"capture"`                // capture: quick capture (shortcut, timestamps, Jots heading)
	ClosedAppReminders bool          `json:"closedAppReminders"`     // closedAppReminders: macOS delivers the reminder while Gloamlog is quit
	Sources            SourcesPrefs  `json:"sources"`                // sources: "Suggested from your work"
	// ShowMenuBar is NOT part of the Swift Settings struct: it was its own UserDefaults key, "showMenuBar" (default true). It lives
	// here so that one file holds every preference and ImportLegacyDefaults can carry it over.
	ShowMenuBar bool `json:"showMenuBar"`
}

// DefaultSettings returns the defaults. home is the user's home folder (only used for the default storage folder).
func DefaultSettings(home string) Settings {
	return Settings{
		ReminderMinutes:    16*60 + 55,
		Weekdays:           WeekdaySet{2, 3, 4, 5, 6},
		Mode:               ModeStrict,
		SnoozeMinutes:      15,
		StorageFolder:      DefaultFolder(home),
		LaunchAtLogin:      true,
		Template:           DefaultTemplate,
		CarryOverHeadings:  DefaultCarryOverHeadings(),
		MinWords:           DefaultMinWords,
		Appearance:         AppearanceSystem,
		CatchUpWindowDays:  30,
		Capture:            DefaultCapturePrefs(),
		ClosedAppReminders: true,
		Sources:            DefaultSourcesPrefs(),
		ShowMenuBar:        true,
	}
}

// IsWorkday reports whether weekday (1 = Sunday ... 7 = Saturday) is a scheduled day.
func (s Settings) IsWorkday(weekday int) bool { return s.Weekdays.Has(weekday) }

// Normalized clamps invalid values. It returns a corrected copy and never changes s.
func (s Settings) Normalized() Settings {
	n := s
	n.ReminderMinutes = clampInt(s.ReminderMinutes, 0, 24*60-1)
	seen := map[int]bool{}
	days := WeekdaySet{}
	for _, d := range s.Weekdays {
		if d >= 1 && d <= 7 && !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}
	sort.Ints(days)
	if len(days) == 0 {
		days = WeekdaySet{2, 3, 4, 5, 6}
	}
	n.Weekdays = days
	n.SnoozeMinutes = 15
	for _, c := range SnoozeChoices {
		if c == s.SnoozeMinutes {
			n.SnoozeMinutes = c
		}
	}
	n.MinWords = clampInt(s.MinWords, MinWordsMin, MinWordsMax)
	n.CarryOverHeadings = []string{}
	for _, h := range s.CarryOverHeadings {
		if t := trimmed(h); t != "" {
			n.CarryOverHeadings = append(n.CarryOverHeadings, t)
		}
	}
	n.CatchUpWindowDays = clampInt(s.CatchUpWindowDays, CatchUpWindowMin, CatchUpWindowMax)
	if s.LogStartDate != "" && !DayKeyIsWellFormed(s.LogStartDate) {
		n.LogStartDate = ""
	}
	if s.WeekStart < 1 || s.WeekStart > 7 {
		n.WeekStart = 0
	}
	return n
}

func clampInt(v, lo, hi int) int { return min(max(v, lo), hi) }

// ---- decoding (tolerant, like the Swift init(from:)) ----

// DecodeSettings decodes a settings JSON blob over the defaults and normalizes it. Garbage, or a wrong type for a known key, gives
// the defaults. home is only used for the default storage folder.
func DecodeSettings(data []byte, home string) Settings {
	s := DefaultSettings(home)
	if err := s.decodeOver(data); err != nil {
		return DefaultSettings(home)
	}
	return s.Normalized()
}

// LoadSettings reads the settings file. A missing or unreadable file gives the defaults.
func LoadSettings(path, home string) Settings {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultSettings(home)
	}
	return DecodeSettings(data, home)
}

// Save writes the normalized settings to path atomically, creating the folder if needed.
func (s Settings) Save(path string) error {
	data, err := json.MarshalIndent(s.Normalized(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return AtomicWrite(append(data, '\n'), path, "")
}

type rawObject map[string]json.RawMessage

func parseObject(data []byte) (rawObject, error) {
	var obj rawObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	if obj == nil { // the JSON text "null": Swift's keyed container fails on that too
		return nil, errors.New("settings: not a JSON object")
	}
	return obj, nil
}

// field decodes obj[key] into dst. A missing key or a JSON null leaves dst alone (Swift: decodeIfPresent ?? default); any other
// mismatch is an error.
func (o rawObject) field(key string, dst any) error {
	raw, ok := o[key]
	if !ok || isNull(raw) {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("settings key %q: %w", key, err)
	}
	return nil
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// decodeOver fills s from the JSON object in data, key by key, keeping the current value for every key that is missing.
func (s *Settings) decodeOver(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return err
	}
	var weekdays []int
	var mode, appearance, folder, template *string
	steps := []struct {
		key string
		dst any
	}{
		{"reminderMinutes", &s.ReminderMinutes}, {"weekdays", &weekdays}, {"mode", &mode}, {"snoozeMinutes", &s.SnoozeMinutes},
		{"storageFolder", &folder}, {"launchAtLogin", &s.LaunchAtLogin}, {"carryOverHeadings", &s.CarryOverHeadings},
		{"minWords", &s.MinWords}, {"onboarded", &s.Onboarded}, {"logStartDate", &s.LogStartDate}, {"appearance", &appearance},
		{"weekStart", &s.WeekStart}, {"catchUpWindowDays", &s.CatchUpWindowDays}, {"closedAppReminders", &s.ClosedAppReminders},
		{"showMenuBar", &s.ShowMenuBar}, {"template", &template},
	}
	for _, st := range steps {
		if err := o.field(st.key, st.dst); err != nil {
			return err
		}
	}
	if weekdays != nil {
		s.Weekdays = WeekdaySet(weekdays)
	}
	if mode != nil {
		if *mode != string(ModeStrict) && *mode != string(ModeGentle) {
			return fmt.Errorf("settings: unknown mode %q", *mode)
		}
		s.Mode = ReminderMode(*mode)
	}
	if appearance != nil {
		switch AppAppearance(*appearance) {
		case AppearanceSystem, AppearanceLight, AppearanceDark:
			s.Appearance = AppAppearance(*appearance)
		default:
			return fmt.Errorf("settings: unknown appearance %q", *appearance)
		}
	}
	if folder != nil {
		s.StorageFolder = folderFromJSON(*folder)
	}
	if raw, ok := o["capture"]; ok && !isNull(raw) {
		if err := s.Capture.decodeOver(raw); err != nil {
			return err
		}
	}
	// A damaged sources value never costs the other settings.
	if raw, ok := o["sources"]; ok && !isNull(raw) {
		p := DefaultSourcesPrefs()
		if p.decodeOver(raw) == nil {
			s.Sources = p
		} else {
			s.Sources = DefaultSourcesPrefs()
		}
	}
	if template != nil {
		s.Template = *template
	} else if t, ok := legacySectionsTemplate(o); ok {
		s.Template = t
	}
	return nil
}

// folderFromJSON turns a stored folder into a path. Swift encoded a URL, so the old blob holds "file:///tmp/old-logs/".
func folderFromJSON(v string) string {
	if strings.HasPrefix(v, "file://") {
		if u, err := url.Parse(v); err == nil {
			return filepath.Clean(u.Path)
		}
	}
	return v
}

// legacySectionsTemplate: an old `sections` array with no `template` becomes "## <title>" blocks. Unreadable sections are ignored,
// and so is a list whose titles are all blank (the default template stays).
func legacySectionsTemplate(o rawObject) (string, bool) {
	raw, ok := o["sections"]
	if !ok || isNull(raw) {
		return "", false
	}
	var old []struct {
		Title *string `json:"title"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		return "", false
	}
	var blocks []string
	for _, sec := range old {
		if sec.Title == nil { // Swift's LegacySection needs a title: without one the whole array fails to decode
			return "", false
		}
		if t := trimmed(*sec.Title); t != "" {
			blocks = append(blocks, "## "+t)
		}
	}
	if len(blocks) == 0 {
		return "", false
	}
	return strings.Join(blocks, "\n\n"), true
}

// decodeOver mirrors CapturePrefs.init(from:): a missing hotKey means the default shortcut, an explicit null means off.
func (p *CapturePrefs) decodeOver(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return err
	}
	if raw, ok := o["hotKey"]; ok {
		if isNull(raw) {
			p.HotKey = nil
		} else {
			var spec struct {
				KeyCode         *uint32 `json:"keyCode"`
				CarbonModifiers *uint32 `json:"carbonModifiers"`
			}
			if err := json.Unmarshal(raw, &spec); err != nil || spec.KeyCode == nil || spec.CarbonModifiers == nil {
				return errors.New("settings key \"hotKey\": malformed")
			}
			p.HotKey = &HotKeySpec{KeyCode: *spec.KeyCode, CarbonModifiers: *spec.CarbonModifiers}
		}
	} else {
		hk := DefaultJotHotKey
		p.HotKey = &hk
	}
	for _, st := range []struct {
		key string
		dst any
	}{{"timestamps", &p.Timestamps}, {"todoShorthand", &p.TodoShorthand}, {"jotsHeading", &p.JotsHeading},
		{"jotsCountTowardLogged", &p.JotsCountTowardLogged}} {
		if err := o.field(st.key, st.dst); err != nil {
			return err
		}
	}
	return nil
}

// decodeOver mirrors SourcesPrefs.init(from:): every field is forgiving (a bad value keeps the default).
func (p *SourcesPrefs) decodeOver(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return err
	}
	for _, st := range []struct {
		key string
		dst any
	}{{"claude", &p.Claude}, {"git", &p.Git}, {"consented", &p.Consented}, {"allowed", &p.Allowed},
		{"allProjects", &p.AllProjects}, {"autoAdd", &p.AutoAdd}, {"gitRoots", &p.GitRoots}, {"heading", &p.Heading}} {
		_ = o.field(st.key, st.dst) // errors ignored on purpose
	}
	if p.Allowed == nil {
		p.Allowed = []string{}
	}
	if p.GitRoots == nil {
		p.GitRoots = []string{}
	}
	return nil
}

// ---- import from the old UserDefaults ----

// ImportLegacyDefaults builds Settings from the values of the old app's UserDefaults domain, so existing users keep their settings.
// `old` is the plist dictionary the caller read (e.g. ~/Library/Preferences/<bundle id>.plist). It understands:
//
//	"dailylog.settings.v2"  the settings blob: []byte or string holding JSON (a plist <data> value), or an already decoded map
//	"remindMinutes"         the v0.1 reminder time, used only when there is no blob; it also marks the user as onboarded
//	"showMenuBar"           the menu-bar item switch (a bool)
//
// ok is false when none of those keys is there (a fresh install): the result is then just DefaultSettings(home).
func ImportLegacyDefaults(old map[string]any, home string) (s Settings, ok bool) {
	s = DefaultSettings(home)
	blob, hasBlob := old[LegacySettingsKey]
	if hasBlob {
		ok = true
		if data, good := legacyBlobBytes(blob); good {
			s = DecodeSettings(data, home)
		}
	} else if v, has := old["remindMinutes"]; has {
		if n, good := anyToInt(v); good {
			s.ReminderMinutes = n
			s.Onboarded = true
			ok = true
		}
	}
	if b, has := old["showMenuBar"].(bool); has {
		s.ShowMenuBar = b
		ok = true
	}
	return s.Normalized(), ok
}

func legacyBlobBytes(v any) ([]byte, bool) {
	switch t := v.(type) {
	case []byte:
		return t, true
	case string:
		return []byte(t), true
	case map[string]any:
		data, err := json.Marshal(t)
		return data, err == nil
	}
	return nil, false
}

func anyToInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case uint64:
		return int(t), true
	case float64:
		return int(t), true
	}
	return 0, false
}
