package main

// The Review screen (week / month / year / range), its copies and its exports. Every rule lives in internal/core
// (BuildReview, ReviewMarkdown, PlanExport...); this file only resolves the period the screen asks for, turns the summary into the
// shapes the page draws (dates as words, markdown as HTML), and asks where to save. All methods start with "Review" so they never
// clash with another screen's.

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/dasariprashant0/gloamlog/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// earliestReviewDay is how far back the screen steps (the Swift app's AppModel.earliestDay).
const earliestReviewDay = "2000-01-01"

// ReviewRequest says which review the screen wants. The screen keeps scale + day (or from/to for a range) and sends them back
// every time; Shift steps to the previous (-1) or next (1) period first, and the answer carries the period it landed on.
type ReviewRequest struct {
	Scale          string `json:"scale"` // week, month, year or range
	Day            string `json:"day"`   // any day of the week, month or year ("" = today)
	From           string `json:"from"`  // range only
	To             string `json:"to"`
	Shift          int    `json:"shift"`
	IncludeSkipped bool   `json:"includeSkipped"`
	IncludeJots    bool   `json:"includeJots"`
	BySection      bool   `json:"bySection"` // false = group by day
}

// ReviewTile is one day of the week strip.
type ReviewTile struct {
	Day     string         `json:"day"`
	Weekday string         `json:"weekday"` // "Mon"
	Number  int            `json:"number"`
	Status  core.DayStatus `json:"status"`
	Bottom  string         `json:"bottom"` // "63 words", "Write", the skip reason, or ""
	IsToday bool           `json:"isToday"`
	Opens   bool           `json:"opens"` // false for days that have not happened yet
	Spoken  string         `json:"spoken"`
}

// ReviewDayView is a day's writing for "Group by Day". HTML is the page as shown, ready to draw.
type ReviewDayView struct {
	Day        string         `json:"day"`
	Label      string         `json:"label"`  // "Mon 5 Oct"
	Spoken     string         `json:"spoken"` // "Monday 5 October"
	Status     core.DayStatus `json:"status"`
	Words      int            `json:"words"`
	SkipReason string         `json:"skipReason"`
	HTML       string         `json:"html"`
}

type ReviewItemView struct {
	Day   string `json:"day"`
	Label string `json:"label"` // "Mon 5"
	Text  string `json:"text"`
	HTML  string `json:"html"`
}

// ReviewGroupView is one roll-up of "Group by Section".
type ReviewGroupView struct {
	Title string               `json:"title"` // without a leading emoji or mark
	Kind  core.ReviewGroupKind `json:"kind"`
	Items []ReviewItemView     `json:"items"`
}

// ReviewMonthView is one row of a Year.
type ReviewMonthView struct {
	Month    string            `json:"month"` // "2026-09"
	Name     string            `json:"name"`  // "September"
	Line     string            `json:"line"`  // "20 logged · 1 skipped · 2 not logged", or "–"
	Counts   core.ReviewCounts `json:"counts"`
	IsFuture bool              `json:"isFuture"`
	Inert    bool              `json:"inert"` // not started or nothing to count: shown as a dash
	Spoken   string            `json:"spoken"`
}

// ReviewData is everything the Review screen draws for one request.
type ReviewData struct {
	Period       core.ReviewPeriod `json:"period"`
	Title        string            `json:"title"`    // "Week of 5 to 11 Oct 2026"
	FileName     string            `json:"fileName"` // "Gloamlog 2026-W41"
	CountsLine   string            `json:"countsLine"`
	TasksLine    string            `json:"tasksLine"`
	Counts       core.ReviewCounts `json:"counts"`
	NotLogged    int               `json:"notLogged"`
	Empty        bool              `json:"empty"`
	EmptyText    string            `json:"emptyText"`
	CanPrev      bool              `json:"canPrev"`
	CanNext      bool              `json:"canNext"`
	IsCurrent    bool              `json:"isCurrent"`
	Today        string            `json:"today"`
	MinWords     int               `json:"minWords"`
	CatchUpLabel string            `json:"catchUpLabel"` // "Catch up this week"
	MissingDays  []string          `json:"missingDays"`  // the unlogged days of the period, oldest first
	Tiles        []ReviewTile      `json:"tiles"`        // week only
	Days         []ReviewDayView   `json:"days"`
	Groups       []ReviewGroupView `json:"groups"`
	Months       []ReviewMonthView `json:"months"` // year only
}

// reviewBuilt is a resolved request: the summary and what was used to make it.
type reviewBuilt struct {
	sum    core.ReviewSummary
	docs   map[string]core.DayDocument
	states map[string]core.DayStatus
	since  string
	cal    core.Calendar
	now    time.Time
	today  string
	html   func(md string) string // markdown to HTML for the page (images through /dlasset/)
}

// reviewPeriod is the period a request means: the scale around the day (or the range), stepped by Shift when that stays inside
// the days that exist. A range never reaches past today.
func (n *Notes) reviewPeriod(req ReviewRequest) core.ReviewPeriod {
	now, cal := n.now(), n.cal()
	today := core.DayKeyString(now, time.Local)
	scale := core.ReviewScale(req.Scale)
	if scale != core.ScaleWeek && scale != core.ScaleMonth && scale != core.ScaleYear && scale != core.ScaleRange {
		scale = core.ScaleWeek
	}
	var p core.ReviewPeriod
	if scale == core.ScaleRange {
		from, ok1 := core.DayKeyDate(req.From, time.Local)
		to, ok2 := core.DayKeyDate(req.To, time.Local)
		if !ok1 {
			from = cal.StartOfWeek(now)
		}
		if !ok2 {
			to = now
		}
		p = core.ReviewRange(from, to, cal)
	} else {
		anchor, ok := core.DayKeyDate(req.Day, time.Local)
		if !ok {
			anchor = now
		}
		p = core.ReviewPeriodContaining(scale, anchor, cal)
	}
	if req.Shift != 0 {
		if next, ok := core.ShiftReview(p, req.Shift, cal); ok && stepAllowed(next, req.Shift, today) {
			p = next
		}
	}
	if p.Scale == core.ScaleRange {
		p.End = min(p.End, today)
		p.Start = min(p.Start, p.End)
	}
	return p
}

// stepAllowed: back as far as 2000, forward only to a period that has started.
func stepAllowed(next core.ReviewPeriod, dir int, today string) bool {
	if dir < 0 {
		return next.End >= earliestReviewDay
	}
	return next.Start <= today
}

func (n *Notes) reviewBuild(req ReviewRequest) (reviewBuilt, error) {
	p := n.reviewPeriod(req)
	b := reviewBuilt{cal: n.cal(), now: n.now()}
	b.today = core.DayKeyString(b.now, time.Local)
	states, since, err := n.states()
	if err != nil {
		return b, err
	}
	b.states, b.since = states, since
	b.docs = map[string]core.DayDocument{}
	if p.Scale != core.ScaleYear { // a year reads no pages
		if b.docs, err = n.store.Documents(p.Start, p.End); err != nil {
			return b, err
		}
	}
	opts := core.ReviewOptions{Grouping: core.WeekByDay, IncludeSkipped: req.IncludeSkipped, IncludeJots: req.IncludeJots}
	if req.BySection {
		opts.Grouping = core.WeekBySection
	}
	b.sum = core.BuildReview(p, b.docs, states, n.settings, opts, b.cal, b.now)
	images := n.assetImages()
	b.html = func(md string) string { return core.MDBody(md, images) }
	return b, nil
}

// assetImages links an image path on a page to /dlasset/ (the app's asset route) when the file is there. Caller holds n.mu.
func (n *Notes) assetImages() core.MDImages {
	assets := n.store.Assets()
	return func(path string) (string, bool) {
		if assets.Resolve(path) == "" {
			return "", false
		}
		parts := strings.Split(path, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		return "/dlasset/" + strings.Join(parts, "/"), true
	}
}

// Review builds the review the screen asked for.
func (n *Notes) Review(req ReviewRequest) (ReviewData, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, err := n.reviewBuild(req)
	if err != nil {
		return ReviewData{}, err
	}
	loc := time.Local
	s, cal, p := b.sum, b.cal, b.sum.Period
	d := ReviewData{
		Period: p, Title: core.ReviewTitle(p, cal, false), FileName: core.ReviewFileName(p, cal),
		Counts: s.Counts, NotLogged: s.Counts.NotWritten(), Empty: s.IsEmpty(), Today: b.today, MinWords: n.settings.MinWords,
		IsCurrent: p.Start <= b.today && b.today <= p.End, MissingDays: []string{},
		Tiles: []ReviewTile{}, Days: []ReviewDayView{}, Groups: []ReviewGroupView{}, Months: []ReviewMonthView{},
	}
	if next, ok := core.ShiftReview(p, -1, cal); ok {
		d.CanPrev = stepAllowed(next, -1, b.today)
	}
	if next, ok := core.ShiftReview(p, 1, cal); ok {
		d.CanNext = stepAllowed(next, 1, b.today)
	}
	switch p.Scale {
	case core.ScaleWeek:
		d.CatchUpLabel = "Catch up this week"
	case core.ScaleMonth:
		d.CatchUpLabel = "Catch up this month"
	default:
		d.CatchUpLabel = "Catch up this range"
	}
	d.TasksLine, _ = core.ReviewTasksLine(s.Tasks)

	// who needs catching up: the same rule as the counts (scheduled, before today, on or after the log start)
	core.ForEachDay(p.Start, min(p.End, b.today), cal, func(k string, _ time.Time) {
		st := core.ResolveStatus(k, b.states[k], b.now, cal, n.settings.Weekdays, b.since)
		if (st == core.StatusPartial || st == core.StatusMissed) &&
			core.IsMissingDay(k, core.DayKeyWeekday(k, loc), st, b.today, b.since, n.settings.Weekdays) {
			d.MissingDays = append(d.MissingDays, k)
		}
	})

	if p.Scale == core.ScaleWeek {
		d.Tiles = n.reviewTiles(b)
	}
	if line, ok := core.ReviewCountsLine(s.Counts); ok {
		d.CountsLine = line
	} else if p.Scale == core.ScaleWeek && allQuiet(d.Tiles) {
		d.CountsLine = "No working days to count yet."
	} else {
		d.CountsLine = "Nothing to count yet."
	}
	if d.Empty {
		d.EmptyText = reviewEmptyText(p, cal)
	}

	short := func(k string) string { return core.DayKeyFormat(k, "EEE d MMM", loc) }
	for _, day := range s.Days {
		v := ReviewDayView{Day: day.Day, Label: short(day.Day), Spoken: core.DayKeyFormat(day.Day, "EEEE d MMMM", loc),
			Status: day.Status, Words: day.Words, SkipReason: day.SkipReason}
		if !day.IsSkipped() {
			v.HTML = b.html(day.Body)
		}
		d.Days = append(d.Days, v)
	}
	for _, g := range s.Groups {
		v := ReviewGroupView{Title: cleanHeading(g.Title), Kind: g.Kind, Items: []ReviewItemView{}}
		for _, it := range g.Items {
			iv := ReviewItemView{Day: it.Day, Label: core.DayKeyFormat(it.Day, "EEE d", loc), Text: it.Text}
			if g.Kind != core.GroupSkipped {
				iv.HTML = b.html(it.Text)
			}
			v.Items = append(v.Items, iv)
		}
		d.Groups = append(d.Groups, v)
	}
	for _, m := range s.Months {
		name := core.DayKeyFormat(m.Month+"-01", "MMMM", loc)
		v := ReviewMonthView{Month: m.Month, Name: name, Counts: m.Counts, IsFuture: m.IsFuture, Inert: m.IsFuture || m.Counts.Total() == 0}
		switch {
		case m.IsFuture:
			v.Line, v.Spoken = "–", name+", not started"
		case m.Counts.Total() == 0:
			v.Line, v.Spoken = "–", name+", nothing to count"
		default:
			v.Line = core.ReviewRowLine(m)
			v.Spoken = name + ", " + v.Line
		}
		d.Months = append(d.Months, v)
	}
	return d, nil
}

func allQuiet(tiles []ReviewTile) bool {
	for _, t := range tiles {
		if t.Status != core.StatusOff && t.Status != core.StatusFuture {
			return false
		}
	}
	return true
}

func reviewEmptyText(p core.ReviewPeriod, cal core.Calendar) string {
	switch p.Scale {
	case core.ScaleWeek:
		return "Nothing logged this week yet."
	case core.ScaleMonth:
		return "Nothing logged in " + core.DayKeyFormat(p.Start, "MMMM", time.Local) + " yet."
	case core.ScaleYear:
		return "No pages in " + p.Start[:4] + " yet."
	}
	return "Nothing logged in this range yet."
}

var statusWords = map[core.DayStatus]string{
	core.StatusLogged: "logged", core.StatusPartial: "started", core.StatusMissed: "not logged",
	core.StatusSkipped: "skipped", core.StatusOff: "not a working day", core.StatusFuture: "upcoming",
}

// reviewTiles is the seven days of a week: every day opens (except those still to come), empty weeks included.
func (n *Notes) reviewTiles(b reviewBuilt) []ReviewTile {
	tiles := make([]ReviewTile, 0, 7)
	for i := 0; i < 7; i++ {
		day, ok := core.DayKeyAdding(b.sum.Period.Start, i, time.Local)
		if !ok {
			continue
		}
		st := core.ResolveStatus(day, b.states[day], b.now, b.cal, n.settings.Weekdays, b.since)
		doc, has := b.docs[day]
		bottom := ""
		switch st {
		case core.StatusLogged, core.StatusPartial:
			if has {
				bottom = core.Plural(doc.Words(), "word")
			}
		case core.StatusSkipped:
			bottom = "Skipped"
			if has && doc.SkipReason != "" {
				bottom = doc.SkipReason
			}
		case core.StatusMissed:
			bottom = "Write"
		}
		num := 0
		for _, r := range day[8:] {
			num = num*10 + int(r-'0')
		}
		tiles = append(tiles, ReviewTile{Day: day, Weekday: core.DayKeyFormat(day, "EEE", time.Local), Number: num, Status: st,
			Bottom: bottom, IsToday: day == b.today, Opens: day <= b.today,
			Spoken: core.DayKeyFormat(day, "EEEE d MMMM", time.Local) + ", " + statusWords[st]})
	}
	return tiles
}

// cleanHeading drops a leading emoji or mark from a heading ("✅ Finished" -> "Finished").
func cleanHeading(raw string) string {
	t := strings.TrimLeftFunc(raw, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if t = strings.TrimSpace(t); t != "" {
		return t
	}
	return strings.TrimSpace(raw)
}

// ---- copy ----

// ReviewCopy is a review as text. Text is the markdown ("markdown", "rich") or the plain text ("plain"); HTML is the pasteboard
// page for "rich" and the printable fragment for "print" (images by /dlasset/ link, drawn by the page itself).
type ReviewCopy struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	HTML  string `json:"html"`
}

func (n *Notes) ReviewText(req ReviewRequest, flavor string) (ReviewCopy, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, err := n.reviewBuild(req)
	if err != nil {
		return ReviewCopy{}, err
	}
	sum := core.WithoutPersonal(b.sum) // a copy is the work review; the screen and the export keep Personal
	md := core.ReviewMarkdown(sum, b.cal)
	out := ReviewCopy{Title: core.ReviewTitle(sum.Period, b.cal, false), Text: md}
	switch flavor {
	case "plain":
		out.Text = core.MDPlain(md)
	case "rich":
		out.HTML = core.ReviewRichHTML(sum, b.cal)
	case "print": // the PDF is an export: Personal stays in
		out.HTML = b.html(core.ReviewMarkdown(b.sum, b.cal))
	case "markdown":
	default:
		return ReviewCopy{}, errors.New("unknown copy flavor: " + flavor)
	}
	return out, nil
}

// ReviewCopyText puts plain text on the clipboard (the page writes the rich flavour itself, in the click that asked for it).
func (n *Notes) ReviewCopyText(text string) error {
	app := application.Get()
	if app == nil || !app.Clipboard.SetText(text) {
		return errors.New("couldn't reach the clipboard")
	}
	return nil
}

// ---- export ----

// ReviewExportInfo says what an export would hold, for the sheet. Nothing is written.
type ReviewExportInfo struct {
	Name   string `json:"name"` // "Gloamlog 2026-W41"
	Days   int    `json:"days"`
	Images int    `json:"images"`
	Detail string `json:"detail"` // "5 days, 2 images. Skipped days included. Jots left out."
}

func (n *Notes) ReviewExportInfo(req ReviewRequest) (ReviewExportInfo, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, err := n.reviewBuild(req)
	if err != nil {
		return ReviewExportInfo{}, err
	}
	plan := n.reviewPlan(b, core.ReviewFileName(b.sum.Period, b.cal))
	return ReviewExportInfo{Name: plan.Name, Days: plan.DayCount, Images: len(plan.Assets), Detail: exportDetail(plan, req)}, nil
}

func exportDetail(plan core.ExportPlan, req ReviewRequest) string {
	s := core.Plural(plan.DayCount, "day")
	if len(plan.Assets) > 0 {
		s += ", " + core.Plural(len(plan.Assets), "image")
	}
	s += ". Skipped days " + map[bool]string{true: "included", false: "left out"}[req.IncludeSkipped] + "."
	s += " Jots " + map[bool]string{true: "included", false: "left out"}[req.IncludeJots] + "."
	return s
}

func (n *Notes) reviewPlan(b reviewBuilt, name string) core.ExportPlan {
	days := len(b.sum.Days)
	if b.sum.Period.Scale == core.ScaleYear {
		days = b.sum.Counts.Logged
	}
	return core.PlanExport(name, core.ReviewMarkdown(b.sum, b.cal), days, n.store.Assets())
}

// ReviewChoosePlace asks where to save ("markdown", "zip") or which folder to put the new folder in ("folder").
// "" means the person cancelled.
func (n *Notes) ReviewChoosePlace(req ReviewRequest, kind string) (string, error) {
	n.mu.Lock()
	name := core.ReviewFileName(n.reviewPeriod(req), n.cal())
	n.mu.Unlock()
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to ask from")
	}
	win, haveWin := app.Window.GetByName("main")
	switch kind {
	case "folder":
		d := app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
			SetTitle("Export").SetMessage("Choose where to put the “" + name + "” folder.").SetButtonText("Choose")
		if haveWin {
			d.AttachToWindow(win)
		}
		return d.PromptForSingleSelection()
	case "markdown", "zip":
		d := app.Dialog.SaveFile().CanCreateDirectories(true).SetButtonText("Save")
		if kind == "markdown" {
			d.SetFilename(name+".md").AddFilter("Markdown", "*.md").SetMessage("The images this review uses go in an assets folder beside the file.")
		} else {
			d.SetFilename(name+".zip").AddFilter("Zip", "*.zip")
		}
		if haveWin {
			d.AttachToWindow(win)
		}
		path, err := d.PromptForSingleSelection()
		if err == nil && path != "" {
			want := map[string]string{"markdown": ".md", "zip": ".zip"}[kind]
			if !strings.EqualFold(filepath.Ext(path), want) {
				path += want
			}
		}
		return path, err
	}
	return "", errors.New("unknown export kind: " + kind)
}

// ReviewSaved is what a finished export says.
type ReviewSaved struct {
	Line string `json:"line"` // "Saved 5 days and 2 images."
	Path string `json:"path"` // the file, folder or zip that was made: Show in Finder points at it
}

// ReviewWrite writes the export to the place chosen by ReviewChoosePlace (also what Try again calls, with the same place).
func (n *Notes) ReviewWrite(req ReviewRequest, kind, path string) (ReviewSaved, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, err := n.reviewBuild(req)
	if err != nil {
		return ReviewSaved{}, err
	}
	name := core.ReviewFileName(b.sum.Period, b.cal)
	var layout core.ExportLayout
	switch kind {
	case "markdown":
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) // the file is called what the person typed
		layout = core.ExportLayout{Kind: core.ExportFile, Path: path}
	case "folder":
		layout = core.ExportLayout{Kind: core.ExportFolder, Path: path}
	case "zip":
		layout = core.ExportLayout{Kind: core.ExportZip, Path: path}
	default:
		return ReviewSaved{}, errors.New("unknown export kind: " + kind)
	}
	plan := n.reviewPlan(b, name)
	out, err := core.WriteExport(plan, layout)
	if err != nil {
		return ReviewSaved{}, err
	}
	return ReviewSaved{Line: plan.ResultLine(), Path: out}, nil
}
