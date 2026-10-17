//go:build !darwin

package core

import "os"

// Only macOS has dataless files.
func fileInfoIsDataless(fi os.FileInfo) bool { return false }
