package testutils

import (
	"archive/zip"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/mobigen"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// pageSpec is the pages a CBZ written for withFileOnDisk has.
type pageSpec struct {
	count, width, height int
}

// writeFileOnDisk writes a small valid file of fileType for withFileOnDisk
// and returns its path without the extension, as books store it. Only the
// types the E2E tests open have a writer.
func writeFileOnDisk(root, fileType, title string, pages pageSpec) (string, error) {
	var write func(path, title string) error
	switch fileType {
	case models.FileTypeEPUB:
		write = writeMinimalEPUB
	case models.FileTypeMOBI:
		write = writeMinimalMOBI
	case models.FileTypeCBZ:
		write = func(path, _ string) error { return writeMinimalCBZ(path, pages) }
	default:
		return "", errcodes.ValidationError("withFileOnDisk requires fileType epub, mobi, or cbz")
	}
	base, err := tempFilePath(root, title)
	if err != nil {
		return "", err
	}
	if err := write(base+"."+fileType, title); err != nil {
		return "", err
	}
	return base, nil
}

// writeMinimalMOBI writes a MOBI6 file with a title and one short chapter.
// foliate cannot open the KF8 files mobigen builds (they have no skeleton
// index), so this stays MOBI6.
func writeMinimalMOBI(path, title string) error {
	b, err := mobigen.Build(mobigen.Options{Kind: mobigen.KindMOBI6, Title: title})
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.WriteFile(path, b, 0o644)) //nolint:gosec // test-mode fixture, read by the server
}

var pageShades = []uint8{80, 120, 160, 200}

// writeMinimalCBZ writes a CBZ of solid PNG pages, each a different shade so
// a screenshot shows where one page ends.
func writeMinimalCBZ(path string, pages pageSpec) (err error) {
	out, err := os.Create(path) //nolint:gosec // test-mode fixture, read by the server
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	archive := zip.NewWriter(out)
	for i := range pages.count {
		w, err := archive.Create(fmt.Sprintf("page%03d.png", i+1))
		if err != nil {
			return errors.WithStack(err)
		}
		img := image.NewGray(image.Rect(0, 0, pages.width, pages.height))
		shade := pageShades[i%len(pageShades)]
		for p := range img.Pix {
			img.Pix[p] = shade
		}
		if err := png.Encode(w, img); err != nil {
			return errors.WithStack(err)
		}
	}
	return errors.WithStack(archive.Close())
}

// tempFilePath returns the path, without extension, for a new file in a fresh
// directory under root. Path separators in the title are replaced so the file
// lands in that directory.
func tempFilePath(root, title string) (string, error) {
	if root == "" {
		return "", errors.New("no directory configured for withFileOnDisk")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", errors.WithStack(err)
	}
	dir, err := os.MkdirTemp(root, "book-")
	if err != nil {
		return "", errors.WithStack(err)
	}
	name := strings.NewReplacer("/", "_", string(filepath.Separator), "_").Replace(title)
	return filepath.Join(dir, name), nil
}
