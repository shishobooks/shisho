package chapters

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

type handler struct {
	chapterService *Service
	bookService    *books.Service
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Verify file exists and check access
	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	chapters, err := h.chapterService.ListChapters(ctx, fileID)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, map[string]any{
		"chapters": chapters,
	}))
}

func (h *handler) replace(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Bind payload
	var payload ReplaceChaptersPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Verify file exists and check access
	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Validate chapters based on file type
	if err := validateChapters(file, payload.Chapters); err != nil {
		return err
	}

	// Convert input to ParsedChapter
	chapters := convertInputToChapters(payload.Chapters)

	// Replace chapters
	if err := h.chapterService.ReplaceChapters(ctx, fileID, chapters); err != nil {
		return errors.WithStack(err)
	}

	// Return updated chapters
	updatedChapters, err := h.chapterService.ListChapters(ctx, fileID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Write sidecar file with updated chapters
	fileWithRelations, err := h.bookService.RetrieveFileWithRelations(ctx, fileID)
	if err == nil {
		// Best effort - don't fail the request if sidecar write fails
		_ = sidecar.WriteFileSidecarWithChapters(fileWithRelations, updatedChapters)
	}

	return errors.WithStack(c.JSON(http.StatusOK, map[string]any{
		"chapters": updatedChapters,
	}))
}

// validateChapters validates chapter data against file constraints.
func validateChapters(file *models.File, chapters []ChapterInput) error {
	for _, ch := range chapters {
		switch file.FileType {
		case models.FileTypeCBZ:
			if ch.StartPage != nil && file.PageCount != nil {
				if *ch.StartPage >= *file.PageCount {
					return errcodes.ValidationError("start_page must be less than page_count")
				}
			}
		case models.FileTypeM4B:
			if ch.StartTimestampMs != nil && file.AudiobookDurationSeconds != nil {
				maxMs := int64(*file.AudiobookDurationSeconds * 1000)
				if *ch.StartTimestampMs > maxMs {
					return errcodes.ValidationError("start_timestamp_ms exceeds file duration")
				}
			}
		}

		// Validate children recursively
		if len(ch.Children) > 0 {
			if err := validateChapters(file, ch.Children); err != nil {
				return err
			}
		}
	}
	return nil
}

// convertInputToChapters converts ChapterInput slice to ParsedChapter slice.
func convertInputToChapters(inputs []ChapterInput) []mediafile.ParsedChapter {
	chapters := make([]mediafile.ParsedChapter, 0, len(inputs))
	for _, in := range inputs {
		ch := mediafile.ParsedChapter{
			Title:            in.Title,
			StartPage:        in.StartPage,
			StartTimestampMs: in.StartTimestampMs,
			Href:             in.Href,
		}
		if len(in.Children) > 0 {
			ch.Children = convertInputToChapters(in.Children)
		}
		chapters = append(chapters, ch)
	}
	return chapters
}
