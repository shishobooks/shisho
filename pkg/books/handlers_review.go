package books

import (
	"net/http"
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

// BulkSetReviewPayload is the request body for POST /books/bulk/review.
type BulkSetReviewPayload struct {
	BookIDs  []int   `json:"book_ids" validate:"required,min=1,max=500"`
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
	// Supplements never participate in review state — reject overrides
	// rather than silently persisting orphan rows.
	if file.FileRole != models.FileRoleMain {
		return errcodes.BadRequest("Cannot set review state on a supplement file")
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
	return c.JSON(http.StatusOK, updated)
}

// setBookReview cascades a review override to all main files of a book.
func (h *handler) setBookReview(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.BadRequest("Invalid book id")
	}

	var payload SetReviewPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	ctx := c.Request().Context()
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return err
	}
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}
	for _, f := range book.Files {
		if f.FileRole != models.FileRoleMain {
			continue
		}
		if err := review.SetOverride(ctx, h.bookService.DB(), f.ID, payload.Override, criteria); err != nil {
			return err
		}
	}

	updated, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, updated)
}

// bulkSetReview applies a review override to all main files across multiple books.
func (h *handler) bulkSetReview(c echo.Context) error {
	var payload BulkSetReviewPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, _ := c.Get("user").(*models.User)

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}

	for _, bookID := range payload.BookIDs {
		id := bookID
		book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
		if err != nil {
			continue // silently skip missing books
		}
		if user != nil && !user.HasLibraryAccess(book.LibraryID) {
			continue
		}
		for _, f := range book.Files {
			if f.FileRole != models.FileRoleMain {
				continue
			}
			if err := review.SetOverride(ctx, h.bookService.DB(), f.ID, payload.Override, criteria); err != nil {
				return err
			}
		}
	}
	return c.NoContent(http.StatusNoContent)
}
