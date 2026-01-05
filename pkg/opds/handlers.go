package opds

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type handler struct {
	opdsService   *Service
	bookService   *books.Service
	downloadCache *downloadcache.Cache
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

	validTypes := map[string]bool{
		models.FileTypeEPUB: true,
		models.FileTypeCBZ:  true,
		models.FileTypeM4B:  true,
	}

	for _, t := range parseFileTypes(types) {
		if !validTypes[t] {
			return errcodes.ValidationError("Invalid file type: " + t)
		}
	}

	return nil
}

// checkLibraryAccess checks if the user has access to the specified library.
func checkLibraryAccess(c echo.Context, libraryID int) error {
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(libraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}
	return nil
}

// getUserAccessibleLibraryIDs returns the library IDs the user can access, or nil for all.
func getUserAccessibleLibraryIDs(c echo.Context) []int {
	if user, ok := c.Get("user").(*models.User); ok {
		return user.GetAccessibleLibraryIDs()
	}
	return nil
}

// catalog handles the root catalog feed (lists libraries).
func (h *handler) catalog(c echo.Context) error {
	ctx := c.Request().Context()
	fileTypes := c.Param("types")

	if err := validateFileTypes(fileTypes); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	libraryIDs := getUserAccessibleLibraryIDs(c)
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
		return err
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	feed, err := h.opdsService.BuildLibraryAllBooksFeed(ctx, baseURL, fileTypes, libraryID, limit, offset)
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
		return err
	}

	seriesID, err := strconv.Atoi(c.Param("seriesID"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	feed, err := h.opdsService.BuildLibrarySeriesBooksFeed(ctx, baseURL, fileTypes, libraryID, seriesID, limit, offset)
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
		return err
	}

	authorName, err := url.PathUnescape(c.Param("authorName"))
	if err != nil {
		return errcodes.NotFound("Author")
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	feed, err := h.opdsService.BuildLibraryAuthorBooksFeed(ctx, baseURL, fileTypes, libraryID, authorName, limit, offset)
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
		return err
	}

	query := c.QueryParam("q")
	if query == "" {
		return errcodes.ValidationError("Search query is required")
	}

	limit, offset := getPaginationParams(c)
	baseURL := getBaseURL(c)

	feed, err := h.opdsService.BuildLibrarySearchFeed(ctx, baseURL, fileTypes, libraryID, query, limit, offset)
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

	libraryID, err := strconv.Atoi(c.Param("libraryID"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	if err := checkLibraryAccess(c, libraryID); err != nil {
		return err
	}

	baseURL := getBaseURL(c)
	desc := h.opdsService.BuildLibraryOpenSearchDescription(baseURL, fileTypes, libraryID)

	c.Response().Header().Set(echo.HeaderContentType, MimeTypeOpenSearch)
	return c.XML(http.StatusOK, desc)
}

// download handles file downloads with generated metadata.
// For OPDS clients, we try to generate a file with embedded metadata.
// If generation fails, we fall back to the original file.
func (h *handler) download(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		ID: &fileID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := checkLibraryAccess(c, file.LibraryID); err != nil {
		return err
	}

	// Check if source file exists
	if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
		return errcodes.NotFound("File")
	}

	// Get the full book with relations for generation
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Try to generate/get from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerate(ctx, book, file)
	if err != nil {
		// For OPDS clients, fall back to original file on generation error
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			log.Warn("file generation failed, serving original", logger.Data{
				"file_id":   file.ID,
				"file_type": file.FileType,
				"error":     genErr.Message,
			})
			// Fall back to original file
			filename := filepath.Base(file.Filepath)
			c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
			return c.File(file.Filepath)
		}
		return errors.WithStack(err)
	}

	// Set content disposition for download with the formatted filename
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+downloadFilename+"\"")

	return c.File(cachedPath)
}

// respondXML sends an XML response with the correct content type.
func respondXML(c echo.Context, data interface{}) error {
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
