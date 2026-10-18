package main

// Sync across Macs (the Swift SyncController, minus the window): Gloamlog runs no sync, the folder provider (iCloud Drive, Dropbox, ...)
// moves the files. This watches the folder, keeps count of what is still coming from the cloud and which conflict copies are here, and
// settles a conflict without ever losing a line. The rules live in internal/core (syncscan, syncresolve, conflictfiles, folderwatcher).

import (
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// requestDownload asks the provider for one file's bytes. A variable so the tests do not touch iCloud.
var requestDownload = core.RequestDownload

// syncService is the per-Notes sync state. Notes has no field for it (it lives in notes.go), so it is looked up by *Notes.
type syncService struct {
	mu sync.Mutex // one scan, settle or resolve at a time; taken before Notes.mu, never inside it

	kindDir string
	kind    core.FolderKind

	dl        sync.Mutex // guards requested and inFlight
	requested map[string]time.Time
	inFlight  map[string]bool

	last string // the state as the UI last saw it, to tell the 30 s tick whether to speak up
}

var syncServices sync.Map // *Notes -> *syncService

func (n *Notes) syncSvc() *syncService {
	v, _ := syncServices.LoadOrStore(n, &syncService{requested: map[string]time.Time{}, inFlight: map[string]bool{}})
	return v.(*syncService)
}

// SyncStatus is what the sidebar chip and Storage settings show. Quiet by design: nothing to say when both counts are 0.
type SyncStatus struct {
	Kind         string   `json:"kind"`      // "local", "icloud", "cloud" or "network"
	KindLabel    string   `json:"kindLabel"` // "Local", "iCloud Drive", "Dropbox", "Network or external drive"
	Source       string   `json:"source"`    // "iCloud", the provider's name, or "your cloud folder": "Downloading from iCloud…"
	Conflicts    int      `json:"conflicts"` // conflict copies to review
	ConflictDays []string `json:"conflictDays"`
	PendingDays  []string `json:"pendingDays"` // days with something still on its way, oldest first
	BlockedDays  []string `json:"blockedDays"` // days with no local page at all: not written to, not "unlogged"
}

func (s SyncStatus) key() string {
	k := s.Kind
	for _, group := range [][]string{s.ConflictDays, s.PendingDays, s.BlockedDays} {
		k += "|"
		for _, d := range group {
			k += d + ","
		}
	}
	return k
}

func sortedKeys(m map[string]bool) []string {
	out := []string{}
	for d := range m {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// refresh looks at the folder once: settles identical conflict copies, asks for what is still in the cloud (once every 2 minutes per
// file), and returns the scan. The caller holds svc.mu.
func (n *Notes) refreshLocked(svc *syncService) (core.SyncScan, core.FolderKind, *core.LogStore) {
	store := n.currentStore()
	if svc.kindDir != store.Dir || svc.kind.Kind == "" {
		svc.kindDir, svc.kind = store.Dir, core.DetectFolderKind(store.Dir)
		svc.dl.Lock()
		svc.requested = map[string]time.Time{}
		svc.dl.Unlock()
	}
	scan := core.SettleIdenticalConflicts(store, core.ScanSyncFolder(store.Dir), n.now())
	svc.requestDownloads(store.Dir, svc.kind, scan.Remote, false)
	return scan, svc.kind, store
}

func (n *Notes) currentStore() *core.LogStore {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store
}

// requestDownloads asks the provider for every file that is not here yet (once every 2 minutes per file unless force), each on its own
// goroutine: for providers other than iCloud the request is a read that blocks until the bytes are here.
func (svc *syncService) requestDownloads(dir string, kind core.FolderKind, remote []core.RemoteFile, force bool) {
	svc.dl.Lock()
	defer svc.dl.Unlock()
	now := time.Now()
	for _, r := range remote {
		if svc.inFlight[r.Name] {
			continue
		}
		if at, ok := svc.requested[r.Name]; ok && !force && now.Sub(at) <= 2*time.Minute {
			continue
		}
		svc.requested[r.Name] = now
		svc.inFlight[r.Name] = true
		path, name := filepath.Join(dir, r.Name), r.Name
		go func() {
			requestDownload(path, kind.IsICloud())
			svc.dl.Lock()
			delete(svc.inFlight, name)
			svc.dl.Unlock()
		}()
	}
}

func statusOf(scan core.SyncScan, kind core.FolderKind) SyncStatus {
	st := SyncStatus{Kind: kind.Kind, KindLabel: kind.Label(), Source: kind.SourceName(), Conflicts: len(scan.Conflicts),
		ConflictDays: []string{}, PendingDays: scan.PendingDays(), BlockedDays: sortedKeys(scan.BlockedDays())}
	for _, c := range scan.Conflicts {
		if len(st.ConflictDays) == 0 || st.ConflictDays[len(st.ConflictDays)-1] != c.Day {
			st.ConflictDays = append(st.ConflictDays, c.Day) // the scan is sorted by day
		}
	}
	return st
}

// SyncState looks at the folder now and says what is waiting: conflict copies to review and days still downloading.
func (n *Notes) SyncState() SyncStatus {
	svc := n.syncSvc()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	scan, kind, _ := n.refreshLocked(svc)
	st := statusOf(scan, kind)
	svc.last = st.key()
	return st
}

// ConflictSide is one version of a conflicted day, for the side-by-side view.
type ConflictSide struct {
	Title string            `json:"title"` // "This Mac, Thu 8 Oct, 4:30 pm, 12 words"
	Lines []core.MarkedLine `json:"lines"` // every line, and whether the other version lacks it
}

// ConflictItem is one conflict copy with both versions. When a side cannot be read yet (Error is set) Mine and Theirs are null.
type ConflictItem struct {
	Name          string        `json:"name"`
	Day           string        `json:"day"`
	Family        string        `json:"family"`
	Error         string        `json:"error"` // "" = both versions were read
	Mine          *ConflictSide `json:"mine"`
	Theirs        *ConflictSide `json:"theirs"`
	MineExists    bool          `json:"mineExists"`    // this Mac has a page for the day: Keep mine makes sense
	TheirsHasText bool          `json:"theirsHasText"` // the other copy has text: Take theirs makes sense
}

func sideTitle(who string, at time.Time, words int) string {
	when := "time unknown"
	if !at.IsZero() {
		at = at.In(time.Local)
		when = at.Format("Mon 2 Jan") + ", " + at.Format("3:04 pm")
	}
	count := strconv.Itoa(words) + " words"
	if words == 1 {
		count = "1 word"
	}
	return who + ", " + when + ", " + count
}

func conflictItem(store *core.LogStore, c core.ConflictCopy) ConflictItem {
	item := ConflictItem{Name: c.Name, Day: c.Day, Family: string(c.Family)}
	sides, err := core.ReadConflictSides(store, c)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	mine, theirs := sides.MineBody(), sides.Theirs.Body
	item.MineExists = sides.Mine != nil
	item.TheirsHasText = core.HasContent(theirs)
	mineWho := "This Mac"
	switch {
	case sides.Mine == nil:
		mineWho = "This Mac, no page"
	case sides.Mine.IsSkipped:
		mineWho = "This Mac, skipped"
	}
	mineTitle := mineWho
	if sides.Mine != nil {
		mineTitle = sideTitle(mineWho, sides.MineDate, core.CountWords(mine))
	}
	theirWho := "Other copy"
	if sides.Theirs.IsSkipped {
		theirWho = "Other copy, skipped"
	}
	item.Mine = &ConflictSide{Title: mineTitle, Lines: core.MarkLines(mine, theirs)}
	item.Theirs = &ConflictSide{Title: sideTitle(theirWho, sides.TheirsDate, core.CountWords(theirs)), Lines: core.MarkLines(theirs, mine)}
	return item
}

// SyncConflicts is the conflict copies to review, by day, each with both versions read from disk now. The UI saves the open page
// first, so "this Mac" includes what was typed a moment ago.
func (n *Notes) SyncConflicts() []ConflictItem {
	svc := n.syncSvc()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	scan, _, store := n.refreshLocked(svc)
	items := []ConflictItem{}
	for _, c := range scan.Conflicts {
		items = append(items, conflictItem(store, c))
	}
	return items
}

// ResolveConflict settles one conflict copy: choice is "keep-mine", "keep-theirs" or "keep-both". The other copy is kept under
// backups/conflicts first and never deleted; the page's old text goes into its version history. A failure leaves everything as it was
// and its message is the sentence to show. The UI saves the open page before the call, refuses while it has unsaved edits, and reloads
// the day afterwards.
func (n *Notes) ResolveConflict(name, choice string) error {
	svc := n.syncSvc()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	scan, _, store := n.refreshLocked(svc)
	var c *core.ConflictCopy
	for i := range scan.Conflicts { // by name from a fresh scan, so nothing but a real conflict copy of this folder is ever touched
		if scan.Conflicts[i].Name == name {
			c = &scan.Conflicts[i]
		}
	}
	if c == nil {
		return errors.New("This conflict is already settled.")
	}
	n.mu.Lock() // saves of the store are serialised, ResolveConflict changes MinBackupInterval for its one save
	defer n.mu.Unlock()
	return core.ResolveConflict(store, *c, core.ConflictChoice(choice), n.now())
}

// DownloadDay asks the provider for one day's page now ("Try again" on the still-downloading page). It returns at once.
func (n *Notes) DownloadDay(day string) error {
	if !core.DayKeyIsWellFormed(day) {
		return errors.New("not a day: " + day)
	}
	svc := n.syncSvc()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	_, kind, store := n.refreshLocked(svc)
	svc.requestDownloads(store.Dir, kind, []core.RemoteFile{{Name: day + ".md", Day: day}}, true)
	return nil
}

// DownloadAll asks the provider for every file that is not here yet ("Download all" in Settings > Storage). It returns at once.
func (n *Notes) DownloadAll() {
	svc := n.syncSvc()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	scan, kind, store := n.refreshLocked(svc)
	svc.requestDownloads(store.Dir, kind, scan.Remote, true)
}

// startSync watches the log folder (a poll every few seconds, see core.FolderWatcher) and calls notify("folder-changed") when day
// files, placeholders or conflict copies change, so the UI reloads and asks SyncState again. A 30 s tick does the same scan as the
// Swift fallback timer and speaks up only when the sync state changed. It returns the function that stops both.
func startSync(n *Notes, notify func(event string)) (stop func()) {
	svc := n.syncSvc()
	n.SyncState() // the state the UI starts from, so the first 30 s tick only speaks up about a change
	w := core.NewFolderWatcher(func() string { return n.currentStore().Dir }, 3*time.Second, func() {
		n.SyncState() // settles identical copies and asks for downloads before the UI looks
		notify("folder-changed")
	})
	w.Start()
	quit, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-quit:
				return
			case <-t.C:
				svc.mu.Lock()
				before := svc.last
				svc.mu.Unlock()
				if st := n.SyncState(); st.key() != before {
					notify("folder-changed")
				}
			}
		}
	}()
	return func() {
		w.Stop()
		close(quit)
		<-done
	}
}
