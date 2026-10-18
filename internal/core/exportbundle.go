// exportbundle.go - what "Save as Markdown" writes: the review as one .md file plus an assets/ folder holding ONLY the images it uses.
// Port of app/Core/Export/ExportBundle.swift.
//
//	PlanExport(name, markdown, dayCount, assets) ExportPlan
//	    Finds every image the markdown links (outside code), resolves each through AssetStore.Resolve (so nothing outside the log folder
//	    is ever read), and rewrites the links to "assets/<file>". A link that points at no file stays as written and is listed in Missing.
//	    Two different files with one name get a "2-" prefix. The plan is computed first, so a test (or a preview) can see exactly what a
//	    bundle holds.
//	WriteExport(plan, layout) (path, error)
//	    ExportFile(md)       <dir>/<name>.md with <dir>/assets/ beside it (assets/ only when an image is used; a file already there
//	                         with the same bytes is kept)
//	    ExportFolder(parent) a new folder <parent>/<name>/ ("<name> 2" when taken) holding the .md and assets/
//	    ExportZip(dest)      the same folder, zipped with /usr/bin/ditto (argument array, no shell), the folder being the single
//	                         top-level entry
//	    Images are copied before the .md is written (atomically), so a failure never leaves a .md without its pictures. Errors: *ExportError.
package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExportAsset is one image of a bundle.
type ExportAsset struct {
	Path   string `json:"path"`   // "assets/<file>" inside the export
	Source string `json:"source"` // the real file in the log folder
}

// ExportPlan is what a bundle will hold.
type ExportPlan struct {
	Name     string        `json:"name"` // "Gloamlog 2026-W41": the file or folder name without ".md"
	Markdown string        `json:"markdown"`
	Assets   []ExportAsset `json:"assets"`
	Missing  []string      `json:"missing"` // image links that point at no file in the log folder
	DayCount int           `json:"dayCount"`
}

// ResultLine is "Saved 5 days and 2 images."
func (p ExportPlan) ResultLine() string {
	days := Plural(p.DayCount, "day")
	if len(p.Assets) == 0 {
		return "Saved " + days + "."
	}
	return "Saved " + days + " and " + Plural(len(p.Assets), "image") + "."
}

// ExportError says why a bundle could not be written.
type ExportError struct {
	Zip bool // the zip step failed (Swift: .zipFailed); otherwise a file could not be written (.cannotWrite)
	Msg string
}

func (e *ExportError) Error() string { return e.Msg }

// ExportLayoutKind is how a bundle is laid out on disk.
type ExportLayoutKind int

const (
	ExportFile   ExportLayoutKind = iota // Path is the .md file
	ExportFolder                         // Path is the folder that will hold the new <name> folder
	ExportZip                            // Path is the .zip file
)

// ExportLayout is where and how WriteExport writes.
type ExportLayout struct {
	Kind ExportLayoutKind
	Path string
}

// PlanExport computes the bundle (see the header).
func PlanExport(name, markdown string, dayCount int, store AssetStore) ExportPlan {
	plan := ExportPlan{Name: name, Assets: []ExportAsset{}, Missing: []string{}, DayCount: dayCount}
	taken := map[string]string{}
	md := markdown
	for _, target := range MDImageTargets(markdown) {
		src := store.Resolve(target)
		if src == "" {
			plan.Missing = append(plan.Missing, target)
			continue
		}
		base := filepath.Base(src)
		file, n := base, 2
		for {
			other, ok := taken[file]
			if !ok || other == src {
				break
			}
			file = fmt.Sprintf("%d-%s", n, base)
			n++
		}
		rel := AssetFolderName + "/" + file
		if _, ok := taken[file]; !ok {
			taken[file] = src
			plan.Assets = append(plan.Assets, ExportAsset{Path: rel, Source: src})
		}
		if target != rel {
			md = strings.ReplaceAll(md, "]("+target, "]("+rel)
		}
	}
	plan.Markdown = md
	return plan
}

// WriteExport writes the bundle and returns the file (ExportFile), folder (ExportFolder) or zip (ExportZip) it made.
func WriteExport(plan ExportPlan, layout ExportLayout) (string, error) {
	out, err := writeExport(plan, layout)
	if err != nil {
		if _, ok := err.(*ExportError); !ok {
			err = &ExportError{Msg: err.Error()}
		}
		return "", err
	}
	return out, nil
}

func writeExport(plan ExportPlan, layout ExportLayout) (string, error) {
	switch layout.Kind {
	case ExportFile:
		if err := exportTree(plan, filepath.Base(layout.Path), filepath.Dir(layout.Path)); err != nil {
			return "", err
		}
		return layout.Path, nil
	case ExportFolder:
		dir, n := filepath.Join(layout.Path, plan.Name), 2
		for {
			if _, err := os.Lstat(dir); err != nil {
				break
			}
			dir = filepath.Join(layout.Path, fmt.Sprintf("%s %d", plan.Name, n))
			n++
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if err := exportTree(plan, plan.Name+".md", dir); err != nil {
			return "", err
		}
		return dir, nil
	default: // ExportZip
		tmp := os.Getenv("TMPDIR")
		if tmp == "" {
			tmp = os.TempDir()
		}
		work := filepath.Join(tmp, "gloamlog-export-"+newUUID())
		defer os.RemoveAll(work) // best effort: a leftover temp folder is the system's to sweep
		root := filepath.Join(work, plan.Name)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		if err := exportTree(plan, plan.Name+".md", root); err != nil {
			return "", err
		}
		zip := filepath.Join(work, "out.zip")
		if err := exportDitto(root, zip); err != nil {
			return "", err
		}
		if err := os.RemoveAll(layout.Path); err != nil { // an old file of that name is replaced
			return "", err
		}
		if err := os.Rename(zip, layout.Path); err != nil {
			if err := copyFile(zip, layout.Path); err != nil { // another volume: rename cannot cross it
				return "", err
			}
		}
		return layout.Path, nil
	}
}

func exportTree(plan ExportPlan, mdName, dir string) error {
	if len(plan.Assets) > 0 {
		if err := os.MkdirAll(filepath.Join(dir, AssetFolderName), 0o755); err != nil {
			return err
		}
		for _, a := range plan.Assets {
			to := filepath.Join(dir, filepath.FromSlash(a.Path))
			if _, err := os.Lstat(to); err == nil {
				if sameBytes(to, a.Source) {
					continue
				}
				if err := os.Remove(to); err != nil {
					return err
				}
			}
			if err := copyFile(a.Source, to); err != nil {
				return err
			}
		}
	}
	return AtomicWrite([]byte(plan.Markdown), filepath.Join(dir, mdName), "")
}

func sameBytes(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

// copyFile copies src to dst (which must not exist) keeping src's permission bits, like FileManager.copyItem.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()    // best effort: the copy error below is what's returned
		os.Remove(dst) // drop the partial export file; the source is untouched
		return err
	}
	return out.Close()
}

func exportDitto(dir, zip string) error {
	cmd := exec.Command("/usr/bin/ditto", "-c", "-k", "--keepParent", "--norsrc", "--noextattr", "--noqtn", "--noacl", dir, zip)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := trimmed(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return &ExportError{Zip: true, Msg: msg}
	}
	return nil
}
