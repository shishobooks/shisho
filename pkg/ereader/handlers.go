package ereader

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/uptrace/bun"
)

const defaultPageSize = 50

type handler struct {
	db              *bun.DB
	libraryService  *libraries.Service
	bookService     *books.Service
	seriesService   *series.Service
	peopleService   *people.Service
	downloadCache   *downloadcache.Cache
	settingsService *settings.Service
}

func newHandler(
	db *bun.DB,
	libraryService *libraries.Service,
	bookService *books.Service,
	seriesService *series.Service,
	peopleService *people.Service,
	downloadCache *downloadcache.Cache,
	settingsService *settings.Service,
) *handler {
	return &handler{
		db:              db,
		libraryService:  libraryService,
		bookService:     bookService,
		seriesService:   seriesService,
		peopleService:   peopleService,
		downloadCache:   downloadCache,
		settingsService: settingsService,
	}
}

func (h *handler) baseURL(c echo.Context) string {
	apiKey := c.Param("apiKey")
	return "/ereader/key/" + apiKey
}

// getUserLibraryIDs gets the library IDs a user can access.
func (h *handler) getUserLibraryIDs(ctx echo.Context, userID int) ([]int, error) {
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Relation("LibraryAccess").
		Where("u.id = ?", userID).
		Scan(ctx.Request().Context())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return user.GetAccessibleLibraryIDs(), nil
}

// resolveSort returns the stored user-library sort preference for the
// API-key-authenticated caller, falling back to sortspec.BuiltinDefault
// when no preference is set. The eReader browser has no explicit URL
// sort input — the stored preference (or builtin default) is the only
// input.
//
// The books service also falls back to BuiltinDefault when Sort is nil,
// so returning BuiltinDefault here is belt-and-suspenders: it keeps the
// eReader surface explicit about what sort it applies and insulates the
// eReader UI from a future change to the service's default. It also
// guarantees this function never returns nil, so callers can rely on
// the result without guarding.
func (h *handler) resolveSort(ctx context.Context, apiKey *apikeys.APIKey, libraryID int) []sortspec.SortLevel {
	if apiKey == nil {
		return sortspec.BuiltinDefault()
	}
	resolved := sortspec.ResolveForLibrary(ctx, h.settingsService, apiKey.UserID, libraryID, nil)
	if resolved == nil {
		return sortspec.BuiltinDefault()
	}
	return resolved
}

// Libraries lists all libraries the user has access to.
func (h *handler) Libraries(c echo.Context) error {
	ctx := c.Request().Context()
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	// Get user's accessible library IDs
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}

	// List libraries
	libs, err := h.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{
		LibraryIDs: libraryIDs,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(""))
	content.WriteString("<h1>Libraries</h1>")

	baseURL := h.baseURL(c)
	for _, lib := range libs {
		libraryURL := fmt.Sprintf("%s/libraries/%d", baseURL, lib.ID)
		content.WriteString(itemHTML(lib.Name, libraryURL, ""))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// LibraryNav shows navigation options for a library.
func (h *handler) LibraryNav(c echo.Context) error {
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>Library</h1>")

	// Navigation options
	content.WriteString(itemHTML("All Books", fmt.Sprintf("%s/libraries/%s/all", baseURL, libraryID), "Browse all books"))
	content.WriteString(itemHTML("Series", fmt.Sprintf("%s/libraries/%s/series", baseURL, libraryID), "Browse by series"))
	content.WriteString(itemHTML("Authors", fmt.Sprintf("%s/libraries/%s/authors", baseURL, libraryID), "Browse by author"))
	content.WriteString(itemHTML("Search", fmt.Sprintf("%s/libraries/%s/search", baseURL, libraryID), "Search for books"))

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// LibraryAllBooks shows paginated list of all books in a library.
func (h *handler) LibraryAllBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	// Parse query params
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return errcodes.ValidationError("Invalid library ID")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, libraryIDInt) {
		return errcodes.Forbidden("Access to this library is denied")
	}

	sort := h.resolveSort(ctx, apiKey, libraryIDInt)

	booksResult, total, err := h.listBooksPaginated(ctx, books.ListBooksOptions{
		LibraryID: &libraryIDInt,
		Sort:      sort,
	}, page, typesFilter)
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>All Books</h1>")

	currentURL := fmt.Sprintf("%s/libraries/%s/all", baseURL, libraryID)
	content.WriteString(filterBar(currentURL, typesFilter, coversParam))

	for _, book := range booksResult {
		meta := formatBookMeta(book)
		bookURL := buildBookURL(baseURL, book.ID, coversParam)
		coverURL := getBookCoverURL(baseURL, book)
		content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
	}

	// Pagination (preserve filter params)
	totalPages := (total + defaultPageSize - 1) / defaultPageSize
	content.WriteString(paginationWithParams(page, totalPages, currentURL, typesFilter, coversParam))

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// LibrarySeries shows list of series in a library.
func (h *handler) LibrarySeries(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return errcodes.ValidationError("Invalid library ID")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, libraryIDInt) {
		return errcodes.Forbidden("Access to this library is denied")
	}

	// Get series for this library
	seriesList, _, err := h.seriesService.ListSeriesWithTotal(ctx, series.ListSeriesOptions{
		LibraryID: &libraryIDInt,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>Series</h1>")

	for _, s := range seriesList {
		seriesURL := fmt.Sprintf("%s/libraries/%s/series/%d", baseURL, libraryID, s.ID)
		content.WriteString(itemHTML(s.Name, seriesURL, fmt.Sprintf("%d books", s.BookCount)))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// SeriesBooks shows books in a series.
func (h *handler) SeriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	seriesID := c.Param("seriesId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	// Parse query and filter params
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return errcodes.ValidationError("Invalid library ID")
	}

	seriesIDInt, err := strconv.Atoi(seriesID)
	if err != nil {
		return errcodes.ValidationError("Invalid series ID")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, libraryIDInt) {
		return errcodes.Forbidden("Access to this library is denied")
	}

	sort := h.resolveSort(ctx, apiKey, libraryIDInt)

	// Get series info
	s, err := h.seriesService.RetrieveSeriesByID(ctx, seriesIDInt)
	if err != nil {
		return errors.WithStack(err)
	}
	if s == nil {
		return errcodes.NotFound("Series")
	}

	// Get books in series (paginated)
	booksResult, total, err := h.listBooksPaginated(ctx, books.ListBooksOptions{
		LibraryID: &libraryIDInt,
		SeriesID:  &seriesIDInt,
		Sort:      sort,
	}, page, typesFilter)
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", s.Name))

	currentURL := fmt.Sprintf("%s/libraries/%s/series/%s", baseURL, libraryID, seriesID)
	content.WriteString(filterBar(currentURL, typesFilter, coversParam))

	for _, book := range booksResult {
		meta := formatBookMeta(book)
		bookURL := buildBookURL(baseURL, book.ID, coversParam)
		coverURL := getBookCoverURL(baseURL, book)
		content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
	}

	// Pagination (preserve filter params)
	totalPages := (total + defaultPageSize - 1) / defaultPageSize
	content.WriteString(paginationWithParams(page, totalPages, currentURL, typesFilter, coversParam))

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// LibraryAuthors shows list of authors in a library.
func (h *handler) LibraryAuthors(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return errcodes.ValidationError("Invalid library ID")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, libraryIDInt) {
		return errcodes.Forbidden("Access to this library is denied")
	}

	// Get authors for this library (people with books in this library)
	authorsList, _, err := h.peopleService.ListPeopleWithTotal(ctx, people.ListPeopleOptions{
		LibraryID: &libraryIDInt,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>Authors</h1>")

	for _, author := range authorsList {
		authorURL := fmt.Sprintf("%s/libraries/%s/authors/%d", baseURL, libraryID, author.ID)
		content.WriteString(itemHTML(author.Name, authorURL, ""))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// AuthorBooks shows books by an author.
func (h *handler) AuthorBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	authorID := c.Param("authorId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	// Parse query and filter params
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return errcodes.ValidationError("Invalid library ID")
	}

	authorIDInt, err := strconv.Atoi(authorID)
	if err != nil {
		return errcodes.ValidationError("Invalid author ID")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, libraryIDInt) {
		return errcodes.Forbidden("Access to this library is denied")
	}

	// Get author info, scoped to the library param so a person ID from a
	// sibling library cannot leak its name through the page heading.
	author, err := h.peopleService.RetrievePerson(ctx, people.RetrievePersonOptions{
		ID:        &authorIDInt,
		LibraryID: &libraryIDInt,
	})
	if err != nil {
		if errors.Is(err, errcodes.NotFound("Person")) {
			return errcodes.NotFound("Author")
		}
		return errors.WithStack(err)
	}

	// Resolve sort once (so the type-filter branch and the SQL query use
	// the same ordering) and fetch books for this author + library via
	// the books service. Going through the books service applies the
	// user's stored sort preference and lets SQL do the library/author
	// filtering — the previous code fetched all books for the author
	// across every library and filtered in Go, which both ignored sort
	// and scanned more rows than necessary.
	sort := h.resolveSort(ctx, apiKey, libraryIDInt)
	booksInLibrary, total, err := h.listBooksPaginated(ctx, books.ListBooksOptions{
		LibraryID: &libraryIDInt,
		PersonID:  &authorIDInt,
		Sort:      sort,
	}, page, typesFilter)
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", author.Name))

	currentURL := fmt.Sprintf("%s/libraries/%s/authors/%s", baseURL, libraryID, authorID)
	content.WriteString(filterBar(currentURL, typesFilter, coversParam))

	for _, book := range booksInLibrary {
		meta := formatBookMeta(book)
		bookURL := buildBookURL(baseURL, book.ID, coversParam)
		coverURL := getBookCoverURL(baseURL, book)
		content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
	}

	// Pagination (preserve filter params)
	totalPages := (total + defaultPageSize - 1) / defaultPageSize
	content.WriteString(paginationWithParams(page, totalPages, currentURL, typesFilter, coversParam))

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// LibrarySearch shows search form and results.
func (h *handler) LibrarySearch(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	query := c.QueryParam("q")
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := strconv.Atoi(libraryID)
	if err != nil {
		return errcodes.ValidationError("Invalid library ID")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, libraryIDInt) {
		return errcodes.Forbidden("Access to this library is denied")
	}

	sort := h.resolveSort(ctx, apiKey, libraryIDInt)

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>Search</h1>")

	searchURL := fmt.Sprintf("%s/libraries/%s/search", baseURL, libraryID)
	content.WriteString(searchForm(searchURL, query))
	content.WriteString(filterBar(searchURL, typesFilter, coversParam, [2]string{"q", query}))

	if query != "" {
		// Search for books (paginated)
		booksResult, total, err := h.listBooksPaginated(ctx, books.ListBooksOptions{
			LibraryID: &libraryIDInt,
			Search:    &query,
			Sort:      sort,
		}, page, typesFilter)
		if err != nil {
			return errors.WithStack(err)
		}

		content.WriteString(fmt.Sprintf("<p>Found %d results</p>", total))

		for _, book := range booksResult {
			meta := formatBookMeta(book)
			bookURL := buildBookURL(baseURL, book.ID, coversParam)
			coverURL := getBookCoverURL(baseURL, book)
			content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
		}

		// Pagination (preserve q + filter params)
		totalPages := (total + defaultPageSize - 1) / defaultPageSize
		content.WriteString(paginationWithParams(page, totalPages, searchURL, typesFilter, coversParam, [2]string{"q", query}))
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// Download handles book downloads with Kobo detection for KePub conversion.
func (h *handler) Download(c echo.Context) error {
	ctx := c.Request().Context()
	bookID := c.Param("bookId")
	baseURL := h.baseURL(c)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	// Parse cover toggle
	coversParam := c.QueryParam("covers")
	showCover := coversParam == "on"

	bookIDInt, err := strconv.Atoi(bookID)
	if err != nil {
		return errcodes.ValidationError("Invalid book ID")
	}

	// Get book details
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &bookIDInt,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	if book == nil {
		return errcodes.NotFound("Book")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, book.LibraryID) {
		return errcodes.Forbidden("Access to this book is denied")
	}

	// Get the primary file from the book's files
	var mainFile *models.File
	for _, f := range book.Files {
		if book.PrimaryFileID != nil && f.ID == *book.PrimaryFileID {
			mainFile = f
			break
		}
	}
	if mainFile == nil && len(book.Files) > 0 {
		mainFile = book.Files[0] // Fallback to first file if no primary set
	}
	if mainFile == nil {
		return errcodes.NotFound("No files available for this book")
	}

	// Detect Kobo device from User-Agent
	userAgent := c.Request().Header.Get("User-Agent")
	isKobo := strings.Contains(strings.ToLower(userAgent), "kobo")

	// Generate download link using eReader file download route (with API key auth)
	var downloadURL string
	supportsKepub := mainFile.FileType == models.FileTypeEPUB || mainFile.FileType == models.FileTypeCBZ
	if isKobo && supportsKepub {
		// Use KePub conversion for Kobo
		downloadURL = fmt.Sprintf("%s/file/%d/kepub", baseURL, mainFile.ID)
	} else {
		// Use original file
		downloadURL = fmt.Sprintf("%s/file/%d", baseURL, mainFile.ID)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", book.Title))

	// Cover toggle (only show if book has a cover)
	hasCover := hasBookCover(book)
	if hasCover {
		currentURL := fmt.Sprintf("%s/download/%d", baseURL, book.ID)
		content.WriteString(coverToggle(currentURL, coversParam))

		// Show cover if enabled
		if showCover {
			coverURL := getBookCoverURL(baseURL, book)
			content.WriteString(fmt.Sprintf(`<p><img src="%s" alt="" style="max-width: 150px; max-height: 200px;"></p>`, coverURL))
		}
	}

	if len(book.Authors) > 0 {
		var authorNames []string
		for _, a := range book.Authors {
			if a.Person != nil {
				authorNames = append(authorNames, a.Person.Name)
			}
		}
		if len(authorNames) > 0 {
			content.WriteString(fmt.Sprintf("<p>By: %s</p>", strings.Join(authorNames, ", ")))
		}
	}

	// Show file metadata
	var metaParts []string

	// File size
	if mainFile.FilesizeBytes > 0 {
		metaParts = append(metaParts, formatFileSize(mainFile.FilesizeBytes))
	}

	// Page count (CBZ)
	if mainFile.PageCount != nil && *mainFile.PageCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d pages", *mainFile.PageCount))
	}

	// Duration (M4B audiobooks)
	if mainFile.AudiobookDurationSeconds != nil && *mainFile.AudiobookDurationSeconds > 0 {
		metaParts = append(metaParts, formatDuration(*mainFile.AudiobookDurationSeconds))
	}

	// Narrators (M4B audiobooks)
	if len(mainFile.Narrators) > 0 {
		var narratorNames []string
		for _, n := range mainFile.Narrators {
			if n.Person != nil {
				narratorNames = append(narratorNames, n.Person.Name)
			}
		}
		if len(narratorNames) > 0 {
			metaParts = append(metaParts, "Narrated by: "+strings.Join(narratorNames, ", "))
		}
	}

	if len(metaParts) > 0 {
		content.WriteString(fmt.Sprintf("<p><i>%s</i></p>", strings.Join(metaParts, " • ")))
	}

	if book.Description != nil && *book.Description != "" {
		content.WriteString(fmt.Sprintf("<p>%s</p>", *book.Description))
	}

	// Show appropriate format in download link (button style for easier tapping)
	downloadFormat := strings.ToUpper(mainFile.FileType)
	if isKobo && supportsKepub {
		downloadFormat = "KePub"
	}
	content.WriteString(fmt.Sprintf(`<p><a href="%s" class="nav-btn" style="display: inline-block;">Download %s</a></p>`, downloadURL, downloadFormat))

	// Note for CBZ files about conversion time
	if mainFile.FileType == models.FileTypeCBZ {
		content.WriteString(`<p style="font-size: 0.9em; color: #666;"><i>Note: The download may take a moment to start while the file is being prepared.</i></p>`)
	}

	return c.HTML(http.StatusOK, RenderPage(content.String()))
}

// Cover serves a book cover image.
func (h *handler) Cover(c echo.Context) error {
	ctx := c.Request().Context()
	bookID := c.Param("bookId")
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	bookIDInt, err := strconv.Atoi(bookID)
	if err != nil {
		return errcodes.ValidationError("Invalid book ID")
	}

	// Get book details
	book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &bookIDInt,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	if book == nil {
		return errcodes.NotFound("Book")
	}

	// Check library access
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, book.LibraryID) {
		return errcodes.Forbidden("Access to this book is denied")
	}

	// Get the library to determine cover aspect ratio preference
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &book.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Select the appropriate file based on the library's cover aspect ratio setting
	coverFile := selectCoverFile(book.Files, library.CoverAspectRatio)
	if coverFile == nil || coverFile.CoverImageFilename == nil || *coverFile.CoverImageFilename == "" {
		return errcodes.NotFound("Cover")
	}

	// Resolve via the file's parent dir — book.Filepath may be a synthetic
	// organized-folder path that doesn't exist on disk for root-level files.
	coverPath := filepath.Join(filepath.Dir(coverFile.Filepath), *coverFile.CoverImageFilename)
	c.Response().Header().Set("Cache-Control", "private, no-cache")
	return errors.WithStack(c.File(coverPath))
}

// DownloadFile handles file downloads with API key authentication.
func (h *handler) DownloadFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	fileID, err := strconv.Atoi(c.Param("fileId"))
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
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, file.LibraryID) {
		return errcodes.Forbidden("Access to this file is denied")
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

	// Find the file with all relations from the book's files
	var fileWithRelations *models.File
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}
	if fileWithRelations == nil {
		fileWithRelations = file
	}

	// Try to generate/get from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerate(ctx, book, fileWithRelations)
	if err != nil {
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			log.Warn("file generation failed, serving original", logger.Data{
				"file_id":   file.ID,
				"file_type": file.FileType,
				"error":     genErr.Message,
			})
			filename := filepath.Base(file.Filepath)
			c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
			return c.File(file.Filepath)
		}
		return errors.WithStack(err)
	}

	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+downloadFilename+"\"")
	return c.File(cachedPath)
}

// DownloadFileKepub handles KePub file downloads with API key authentication.
func (h *handler) DownloadFileKepub(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)
	apiKey := GetAPIKeyFromContext(ctx)
	if apiKey == nil {
		return errcodes.Unauthorized("API key not found")
	}

	fileID, err := strconv.Atoi(c.Param("fileId"))
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
	libraryIDs, err := h.getUserLibraryIDs(c, apiKey.UserID)
	if err != nil {
		return err
	}
	if libraryIDs != nil && !containsInt(libraryIDs, file.LibraryID) {
		return errcodes.Forbidden("Access to this file is denied")
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

	// Find the file with all relations from the book's files
	var fileWithRelations *models.File
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}
	if fileWithRelations == nil {
		fileWithRelations = file
	}

	// Try to generate/get KePub from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerateKepub(ctx, book, fileWithRelations)
	if err != nil {
		if errors.Is(err, filegen.ErrKepubNotSupported) {
			log.Warn("kepub conversion not supported, serving original", logger.Data{
				"file_id":   file.ID,
				"file_type": file.FileType,
			})
			filename := filepath.Base(file.Filepath)
			c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
			return c.File(file.Filepath)
		}
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			log.Warn("kepub generation failed, serving original", logger.Data{
				"file_id":   file.ID,
				"file_type": file.FileType,
				"error":     genErr.Message,
			})
			filename := filepath.Base(file.Filepath)
			c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
			return c.File(file.Filepath)
		}
		return errors.WithStack(err)
	}

	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+downloadFilename+"\"")
	return c.File(cachedPath)
}

// selectCoverFile selects the appropriate file for cover display.
func selectCoverFile(files []*models.File, coverAspectRatio string) *models.File {
	var bookFiles, audiobookFiles []*models.File
	for _, f := range files {
		if f.CoverImageFilename == nil || *f.CoverImageFilename == "" {
			continue
		}
		switch f.FileType {
		case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF:
			bookFiles = append(bookFiles, f)
		case models.FileTypeM4B:
			audiobookFiles = append(audiobookFiles, f)
		}
	}

	switch coverAspectRatio {
	case "audiobook", "audiobook_fallback_book":
		if len(audiobookFiles) > 0 {
			return audiobookFiles[0]
		}
		if len(bookFiles) > 0 {
			return bookFiles[0]
		}
	default: // "book", "book_fallback_audiobook", or any other value
		if len(bookFiles) > 0 {
			return bookFiles[0]
		}
		if len(audiobookFiles) > 0 {
			return audiobookFiles[0]
		}
	}

	return nil
}

// Helper functions

func formatBookMeta(book *models.Book) string {
	var parts []string
	for _, a := range book.Authors {
		if a.Person != nil {
			parts = append(parts, a.Person.Name)
			break
		}
	}

	// Add file types
	fileTypes := getBookFileTypes(book)
	if len(fileTypes) > 0 {
		parts = append(parts, strings.Join(fileTypes, ", "))
	}

	return strings.Join(parts, " • ")
}

// getBookFileTypes returns a list of unique file types for a book (e.g., ["EPUB", "M4B"]).
func getBookFileTypes(book *models.Book) []string {
	seen := make(map[string]bool)
	var types []string
	for _, f := range book.Files {
		if f.FileType != "" && !seen[f.FileType] {
			seen[f.FileType] = true
			types = append(types, strings.ToUpper(f.FileType))
		}
	}
	return types
}

// buildBookURL builds a book detail URL with optional covers param.
func buildBookURL(baseURL string, bookID int, coversParam string) string {
	url := fmt.Sprintf("%s/download/%d", baseURL, bookID)
	if coversParam == "on" {
		url += "?covers=on"
	}
	return url
}

// formatFileSize formats bytes into human-readable size.
func formatFileSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// formatDuration formats seconds into hours and minutes.
func formatDuration(seconds float64) string {
	totalMinutes := int(seconds / 60)
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// getBookFileType returns the primary file type for a book.
func getBookFileType(book *models.Book) string {
	if book.PrimaryFileID != nil {
		for _, f := range book.Files {
			if f.ID == *book.PrimaryFileID {
				return f.FileType
			}
		}
	}
	if len(book.Files) > 0 {
		return book.Files[0].FileType
	}
	return ""
}

// filterBooksByType filters books to only include those matching the specified type.
func filterBooksByType(books []*models.Book, fileType string) []*models.Book {
	if fileType == "" || fileType == "all" {
		return books
	}

	var filtered []*models.Book
	for _, book := range books {
		bookType := getBookFileType(book)
		if strings.EqualFold(bookType, fileType) {
			filtered = append(filtered, book)
		}
	}
	return filtered
}

// getBookCoverURL returns the cover URL for a book using the eReader cover endpoint.
// Returns empty string if the book has no cover.
func getBookCoverURL(baseURL string, book *models.Book) string {
	if !hasBookCover(book) {
		return ""
	}
	return fmt.Sprintf("%s/cover/%d", baseURL, book.ID)
}

// hasBookCover checks if a book has any file with a cover image.
func hasBookCover(book *models.Book) bool {
	for _, f := range book.Files {
		if f.CoverImageFilename != nil && *f.CoverImageFilename != "" {
			return true
		}
	}
	return false
}

func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func intPtr(i int) *int {
	return &i
}

// parsePageParam parses a ?page= query value, defaulting to 1 for
// missing or invalid input (including non-positive numbers).
func parsePageParam(raw string) int {
	if raw == "" {
		return 1
	}
	if p, err := strconv.Atoi(raw); err == nil && p > 0 {
		return p
	}
	return 1
}

// listBooksPaginated fetches a single page of books, applying the
// eReader's in-memory type filter (when active) before paginating.
// Returns the books for the requested page and the total count used to
// drive the pagination UI.
//
// When typesFilter is active, we fetch all books and filter in Go
// because the books service's FileTypes filter is "any file matches"
// while the eReader UI shows the dominant per-book type via
// getBookFileType — the two definitions disagree on books with mixed
// file types, so we filter to match what the UI displays.
func (h *handler) listBooksPaginated(ctx context.Context, opts books.ListBooksOptions, page int, typesFilter string) ([]*models.Book, int, error) {
	if typesFilter != "" && typesFilter != "all" {
		allBooks, _, err := h.bookService.ListBooksWithTotal(ctx, opts)
		if err != nil {
			return nil, 0, errors.WithStack(err)
		}
		filtered := filterBooksByType(allBooks, typesFilter)
		total := len(filtered)
		offset := (page - 1) * defaultPageSize
		if offset >= total {
			return nil, total, nil
		}
		end := offset + defaultPageSize
		if end > total {
			end = total
		}
		return filtered[offset:end], total, nil
	}

	offset := (page - 1) * defaultPageSize
	opts.Limit = intPtr(defaultPageSize)
	opts.Offset = intPtr(offset)
	result, total, err := h.bookService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return result, total, nil
}

// ResolveShortURL redirects a short code to the full eReader URL.
func ResolveShortURL(c echo.Context, apiKeyService *apikeys.Service) error {
	shortCode := c.Param("shortCode")

	apiKey, err := apiKeyService.ResolveShortCode(c.Request().Context(), shortCode)
	if err != nil {
		return errors.WithStack(err)
	}
	if apiKey == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Short URL not found or expired")
	}

	redirectURL := fmt.Sprintf("/ereader/key/%s/", apiKey.Key)
	return c.Redirect(http.StatusFound, redirectURL)
}
