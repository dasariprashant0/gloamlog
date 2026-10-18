// capturejournal.go - write-ahead journal: a note is durable here (fsynced) before anything else happens.
// Port of app/Core/Capture/CaptureJournal.swift.
//
//	File: <dir>/journal.jsonl, mode 0600, one JSON object per line: a note record, or {"ack":"<id>"}.
//	Append   open(O_RDWR|O_APPEND|O_CREAT, 0600), write ONE line, fsync (F_FULLFSYNC on macOS): the durability point. Errors: *JournalError.
//	Ack      appends {"ack": id}.
//	Pending  notes without an ack, in order; tolerant of a torn last line (a crash mid-write) and of unknown/garbled lines.
//	Compact  rewrites the file without acked notes (atomically); call at launch.
//
// Details:
//   - A record is {"day","id","source","stamp"?,"text","todo","ts" (ISO 8601, whole milliseconds, UTC),"v":1} (keys sorted, "/" not
//     escaped, as Swift's JSONEncoder wrote it). Reading is lenient: "v" and "todo" may be missing, "ts" may lack the milliseconds or be
//     a number (epoch seconds), unknown keys are ignored, a line that is not an object, has no usable id/day/text/ts or a malformed
//     day is skipped (a day like "../x" can never reach a file name). The same id twice is one note (the first wins). An ack is any
//     object whose "ack" is a string, wherever it sits in the file (acks are not order dependent).
//   - A torn last line (power cut mid-write) is ignored by Pending. The next append never glues onto it: when the file does not end in
//     a newline the new record starts on a fresh line, so a fragment can only ever cost the one record that was being written.
//   - A write that fails half way is cut back to the size the file had before it, so a failed append leaves no fragment either.
//   - The first record of a new file also syncs the folder entry. The folder is created on demand (0700). The file is created 0600 and
//     tightened to 0600 if it existed wider.
//   - Compact writes through AtomicWrite (old complete content or new complete content, never a mix), keeps the raw lines of the pending
//     notes in order, and does nothing when the file would not change. A missing file is not an error.
//
// Not safe for two journals on one file (two processes): the Swift app had the same limit.
package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// JournalError is what the journal's writers return: an I/O problem, with the reason.
type JournalError struct{ Msg string }

func (e *JournalError) Error() string { return e.Msg }

// CaptureJournal is the journal in Dir. Use one value per folder.
type CaptureJournal struct{ Dir string }

// NewCaptureJournal makes a journal in dir (created on the first write).
func NewCaptureJournal(dir string) *CaptureJournal { return &CaptureJournal{Dir: dir} }

// DefaultCaptureDir is <home>/Library/Application Support/Gloamlog/capture.
func DefaultCaptureDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Gloamlog", "capture")
}

// Path is the journal file.
func (j *CaptureJournal) Path() string { return filepath.Join(j.Dir, "journal.jsonl") }

// ---- write ----

// Append makes the note durable: the line is written and flushed to the disk when it returns nil.
func (j *CaptureJournal) Append(item CaptureItem) error {
	return j.writeLine([]byte(journalLine(item)))
}

// Ack marks a note as delivered.
func (j *CaptureJournal) Ack(id string) error { return j.writeLine([]byte(journalAckLine(id))) }

// syncFile is F_FULLFSYNC where the platform has it (Go's File.Sync does that on macOS), plain fsync when that is unsupported.
func syncFile(f *os.File) error {
	if err := f.Sync(); err != nil {
		return syscall.Fsync(int(f.Fd()))
	}
	return nil
}

func (j *CaptureJournal) writeLine(line []byte) error {
	if fi, err := os.Stat(j.Dir); err != nil || !fi.IsDir() {
		if err := os.MkdirAll(j.Dir, 0o700); err != nil {
			return &JournalError{fmt.Sprintf("cannot create %s: %v", j.Dir, err)}
		}
	}
	path := j.Path()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return &JournalError{fmt.Sprintf("cannot open %s: %v", path, err)}
	}
	defer f.Close()

	var sizeBefore int64
	var data []byte
	if fi, err := f.Stat(); err == nil {
		sizeBefore = fi.Size()
		if fi.Mode().Perm() != 0o600 {
			_ = f.Chmod(0o600) // best effort: tighten an old file's mode; the note still gets written
		}
		if sizeBefore > 0 { // a torn last line: start this record on a line of its own
			var last [1]byte
			if n, _ := f.ReadAt(last[:], sizeBefore-1); n == 1 && last[0] != '\n' {
				data = append(data, '\n')
			}
		}
	}
	data = append(data, line...)
	data = append(data, '\n')

	if _, err := f.Write(data); err != nil {
		_ = f.Truncate(sizeBefore) // best effort: do not leave half a line behind
		return &JournalError{fmt.Sprintf("cannot write %s: %v", path, err)}
	}
	if err := syncFile(f); err != nil {
		return &JournalError{fmt.Sprintf("cannot flush %s: %v", path, err)}
	}
	if sizeBefore == 0 { // a new file: make its directory entry durable too
		if d, err := os.Open(j.Dir); err == nil {
			_ = syncFile(d) // best effort: the record itself is already fsynced
			d.Close()
		}
	}
	return nil
}

// ---- read ----

// Pending are the notes without an ack, in the order they were written. Never fails: an unreadable or missing file has nothing pending.
func (j *CaptureJournal) Pending() []CaptureItem {
	data, err := os.ReadFile(j.Path())
	if err != nil {
		return nil // no journal (or unreadable): nothing pending; Append reports a real write failure
	}
	live, acked := scanJournal(data)
	var out []CaptureItem
	for _, e := range live {
		if !acked[e.item.ID] {
			out = append(out, e.item)
		}
	}
	return out
}

// Compact rewrites the file with the raw lines of the pending notes only (acknowledged notes, acks, garbage and a torn last line are dropped).
func (j *CaptureJournal) Compact() error {
	if _, err := os.Stat(j.Path()); err != nil {
		return nil
	}
	AtomicSweep(j.Dir)
	data, err := os.ReadFile(j.Path())
	if err != nil {
		return &JournalError{fmt.Sprintf("cannot read %s: %v", j.Path(), err)}
	}
	live, acked := scanJournal(data)
	out := []byte{}
	for _, e := range live {
		if !acked[e.item.ID] {
			out = append(out, e.raw...)
			out = append(out, '\n')
		}
	}
	if bytes.Equal(out, data) {
		return nil
	}
	if err := AtomicWrite(out, j.Path(), ""); err != nil {
		return &JournalError{fmt.Sprintf("cannot compact %s: %v", j.Path(), err)}
	}
	return nil
}

// ---- records ----

// jsonString writes s as a JSON string the way Swift's JSONEncoder does: only the quote, the backslash and control characters are
// escaped ("/", "<", ">", "&" and U+2028/9 are not), which Go's encoding/json would escape.
func jsonString(s string) string {
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// journalLine is the journal line (without the newline) for a note. Tests build journals of any size with it, without 2,000 fsyncs.
func journalLine(item CaptureItem) string {
	var b strings.Builder
	b.WriteString(`{"day":` + jsonString(item.Day) + `,"id":` + jsonString(item.ID) + `,"source":` + jsonString(item.Source))
	if item.Stamp != "" {
		b.WriteString(`,"stamp":` + jsonString(item.Stamp))
	}
	b.WriteString(`,"text":` + jsonString(item.Text) + `,"todo":`)
	if item.Todo {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString(`,"ts":` + jsonString(journalISO(item.Ts)) + `,"v":1}`)
	return b.String()
}

func journalAckLine(id string) string { return `{"ack":` + jsonString(id) + `}` }

// journalISO is "2026-10-07T18:32:01.250Z": whole milliseconds, rounded, formatted by hand so rounding can never move a note by a
// millisecond.
func journalISO(t time.Time) string {
	ms := t.Unix()*1000 + int64(t.Nanosecond()+500_000)/1_000_000
	secs := ms / 1000
	if ms < 0 && ms%1000 != 0 {
		secs-- // floor, like Swift's hand-rolled division
	}
	return time.Unix(secs, 0).UTC().Format("2006-01-02T15:04:05") + fmt.Sprintf(".%03dZ", ms-secs*1000)
}

func journalDate(v any) (time.Time, bool) {
	switch x := v.(type) {
	case string:
		t, err := time.Parse(time.RFC3339Nano, x) // with or without fractional seconds
		return t, err == nil
	case float64:
		sec := int64(x)
		return time.Unix(sec, int64((x-float64(sec))*1e9)), true
	}
	return time.Time{}, false
}

type journalEntry struct {
	item   CaptureItem
	ackID  string
	isAck  bool
	isItem bool
}

func parseJournalLine(line []byte) journalEntry {
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil || obj == nil {
		return journalEntry{}
	}
	if id, ok := obj["ack"].(string); ok {
		return journalEntry{ackID: id, isAck: true}
	}
	id, _ := obj["id"].(string)
	day, _ := obj["day"].(string)
	text, _ := obj["text"].(string)
	ts, tsOK := journalDate(obj["ts"])
	if id == "" || !DayKeyIsWellFormed(day) || text == "" || !tsOK {
		return journalEntry{}
	}
	stamp, _ := obj["stamp"].(string)
	todo, _ := obj["todo"].(bool)
	source, ok := obj["source"].(string)
	if !ok {
		source = "unknown"
	}
	return journalEntry{isItem: true, item: CaptureItem{ID: id, Ts: ts, Day: day, Text: text, Stamp: stamp, Todo: todo, Source: source}}
}

type journalLive struct {
	item CaptureItem
	raw  []byte
}

// scanJournal returns every readable note (first occurrence of an id wins) with its raw line, and every acknowledged id.
func scanJournal(data []byte) (live []journalLive, acked map[string]bool) {
	acked = map[string]bool{}
	seen := map[string]bool{}
	for _, raw := range bytes.Split(data, []byte{'\n'}) {
		if len(raw) == 0 {
			continue
		}
		e := parseJournalLine(raw)
		switch {
		case e.isAck:
			acked[e.ackID] = true
		case e.isItem && !seen[e.item.ID]:
			seen[e.item.ID] = true
			live = append(live, journalLive{item: e.item, raw: append([]byte(nil), raw...)})
		}
	}
	return live, acked
}
