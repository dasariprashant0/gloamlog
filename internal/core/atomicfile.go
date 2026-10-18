// atomicfile.go - crash-safe replace of a file, used for day files, backups and settings. Standard library + POSIX only.
// Port of app/Core/AtomicFile.swift.
//
// Why not os.WriteFile? It truncates the target in place: a crash mid-write leaves a half-written day file. Here the bytes go to a
// temp file named by us, then rename(2) swaps it in, so the target is always either its old complete content or the new one.
// The temp file is named
//
//	".gloamlog-tmp-<pid of the writer>-<uuid>"   hidden (dot file), so it is never listed as a day, a backup or an unrecognised file.
//
// AtomicWrite  1. the bytes go to the temp file (in tempDir, default the target's folder; must be on the target's volume),
//  2. fsync, 3. the target's permissions are copied onto it, 4. rename(2) over the target. If anything fails the temp
//     file is removed again and the error is returned. A symlink at the target is replaced, not followed.
//
// AtomicSweep  removes ".gloamlog-tmp-*" files whose writer process is no longer running (the leftovers of a kill). A temp file of
//
//	a live process, including this one, is never touched; anything not starting with the prefix is never touched.
package core

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// AtomicTempPrefix starts the name of every temp file AtomicWrite creates.
const AtomicTempPrefix = ".gloamlog-tmp-"

// OwnWriteHook is called with the path of every file AtomicWrite has just replaced. The Swift app used it (OwnWrites.note) so the
// folder watcher could tell its own writes from somebody else's; the folder watcher is not ported yet, so nothing sets it for now.
var OwnWriteHook func(path string)

// AtomicWrite replaces dest with data as described at the top of this file. tempDir "" means dest's own folder.
func AtomicWrite(data []byte, dest, tempDir string) error {
	if tempDir == "" {
		tempDir = filepath.Dir(dest)
	}
	tmp := filepath.Join(tempDir, fmt.Sprintf("%s%d-%s", AtomicTempPrefix, os.Getpid(), newUUID()))
	if err := writeAndRename(data, tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	if OwnWriteHook != nil {
		OwnWriteHook(dest)
	}
	return nil
}

func writeAndRename(data []byte, tmp, dest string) error {
	// O_EXCL: never write into a file that is already there. 0o666 lets the umask decide, as Foundation's write did.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		// Plain fsync(2), best effort (not every file system supports it). os.File.Sync would do F_FULLFSYNC on macOS, which is
		// far slower, and the Swift app never used it.
		_ = syscall.Fsync(int(f.Fd()))
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	// Lstat, not Stat: only a regular file's permissions are copied (a symlink at the target is simply replaced).
	if fi, lerr := os.Lstat(dest); lerr == nil && fi.Mode().IsRegular() {
		_ = os.Chmod(tmp, fi.Mode().Perm()) // best effort: the content is what must not be lost
	}
	return os.Rename(tmp, dest)
}

// AtomicSweep removes the temp files of writers that no longer exist. Never fails; a folder that cannot be listed is skipped.
func AtomicSweep(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), AtomicTempPrefix) && isStaleTemp(e.Name()) {
			os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// isStaleTemp: ".gloamlog-tmp-<pid>-...": stale when no process with that pid is running. A name without a readable pid is stale too.
func isStaleTemp(name string) bool {
	rest := name[len(AtomicTempPrefix):]
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	pid, err := strconv.ParseInt(rest[:n], 10, 32) // fails for no digits and for a number too big for a pid
	if err != nil {
		return true
	}
	if int(pid) == os.Getpid() {
		return false
	}
	// ESRCH means no such process; EPERM means the process exists (it just is not ours).
	return syscall.Kill(int(pid), 0) == syscall.ESRCH
}

// newUUID returns a random version-4 UUID in upper case ("1B4E28BA-2FA1-4D3B-A8F5-3C9E1F0A7B21"), the shape Foundation's UUID prints.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand does not fail on a working system
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}
