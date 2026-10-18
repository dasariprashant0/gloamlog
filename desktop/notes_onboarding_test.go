package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardingInfoFindsLogsAndICloud(t *testing.T) {
	home := t.TempDir()
	n := &Notes{home: home, settingsPath: filepath.Join(home, "support", "settings.json")}

	info := n.OnboardingInfo("")
	if info.ICloudFolder != "" || info.Found != nil || info.DefaultFolder != filepath.Join(home, "Gloamlog") {
		t.Fatalf("fresh home: %+v", info)
	}

	obWrite(t, filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", ".keep"), "")
	folder := filepath.Join(home, "logs")
	for _, d := range []string{"2026-03-02", "2026-01-05", "2026-02-10"} {
		obWrite(t, filepath.Join(folder, d+".md"), "# "+d)
	}
	obWrite(t, filepath.Join(folder, "notes.md"), "not a day")
	info = n.OnboardingInfo(folder)
	if want := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "Gloamlog"); info.ICloudFolder != want {
		t.Errorf("iCloud folder %q", info.ICloudFolder)
	}
	if f := info.Found; f == nil || f.Count != 3 || f.First != "2026-01-05" || f.Last != "2026-03-02" {
		t.Errorf("found %+v", info.Found)
	}
}

func TestWhatsNewSeenIsRemembered(t *testing.T) {
	dir := t.TempDir()
	n := &Notes{settingsPath: filepath.Join(dir, "Gloamlog", "settings.json")}
	if n.WhatsNewSeen() {
		t.Fatal("seen before it was shown")
	}
	obWrite(t, n.statePath(), `{"other":"kept","whatsNewSeen":"0.6"}`)
	if n.WhatsNewSeen() {
		t.Error("an older release's mark must not hide this release's sheet")
	}
	if err := n.MarkWhatsNewSeen(); err != nil {
		t.Fatal(err)
	}
	if !n.WhatsNewSeen() {
		t.Error("not remembered")
	}
	if got := obRead(t, n.statePath()); !strings.Contains(got, `"other": "kept"`) {
		t.Errorf("state.json lost a key: %s", got)
	}
	if st, err := os.Stat(n.statePath()); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("state.json mode %v, %v", st, err)
	}
	obWrite(t, n.statePath(), "{broken")
	if n.WhatsNewSeen() || n.MarkWhatsNewSeen() != nil || !n.WhatsNewSeen() {
		t.Error("a damaged state.json must be replaced, not fail")
	}
}

func obWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func obRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
