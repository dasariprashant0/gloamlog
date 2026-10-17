package main

import (
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// testHome is a made-up home folder: these tests never touch the real one.
const testHome = "/Users/someone"

func TestWritableDecidesWhatReachesDisk(t *testing.T) {
	s := core.DefaultSettings(testHome)
	s.Template = "## What I did\n\n## To do next"
	for _, c := range []struct{ in, want string }{
		{"", ""},                                 // nothing typed: nothing to write
		{"##\n\n###\n#", ""},                     // select-all + delete leaves bare markers: emptied
		{"## What I did\n\n## To do next\n", ""}, // untouched template: nothing to write
		{"## What I did\n- shipped it\n", "## What I did\n- shipped it\n"}, // real words stay
		{"####### seven", "####### seven"},                                 // not a heading marker: text
		{"## Done", "## Done"},                                             // a heading with words is an edit
	} {
		if got := writable(c.in, s); got != c.want {
			t.Errorf("writable(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
