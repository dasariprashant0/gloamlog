package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestZoneNameReadsTheLocaltimeLink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink("/var/db/timezone/zoneinfo/Asia/Kolkata", link); err != nil {
		t.Fatal(err)
	}
	if got := zoneName(link); got != "Asia/Kolkata" {
		t.Fatalf("zone %q", got)
	}
	if got := zoneName(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing link: %q", got)
	}
}

func TestAppZoneIsLocalUntilTheZoneChanges(t *testing.T) {
	defer zone.Store(nil)
	if appZone() != time.Local {
		t.Fatal("before any change the app uses the Mac's zone")
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("no zone database")
	}
	zone.Store(tokyo)
	if appZone() != tokyo {
		t.Fatal("after a change the app uses the new zone")
	}
}
