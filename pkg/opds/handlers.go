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
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/libraries"
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
	downloadCache   *downloadcache.Cache
	settingsService *settings.Service
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
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	// Check for X-Forwarded-Proto header (for reverse proxies)
	if proto := c.Request().Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	// Check for X-Forwarded-Prefix header (set by reverse proxies that strip path prefixes)
	prefix := c.Request().Header.Get("X-Forwarded-Prefix")

	return scheme + "://" + c.Request().Host + prefix + "/opds/v1"
}

// getBaseURLKepub returns the base URL for KePub OPDS feeds.
func getBaseURLKepub(c echo.Context) string {
	return getBaseURL(c) + "/kepub"
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

// validateFileTypes validates the file types parameter.
func validateFileTypes(types string) error {
	if types == "" {
		return errcodes.ValidationError("File types parameter is required")
	}

	for _, t := range parseFileTypes(types) {
		if !models.IsBuiltInFileType(t) {
			return errcodes.ValidationError("Invalid file type: " + t)
		}
	}

	return nil
}

// validateKepubFileTypes validates the file types parameter of a KePub feed.
// The feed skips MOBI and AZW3 (see kepubFileTypes), so it needs at least one
// other type; with none, its book queries would apply no type filter at all.
func validateKepubFileTypes(types string) error {
	if err := validateFileTypes(types); err != nil {
		return err
	}
	if len(kepubFileTypes(types)) == 0 {
		return errcodes.ValidationError("KePub feeds need a file type other than mobi or azw3")
	}
	return nil
}

// accessibleLibraryIDs returns the library IDs the Basic Auth user can
// access, or nil for all of them. With no user in context it returns 401
// rather than nil, which the feed queries would read as every library.
func accessibleLibraryIDs(c echo.Context) ([]int, error) {
	user, err := auth.RequireUser(c)
	if err != nil {
		return nil, err
	}
	return user.GetAccessibleLibraryIDs(), nil
}

// catalog handles the root catalog feed (lists libraries).
func (h *handler) catalog(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	libraryIDs, err := accessibleLibraryIDs(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildCatalogFeed(ctx, baseURL, fileTypes, libraryIDs)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryCatalog handles the library catalog feed.
func (h *handler) libraryCatalog(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	feed, err := h.opdsService.BuildLibraryCatalogFeed(ctx, baseURL, fileTypes, libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAllBooks handles the all books feed for a library.
func (h *handler) libraryAllBooks(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

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

	feed, err := h.opdsService.BuildLibraryAllBooksFeed(ctx, baseURL, fileTypes, libraryID, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySeriesList handles the series list feed for a library.
func (h *handler) librarySeriesList(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	feed, err := h.opdsService.BuildLibrarySeriesListFeed(ctx, baseURL, fileTypes, libraryID, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySeriesBooks handles the books in a series feed for a library.
func (h *handler) librarySeriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

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

	feed, err := h.opdsService.BuildLibrarySeriesBooksFeed(ctx, baseURL, fileTypes, libraryID, seriesID, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAuthorsList handles the authors list feed for a library.
func (h *handler) libraryAuthorsList(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	feed, err := h.opdsService.BuildLibraryAuthorsListFeed(ctx, baseURL, fileTypes, libraryID, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAuthorBooks handles the books by author feed for a library.
func (h *handler) libraryAuthorBooks(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

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

	feed, err := h.opdsService.BuildLibraryAuthorBooksFeed(ctx, baseURL, fileTypes, libraryID, authorName, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySearch handles the search feed for a library.
func (h *handler) librarySearch(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

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

	feed, err := h.opdsService.BuildLibrarySearchFeed(ctx, baseURL, fileTypes, libraryID, query, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryOpenSearch handles the OpenSearch description for a library.
func (h *handler) libraryOpenSearch(c echo.Context) error {
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	desc := h.opdsService.BuildLibraryOpenSearchDescription(baseURL, fileTypes, libraryID)

	c.Response().Header().Set(echo.HeaderContentType, MimeTypeOpenSearch)
	return c.XML(http.StatusOK, desc)
}

// KePub handlers - same as regular handlers but generate feeds with KePub download links.

// catalogKepub handles the root catalog feed (lists libraries) with KePub links.
func (h *handler) catalogKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

	baseURL := getBaseURLKepub(c)
	libraryIDs, err := accessibleLibraryIDs(c)
	if err != nil {
		return err
	}
	feed, err := h.opdsService.BuildCatalogFeed(ctx, baseURL, fileTypes, libraryIDs)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryCatalogKepub handles the library catalog feed with KePub links.
func (h *handler) libraryCatalogKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURLKepub(c)
	feed, err := h.opdsService.BuildLibraryCatalogFeed(ctx, baseURL, fileTypes, libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAllBooksKepub handles the all books feed with KePub links.
func (h *handler) libraryAllBooksKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURLKepub(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	feed, err := h.opdsService.BuildLibraryAllBooksFeedKepub(ctx, baseURL, fileTypes, libraryID, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySeriesListKepub handles the series list feed with KePub links.
func (h *handler) librarySeriesListKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURLKepub(c)

	feed, err := h.opdsService.BuildLibrarySeriesListFeed(ctx, baseURL, fileTypes, libraryID, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySeriesBooksKepub handles the books in a series feed with KePub links.
func (h *handler) librarySeriesBooksKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

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
	baseURL := getBaseURLKepub(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	feed, err := h.opdsService.BuildLibrarySeriesBooksFeedKepub(ctx, baseURL, fileTypes, libraryID, seriesID, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAuthorsListKepub handles the authors list feed with KePub links.
func (h *handler) libraryAuthorsListKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURLKepub(c)

	feed, err := h.opdsService.BuildLibraryAuthorsListFeed(ctx, baseURL, fileTypes, libraryID, limit, offset)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryAuthorBooksKepub handles the books by author feed with KePub links.
func (h *handler) libraryAuthorBooksKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

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
	baseURL := getBaseURLKepub(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	feed, err := h.opdsService.BuildLibraryAuthorBooksFeedKepub(ctx, baseURL, fileTypes, libraryID, authorName, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// librarySearchKepub handles the search feed with KePub links.
func (h *handler) librarySearchKepub(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

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
	baseURL := getBaseURLKepub(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	sort := h.resolveSort(c.Request().Context(), user, libraryID)

	feed, err := h.opdsService.BuildLibrarySearchFeedKepub(ctx, baseURL, fileTypes, libraryID, query, limit, offset, sort)
	if err != nil {
		return errors.WithStack(err)
	}

	return respondXML(c, feed)
}

// libraryOpenSearchKepub handles the OpenSearch description for KePub feeds.
func (h *handler) libraryOpenSearchKepub(c echo.Context) error {
	fileTypes := c.Param("types")

	if err := validateKepubFileTypes(fileTypes); err != nil {
		return err
	}

	libraryID, err := httputil.ParamID(c, "libraryID", "Library")
	if err != nil {
		return err
	}

	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURLKepub(c)
	desc := h.opdsService.BuildLibraryOpenSearchDescription(baseURL, fileTypes, libraryID)

	c.Response().Header().Set(echo.HeaderContentType, MimeTypeOpenSearch)
	return c.XML(http.StatusOK, desc)
}

// download handles file downloads with generated metadata. When there is
// nothing to generate (a supplement or a type with no generator), OPDS
// clients get the original file.
func (h *handler) download(c echo.Context) error {
	return h.serveDownload(c, false)
}

// downloadKepub handles KePub file downloads. KePub conversion is only
// supported for EPUB and CBZ files; other types get the original file.
func (h *handler) downloadKepub(c echo.Context) error {
	return h.serveDownload(c, true)
}

// serveDownload serves the file named by the id param, generated or as the
// original, per books.ResolveFallbackDownload.
func (h *handler) serveDownload(c echo.Context, kepub bool) error {
	ctx := c.Request().Context()

	fileID, err := httputil.ParamID(c, "id", "File")
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

	download, err := books.ResolveFallbackDownload(ctx, h.downloadCache, book, fileWithRelations, kepub)
	if err != nil {
		return err
	}
	return download.Serve(c)
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
