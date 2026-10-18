// conflictfiles.go - what happens to a conflict copy once the user has chosen: it is MOVED into the safety copies
// (<backups>/conflicts/<yyyyMMdd-HHmmss>-<its name>), never deleted. Port of app/Core/Sync/ConflictFiles.swift.
// A move is done in two steps so that a failure can never lose text: KeepConflictCopy copies the file and checks the copy byte for
// byte; only after the chosen text has been saved does ReleaseConflictCopy take the original out of the shared folder, and only if
// it still matches what was kept.
package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConflictsFolder is where kept conflict copies live: <backupDir>/conflicts.
func ConflictsFolder(backupDir string) string { return filepath.Join(backupDir, "conflicts") }

// KeepConflictCopy writes the file's bytes into the conflicts folder and verifies them. The original is not touched. An earlier safety
// copy is never overwritten (an identical one is reused, so retrying is harmless). It fails when the safety copy cannot be made or does
// not match. name is what the copy is called after the time stamp ("" = the file's own name). The stamp is UTC.
func KeepConflictCopy(copyPath, backupDir string, now time.Time, name string) (string, error) {
	data, err := os.ReadFile(copyPath)
	if err != nil {
		return "", err
	}
	dir := ConflictsFolder(backupDir)
	label := name
	if label == "" {
		label = filepath.Base(copyPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// the same bytes are already kept under this name (an earlier try or scan): reuse that copy instead of piling up duplicates
	existing, _ := os.ReadDir(dir) // an unreadable folder just means nothing to reuse; the write below reports a real problem
	for _, e := range existing {
		if strings.HasSuffix(e.Name(), "-"+label) {
			path := filepath.Join(dir, e.Name())
			if got, err := os.ReadFile(path); err == nil && bytes.Equal(got, data) {
				return path, nil
			}
		}
	}
	stamp := backupStem(now)
	dest := filepath.Join(dir, stamp+"-"+label)
	for n := 2; pathExists(dest); n++ {
		if got, err := os.ReadFile(dest); err == nil && bytes.Equal(got, data) {
			return dest, nil
		}
		if n > 99 {
			return "", os.ErrExist
		}
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d-%s", stamp, n, label))
	}
	if err := AtomicWrite(data, dest, dir); err != nil {
		return "", err
	}
	if got, err := os.ReadFile(dest); err != nil || !bytes.Equal(got, data) {
		_ = os.Remove(dest) // best effort: a bad copy must not stay behind looking like a good one
		return "", errors.New("the safety copy does not match the original")
	}
	return dest, nil
}

// ReleaseConflictCopy takes the original out of the shared folder, but only if it still holds exactly what kept holds. Anything else
// (the provider wrote to it again meanwhile) leaves it where it is, and it is offered again.
func ReleaseConflictCopy(copyPath, keptPath string) error {
	a, errA := os.ReadFile(copyPath)
	b, errB := os.ReadFile(keptPath)
	if errA != nil || errB != nil || !bytes.Equal(a, b) {
		return errors.New("the other copy changed, so it was left where it is")
	}
	return os.Remove(copyPath)
}
