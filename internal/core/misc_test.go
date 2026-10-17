package core

// Ported from tests/main.swift: "legacy migration" (the two folder/defaults tests), "ClaudeMd" and "UpdateCheck".
// One file because these Swift files are tiny.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyMigrationDefaultsCopiedOnceNeverOverwrittenSystemKeysIgnored(t *testing.T) {
	old := map[string]any{"dailylog.settings.v2": []byte{1, 2, 3}, "showMenuBar": true, "unrelated": "x"}
	cur := map[string]any{}
	n := MigrateDefaults(old, cur)
	expect(t, n == 2, "copied %d", n)
	expect(t, equal(cur["dailylog.settings.v2"], []byte{1, 2, 3}) && cur["showMenuBar"] == true, "copied values")
	_, has := cur["unrelated"]
	expect(t, !has, "keys outside our namespace are not copied")
	cur["dailylog.settings.v2"] = []byte{9}
	expect(t, MigrateDefaults(old, cur) == 0 && equal(cur["dailylog.settings.v2"], []byte{9}), "second run changes nothing, existing values win")
	expect(t, MigrateDefaults(nil, cur) == 0, "nil old")
}

func TestLegacyMigrationSupportFolderMovesOnceWhenTheNewOneIsAbsent(t *testing.T) {
	support := t.TempDir()
	old := filepath.Join(support, "Daily Log/backups/2026-10-06")
	must(t, os.MkdirAll(old, 0o755))
	must(t, os.WriteFile(filepath.Join(old, "20261006-171500.md"), []byte("v1"), 0o644))
	expect(t, MigrateSupportFolder(support), "moved")
	expect(t, !pathExists(filepath.Join(support, "Daily Log")), "old folder gone")
	expect(t, pathExists(filepath.Join(support, "Gloamlog/backups/2026-10-06/20261006-171500.md")), "backup survives the move")
	expect(t, !MigrateSupportFolder(support), "nothing left to move")
	must(t, os.MkdirAll(filepath.Join(support, "Daily Log"), 0o755))
	expect(t, !MigrateSupportFolder(support), "both exist: leave both alone")
	expect(t, pathExists(filepath.Join(support, "Daily Log")) && pathExists(filepath.Join(support, "Gloamlog")), "both still there")
	expect(t, !MigrateSupportFolder(t.TempDir()), "nothing at all")
}

func TestClaudeMdAddReplaceRemoveKeepHandWrittenSection(t *testing.T) {
	base := "# Notes\n\nSome rule.\n"
	on := ClaudeMdApply(base)
	expect(t, strings.HasPrefix(on, strings.TrimSpace(base)) && strings.Contains(on, ClaudeMdBegin) && strings.HasSuffix(on, ClaudeMdEnd+"\n"), "added at the end")
	expect(t, ClaudeMdApply(on) == on, "applying twice changes nothing")
	expect(t, ClaudeMdRemove(on) == base, "removing restores the file exactly: %q", ClaudeMdRemove(on))
	expect(t, ClaudeMdApply("") == ClaudeMdBlock+"\n" && ClaudeMdRemove(ClaudeMdBlock+"\n") == "", "empty file")
	hand := "## Gloamlog work log\ncall the gloamlog `log_work` tool\n"
	expect(t, ClaudeMdApply(hand) == hand && ClaudeMdRemove(hand) == hand && ClaudeMdIsPresent(hand), "a hand-written section is kept")
	stale := base + "\n" + ClaudeMdBegin + "\nold text\n" + ClaudeMdEnd + "\n\n## After\n"
	expect(t, !strings.Contains(ClaudeMdApply(stale), "old text") && strings.HasSuffix(ClaudeMdApply(stale), "## After\n"), "a stale block is replaced in place")
}

func TestUpdateCheckVersionsReleaseJSONChecksumTrustedHosts(t *testing.T) {
	expect(t, IsNewerVersion("0.6.10", "0.6.9") && IsNewerVersion("1.0", "0.9.9") && IsNewerVersion("0.6.1", "0.6"), "newer")
	expect(t, !IsNewerVersion("0.6.0", "0.6.0") && !IsNewerVersion("0.5.9", "0.6.0") && !IsNewerVersion("0.6", "0.6.0"), "same or older")
	js := `{"tag_name":"v0.7.0","draft":false,"prerelease":false,"html_url":"https://github.com/o/r/releases/tag/v0.7.0","body":"Notes","assets":[{"name":"Gloamlog.zip","browser_download_url":"https://github.com/o/r/releases/download/v0.7.0/Gloamlog.zip"},{"name":"Gloamlog.zip.sha256","browser_download_url":"https://github.com/o/r/releases/download/v0.7.0/Gloamlog.zip.sha256"}]}`
	r, ok := ParseRelease([]byte(js))
	expect(t, ok && r.Version == "0.7.0" && r.Notes == "Notes" && r.ShaURL != "", "parsed")
	_, ok = ParseRelease([]byte(strings.Replace(js, `"prerelease":false`, `"prerelease":true`, 1)))
	expect(t, !ok, "a prerelease is not offered")
	_, ok = ParseRelease([]byte(strings.Replace(js, `Gloamlog.zip"`, `Other.zip"`, 1)))
	expect(t, !ok, "no zip asset, no update")
	_, ok = ParseRelease([]byte("[]"))
	expect(t, !ok, "garbage")
	h := strings.Repeat("ab", 32)
	got, ok := ChecksumFromLine(h + "  Gloamlog.zip\n")
	_, bad1 := ChecksumFromLine("nothex")
	_, bad2 := ChecksumFromLine("abc")
	expect(t, ok && got == h && !bad1 && !bad2, "checksum line")
	expect(t, IsTrustedDownload("https://github.com/x") && !IsTrustedDownload("http://github.com/x") &&
		!IsTrustedDownload("https://evilgithub.com/x") && !IsTrustedDownload("https://github.com.evil.io/x"), "only https github.com")
}
