package main

import "testing"

// The live "n/20" label agrees with "logged" on a list page; a classic page keeps counting every word, as before.
func TestWordsLabelMatchesLoggedOnAListPage(t *testing.T) {
	n, _, _, _ := captureNotes(t, true)
	if got := n.Words("## Log\n\n- 09:40 fixed the login loop #next\n- 10:00 two words"); got != 6 {
		t.Fatalf("list page: %d words, want 6 (no times, no marks)", got)
	}
	if got := n.Words("## What I did\n\none two\n\n## Jots\n\n- 09:40 three"); got != 4 {
		t.Fatalf("classic page: %d words, want 4", got)
	}
	for _, empty := range []string{"", "  \n\t", "## Log", "## Log\n\n- 09:40 #next"} {
		if got := n.Words(empty); got != 0 {
			t.Fatalf("%q: %d words, want 0", empty, got)
		}
	}
}
