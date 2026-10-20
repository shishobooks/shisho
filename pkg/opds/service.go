package opds

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/seriesnum"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/uptrace/bun"
)

// Service handles OPDS feed generation.
type Service struct {
	db             *bun.DB
	bookService    *books.Service
	libraryService *libraries.Service
	listsService   *lists.Service
	seriesService  *series.Service
	peopleService  *people.Service
	reachService   *reach.Service
}

// NewService creates a new OPDS service around the shared books service.
func NewService(db *bun.DB, bookService *books.Service) *Service {
	return &Service{
		db:             db,
		bookService:    bookService,
		libraryService: libraries.NewService(db),
		listsService:   lists.NewService(db),
		seriesService:  series.NewService(db),
		peopleService:  people.NewService(db),
		reachService:   reach.NewService(db, bookService),
	}
}

// BuildCatalogFeed builds the root navigation feed for what r reaches: the
// libraries with something to deliver when withLibraries, then a Lists
// entry when withLists and the user owns or has been shared a list with
// something to deliver. A list-scoped Device gets an entry for its list
// instead, when the list has something to deliver. No entry opens empty.
func (svc *Service) BuildCatalogFeed(ctx context.Context, baseURL string, r *reach.Reach, withLibraries, withLists bool) (*Feed, error) {
	feed := NewFeed(
		baseURL+"/catalog",
		"Shisho Libraries",
	)
	feed.Author = &Author{Name: "Shisho"}

	// Self link
	feed.AddLink(RelSelf, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)

	if withLibraries {
		libs, err := svc.reachService.Libraries(ctx, r)
		if err != nil {
			return nil, err
		}
		for _, lib := range libs {
			entry := NewEntry(
				fmt.Sprintf("%s/libraries/%d", baseURL, lib.ID),
				lib.Name,
			)
			entry.Updated = lib.UpdatedAt
			entry.Content = &Content{Type: "text", Value: "Browse " + lib.Name}
			entry.AddLink(RelSubsection, fmt.Sprintf("%s/libraries/%d", baseURL, lib.ID), MimeTypeNavigation)
			feed.AddEntry(entry)
		}
	}

	if listID := r.ListID(); listID != nil {
		has, err := svc.reachService.HasBooks(ctx, r, books.ListBooksOptions{})
		if err != nil {
			return nil, err
		}
		if !has {
			return feed, nil
		}
		list, err := svc.listsService.RetrieveList(ctx, lists.RetrieveListOptions{ID: listID})
		if err != nil {
			return nil, err
		}
		listURL := fmt.Sprintf("%s/lists/%d", baseURL, list.ID)
		entry := NewEntry(listURL, list.Name)
		entry.Updated = list.UpdatedAt
		entry.AddLink(RelSubsection, listURL, MimeTypeAcquisition)
		feed.AddEntry(entry)
		return feed, nil
	}
	if !withLists {
		return feed, nil
	}

	hasLists, err := svc.listsService.HasLists(ctx, r.User.ID, r.Deliverable())
	if err != nil {
		return nil, err
	}
	if hasLists {
		listsURL := baseURL + "/lists"
		entry := NewEntry(listsURL, "Lists")
		entry.Content = &Content{Type: "text", Value: "Browse your lists"}
		entry.AddLink(RelSubsection, listsURL, MimeTypeNavigation)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildListsFeed builds a navigation feed of the lists the user owns or has
// been shared that hold something in r to deliver, so no list opens to an
// empty page. A list-scoped Device gets only its list.
func (svc *Service) BuildListsFeed(ctx context.Context, baseURL string, r *reach.Reach, limit, offset int) (*Feed, error) {
	userLists, total, err := svc.listsService.ListListsWithTotal(ctx, lists.ListListsOptions{
		UserID:      r.User.ID,
		ListID:      r.ListID(),
		Limit:       &limit,
		Offset:      &offset,
		Deliverable: r.Deliverable(),
	})
	if err != nil {
		return nil, err
	}

	catalogURL := baseURL + "/catalog"
	listsURL := baseURL + "/lists"

	feed := NewFeed(listsURL, "Lists")
	feed.Author = &Author{Name: "Shisho"}

	feed.AddLink(RelSelf, fmt.Sprintf("%s?limit=%d&offset=%d", listsURL, limit, offset), MimeTypeNavigation)
	feed.AddLink(RelStart, catalogURL, MimeTypeNavigation)
	feed.AddLink(RelUp, catalogURL, MimeTypeNavigation)

	addPaginationLinks(feed, listsURL, limit, offset, total)

	for _, list := range userLists {
		listURL := fmt.Sprintf("%s/%d", listsURL, list.ID)
		entry := NewEntry(listURL, list.Name)
		entry.Updated = list.UpdatedAt
		if list.Description != nil && *list.Description != "" {
			entry.Content = &Content{Type: "text", Value: *list.Description}
		}
		entry.AddLink(RelSubsection, listURL, MimeTypeAcquisition)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildListBooksFeed builds an acquisition feed of a list's books in the
// list's default sort, or of the books matching query when it is non-empty.
// Only books in r with something to deliver are included. The caller checks
// that the user can view the list.
func (svc *Service) BuildListBooksFeed(ctx context.Context, baseURL string, r *reach.Reach, listID int, query string, limit, offset int) (*Feed, error) {
	types := r.SourceFileTypes()
	libraryIDs := r.LibraryIDs()

	list, err := svc.listsService.RetrieveList(ctx, lists.RetrieveListOptions{ID: &listID})
	if err != nil {
		return nil, err
	}

	listBooks, total, err := svc.listsService.ListBooksWithTotal(ctx, lists.ListBooksOptions{
		ListID:        listID,
		LibraryIDs:    libraryIDs,
		MainFileTypes: types,
		Search:        query,
		Sort:          list.DefaultSort,
		Limit:         &limit,
		Offset:        &offset,
	})
	if err != nil {
		return nil, err
	}

	catalogURL := baseURL + "/catalog"
	listsURL := baseURL + "/lists"
	listURL := fmt.Sprintf("%s/%d", listsURL, listID)

	var feed *Feed
	if query == "" {
		feed = NewFeed(listURL, list.Name)
		feed.AddLink(RelSelf, fmt.Sprintf("%s?limit=%d&offset=%d", listURL, limit, offset), MimeTypeAcquisition)
		feed.AddLink(RelUp, listsURL, MimeTypeNavigation)
		addPaginationLinks(feed, listURL, limit, offset, total)
	} else {
		encodedQuery := url.QueryEscape(query)
		feed = NewFeed(fmt.Sprintf("%s/search?q=%s", listURL, encodedQuery), "Search: "+query+" - "+list.Name)
		feed.AddLink(RelSelf, fmt.Sprintf("%s/search?q=%s&limit=%d&offset=%d", listURL, encodedQuery, limit, offset), MimeTypeAcquisition)
		feed.AddLink(RelUp, listURL, MimeTypeAcquisition)
		addPaginationLinksWithQuery(feed, listURL+"/search", "q="+encodedQuery, limit, offset, total)
	}
	feed.Author = &Author{Name: "Shisho"}
	feed.AddLink(RelStart, catalogURL, MimeTypeNavigation)
	feed.AddLink(RelSearch, listURL+"/opensearch.xml", MimeTypeOpenSearch)

	// A list spans libraries, so each book takes its own library's cover
	// aspect ratio.
	for _, lb := range listBooks {
		if lb.Book == nil {
			continue
		}
		aspectRatio := ""
		if lb.Book.Library != nil {
			aspectRatio = lb.Book.Library.CoverAspectRatio
		}
		feed.AddEntry(svc.bookToEntry(baseURL, lb.Book, aspectRatio, r.Rules))
	}

	return feed, nil
}

// BuildLibraryCatalogFeed builds a navigation feed for a specific library.
// The Series and Authors entries are left out when nothing in r has one
// with something to deliver.
func (svc *Service) BuildLibraryCatalogFeed(ctx context.Context, baseURL string, r *reach.Reach, libraryID int) (*Feed, error) {
	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}
	sections, err := svc.reachService.Sections(ctx, r, libraryID)
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)

	feed := NewFeed(libBase, lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, libBase, MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// All Books entry
	allBooksEntry := NewEntry(libBase+"/all", "All Books")
	allBooksEntry.Content = &Content{Type: "text", Value: "Browse all books in " + lib.Name}
	allBooksEntry.AddLink(RelSubsection, libBase+"/all", MimeTypeAcquisition)
	feed.AddEntry(allBooksEntry)

	if sections.Series {
		seriesEntry := NewEntry(libBase+"/series", "Series")
		seriesEntry.Content = &Content{Type: "text", Value: "Browse books by series"}
		seriesEntry.AddLink(RelSubsection, libBase+"/series", MimeTypeNavigation)
		feed.AddEntry(seriesEntry)
	}

	if sections.Authors {
		authorsEntry := NewEntry(libBase+"/authors", "Authors")
		authorsEntry.Content = &Content{Type: "text", Value: "Browse books by author"}
		authorsEntry.AddLink(RelSubsection, libBase+"/authors", MimeTypeNavigation)
		feed.AddEntry(authorsEntry)
	}

	return feed, nil
}

// BuildLibraryAllBooksFeed builds an acquisition feed with all books in a library.
func (svc *Service) BuildLibraryAllBooksFeed(ctx context.Context, baseURL string, rules []delivery.Rule, libraryID, limit, offset int, sort []sortspec.SortLevel) (*Feed, error) {
	types := delivery.SourceFileTypes(rules)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:         &limit,
		Offset:        &offset,
		LibraryID:     &libraryID,
		MainFileTypes: types,
		Sort:          sort,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)

	feed := NewFeed(libBase+"/all", "All Books - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/all?limit=%d&offset=%d", libBase, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase, MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, libBase+"/all", limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, book, lib.CoverAspectRatio, rules)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibrarySeriesListFeed builds a navigation feed listing all series in a library.
func (svc *Service) BuildLibrarySeriesListFeed(ctx context.Context, baseURL string, rules []delivery.Rule, libraryID, limit, offset int) (*Feed, error) {
	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	seriesResult, total, err := svc.seriesService.ListSeriesWithTotal(ctx, series.ListSeriesOptions{
		Limit:         &limit,
		Offset:        &offset,
		LibraryID:     &libraryID,
		MainFileTypes: delivery.SourceFileTypes(rules),
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)

	feed := NewFeed(libBase+"/series", "Series - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/series?limit=%d&offset=%d", libBase, limit, offset), MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
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

// retrieveLibrarySeries loads a series and returns a 404 when it belongs to a
// different library than the feed's, so its name never appears in a feed for
// a library the caller may not access.
func (svc *Service) retrieveLibrarySeries(ctx context.Context, libraryID, seriesID int) (*models.Series, error) {
	s, err := svc.seriesService.RetrieveSeries(ctx, series.RetrieveSeriesOptions{
		ID: &seriesID,
	})
	if err != nil {
		return nil, err
	}
	if s.LibraryID != libraryID {
		return nil, errcodes.NotFound("Series")
	}
	return s, nil
}

// BuildLibrarySeriesBooksFeed builds an acquisition feed with books in a series.
func (svc *Service) BuildLibrarySeriesBooksFeed(ctx context.Context, baseURL string, rules []delivery.Rule, libraryID, seriesID, limit, offset int, sort []sortspec.SortLevel) (*Feed, error) {
	types := delivery.SourceFileTypes(rules)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	s, err := svc.retrieveLibrarySeries(ctx, libraryID, seriesID)
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:         &limit,
		Offset:        &offset,
		LibraryID:     &libraryID,
		SeriesID:      &seriesID,
		MainFileTypes: types,
		Sort:          sort,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)

	feed := NewFeed(fmt.Sprintf("%s/series/%d", libBase, seriesID), s.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/series/%d?limit=%d&offset=%d", libBase, seriesID, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase+"/series", MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, fmt.Sprintf("%s/series/%d", libBase, seriesID), limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, book, lib.CoverAspectRatio, rules)
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
// When fileTypes is non-nil, it counts only books with a main file of one of
// those types and leaves out authors with no such book, so a list never
// leads to an empty page; an empty list matches no author.
func (svc *Service) ListAuthorsInLibrary(ctx context.Context, libraryID int, fileTypes []string, limit, offset int) ([]AuthorInfo, int, error) {
	// authorBooks selects one row per (author, book) pair in the library.
	authorBooks := func() *bun.SelectQuery {
		q := svc.db.NewSelect().
			TableExpr("persons p").
			Join("INNER JOIN authors a ON a.person_id = p.id").
			Join("INNER JOIN books b ON b.id = a.book_id").
			Where("b.library_id = ?", libraryID)
		if fileTypes != nil {
			q = q.Where("b.id IN (SELECT book_id FROM files WHERE file_type IN (?) AND file_role = ?)", bun.List(fileTypes), models.FileRoleMain)
		}
		return q
	}

	var authors []AuthorInfo
	err := authorBooks().
		ColumnExpr("p.name").
		ColumnExpr("COUNT(DISTINCT a.book_id) as book_count").
		Group("p.id", "p.name").
		Order("p.sort_name ASC").
		Limit(int64(limit)).
		Offset(int64(offset)).
		Scan(ctx, &authors)
	if err != nil {
		return nil, 0, err
	}

	var total int
	err = svc.db.NewSelect().
		TableExpr("(?) AS distinct_authors", authorBooks().ColumnExpr("DISTINCT p.id")).
		ColumnExpr("COUNT(*) as count").
		Scan(ctx, &total)
	if err != nil {
		return nil, 0, err
	}

	return authors, total, nil
}

// BuildLibraryAuthorsListFeed builds a navigation feed listing all authors in a library.
func (svc *Service) BuildLibraryAuthorsListFeed(ctx context.Context, baseURL string, rules []delivery.Rule, libraryID, limit, offset int) (*Feed, error) {
	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	authors, total, err := svc.ListAuthorsInLibrary(ctx, libraryID, delivery.SourceFileTypes(rules), limit, offset)
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)

	feed := NewFeed(libBase+"/authors", "Authors - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/authors?limit=%d&offset=%d", libBase, limit, offset), MimeTypeNavigation)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
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
//
// Implementation note: the original version queried for unsorted book
// IDs, paginated those IDs in Go, then fetched the entire library's
// books with sort applied and filtered down to the paginated IDs. That
// pattern broke pagination (page 1 wasn't "first N in sort order") and
// scaled poorly. We now look up the person row (library-scoped via
// peopleService.RetrievePerson) and delegate to the books service's
// PersonID filter, which lets SQL apply the sort, limit, and offset
// together.
//
// Name-match semantics change: the old implementation used `p.name = ?`
// (case-sensitive under SQLite's default BINARY collation), whereas
// peopleService.RetrievePerson uses `p.name = ? COLLATE NOCASE`. This
// aligns with the creation-side invariant — `persons` has a UNIQUE
// index on (name COLLATE NOCASE, library_id), so there's at most one
// person per (library, name) regardless of case, and the canonical
// casing is whatever was first inserted. Any author URL Shisho
// generates itself is already canonical; the change only affects OPDS
// clients that hand-craft URLs with non-canonical casing — those now
// resolve instead of 404ing.
func (svc *Service) ListBooksByAuthor(ctx context.Context, libraryID int, authorName string, fileTypes []string, limit, offset int, sort []sortspec.SortLevel) ([]*models.Book, int, error) {
	person, err := svc.peopleService.RetrievePerson(ctx, people.RetrievePersonOptions{
		Name:      &authorName,
		LibraryID: &libraryID,
	})
	if err != nil {
		// "No such author in this library" yields an empty acquisition
		// feed rather than a 404 — preserves the previous behavior of
		// the unsorted-ID query, which returned empty when nothing
		// joined.
		if errors.Is(err, errcodes.NotFound("Person")) {
			return []*models.Book{}, 0, nil
		}
		return nil, 0, err
	}

	return svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:         &limit,
		Offset:        &offset,
		LibraryID:     &libraryID,
		PersonID:      &person.ID,
		MainFileTypes: fileTypes,
		Sort:          sort,
	})
}

// BuildLibraryAuthorBooksFeed builds an acquisition feed with books by an author.
func (svc *Service) BuildLibraryAuthorBooksFeed(ctx context.Context, baseURL string, rules []delivery.Rule, libraryID int, authorName string, limit, offset int, sort []sortspec.SortLevel) (*Feed, error) {
	types := delivery.SourceFileTypes(rules)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.ListBooksByAuthor(ctx, libraryID, authorName, types, limit, offset, sort)
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)
	encodedName := url.PathEscape(authorName)

	feed := NewFeed(fmt.Sprintf("%s/authors/%s", libBase, encodedName), authorName+" - "+lib.Name)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/authors/%s?limit=%d&offset=%d", libBase, encodedName, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase+"/authors", MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinks(feed, fmt.Sprintf("%s/authors/%s", libBase, encodedName), limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, book, lib.CoverAspectRatio, rules)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibrarySearchFeed builds an acquisition feed with search results.
func (svc *Service) BuildLibrarySearchFeed(ctx context.Context, baseURL string, rules []delivery.Rule, libraryID int, query string, limit, offset int, sort []sortspec.SortLevel) (*Feed, error) {
	types := delivery.SourceFileTypes(rules)

	lib, err := svc.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return nil, err
	}

	booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		Limit:         &limit,
		Offset:        &offset,
		LibraryID:     &libraryID,
		MainFileTypes: types,
		Search:        &query,
		Sort:          sort,
	})
	if err != nil {
		return nil, err
	}

	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)
	encodedQuery := url.QueryEscape(query)

	feed := NewFeed(
		fmt.Sprintf("%s/search?q=%s", libBase, encodedQuery),
		"Search: "+query+" - "+lib.Name,
	)
	feed.Author = &Author{Name: "Shisho"}

	// Links
	feed.AddLink(RelSelf, fmt.Sprintf("%s/search?q=%s&limit=%d&offset=%d", libBase, encodedQuery, limit, offset), MimeTypeAcquisition)
	feed.AddLink(RelStart, baseURL+"/catalog", MimeTypeNavigation)
	feed.AddLink(RelUp, libBase, MimeTypeNavigation)
	feed.AddLink(RelSearch, libBase+"/opensearch.xml", MimeTypeOpenSearch)

	// Pagination links
	addPaginationLinksWithQuery(feed, libBase+"/search", "q="+encodedQuery, limit, offset, total)

	// Add book entries
	for _, book := range booksResult {
		entry := svc.bookToEntry(baseURL, book, lib.CoverAspectRatio, rules)
		feed.AddEntry(entry)
	}

	return feed, nil
}

// BuildLibraryOpenSearchDescription builds an OpenSearch description for a library.
func (svc *Service) BuildLibraryOpenSearchDescription(baseURL string, libraryID int) *OpenSearchDescription {
	libBase := fmt.Sprintf("%s/libraries/%d", baseURL, libraryID)
	return NewOpenSearchDescription(
		"Shisho",
		"Search the Shisho catalog",
		libBase+"/search?q={searchTerms}",
	)
}

// BuildListOpenSearchDescription builds an OpenSearch description for
// searching within a list.
func (svc *Service) BuildListOpenSearchDescription(baseURL string, listID int) *OpenSearchDescription {
	listURL := fmt.Sprintf("%s/lists/%d", baseURL, listID)
	return NewOpenSearchDescription(
		"Shisho",
		"Search this list",
		listURL+"/search?q={searchTerms}",
	)
}

// bookToEntry converts a Book model to an OPDS entry with one acquisition
// link per File the rules deliver.
func (svc *Service) bookToEntry(baseURL string, book *models.Book, coverAspectRatio string, rules []delivery.Rule) Entry {
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
					seriesParts = append(seriesParts, fmt.Sprintf("%s #%s", bs.Series.Name, seriesnum.FormatRange(*bs.SeriesNumber, bs.SeriesNumberEnd)))
				} else {
					seriesParts = append(seriesParts, bs.Series.Name)
				}
			}
		}
		if len(seriesParts) > 0 {
			entry.Summary = strings.Join(seriesParts, " • ")
		}
	}

	// Dublin Core metadata. OPDS entries are per-book but language/publisher
	// are file-level in our model. Use the first main file (ordered by
	// file_type via the query) that has a non-empty value for each field.
	// Supplements don't represent the book — exclude them from the
	// priority chain so a stray supplement-level Language or Publisher
	// can't leak into the OPDS entry's book-level metadata.
	for _, file := range book.Files {
		if file.FileRole == models.FileRoleSupplement {
			continue
		}
		if entry.Language == "" && file.Language != nil && *file.Language != "" {
			entry.Language = *file.Language
		}
		if entry.Publisher == "" && file.Publisher != nil {
			entry.Publisher = file.Publisher.Name
		}
		if entry.Language != "" && entry.Publisher != "" {
			break
		}
	}

	// Download links live at the root, outside /opds/v1, and cover links
	// inside it, so they authenticate with Basic Auth: the books API uses
	// session cookies, and bare /books paths serve the SPA.
	apiBase := strings.TrimSuffix(baseURL, "/opds/v1")

	// Cover image link - select appropriate file based on cover aspect ratio
	coverFile := covers.SelectFile(book.Files, coverAspectRatio)
	if coverFile != nil && coverFile.CoverImageFilename != nil && *coverFile.CoverImageFilename != "" {
		ext := filepath.Ext(*coverFile.CoverImageFilename)
		mimeType := CoverMimeType(ext)
		coverURL := fmt.Sprintf("%s/books/%d/cover", baseURL, book.ID)
		entry.AddImageLink(coverURL, mimeType)
		entry.AddThumbnailLink(coverURL, mimeType)
	}

	for _, d := range delivery.Resolve(rules, book).All() {
		entry.AddAcquisitionLink(
			DownloadURL(apiBase, d.File.ID, d.DeliveredFileType),
			FileTypeMimeType(d.DeliveredFileType),
			models.FileTypeLabel(d.DeliveredFileType)+" · "+d.File.ResolveTitle(book.Title),
		)
	}

	return entry
}

// DownloadURL is the acquisition link for a File delivered as
// deliveredType. It ends in the delivered File Type's extension, since
// KOReader recognizes a download by its extension or content type, and a
// KePub's extension, .kepub.epub, is the only thing that tells it from an
// EPUB.
func DownloadURL(apiBase string, fileID int, deliveredType string) string {
	return fmt.Sprintf("%s/opds/download/%d.%s", apiBase, fileID, models.DeliveredFileTypeExtension(deliveredType))
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
