package main

// "Check for Updates…": one click checks GitHub's latest release and, when it is newer, downloads it, checks it and swaps it
// in without leaving the app (Swift: UI/Updater.swift; docs/port/PARITY.md 6.4). The only network traffic is the click: one
// request to api.github.com and, on install, the release's zip and checksum from github.com. Nothing runs in the background.
//
// Install steps: https + github.com hosts only -> checksum file required -> SHA-256 of the zip must match -> ditto unpacks
// -> the new bundle's id and version must match, codesign must verify -> a detached /bin/sh script waits for this process
// to quit, moves the old app aside, copies the new one in (the old one is put back on failure) and opens it. Trust is GitHub
// plus the checksum; there is no developer signature to check until the app is notarised.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// UpdateResult is the answer of a check. State: "none" (no release published, 404), "error", "uptodate" or "available".
type UpdateResult struct {
	State   string           `json:"state"`
	Current string           `json:"current"`
	Release core.ReleaseInfo `json:"release"` // only for "available"; Notes are at most 600 characters
}

// InstallOutcome is how an install ended when the app is still here. Status: "failed" (nothing was installed; Message says
// why) or "fallback" (can't install by itself; offer the download page). On success the app quits and nothing is returned.
type InstallOutcome struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// UpdateProgress is the payload of the "update-progress" event. Phase: "downloading" (Fraction 0...1), "verifying" or
// "restarting".
type UpdateProgress struct {
	Phase    string  `json:"phase"`
	Fraction float64 `json:"fraction"`
}

const (
	maxFeedBytes = 4 << 20
	maxZipBytes  = 1 << 30
	notesLimit   = 600
)

// swapScript outlives this process. Args: pid, new app path, destination path. It gives up after 60 s if the app never quits
// (for example a "save your edits?" prompt was cancelled).
const swapScript = `i=0; while kill -0 "$1" 2>/dev/null; do i=$((i+1)); [ $i -gt 300 ] && exit 1; sleep 0.2; done
OLD="$3.old"; rm -rf "$OLD"; mv "$3" "$OLD" || exit 1
if /usr/bin/ditto "$2" "$3"; then rm -rf "$OLD"; else rm -rf "$3"; mv "$OLD" "$3"; fi
xattr -dr com.apple.quarantine "$3" 2>/dev/null
open "$3"; rm -rf "$(dirname "$(dirname "$2")")"
`

// updater holds everything the flow touches, so tests can swap the network, the commands and the app bundle.
type updater struct {
	mu        sync.Mutex
	busy      bool
	client    *http.Client
	latestURL string
	current   string
	bundleID  string
	trusted   func(string) bool
	appPath   func() (string, error)                    // the running .app bundle
	workDir   func() (string, error)                    // a fresh temp dir for the download
	run       func(name string, args ...string) error   // ditto, codesign
	plist     func(path string) map[string]any          // an Info.plist as values
	start     func(script string, args ...string) error // the detached swap script
	emit      func(UpdateProgress)
	quit      func()
}

var theUpdater = newUpdater()

func newUpdater() *updater {
	return &updater{
		client:    &http.Client{},
		latestURL: core.UpdateLatestURL,
		current:   version,
		bundleID:  bundleID,
		trusted:   core.IsTrustedDownload,
		appPath:   runningBundle,
		workDir:   func() (string, error) { return os.MkdirTemp("", "gloamlog-update-") },
		run:       func(name string, args ...string) error { return exec.Command(name, args...).Run() },
		plist:     readInfoPlist,
		start:     startDetached,
		emit: func(p UpdateProgress) {
			if app := application.Get(); app != nil {
				app.Event.Emit("update-progress", p)
			}
		},
		quit: func() {
			if app := application.Get(); app != nil {
				app.Quit()
			}
		},
	}
}

// CheckForUpdates asks GitHub for the latest release. Only the "Check for Updates" click calls it.
func (n *Notes) CheckForUpdates() UpdateResult { return theUpdater.check(context.Background()) }

// InstallUpdate downloads, verifies and installs `release` (from CheckForUpdates), then restarts the app. Progress comes as
// "update-progress" events. It returns only when nothing was installed.
func (n *Notes) InstallUpdate(release core.ReleaseInfo) InstallOutcome {
	return theUpdater.install(release)
}

// OpenReleasePage opens a release page in the browser (github.com only).
func (n *Notes) OpenReleasePage(url string) error {
	if !core.IsTrustedDownload(url) {
		return errors.New("not a GitHub page")
	}
	return exec.Command("open", url).Run()
}

func (u *updater) check(ctx context.Context) UpdateResult {
	res := UpdateResult{State: "error", Current: u.current}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.latestURL, nil)
	if err != nil {
		return res
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.client.Do(req)
	if err != nil {
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		res.State = "none"
		return res
	}
	if resp.StatusCode != http.StatusOK {
		return res
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return res
	}
	rel, ok := core.ParseRelease(data)
	if !ok {
		return res
	}
	if !core.IsNewerVersion(rel.Version, u.current) {
		res.State = "uptodate"
		return res
	}
	rel.Notes = clip(strings.TrimSpace(rel.Notes), notesLimit)
	res.State, res.Release = "available", rel
	return res
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (u *updater) install(rel core.ReleaseInfo) InstallOutcome {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return InstallOutcome{Status: "failed", Message: "An update is already being installed."}
	}
	u.busy = true
	u.mu.Unlock()
	out := u.installNow(rel)
	if out.Status != "restarting" {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}
	return out
}

func failed(msg string) InstallOutcome   { return InstallOutcome{Status: "failed", Message: msg} }
func fallback(msg string) InstallOutcome { return InstallOutcome{Status: "fallback", Message: msg} }

func (u *updater) installNow(rel core.ReleaseInfo) InstallOutcome {
	if rel.ShaURL == "" || !u.trusted(rel.ZipURL) || !u.trusted(rel.ShaURL) {
		return fallback("This release has no checksum file, so Gloamlog won't install it by itself.")
	}
	dest, err := u.appPath()
	if err != nil {
		return fallback("Gloamlog isn't running from an app, so it can't replace itself.")
	}
	if dir := filepath.Dir(dest); syscall.Access(dir, 2) != nil {
		return fallback("Gloamlog can't replace itself in " + dir + ".")
	}
	u.emit(UpdateProgress{Phase: "downloading"})

	sha, err := u.fetchText(rel.ShaURL)
	want, ok := core.ChecksumFromLine(sha)
	if err != nil || !ok {
		return failed("The checksum could not be read.")
	}
	work, err := u.workDir()
	if err != nil {
		return failed("The update could not be prepared: " + err.Error())
	}
	// Best effort: a leftover temp folder loses no data, and the refusal being reported matters more than a failed delete.
	cleanup := func(o InstallOutcome) InstallOutcome { _ = os.RemoveAll(work); return o }

	zip := filepath.Join(work, "Gloamlog.zip")
	got, err := u.download(rel.ZipURL, zip)
	if err != nil {
		return cleanup(failed("The download failed."))
	}
	u.emit(UpdateProgress{Phase: "verifying", Fraction: 1})
	if got != want {
		return cleanup(failed("The download did not match its checksum, so nothing was installed."))
	}
	out := filepath.Join(work, "out")
	if err := u.run("/usr/bin/ditto", "-x", "-k", zip, out); err != nil {
		return cleanup(failed("The download could not be unpacked."))
	}
	app := filepath.Join(out, "Gloamlog.app")
	info := u.plist(filepath.Join(app, "Contents", "Info.plist"))
	id, _ := info["CFBundleIdentifier"].(string)
	ver, _ := info["CFBundleShortVersionString"].(string)
	if id != u.bundleID || ver != rel.Version || u.run("/usr/bin/codesign", "--verify", "--deep", "--strict", app) != nil {
		return cleanup(failed("The download is not a valid Gloamlog " + rel.Version + ", so nothing was installed."))
	}

	u.emit(UpdateProgress{Phase: "restarting"})
	if err := u.start(swapScript, strconv.Itoa(os.Getpid()), app, dest); err != nil {
		return cleanup(failed("The update could not be started."))
	}
	u.quit()
	return InstallOutcome{Status: "restarting"}
}

func (u *updater) fetchText(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return string(b), err
}

// download saves url to path and returns the SHA-256 of what was saved (lower-case hex), reporting progress as it goes.
func (u *updater) download(url, path string) (string, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	body := &progressReader{r: io.LimitReader(resp.Body, maxZipBytes), total: resp.ContentLength, emit: u.emit}
	if _, err := io.Copy(io.MultiWriter(f, h), body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), f.Sync()
}

// progressReader reports how much of the download has arrived, at most about a hundred times.
type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	last  float64
	emit  func(UpdateProgress)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.total > 0 {
		if f := float64(p.read) / float64(p.total); f-p.last >= 0.01 {
			p.last = f
			p.emit(UpdateProgress{Phase: "downloading", Fraction: min(f, 1)})
		}
	}
	return n, err
}

// runningBundle is the .app this process runs from (".../Gloamlog.app/Contents/MacOS/Gloamlog"); an error for a plain binary.
func runningBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	app := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Base(filepath.Dir(exe)) != "MacOS" || !strings.HasSuffix(app, ".app") {
		return "", errors.New("not an app bundle")
	}
	return app, nil
}

// readInfoPlist reads an Info.plist: the XML form directly, a binary one through plutil. Nil when it can't be read.
func readInfoPlist(path string) map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if m, ok := parsePlist(b).(map[string]any); ok {
		return m
	}
	if out, err := exec.Command("/usr/bin/plutil", "-convert", "xml1", "-o", "-", path).Output(); err == nil {
		m, _ := parsePlist(out).(map[string]any)
		return m
	}
	return nil
}

// startDetached starts /bin/sh -c script in its own session, so it keeps running after this process quits.
func startDetached(script string, args ...string) error {
	cmd := exec.Command("/bin/sh", append([]string{"-c", script, "sh"}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
