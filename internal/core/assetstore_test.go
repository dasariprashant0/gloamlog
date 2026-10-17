package core

// Ported from tests/main.swift: "assets".

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAssetSaveNamesDedupeExtAllowlistSizeCap(t *testing.T) {
	store := newStore(t)
	a := store.Assets()
	p := val(a.Save([]byte("hello"), "png", "2026-10-06"))
	expect(t, p == "assets/2026-10-06-2cf24dba.png", "SHA-256 prefix of the bytes: %s", p)
	expect(t, pathExists(filepath.Join(store.Dir, p)), "file written")
	expect(t, val(a.Save([]byte("hello"), "png", "2026-10-06")) == p, "same bytes, same day, same path")
	expect(t, len(dirNames(t, filepath.Join(store.Dir, "assets"))) == 1, "one file")
	expect(t, val(a.Save([]byte("hello"), "png", "2026-10-07")) == "assets/2026-10-07-2cf24dba.png", "another day")
	q := val(a.Save(pngBytes, "png", "2026-10-06"))
	expect(t, q != p && regexp.MustCompile(`^assets/2026-10-06-[0-9a-f]{8}\.png$`).MatchString(q), "%s", q)
	for _, c := range [][2]string{{"PNG", "png"}, {".jpg", "jpg"}, {"jpeg", "jpeg"}, {"GIF", "gif"}, {" webp ", "webp"}} {
		r := val(a.Save([]byte("x-"+c[0]), c[0], "2026-10-06"))
		expect(t, strings.HasSuffix(r, "."+c[1]) && strings.HasPrefix(r, "assets/2026-10-06-"), "%s -> %s", c[0], r)
	}
	for _, ext := range []string{"svg", "exe", "png/../x", "", "md", "heic", "png.exe", "../png"} {
		_, err := a.Save(pngBytes, ext, "2026-10-06")
		expectErr(t, err, ErrUnsupportedAsset, ext)
	}
	_, err := a.Save(nil, "png", "2026-10-06")
	expectErr(t, err, ErrEmptyAsset, "")
	for _, bad := range []string{"../x", "2026-10-6", "assets"} {
		_, err = a.Save(pngBytes, "png", bad)
		expectErr(t, err, ErrBadDay, bad)
	}
	_, err = a.Save(make([]byte, AssetMaxBytes+1), "png", "2026-10-06")
	expectErr(t, err, ErrAssetTooLarge, itoa(AssetMaxBytes+1))
	exact := val(a.Save(make([]byte, AssetMaxBytes), "png", "2026-10-06"))
	expect(t, AssetMaxBytes == 20*1024*1024 && strings.HasPrefix(exact, "assets/"), "exactly 20 MB is accepted")
	nofolder := AssetStore{Dir: filepath.Join(t.TempDir(), "no-such")}
	_, err = nofolder.Save(pngBytes, "png", "2026-10-06")
	expectErr(t, err, ErrFolderMissing, nofolder.Dir)
}

func TestAssetFileExtension(t *testing.T) {
	for _, c := range []struct{ mime, name, want string }{
		{"image/png", "", "png"}, {"IMAGE/JPEG; charset=binary", "x.png", "jpg"}, {"image/jpg", "", "jpg"},
		{"image/gif", "a.png", "gif"}, {"image/webp", "", "webp"}, {" image/png ", "", "png"},
		{"application/octet-stream", "Photo.JPEG", "jpeg"}, {"", "pic.webp", "webp"}, {"", "pic.gif", "gif"},
		{"image/svg+xml", "a.svg", ""}, {"", "", ""}, {"text/plain", "notes.md", ""}, {"image/heic", "a.heic", ""},
	} {
		if got := AssetFileExtension(c.mime, c.name); got != c.want {
			t.Errorf("AssetFileExtension(%q, %q) = %q, want %q", c.mime, c.name, got, c.want)
		}
	}
}

func TestAssetResolveInsideTheFolderOnly(t *testing.T) {
	store := newStore(t)
	a, dir := store.Assets(), store.Dir
	p := val(a.Save(pngBytes, "png", "2026-10-06"))
	real, _ := assetRealPath(filepath.Join(dir, p))
	expect(t, a.Resolve(p) == real && pathExists(real), "resolves to the real file")
	expect(t, a.Resolve("./"+p) == real && a.Resolve("assets//"+p[len("assets/"):]) == real, "normalised")
	expect(t, a.Resolve("") == "" && a.Resolve("assets/") == "" && a.Resolve("assets") == "", "empty and folder paths")
	for _, bad := range []string{"../outside.png", "assets/../../outside.png", "assets/../" + p[len("assets/"):], "assets/..", "..", "a/b/../../../x.png"} {
		expect(t, a.Resolve(bad) == "", "%s", bad)
	}
	expect(t, a.Resolve("/etc/passwd.png") == "" && a.Resolve("/"+p) == "" && a.Resolve(real) == "", "absolute paths")
	expect(t, a.Resolve("file:///etc/hosts.png") == "" && a.Resolve("https://example.com/a.png") == "", "URLs")
	expect(t, a.Resolve("%2e%2e/x.png") == "" && a.Resolve("assets/nope.png") == "", "missing file")
	must(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("notes"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "assets/a.svg"), []byte("x"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "assets/noext"), []byte("x"), 0o644))
	expect(t, a.Resolve("notes.md") == "" && a.Resolve("assets/a.svg") == "" && a.Resolve("assets/noext") == "", "non-image extensions")
	must(t, os.MkdirAll(filepath.Join(dir, "assets/dir.png"), 0o755))
	expect(t, a.Resolve("assets/dir.png") == "", "a directory named like an image")
	must(t, os.WriteFile(filepath.Join(dir, "loose.JPG"), pngBytes, 0o644))
	expect(t, a.Resolve("loose.JPG") != "", "images elsewhere in the folder are fine, ext case-insensitive")
	// dlasset:// URLs
	expect(t, a.ResolveAssetURL("dlasset://local/"+p) == real, "dlasset URL")
	expect(t, a.ResolveAssetURL("https://local/"+p) == "" && a.ResolveAssetURL("dlasset://local/assets/%2e%2e/%2e%2e/x.png") == "", "other scheme, encoded dots")
	expect(t, a.ResolveAssetURL("dlasset://local/") == "", "no path")
}

func TestAssetSymlinksCannotEscapeTheStorageFolder(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "store")
	outside := filepath.Join(base, "outside")
	must(t, os.MkdirAll(dir, 0o755))
	must(t, os.MkdirAll(outside, 0o755))
	a := AssetStore{Dir: dir}
	must(t, os.WriteFile(filepath.Join(outside, "secret.png"), pngBytes, 0o644))
	must(t, os.WriteFile(filepath.Join(outside, "passwords.txt"), []byte("top secret"), 0o644))
	assets := filepath.Join(dir, "assets")
	must(t, os.MkdirAll(assets, 0o755))
	must(t, os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(assets, "evil.png")))
	must(t, os.Symlink(outside, filepath.Join(assets, "linkdir")))
	must(t, os.Symlink(filepath.Join(outside, "passwords.txt"), filepath.Join(assets, "txt.png")))
	must(t, os.Symlink("../../outside/secret.png", filepath.Join(assets, "rel.png")))
	expect(t, a.Resolve("assets/evil.png") == "", "file symlink leaving the folder")
	expect(t, a.Resolve("assets/linkdir/secret.png") == "", "directory symlink leaving the folder")
	expect(t, a.Resolve("assets/txt.png") == "" && a.Resolve("assets/rel.png") == "", "png-named links to other files")
	// links that stay inside are fine, but only to images
	p := val(a.Save(pngBytes, "png", "2026-10-06"))
	must(t, os.Symlink(filepath.Join(dir, p), filepath.Join(assets, "alias.png")))
	must(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("notes"), 0o644))
	must(t, os.Symlink(filepath.Join(dir, "notes.md"), filepath.Join(assets, "md.png")))
	want, _ := assetRealPath(filepath.Join(dir, p))
	expect(t, a.Resolve("assets/alias.png") == want, "alias inside the folder resolves to the real file")
	expect(t, a.Resolve("assets/md.png") == "", "a link to a non-image inside the folder")
	// the storage folder itself reached through a symlink still works
	linkToStore := filepath.Join(t.TempDir(), "storage-link")
	must(t, os.Symlink(dir, linkToStore))
	expect(t, AssetStore{Dir: linkToStore}.Resolve(p) != "", "root given as a symlink")
	// a symlinked assets/ directory pointing outside: no saves, no resolves
	dir2 := filepath.Join(base, "store2")
	outside2 := filepath.Join(base, "outside2")
	must(t, os.MkdirAll(dir2, 0o755))
	must(t, os.MkdirAll(outside2, 0o755))
	must(t, os.WriteFile(filepath.Join(outside2, "a.png"), pngBytes, 0o644))
	must(t, os.Symlink(outside2, filepath.Join(dir2, "assets")))
	_, err := AssetStore{Dir: dir2}.Save(pngBytes, "png", "2026-10-06")
	expectErr(t, err, ErrBadPath, "assets")
	expect(t, AssetStore{Dir: dir2}.Resolve("assets/a.png") == "", "everything behind an escaping assets/ link is refused")
	expect(t, equal(dirNames(t, outside2), []string{"a.png"}), "nothing was written outside")
}
