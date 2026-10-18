//go:build !darwin

package core

import (
	"errors"
	"os"
)

// Only macOS has dataless files.
func fileInfoIsDataless(fi os.FileInfo) bool { return false }

// Every volume counts as local off macOS.
func volumeIsLocal(path string) bool { return true }

// There is no iCloud off macOS: RequestDownload falls back to the plain read.
func askICloudToDownload(path string) error { return errors.New("iCloud downloads need macOS") }
