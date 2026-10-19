package main

import "testing"

func TestShouldQuitHoldsUnsavedEdits(t *testing.T) {
	defer func() { unsavedEdits.Store(false); quitAnyway.Store(false) }()
	if !shouldQuit() {
		t.Fatal("nothing unsaved: quits")
	}
	unsavedEdits.Store(true)
	if shouldQuit() {
		t.Fatal("unsaved edits: the quit is held")
	}
	quitAnyway.Store(true)
	if !shouldQuit() {
		t.Fatal("Quit anyway: quits")
	}
}
