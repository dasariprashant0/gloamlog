package core

import (
	"os"
	"syscall"
)

// sfDataless is SF_DATALESS from <sys/stat.h>: the file's bytes live in the cloud.
const sfDataless = 0x40000000

func fileInfoIsDataless(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Flags&sfDataless != 0
}
