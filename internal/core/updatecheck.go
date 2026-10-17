// updatecheck.go - reads GitHub's "latest release" answer and compares versions. No network here (the UI does that).
// Port of app/Core/UpdateCheck.swift.
// The release must carry two assets: Gloamlog.zip and Gloamlog.zip.sha256 (`shasum -a 256` output). Without the checksum file the
// app refuses to install by itself and only opens the release page.
package core

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// UpdateRepo is the GitHub repository releases are read from.
const UpdateRepo = "dasariprashant0/gloamlog"

// UpdateLatestURL is the GitHub API address of the latest release.
const UpdateLatestURL = "https://api.github.com/repos/" + UpdateRepo + "/releases/latest"

// ReleaseInfo is the part of a GitHub release the updater needs.
type ReleaseInfo struct {
	Version string `json:"version"` // "0.6.1" (a leading "v" removed)
	Notes   string `json:"notes"`
	PageURL string `json:"pageURL"`
	ZipURL  string `json:"zipURL"`
	ShaURL  string `json:"shaURL"` // "" = the release has no checksum file
}

// ParseRelease reads the JSON of GitHub's "latest release" answer. ok is false for a draft, a prerelease, garbage, or a release
// without the Gloamlog.zip asset.
func ParseRelease(data []byte) (ReleaseInfo, bool) {
	var o map[string]any
	if err := json.Unmarshal(data, &o); err != nil || o == nil {
		return ReleaseInfo{}, false
	}
	tag, ok := o["tag_name"].(string)
	if !ok {
		return ReleaseInfo{}, false
	}
	if draft, _ := o["draft"].(bool); draft {
		return ReleaseInfo{}, false
	}
	if pre, _ := o["prerelease"].(bool); pre {
		return ReleaseInfo{}, false
	}
	page, ok := validURL(o["html_url"])
	if !ok {
		return ReleaseInfo{}, false
	}
	assets, _ := o["assets"].([]any)
	assetURL := func(name string) (string, bool) {
		for _, a := range assets {
			m, _ := a.(map[string]any)
			if n, _ := m["name"].(string); n == name {
				return validURL(m["browser_download_url"])
			}
		}
		return "", false
	}
	zip, ok := assetURL("Gloamlog.zip")
	if !ok {
		return ReleaseInfo{}, false
	}
	notes, _ := o["body"].(string)
	sha, _ := assetURL("Gloamlog.zip.sha256")
	return ReleaseInfo{Version: strings.TrimPrefix(tag, "v"), Notes: notes, PageURL: page, ZipURL: zip, ShaURL: sha}, true
}

func validURL(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	if _, err := url.Parse(s); err != nil {
		return "", false
	}
	return s, true
}

// IsNewerVersion compares numeric dotted versions ("0.6.10" > "0.6.9"); anything that is not numbers counts as 0.
func IsNewerVersion(remote, local string) bool {
	parts := func(s string) []int {
		var out []int
		for _, p := range strings.Split(s, ".") {
			if p == "" {
				continue // Swift's split drops empty pieces
			}
			n := 0
			for n < len(p) && p[n] >= '0' && p[n] <= '9' {
				n++
			}
			v, _ := strconv.Atoi(p[:n]) // no digits or too many gives 0
			out = append(out, v)
		}
		return out
	}
	a, b := parts(remote), parts(local)
	for i := 0; i < max(len(a), len(b)); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// ChecksumFromLine is the hash in a `shasum -a 256` line ("<64 hex>  Gloamlog.zip"), lower-cased; ok is false when it is not one.
func ChecksumFromLine(text string) (string, bool) {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == '\n' || r == '\t' })
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", false
	}
	for _, r := range fields[0] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return "", false
		}
	}
	return strings.ToLower(fields[0]), true
}

// IsTrustedDownload: only GitHub may serve the download (https, github.com or a subdomain of it).
func IsTrustedDownload(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return u.Scheme == "https" && (host == "github.com" || strings.HasSuffix(host, ".github.com"))
}
