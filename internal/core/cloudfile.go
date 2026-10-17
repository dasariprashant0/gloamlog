// cloudfile.go - the few facts about cloud files that LogStore and the search cache need: is this day's page on this Mac yet?
// Port of the minimal part of app/Core/Sync/CloudFile.swift (the Sync chunk is ported later; only what chunk A uses is here:
// placeholderName, exists, isDataless, isNotLocal, signature). FolderKind, download requests and OwnWrites are NOT ported.
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
