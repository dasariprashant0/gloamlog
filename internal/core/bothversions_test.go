package core

import (
	"reflect"
	"testing"
)

func TestMergeBoth(t *testing.T) {
	mine := "## Today\n- shipped the thing\n- wrote tests\n"
	// the other copy: one shared line, two new ones that were separated by a blank line, one indented
	theirs := "## Today\n- shipped the thing\n\n- called Sam\n\n  - about the invoice\n- wrote tests"
	got := MergeBoth(mine, theirs)
	want := "## Today\n- shipped the thing\n- wrote tests\n\n## From another copy\n\n- called Sam\n\n  - about the invoice"
	if got != want {
		t.Errorf("MergeBoth =\n%q\nwant\n%q", got, want)
	}
}

func TestMergeBothKeepsMineWhenNothingIsNew(t *testing.T) {
	mine := "line one\nline two\n"
	for _, theirs := range []string{"", "line one", "line two\nline one", "  line one  \n\n", "line one\r\nline two\r\n"} {
		if got := MergeBoth(mine, theirs); got != mine {
			t.Errorf("MergeBoth(mine, %q) = %q, want mine unchanged", theirs, got)
		}
	}
}

func TestMergeBothWithNothingOfOurs(t *testing.T) {
	if got := MergeBoth("", "a\nb"); got != "a\nb" {
		t.Errorf("empty base = %q", got)
	}
	if got := MergeBoth(" \n\t\n", "a"); got != "a" {
		t.Errorf("whitespace-only base = %q", got)
	}
}

func TestBothExtraLinesBlankLinesOnlyBetweenExtras(t *testing.T) {
	got := BothExtraLines("keep", "new1\n\nkeep\n\nnew2\nnew3\n\n\nnew4")
	want := []string{"new1", "new2", "new3", "", "new4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMarkLines(t *testing.T) {
	got := MarkLines("a\n\nb\n  c  ", "c\na")
	want := []MarkedLine{{"a", false}, {"", false}, {"b", true}, {"  c  ", false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}
