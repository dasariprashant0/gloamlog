package main

import (
	"os"
	"path/filepath"
	"testing"
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
