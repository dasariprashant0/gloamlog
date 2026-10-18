package main

// Native helpers for the Settings window (Swift: AppModel+Storage.swift and the login item). The folder change keeps the Swift
// rules: nothing is deleted, "move" copies first and removes only byte-identical sources, days present in both are never overwritten.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// SettingsInfo is what the Settings window shows that is not a setting.
type SettingsInfo struct {
	Version         string `json:"version"`
	BackupDir       string `json:"backupDir"`
	BackupDirExists bool   `json:"backupDirExists"`
	DefaultFolder   string `json:"defaultFolder"`
	LoginEnabled    bool   `json:"loginEnabled"` // the login item is registered with macOS
}

func (n *Notes) SettingsInfo() SettingsInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.info()
}

func (n *Notes) info() SettingsInfo {
	_, err := os.Stat(n.backupDir)
	i := SettingsInfo{Version: version, BackupDir: n.backupDir, BackupDirExists: err == nil, DefaultFolder: core.DefaultFolder(n.home)}
	if app := application.Get(); app != nil && app.Autostart != nil {
		i.LoginEnabled, _ = app.Autostart.IsEnabled()
	}
	return i
}

// SetLoginItem registers or removes the login item and remembers the result in the settings.
func (n *Notes) SetLoginItem(on bool) (SettingsInfo, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	app := application.Get()
	if app == nil || app.Autostart == nil {
		return n.info(), errors.New("the login item is not available here")
	}
	var err error
	if on {
		err = app.Autostart.Enable()
	} else {
		err = app.Autostart.Disable()
	}
	i := n.info()
	s := n.settings
	s.LaunchAtLogin = i.LoginEnabled
	if s.LaunchAtLogin != n.settings.LaunchAtLogin {
		if serr := s.Save(n.settingsPath); serr == nil {
			n.apply(s)
		}
	}
	return i, err
}

// OpenLoginItems opens System Settings at Login Items (for "Approve Gloamlog...").
func (n *Notes) OpenLoginItems() error {
	return exec.Command("open", "x-apple.systempreferences:com.apple.LoginItems-Settings.extension").Run()
}

// Reveal shows the paths selected in Finder.
func (n *Notes) Reveal(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return exec.Command("open", append([]string{"-R"}, paths...)...).Run()
}

// ChooseFolder asks for a folder (message says what it is for); "" when the user cancels.
func (n *Notes) ChooseFolder(message string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to ask from")
	}
	d := app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		SetTitle("Choose a folder").SetMessage(message).SetButtonText("Choose")
	if w, ok := app.Window.GetByName("settings"); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// RecreateFolder makes the log folder again when it went missing ("Create it again").
func (n *Notes) RecreateFolder() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store.EnsureFolder()
}

// FolderPlan says what a change to `path` would meet, so the window can ask the right question.
type FolderPlan struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Same     bool   `json:"same"`     // already the log folder
	Writable bool   `json:"writable"` // false: nothing can be saved there
	Mine     int    `json:"mine"`     // day files in the current folder
	Theirs   int    `json:"theirs"`   // day files already in the new folder
}

func (n *Notes) PlanFolderChange(path string) FolderPlan {
	n.mu.Lock()
	defer n.mu.Unlock()
	dir, old := filepath.Clean(path), filepath.Clean(n.settings.StorageFolder)
	p := FolderPlan{Path: dir, Name: filepath.Base(dir), Same: dir == old}
	if p.Same {
		return p
	}
	_ = os.MkdirAll(dir, 0o755) // like the Swift flow: a folder you just made in the picker, or the default
	p.Writable = syscall.Access(dir, 2) == nil
	p.Mine, p.Theirs = len(dayFileNames(old)), len(dayFileNames(dir))
	return p
}

// FolderResult is what a folder change did, in words, plus the days that already existed in the new folder.
type FolderResult struct {
	Text      string   `json:"text"`
	Conflicts []string `json:"conflicts"`
}

// ChangeFolder switches the log folder. mode: "use" (just switch), "copy" or "move" (bring the logs and images along first).
func (n *Notes) ChangeFolder(path, mode string) (FolderResult, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	dir, old := filepath.Clean(path), filepath.Clean(n.settings.StorageFolder)
	if dir == old {
		return FolderResult{}, errors.New("That is already your log folder.")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil || syscall.Access(dir, 2) != nil {
		return FolderResult{}, errors.New("Can't save to this folder. Nothing was changed.")
	}
	name := filepath.Base(dir)
	res := FolderResult{Conflicts: []string{}}
	switch mode {
	case "copy", "move":
		// An old folder we can't list would "copy 0 logs" and then switch away from it: refuse instead, nothing changed.
		if _, err := os.ReadDir(old); err != nil {
			return FolderResult{}, errors.New("Couldn't read your current log folder, so nothing was changed: " + err.Error())
		}
		res = transfer(old, dir, mode == "move")
	default:
		res.Text = "Now saving to " + name + "."
		if len(dayFileNames(old)) > 0 {
			res.Text += " Your old logs stayed where they were."
		}
	}
	s := n.settings
	s.StorageFolder = dir
	s = s.Normalized()
	if err := s.Save(n.settingsPath); err != nil {
		return res, err
	}
	n.apply(s)
	return res, nil
}

func dayFileNames(dir string) []string {
	entries, _ := os.ReadDir(dir) // a count only; ChangeFolder checks the old folder is readable before copying
	var out []string
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".md") && core.DayKeyIsWellFormed(strings.TrimSuffix(n, ".md")) {
			out = append(out, n)
		}
	}
	return out
}

func transfer(from, to string, move bool) FolderResult {
	copied, failed, images, imageFailed := 0, 0, 0, 0
	res := FolderResult{Conflicts: []string{}}
	for _, name := range dayFileNames(from) {
		src, dst := filepath.Join(from, name), filepath.Join(to, name)
		if _, err := os.Lstat(dst); err == nil {
			res.Conflicts = append(res.Conflicts, dst)
			continue
		}
		if err := copyNew(src, dst, move); err != nil {
			failed++
		} else {
			copied++
		}
	}
	images, imageFailed = transferAssets(from, to, move)
	verb := "Copied"
	if move {
		verb = "Moved"
	}
	res.Text = fmt.Sprintf("%s %d %s", verb, copied, plural(copied, "log"))
	if images > 0 {
		res.Text += fmt.Sprintf(" and %d %s", images, plural(images, "image"))
	}
	res.Text += "."
	if len(res.Conflicts) > 0 {
		res.Text += fmt.Sprintf(" %d %s already existed in the new folder and were left alone.", len(res.Conflicts), plural(len(res.Conflicts), "day"))
	}
	if failed+imageFailed > 0 {
		res.Text += fmt.Sprintf(" %d could not be copied and stayed where they were.", failed+imageFailed)
	}
	return res
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// transferAssets brings assets/ along. A symlinked assets folder that leaves the log folder is not followed.
func transferAssets(from, to string, move bool) (copied, failed int) {
	root, err := filepath.EvalSymlinks(from)
	if err != nil {
		return 0, 0
	}
	real, err := filepath.EvalSymlinks(filepath.Join(from, core.AssetFolderName))
	if err != nil || !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return 0, 0
	}
	entries, _ := os.ReadDir(real) // no readable assets folder: no images to bring along
	for _, e := range entries {
		n := e.Name()
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(n), "."))
		if e.IsDir() || strings.HasPrefix(n, ".") || !map[string]bool{"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true}[ext] {
			continue
		}
		src, dst := filepath.Join(real, n), filepath.Join(to, core.AssetFolderName, n)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			failed++
			continue
		}
		if _, err := os.Lstat(dst); err == nil { // names carry a content hash: the same name is the same picture
			if move && sameBytes(src, dst) {
				_ = os.Remove(src) // same picture already there; if the delete fails the original simply stays
			}
			continue
		}
		if err := copyNew(src, dst, move); err != nil {
			failed++
		} else {
			copied++
		}
	}
	return copied, failed
}

// copyNew copies src to a file that must not exist yet; with move it then removes src, but only when the copy is identical.
func copyNew(src, dst string, move bool) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()    // best effort: the copy failed; the original is untouched
		os.Remove(dst) // drop the partial copy
		return err
	}
	// On disk before the original can go: a move must never leave a day only in a page cache.
	if err := out.Sync(); err != nil {
		out.Close()    // best effort: the copy failed; the original is untouched
		os.Remove(dst) // drop the partial copy
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst) // drop the partial copy; the original is untouched
		return err
	}
	if move && sameBytes(src, dst) {
		_ = os.Remove(src) // copy verified identical; if the delete fails the original simply stays
	}
	return nil
}

func sameBytes(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}
