// capturetypes.go - quick capture ("Jot") shared types. Port of app/Core/Capture/CaptureTypes.swift.
// HotKeySpec and CapturePrefs live in settings.go (Settings holds them).
//
// PageHandle / EditorLookup are the seam to the live editor: the Core never imports it. The desktop service implements them over the
// editor bridge. Callbacks may run on any goroutine, synchronously or later; DayWriter copes with both.
package core

import "time"

// CaptureItem is one captured note. It is written to the CaptureJournal before anything else happens, so it can never be lost.
type CaptureItem struct {
	ID     string    `json:"id"`     // UUID string
	Ts     time.Time `json:"ts"`     // when the note was captured (whole milliseconds survive the journal)
	Day    string    `json:"day"`    // the effective "today" when captured, yyyy-MM-dd; never changes
	Text   string    `json:"text"`   // trimmed, control characters removed (newlines kept), at most 4,000 characters
	Stamp  string    `json:"stamp"`  // "14:32"; "" when timestamps are off or the note is a to-do (Swift: String?)
	Todo   bool      `json:"todo"`   // the text began with "[]" and todoShorthand is on; the "[]" is already removed from Text
	Source string    `json:"source"` // "hotkey", "menubar", "menu", "claude"
}

// JotAppendKind is what the live editor did with an append request.
type JotAppendKind int

const (
	JotAppended       JotAppendKind = iota // the page now holds the note; NormalizedMarkdown is the editor's markdown after the change
	JotAlreadyPresent                      // a replay of a line the page already contains
	JotNotLoaded                           // the page is not ready yet: the writer keeps the note and retries after load
	JotStale                               // the page was swapped while the call was in flight: re-route once
)

// JotAppendResult is the answer of PageHandle.AppendJot. (Swift: JotAppendResult, with .appended(normalizedMarkdown:).)
type JotAppendResult struct {
	Kind               JotAppendKind
	NormalizedMarkdown string // only for JotAppended
}

// PageHandle is the open page for a day. The editor layer implements it.
type PageHandle interface {
	Day() string
	IsLoaded() bool
	// AppendJot appends the rendered note under the Jots heading in the LIVE document (one editor transaction: the user's caret, text and
	// undo history are untouched) and reports the outcome through done, once. done may be called before AppendJot returns.
	AppendJot(item CaptureItem, done func(JotAppendResult))
}

// EditorLookup finds the page currently open for a day.
type EditorLookup interface {
	// Page returns the page open for day (including a hidden window's page and pages that could not be saved), or nil. Return a true
	// nil interface, not a typed nil pointer. It must not call back into the DayWriter (it is called with the writer's lock held).
	Page(day string) PageHandle
}

// CaptureReceipt is what a capture returns to the panel.
type CaptureReceipt struct {
	ID      string `json:"id"`
	Waiting int    `json:"waiting"` // journal items not yet acknowledged, including this one; 0 means it is already in the file
}
