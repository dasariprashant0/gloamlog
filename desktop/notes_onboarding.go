package main

// Helpers for the first-run setup and the What's new sheet (Swift: Onboarding.swift, MainView.swift WhatsNewSheet).
// The Swift app remembered "seen" in UserDefaults; here it is state.json next to settings.json (there is no settings field).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// whatsNewRelease names the release the What's new sheet talks about. Bump it with the sheet's text.
const whatsNewRelease = "0.7"

// FoundLogs is a summary of the day pages already in a folder (file names only; nothing is read or changed).
type FoundLogs struct {
	Count int    `json:"count"`
	First string `json:"first"` // "2026-01-05"
	Last  string `json:"last"`
}

// OnboardingInfo is what the folder step needs to know.
type OnboardingInfo struct {
	ICloudFolder  string     `json:"icloudFolder"`  // "" when iCloud Drive isn't on this Mac
	DefaultFolder string     `json:"defaultFolder"` // ~/Gloamlog
	Found         *FoundLogs `json:"found"`         // day pages in `folder`; nil when there are none
}

// OnboardingInfo looks at `folder` ("" = the default folder) for existing day pages and says whether iCloud Drive exists.
func (n *Notes) OnboardingInfo(folder string) OnboardingInfo {
	n.mu.Lock()
	home := n.home
	n.mu.Unlock()
	info := OnboardingInfo{DefaultFolder: core.DefaultFolder(home)}
	base := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	if st, err := os.Stat(base); err == nil && st.IsDir() {
		info.ICloudFolder = filepath.Join(base, "Gloamlog")
	}
	if folder == "" {
		folder = info.DefaultFolder
	}
	info.Found = foundLogs(folder)
	return info
}

func foundLogs(folder string) *FoundLogs {
	names := dayFileNames(filepath.Clean(folder))
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return &FoundLogs{Count: len(names), First: strings.TrimSuffix(names[0], ".md"), Last: strings.TrimSuffix(names[len(names)-1], ".md")}
}

// ---- What's new: state.json ----

func (n *Notes) statePath() string { return filepath.Join(filepath.Dir(n.settingsPath), "state.json") }

func readState(path string) map[string]any {
	m := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &m) // damaged file: start over, it only holds "seen" marks
	}
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// WhatsNewSeen reports whether the What's new sheet for this release was already shown (or skipped by first-run setup).
func (n *Notes) WhatsNewSeen() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	s, _ := readState(n.statePath())["whatsNewSeen"].(string)
	return s == whatsNewRelease
}

// MarkWhatsNewSeen remembers that the sheet was shown. Other keys in state.json stay.
func (n *Notes) MarkWhatsNewSeen() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	path := n.statePath()
	m := readState(path)
	m["whatsNewSeen"] = whatsNewRelease
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
