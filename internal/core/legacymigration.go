// legacymigration.go - one-time carry-over from the app's earlier name (Daily Log, bundle id local.dailylog.app).
// Port of app/Core/LegacyMigration.swift. Every location is a parameter, so tests never touch the real home folder.
//
// Why: renaming the app changes its bundle identifier, and macOS keeps preferences per identifier. Without this the
// first launch after the rename would forget onboarding, the template and the log folder, and orphan the backups.
// It only ever copies or moves; it never deletes, and never overwrites a value the new app already has.
//
// Go vs Swift: Go cannot read UserDefaults, so the preference step works on two plain maps (the caller reads the old plist into
// one and writes the other back). The Swift run(into:) that touched the real UserDefaults and Application Support is not ported:
// the app calls MigrateDefaults and MigrateSupportFolder itself with the real locations.
package core

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	LegacyDefaultsDomain = "local.dailylog.app"
	LegacySupportFolder  = "Daily Log"
	NewSupportFolder     = "Gloamlog"
	// LegacyKeyPrefix: the keys carried over are everything under our own prefix plus the two UI toggles.
	LegacyKeyPrefix = "dailylog."
)

var legacyExtraKeys = []string{"showMenuBar", "yesterdayExpanded"}

// MigrateDefaults copies keys the new domain (current) does not have yet from old into current. It returns how many were copied.
// A nil old map copies nothing.
func MigrateDefaults(old, current map[string]any) int {
	copied := 0
	for key, value := range old {
		if !strings.HasPrefix(key, LegacyKeyPrefix) && !contains(legacyExtraKeys, key) {
			continue
		}
		if _, have := current[key]; !have {
			current[key] = value
			copied++
		}
	}
	return copied
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// MigrateSupportFolder moves "<support>/Daily Log" to "<support>/Gloamlog" when only the old folder exists (the backups live there).
func MigrateSupportFolder(support string) bool {
	oldDir := filepath.Join(support, LegacySupportFolder)
	newDir := filepath.Join(support, NewSupportFolder)
	if _, err := os.Stat(oldDir); err != nil {
		return false
	}
	if _, err := os.Stat(newDir); err == nil {
		return false
	}
	return os.Rename(oldDir, newDir) == nil
}
