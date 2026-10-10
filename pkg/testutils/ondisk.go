package testutils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/mobigen"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// writeFileOnDisk writes a small valid file of fileType for withFileOnDisk
// and returns its path without the extension, as books store it. Only the
// types the E2E tests open have a writer.
func writeFileOnDisk(root, fileType, title string) (string, error) {
	var write func(path, title string) error
	switch fileType {
	case models.FileTypeEPUB:
		write = writeMinimalEPUB
	case models.FileTypeMOBI:
		write = writeMinimalMOBI
	default:
		return "", errcodes.ValidationError("withFileOnDisk requires fileType epub or mobi")
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
