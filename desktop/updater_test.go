package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// The tests never touch the network (an http.Handler answers through a stub transport, no port is opened), /Applications or
// the real app: commands are stubbed and the "installed app" is a folder in t.TempDir().

// stubClient answers every request with h, in memory.
func stubClient(h http.Handler) *http.Client {
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Result(), nil
	})}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const stubHost = "https://releases.test"

func releaseJSON(version string, assets ...string) string {
	var a []string
	for _, name := range assets {
		a = append(a, fmt.Sprintf(`{"name":%q,"browser_download_url":"https://github.com/x/y/releases/download/v%s/%s"}`, name, version, name))
	}
	return fmt.Sprintf(`{"tag_name":"v%s","html_url":"https://github.com/x/y/releases/tag/v%s","body":%q,"assets":[%s]}`,
		version, version, "  "+strings.Repeat("é", 700)+"  ", strings.Join(a, ","))
}

func checkWith(t *testing.T, current string, status int, body string) UpdateResult {
	t.Helper()
	u := newUpdater()
	u.client = stubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	u.latestURL, u.current = stubHost+"/latest", current
	return u.check(t.Context())
}

func TestCheckStates(t *testing.T) {
	zip := []string{"Gloamlog.zip", "Gloamlog.zip.sha256"}
	if r := checkWith(t, "0.6", 404, ""); r.State != "none" {
		t.Errorf("404: %+v", r)
	}
	if r := checkWith(t, "0.6", 500, ""); r.State != "error" {
		t.Errorf("500: %+v", r)
	}
	if r := checkWith(t, "0.6", 200, "not json"); r.State != "error" {
		t.Errorf("garbage: %+v", r)
	}
	if r := checkWith(t, "0.6", 200, releaseJSON("0.7", "Gloamlog.zip.sha256")); r.State != "error" {
		t.Errorf("no zip asset: %+v", r)
	}
	if r := checkWith(t, "0.7", 200, releaseJSON("0.7", zip...)); r.State != "uptodate" || r.Current != "0.7" {
		t.Errorf("same version: %+v", r)
	}
	r := checkWith(t, "0.6.9", 200, releaseJSON("0.6.10", zip...))
	if r.State != "available" || r.Release.Version != "0.6.10" || r.Current != "0.6.9" {
		t.Fatalf("newer: %+v", r)
	}
	if n := len([]rune(r.Release.Notes)); n != 600 || strings.HasPrefix(r.Release.Notes, " ") {
		t.Errorf("notes: %d characters, want 600 trimmed", n)
	}
	if r.Release.ShaURL == "" {
		t.Error("checksum asset lost")
	}
	if r := checkWith(t, "0.6", 200, releaseJSON("0.7", "Gloamlog.zip")); r.State != "available" || r.Release.ShaURL != "" {
		t.Errorf("no checksum file still counts as available: %+v", r)
	}
}

func TestCheckNetworkErrorIsError(t *testing.T) {
	u := newUpdater()
	u.client = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	u.latestURL = stubHost + "/latest"
	if r := u.check(t.Context()); r.State != "error" {
		t.Errorf("closed server: %+v", r)
	}
}

// ---- install ----

type fakeInstall struct {
	u       *updater
	dest    string
	started [][]string
	quit    bool
	events  []UpdateProgress
	ran     []string
	rel     core.ReleaseInfo
}

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>%s</string>
<key>CFBundleShortVersionString</key><string>%s</string>
</dict></plist>`

// newFakeInstall serves a zip with these bytes and a checksum file; "ditto" writes an app with the given id and version.
func newFakeInstall(t *testing.T, zipBytes []byte, shaLine, id, ver string) *fakeInstall {
	t.Helper()
	client := stubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Gloamlog.zip":
			_, _ = w.Write(zipBytes)
		case "/Gloamlog.zip.sha256":
			_, _ = w.Write([]byte(shaLine))
		default:
			http.NotFound(w, r)
		}
	}))
	root := t.TempDir()
	f := &fakeInstall{dest: filepath.Join(root, "apps", "Gloamlog.app"), rel: core.ReleaseInfo{Version: ver, ZipURL: stubHost + "/Gloamlog.zip", ShaURL: stubHost + "/Gloamlog.zip.sha256"}}
	if err := os.MkdirAll(f.dest, 0o755); err != nil {
		t.Fatal(err)
	}
	u := newUpdater()
	u.client = client
	u.current, u.bundleID = "0.6", "io.example.gloamlog"
	u.trusted = func(string) bool { return true } // releases.test is not github.com; the host rules have their own test
	u.appPath = func() (string, error) { return f.dest, nil }
	u.workDir = func() (string, error) {
		return filepath.Join(root, "work"), os.MkdirAll(filepath.Join(root, "work"), 0o755)
	}
	u.run = func(name string, args ...string) error {
		f.ran = append(f.ran, filepath.Base(name))
		if filepath.Base(name) == "ditto" {
			c := filepath.Join(args[len(args)-1], "Gloamlog.app", "Contents")
			if err := os.MkdirAll(c, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(c, "Info.plist"), []byte(fmt.Sprintf(infoPlist, id, ver)), 0o644)
		}
		return nil
	}
	u.start = func(script string, args ...string) error {
		f.started = append(f.started, append([]string{script}, args...))
		return nil
	}
	u.emit = func(p UpdateProgress) { f.events = append(f.events, p) }
	u.quit = func() { f.quit = true }
	f.u = u
	return f
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestInstallHappyPath(t *testing.T) {
	zip := []byte("pretend zip bytes")
	f := newFakeInstall(t, zip, strings.ToUpper(sum(zip))+"  Gloamlog.zip\n", "io.example.gloamlog", "0.7")
	out := f.u.install(f.rel)
	if out.Status != "restarting" || !f.quit {
		t.Fatalf("outcome %+v, quit %v", out, f.quit)
	}
	if got := strings.Join(f.ran, ","); got != "ditto,codesign" {
		t.Errorf("commands %q", got)
	}
	if len(f.started) != 1 || f.started[0][0] != swapScript {
		t.Fatalf("script start %v", f.started)
	}
	args := f.started[0]
	if args[1] != fmt.Sprint(os.Getpid()) || !strings.HasSuffix(args[2], "out/Gloamlog.app") || args[3] != f.dest {
		t.Errorf("script args %v", args[1:])
	}
	var phases []string
	for _, e := range f.events {
		phases = append(phases, e.Phase)
	}
	if p := strings.Join(phases, ","); !strings.HasPrefix(p, "downloading") || !strings.HasSuffix(p, "verifying,restarting") {
		t.Errorf("phases %s", p)
	}
	if !f.u.busy {
		t.Error("a second install must be refused while the app is restarting")
	}
}

func TestInstallRefusals(t *testing.T) {
	zip := []byte("pretend zip bytes")
	good := sum(zip) + "  Gloamlog.zip\n"
	cases := []struct {
		name   string
		sha    string
		id     string
		ver    string
		tweak  func(f *fakeInstall)
		status string
		text   string
	}{
		{"checksum differs", sum([]byte("other")) + "  Gloamlog.zip", "io.example.gloamlog", "0.7", nil, "failed", "did not match its checksum"},
		{"checksum unreadable", "not a hash", "io.example.gloamlog", "0.7", nil, "failed", "checksum could not be read"},
		{"other bundle id", good, "com.evil.app", "0.7", nil, "failed", "not a valid Gloamlog 0.7"},
		{"other version", good, "io.example.gloamlog", "0.8", func(f *fakeInstall) { f.rel.Version = "0.7" }, "failed", "not a valid Gloamlog 0.7"},
		{"signature fails", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) {
			run := f.u.run
			f.u.run = func(name string, args ...string) error {
				if filepath.Base(name) == "codesign" {
					return errors.New("bad signature")
				}
				return run(name, args...)
			}
		}, "failed", "not a valid Gloamlog 0.7"},
		{"unpack fails", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) {
			f.u.run = func(string, ...string) error { return errors.New("ditto") }
		}, "failed", "could not be unpacked"},
		{"no checksum file", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) { f.rel.ShaURL = "" }, "fallback", "no checksum file"},
		{"checksum file 404", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) { f.rel.ShaURL += "-missing" }, "failed", "checksum could not be read"},
		{"zip 404", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) { f.rel.ZipURL += "-missing" }, "failed", "download failed"},
		{"not an app bundle", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) {
			f.u.appPath = func() (string, error) { return "", errors.New("plain binary") }
		}, "fallback", "isn't running from an app"},
		{"folder not writable", good, "io.example.gloamlog", "0.7", func(f *fakeInstall) {
			f.u.appPath = func() (string, error) { return "/System/Applications/Gloamlog.app", nil }
		}, "fallback", "can't replace itself in /System/Applications"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeInstall(t, zip, c.sha, c.id, c.ver)
			if c.tweak != nil {
				c.tweak(f)
			}
			out := f.u.install(f.rel)
			if out.Status != c.status || !strings.Contains(out.Message, c.text) {
				t.Errorf("outcome %+v, want %s containing %q", out, c.status, c.text)
			}
			if f.quit || len(f.started) > 0 {
				t.Error("nothing may be swapped or quit after a refusal")
			}
			if f.u.busy {
				t.Error("a refusal must leave the updater free for another try")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(f.dest), "..", "work", "Gloamlog.zip")); err == nil {
				t.Error("the download was not cleaned up")
			}
		})
	}
}

// With the real host rules a server that is not GitHub is never downloaded from, whatever the release says.
func TestInstallRefusesOtherHosts(t *testing.T) {
	zip := []byte("z")
	f := newFakeInstall(t, zip, sum(zip), "io.example.gloamlog", "0.7")
	f.u.trusted = core.IsTrustedDownload
	if out := f.u.install(f.rel); out.Status != "fallback" || len(f.events) != 0 {
		t.Errorf("outcome %+v, events %v", out, f.events)
	}
	f.rel.ZipURL, f.rel.ShaURL = "http://github.com/x/Gloamlog.zip", "https://evilgithub.com/x/Gloamlog.zip.sha256"
	if out := f.u.install(f.rel); out.Status != "fallback" {
		t.Errorf("plain http / look-alike host: %+v", out)
	}
	for url, want := range map[string]bool{
		"https://github.com/a/b": true, "https://objects.github.com/x": true, "https://api.github.com/x": true,
		"http://github.com/a": false, "https://github.com.evil.io/a": false, "https://notgithub.com/a": false, "file:///etc/passwd": false,
	} {
		if core.IsTrustedDownload(url) != want {
			t.Errorf("IsTrustedDownload(%q) = %v", url, !want)
		}
	}
	if err := (&Notes{}).OpenReleasePage("https://example.com/"); err == nil {
		t.Error("OpenReleasePage must refuse other hosts")
	}
}

func TestRunningBundleNeedsAnAppFolder(t *testing.T) {
	// Under `go test` the binary is not inside an .app.
	if p, err := runningBundle(); err == nil {
		t.Errorf("runningBundle() = %q, want an error", p)
	}
}

// ---- the swap script ----

func TestSwapScriptSyntax(t *testing.T) {
	if out, err := exec.Command("/bin/sh", "-n", "-c", swapScript).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v\n%s", err, out)
	}
	for _, want := range []string{`kill -0 "$1"`, `-gt 300`, `OLD="$3.old"`, `mv "$3" "$OLD" || exit 1`, `/usr/bin/ditto "$2" "$3"`,
		`mv "$OLD" "$3"`, `xattr -dr com.apple.quarantine "$3"`, `open "$3"`} {
		if !strings.Contains(swapScript, want) {
			t.Errorf("script lost %q", want)
		}
	}
}

// Runs the real script on temp folders with a stub `open`: the old app is replaced, the work dir goes, the new app opens.
func TestSwapScriptReplacesTheApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("ditto and xattr are macOS tools")
	}
	root := t.TempDir()
	write := func(path, text string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(root, "apps", "Gloamlog.app")
	write(filepath.Join(dest, "version"), "old")
	work := filepath.Join(root, "work")
	fresh := filepath.Join(work, "out", "Gloamlog.app")
	write(filepath.Join(fresh, "version"), "new")
	bin := filepath.Join(root, "bin")
	opened := filepath.Join(root, "opened")
	write(filepath.Join(bin, "open"), "#!/bin/sh\necho \"$1\" > "+opened+"\n")
	if err := os.Chmod(filepath.Join(bin, "open"), 0o755); err != nil {
		t.Fatal(err)
	}
	done := exec.Command("true")
	_ = done.Run() // a pid that is already gone, so the script does not wait

	cmd := exec.Command("/bin/sh", "-c", swapScript, "sh", fmt.Sprint(done.Process.Pid), fresh, dest)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("script: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "version")); string(b) != "new" {
		t.Errorf("app is %q, want new", b)
	}
	if _, err := os.Stat(dest + ".old"); err == nil {
		t.Error("the old copy was left behind")
	}
	if _, err := os.Stat(work); err == nil {
		t.Error("the work folder was left behind")
	}
	if b, _ := os.ReadFile(opened); strings.TrimSpace(string(b)) != dest {
		t.Errorf("opened %q", b)
	}
}

// A copy that fails puts the old app back.
func TestSwapScriptRestoresOnFailure(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("ditto and xattr are macOS tools")
	}
	root := t.TempDir()
	dest := filepath.Join(root, "Gloamlog.app")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "version"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	_ = os.MkdirAll(bin, 0o755)
	_ = os.WriteFile(filepath.Join(bin, "open"), []byte("#!/bin/sh\n"), 0o755)
	done := exec.Command("true")
	_ = done.Run()
	cmd := exec.Command("/bin/sh", "-c", swapScript, "sh", fmt.Sprint(done.Process.Pid), filepath.Join(root, "missing", "x", "Gloamlog.app"), dest)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	_ = cmd.Run()
	if b, _ := os.ReadFile(filepath.Join(dest, "version")); string(b) != "old" {
		t.Errorf("app is %q, want the old one back", b)
	}
}
