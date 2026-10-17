// logstore.go - reads/writes the markdown day files in the storage folder (one free-form page per day).
// Port of app/Core/LogStore.swift.
//
// API (all return a *LogError unless noted; test with errors.Is(err, ErrHasContent) etc.):
//
//	NewLogStore(dir, backupDir, minBackupInterval, clock)
//	                                  backupDir = where safety copies go ("" = no backups). The UI passes
//	                                  ~/Library/Application Support/Gloamlog/backups so they never sync with the logs.
//	                                  clock nil = time.Now.
//	EnsureFolder()                    create the folder (onboarding / "Choose folder")
//	CheckFolder(writable)             ErrFolderMissing / ErrFolderNotWritable
//	ListDays() []string               day keys that have a file, newest first (the assets/ folder, a backups folder inside
//	                                  the storage folder and foreign files are ignored)
//	Load(day) *DayDocument            nil, nil = no file. v0.1/v0.2 files load as plain markdown (see markdownformat.go)
//	                                  Fails with ErrNotLocal while the day's page is still in the cloud (iCloud placeholder, dataless
//	                                  file): so do Save/Skip/Unskip/RestoreBackup. IsNotLocal(day) says so without reading.
//	Save(day, body) SaveOutcome       atomic; writes "# day\n\n<body>\n". Replaces a skip marker ("write a log anyway").
//	                                  A whitespace-only body deletes an existing log file instead (an emptied page is "no log")
//	                                  and never touches a skip marker. Identical content is not rewritten. The UI must only
//	                                  call this after a real edit. See Backups.
//	Skip(day, reason)                 writes "> Skipped: reason"; fails with ErrHasContent if the day has writing (headings alone are overwritten)
//	SkipRange(from, to, reason, weekdays, cal) []string   one file per scheduled day; days with writing are left alone
//	Unskip(day) bool                  deletes the marker only
//	FileStates(minWords) map[string]DayStatus   logged/partial/skipped per file; days with 0 words are omitted
//	                                  (input for Status/Streak/Heatmap/WeeklyReview). Follows the Jots rules (SetJotsRules): jots are
//	                                  not words (unless CountTowardLogged), and a day with only jots is partial, so it is in the map.
//	Documents(from, to) / AllDocuments()
//	Search(query, heading) []SearchHit  see search.go
//	FolderSummary()                   counts for Settings > Storage
//	FolderStamp() string              changes when a day file is added, removed, renamed, edited or touched; stat only, no reads
//	Assets() AssetStore               images for this folder
//
// Writes (day files and backups) go through AtomicWrite: a hidden ".gloamlog-tmp-<pid>-<uuid>" file, then rename. A SIGKILL can leave
//
//	at most that one hidden file, which the next store's first write removes (tests/run-kill-tests.sh proves it: 40 killed rounds).
//	SkipDays/UnskipDays (batch skip + undo, catchup.go) and FolderStamp (change detection without reading files) live here and there.
//
// Backups (safety net so autosave can never destroy a day of writing; only when backupDir is set):
//
//	Before Save overwrites OR deletes an existing day file, the old file is copied byte-for-byte to
//	<backupDir>/<yyyy-MM-dd>/<yyyyMMdd-HHmmss>.md (UTC time stamp, atomic write) and only the newest 10 per day are kept
//	(MaxBackupsPerDay). No backup when: the new content equals the file, the file is a skip marker, or the file
//	has nothing but its title. Names always increase (a same-second save or a clock set back still sorts newest), so
//	pruning never drops the latest copy. MinBackupInterval > 0 makes overwrites skip the copy while the
//	newest backup is younger than that, so the 10 slots span longer in a typing session (deletes/restores always copy).
//	A failing backup NEVER fails the save: the write/delete still happens and the problem is reported twice,
//	  Save(...) -> SaveOutcome{BackedUp, BackupWarning}   for that call, and
//	  LastBackupWarning(): string                         latest backup/prune problem, "" after the next good backup.
//	Backups are never listed by ListDays/FileStates/Search/Documents/FolderSummary.
//	ListBackups(day) []BackupInfo     newest first; empty without backupDir (does not fail)
//	RestoreBackup(day, backup) DayDocument   first backs up the current file (fails with ErrBackupFailed if that
//	                                         is impossible and nothing was changed), then writes the old copy back.
//	                                         ErrBackupsDisabled without backupDir; ErrIO for an unknown/forged backup.
//
// Go vs Swift: text files are read like Foundation's String(contentsOf:encoding: .utf8): invalid UTF-8 is an error, and a leading
// UTF-8 byte-order mark is dropped. The "Throws" of Swift is the second return value here.
package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// ---- errors ----

// The kinds of LogError. Match with errors.Is(err, ErrHasContent); the day or path is in LogError.Detail.
var (
	ErrFolderMissing     = errors.New("storage folder is missing")
	ErrFolderNotWritable = errors.New("storage folder is not writable")
	ErrBadDay            = errors.New("not a day key")
	ErrHasContent        = errors.New("the day has writing")    // refusing to overwrite a day that has writing
	ErrUnsupportedAsset  = errors.New("unsupported image type") // extension not in png/jpg/jpeg/gif/webp
	ErrAssetTooLarge     = errors.New("image is too large")     // Detail = bytes; max is AssetMaxBytes (20 MB)
	ErrEmptyAsset        = errors.New("image is empty")
	ErrBadPath           = errors.New("path leaves the storage folder")     // e.g. assets/ is a symlink leaving the storage folder
	ErrBackupsDisabled   = errors.New("backups are off")                    // RestoreBackup on a store created without backupDir
	ErrBackupFailed      = errors.New("could not back up the current page") // RestoreBackup could not make its safety copy (nothing changed)
	ErrNotLocal          = errors.New("the day is still in the cloud")      // never read, changed or replaced until it is here
	ErrIO                = errors.New("file error")
)

// LogError is the error type of this package: a Kind (one of the Err... values) and the day, path or message it is about.
type LogError struct {
	Kind   error
	Detail string
}

func (e *LogError) Error() string {
	if e.Kind == ErrNotLocal { // the only case with a sentence of its own (Swift: CloudFile.swift's LocalizedError)
		return fmt.Sprintf("%s is still downloading from your cloud folder, so it can't be changed yet.", e.Detail)
	}
	if e.Detail == "" {
		return e.Kind.Error()
	}
	return e.Kind.Error() + ": " + e.Detail
}

// Unwrap makes errors.Is(err, ErrHasContent) work.
func (e *LogError) Unwrap() error { return e.Kind }

func logErr(kind error, detail string) error { return &LogError{Kind: kind, Detail: detail} }

// LogErrorDetail is the day, path or message a LogError carries ("" for any other error).
func LogErrorDetail(err error) string {
	var le *LogError
	if errors.As(err, &le) {
		return le.Detail
	}
	return ""
}

// ---- types ----

// FolderSummary is the counts for Settings > Storage.
type FolderSummary struct {
	Logs         int `json:"logs"`
	Skipped      int `json:"skipped"`
	Unrecognized int `json:"unrecognized"`
}

// BackupInfo is one safety copy of a day file. Date is the (UTC) time stamp in its file name; Words counts the saved page.
type BackupInfo struct {
	Date  time.Time `json:"date"`
	Path  string    `json:"path"`
	Words int       `json:"words"`
}

// SaveOutcome is what Save did besides writing: whether the old file was copied, and any non-fatal backup problem to show the user.
type SaveOutcome struct {
	BackedUp      bool   `json:"backedUp"`
	BackupWarning string `json:"backupWarning"` // "" = none
}

const (
	// MaxBackupsPerDay is how many safety copies of one day are kept.
	MaxBackupsPerDay = 10
	backupExt        = ".md"
)

// Backup file names: "yyyyMMdd-HHmmss.md" in UTC, so DST and time zones cannot reorder or duplicate them.
func backupStem(t time.Time) string { return t.UTC().Format("20060102-150405") }

// backupDate parses a backup file name; ok is false for anything that is not exactly that shape and a real date.
func backupDate(name string) (time.Time, bool) {
	if !strings.HasSuffix(name, backupExt) {
		return time.Time{}, false
	}
	stem := strings.TrimSuffix(name, backupExt)
	if len(stem) != 15 || stem[8] != '-' {
		return time.Time{}, false
	}
	for i := 0; i < len(stem); i++ {
		if i != 8 && (stem[i] < '0' || stem[i] > '9') {
			return time.Time{}, false
		}
	}
	t, err := time.Parse("20060102-150405", stem)
	if err != nil || backupStem(t) != stem { // the round trip rejects month 13, 25:00 and friends
		return time.Time{}, false
	}
	return t, true
}

// ---- the store ----

// LogStore reads and writes the day files of one storage folder. Safe to use from several goroutines.
type LogStore struct {
	Dir               string
	BackupDir         string // "" = no backups
	MinBackupInterval time.Duration

	clock       func() time.Time
	mu          sync.Mutex // guards swept and lastBackupWarning
	swept       bool
	lastWarning string
	searchIndex *SearchIndex
}

// NewLogStore makes a store over dir. See the header for backupDir and minBackupInterval; clock nil means time.Now.
func NewLogStore(dir, backupDir string, minBackupInterval time.Duration, clock func() time.Time) *LogStore {
	if clock == nil {
		clock = time.Now
	}
	return &LogStore{Dir: dir, BackupDir: backupDir, MinBackupInterval: minBackupInterval, clock: clock, searchIndex: newSearchIndex()}
}

// Assets is the image store of this folder.
func (s *LogStore) Assets() AssetStore { return AssetStore{Dir: s.Dir} }

// LastBackupWarning is the latest backup/prune problem, "" after the next good backup.
func (s *LogStore) LastBackupWarning() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastWarning
}

// ---- folder ----

// EnsureFolder creates the storage folder if it is missing, then checks it.
func (s *LogStore) EnsureFolder() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return logErr(ErrFolderNotWritable, s.Dir)
	}
	return s.CheckFolder(true)
}

// CheckFolder fails with ErrFolderMissing when the folder is not there and, if writable is true, ErrFolderNotWritable when it cannot be written.
func (s *LogStore) CheckFolder(writable bool) error {
	fi, err := os.Stat(s.Dir)
	if err != nil || !fi.IsDir() {
		return logErr(ErrFolderMissing, s.Dir)
	}
	if writable && !isWritable(s.Dir) {
		return logErr(ErrFolderNotWritable, s.Dir)
	}
	return nil
}

// isWritable is access(2) with W_OK, what FileManager.isWritableFile does.
func isWritable(path string) bool { return syscall.Access(path, 2) == nil }

// Path is the file of a day ("<dir>/2026-10-06.md"). It does not check the day.
func (s *LogStore) Path(day string) string { return filepath.Join(s.Dir, day+".md") }

func (s *LogStore) validate(day string) error {
	if !DayKeyIsWellFormed(day) {
		return logErr(ErrBadDay, day)
	}
	return nil
}

// IsNotLocal is true when the day's page is in the folder but not (fully) on this Mac yet: an iCloud placeholder or a dataless file.
// Load, Save, Skip, Unskip and RestoreBackup refuse such a day with ErrNotLocal, so a day that is merely still downloading is never
// read as "no log" and never gets a page created over it.
func (s *LogStore) IsNotLocal(day string) bool { return dayIsNotLocal(s.Dir, day) }

func (s *LogStore) requireLocal(day string) error {
	if s.IsNotLocal(day) {
		return logErr(ErrNotLocal, day)
	}
	return nil
}

// loadIfLocal is Load for loops over many days: a day that is not here yet is left out instead of failing the whole call.
func (s *LogStore) loadIfLocal(day string) (*DayDocument, error) {
	doc, err := s.Load(day)
	if errors.Is(err, ErrNotLocal) {
		return nil, nil
	}
	return doc, err
}

// readText reads a text file like String(contentsOf:encoding: .utf8): invalid UTF-8 is an error, a leading BOM is dropped.
func readText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return decodeText(data)
}

func decodeText(data []byte) (string, error) {
	if !utf8.Valid(data) {
		return "", errors.New("the file is not valid UTF-8")
	}
	return strings.TrimPrefix(string(data), utf8BOM), nil
}

// ---- read ----

// dayFiles lists the day keys (newest first) and counts the files that are not days (the "unrecognised" ones).
func (s *LogStore) dayFiles() (days []string, other int, err error) {
	if err = s.CheckFolder(false); err != nil {
		return nil, 0, err
	}
	entries, rerr := os.ReadDir(s.Dir)
	if rerr != nil {
		return nil, 0, logErr(ErrIO, rerr.Error())
	}
	reserved := map[string]bool{AssetFolderName: true}
	if s.BackupDir != "" && filepath.Clean(filepath.Dir(s.BackupDir)) == filepath.Clean(s.Dir) {
		reserved[filepath.Base(s.BackupDir)] = true // a backups folder inside the storage folder is not "unrecognised"
	}
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") || reserved[n] {
			continue
		}
		if strings.HasSuffix(n, ".md") && DayKeyIsWellFormed(strings.TrimSuffix(n, ".md")) {
			days = append(days, strings.TrimSuffix(n, ".md"))
		} else {
			other++
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days, other, nil
}

// ListDays lists the day keys that have a file, newest first.
func (s *LogStore) ListDays() ([]string, error) {
	days, _, err := s.dayFiles()
	return days, err
}

// Load reads one day. nil, nil means there is no file for that day.
func (s *LogStore) Load(day string) (*DayDocument, error) {
	if err := s.validate(day); err != nil {
		return nil, err
	}
	if err := s.CheckFolder(false); err != nil {
		return nil, err
	}
	if err := s.requireLocal(day); err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.Path(day)); err != nil {
		return nil, nil
	}
	text, err := readText(s.Path(day))
	if err != nil {
		return nil, logErr(ErrIO, err.Error())
	}
	doc := ParseDay(day, text)
	return &doc, nil
}

// FileStates is the status of every day that has a file; days that count as "no log" (status missed) are left out.
func (s *LogStore) FileStates(minWords int) (map[string]DayStatus, error) {
	days, err := s.ListDays()
	if err != nil {
		return nil, err
	}
	out := map[string]DayStatus{}
	for _, d := range days {
		doc, err := s.Load(d)
		if err != nil || doc == nil {
			continue
		}
		if st := doc.Status(minWords); st != StatusMissed {
			out[d] = st
		}
	}
	return out, nil
}

// Documents loads the days from...to (inclusive) that have a file.
func (s *LogStore) Documents(from, to string) (map[string]DayDocument, error) {
	days, err := s.ListDays()
	if err != nil {
		return nil, err
	}
	out := map[string]DayDocument{}
	for _, d := range days {
		if d < from || d > to {
			continue
		}
		doc, err := s.loadIfLocal(d)
		if err != nil {
			return nil, err
		}
		if doc != nil {
			out[d] = *doc
		}
	}
	return out, nil
}

// AllDocuments loads every day, newest first.
func (s *LogStore) AllDocuments() ([]DayDocument, error) {
	days, err := s.ListDays()
	if err != nil {
		return nil, err
	}
	var out []DayDocument
	for _, d := range days {
		doc, err := s.loadIfLocal(d)
		if err != nil {
			return nil, err
		}
		if doc != nil {
			out = append(out, *doc)
		}
	}
	return out, nil
}

// FolderSummary counts the logs, skip markers and unrecognised files in the folder.
func (s *LogStore) FolderSummary() (FolderSummary, error) {
	var sum FolderSummary
	days, other, err := s.dayFiles()
	if err != nil {
		return sum, err
	}
	sum.Unrecognized = other
	for _, d := range days {
		doc, err := s.loadIfLocal(d)
		if err != nil {
			return sum, err
		}
		if doc == nil {
			continue
		}
		if doc.IsSkipped {
			sum.Skipped++
		} else if doc.HasContent() {
			sum.Logs++
		}
	}
	return sum, nil
}

// FolderStamp is a string that changes whenever a day file is added, removed, renamed, edited or touched: the sorted (name, mtime in
// ns, size) of the YYYY-MM-DD.md files, one per line. Only readdir and stat are called, no file is opened or read (3,650 files take a
// few ms), so the UI can poll it and reload only when it changed. assets/, a backups folder, hidden, foreign files and folders are
// ignored; a symlinked day file counts through its target. A missing folder fails like the other reads.
func (s *LogStore) FolderStamp() (string, error) {
	if err := s.CheckFolder(false); err != nil {
		return "", err
	}
	d, err := os.Open(s.Dir)
	if err != nil {
		return "", logErr(ErrIO, err.Error())
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return "", logErr(ErrIO, err.Error())
	}
	// Like the Swift fstatat(dirfd, name): stat relative to the open folder, which skips walking the whole path 3,650 times. A day
	// file that is a symlink leaving the folder is refused by os.Root, so that one case falls back to a plain os.Stat.
	root, rootErr := os.OpenRoot(s.Dir)
	if rootErr == nil {
		defer root.Close()
	}
	stat := func(name string) (os.FileInfo, error) {
		if rootErr == nil {
			if fi, err := root.Stat(name); err == nil {
				return fi, nil
			}
		}
		return os.Stat(filepath.Join(s.Dir, name))
	}
	type row struct {
		key  int
		line string
	}
	var rows []row
	for _, name := range names {
		key, ok := dayNumberOfFileName(name)
		if !ok { // not YYYY-MM-DD.md: never even stat'ed
			continue
		}
		fi, err := stat(name)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		// a dataless file (its bytes arrive later from the cloud) gets a marker, so the arrival changes the stamp
		cloud := ""
		if fileInfoIsDataless(fi) {
			cloud = " dataless"
		}
		rows = append(rows, row{key, fmt.Sprintf("%s %d.%d %d%s", name, fi.ModTime().Unix(), fi.ModTime().Nanosecond(), fi.Size(), cloud)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = r.line
	}
	return strings.Join(lines, "\n"), nil
}

// dayNumberOfFileName is 20261005 for "2026-10-05.md" (digits at the date positions, nothing before or after); ok is false for any other name.
func dayNumberOfFileName(name string) (int, bool) {
	if len(name) != 13 || name[4] != '-' || name[7] != '-' || name[10:] != ".md" {
		return 0, false
	}
	n := 0
	for _, i := range [...]int{0, 1, 2, 3, 5, 6, 8, 9} {
		digit := int(name[i]) - '0'
		if digit < 0 || digit > 9 {
			return 0, false
		}
		n = n*10 + digit
	}
	return n, true
}

// ---- write ----

// sweepOnce removes the temp files a killed writer left behind (AtomicSweep), in the log folder and the backups folder, at the first
// write or backup of a store. Reading never sweeps. Once per store, i.e. once per launch.
func (s *LogStore) sweepOnce() {
	s.mu.Lock()
	already := s.swept
	s.swept = true
	s.mu.Unlock()
	if already {
		return
	}
	AtomicSweep(s.Dir)
	if s.BackupDir != "" {
		AtomicSweep(s.BackupDir)
	}
}

func (s *LogStore) write(text, day string) error {
	if err := s.CheckFolder(true); err != nil {
		return err
	}
	if err := s.requireLocal(day); err != nil {
		return err
	}
	s.sweepOnce()
	if err := AtomicWrite([]byte(text), s.Path(day), ""); err != nil {
		if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
			return logErr(ErrFolderNotWritable, s.Dir)
		}
		return logErr(ErrIO, err.Error())
	}
	return nil
}

// fileSnapshot is what Save read of the file it is about to replace. Reading can fail without failing the save (the backup then
// reports a warning), so data and text each say whether they are valid.
type fileSnapshot struct {
	haveData bool
	data     []byte
	haveText bool
	text     string
}

func readSnapshot(path string) fileSnapshot {
	var snap fileSnapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return snap
	}
	snap.haveData, snap.data = true, data
	if text, err := decodeText(data); err == nil {
		snap.haveText, snap.text = true, text
	}
	return snap
}

// Save writes the page of a day (see the header). The outcome says whether the old file was backed up first.
func (s *LogStore) Save(day, body string) (SaveOutcome, error) {
	var out SaveOutcome
	if err := s.validate(day); err != nil {
		return out, err
	}
	if err := s.CheckFolder(true); err != nil {
		return out, err
	}
	if err := s.requireLocal(day); err != nil { // before anything is read, backed up, written or deleted
		return out, err
	}
	file := s.Path(day)
	_, statErr := os.Stat(file)
	exists := statErr == nil
	var old fileSnapshot
	if exists {
		old = readSnapshot(file)
	}
	if TrimBody(body) == "" {
		if !exists {
			return out, nil
		}
		if old.haveText && ParseDay(day, old.text).IsSkipped {
			return out, nil
		}
		s.note(s.backUpCurrent(day, old, false), &out)
		if err := os.Remove(file); err != nil {
			return out, logErr(ErrIO, err.Error())
		}
		return out, nil
	}
	text := SerializeDay(day, body)
	if old.haveText && old.text == text { // identical: nothing to write, nothing to back up
		return out, nil
	}
	if exists {
		s.note(s.backUpCurrent(day, old, true), &out)
	}
	return out, s.write(text, day)
}

// ---- backups ----

type backupKind int

const (
	backupNotNeeded backupKind = iota
	backupSaved
	backupFailed
)

// backupResult: for backupSaved, msg is a warning about pruning ("" = none); for backupFailed it is the problem.
type backupResult struct {
	kind backupKind
	msg  string
}

func (s *LogStore) note(r backupResult, out *SaveOutcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.kind {
	case backupSaved:
		out.BackedUp = true
		out.BackupWarning = r.msg
		s.lastWarning = r.msg
	case backupFailed:
		out.BackupWarning = r.msg
		s.lastWarning = r.msg
	}
}

func (s *LogStore) backupDayDir(day string) (string, bool) {
	if s.BackupDir == "" || !DayKeyIsWellFormed(day) {
		return "", false
	}
	return filepath.Join(s.BackupDir, day), true
}

// backupNames lists the valid backup file names in a day folder, newest first (names sort chronologically).
func backupNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if _, ok := backupDate(e.Name()); ok {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

// backUpCurrent copies the current day file (already read by the caller) into the backup folder, then prunes. Never fails.
func (s *LogStore) backUpCurrent(day string, cur fileSnapshot, throttled bool) backupResult {
	dayDir, ok := s.backupDayDir(day)
	if !ok {
		return backupResult{}
	}
	s.sweepOnce()
	if cur.haveText {
		page := ParseDay(day, cur.text)
		if page.IsSkipped || TrimBody(page.Body) == "" {
			return backupResult{} // nothing worth keeping
		}
	}
	if !cur.haveData {
		return backupResult{backupFailed, fmt.Sprintf("Could not read %s.md to back it up.", day)}
	}
	fail := func(err error) backupResult {
		return backupResult{backupFailed, fmt.Sprintf("Could not write a backup of %s.md: %v", day, err)}
	}
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		return fail(err)
	}
	stamp := s.clock()
	if names := backupNames(dayDir); len(names) > 0 {
		if newest, ok := backupDate(names[0]); ok {
			if throttled && s.MinBackupInterval > 0 && stamp.Sub(newest) < s.MinBackupInterval {
				return backupResult{}
			}
			if backupStem(stamp) <= backupStem(newest) {
				stamp = newest.Add(time.Second) // keep names increasing
			}
		}
	}
	if err := AtomicWrite(cur.data, filepath.Join(dayDir, backupStem(stamp)+backupExt), s.BackupDir); err != nil {
		return fail(err)
	}
	warning := ""
	names := backupNames(dayDir)
	for i := MaxBackupsPerDay; i < len(names); i++ {
		if err := os.Remove(filepath.Join(dayDir, names[i])); err != nil {
			warning = fmt.Sprintf("Could not remove old backups of %s.md: %v", day, err)
		}
	}
	return backupResult{backupSaved, warning}
}

// ListBackups lists the safety copies of one day, newest first. Empty when backups are off, the day is malformed or nothing was saved yet.
func (s *LogStore) ListBackups(day string) []BackupInfo {
	dayDir, ok := s.backupDayDir(day)
	if !ok {
		return nil
	}
	var out []BackupInfo
	for _, name := range backupNames(dayDir) {
		date, ok := backupDate(name)
		if !ok {
			continue
		}
		path := filepath.Join(dayDir, name)
		// the size of the version, jots included (the "logged" rule is not what a restore list is about)
		words := 0
		if raw, err := readText(path); err == nil {
			if page := ParseDay(day, raw); !page.IsSkipped {
				words = CountWords(page.Body)
			}
		}
		out = append(out, BackupInfo{Date: date, Path: path, Words: words})
	}
	return out
}

// RestoreBackup writes a backup back as the day file. The current file is backed up first (so a restore can itself be undone);
// if that is impossible nothing is changed and ErrBackupFailed is returned. Only the file NAME of backup.Path is used,
// resolved inside this store's backup folder for day.
func (s *LogStore) RestoreBackup(day string, backup BackupInfo) (DayDocument, error) {
	if err := s.validate(day); err != nil {
		return DayDocument{}, err
	}
	dayDir, ok := s.backupDayDir(day)
	if !ok {
		return DayDocument{}, logErr(ErrBackupsDisabled, "")
	}
	name := filepath.Base(backup.Path)
	if _, ok := backupDate(name); !ok {
		return DayDocument{}, logErr(ErrIO, "Not a backup file: "+name)
	}
	text, err := readText(filepath.Join(dayDir, name))
	if err != nil {
		return DayDocument{}, logErr(ErrIO, fmt.Sprintf("Backup %s could not be read.", name))
	}
	if err := s.CheckFolder(true); err != nil {
		return DayDocument{}, err
	}
	if err := s.requireLocal(day); err != nil {
		return DayDocument{}, err
	}
	file := s.Path(day)
	_, statErr := os.Stat(file)
	exists := statErr == nil
	var cur fileSnapshot
	if exists {
		cur = readSnapshot(file)
	}
	if !cur.haveText || cur.text != text {
		if exists {
			r := s.backUpCurrent(day, cur, false)
			s.mu.Lock()
			switch r.kind {
			case backupFailed:
				s.lastWarning = r.msg
				s.mu.Unlock()
				return DayDocument{}, logErr(ErrBackupFailed, r.msg)
			case backupSaved:
				s.lastWarning = r.msg
			}
			s.mu.Unlock()
		}
		if err := s.write(text, day); err != nil {
			return DayDocument{}, err
		}
	}
	return ParseDay(day, text), nil
}

// ---- skip ----

// Skip writes a skip marker for the day. A day with writing is refused (ErrHasContent); a page with only headings is replaced.
func (s *LogStore) Skip(day, reason string) error {
	if err := s.validate(day); err != nil {
		return err
	}
	e, err := s.Load(day)
	if err != nil {
		return err
	}
	if e != nil && !e.IsSkipped && e.HasContent() {
		return logErr(ErrHasContent, day)
	}
	return s.write(SerializeSkip(day, reason), day)
}

// SkipRange skips every scheduled day (per weekdays) in from...to inclusive. Returns the days written; days with writing are left alone.
func (s *LogStore) SkipRange(from, to, reason string, weekdays WeekdaySet, cal Calendar) ([]string, error) {
	if err := s.validate(from); err != nil {
		return nil, err
	}
	if err := s.validate(to); err != nil {
		return nil, err
	}
	var written []string
	d := from
	for guard := 0; d <= to && guard < 3700; guard++ {
		if weekdays.Has(DayKeyWeekday(d, cal.location())) {
			if err := s.Skip(d, reason); err == nil {
				written = append(written, d)
			} else if !errors.Is(err, ErrHasContent) {
				return written, err
			}
		}
		next, ok := DayKeyAdding(d, 1, cal.location())
		if !ok {
			break
		}
		d = next
	}
	return written, nil
}

// Unskip deletes the skip marker of a day. It reports whether there was one; a log is never deleted.
func (s *LogStore) Unskip(day string) (bool, error) {
	if err := s.validate(day); err != nil {
		return false, err
	}
	e, err := s.Load(day)
	if err != nil {
		return false, err
	}
	if e == nil || !e.IsSkipped {
		return false, nil
	}
	if err := os.Remove(s.Path(day)); err != nil {
		return false, logErr(ErrIO, err.Error())
	}
	return true, nil
}

// ---- search (search.go). Pages are parsed once per run and re-read only when their file changed: searchindex.go. ----

// Search finds the pages that match every word of query, one hit per heading-delimited section, newest day first. heading, when not
// nil, restricts the hits to sections with that (normalised) heading. A day that is not on this Mac yet is left out.
func (s *LogStore) Search(query string, heading *string) ([]SearchHit, error) {
	pages, err := s.searchIndex.pages(s, s.loadIfLocal)
	if err != nil {
		return nil, err
	}
	var out []SearchHit
	for _, p := range pages {
		out = append(out, searchHits(p.day, p.sections, query, heading)...)
	}
	return out, nil
}

// WarmSearch fills the search cache ahead of the first search; the app calls it off the main thread after launch and when the folder changes.
func (s *LogStore) WarmSearch() error {
	_, err := s.searchIndex.pages(s, s.loadIfLocal)
	return err
}

// itoa is strconv.Itoa; asset errors carry the byte count as text.
func itoa(n int) string { return strconv.Itoa(n) }
