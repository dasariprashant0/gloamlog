package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// A settings.json that can't be decoded is moved aside before the defaults take over, so the next save can't erase it.
func TestDamagedSettingsAreSetAsideNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	now := time.Date(2026, 10, 18, 17, 30, 0, 0, time.UTC)

	if note, blocked := setAsideDamagedSettings(path, now); note != "" || blocked {
		t.Fatalf("missing file is a first run: %q %v", note, blocked)
	}
	if err := os.WriteFile(path, []byte(`{"storageFolder":"/Users/someone/Notes","minWords":30}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if note, _ := setAsideDamagedSettings(path, now); note != "" {
		t.Fatalf("a good file must stay: %q", note)
	}
	damaged := `{"storageFolder":"/Users/someone/Notes","minWords":"thirty"` // wrong type and cut off
	if err := os.WriteFile(path, []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}
	note, blocked := setAsideDamagedSettings(path, now)
	if blocked || !strings.Contains(note, "settings.json.damaged-20261018-173000") {
		t.Fatalf("note %q blocked %v", note, blocked)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the damaged file should have moved away, stat err = %v", err)
	}
	if got, err := os.ReadFile(path + ".damaged-20261018-173000"); err != nil || string(got) != damaged {
		t.Errorf("set-aside copy: %q, %v", got, err)
	}
}

// A settings.json that exists but can't be read (no permission, a disk error) is not damaged: it stays put and blocks saves.
func TestUnreadableSettingsStayInPlaceAndBlockSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minWords":30}`), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running as a user who can read a 000 file")
	}
	note, blocked := setAsideDamagedSettings(path, time.Now())
	if !blocked || note == "" {
		t.Fatalf("note %q blocked %v", note, blocked)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("the file moved: %v", err)
	}
}
