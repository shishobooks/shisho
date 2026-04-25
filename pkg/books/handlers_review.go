package books

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// SetReviewPayload is the request body for PATCH /books/files/:id/review.
type SetReviewPayload struct {
	Override *string `json:"override" validate:"omitempty,oneof=reviewed unreviewed"`
}

// setFileReview sets or clears the review override for a single file.
func (h *handler) setFileReview(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.BadRequest("Invalid file id")
	}

	var payload SetReviewPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	ctx := c.Request().Context()
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &id})
	if err != nil {
		return err
	}
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}
	if err := review.SetOverride(ctx, h.bookService.DB(), id, payload.Override, criteria); err != nil {
		return err
	}

	updated, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &id})
	if err != nil {
		return err
	}
	return c.JSON(200, updated)
}
