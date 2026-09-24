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
// MIME type. The new cover is installed atomically before any existing cover
// image with the same base name and a different extension is removed, so a
// failed write leaves the previous cover readable on disk.
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

	coverFilename := coverBaseName + ext
	coverFilepath := filepath.Join(coverDir, coverFilename)
	pageData, err := os.ReadFile(cachedPath)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read extracted cover page")
	}
	if err := fileutils.WriteFileAtomic(coverFilepath, pageData, 0644); err != nil {
		return "", "", errors.Wrap(err, "failed to save cover image")
	}

	// Only now that the replacement is in place, remove previous covers with
	// this base name at other extensions.
	for _, existingExt := range fileutils.CoverImageExtensions {
		if existingExt == ext {
			continue
		}
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, err := os.Stat(existingPath); err == nil {
			if err := os.Remove(existingPath); err != nil {
				log.Warn("failed to remove existing cover", logger.Data{"path": existingPath, "error": err.Error()})
			}
		}
	}

	return coverFilename, mimeType, nil
}
