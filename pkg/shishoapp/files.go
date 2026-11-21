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

// requireFile loads the :id File, and with named, what its download's name
// is made of: a stream asks for many ranges and names nothing. A File
// outside the user's Libraries, or missing from disk, is a 404, never a 403,
// so its id reveals nothing.
func (h *handler) requireFile(c echo.Context, named bool) (*models.File, error) {
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
	q := h.db.NewSelect().Model(file).Where("f.id = ?", id)
	if named {
		q = withDownloadName(q)
	}
	err = q.Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcodes.NotFound("File")
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !user.HasLibraryAccess(file.LibraryID) || named && file.Book == nil {
		return nil, errcodes.NotFound("File")
	}
	if err := books.RequireFileOnDisk(c, file, "File"); err != nil {
		return nil, err
	}
	return file, nil
}

// withDownloadName loads what an Original File's download name is made of.
func withDownloadName(q *bun.SelectQuery) *bun.SelectQuery {
	return q.
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
		Relation("Narrators.Person")
}

// streamCookies takes the credential and protocol a stream request carries
// as cookies (CookieCredential, CookieProtocol), when it carries no
// headers: the system player makes its own requests and sends only the
// cookies the app hands it. Headers, when present, win.
func streamCookies(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header
		if header.Get(echo.HeaderAuthorization) == "" {
			if cookie, err := c.Cookie(CookieCredential); err == nil && cookie.Value != "" {
				header.Set(echo.HeaderAuthorization, "Bearer "+cookie.Value)
			}
		}
		if header.Get(HeaderAppProtocol) == "" {
			if cookie, err := c.Cookie(CookieProtocol); err == nil {
				header.Set(HeaderAppProtocol, cookie.Value)
			}
		}
		return next(c)
	}
}

// download serves an Original File as it sits in the library (ADR 0022),
// with Range support, so an interrupted download resumes.
func (h *handler) download(c echo.Context) error {
	file, err := h.requireFile(c, true)
	if err != nil {
		return err
	}
	return books.OriginalDownload(file.Book, file).Serve(c)
}

// stream serves an audiobook File for playback, with Range support so the
// player can seek. Only audiobooks stream; anything else downloads.
func (h *handler) stream(c echo.Context) error {
	file, err := h.requireFile(c, false)
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
