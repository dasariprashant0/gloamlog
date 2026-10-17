// assetstore.go - pasted/dropped images, stored in <storage>/assets/ and referenced from markdown by a path
// RELATIVE to the storage folder:  ![](assets/2026-10-06-1a2b3c4d.png)
// Port of app/Core/AssetStore.swift.
//
// API: AssetStore{Dir}                `Dir` = the storage folder (same one LogStore uses)
//
//	Save(data, ext, day) (string, error)       relative path "assets/<day>-<8hex>.<ext>". The 8 hex are the start of the
//	                                           SHA-256 of the data, so saving identical bytes twice for a day returns the same
//	                                           path and writes one file. Allowed ext (case-insensitive, leading "." ok):
//	                                           png jpg jpeg gif webp. Max 20 MB, no empty data. Atomic write.
//	                                           Errors (see logstore.go): ErrUnsupportedAsset, ErrAssetTooLarge, ErrEmptyAsset, ErrBadDay,
//	                                           ErrFolderMissing, ErrFolderNotWritable, ErrBadPath (assets/ links outside the folder), ErrIO.
//	Resolve(relativePath) string               the real file for a markdown image path, or "". "" when: empty, absolute,
//	                                           contains "..", extension not allowed, file missing/not a regular file, or the
//	                                           real path (symlinks resolved) is outside the storage folder or not an image ext.
//	ResolveAssetURL(rawURL) string             same for a dlasset://local/<relative path> URL (uses the decoded URL path)
//	AssetFileExtension(mime, name) string      "image/png" -> "png", "image/jpeg" -> "jpg", gif, webp; falls back to the
//	                                           file name's extension; "" when neither is an allowed image type (for uploadImage)
//
// Security: symlinks that lead out of the storage folder are refused, including a symlinked assets directory.
package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	// AssetMaxBytes is the biggest image that is stored: 20 MB.
	AssetMaxBytes = 20 * 1024 * 1024
	// AssetFolderName is the folder inside the storage folder that holds the images.
	AssetFolderName = "assets"
)

var assetExtensions = map[string]bool{"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true}

// AssetStore stores and finds the images of one storage folder.
type AssetStore struct {
	Dir string
}

// assetRealPath is the canonical absolute path with every symlink resolved; ok is false if the path does not exist.
func assetRealPath(path string) (string, bool) {
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p, true
}

// extensionOf is NSString.pathExtension, lower-cased: the text after the last dot, but a dot that only starts the name (".png") does
// not make an extension, and neither does a trailing dot.
func extensionOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// AssetFileExtension is the allowed extension for an upload, from its MIME type (parameters ignored) or, failing that, its file name.
// "" means neither is an allowed image type.
func AssetFileExtension(mime, name string) string {
	m := mime
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = m[:i]
	}
	switch strings.ToLower(trimmed(m)) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg", "image/pjpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	}
	if e := extensionOf(name); assetExtensions[e] {
		return e
	}
	return ""
}

// Save stores an image for a day and returns its path relative to the storage folder.
func (a AssetStore) Save(data []byte, ext, day string) (string, error) {
	if !DayKeyIsWellFormed(day) {
		return "", logErr(ErrBadDay, day)
	}
	e := strings.TrimPrefix(strings.ToLower(trimmed(ext)), ".")
	if !assetExtensions[e] {
		return "", logErr(ErrUnsupportedAsset, ext)
	}
	if len(data) == 0 {
		return "", logErr(ErrEmptyAsset, "")
	}
	if len(data) > AssetMaxBytes {
		return "", logErr(ErrAssetTooLarge, itoa(len(data)))
	}
	if fi, err := os.Stat(a.Dir); err != nil || !fi.IsDir() {
		return "", logErr(ErrFolderMissing, a.Dir)
	}
	assets := filepath.Join(a.Dir, AssetFolderName)
	if _, err := os.Stat(assets); err != nil {
		if !isWritable(a.Dir) {
			return "", logErr(ErrFolderNotWritable, a.Dir)
		}
		if err := os.Mkdir(assets, 0o755); err != nil {
			return "", logErr(ErrFolderNotWritable, a.Dir)
		}
	}
	realRoot, ok1 := assetRealPath(a.Dir)
	realAssets, ok2 := assetRealPath(assets)
	if !ok1 || !ok2 || !strings.HasPrefix(realAssets, realRoot+"/") {
		return "", logErr(ErrBadPath, AssetFolderName)
	}
	if !isWritable(realAssets) {
		return "", logErr(ErrFolderNotWritable, assets)
	}

	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	for _, n := range []int{8, 12, 16, 24, 64} {
		name := day + "-" + hexSum[:n] + "." + e
		path := filepath.Join(assets, name)
		if _, err := os.Stat(path); err == nil {
			if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
				return AssetFolderName + "/" + name, nil
			}
			continue // different bytes with the same short hash: use a longer one
		}
		if err := AtomicWrite(data, path, ""); err != nil {
			return "", logErr(ErrIO, err.Error())
		}
		return AssetFolderName + "/" + name, nil
	}
	return "", logErr(ErrIO, "could not pick a unique asset name")
}

// Resolve is the real file for a markdown image path, or "" (see the header for when).
func (a AssetStore) Resolve(relativePath string) string {
	if relativePath == "" || strings.HasPrefix(relativePath, "/") || strings.Contains(relativePath, "\x00") {
		return ""
	}
	var comps []string
	for _, c := range strings.Split(relativePath, "/") {
		switch c {
		case "", ".":
			continue
		case "..":
			return ""
		}
		comps = append(comps, c)
	}
	if len(comps) == 0 || !assetExtensions[extensionOf(comps[len(comps)-1])] {
		return ""
	}
	path := filepath.Join(append([]string{a.Dir}, comps...)...)
	realRoot, ok1 := assetRealPath(a.Dir)
	real, ok2 := assetRealPath(path)
	if !ok1 || !ok2 || !strings.HasPrefix(real, realRoot+"/") || !assetExtensions[extensionOf(real)] {
		return ""
	}
	if fi, err := os.Stat(real); err != nil || fi.IsDir() {
		return ""
	}
	return real
}

// ResolveAssetURL is Resolve for a dlasset://local/<relative path> URL (the decoded URL path is used).
func (a AssetStore) ResolveAssetURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "dlasset" {
		return ""
	}
	return a.Resolve(strings.TrimLeft(u.Path, "/"))
}
