package core

// Ported from tests/main.swift "K2 CaptureJournal" (append/pending/ack order, record format, torn last line, tolerant reading,
// unwritable folder, permissions, compaction, a writer dropped without any ack).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// jdir is a journal folder that does not exist yet: the journal makes it.
func jdir(t *testing.T) string { return filepath.Join(t.TempDir(), "capture") }

func jfile(dir string) string { return filepath.Join(dir, "journal.jsonl") }

func fmode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func appendRaw(t *testing.T, path, s string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	must(t, err)
	_, err = f.WriteString(s)
	must(t, err)
	must(t, f.Close())
}

func jlines(path string) []string {
	b, _ := os.ReadFile(path)
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// jitem is the Swift test's jitem(n): a note "note n" captured n seconds after the fixture time.
func jitem(n int) CaptureItem {
	return CaptureItem{ID: fmt.Sprintf("id-%d", n), Ts: jotT0.Add(time.Duration(n) * time.Second), Day: "2026-10-07",
		Text: fmt.Sprintf("note %d", n), Stamp: "14:32", Source: "hotkey"}
}

// sameItems compares notes field by field (a time read back from the journal is in UTC, the fixture's is in New York).
func sameItems(a, b []CaptureItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.ID != y.ID || !x.Ts.Equal(y.Ts) || x.Day != y.Day || x.Text != y.Text || x.Stamp != y.Stamp || x.Todo != y.Todo || x.Source != y.Source {
			return false
		}
	}
	return true
}

func ids(items []CaptureItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestJournalAppendPendingAckInOrderFolderCreatedFile0600(t *testing.T) {
	d := jdir(t)
	j := NewCaptureJournal(d)
	a, b, c := jitem(1), jitem(2), jitem(3)
	expect(t, len(j.Pending()) == 0, "no file yet")
	must(t, j.Append(a))
	must(t, j.Append(b))
	must(t, j.Append(c))
	expect(t, sameItems(j.Pending(), []CaptureItem{a, b, c}))
	must(t, j.Ack(b.ID))
	expect(t, sameItems(j.Pending(), []CaptureItem{a, c}))
	must(t, j.Ack("never-seen"))
	expect(t, sameItems(j.Pending(), []CaptureItem{a, c}), "an unknown ack changes nothing")
	must(t, j.Ack(a.ID))
	must(t, j.Ack(c.ID))
	expect(t, len(j.Pending()) == 0)
	expect(t, j.Path() == jfile(d) && fmode(t, jfile(d)) == 0o600, "journal.jsonl, mode %o", fmode(t, jfile(d)))
	expect(t, fmode(t, d) == 0o700, "the folder is private too: %o", fmode(t, d))
	expect(t, len(NewCaptureJournal(d).Pending()) == 0 && NewCaptureJournal(d).Path() == j.Path())
}

func TestJournalOneJSONLinePerRecordAndAnyTextSurvives(t *testing.T) {
	d := jdir(t)
	j := NewCaptureJournal(d)
	wild := "line one\nline \"two\" \\ back\tslash   sep \U0001F468‍\U0001F469‍\U0001F467 café </script> {\"x\":1} \x07 bell \x7f del"
	items := []CaptureItem{jitem(1), jitem(2), jitem(3)}
	items[0].Text = wild
	items[1].Text = strings.Repeat("word ", 800)
	items[2].Text = "[] literal"
	for _, it := range items {
		must(t, j.Append(it))
	}
	must(t, j.Ack("id-9"))
	lines := jlines(jfile(d))
	expect(t, len(lines) == 4, "three notes and one ack, one physical line each: %d", len(lines))
	for _, l := range lines[:3] {
		var o map[string]any
		must(t, json.Unmarshal([]byte(l), &o))
		_, hasID := o["id"].(string)
		_, hasTS := o["ts"].(string)
		expect(t, o["v"] == float64(1) && hasID && hasTS && o["day"] == "2026-10-07" && o["text"] != nil && o["source"] == "hotkey" && o["todo"] == false, l)
	}
	expect(t, strings.HasPrefix(lines[0], `{"day":"2026-10-07","id":"id-1","source":"hotkey","stamp":"14:32","text":"line one\nline \"two\" \\ back\tslash `), "keys sorted: %s", lines[0])
	expect(t, strings.Contains(lines[0], `"ts":"2026-10-07T18:32:01.000Z"`) && strings.Contains(lines[0], `"todo":false,"ts"`) && strings.HasSuffix(lines[0], `"v":1}`), lines[0])
	expect(t, strings.Contains(lines[0], "</script>") && strings.Contains(lines[0], " "), "slash, angle brackets and U+2028 are written as they are")
	expect(t, lines[3] == `{"ack":"id-9"}`, lines[3])
	expect(t, sameItems(j.Pending(), items), "exact round trip")

	withMs := CaptureItem{ID: "ms", Ts: time.Unix(1_790_000_000, 123_000_000), Day: "2026-10-07", Text: "x", Todo: true, Source: "menu"}
	must(t, j.Append(withMs))
	p := j.Pending()
	back := p[len(p)-1]
	expect(t, back.Stamp == "" && back.Todo && back.Source == "menu" && back.Ts.Sub(withMs.Ts).Abs() < time.Millisecond, "milliseconds, no stamp, to-do")
	expect(t, strings.Contains(jlines(jfile(d))[4], `"ts":"2026-09-21T`), "the ISO text: %s", jlines(jfile(d))[4])
}

func TestJournalISOFormatWholeMillisecondsRounded(t *testing.T) {
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Unix(0, 0), "1970-01-01T00:00:00.000Z"},
		{time.Unix(1_790_000_000, 123_456_789), "2026-09-21T14:13:20.123Z"},
		{time.Unix(1_790_000_000, 999_600_000), "2026-09-21T14:13:21.000Z"}, // rounds up into the next second
		{time.Unix(1_790_000_000, 1_500_000), "2026-09-21T14:13:20.002Z"},   // half rounds away from zero
		{time.Unix(-1, 500_000_000), "1969-12-31T23:59:59.500Z"},
	}
	for _, c := range cases {
		expect(t, journalISO(c.t) == c.want, "%v: got %s want %s", c.t, journalISO(c.t), c.want)
	}
}

func TestJournalTornLastLineNeverLosesEarlierNotesAndNextAppendIsNotGlued(t *testing.T) {
	d := jdir(t)
	j := NewCaptureJournal(d)
	a, b, c := jitem(1), jitem(2), jitem(3)
	must(t, j.Append(a))
	must(t, j.Append(b))
	appendRaw(t, jfile(d), `{"v":1,"id":"torn","ts":"2026-10-07T14:3`) // power cut mid-write: no newline
	expect(t, sameItems(j.Pending(), []CaptureItem{a, b}), "the torn line is ignored, earlier notes intact")
	must(t, j.Append(c))
	expect(t, sameItems(j.Pending(), []CaptureItem{a, b, c}), "a new note after a torn line is a note of its own")
	expect(t, len(jlines(jfile(d))) == 4, "the fragment sits on its own line")
	appendRaw(t, jfile(d), `{"ack":"id-`) // a torn ack
	expect(t, sameItems(j.Pending(), []CaptureItem{a, b, c}), "a torn ack acknowledges nothing")
	must(t, j.Ack(a.ID))
	expect(t, sameItems(j.Pending(), []CaptureItem{b, c}))
}

func TestJournalUnknownGarbledAndFutureLinesSkippedOldRecordsAndDuplicatesTolerated(t *testing.T) {
	d := jdir(t)
	j := NewCaptureJournal(d)
	good := jitem(1)
	text := []string{"", "not json at all", `{"foo":1}`, `[1,2]`, `42`, `"str"`, `{"ack":42}`, `{"v":9,"kind":"future"}`, `{"ack":null}`,
		`{"id":"bad-day","ts":"2026-10-07T14:32:11Z","day":"../../etc","text":"x","source":"hotkey"}`,
		`{"id":"no-text","ts":"2026-10-07T14:32:11Z","day":"2026-10-07","text":"","source":"hotkey"}`,
		`{"id":"no-ts","day":"2026-10-07","text":"x","source":"hotkey"}`,
		`{"id":"","ts":"2026-10-07T14:32:11Z","day":"2026-10-07","text":"x","source":"hotkey"}`,
		journalLine(good), journalLine(good), // the same id twice
		`{"id":"old","ts":"2026-10-07T14:32:11Z","day":"2026-10-07","text":"hi","stamp":"14:32","source":"hotkey"}`, // no v, no todo
		`{"v":2,"id":"newer","ts":"2026-10-07T14:33:00.250Z","day":"2026-10-07","text":"from the future","stamp":null,"todo":true,"source":"x","extra":[1]}`,
		`{"id":"epoch","ts":1790000000.5,"day":"2026-10-07","text":"numeric time"}`,
		`{"ack":"never","also":1}`}
	appendRaw(t, jfile(d), strings.Join(text, "\n")+"\n")
	p := j.Pending()
	expect(t, equal(ids(p), []string{"id-1", "old", "newer", "epoch"}), "%v", ids(p))
	wantOld, _ := time.Parse(time.RFC3339, "2026-10-07T14:32:11Z")
	expect(t, !p[1].Todo && p[1].Stamp == "14:32" && p[1].Ts.Equal(wantOld) && p[1].Text == "hi", "a record without v and todo reads")
	wantNewer, _ := time.Parse(time.RFC3339, "2026-10-07T14:33:00Z")
	expect(t, p[2].Todo && p[2].Stamp == "" && p[2].Ts.Sub(wantNewer) == 250*time.Millisecond, "unknown fields are ignored")
	expect(t, p[3].Source == "unknown" && p[3].Ts.Equal(time.Unix(1_790_000_000, 500_000_000)), "a numeric time reads as epoch seconds, a missing source is unknown")
	appendRaw(t, jfile(d), `{"ack":"old","x":1}`+"\n")
	expect(t, equal(ids(j.Pending()), []string{"id-1", "newer", "epoch"}), "an ack with extra keys still acks")
	appendRaw(t, jfile(d), `{"ack":"id-1"}`+"\n"+journalLine(jitem(1))+"\n")
	expect(t, equal(ids(j.Pending()), []string{"newer", "epoch"}), "acks are not order dependent: an ack before the note still acks it")
}

func TestJournalUnwritableFolderIsATypedErrorAndRecovers(t *testing.T) {
	// the folder's parent is a file
	blocker := filepath.Join(t.TempDir(), "file")
	must(t, os.WriteFile(blocker, []byte("x"), 0o644))
	j1 := NewCaptureJournal(filepath.Join(blocker, "capture"))
	var je *JournalError
	err := j1.Append(jitem(1))
	expect(t, err != nil && asJournalError(err, &je), "append must fail with a JournalError: %v", err)
	err = j1.Ack("id-1")
	expect(t, err != nil && asJournalError(err, &je), "ack must fail with a JournalError: %v", err)
	expect(t, len(j1.Pending()) == 0)

	// a folder without write permission
	d := jdir(t)
	j2 := NewCaptureJournal(d)
	must(t, j2.Append(jitem(1)))
	chmod(t, d, 0o500)
	chmod(t, jfile(d), 0o400)
	defer func() { _ = os.Chmod(d, 0o700) }() // so the temp folder can be removed whatever happens
	err = j2.Append(jitem(2))
	expect(t, err != nil && asJournalError(err, &je), "append must fail with a JournalError: %v", err)
	expect(t, sameItems(j2.Pending(), []CaptureItem{jitem(1)}), "what is already there is still readable")
	chmod(t, d, 0o700)
	chmod(t, jfile(d), 0o600)
	must(t, j2.Append(jitem(2)))
	expect(t, sameItems(j2.Pending(), []CaptureItem{jitem(1), jitem(2)}), "writable again")
}

func asJournalError(err error, target **JournalError) bool {
	je, ok := err.(*JournalError)
	if ok {
		*target = je
	}
	return ok
}

func TestJournalFileWithWiderPermissionsIsTightenedTo0600(t *testing.T) {
	d := jdir(t)
	must(t, os.MkdirAll(d, 0o755))
	must(t, os.WriteFile(jfile(d), nil, 0o644))
	must(t, os.Chmod(jfile(d), 0o644)) // the umask may have narrowed it
	must(t, NewCaptureJournal(d).Append(jitem(1)))
	expect(t, fmode(t, jfile(d)) == 0o600, "%o", fmode(t, jfile(d)))
}

func TestJournalCompactDropsAcknowledgedNotesKeepsOrderAndModeNoTempAndNoOpWhenClean(t *testing.T) {
	d := jdir(t)
	j := NewCaptureJournal(d)
	var body strings.Builder
	var keep []CaptureItem
	for n := 0; n < 2000; n++ {
		it := jitem(n)
		body.WriteString(journalLine(it) + "\n")
		if n%200 == 7 {
			keep = append(keep, it)
		} else {
			body.WriteString(journalAckLine(it.ID) + "\n")
		}
	}
	body.WriteString("garbage line\n{\"v\":1,\"id\":\"torn") // and a torn last line
	must(t, os.MkdirAll(d, 0o700))
	must(t, os.WriteFile(jfile(d), []byte(body.String()), 0o600))
	t0 := time.Now()
	before := j.Pending()
	readTime := time.Since(t0)
	expect(t, sameItems(before, keep) && len(keep) == 10, "10 of 2,000 are still pending, got %d", len(before))
	expect(t, readTime < 5*time.Second, "reading a 4,000-line journal: %v", readTime)
	must(t, j.Compact())
	expect(t, sameItems(j.Pending(), keep), "the same notes in the same order")
	expect(t, len(jlines(jfile(d))) == 10 && !strings.Contains(strings.Join(jlines(jfile(d)), ""), "garbage"), "10 lines: acked notes, acks, garbage and the torn line are gone")
	expect(t, fmode(t, jfile(d)) == 0o600)
	for _, n := range dirNames(t, d) {
		expect(t, !strings.HasPrefix(n, AtomicTempPrefix), "no temp file: %s", n)
	}
	bytes1, _ := os.ReadFile(jfile(d))
	must(t, j.Compact())
	bytes2, _ := os.ReadFile(jfile(d))
	expect(t, string(bytes1) == string(bytes2), "compacting a clean journal changes nothing")
	must(t, j.Append(jitem(5000)))
	expect(t, sameItems(j.Pending(), append(append([]CaptureItem{}, keep...), jitem(5000))), "appends go on after a compaction")
	for _, it := range append(append([]CaptureItem{}, keep...), jitem(5000)) {
		must(t, j.Ack(it.ID))
	}
	must(t, j.Compact())
	b, _ := os.ReadFile(jfile(d))
	expect(t, len(j.Pending()) == 0 && len(b) == 0 && fmode(t, jfile(d)) == 0o600, "everything acknowledged: an empty file")
	must(t, j.Append(jitem(1)))
	expect(t, sameItems(j.Pending(), []CaptureItem{jitem(1)}))
	must(t, NewCaptureJournal(jdir(t)).Compact()) // no file at all: not an error
}

func TestJournalDroppingTheWriterWithoutAnyAckLosesNothing(t *testing.T) {
	d := jdir(t)
	j := NewCaptureJournal(d)
	var items []CaptureItem
	for i := 1; i <= 5; i++ {
		items = append(items, jitem(i))
		must(t, j.Append(items[i-1]))
	}
	must(t, j.Ack(items[1].ID))
	must(t, j.Ack(items[3].ID))
	j = nil // the process dies here: no flush, no compact, no goodbye
	expect(t, sameItems(NewCaptureJournal(d).Pending(), []CaptureItem{items[0], items[2], items[4]}), "pending equals exactly the unacknowledged notes")
}

func TestJournalKeepsATimedNoteAndItsList(t *testing.T) {
	j := NewCaptureJournal(t.TempDir())
	timed := jitem(1)
	timed.Timed, timed.List = true, ListPersonal
	must(t, j.Append(timed))
	must(t, j.Append(jitem(2)))
	p := j.Pending()
	expect(t, len(p) == 2 && p[0].Timed && p[0].List == ListPersonal && !p[1].Timed && p[1].List == "", "%+v", p)
	expect(t, !strings.Contains(journalLine(jitem(2)), "timed") && !strings.Contains(journalLine(jitem(2)), "list"), "an ordinary note's line is unchanged")
}
