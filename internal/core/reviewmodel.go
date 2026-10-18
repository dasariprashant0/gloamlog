// reviewmodel.go - the value types of the Review screen and its copies and exports. No disk, no clock.
// Port of app/Core/Review/ReviewModel.swift.
//
// A ReviewPeriod is an inclusive span of day keys for one of four scales. BuildReview (periodreview.go) turns the day files of a period
// into a ReviewSummary: counts (logged / skipped / started / not written scheduled days), the pages to show (Days, for "Group by Day"),
// the roll-ups per template heading plus Jots and "Other notes" (Groups, for "Group by Section"), the tasks done and still open, and
// for a Year the 12 month rows. ReviewOptions decide what is in it, so the screen and every copy and export show the same thing.
package core

import "strings"

// ReviewScale is week, month, year or a custom range.
type ReviewScale string

const (
	ScaleWeek  ReviewScale = "week"
	ScaleMonth ReviewScale = "month"
	ScaleYear  ReviewScale = "year"
	ScaleRange ReviewScale = "range"
)

// Title is the scale's name with a capital: "Week".
func (s ReviewScale) Title() string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(string(s[:1])) + string(s[1:])
}

// ReviewPeriod is an inclusive span of day keys ("2026-10-05" ... "2026-10-11").
type ReviewPeriod struct {
	Scale ReviewScale `json:"scale"`
	Start string      `json:"start"`
	End   string      `json:"end"`
}

// ReviewOptions decide what a review shows. The zero value is Swift's default except Grouping: WeekByDay is 0 here, Swift defaulted
// to .section, so use DefaultReviewOptions.
type ReviewOptions struct {
	Grouping       WeekGrouping `json:"grouping"`
	IncludeSkipped bool         `json:"includeSkipped"` // off: skipped days vanish from the review, its counts and every export
	IncludeJots    bool         `json:"includeJots"`    // the Jots block is its own group (or stays in place by day) only when on
}

// DefaultReviewOptions are Swift's defaults: by section, skipped days included, Jots left out.
func DefaultReviewOptions() ReviewOptions {
	return ReviewOptions{Grouping: WeekBySection, IncludeSkipped: true}
}

// ReviewCounts: Logged + Skipped + Started + Unlogged = the scheduled days that have happened (plus pages written on days off).
// "Not logged" is Started + Unlogged; today is never counted as unlogged while it is still going.
type ReviewCounts struct {
	Logged   int `json:"logged"`
	Skipped  int `json:"skipped"`
	Started  int `json:"started"`
	Unlogged int `json:"unlogged"`
}

// NotWritten is Started + Unlogged.
func (c ReviewCounts) NotWritten() int { return c.Started + c.Unlogged }

// Total is every counted day.
func (c ReviewCounts) Total() int { return c.Logged + c.Skipped + c.Started + c.Unlogged }

// Scheduled is the 5 of "Logged 4 of 5".
func (c ReviewCounts) Scheduled() int { return c.Total() }

func (c *ReviewCounts) add(o ReviewCounts) {
	c.Logged += o.Logged
	c.Skipped += o.Skipped
	c.Started += o.Started
	c.Unlogged += o.Unlogged
}

// ReviewItem is some text a day wrote.
type ReviewItem struct {
	Day  string `json:"day"`
	Text string `json:"text"`
}

// ReviewGroupKind says what a group is.
type ReviewGroupKind string

const (
	GroupHeading ReviewGroupKind = "heading"
	GroupJots    ReviewGroupKind = "jots"
	GroupOther   ReviewGroupKind = "other"
	GroupSkipped ReviewGroupKind = "skipped"
)

// ReviewGroup is one roll-up of the "by section" view.
type ReviewGroup struct {
	Title string          `json:"title"`
	Kind  ReviewGroupKind `json:"kind"`
	Items []ReviewItem    `json:"items"`
}

// TaskStats: Completed = every ticked task of the period (once each); OpenAtEnd = what the last logged day still carries under the
// carry-over headings (the same rule as "From yesterday").
type TaskStats struct {
	Completed []ReviewItem `json:"completed"`
	OpenAtEnd []string     `json:"openAtEnd"`
}

// MonthRow is one month of a Year review.
type MonthRow struct {
	Month    string       `json:"month"` // "2026-09"
	Counts   ReviewCounts `json:"counts"`
	IsFuture bool         `json:"isFuture"` // the month has not started: shown as dashes, inert
}

// ReviewDay is a day with something to show: a page with writing, or (when included) a skip marker.
type ReviewDay struct {
	Day        string    `json:"day"`
	Status     DayStatus `json:"status"` // logged, partial or skipped
	Words      int       `json:"words"`
	SkipReason string    `json:"skipReason"`
	Body       string    `json:"body"` // the page as shown: Jots left out unless included, empty headings pruned; "" when skipped
}

// IsSkipped is true for a skip marker.
func (d ReviewDay) IsSkipped() bool { return d.Status == StatusSkipped }

// ReviewSummary is everything the Review screen and its copies show.
type ReviewSummary struct {
	Period  ReviewPeriod  `json:"period"`
	Options ReviewOptions `json:"options"`
	Counts  ReviewCounts  `json:"counts"`
	Days    []ReviewDay   `json:"days"`
	Groups  []ReviewGroup `json:"groups"`
	Tasks   TaskStats     `json:"tasks"`
	Months  []MonthRow    `json:"months"` // Year only
}

// IsEmpty: nothing to show: no page and no (included) skip in the period; for a Year, no page or skip in any month.
func (s ReviewSummary) IsEmpty() bool {
	if s.Period.Scale == ScaleYear {
		for _, m := range s.Months {
			if m.Counts.Logged+m.Counts.Skipped+m.Counts.Started != 0 {
				return false
			}
		}
		return true
	}
	return len(s.Days) == 0
}
