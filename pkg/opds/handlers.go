package opds

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sortspec"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type handler struct {
	opdsService     *Service
	bookService     *books.Service
	libraryService  *libraries.Service
	listsService    *lists.Service
	downloadCache   *downloadcache.Cache
	settingsService *settings.Service
	deliveryService *delivery.Service
}

// resolveSort resolves user's stored sort preference for libraryID,
// falling back to sortspec.BuiltinDefault when no
// preference exists. OPDS is read-only; there's no explicit ?sort=
// input, so we pass explicit=nil to ResolveForLibrary.
//
// The books service also falls back to BuiltinDefault when Sort is nil,
// so returning BuiltinDefault here is belt-and-suspenders: it keeps the
// OPDS surface explicit about what sort it applies (useful when
// tracing requests) and insulates OPDS from a future change to the
// service's default. Returning the resolved slice directly also lets
// callers log or cache it without re-resolving.
func (h *handler) resolveSort(ctx context.Context, user *models.User, libraryID int) []sortspec.SortLevel {
	resolved := sortspec.ResolveForLibrary(ctx, h.settingsService, user.ID, libraryID, nil)
	if resolved == nil {
		return sortspec.BuiltinDefault()
	}
	return resolved
}

// getBaseURL returns the base URL for OPDS feeds.
func getBaseURL(c echo.Context) string {
	// X-Forwarded-Prefix is set by reverse proxies that strip path prefixes.
	prefix := c.Request().Header.Get("X-Forwarded-Prefix")
	return httputil.PublicOrigin(c.Request()) + prefix + "/opds/v1"
}

// requireSearchQuery returns the q query param both search feeds need, or a
// 422 when it is empty.
func requireSearchQuery(c echo.Context) (string, error) {
	query := c.QueryParam("q")
	if query == "" {
		return "", errcodes.ValidationError("Search query is required")
	}
	return query, nil
}

// getPaginationParams extracts limit and offset from query params.
func getPaginationParams(c echo.Context) (int, int) {
	limit := defaultLimit
	offset := 0

	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return limit, offset
}

// rulesFor returns the Delivery Rules a request's feeds and downloads
// follow: the signed-in user's default.
func (h *handler) rulesFor(c echo.Context) ([]delivery.Rule, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	rules, err := h.deliveryService.UserDefault(c.Request().Context(), user.ID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return rules.Rules, nil
}

// catalog handles the root catalog feed (lists libraries).
func (h *handler) catalog(c echo.Context) error {
	ctx := c.Request().Context()
	baseURL := getBaseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildCatalogFeed(ctx, baseURL, rules, user.ID, user.GetAccessibleLibraryIDs())
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryCatalog handles the library catalog feed.
func (h *handler) libraryCatalog(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	feed, err := h.opdsService.BuildLibraryCatalogFeed(ctx, baseURL, libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAllBooks handles the all books feed for a library.
func (h *handler) libraryAllBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildLibraryAllBooksFeed(ctx, baseURL, rules, libraryID, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySeriesList handles the series list feed for a library.
func (h *handler) librarySeriesList(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildLibrarySeriesListFeed(ctx, baseURL, rules, libraryID, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySeriesBooks handles the books in a series feed for a library.
func (h *handler) librarySeriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	seriesID, err := httputil.ParamID(c, "seriesID", "Series")
	if err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildLibrarySeriesBooksFeed(ctx, baseURL, rules, libraryID, seriesID, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAuthorsList handles the authors list feed for a library.
func (h *handler) libraryAuthorsList(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildLibraryAuthorsListFeed(ctx, baseURL, rules, libraryID, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAuthorBooks handles the books by author feed for a library.
func (h *handler) libraryAuthorBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	authorName, err := url.PathUnescape(c.Param("authorName"))
	if err != nil {
		return errcodes.NotFound("Author")
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildLibraryAuthorBooksFeed(ctx, baseURL, rules, libraryID, authorName, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySearch handles the search feed for a library.
func (h *handler) librarySearch(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	query, err := requireSearchQuery(c)
	if err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildLibrarySearchFeed(ctx, baseURL, rules, libraryID, query, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryOpenSearch handles the OpenSearch description for a library.
func (h *handler) libraryOpenSearch(c echo.Context) error {
	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	desc := h.opdsService.BuildLibraryOpenSearchDescription(baseURL, libraryID)

	c.Response().Header().Set(echo.HeaderContentType, MimeTypeOpenSearch)
	return c.XML(http.StatusOK, desc)
}

// download serves the File named by the file param, "{id}.{extension}",
// delivered as the File Type the extension names, or as its own File Type
// for a bare id. Delivered as its own type, a failed generation sends the
// Original File; delivered through a Conversion, it is an error, per
// books.ResolveDeliveryDownload.
func (h *handler) download(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, deliveredType, err := parseDownloadParam(c.Param("file"))
	if err != nil {
		return err
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		ID: &fileID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	if deliveredType == "" {
		deliveredType = file.FileType
	}
	// A supplement goes only as itself, and no link names a File Type its
	// File can't become.
	if deliveredType != file.FileType {
		if _, ok := models.ConversionChain(file.FileType, deliveredType); !ok || file.FileRole == models.FileRoleSupplement {
			return errcodes.InvalidState(books.NoConversionMessage(file.FileType, deliveredType))
		}
	}

	// Check if source file exists
	if err := books.RequireFileOnDisk(c, file, "File"); err != nil {
		return err
	}

	// Get the full book with relations for generation
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Find the file with all relations from the book's files (includes identifiers for fingerprinting)
	fileWithRelations := file
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}

	download, err := books.ResolveDeliveryDownload(c, h.downloadCache, book, fileWithRelations, deliveredType)
	if err != nil {
		return err
	}
	return download.Serve(c)
}

// parseDownloadParam splits a download route's file param, "{id}" or
// "{id}.{extension}" (DownloadURL), into the File's id and the delivered
// File Type its extension names ("" for none). An id that does not parse
// or an extension no File Type has is a 404.
func parseDownloadParam(param string) (int, string, error) {
	idPart, ext, hasExt := strings.Cut(param, ".")
	id, err := strconv.Atoi(idPart)
	if err != nil || id <= 0 {
		return 0, "", errcodes.NotFound("File")
	}
	if !hasExt {
		return id, "", nil
	}
	deliveredType, ok := models.DeliveredFileTypeForExtension(ext)
	if !ok {
		return 0, "", errcodes.NotFound("File")
	}
	return id, deliveredType, nil
}

// bookCover serves a book's cover image. Mirrors `pkg/books/handlers.go`
// `bookCover` but lives under the root OPDS group so it accepts Basic Auth
// rather than the API's session authentication.
func (h *handler) bookCover(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &book.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return covers.ServeBookCover(c, book.Files, library.CoverAspectRatio, covers.CacheControlNoCache, "Cover")
}

// isKOReader returns true when the request comes from KOReader's OPDS client.
func isKOReader(c echo.Context) bool {
	return strings.Contains(c.Request().UserAgent(), "KOReader")
}

// truncateFeedAuthors keeps only the first author per entry. KOReader's
// XML parser treats <author> as a scalar (last element wins), so
// multiple elements cause it to show only the last author.
func truncateFeedAuthors(feed *Feed) {
	for i := range feed.Entries {
		if len(feed.Entries[i].Authors) > 1 {
			feed.Entries[i].Authors = feed.Entries[i].Authors[:1]
		}
	}
}

// lists handles the feed of the user's own and shared lists.
func (h *handler) lists(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildListsFeed(c.Request().Context(), getBaseURL(c), rules, user.ID, user.GetAccessibleLibraryIDs(), limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// listBooks handles the books feed of a list the user can view.
func (h *handler) listBooks(c echo.Context) error {
	return h.serveListBooks(c, "")
}

// listSearch handles search within a list.
func (h *handler) listSearch(c echo.Context) error {
	query, err := requireSearchQuery(c)
	if err != nil {
		return err
	}
	return h.serveListBooks(c, query)
}

// listOpenSearch handles the OpenSearch description for a list.
func (h *handler) listOpenSearch(c echo.Context) error {
	listID, _, err := h.requireViewableList(c)
	if err != nil {
		return err
	}

	desc := h.opdsService.BuildListOpenSearchDescription(getBaseURL(c), listID)

	c.Response().Header().Set(echo.HeaderContentType, MimeTypeOpenSearch)
	return c.XML(http.StatusOK, desc)
}

// requireViewableList returns the listID param and the user, or a 404 when
// the user cannot view the list, so the id does not confirm the list exists.
func (h *handler) requireViewableList(c echo.Context) (int, *models.User, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return 0, nil, err
	}

	listID, err := httputil.ParamID(c, "listID", "List")
	if err != nil {
		return 0, nil, err
	}

	canView, err := h.listsService.CanView(c.Request().Context(), listID, user.ID)
	if err != nil {
		return 0, nil, errors.WithStack(err)
	}
	if !canView {
		return 0, nil, errcodes.NotFound("List")
	}

	return listID, user, nil
}

func (h *handler) serveListBooks(c echo.Context, query string) error {
	listID, user, err := h.requireViewableList(c)
	if err != nil {
		return err
	}
	rules, err := h.rulesFor(c)
	if err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	feed, err := h.opdsService.BuildListBooksFeed(c.Request().Context(), getBaseURL(c), rules, listID, user.GetAccessibleLibraryIDs(), query, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// respondXML sends an XML response with the correct content type.
func respondXML(c echo.Context, data interface{}) error {
	if feed, ok := data.(*Feed); ok && isKOReader(c) {
		truncateFeedAuthors(feed)
	}

	c.Response().Header().Set(echo.HeaderContentType, MimeTypeAtom+"; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)

	// Write XML declaration
	if _, err := c.Response().Write([]byte(xml.Header)); err != nil {
		return errors.WithStack(err)
	}

	// Encode the feed
	encoder := xml.NewEncoder(c.Response())
	encoder.Indent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
