package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// OpenLicences opens the third-party licences the app bundle carries (Settings > About; build.sh copies
// THIRD_PARTY_LICENSES.md into Contents/Resources) in TextEdit.
func (*Windows) OpenLicences() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	p := filepath.Join(filepath.Dir(exe), "..", "Resources", "THIRD_PARTY_LICENSES.md")
	if _, err := os.Stat(p); err != nil {
		return errors.New("The licence list is part of the built app; this build has none.")
	}
	return exec.Command("open", "-e", p).Run()
}
