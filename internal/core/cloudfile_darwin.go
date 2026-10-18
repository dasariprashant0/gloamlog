package core

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// sfDataless is SF_DATALESS from <sys/stat.h>: the file's bytes live in the cloud.
const sfDataless = 0x40000000

func fileInfoIsDataless(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Flags&sfDataless != 0
}

// volumeIsLocal is false for a network volume (statfs without MNT_LOCAL). A path that cannot be examined counts as local.
func volumeIsLocal(path string) bool {
	const mntLocal = 0x1000 // MNT_LOCAL from <sys/mount.h>
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return true
	}
	return st.Flags&mntLocal != 0
}

// askICloudToDownload is `brctl download`, the command-line twin of startDownloadingUbiquitousItem (which needs Objective-C).
func askICloudToDownload(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/brctl", "download", path).Run()
}
