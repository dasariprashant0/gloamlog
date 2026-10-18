package main

// Notes is the service the React UI calls (Wails generates TypeScript for every exported method). It holds the settings and
// the store and turns core results into the shapes the screens draw. It replaces the Swift AppModel glue; the rules live in
// internal/core.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

type Notes struct {
	mu           sync.Mutex
	home         string
	settingsPath string
	backupDir    string
	settings     core.Settings
	store        *core.LogStore
	now          func() time.Time

	// OnSettings runs after every saved settings change (main.go re-registers the Jot shortcut). Called without the lock.
	OnSettings func(core.Settings)
	hotKeyErr  string // why the Jot shortcut could not be registered ("" = fine or off)

	settingsNote    string // a damaged settings.json was set aside: say so on the page
	settingsBlocked bool   // ...and it couldn't even be moved: never save over it
}

// NewNotes loads settings.json, or imports the Swift app's preferences once when there is none.
func NewNotes() (*Notes, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	support := filepath.Join(home, "Library", "Application Support", "Gloamlog")
	// Very old versions kept their support files under "Daily Log": move them over first (only when the new folder doesn't
	// exist yet), as the Swift app did on each launch.
	core.MigrateSupportFolder(filepath.Dir(support))
	n := &Notes{
		home:         home,
		settingsPath: filepath.Join(support, "settings.json"),
		backupDir:    filepath.Join(support, "backups"),
		now:          time.Now,
	}
	if _, err := os.Stat(n.settingsPath); errors.Is(err, os.ErrNotExist) {
		// The second result only says whether anything was imported; a fresh install (no old app) gets the defaults.
		s, _ := core.ImportLegacyDefaults(readLegacyDefaults(bundleID), home)
		if err := os.MkdirAll(support, 0o700); err != nil {
			return nil, err
		}
		if err := s.Save(n.settingsPath); err != nil {
			return nil, err
		}
	}
	n.settingsNote, n.settingsBlocked = setAsideDamagedSettings(n.settingsPath, time.Now())
	n.apply(core.LoadSettings(n.settingsPath, home))
	return n, nil
}

func (n *Notes) apply(s core.Settings) {
	n.settings = s
	n.store = core.NewLogStore(s.StorageFolder, n.backupDir, 10*time.Minute, n.now)
	// the process-wide jot rules, as the Swift app set them on every settings change (it was never set in the port)
	core.SetJotsRules(core.JotsRules{Heading: s.Capture.JotsHeading, CountTowardLogged: s.Capture.JotsCountTowardLogged,
		NewDaysLog: core.IsLogPage(s.Template)})
}

func (n *Notes) cal() core.Calendar { return core.NewCalendar(time.Local, n.settings.WeekStart) }

// states is the status of every day with a file, and the day the log starts.
func (n *Notes) states() (map[string]core.DayStatus, string, error) {
	files, err := n.store.FileStates(n.settings.MinWords)
	if err != nil {
		return nil, "", err
	}
	return files, core.EffectiveSince(n.settings.LogStartDate, files), nil
}

// ---- settings ----

func (n *Notes) Settings() core.Settings {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.settings
}

func (n *Notes) SaveSettings(s core.Settings) (core.Settings, error) {
	n.mu.Lock()
	if n.settingsBlocked {
		old := n.settings
		n.mu.Unlock()
		return old, errors.New("Your settings file couldn't be read or moved aside, so it isn't saved over. " + n.settingsNote)
	}
	s = s.Normalized()
	if err := s.Save(n.settingsPath); err != nil {
		old := n.settings
		n.mu.Unlock()
		return old, err
	}
	n.apply(s)
	hook := n.OnSettings
	n.mu.Unlock()
	if hook != nil {
		hook(s)
	}
	return s, nil
}

// HotKeyStatus says why the Jot shortcut isn't working ("" when it is, or when it is off).
func (n *Notes) HotKeyStatus() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.hotKeyErr
}

func (n *Notes) setHotKeyErr(msg string) {
	n.mu.Lock()
	n.hotKeyErr = msg
	n.mu.Unlock()
}

// ---- overview: sidebar and menu bar ----

type Overview struct {
	Today        string             `json:"today"`
	TodayStatus  core.DayStatus     `json:"todayStatus"`
	CatchUp      []string           `json:"catchUp"`
	Streak       core.StreakResult  `json:"streak"`
	Recent       []string           `json:"recent"` // newest pages first, today left out (the calendar reaches older ones)
	Folder       string             `json:"folder"`
	FolderError  string             `json:"folderError"` // "" = the folder is there and writable
	Onboarded    bool               `json:"onboarded"`
	Summary      core.FolderSummary `json:"summary"`
	SettingsNote string             `json:"settingsNote"` // "" unless a damaged settings file was set aside
}

func (n *Notes) Overview() (Overview, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	now, cal, s := n.now(), n.cal(), n.settings
	o := Overview{Today: core.DayKeyString(now, time.Local), Folder: s.StorageFolder, Onboarded: s.Onboarded, SettingsNote: n.settingsNote}
	if err := n.store.CheckFolder(true); err != nil {
		o.FolderError = core.LogErrorDetail(err)
		return o, nil
	}
	states, since, err := n.states()
	if err != nil {
		return o, err
	}
	o.TodayStatus = core.ResolveStatus(o.Today, states[o.Today], now, cal, s.Weekdays, since)
	o.CatchUp = core.CatchUpMissing(states, now, cal, s.Weekdays, since, s.CatchUpWindowDays)
	o.Streak = core.ComputeStreak(states, now, cal, s.Weekdays, since)
	days, err := n.store.ListDays()
	if err != nil {
		return o, err
	}
	for _, d := range slices.Backward(days) {
		if d != o.Today && len(o.Recent) < 7 {
			o.Recent = append(o.Recent, d)
		}
	}
	o.Summary, err = n.store.FolderSummary()
	return o, err
}

// Month is the sidebar calendar for one month (month 1...12).
func (n *Notes) Month(year, month int) ([][]core.MonthCell, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	states, since, err := n.states()
	if err != nil {
		return nil, err
	}
	return core.MonthGridRows(year, month, states, n.now(), n.cal(), n.settings.Weekdays, since), nil
}

// Heatmap is the last `weeks` weeks, for the streak popover and the menu bar.
func (n *Notes) Heatmap(weeks int) ([][]core.HeatCell, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	states, since, err := n.states()
	if err != nil {
		return nil, err
	}
	return core.HeatmapWeeks(states, n.now(), n.cal(), n.settings.Weekdays, weeks, since), nil
}

// JumpTo reads "yesterday", "fri", "12 oct", "2026-10-12"... into a day key ("" when it means nothing).
func (n *Notes) JumpTo(input string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	day, ok := core.ParseDateJump(input, n.now(), n.cal())
	if !ok {
		return ""
	}
	return day
}

// ---- one day ----

type Page struct {
	Day          string          `json:"day"`
	Title        string          `json:"title"` // "Wednesday 14 October"
	Body         string          `json:"body"`  // the file's body, or the template for a day with no file
	Exists       bool            `json:"exists"`
	IsSkipped    bool            `json:"isSkipped"`
	SkipReason   string          `json:"skipReason"`
	Status       core.DayStatus  `json:"status"`
	Words        int             `json:"words"`
	NotLocal     bool            `json:"notLocal"` // still in iCloud, not downloaded
	Carry        *core.CarryCard `json:"carry"`
	CarryApplied bool            `json:"carryApplied"`
}

func (n *Notes) Open(day string) (Page, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !core.DayKeyIsWellFormed(day) {
		return Page{}, errors.New("not a day: " + day)
	}
	now, cal, s := n.now(), n.cal(), n.settings
	p := Page{Day: day, NotLocal: n.store.IsNotLocal(day)}
	if t, ok := core.DayKeyDate(day, time.Local); ok {
		p.Title = t.Format("Monday 2 January")
		if t.Year() != now.Year() {
			p.Title = t.Format("Monday 2 January 2006")
		}
	}
	doc, err := n.store.Load(day)
	if err != nil {
		return p, err
	}
	states, since, err := n.states()
	if err != nil {
		return p, err
	}
	p.Status = core.ResolveStatus(day, states[day], now, cal, s.Weekdays, since)
	if doc != nil {
		p.Exists, p.Body, p.IsSkipped, p.SkipReason, p.Words = true, doc.Body, doc.IsSkipped, doc.SkipReason, doc.Words()
	} else {
		p.Body = s.Template
	}
	if day == core.DayKeyString(now, time.Local) && !p.IsSkipped {
		load := func(d string) *core.DayDocument { doc, _ := n.store.Load(d); return doc } // an unreadable day offers no carry-over; nothing is written
		if c := core.CarryCardBefore(day, cal, s, load); c != nil {
			p.Carry, p.CarryApplied = c, core.IsCarryApplied(*c, p.Body)
		}
	}
	return p, nil
}

// Save writes the page. The editor sends the whole markdown body; the store adds the "# day" title and keeps a safety copy.
func (n *Notes) Save(day, body string) (core.SaveOutcome, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.Save(day, writable(body, n.settings))
}

// writable is what a save writes for the editor's markdown (Swift DayEditor.writable): a page of nothing but empty headings
// ("##", what select-all + delete leaves) is an emptied page, which the store turns into "no log"; and the untouched
// template above nothing but jots writes only the jots, so template headings reach the file with the user's first words.
func writable(md string, s core.Settings) string {
	emptied := true
	for _, l := range strings.Split(md, "\n") {
		if t := strings.TrimSpace(l); t != "" && (len(t) > 6 || strings.Trim(t, "#") != "") {
			emptied = false
			break
		}
	}
	if emptied {
		return ""
	}
	return core.StripUntouchedTemplate(md, s.Template, s.Capture.JotsHeading)
}

// Words counts the words that count toward "logged" (the live progress line on the page).
func (n *Notes) Words(body string) int { return core.CountWords(body) }

// CarryOver returns the body with the carried items added under the carry heading (the page saves it like any edit).
func (n *Notes) CarryOver(card core.CarryCard, body string) string {
	return core.ApplyCarryOver(card, body)
}

func (n *Notes) Skip(day, reason string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.Skip(day, reason)
}

func (n *Notes) SkipDays(days []string, reason string) ([]string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.SkipDays(days, reason)
}

func (n *Notes) Unskip(days []string) ([]string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.UnskipDays(days)
}

func (n *Notes) Backups(day string) []core.BackupInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.ListBackups(day)
}

func (n *Notes) Restore(day string, backup core.BackupInfo) (core.DayDocument, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.RestoreBackup(day, backup)
}

// SaveImage stores a pasted or dropped image under assets/ and returns the path the markdown links to.
func (n *Notes) SaveImage(day, name, mime, dataBase64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.Assets().Save(data, core.AssetFileExtension(mime, name), day)
}

// ---- search and review ----

// Hit is a search result cut into three strings, so the UI can mark the match without byte offsets.
type Hit struct {
	Day     string  `json:"day"`
	Heading *string `json:"heading"`
	Before  string  `json:"before"`
	Match   string  `json:"match"`
	After   string  `json:"after"`
}

func (n *Notes) Search(query string) ([]Hit, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	hits, err := n.store.Search(query, nil)
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{h.Day, h.Heading, h.Snippet[:h.MatchStart], h.Snippet[h.MatchStart:h.MatchEnd], h.Snippet[h.MatchEnd:]})
	}
	return out, err
}

type Week struct {
	Summary  core.WeekSummary `json:"summary"`
	Markdown string           `json:"markdown"`
}

// Week summarises the week holding `day`, shifted by `shift` weeks; bySection groups by heading instead of by day.
func (n *Notes) Week(day string, shift int, bySection bool) (Week, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	now, cal := n.now(), n.cal()
	date, ok := core.DayKeyDate(day, time.Local)
	if !ok {
		date = now
	}
	date = core.ShiftWeek(date, shift, cal)
	start := cal.StartOfWeek(date)
	from, to := core.DayKeyString(start, time.Local), core.DayKeyString(start.AddDate(0, 0, 6), time.Local)
	docs, err := n.store.Documents(from, to)
	if err != nil {
		return Week{}, err
	}
	states, _, err := n.states()
	if err != nil {
		return Week{}, err
	}
	sum := core.SummarizeWeek(date, docs, states, n.settings, cal, now)
	grouping := core.WeekByDay
	if bySection {
		grouping = core.WeekBySection
	}
	return Week{sum, core.WeekMarkdown(sum, grouping, cal)}, nil
}

// ---- images in the editor ----

// serveAssets answers /dlasset/<path> with a file from the notes folder's assets, and passes every other request on.
func (n *Notes) serveAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel, ok := strings.CutPrefix(r.URL.Path, "/dlasset/")
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		n.mu.Lock()
		path := n.store.Assets().Resolve(rel)
		n.mu.Unlock()
		if path == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path)
	})
}

// setAsideDamagedSettings protects a settings.json that exists but can't be used. LoadSettings would fall back to the
// defaults (notes folder ~/Gloamlog, not onboarded) and the next save would overwrite the real file, so the person's folder
// choice and template would be lost and their notes would look gone. The damaged file is moved to
// settings.json.damaged-<time> first; a file that can't be read at all is left in place. It returns a sentence for the page, and blocked when the file can't be read or moved (then
// nothing may save over it). A missing file is the normal first run: "", false.
func setAsideDamagedSettings(path string, now time.Time) (note string, blocked bool) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		// Unreadable is not damaged (no permission, a disk error, an iCloud file not yet here): leave it where it is,
		// run on defaults this time, and save nothing over it.
		return "Gloamlog couldn't read its settings file (" + path + "), so it is using defaults for now and leaving the file as it is. Settings changes won't be saved until it can be read; quit and open Gloamlog again to retry.", true
	}
	var probe core.Settings
	if json.Unmarshal(data, &probe) == nil {
		return "", false // readable and well formed
	}
	aside := path + ".damaged-" + now.Format("20060102-150405")
	if rerr := os.Rename(path, aside); rerr != nil {
		return "Gloamlog couldn't read its settings file (" + path + "), so it is using defaults for now and leaving the file as it is.", true
	}
	return "Gloamlog couldn't read its settings file, so it is using defaults. The old file is kept as " + filepath.Base(aside) + " next to it.", false
}
