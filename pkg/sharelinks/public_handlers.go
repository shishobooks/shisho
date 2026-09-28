package sharelinks

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

// publicHandler serves the unauthenticated /share/:token family. It has its
// own handlers rather than mounting the authenticated books handlers, which
// skip the library access check when no user is in context. Every endpoint
// goes through resolve first.
type publicHandler struct {
	service            *Service
	bookService        *books.Service
	appSettingsService *appsettings.Service
	downloadCache      *downloadcache.Cache
}

// errUnavailable is the one response for a token that cannot be used, so a
// recipient cannot tell an expired link from a mistyped one.
func errUnavailable() error {
	return errcodes.NotFound("Share Link")
}

// resolve decides whether the token in the path is usable and returns the
// link and its book. A link resolves only while sharing is enabled, the
// token exists, the link is active (not revoked, not expired), and its
// creator is active and can still reach the book's library. A deleted
// creator or book takes the row with it. Every failure returns
// errUnavailable.
func (h *publicHandler) resolve(c echo.Context) (*models.ShareLink, *models.Book, error) {
	ctx := c.Request().Context()
	token := c.Param("token")
	if !wellFormedToken(token) {
		return nil, nil, errUnavailable()
	}

	settings, err := LoadSettings(ctx, h.appSettingsService)
	if err != nil {
		return nil, nil, err
	}
	if !settings.Enabled {
		return nil, nil, errUnavailable()
	}

	link, err := h.service.RetrieveByToken(ctx, token)
	if errors.Is(err, errcodes.NotFound("Share Link")) {
		return nil, nil, errUnavailable()
	}
	if err != nil {
		return nil, nil, err
	}
	creator := link.CreatedByUser
	if link.State(time.Now()) != models.ShareLinkStateActive || creator == nil || !creator.IsActive {
		return nil, nil, errUnavailable()
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &link.BookID})
	if errors.Is(err, errcodes.NotFound("Book")) {
		return nil, nil, errUnavailable()
	}
	if err != nil {
		return nil, nil, err
	}
	if !creator.HasLibraryAccess(book.LibraryID) {
		return nil, nil, errUnavailable()
	}
	return link, book, nil
}

// resolveFile resolves the token and returns the requested file, which must
// belong to the link's book.
func (h *publicHandler) resolveFile(c echo.Context) (*models.ShareLink, *models.Book, *models.File, error) {
	link, book, err := h.resolve(c)
	if err != nil {
		return nil, nil, nil, err
	}
	fileID, err := strconv.Atoi(c.Param("fileId"))
	if err != nil {
		return nil, nil, nil, errcodes.NotFound("File")
	}
	for _, f := range book.Files {
		if f.ID == fileID {
			return link, book, f, nil
		}
	}
	return nil, nil, nil, errcodes.NotFound("File")
}

// countAccess applies a usage count. A failed count is logged rather than
// returned, so the recipient still gets the page or the file.
func countAccess(ctx context.Context, link *models.ShareLink, record func(context.Context, int) error) {
	if err := record(ctx, link.ID); err != nil {
		logger.FromContext(ctx).Warn("failed to record share link access", logger.Data{
			"share_link_id": link.ID,
			"error":         err.Error(),
		})
	}
}

func coverAspectRatio(book *models.Book) string {
	if book.Library == nil {
		return ""
	}
	return book.Library.CoverAspectRatio
}

func (h *publicHandler) book(c echo.Context) error {
	link, book, err := h.resolve(c)
	if err != nil {
		return err
	}
	countAccess(c.Request().Context(), link, h.service.RecordOpen)
	aspectRatio := coverAspectRatio(book)
	// The cache key reads cover filenames, so compute it before blanking.
	book.CoverCacheKey = covers.CacheKey(book.Files, aspectRatio)
	blankForRecipient(book)

	return errors.WithStack(c.JSON(http.StatusOK, SharedBookResponse{
		Book:             *book,
		SharedBy:         link.CreatedByUser.Username,
		ExpiresAt:        link.ExpiresAt,
		CoverAspectRatio: aspectRatio,
	}))
}

// blankForRecipient removes everything that describes the server's layout:
// filesystem paths (a supplement keeps its filename as its display name),
// cover filenames, scan errors (which can quote paths), and the library. It
// also drops the library-facing fields the recipient page hides (sort title,
// file URLs, file identifiers). Other fields the page does not render
// (timestamps, sources, review flags) stay; timestamps are needed because
// file cover URLs use updated_at as their cache key. The downloaded file
// itself still carries the book's full metadata.
func blankForRecipient(book *models.Book) {
	book.Filepath = ""
	book.Library = nil
	book.SortTitle = ""
	for _, f := range book.Files {
		// A supplement's label is its filename, so resolve it while the path
		// is still here. A main file is labeled only by its stored name: one
		// without a name gets no label rather than its on-disk filename, and
		// the page shows its type.
		if f.FileRole == models.FileRoleSupplement {
			f.DisplayName = f.ResolveDisplayName()
		} else {
			f.DisplayName = ""
			if f.Name != nil {
				f.DisplayName = *f.Name
			}
		}
		f.Filepath = ""
		f.CoverImageFilename = nil
		f.ScanError = nil
		f.Book = nil
		f.URL = nil
		f.Identifiers = nil
	}
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			bs.Series.CoverImageFilename = nil
		}
	}
}

func (h *publicHandler) bookCover(c echo.Context) error {
	_, book, err := h.resolve(c)
	if err != nil {
		return err
	}
	return covers.ServeBookCover(c, book.Files, coverAspectRatio(book), covers.CacheControlImmutable, "Cover")
}

func (h *publicHandler) fileCover(c echo.Context) error {
	_, _, file, err := h.resolveFile(c)
	if err != nil {
		return err
	}
	if file.FileRole == models.FileRoleSupplement {
		return errcodes.NotFound("Cover")
	}
	coverPath := covers.FileCoverPath(file)
	if _, err := os.Stat(coverPath); err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
	}
	c.Response().Header().Set("Cache-Control", covers.CacheControlImmutable)
	return errors.WithStack(c.File(coverPath))
}

// download serves the generated file with metadata injected, the same file
// the authenticated download route serves. Supplements and formats with no
// generator download as-is. Original and KePub variants are not offered.
func (h *publicHandler) download(c echo.Context) error {
	ctx := c.Request().Context()
	link, book, file, err := h.resolveFile(c)
	if err != nil {
		return err
	}
	if err := books.RequireFileOnDisk(c, file, "File"); err != nil {
		return err
	}

	path, filename, err := h.downloadFile(ctx, book, file)
	if err != nil {
		return err
	}
	if startsDownload(c.Request()) {
		countAccess(ctx, link, h.service.RecordDownload)
	}
	httputil.SetAttachmentFilename(c.Response(), filename)
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return errors.WithStack(c.File(path))
}

// downloadFile returns the path to serve and its download filename: the
// generated file, or the original for supplements, formats with no generator
// (those only a plugin parses), and a failed generation.
func (h *publicHandler) downloadFile(ctx context.Context, book *models.Book, file *models.File) (string, string, error) {
	original := func() (string, string, error) {
		return file.Filepath, filepath.Base(file.Filepath), nil
	}
	if file.FileRole == models.FileRoleSupplement {
		return original()
	}
	if _, err := filegen.GetGenerator(file.FileType); err != nil {
		return original()
	}

	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerate(ctx, book, file)
	if err != nil {
		var genErr *filegen.GenerationError
		if !errors.As(err, &genErr) {
			return "", "", errors.WithStack(err)
		}
		// A recipient has no Download Original to fall back on, so serve the
		// original rather than an error, as the eReader download does.
		logger.FromContext(ctx).Warn("share link file generation failed, serving original", logger.Data{
			"file_id":   file.ID,
			"file_type": file.FileType,
			"error":     genErr.Message,
		})
		return original()
	}
	return cachedPath, downloadFilename, nil
}

// startsDownload reports whether a download request counts as a download. HEAD
// only checks the file, and a range past the first byte continues a download
// already counted (a resumed transfer or a download manager's later segment).
func startsDownload(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	rng := r.Header.Get("Range")
	return rng == "" || strings.HasPrefix(rng, "bytes=0-")
}
