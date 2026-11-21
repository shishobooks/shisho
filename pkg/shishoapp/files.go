package shishoapp

import (
	"database/sql"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// requireFile loads the :id File with what its download's name is made of.
// A File outside the user's Libraries, or missing from disk, is a 404,
// never a 403, so its id reveals nothing.
func (h *handler) requireFile(c echo.Context) (*models.File, error) {
	ctx := c.Request().Context()
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return nil, err
	}
	file := new(models.File)
	err = h.db.NewSelect().Model(file).
		Relation("Book").
		Relation("Book.Authors", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("a.sort_order ASC")
		}).
		Relation("Book.Authors.Person").
		Relation("Book.BookSeries", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("bs.sort_order ASC")
		}).
		Relation("Book.BookSeries.Series").
		Relation("Narrators", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("n.sort_order ASC")
		}).
		Relation("Narrators.Person").
		Where("f.id = ?", id).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcodes.NotFound("File")
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !user.HasLibraryAccess(file.LibraryID) || file.Book == nil {
		return nil, errcodes.NotFound("File")
	}
	if err := books.RequireFileOnDisk(c, file, "File"); err != nil {
		return nil, err
	}
	return file, nil
}

// download serves an Original File as it sits in the library (ADR 0022),
// with Range support, so an interrupted download resumes.
func (h *handler) download(c echo.Context) error {
	file, err := h.requireFile(c)
	if err != nil {
		return err
	}
	return books.OriginalDownload(file.Book, file).Serve(c)
}

// stream serves an audiobook File for playback, with Range support so the
// player can seek. Only audiobooks stream; anything else downloads.
func (h *handler) stream(c echo.Context) error {
	file, err := h.requireFile(c)
	if err != nil {
		return err
	}
	if !models.IsAudiobookFileType(file.FileType) {
		return errcodes.NotFound("File")
	}
	return httputil.ServeFile(c, file.Filepath, errcodes.NotFound("File"),
		httputil.WithContentType(models.FileTypeMimeType(file.FileType)),
		httputil.WithCacheControl("private, no-store"))
}
