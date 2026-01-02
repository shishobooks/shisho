package opds

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/uptrace/bun"
)

// Service handles OPDS feed generation.
type Service struct {
	db             *bun.DB
	bookService    *books.Service
	libraryService *libraries.Service
	seriesService  *series.Service
}

// NewService creates a new OPDS service.
func NewService(db *bun.DB) *Service {
	return &Service{
		db:             db,
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
		seriesService:  series.NewService(db),
	}
}

// parseFileTypes converts "epub+cbz" to []string{"epub", "cbz"}.
func parseFileTypes(types string) []string {
	if types == "" {
		return nil
	}
	return strings.Split(types, "+")
}

// BuildCatalogFeed builds the root navigation feed listing all libraries.
// If libraryIDs is non-nil, only libraries with those IDs are included.
func (svc *Service) BuildCatalogFeed(ctx context.Context, baseURL, fileTypes string, libraryIDs []int) (*Feed, error) {
	opts := libraries.ListLibrariesOptions{}
	if libraryIDs != nil {
		opts.LibraryIDs = libraryIDs
	}
	libs, err := svc.libraryService.ListLibraries(ctx, opts)
	if err != nil {
		return nil, err
	}

	feed := NewFeed(
		baseURL+"/"+fileTypes+"/catalog",
		"Shisho Libraries",
	)
	feed.Author = &Author{Name: "Shisho"}

	// Self link
	feed.AddLink(RelSelf, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)

	// Add library entries
	for _, lib := range libs {
		entry := NewEntry(
			fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, lib.ID),
			lib.Name,
		)
		entry.Updated = lib.UpdatedAt
		entry.Content = &Content{Type: "text", Value: "Browse " + lib.Name}
		entry.AddLink(RelSubsection, fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, lib.ID), MimeTypeNavigation)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibraryCatalogFeed builds a navigation feed for a specific library.
func (svc *Service) BuildLibraryCatalogFeed(ctx context.Context, baseURL, fileTypes string, libraryID int) (*Feed, error) {
	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)

	feed := NewFeed(libBase, lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, libBase, MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// All Books entry
	allBooksEntry := NewEntry(libBase+"/all", "All Books")
	allBooksEntry.Content = &Content{Type: "text", Value: "Browse all books in " + lib.Name}
	allBooksEntry.AddLink(RelSubsection, libBase+"/all", MimeTypeAcquisition)
	feed.AddEntry(allBooksEntry)

	// Series entry
	seriesEntry := NewEntry(libBase+"/series", "Series")
	seriesEntry.Content = &Content{Type: "text", Value: "Browse books by series"}
	seriesEntry.AddLink(RelSubsection, libBase+"/series", MimeTypeNavigation)
	feed.AddEntry(seriesEntry)

	// Authors entry
	authorsEntry := NewEntry(libBase+"/authors", "Authors")
	authorsEntry.Content = &Content{Type: "text", Value: "Browse books by author"}
	authorsEntry.AddLink(RelSubsection, libBase+"/authors", MimeTypeNavigation)
	feed.AddEntry(authorsEntry)

	return feed, nil
}

// BuildLibraryAllBooksFeed builds an acquisition feed with all books in a library.
func (svc *Service) BuildLibraryAllBooksFeed(ctx context.Context, baseURL, fileTypes string, libraryID, limit, offset int) (*Feed, error) {
	types := parseFileTypes(fileTypes)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:     &limit,
		Offset:    &offset,
		LibraryID: &libraryID,
		FileTypes: types,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)

	feed := NewFeed(libBase+"/all", "All Books - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/all?limit=%d&offset=%d", libBase, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase, MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, libBase+"/all", limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, fileTypes, libraryID, book, types)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibrarySeriesListFeed builds a navigation feed listing all series in a library.
func (svc *Service) BuildLibrarySeriesListFeed(ctx context.Context, baseURL, fileTypes string, libraryID, limit, offset int) (*Feed, error) {
	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	seriesResult, total, err := svc.seriesService.ListSeriesWithTotal(ctx, series.ListSeriesOptions{
		Limit:     &limit,
		Offset:    &offset,
		LibraryID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)

	feed := NewFeed(libBase+"/series", "Series - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/series?limit=%d&offset=%d", libBase, limit, offset), MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase, MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, libBase+"/series", limit, offset, total)

	// Add series entries
	for _, s := range seriesResult {
		entry := NewEntry(
			fmt.Sprintf("%s/series/%d", libBase, s.ID),
			s.Name,
		)
		entry.Updated = s.UpdatedAt
		if s.Description != nil {
			entry.Content = &Content{Type: "text", Value: *s.Description}
		} else {
			entry.Content = &Content{Type: "text", Value: fmt.Sprintf("%d books in series", s.BookCount)}
		}
		entry.AddLink(RelSubsection, fmt.Sprintf("%s/series/%d", libBase, s.ID), MimeTypeAcquisition)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibrarySeriesBooksFeed builds an acquisition feed with books in a series.
func (svc *Service) BuildLibrarySeriesBooksFeed(ctx context.Context, baseURL, fileTypes string, libraryID, seriesID, limit, offset int) (*Feed, error) {
	types := parseFileTypes(fileTypes)

	s, err := svc.seriesService.RetrieveSeries(ctx, series.RetrieveSeriesOptions{
		ID: &seriesID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:     &limit,
		Offset:    &offset,
		LibraryID: &libraryID,
		SeriesID:  &seriesID,
		FileTypes: types,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)

	feed := NewFeed(fmt.Sprintf("%s/series/%d", libBase, seriesID), s.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/series/%d?limit=%d&offset=%d", libBase, seriesID, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase+"/series", MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, fmt.Sprintf("%s/series/%d", libBase, seriesID), limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, fileTypes, libraryID, book, types)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// AuthorInfo holds aggregated author information.
type AuthorInfo struct {
	Name      string
	BookCount int
}

// ListAuthorsInLibrary lists distinct authors in a library with book counts.
func (svc *Service) ListAuthorsInLibrary(ctx context.Context, libraryID, limit, offset int) ([]AuthorInfo, int, error) {
	var authors []AuthorInfo

	// Get distinct authors with book counts using persons and authors tables
	err := svc.db.NewSelect().
		TableExpr("persons p").
		ColumnExpr("p.name").
		ColumnExpr("COUNT(DISTINCT a.book_id) as book_count").
		Join("INNER JOIN authors a ON a.person_id = p.id").
		Join("INNER JOIN books b ON b.id = a.book_id").
		Where("b.library_id = ?", libraryID).
		Group("p.id", "p.name").
		Order("p.sort_name ASC").
		Limit(limit).
		Offset(offset).
		Scan(ctx, &authors)
	if err != nil {
		return nil, 0, err
	}

	// Get total count
	var total int
	err = svc.db.NewSelect().
		TableExpr("(SELECT DISTINCT p.id FROM persons p INNER JOIN authors a ON a.person_id = p.id INNER JOIN books b ON b.id = a.book_id WHERE b.library_id = ?) as distinct_authors", libraryID).
		ColumnExpr("COUNT(*) as count").
		Scan(ctx, &total)
	if err != nil {
		return nil, 0, err
	}

	return authors, total, nil
}

// BuildLibraryAuthorsListFeed builds a navigation feed listing all authors in a library.
func (svc *Service) BuildLibraryAuthorsListFeed(ctx context.Context, baseURL, fileTypes string, libraryID, limit, offset int) (*Feed, error) {
	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	authors, total, err := svc.ListAuthorsInLibrary(ctx, libraryID, limit, offset)
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)

	feed := NewFeed(libBase+"/authors", "Authors - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/authors?limit=%d&offset=%d", libBase, limit, offset), MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase, MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, libBase+"/authors", limit, offset, total)

	// Add author entries
	for _, author := range authors {
		encodedName := url.PathEscape(author.Name)
		entry := NewEntry(
			fmt.Sprintf("%s/authors/%s", libBase, encodedName),
			author.Name,
		)
		entry.Content = &Content{Type: "text", Value: fmt.Sprintf("%d books", author.BookCount)}
		entry.AddLink(RelSubsection, fmt.Sprintf("%s/authors/%s", libBase, encodedName), MimeTypeAcquisition)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// ListBooksByAuthor lists books by a specific author in a library.
func (svc *Service) ListBooksByAuthor(ctx context.Context, libraryID int, authorName string, fileTypes []string, limit, offset int) ([]*models.Book, int, error) {
	var bookIDs []int

	// Get book IDs for this author using persons and authors tables
	q := svc.db.NewSelect().
		TableExpr("authors a").
		ColumnExpr("DISTINCT a.book_id").
		Join("INNER JOIN persons p ON p.id = a.person_id").
		Join("INNER JOIN books b ON b.id = a.book_id").
		Where("b.library_id = ? AND p.name = ?", libraryID, authorName)

	if len(fileTypes) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.In(fileTypes))
	}

	err := q.Scan(ctx, &bookIDs)
	if err != nil {
		return nil, 0, err
	}

	if len(bookIDs) == 0 {
		return []*models.Book{}, 0, nil
	}

	// Get books with pagination
	total := len(bookIDs)

	// Apply pagination to book IDs
	start := offset
	if start >= len(bookIDs) {
		return []*models.Book{}, total, nil
	}
	end := start + limit
	if end > len(bookIDs) {
		end = len(bookIDs)
	}
	paginatedIDs := bookIDs[start:end]

	// Fetch full book details
	booksResult, err := svc.bookService.ListBooks(ctx, books.ListBooksOptions{
		LibraryID: &libraryID,
		FileTypes: fileTypes,
	})
	if err != nil {
		return nil, 0, err
	}

	// Filter to only include books in paginatedIDs
	idSet := make(map[int]bool)
	for _, id := range paginatedIDs {
		idSet[id] = true
	}

	var filtered []*models.Book
	for _, book := range booksResult {
		if idSet[book.ID] {
			filtered = append(filtered, book)
		}
	}

	return filtered, total, nil
}

// BuildLibraryAuthorBooksFeed builds an acquisition feed with books by an author.
func (svc *Service) BuildLibraryAuthorBooksFeed(ctx context.Context, baseURL, fileTypes string, libraryID int, authorName string, limit, offset int) (*Feed, error) {
	types := parseFileTypes(fileTypes)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.ListBooksByAuthor(ctx, libraryID, authorName, types, limit, offset)
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)
	encodedName := url.PathEscape(authorName)

	feed := NewFeed(fmt.Sprintf("%s/authors/%s", libBase, encodedName), authorName+" - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/authors/%s?limit=%d&offset=%d", libBase, encodedName, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase+"/authors", MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, fmt.Sprintf("%s/authors/%s", libBase, encodedName), limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, fileTypes, libraryID, book, types)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibrarySearchFeed builds an acquisition feed with search results.
func (svc *Service) BuildLibrarySearchFeed(ctx context.Context, baseURL, fileTypes string, libraryID int, query string, limit, offset int) (*Feed, error) {
	types := parseFileTypes(fileTypes)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:     &limit,
		Offset:    &offset,
		LibraryID: &libraryID,
		FileTypes: types,
		Search:    &query,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)
	encodedQuery := url.QueryEscape(query)

	feed := NewFeed(
		fmt.Sprintf("%s/search?q=%s", libBase, encodedQuery),
		"Search: "+query+" - "+lib.Name,
	)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/search?q=%s&limit=%d&offset=%d", libBase, encodedQuery, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/"+fileTypes+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase, MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinksWithQuery(feed, libBase+"/search", "q="+encodedQuery, limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, fileTypes, libraryID, book, types)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibraryOpenSearchDescription builds an OpenSearch description for a library.
func (svc *Service) BuildLibraryOpenSearchDescription(baseURL, fileTypes string, libraryID int) *OpenSearchDescription {
	libBase := fmt.Sprintf("%s/%s/libraries/%d", baseURL, fileTypes, libraryID)
	return NewOpenSearchDescription(
		"Shisho",
		"Search the Shisho catalog",
		libBase+"/search?q={searchTerms}",
	)
}

// bookToEntry converts a Book model to an OPDS entry.
func (svc *Service) bookToEntry(baseURL, _ string, _ int, book *models.Book, types []string) Entry {
	entry := NewEntry(
		fmt.Sprintf("urn:shisho:book:%d", book.ID),
		book.Title,
	)
	entry.Updated = book.UpdatedAt
	entry.Published = book.CreatedAt

	// Authors
	for _, author := range book.Authors {
		if author.Person != nil {
			entry.Authors = append(entry.Authors, Author{Name: author.Person.Name})
		}
	}

	// Summary
	if book.Subtitle != nil {
		entry.Summary = *book.Subtitle
	}

	// Series info - show all series the book belongs to
	if len(book.BookSeries) > 0 {
		var seriesParts []string
		for _, bs := range book.BookSeries {
			if bs.Series != nil {
				if bs.SeriesNumber != nil {
					seriesParts = append(seriesParts, fmt.Sprintf("%s #%.0f", bs.Series.Name, *bs.SeriesNumber))
				} else {
					seriesParts = append(seriesParts, bs.Series.Name)
				}
			}
		}
		if len(seriesParts) > 0 {
			entry.Summary = strings.Join(seriesParts, " • ")
		}
	}

	// Extract API base from baseURL (baseURL is like "http://host/api/opds/v1", we need "http://host/api")
	apiBase := strings.TrimSuffix(baseURL, "/opds/v1")

	// Cover image link
	coverImage := book.ResolveCoverImage()
	if coverImage != "" {
		ext := filepath.Ext(coverImage)
		mimeType := CoverMimeType(ext)
		coverURL := fmt.Sprintf("%s/books/%d/cover", apiBase, book.ID)
		entry.AddImageLink(coverURL, mimeType)
		entry.AddThumbnailLink(coverURL, mimeType)
	}

	// Acquisition links for each file
	for _, file := range book.Files {
		// If filtering by types, only include matching files
		if len(types) > 0 && !containsString(types, file.FileType) {
			continue
		}

		mimeType := FileTypeMimeType(file.FileType)
		downloadURL := fmt.Sprintf("%s/opds/download/%d", apiBase, file.ID)
		entry.AddAcquisitionLink(downloadURL, mimeType)
	}

	return entry
}

// addPaginationLinks adds pagination links to a feed.
func addPaginationLinks(feed *Feed, baseURL string, limit, offset, total int) {
	if offset > 0 {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		feed.AddLink(RelPrevious, fmt.Sprintf("%s?limit=%d&offset=%d", baseURL, limit, prevOffset), MimeTypeAcquisition)
		feed.AddLink(RelFirst, fmt.Sprintf("%s?limit=%d&offset=0", baseURL, limit), MimeTypeAcquisition)
	}
	if offset+limit < total {
		feed.AddLink(RelNext, fmt.Sprintf("%s?limit=%d&offset=%d", baseURL, limit, offset+limit), MimeTypeAcquisition)
		lastOffset := ((total - 1) / limit) * limit
		feed.AddLink(RelLast, fmt.Sprintf("%s?limit=%d&offset=%d", baseURL, limit, lastOffset), MimeTypeAcquisition)
	}
}

// addPaginationLinksWithQuery adds pagination links with an additional query parameter.
func addPaginationLinksWithQuery(feed *Feed, baseURL, query string, limit, offset, total int) {
	if offset > 0 {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		feed.AddLink(RelPrevious, fmt.Sprintf("%s?%s&limit=%d&offset=%d", baseURL, query, limit, prevOffset), MimeTypeAcquisition)
		feed.AddLink(RelFirst, fmt.Sprintf("%s?%s&limit=%d&offset=0", baseURL, query, limit), MimeTypeAcquisition)
	}
	if offset+limit < total {
		feed.AddLink(RelNext, fmt.Sprintf("%s?%s&limit=%d&offset=%d", baseURL, query, limit, offset+limit), MimeTypeAcquisition)
		lastOffset := ((total - 1) / limit) * limit
		feed.AddLink(RelLast, fmt.Sprintf("%s?%s&limit=%d&offset=%d", baseURL, query, limit, lastOffset), MimeTypeAcquisition)
	}
}

// containsString checks if a slice contains a string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
