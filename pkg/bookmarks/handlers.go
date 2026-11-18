package bookmarks

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	bookmarkService *Service
}

// requireAudiobook returns the File the path names when the user can reach
// it and it's an audiobook, the only kind of File with Bookmarks.
func (h *handler) requireAudiobook(c echo.Context) (int, error) {
	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return 0, err
	}
	target, err := h.bookmarkService.RetrieveFileTarget(c.Request().Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errcodes.NotFound("File")
	}
	if err != nil {
		return 0, err
	}
	if err := auth.RequireLibraryAccessFor(c, target.LibraryID); err != nil {
		return 0, err
	}
	if !models.IsAudiobookFileType(target.FileType) {
		return 0, errcodes.InvalidState("Only audiobooks have bookmarks.")
	}
	return id, nil
}

// requireBookmark returns the signed-in user's Bookmark the path names. A
// Bookmark of another user's is not found, so its existence isn't revealed.
func (h *handler) requireBookmark(c echo.Context, userID int) (*models.Bookmark, error) {
	id, err := httputil.ParamID(c, "id", "Bookmark")
	if err != nil {
		return nil, err
	}
	ctx := c.Request().Context()
	bookmark, err := h.bookmarkService.Retrieve(ctx, userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcodes.NotFound("Bookmark")
	}
	if err != nil {
		return nil, err
	}
	target, err := h.bookmarkService.RetrieveFileTarget(ctx, bookmark.FileID)
	if err != nil {
		// The File's Bookmarks go with it, so it's there.
		return nil, err
	}
	if err := auth.RequireLibraryAccessFor(c, target.LibraryID); err != nil {
		return nil, err
	}
	return bookmark, nil
}

func (h *handler) list(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	fileID, err := h.requireAudiobook(c)
	if err != nil {
		return err
	}
	bookmarks, err := h.bookmarkService.List(c.Request().Context(), user.ID, fileID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, bookmarks))
}

func (h *handler) create(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	fileID, err := h.requireAudiobook(c)
	if err != nil {
		return err
	}
	params := CreateBookmarkPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	bookmark, err := h.bookmarkService.Create(c.Request().Context(), user.ID, fileID, *params.Seconds, params.Note)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusCreated, bookmark))
}

func (h *handler) update(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookmark, err := h.requireBookmark(c, user.ID)
	if err != nil {
		return err
	}
	params := UpdateBookmarkPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := h.bookmarkService.UpdateNote(c.Request().Context(), bookmark, params.Note); err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, bookmark))
}

func (h *handler) delete(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookmark, err := h.requireBookmark(c, user.ID)
	if err != nil {
		return err
	}
	if err := h.bookmarkService.Delete(c.Request().Context(), bookmark); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}
