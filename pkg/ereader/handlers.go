package ereader

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sortspec"
)

const defaultPageSize = 50

type handler struct {
	libraryService  *libraries.Service
	bookService     *books.Service
	seriesService   *series.Service
	peopleService   *people.Service
	downloadCache   *downloadcache.Cache
	settingsService *settings.Service
	reachService    *reach.Service
}

func newHandler(
	libraryService *libraries.Service,
	bookService *books.Service,
	seriesService *series.Service,
	peopleService *people.Service,
	downloadCache *downloadcache.Cache,
	settingsService *settings.Service,
	reachService *reach.Service,
) *handler {
	return &handler{
		libraryService:  libraryService,
		bookService:     bookService,
		seriesService:   seriesService,
		peopleService:   peopleService,
		downloadCache:   downloadCache,
		settingsService: settingsService,
		reachService:    reachService,
	}
}

func (h *handler) baseURL(c echo.Context) string {
	return "/ereader/key/" + c.Param("credential")
}

// render sends a page, with a banner when the browser looks like a Kindle
// or a Kobo but the Device's type says otherwise.
func (h *handler) render(c echo.Context, content string) error {
	device, err := devices.RequireDevice(c)
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, RenderPage(typeMismatchBanner(device.DeviceType, c.Request().UserAgent())+content))
}

// requireEntityAccess returns resource's 404 when the Device's owner cannot
// access libraryID, the same response as an id with no row. It matches the
// Kobo per-file rule, so a Device cannot probe for books or files outside its
// owner's libraries. Library paths use requireLibrary instead.
func requireEntityAccess(c echo.Context, libraryID int, resource string) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if !user.HasLibraryAccess(libraryID) {
		return errcodes.NotFound(resource)
	}
	return nil
}

// resolveSort returns the stored user-library sort preference for the
// Device's owner, falling back to sortspec.BuiltinDefault
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
func (h *handler) resolveSort(ctx context.Context, userID int, libraryID int) []sortspec.SortLevel {
	resolved := sortspec.ResolveForLibrary(ctx, h.settingsService, userID, libraryID, nil)
	if resolved == nil {
		return sortspec.BuiltinDefault()
	}
	return resolved
}

// Libraries lists the libraries the Device reaches that have a book with
// something to deliver.
func (h *handler) Libraries(c echo.Context) error {
	r, err := h.reach(c)
	if err != nil {
		return err
	}
	libs, err := h.reachService.Libraries(c.Request().Context(), r)
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

	return h.render(c, content.String())
}

// LibraryNav shows navigation options for a library, leaving out Series and
// Authors when the library has none with something to deliver.
func (h *handler) LibraryNav(c echo.Context) error {
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)

	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}
	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}
	sections, err := h.reachService.Sections(c.Request().Context(), r, libraryIDInt)
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>Library</h1>")

	// Navigation options
	content.WriteString(itemHTML("All Books", fmt.Sprintf("%s/libraries/%s/all", baseURL, libraryID), "Browse all books"))
	if sections.Series {
		content.WriteString(itemHTML("Series", fmt.Sprintf("%s/libraries/%s/series", baseURL, libraryID), "Browse by series"))
	}
	if sections.Authors {
		content.WriteString(itemHTML("Authors", fmt.Sprintf("%s/libraries/%s/authors", baseURL, libraryID), "Browse by author"))
	}
	content.WriteString(itemHTML("Search", fmt.Sprintf("%s/libraries/%s/search", baseURL, libraryID), "Search for books"))

	return h.render(c, content.String())
}

// LibraryAllBooks shows paginated list of all books in a library.
func (h *handler) LibraryAllBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Parse query params
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}

	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}
	typesFilter = typesFilterFor(r, typesFilter)

	sort := h.resolveSort(ctx, user.ID, libraryIDInt)

	booksResult, total, err := h.listBooksPaginated(ctx, r, books.ListBooksOptions{
		LibraryID: &libraryIDInt,
		Sort:      sort,
	}, page, typesFilter)
	if err != nil {
		return errors.WithStack(err)
	}
	fileTypes, err := h.reachService.DeliveredFileTypesWithBooks(ctx, r, books.ListBooksOptions{LibraryID: &libraryIDInt})
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>All Books</h1>")

	currentURL := fmt.Sprintf("%s/libraries/%s/all", baseURL, libraryID)
	content.WriteString(filterBar(currentURL, fileTypes, typesFilter, coversParam))

	for _, book := range booksResult {
		meta := formatBookMeta(book, r.Rules)
		bookURL := buildBookURL(baseURL, book.ID, coversParam)
		coverURL := getBookCoverURL(baseURL, book)
		content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
	}

	// Pagination (preserve filter params)
	totalPages := (total + defaultPageSize - 1) / defaultPageSize
	content.WriteString(paginationWithParams(page, totalPages, currentURL, typesFilter, coversParam))

	return h.render(c, content.String())
}

// LibrarySeries shows list of series in a library.
func (h *handler) LibrarySeries(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}

	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}

	// Get series for this library
	seriesList, _, err := h.seriesService.ListSeriesWithTotal(ctx, series.ListSeriesOptions{
		LibraryID:     &libraryIDInt,
		ListID:        r.ListID(),
		MainFileTypes: r.SourceFileTypes(),
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

	return h.render(c, content.String())
}

// SeriesBooks shows books in a series.
func (h *handler) SeriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	seriesID := c.Param("seriesId")
	baseURL := h.baseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Parse query and filter params
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}

	seriesIDInt, err := httputil.ParamID(c, "seriesId", "Series")
	if err != nil {
		return err
	}

	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}
	typesFilter = typesFilterFor(r, typesFilter)

	sort := h.resolveSort(ctx, user.ID, libraryIDInt)

	// Get series info
	s, err := h.seriesService.RetrieveSeriesByID(ctx, seriesIDInt)
	if err != nil {
		return errors.WithStack(err)
	}
	// A series from another library is not found here, so its name never
	// renders under a library path it does not belong to.
	if s == nil || s.LibraryID != libraryIDInt {
		return errcodes.NotFound("Series")
	}
	if err := h.requireListedBooks(ctx, r, books.ListBooksOptions{LibraryID: &libraryIDInt, SeriesID: &seriesIDInt}, "Series"); err != nil {
		return err
	}

	// Get books in series (paginated)
	booksResult, total, err := h.listBooksPaginated(ctx, r, books.ListBooksOptions{
		LibraryID: &libraryIDInt,
		SeriesID:  &seriesIDInt,
		Sort:      sort,
	}, page, typesFilter)
	if err != nil {
		return errors.WithStack(err)
	}
	fileTypes, err := h.reachService.DeliveredFileTypesWithBooks(ctx, r, books.ListBooksOptions{LibraryID: &libraryIDInt, SeriesID: &seriesIDInt})
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", html.EscapeString(s.Name)))

	currentURL := fmt.Sprintf("%s/libraries/%s/series/%s", baseURL, libraryID, seriesID)
	content.WriteString(filterBar(currentURL, fileTypes, typesFilter, coversParam))

	for _, book := range booksResult {
		meta := formatBookMeta(book, r.Rules)
		bookURL := buildBookURL(baseURL, book.ID, coversParam)
		coverURL := getBookCoverURL(baseURL, book)
		content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
	}

	// Pagination (preserve filter params)
	totalPages := (total + defaultPageSize - 1) / defaultPageSize
	content.WriteString(paginationWithParams(page, totalPages, currentURL, typesFilter, coversParam))

	return h.render(c, content.String())
}

// LibraryAuthors shows list of authors in a library.
func (h *handler) LibraryAuthors(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}

	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}

	// Get authors for this library (people with books in this library)
	authorsList, _, err := h.peopleService.ListPeopleWithTotal(ctx, people.ListPeopleOptions{
		LibraryID:     &libraryIDInt,
		ListID:        r.ListID(),
		MainFileTypes: r.SourceFileTypes(),
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

	return h.render(c, content.String())
}

// AuthorBooks shows books by an author.
func (h *handler) AuthorBooks(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	authorID := c.Param("authorId")
	baseURL := h.baseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	// Parse query and filter params
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}

	authorIDInt, err := httputil.ParamID(c, "authorId", "Author")
	if err != nil {
		return err
	}

	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}
	typesFilter = typesFilterFor(r, typesFilter)

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

	// Fetch books for this author and library through the books service,
	// which applies the user's stored sort preference.
	sort := h.resolveSort(ctx, user.ID, libraryIDInt)
	if err := h.requireListedBooks(ctx, r, books.ListBooksOptions{LibraryID: &libraryIDInt, PersonID: &authorIDInt}, "Author"); err != nil {
		return err
	}
	booksInLibrary, total, err := h.listBooksPaginated(ctx, r, books.ListBooksOptions{
		LibraryID: &libraryIDInt,
		PersonID:  &authorIDInt,
		Sort:      sort,
	}, page, typesFilter)
	if err != nil {
		return errors.WithStack(err)
	}
	fileTypes, err := h.reachService.DeliveredFileTypesWithBooks(ctx, r, books.ListBooksOptions{LibraryID: &libraryIDInt, PersonID: &authorIDInt})
	if err != nil {
		return errors.WithStack(err)
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", html.EscapeString(author.Name)))

	currentURL := fmt.Sprintf("%s/libraries/%s/authors/%s", baseURL, libraryID, authorID)
	content.WriteString(filterBar(currentURL, fileTypes, typesFilter, coversParam))

	for _, book := range booksInLibrary {
		meta := formatBookMeta(book, r.Rules)
		bookURL := buildBookURL(baseURL, book.ID, coversParam)
		coverURL := getBookCoverURL(baseURL, book)
		content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
	}

	// Pagination (preserve filter params)
	totalPages := (total + defaultPageSize - 1) / defaultPageSize
	content.WriteString(paginationWithParams(page, totalPages, currentURL, typesFilter, coversParam))

	return h.render(c, content.String())
}

// LibrarySearch shows search form and results.
func (h *handler) LibrarySearch(c echo.Context) error {
	ctx := c.Request().Context()
	libraryID := c.Param("libraryId")
	baseURL := h.baseURL(c)
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}

	query := c.QueryParam("q")
	page := parsePageParam(c.QueryParam("page"))
	typesFilter := c.QueryParam("types")
	coversParam := c.QueryParam("covers")
	showCovers := coversParam == "on"

	libraryIDInt, err := httputil.ParamID(c, "libraryId", "Library")
	if err != nil {
		return err
	}

	r, err := h.requireLibrary(c, libraryIDInt)
	if err != nil {
		return err
	}
	typesFilter = typesFilterFor(r, typesFilter)

	sort := h.resolveSort(ctx, user.ID, libraryIDInt)

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString("<h1>Search</h1>")

	searchURL := fmt.Sprintf("%s/libraries/%s/search", baseURL, libraryID)
	filterOpts := books.ListBooksOptions{LibraryID: &libraryIDInt}
	if query != "" {
		filterOpts.Search = &query
	}
	fileTypes, err := h.reachService.DeliveredFileTypesWithBooks(ctx, r, filterOpts)
	if err != nil {
		return errors.WithStack(err)
	}
	content.WriteString(searchForm(searchURL, query))
	content.WriteString(filterBar(searchURL, fileTypes, typesFilter, coversParam, [2]string{"q", query}))

	if query != "" {
		// Search for books (paginated)
		booksResult, total, err := h.listBooksPaginated(ctx, r, books.ListBooksOptions{
			LibraryID: &libraryIDInt,
			Search:    &query,
			Sort:      sort,
		}, page, typesFilter)
		if err != nil {
			return errors.WithStack(err)
		}

		content.WriteString(fmt.Sprintf("<p>Found %d results</p>", total))

		for _, book := range booksResult {
			meta := formatBookMeta(book, r.Rules)
			bookURL := buildBookURL(baseURL, book.ID, coversParam)
			coverURL := getBookCoverURL(baseURL, book)
			content.WriteString(itemHTMLWithCover(book.Title, bookURL, meta, coverURL, showCovers))
		}

		// Pagination (preserve q + filter params)
		totalPages := (total + defaultPageSize - 1) / defaultPageSize
		content.WriteString(paginationWithParams(page, totalPages, searchURL, typesFilter, coversParam, [2]string{"q", query}))
	}

	return h.render(c, content.String())
}

// Download shows a book's page, with a download for each File the Device's
// Delivery Rules deliver, as the File Type they deliver it as.
func (h *handler) Download(c echo.Context) error {
	ctx := c.Request().Context()
	baseURL := h.baseURL(c)
	// Parse cover toggle
	coversParam := c.QueryParam("covers")
	showCover := coversParam == "on"

	bookIDInt, err := httputil.ParamID(c, "bookId", "Book")
	if err != nil {
		return err
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

	r, err := h.reach(c)
	if err != nil {
		return err
	}
	if err := h.requireBookInScope(c, r, book.LibraryID, book.ID, "Book"); err != nil {
		return err
	}

	if !slices.ContainsFunc(book.Files, func(f *models.File) bool { return f.FileRole == models.FileRoleMain }) {
		return errcodes.NotFound("File")
	}

	var content strings.Builder
	content.WriteString(navBar(baseURL + "/"))
	content.WriteString(fmt.Sprintf("<h1>%s</h1>", html.EscapeString(book.Title)))

	// Cover toggle (only show if book has a cover)
	hasCover := hasBookCover(book)
	if hasCover {
		currentURL := fmt.Sprintf("%s/download/%d", baseURL, book.ID)
		content.WriteString(coverToggle(currentURL, coversParam))

		// Show cover if enabled
		if showCover {
			coverURL := getBookCoverURL(baseURL, book)
			content.WriteString(fmt.Sprintf(`<p><img src="%s" alt="" style="max-width: 150px; max-height: 200px;"></p>`, html.EscapeString(coverURL)))
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
			content.WriteString(fmt.Sprintf("<p>By: %s</p>", html.EscapeString(strings.Join(authorNames, ", "))))
		}
	}

	if book.Description != nil {
		content.WriteString(descriptionHTML(*book.Description))
	}

	// Listings hide a book with nothing to deliver, but its page is still
	// reachable by URL.
	deliveries := delivery.Resolve(r.Rules, book).All()
	kindle := r.Device.DeviceType == devices.DeviceTypeKindle
	switch {
	case len(deliveries) == 0:
		content.WriteString(`<p><b>Not delivered to this Device.</b> Its Delivery Rules have no rule for this book's file types.</p>`)
	case kindle && deliveries[0].DeliveredFileType == models.FileTypeAZW3 && hasMainFile(book, models.FileTypeMOBI):
		// Putting MOBI first only helps a book that has a MOBI.
		content.WriteString(`<p><i>Some Kindles refuse AZW3 downloads. If yours does, put MOBI first in this Device's Delivery Rules.</i></p>`)
	}
	for _, d := range deliveries {
		content.WriteString(fileDownloadEntry(baseURL, book.Title, d, kindle))
	}

	return h.render(c, content.String())
}

// hasMainFile reports whether book has a main File of fileType.
func hasMainFile(book *models.Book, fileType string) bool {
	return slices.ContainsFunc(book.Files, func(f *models.File) bool {
		return f.FileType == fileType && f.FileRole == models.FileRoleMain
	})
}

// Cover serves a book cover image.
func (h *handler) Cover(c echo.Context) error {
	ctx := c.Request().Context()
	bookIDInt, err := httputil.ParamID(c, "bookId", "Book")
	if err != nil {
		return err
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

	r, err := h.scopeOnly(c)
	if err != nil {
		return err
	}
	if err := h.requireBookInScope(c, r, book.LibraryID, book.ID, "Book"); err != nil {
		return err
	}

	// Get the library to determine cover aspect ratio preference
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &book.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return covers.ServeBookCover(c, book.Files, library.CoverAspectRatio, covers.CacheControlNoCache, "Cover")
}

// DownloadFile serves a File delivered as its own File Type. A failed
// generation sends the Original File, per books.ResolveDeliveryDownload.
func (h *handler) DownloadFile(c echo.Context) error {
	return h.serveDownload(c, "")
}

// DownloadFileAs serves a File delivered as the File Type in the
// deliveredType param, such as "kepub". Delivered through a Conversion, a
// failed Conversion is an error, never the Original File in its place.
func (h *handler) DownloadFileAs(c echo.Context) error {
	deliveredType := c.Param("deliveredType")
	if !models.IsBuiltInFileType(deliveredType) {
		return errcodes.NotFound("File")
	}
	return h.serveDownload(c, deliveredType)
}

// DownloadFileKindle handles downloads for a Kindle's browser, whose URL ends
// in the filename (see kindleFilename), and whose extension names the File
// Type it is delivered as. A Kindle accepts a download only when its
// filename has an extension it knows, and Kindles have been reported
// rejecting some Content-Disposition filenames, so this route sends none and
// the Kindle names the file from the URL.
func (h *handler) DownloadFileKindle(c echo.Context) error {
	_, ext, found := strings.Cut(c.Param("filename"), ".")
	if !found {
		return errcodes.NotFound("File")
	}
	deliveredType, ok := models.DeliveredFileTypeForExtension(strings.ToLower(ext))
	if !ok {
		return errcodes.NotFound("File")
	}
	return h.serveDownload(c, deliveredType, httputil.WithAttachment(""))
}

// serveDownload serves the File named by the fileId param delivered as
// deliveredType ("" for its own File Type), per
// books.ResolveDeliveryDownload: a failed generation of its own File Type
// sends the Original File, and a failed Conversion is an error. opts apply
// after the download's defaults.
func (h *handler) serveDownload(c echo.Context, deliveredType string, opts ...httputil.ServeOption) error {
	ctx := c.Request().Context()
	fileID, err := httputil.ParamID(c, "fileId", "File")
	if err != nil {
		return err
	}

	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		ID: &fileID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	r, err := h.scopeOnly(c)
	if err != nil {
		return err
	}
	if err := h.requireBookInScope(c, r, file.LibraryID, file.BookID, "File"); err != nil {
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

	// Find the file with all relations from the book's files
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
	return download.Serve(c, opts...)
}

// Helper functions

// formatBookMeta returns a book's list-row metadata: its first author and
// the File Types the Device's Delivery Rules deliver it as.
func formatBookMeta(book *models.Book, rules []delivery.Rule) string {
	var parts []string
	for _, a := range book.Authors {
		if a.Person != nil {
			parts = append(parts, a.Person.Name)
			break
		}
	}

	var types []string
	for _, d := range delivery.Resolve(rules, book).All() {
		if label := models.FileTypeLabel(d.DeliveredFileType); !slices.Contains(types, label) {
			types = append(types, label)
		}
	}
	if len(types) > 0 {
		parts = append(parts, strings.Join(types, ", "))
	}

	return strings.Join(parts, " • ")
}

// typeMismatchBanner warns, on the device itself, when its browser looks
// like a Kindle or a Kobo but the Device's type is another. The User-Agent
// never changes what is served; the Device's type and rules decide that.
func typeMismatchBanner(deviceType, userAgent string) string {
	var looksLike string
	switch {
	case isKindle(userAgent) && deviceType != devices.DeviceTypeKindle:
		looksLike = devices.DeviceTypeKindle
	case isKobo(userAgent) && deviceType != devices.DeviceTypeKobo:
		looksLike = devices.DeviceTypeKobo
	default:
		return ""
	}
	label := html.EscapeString(devices.TypeLabel(looksLike))
	return fmt.Sprintf(`<div style="padding: 12px; margin: 8px 0; border: 2px solid #000;"><b>This looks like a %s, but this Device is set up as %s.</b> Downloads may not open here. Change its type to %s on its Device page in Shisho.</div>`,
		label, html.EscapeString(devices.TypeLabelWithArticle(deviceType)), label)
}

// isKindle reports whether a User-Agent is an e-ink Kindle's browser, which
// every firmware reports as "Kindle/<version>". Fire tablets browse with
// Silk, whose User-Agent never has "Kindle/", and have apps for other
// formats, so they are not treated as Kindles.
func isKindle(userAgent string) bool {
	return strings.Contains(strings.ToLower(userAgent), "kindle/")
}

// isKobo reports whether a User-Agent is a Kobo's browser.
func isKobo(userAgent string) bool {
	return strings.Contains(strings.ToLower(userAgent), "kobo")
}

// kindleFilename returns the filename that ends a file's Kindle download
// URL: the File's ID plus the extension of the File Type it is delivered
// as, which DownloadFileKindle reads the delivered File Type from. A Kindle
// keeps a reading position per filename, so the name never comes from the
// title: a download after a rename replaces the book in place and opens
// where the reader left off, where a new name would add a second copy that
// opens at the start. The Kindle's library shows the title inside the file.
func kindleFilename(f *models.File, deliveredType string) string {
	return fmt.Sprintf("Shisho_%d.%s", f.ID, models.DeliveredFileTypeExtension(deliveredType))
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

// fileDownloadEntry renders one delivered File's download block with its
// name, its File Type and size, and a link that downloads it as the File
// Type the rules deliver it as. A Kindle gets the URL ending in the filename
// it needs. Uses block-level elements with large tap targets for eReader
// compatibility (no flexbox).
func fileDownloadEntry(baseURL, bookTitle string, d delivery.Delivery, kindle bool) string {
	f := d.File
	displayName := f.ResolveTitle(bookTitle)

	var downloadURL string
	switch {
	case kindle:
		downloadURL = fmt.Sprintf("%s/file/%d/kindle/%s", baseURL, f.ID, kindleFilename(f, d.DeliveredFileType))
	case d.DeliveredFileType == f.FileType:
		downloadURL = fmt.Sprintf("%s/file/%d", baseURL, f.ID)
	default:
		downloadURL = fmt.Sprintf("%s/file/%d/%s", baseURL, f.ID, d.DeliveredFileType)
	}

	// Build metadata line: type badge + file size
	var metaParts []string
	metaParts = append(metaParts, models.FileTypeLabel(f.FileType))
	if f.FilesizeBytes > 0 {
		metaParts = append(metaParts, formatFileSize(f.FilesizeBytes))
	}

	var sb strings.Builder
	sb.WriteString(`<div style="padding: 12px 0; border-bottom: 1px solid #ccc;">`)
	sb.WriteString(fmt.Sprintf(`<div style="font-weight: bold;">%s</div>`, html.EscapeString(displayName)))
	sb.WriteString(fmt.Sprintf(`<div style="font-size: 0.9em; color: #666; margin: 4px 0;">%s</div>`, html.EscapeString(strings.Join(metaParts, " • "))))
	sb.WriteString(fmt.Sprintf(`<a href="%s" class="nav-btn" style="display: inline-block; margin-top: 8px;">Download %s</a>`, html.EscapeString(downloadURL), html.EscapeString(models.FileTypeLabel(d.DeliveredFileType))))
	if f.FileType == models.FileTypeCBZ || len(d.Chain) > 0 {
		sb.WriteString(`<div style="font-size: 0.9em; color: #666; margin-top: 4px;"><i>Note: The download may take a moment while the file is being prepared.</i></div>`)
	}
	sb.WriteString(`</div>`)

	return sb.String()
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

// typesFilterFor returns the type filter a page applies: typesFilter, or
// "" (All) when the rules deliver nothing as that File Type, as in a link
// saved before the rules changed, so the link still opens a full list.
func typesFilterFor(r *reach.Reach, typesFilter string) string {
	if typesFilter == "" || typesFilter == "all" || len(reach.DeliveredAs(r.Rules, strings.ToLower(typesFilter))) == 0 {
		return ""
	}
	return strings.ToLower(typesFilter)
}

// listBooksPaginated fetches a single page of books in scope that the
// Device's Delivery Rules deliver something from, narrowed to books they
// deliver as typesFilter's File Type unless it is empty or "all". A type
// they deliver nothing as, such as a source File Type in a link saved
// before the rules sent it as another type, matches no book. Returns the
// books for the requested page and the total count used to drive the
// pagination UI.
func (h *handler) listBooksPaginated(ctx context.Context, r *reach.Reach, opts books.ListBooksOptions, page int, typesFilter string) ([]*models.Book, int, error) {
	opts.ListID = r.ListID()
	opts.MainFileTypes = r.SourceFileTypes()
	if typesFilter != "" && typesFilter != "all" {
		opts.MainFileMatches = reach.DeliveredAs(r.Rules, strings.ToLower(typesFilter))
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
func ResolveShortURL(c echo.Context, deviceService *devices.Service) error {
	shortCode := c.Param("shortCode")

	device, err := deviceService.ResolveShortCode(c.Request().Context(), shortCode)
	if err != nil {
		return errors.WithStack(err)
	}
	if device == nil || !device.HasConnection(devices.ConnectionEReaderBrowser) {
		return errcodes.NotFound("Short URL")
	}

	// Refuse before revealing the Device's URL when the owner can no longer
	// use it.
	if _, err := deviceService.AuthenticateOwner(c.Request().Context(), device); err != nil {
		return errors.WithStack(err)
	}

	redirectURL := fmt.Sprintf("/ereader/key/%s/", device.Credential)
	return c.Redirect(http.StatusFound, redirectURL)
}
