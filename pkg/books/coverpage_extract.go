package books

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
)

// ExtractCoverPageToFile renders `page` from the given page-based file (CBZ or
// PDF) via the appropriate page cache and writes the rendered image as the
// cover file alongside the book. Returns the cover filename (not path) and
// MIME type. Any existing cover image with the same base name is removed first
// regardless of extension.
//
// Callers are responsible for updating the file's CoverPage, CoverImageFilename,
// CoverMimeType, and CoverSource fields on the model and persisting them.
func ExtractCoverPageToFile(
	file *models.File,
	bookFilepath string,
	page int,
	cbzCache *cbzpages.Cache,
	pdfCache *pdfpages.Cache,
	log logger.Logger,
) (filename string, mimeType string, err error) {
	var cachedPath string
	switch file.FileType {
	case models.FileTypeCBZ:
		cachedPath, mimeType, err = cbzCache.GetPage(file.Filepath, file.ID, page)
	case models.FileTypePDF:
		cachedPath, mimeType, err = pdfCache.GetPage(file.Filepath, file.ID, page)
	default:
		return "", "", errors.Errorf("file type %q does not support page-based covers", file.FileType)
	}
	if err != nil {
		return "", "", errors.Wrap(err, "failed to extract cover page")
	}

	// Use the write-side resolver so root-level files (whose bookFilepath may
	// be a synthetic organized-folder path that doesn't yet exist on disk)
	// land the cover next to the file instead of failing on a stale path.
	coverDir := fileutils.ResolveCoverDirForWrite(bookFilepath, file.Filepath)
	coverBaseName := filepath.Base(file.Filepath) + ".cover"

	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = filepath.Ext(cachedPath)
	}

	// Delete any existing cover with this base name (regardless of extension).
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, err := os.Stat(existingPath); err == nil {
			if err := os.Remove(existingPath); err != nil {
				log.Warn("failed to remove existing cover", logger.Data{"path": existingPath, "error": err.Error()})
			}
		}
	}

	coverFilename := coverBaseName + ext
	coverFilepath := filepath.Join(coverDir, coverFilename)
	if err := copyFile(cachedPath, coverFilepath); err != nil {
		return "", "", errors.Wrap(err, "failed to save cover image")
	}

	return coverFilename, mimeType, nil
}
