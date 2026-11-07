package testgen

import (
	"archive/zip"
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
)

// handoff is a short EPUB written to catch where readers disagree about a
// position: whitespace in the source, inline markup, comments and CDATA
// splitting text, entities, a non-linear spine item, and a long last
// chapter.
//
//go:embed testdata/handoff
var handoff embed.FS

// GenerateHandoffEPUB writes the handoff EPUB to dir as filename and returns
// its path. Tests of positions moving between readers use it; their expected
// XPointers and CFIs were taken from KOReader and foliate reading it, so a
// change to it means taking them again (see pkg/reflowable's tests).
func GenerateHandoffEPUB(t *testing.T, dir, filename string) string {
	t.Helper()
	dest := filepath.Join(dir, filename)
	out, err := os.Create(dest)
	if err != nil {
		t.Fatalf("failed to create EPUB file: %v", err)
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	root := "testdata/handoff"
	add := func(name string, method uint16) {
		data, err := handoff.ReadFile(path.Join(root, name))
		if err != nil {
			t.Fatalf("failed to read %s: %v", name, err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatalf("failed to add %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
	// The mimetype entry comes first and uncompressed, as EPUB requires.
	add("mimetype", zip.Store)
	err = fs.WalkDir(handoff, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if rel := name[len(root)+1:]; rel != "mimetype" {
			add(rel, zip.Deflate)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk the handoff EPUB: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to finish the handoff EPUB: %v", err)
	}
	return dest
}
