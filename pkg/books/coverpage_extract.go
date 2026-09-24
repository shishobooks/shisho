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
// cover file alongside the book. Returns the cover filename (not path), the
// MIME type, and the previous cover files with the same base name at other
// extensions. The new cover is installed atomically and nothing is deleted
// here: callers remove the stale files only after their database write
// succeeds, so a failed write never leaves the row naming a deleted file and
// a failed install leaves the previous cover readable on disk.
//
// Callers are responsible for updating the file's CoverPage, CoverImageFilename,
// CoverMimeType, and CoverSource fields on the model and persisting them.
func ExtractCoverPageToFile(
	file *models.File,
	bookFilepath string,
	page int,
	cbzCache *cbzpages.Cache,
	pdfCache *pdfpages.Cache,
) (filename string, mimeType string, stale []string, err error) {
	var cachedPath string
	switch file.FileType {
	case models.FileTypeCBZ:
		cachedPath, mimeType, err = cbzCache.GetPage(file.Filepath, file.ID, page)
	case models.FileTypePDF:
		cachedPath, mimeType, err = pdfCache.GetPage(file.Filepath, file.ID, page)
	default:
		return "", "", nil, errors.Errorf("file type %q does not support page-based covers", file.FileType)
	}
	if err != nil {
		return "", "", nil, errors.Wrap(err, "failed to extract cover page")
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
		return "", "", nil, errors.Wrap(err, "failed to read extracted cover page")
	}
	if err := fileutils.WriteFileAtomic(coverFilepath, pageData, 0644); err != nil {
		return "", "", nil, errors.Wrap(err, "failed to save cover image")
	}

	return coverFilename, mimeType, fileutils.OtherCoverExtensions(coverDir, coverBaseName, ext), nil
}

// RemoveStaleCovers deletes the cover files an extractor reported as
// superseded. Call it only after the database write that references the
// replacement has succeeded.
func RemoveStaleCovers(stale []string, log logger.Logger) {
	for _, path := range stale {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Warn("failed to remove stale cover", logger.Data{"path": path, "error": err.Error()})
		}
	}
}
