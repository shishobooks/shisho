package books

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

// updateFileCoverPagePayload is the request body for setting a CBZ cover page.
type updateFileCoverPagePayload struct {
	Page int `json:"page"` // 0-indexed page number
}

// updateFileCoverPage handles PUT /files/:id/cover-page
// Sets the cover page for a CBZ file and extracts it as an external cover image.
func (h *handler) updateFileCoverPage(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	var payload updateFileCoverPagePayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError("Invalid request body")
	}

	// Fetch the file with book relation
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Validate file type is CBZ
	if file.FileType != models.FileTypeCBZ {
		return errcodes.ValidationError("Cover page selection is only available for CBZ files")
	}

	// Validate page is within bounds
	if file.PageCount == nil || payload.Page < 0 || payload.Page >= *file.PageCount {
		return errcodes.ValidationError("Page number is out of bounds")
	}

	// Extract the page image using the page cache
	cachedPath, mimeType, err := h.pageCache.GetPage(file.Filepath, file.ID, payload.Page)
	if err != nil {
		log.Error("failed to extract page from CBZ", logger.Data{"error": err.Error(), "page": payload.Page})
		return errcodes.ValidationError("Failed to extract page from CBZ file")
	}

	// Determine cover directory (same logic as fileCover and uploadFileCover)
	// Use file.Book which is already loaded by RetrieveFile
	isRootLevelBook := false
	if info, err := os.Stat(file.Book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}
	var coverDir string
	if isRootLevelBook {
		coverDir = filepath.Dir(file.Book.Filepath)
	} else {
		coverDir = file.Book.Filepath
	}

	// Generate the cover filename: {filename}.cover.{ext}
	filename := filepath.Base(file.Filepath)
	coverBaseName := filename + ".cover"

	// Get extension from mime type
	ext := getExtensionFromMimeType(mimeType)
	if ext == "" {
		ext = filepath.Ext(cachedPath)
	}

	// Delete any existing cover with this base name (regardless of extension)
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, err := os.Stat(existingPath); err == nil {
			if err := os.Remove(existingPath); err != nil {
				log.Warn("failed to remove existing cover", logger.Data{"path": existingPath, "error": err.Error()})
			}
		}
	}

	// Copy the extracted page to the cover location
	coverFilePath := filepath.Join(coverDir, coverBaseName+ext)
	if err := copyFile(cachedPath, coverFilePath); err != nil {
		log.Error("failed to copy cover image", logger.Data{"error": err.Error()})
		return errcodes.ValidationError("Failed to save cover image")
	}

	log.Info("set CBZ cover page", logger.Data{
		"file_id":    file.ID,
		"page":       payload.Page,
		"cover_path": coverFilePath,
		"mime_type":  mimeType,
	})

	// Update file's cover metadata
	coverFilename := coverBaseName + ext
	file.CoverPage = &payload.Page
	file.CoverMimeType = &mimeType
	file.CoverSource = strPtr(models.DataSourceManual)
	file.CoverImageFilename = &coverFilename

	if err := h.bookService.UpdateFile(ctx, file, UpdateFileOptions{
		Columns: []string{"cover_page", "cover_mime_type", "cover_source", "cover_image_filename"},
	}); err != nil {
		return errors.WithStack(err)
	}

	// Write sidecar to persist the cover page choice
	if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
		log.Warn("failed to write file sidecar", logger.Data{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, file)
}

// copyFile copies a file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return errors.WithStack(err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return errors.WithStack(err)
	}

	// Copy file permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	if err := dstFile.Chmod(srcInfo.Mode()); err != nil {
		return errors.WithStack(err)
	}

	// Sync to ensure data is written to disk
	if err := dstFile.Sync(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
