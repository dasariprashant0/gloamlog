package core

// Shared test helpers. The Swift tests in tests/main.swift had a tiny runner (test/expect); the same fixtures live here:
// a New York calendar whose weeks start on Monday, "2026-10-05 is a Monday", noon-based dates, temp folders. Every test works in
// t.TempDir(), never in the real home folder.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var nyLoc = mustLoc("America/New_York")

// ny is the harness calendar of the Swift tests: America/New_York, first weekday Monday, no locale (the root locale).
var ny = Calendar{Loc: nyLoc, FirstWeekday: 2}

// workdays is Monday to Friday.
var workdays = WeekdaySet{2, 3, 4, 5, 6}

var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3, 4}

func mustLoc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// calIn is the Swift calIn(): a calendar in any zone with a given first weekday and optional locale.
func calIn(tz string, firstWeekday int, locale string) Calendar {
	return Calendar{Loc: mustLoc(tz), FirstWeekday: firstWeekday, Locale: locale}
}

// mk is a New York instant (Swift mk(y, m, d, h, mi)).
func mk(y, m, d, h, mi int) time.Time { return time.Date(y, time.Month(m), d, h, mi, 0, 0, nyLoc) }

// noon is mk at 12:00.
func noon(y, m, d int) time.Time { return mk(y, m, d, 12, 0) }

// utcCal is the independent oracle for day arithmetic: it uses neither DayKey nor the zone under test.
func utcKey(t time.Time) string { return t.UTC().Format("2006-01-02") }
func utcDate(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.UTC)
}

// lorem is n words "w1 w2 ... wn" ("" for 0).
func lorem(n int, prefix string) string {
	var w []string
	for i := 1; i <= n; i++ {
		w = append(w, fmt.Sprintf("%s%d", prefix, i))
	}
	return strings.Join(w, " ")
}

func newStore(t *testing.T) *LogStore {
	t.Helper()
	return NewLogStore(t.TempDir(), "", 0, nil)
}

func writeRaw(t *testing.T, dir, day, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, day+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileText(t *testing.T, dir, day string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, day+".md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// expect fails the test (and goes on) when cond is false; the message formats like Printf.
func expect(t *testing.T, cond bool, msg ...any) {
	t.Helper()
	if !cond {
		if len(msg) > 0 {
			t.Errorf("%s", fmt.Sprintf(fmt.Sprint(msg[0]), msg[1:]...))
		} else {
			t.Error("expectation failed")
		}
	}
}

// must stops the test when err is not nil.
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// val unwraps a (value, error) pair; an error panics, which the testing package reports as a failure of the running test.
func val[T any](v T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("unexpected error: %v", err))
	}
	return v
}

// expectErr checks that err is a LogError of the given kind (and, when detail != "-", carries that day or path).
func expectErr(t *testing.T, err error, kind error, detail string) {
	t.Helper()
	if !errors.Is(err, kind) {
		t.Errorf("error = %v, want kind %q", err, kind)
		return
	}
	if detail != "-" && LogErrorDetail(err) != detail {
		t.Errorf("error detail = %q, want %q", LogErrorDetail(err), detail)
	}
}

// load fetches a day or fails the test.
func load(t *testing.T, s *LogStore, day string) *DayDocument {
	t.Helper()
	doc, err := s.Load(day)
	must(t, err)
	return doc
}

func save(t *testing.T, s *LogStore, day, body string) SaveOutcome {
	t.Helper()
	out, err := s.Save(day, body)
	must(t, err)
	return out
}

func states(pairs ...any) map[string]DayStatus {
	m := map[string]DayStatus{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1].(DayStatus)
	}
	return m
}

func equal(a, b any) bool { return reflect.DeepEqual(a, b) }

// strs compares two string slices treating nil and empty alike.
func strs(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// W is the word count of a body (Swift: W()).
func W(s string) int { return CountWords(s) }

// chmod changes permissions or fails the test.
func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func setMTime(t *testing.T, path string, mt time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	must(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
