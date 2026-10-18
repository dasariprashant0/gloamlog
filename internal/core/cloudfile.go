// cloudfile.go - the few facts about cloud files that LogStore and the search cache need: is this day's page on this Mac yet?
// Port of app/Core/Sync/CloudFile.swift: placeholderName, exists, isDataless, isNotLocal, signature (chunk A), then FolderKind, download
// requests, canonical paths and OwnWrites (the Sync chunk). Not ported, because they need Objective-C (no cgo here): iCloud's own
// download status of a file, and "this Desktop/Documents folder is in iCloud" (it counts as Local; its dataless files still arrive on
// the 1-byte read that RequestDownload falls back to).
//
// A day whose page is not on this Mac yet is "not local": an iCloud placeholder (".2026-10-08.md.icloud", macOS 13) or a
// dataless file (the name is there, the bytes are not: iCloud Drive since macOS 14 and other providers' on-demand files). Such a
// day is never read, written, skipped or deleted by Gloamlog (LogStore refuses with ErrNotLocal): treating it as "no log" and
// creating a page over it is exactly how a second Mac gets a conflict copy.
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// placeholderName is the hidden name iCloud gives a day file that is still in the cloud.
func placeholderName(day string) string { return "." + day + ".md.icloud" }

// pathExists is true for anything lstat can see, including a dangling symlink.
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// pathIsDataless is true when the file's bytes live in the cloud (macOS only; always false elsewhere).
func pathIsDataless(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fileInfoIsDataless(fi)
}

// dayIsNotLocal is true when the day's page exists in the folder but is not (fully) on this Mac yet. Two lstat calls, nothing is read.
func dayIsNotLocal(dir, day string) bool {
	return pathExists(filepath.Join(dir, placeholderName(day))) || pathIsDataless(filepath.Join(dir, day+".md"))
}

// fileSignature is "mtime.nanoseconds size" of the file now (following symlinks); ok is false when it cannot be stat'ed.
// Equal signatures mean the file was not touched in between.
func fileSignature(path string) (string, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%d.%d %d", fi.ModTime().Unix(), fi.ModTime().Nanosecond(), fi.Size()), true
}

// ---- where the folder lives ----

// FolderKind is where the log folder lives. Kind is "local", "icloud", "cloud" (another provider that keeps the folder in the cloud,
// named in Provider: "Dropbox", "OneDrive", "GoogleDrive") or "network" (a network volume: file events are not reliable there, so it
// is polled).
type FolderKind struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
}

// Label is the name Settings > Storage shows.
func (k FolderKind) Label() string {
	switch k.Kind {
	case "icloud":
		return "iCloud Drive"
	case "cloud":
		return k.Provider
	case "network":
		return "Network or external drive"
	}
	return "Local"
}

// IsICloud is true for iCloud Drive.
func (k FolderKind) IsICloud() bool { return k.Kind == "icloud" }

// SourceName is "iCloud", or the provider's name, for "Downloading from iCloud".
func (k FolderKind) SourceName() string {
	switch k.Kind {
	case "icloud":
		return "iCloud"
	case "cloud":
		return k.Provider
	}
	return "your cloud folder"
}

// DetectFolderKind reads the kind from the folder's real path (symlinks resolved) and its volume. A folder that does not exist yet is local.
func DetectFolderKind(dir string) FolderKind {
	real := CanonicalPath(dir)
	return folderKindForPath(real, volumeIsLocal(real))
}

// folderKindForPath is DetectFolderKind without the disk: the real path and whether its volume is local.
func folderKindForPath(path string, volumeLocal bool) FolderKind {
	if strings.Contains(path, "/Library/Mobile Documents/com~apple~CloudDocs") {
		return FolderKind{Kind: "icloud"}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "CloudStorage" && i+1 < len(parts) {
			provider, _, _ := strings.Cut(parts[i+1], "-") // "OneDrive-Personal" -> "OneDrive"
			return FolderKind{Kind: "cloud", Provider: provider}
		}
	}
	for _, p := range parts {
		if p == "Dropbox" || strings.HasPrefix(p, "Dropbox (") {
			return FolderKind{Kind: "cloud", Provider: "Dropbox"}
		}
	}
	if !volumeLocal {
		return FolderKind{Kind: "network"}
	}
	return FolderKind{Kind: "local"}
}

// ---- download requests ----

// ubiquityDownload asks iCloud for a file's bytes (macOS: brctl; see cloudfile_darwin.go). A variable so tests can fake it.
var ubiquityDownload = askICloudToDownload

// RequestDownload asks the provider for the file's bytes. path is the real file's, never the placeholder's. Call it off the UI thread:
// for providers other than iCloud the request is a plain 1-byte read, which blocks until the bytes are here. Best effort: a file that
// cannot be requested is simply asked for again on the next scan.
func RequestDownload(path string, iCloud bool) {
	if iCloud && ubiquityDownload(path) == nil {
		return
	}
	if f, err := os.Open(path); err == nil {
		var b [1]byte
		_, _ = f.Read(b[:]) // the read is the request; its result is not needed
		_ = f.Close()
	}
}

// CanonicalPath is the path with every symlink resolved, as file events report it (/tmp is /private/tmp). The path itself when it cannot be resolved.
func CanonicalPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// ---- our own writes ----

// ownWrites remembers the signature of every file AtomicWrite wrote, so that the folder watcher can tell our writes from somebody
// else's: a file whose signature still matches is our own echo and wakes nobody.
var ownWrites = struct {
	mu   sync.Mutex
	sigs map[string]string
}{sigs: map[string]string{}}

// NoteOwnWrite records the signature of a file we have just written (AtomicWrite calls it through OwnWriteHook).
func NoteOwnWrite(path string) {
	path = CanonicalPath(path)
	sig, ok := fileSignature(path)
	if !ok {
		return
	}
	ownWrites.mu.Lock()
	defer ownWrites.mu.Unlock()
	if len(ownWrites.sigs) > 512 {
		ownWrites.sigs = map[string]string{}
	}
	ownWrites.sigs[path] = sig
}

// IsOwnWrite is true when path (canonical) is a file we wrote and nobody has touched since.
func IsOwnWrite(path string) bool {
	ownWrites.mu.Lock()
	known, ok := ownWrites.sigs[path]
	ownWrites.mu.Unlock()
	if !ok {
		return false
	}
	sig, ok := fileSignature(path)
	return ok && sig == known
}

func init() { OwnWriteHook = NoteOwnWrite }
