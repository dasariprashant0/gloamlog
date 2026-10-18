package main

import "testing"

// The strip's bullets go where the page keeps its lines: under Log on a list page, under the configured heading otherwise.
func TestSuggestionsGoUnderLogOnAListPage(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	if _, err := n.store.Save("2026-10-14", "## Log\n\n- 09:00 shipped the export"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.store.Save("2026-10-13", "## What I did\n\nwords"); err != nil {
		t.Fatal(err)
	}
	list, err := n.Suggestions("2026-10-14", false)
	if err != nil || list.Heading != "Log" {
		t.Fatalf("list page: heading %q, err %v", list.Heading, err)
	}
	classic, err := n.Suggestions("2026-10-13", false)
	if err != nil || classic.Heading != n.Settings().Sources.HeadingText() {
		t.Fatalf("classic page: heading %q, err %v", classic.Heading, err)
	}
	// no page yet, on a five-heading template (captureNotes): the configured heading
	none, err := n.Suggestions("2026-10-12", false)
	if err != nil || none.Heading != n.Settings().Sources.HeadingText() {
		t.Fatalf("no page: heading %q, err %v", none.Heading, err)
	}
}
