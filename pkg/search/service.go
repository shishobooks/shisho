package search

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

const (
	globalSearchLimit = 5
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

// GlobalSearch searches across books, series, and people in a library.
// Returns up to 5 results per resource type for popover display.
func (svc *Service) GlobalSearch(ctx context.Context, libraryID int, query string) (*GlobalSearchResponse, error) {
	ftsQuery := BuildPrefixQuery(query)
	if ftsQuery == "" {
		return &GlobalSearchResponse{
			Books:  []BookSearchResult{},
			Series: []SeriesSearchResult{},
			People: []PersonSearchResult{},
		}, nil
	}

	// Search books
	books, err := svc.searchBooksInternal(ctx, ftsQuery, libraryID, nil, globalSearchLimit, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Search series
	series, err := svc.searchSeriesInternal(ctx, ftsQuery, libraryID, globalSearchLimit, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Search people
	people, err := svc.searchPeopleInternal(ctx, ftsQuery, libraryID, globalSearchLimit, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &GlobalSearchResponse{
		Books:  books,
		Series: series,
		People: people,
	}, nil
}

// SearchBooks searches books with optional file type filter.
func (svc *Service) SearchBooks(ctx context.Context, libraryID int, query string, fileTypes []string, limit, offset int) ([]BookSearchResult, int, error) {
	ftsQuery := BuildPrefixQuery(query)
	if ftsQuery == "" {
		return []BookSearchResult{}, 0, nil
	}

	books, err := svc.searchBooksInternal(ctx, ftsQuery, libraryID, fileTypes, limit, offset)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	// Get total count
	total, err := svc.countBooksInternal(ctx, ftsQuery, libraryID, fileTypes)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return books, total, nil
}

// SearchSeries searches series.
func (svc *Service) SearchSeries(ctx context.Context, libraryID int, query string, limit, offset int) ([]SeriesSearchResult, int, error) {
	ftsQuery := BuildPrefixQuery(query)
	if ftsQuery == "" {
		return []SeriesSearchResult{}, 0, nil
	}

	series, err := svc.searchSeriesInternal(ctx, ftsQuery, libraryID, limit, offset)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	// Get total count
	total, err := svc.countSeriesInternal(ctx, ftsQuery, libraryID)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return series, total, nil
}

// SearchPeople searches people.
func (svc *Service) SearchPeople(ctx context.Context, libraryID int, query string, limit, offset int) ([]PersonSearchResult, int, error) {
	ftsQuery := BuildPrefixQuery(query)
	if ftsQuery == "" {
		return []PersonSearchResult{}, 0, nil
	}

	people, err := svc.searchPeopleInternal(ctx, ftsQuery, libraryID, limit, offset)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	// Get total count
	total, err := svc.countPeopleInternal(ctx, ftsQuery, libraryID)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return people, total, nil
}

func (svc *Service) searchBooksInternal(ctx context.Context, ftsQuery string, libraryID int, fileTypes []string, limit, offset int) ([]BookSearchResult, error) {
	results := []BookSearchResult{}
	seenIDs := make(map[int]bool)

	// First, search by exact identifier match (only for first page to avoid complexity)
	if offset == 0 {
		idResults, err := svc.searchBooksByIdentifier(ctx, strings.TrimSuffix(ftsQuery, "*"), libraryID, fileTypes, limit)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, r := range idResults {
			results = append(results, r)
			seenIDs[r.ID] = true
		}
	}

	// Then do FTS search
	remaining := limit - len(results)
	if remaining > 0 {
		q := svc.db.NewSelect().
			TableExpr("books_fts").
			ColumnExpr("book_id AS id, library_id, title, subtitle, authors").
			Where("books_fts MATCH ?", ftsQuery).
			Where("library_id = ?", libraryID).
			Order("rank").
			Limit(remaining + len(seenIDs)). // Fetch extra to account for potential duplicates
			Offset(offset)

		if len(fileTypes) > 0 {
			q = q.Where("book_id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.List(fileTypes))
		}

		ftsResults := []BookSearchResult{}
		err := q.Scan(ctx, &ftsResults)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Add FTS results, skipping duplicates from identifier search
		for _, r := range ftsResults {
			if !seenIDs[r.ID] && len(results) < limit {
				results = append(results, r)
				seenIDs[r.ID] = true
			}
		}
	}

	// Populate file types for all results
	if err := svc.populateBookFileTypes(ctx, results); err != nil {
		return nil, errors.WithStack(err)
	}

	return results, nil
}

// populateBookFileTypes fetches and populates file types for a slice of book search results.
func (svc *Service) populateBookFileTypes(ctx context.Context, results []BookSearchResult) error {
	if len(results) == 0 {
		return nil
	}

	// Collect book IDs
	bookIDs := make([]int, len(results))
	for i, r := range results {
		bookIDs[i] = r.ID
	}

	// Query file types for all books in one query
	type bookFileType struct {
		BookID   int    `bun:"book_id"`
		FileType string `bun:"file_type"`
	}
	var fileTypes []bookFileType
	err := svc.db.NewSelect().
		TableExpr("files").
		Column("book_id", "file_type").
		Where("book_id IN (?)", bun.List(bookIDs)).
		GroupExpr("book_id, file_type").
		Scan(ctx, &fileTypes)
	if err != nil {
		return errors.WithStack(err)
	}

	// Build a map of book_id -> []file_type
	fileTypeMap := make(map[int][]string)
	for _, ft := range fileTypes {
		fileTypeMap[ft.BookID] = append(fileTypeMap[ft.BookID], ft.FileType)
	}

	// Populate file types in results
	for i := range results {
		results[i].FileTypes = fileTypeMap[results[i].ID]
	}

	return nil
}

func (svc *Service) countBooksInternal(ctx context.Context, ftsQuery string, libraryID int, fileTypes []string) (int, error) {
	q := svc.db.NewSelect().
		TableExpr("books_fts").
		ColumnExpr("COUNT(*)").
		Where("books_fts MATCH ?", ftsQuery).
		Where("library_id = ?", libraryID)

	if len(fileTypes) > 0 {
		q = q.Where("book_id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.List(fileTypes))
	}

	var count int
	err := q.Scan(ctx, &count)
	return count, errors.WithStack(err)
}

// searchBooksByIdentifier searches for books with matching file identifier values (exact match).
func (svc *Service) searchBooksByIdentifier(ctx context.Context, query string, libraryID int, fileTypes []string, limit int) ([]BookSearchResult, error) {
	// Match against all plausible normalized forms so callers searching with
	// any cosmetic variation (hyphens/spaces for ISBN, lowercase for ASIN,
	// urn:uuid: prefix for UUID) find values stored in canonical form, and
	// legacy rows that predate write-side normalization remain findable.
	searchValues := identifiers.CandidateForms(query)
	if len(searchValues) == 0 {
		return []BookSearchResult{}, nil
	}

	q := svc.db.NewSelect().
		TableExpr("file_identifiers fi").
		ColumnExpr("DISTINCT b.id, b.library_id, b.title, b.subtitle").
		ColumnExpr("(SELECT GROUP_CONCAT(DISTINCT p.name) FROM authors a JOIN persons p ON p.id = a.person_id WHERE a.book_id = b.id ORDER BY a.sort_order) AS authors").
		Join("JOIN files f ON f.id = fi.file_id").
		Join("JOIN books b ON b.id = f.book_id").
		Where("fi.value IN (?)", bun.List(searchValues)).
		Where("b.library_id = ?", libraryID).
		Limit(limit)

	if len(fileTypes) > 0 {
		q = q.Where("f.file_type IN (?)", bun.List(fileTypes))
	}

	results := []BookSearchResult{}
	err := q.Scan(ctx, &results)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return results, nil
}

func (svc *Service) searchSeriesInternal(ctx context.Context, ftsQuery string, libraryID int, limit, offset int) ([]SeriesSearchResult, error) {
	results := []SeriesSearchResult{}

	err := svc.db.NewSelect().
		TableExpr("series_fts sf").
		Join("JOIN series s ON s.id = sf.series_id").
		ColumnExpr("sf.series_id AS id, sf.library_id, s.name").
		ColumnExpr("(SELECT COUNT(*) FROM book_series WHERE series_id = sf.series_id) AS book_count").
		Where("series_fts MATCH ?", ftsQuery).
		Where("sf.library_id = ?", libraryID).
		Order("sf.rank").
		Limit(limit).
		Offset(offset).
		Scan(ctx, &results)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return results, nil
}

func (svc *Service) countSeriesInternal(ctx context.Context, ftsQuery string, libraryID int) (int, error) {
	var count int
	err := svc.db.NewSelect().
		TableExpr("series_fts").
		ColumnExpr("COUNT(*)").
		Where("series_fts MATCH ?", ftsQuery).
		Where("library_id = ?", libraryID).
		Scan(ctx, &count)
	return count, errors.WithStack(err)
}

func (svc *Service) searchPeopleInternal(ctx context.Context, ftsQuery string, libraryID int, limit, offset int) ([]PersonSearchResult, error) {
	results := []PersonSearchResult{}

	err := svc.db.NewSelect().
		TableExpr("persons_fts pf").
		Join("JOIN persons p ON p.id = pf.person_id").
		ColumnExpr("pf.person_id AS id, pf.library_id, p.name, p.sort_name").
		Where("persons_fts MATCH ?", ftsQuery).
		Where("pf.library_id = ?", libraryID).
		Order("pf.rank").
		Limit(limit).
		Offset(offset).
		Scan(ctx, &results)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return results, nil
}

func (svc *Service) countPeopleInternal(ctx context.Context, ftsQuery string, libraryID int) (int, error) {
	var count int
	err := svc.db.NewSelect().
		TableExpr("persons_fts").
		ColumnExpr("COUNT(*)").
		Where("persons_fts MATCH ?", ftsQuery).
		Where("library_id = ?", libraryID).
		Scan(ctx, &count)
	return count, errors.WithStack(err)
}

// IndexBook adds or updates a book in the FTS index.
func (svc *Service) IndexBook(ctx context.Context, book *models.Book) error {
	// First, delete any existing entry
	err := svc.DeleteFromBookIndex(ctx, book.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Collect author names and their aliases (deduplicated)
	seenAuthors := make(map[string]bool)
	seenAuthorIDs := make(map[int]bool)
	var authorNames []string
	for _, author := range book.Authors {
		if author.Person != nil && !seenAuthors[author.Person.Name] {
			authorNames = append(authorNames, author.Person.Name)
			seenAuthors[author.Person.Name] = true
		}
		if author.Person != nil && !seenAuthorIDs[author.PersonID] {
			seenAuthorIDs[author.PersonID] = true
			aliasNames, _ := svc.queryAliasNames(ctx, "person_aliases", "person_id", author.PersonID)
			for _, a := range aliasNames {
				if !seenAuthors[a] {
					authorNames = append(authorNames, a)
					seenAuthors[a] = true
				}
			}
		}
	}

	// Collect file names and narrators with aliases (deduplicated)
	var filenames []string
	seenNarrators := make(map[string]bool)
	seenNarratorIDs := make(map[int]bool)
	var narratorNames []string
	for _, file := range book.Files {
		filenames = append(filenames, file.Filepath)
		for _, narrator := range file.Narrators {
			if narrator.Person != nil && !seenNarrators[narrator.Person.Name] {
				narratorNames = append(narratorNames, narrator.Person.Name)
				seenNarrators[narrator.Person.Name] = true
			}
			if narrator.Person != nil && !seenNarratorIDs[narrator.PersonID] {
				seenNarratorIDs[narrator.PersonID] = true
				aliasNames, _ := svc.queryAliasNames(ctx, "person_aliases", "person_id", narrator.PersonID)
				for _, a := range aliasNames {
					if !seenNarrators[a] {
						narratorNames = append(narratorNames, a)
						seenNarrators[a] = true
					}
				}
			}
		}
	}

	// Collect series names with aliases
	var seriesNames []string
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			seriesNames = append(seriesNames, bs.Series.Name)
			aliasNames, _ := svc.queryAliasNames(ctx, "series_aliases", "series_id", bs.SeriesID)
			seriesNames = append(seriesNames, aliasNames...)
		}
	}

	subtitle := ""
	if book.Subtitle != nil {
		subtitle = *book.Subtitle
	}

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID,
		book.LibraryID,
		book.Title,
		book.Filepath,
		subtitle,
		strings.Join(authorNames, " "),
		strings.Join(filenames, " "),
		strings.Join(narratorNames, " "),
		strings.Join(seriesNames, " "),
	)
	return errors.WithStack(err)
}

// DeleteFromBookIndex removes a book from the FTS index.
func (svc *Service) DeleteFromBookIndex(ctx context.Context, bookID int) error {
	_, err := svc.db.NewDelete().
		TableExpr("books_fts").
		Where("book_id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// IndexSeries adds or updates a series in the FTS index.
func (svc *Service) IndexSeries(ctx context.Context, series *models.Series) error {
	// First, delete any existing entry
	err := svc.DeleteFromSeriesIndex(ctx, series.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Get books in this series for indexing book titles and authors
	var bookTitles []string
	var bookAuthors []string

	type bookInfo struct {
		Title   string
		Authors string
	}
	var books []bookInfo

	err = svc.db.NewSelect().
		TableExpr("books b").
		ColumnExpr("b.title").
		ColumnExpr("(SELECT GROUP_CONCAT(name, ' ') FROM (SELECT DISTINCT p.name FROM authors a JOIN persons p ON a.person_id = p.id WHERE a.book_id = b.id)) AS authors").
		Join("JOIN book_series bs ON bs.book_id = b.id").
		Where("bs.series_id = ?", series.ID).
		Scan(ctx, &books)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, b := range books {
		bookTitles = append(bookTitles, b.Title)
		if b.Authors != "" {
			bookAuthors = append(bookAuthors, b.Authors)
		}
	}

	description := ""
	if series.Description != nil {
		description = *series.Description
	}

	nameWithAliases, err := svc.nameWithAliases(ctx, "series_aliases", "series_id", series.ID, series.Name)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO series_fts (series_id, library_id, name, description, book_titles, book_authors)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		series.ID,
		series.LibraryID,
		nameWithAliases,
		description,
		strings.Join(bookTitles, " "),
		strings.Join(bookAuthors, " "),
	)
	return errors.WithStack(err)
}

// DeleteFromSeriesIndex removes a series from the FTS index.
func (svc *Service) DeleteFromSeriesIndex(ctx context.Context, seriesID int) error {
	_, err := svc.db.NewDelete().
		TableExpr("series_fts").
		Where("series_id = ?", seriesID).
		Exec(ctx)
	return errors.WithStack(err)
}

// IndexPerson adds or updates a person in the FTS index.
func (svc *Service) IndexPerson(ctx context.Context, person *models.Person) error {
	// First, delete any existing entry
	err := svc.DeleteFromPersonIndex(ctx, person.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	nameWithAliases, err := svc.nameWithAliases(ctx, "person_aliases", "person_id", person.ID, person.Name)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO persons_fts (person_id, library_id, name, sort_name)
		 VALUES (?, ?, ?, ?)`,
		person.ID, person.LibraryID, nameWithAliases, person.SortName,
	)
	return errors.WithStack(err)
}

// DeleteFromPersonIndex removes a person from the FTS index.
func (svc *Service) DeleteFromPersonIndex(ctx context.Context, personID int) error {
	_, err := svc.db.NewDelete().
		TableExpr("persons_fts").
		Where("person_id = ?", personID).
		Exec(ctx)
	return errors.WithStack(err)
}

// IndexGenre adds or updates a genre in the FTS index.
func (svc *Service) IndexGenre(ctx context.Context, genre *models.Genre) error {
	// First, delete any existing entry
	err := svc.DeleteFromGenreIndex(ctx, genre.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	nameWithAliases, err := svc.nameWithAliases(ctx, "genre_aliases", "genre_id", genre.ID, genre.Name)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO genres_fts (genre_id, library_id, name)
		 VALUES (?, ?, ?)`,
		genre.ID, genre.LibraryID, nameWithAliases,
	)
	return errors.WithStack(err)
}

// DeleteFromGenreIndex removes a genre from the FTS index.
func (svc *Service) DeleteFromGenreIndex(ctx context.Context, genreID int) error {
	_, err := svc.db.NewDelete().
		TableExpr("genres_fts").
		Where("genre_id = ?", genreID).
		Exec(ctx)
	return errors.WithStack(err)
}

// IndexTag adds or updates a tag in the FTS index.
func (svc *Service) IndexTag(ctx context.Context, tag *models.Tag) error {
	// First, delete any existing entry
	err := svc.DeleteFromTagIndex(ctx, tag.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	nameWithAliases, err := svc.nameWithAliases(ctx, "tag_aliases", "tag_id", tag.ID, tag.Name)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO tags_fts (tag_id, library_id, name)
		 VALUES (?, ?, ?)`,
		tag.ID, tag.LibraryID, nameWithAliases,
	)
	return errors.WithStack(err)
}

// DeleteFromTagIndex removes a tag from the FTS index.
func (svc *Service) DeleteFromTagIndex(ctx context.Context, tagID int) error {
	_, err := svc.db.NewDelete().
		TableExpr("tags_fts").
		Where("tag_id = ?", tagID).
		Exec(ctx)
	return errors.WithStack(err)
}

// ReindexBookByID re-indexes a single book in books_fts using the same SQL
// pattern as RebuildAllIndexes. Useful when related data changes (e.g., an
// author's or series' aliases are modified) without a full book model in hand.
func (svc *Service) ReindexBookByID(ctx context.Context, bookID int) error {
	_, err := svc.db.NewDelete().
		TableExpr("books_fts").
		Where("book_id = ?", bookID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		SELECT
			b.id,
			b.library_id,
			b.title,
			b.filepath,
			COALESCE(b.subtitle, ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT DISTINCT p.name FROM authors a JOIN persons p ON a.person_id = p.id WHERE a.book_id = b.id
				UNION
				SELECT DISTINCT pa.name FROM authors a JOIN person_aliases pa ON pa.person_id = a.person_id WHERE a.book_id = b.id
			)), ''),
			COALESCE((SELECT GROUP_CONCAT(f.filepath, ' ') FROM files f WHERE f.book_id = b.id), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT DISTINCT p.name FROM files f JOIN narrators n ON n.file_id = f.id JOIN persons p ON n.person_id = p.id WHERE f.book_id = b.id
				UNION
				SELECT DISTINCT pa.name FROM files f JOIN narrators n ON n.file_id = f.id JOIN person_aliases pa ON pa.person_id = n.person_id WHERE f.book_id = b.id
			)), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT s.name FROM book_series bs JOIN series s ON bs.series_id = s.id WHERE bs.book_id = b.id
				UNION
				SELECT sa.name FROM book_series bs JOIN series_aliases sa ON sa.series_id = bs.series_id WHERE bs.book_id = b.id
			)), '')
		FROM books b
		WHERE b.id = ?
	`, bookID)
	return errors.WithStack(err)
}

func (svc *Service) queryAliasNames(ctx context.Context, table, fkColumn string, resourceID int) ([]string, error) {
	var names []string
	err := svc.db.NewSelect().
		TableExpr(table).
		Column("name").
		Where(fkColumn+" = ?", resourceID).
		Scan(ctx, &names)
	return names, errors.WithStack(err)
}

func (svc *Service) nameWithAliases(ctx context.Context, table, fkColumn string, resourceID int, primaryName string) (string, error) {
	aliasNames, err := svc.queryAliasNames(ctx, table, fkColumn, resourceID)
	if err != nil {
		return primaryName, err
	}
	if len(aliasNames) == 0 {
		return primaryName, nil
	}
	return primaryName + " " + strings.Join(aliasNames, " "), nil
}

// RebuildAllIndexes rebuilds all FTS indexes from scratch.
// This should be called after a scan job completes.
func (svc *Service) RebuildAllIndexes(ctx context.Context) error {
	// Clear all indexes
	_, err := svc.db.ExecContext(ctx, "DELETE FROM books_fts")
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = svc.db.ExecContext(ctx, "DELETE FROM series_fts")
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = svc.db.ExecContext(ctx, "DELETE FROM persons_fts")
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = svc.db.ExecContext(ctx, "DELETE FROM genres_fts")
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = svc.db.ExecContext(ctx, "DELETE FROM tags_fts")
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild books index (includes person and series aliases in authors/narrators/series_names)
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		SELECT
			b.id,
			b.library_id,
			b.title,
			b.filepath,
			COALESCE(b.subtitle, ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT DISTINCT p.name FROM authors a JOIN persons p ON a.person_id = p.id WHERE a.book_id = b.id
				UNION
				SELECT DISTINCT pa.name FROM authors a JOIN person_aliases pa ON pa.person_id = a.person_id WHERE a.book_id = b.id
			)), ''),
			COALESCE((SELECT GROUP_CONCAT(f.filepath, ' ') FROM files f WHERE f.book_id = b.id), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT DISTINCT p.name FROM files f JOIN narrators n ON n.file_id = f.id JOIN persons p ON n.person_id = p.id WHERE f.book_id = b.id
				UNION
				SELECT DISTINCT pa.name FROM files f JOIN narrators n ON n.file_id = f.id JOIN person_aliases pa ON pa.person_id = n.person_id WHERE f.book_id = b.id
			)), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT s.name FROM book_series bs JOIN series s ON bs.series_id = s.id WHERE bs.book_id = b.id
				UNION
				SELECT sa.name FROM book_series bs JOIN series_aliases sa ON sa.series_id = bs.series_id WHERE bs.book_id = b.id
			)), '')
		FROM books b
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild series index (includes series aliases in name column)
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO series_fts (series_id, library_id, name, description, book_titles, book_authors)
		SELECT
			s.id,
			s.library_id,
			s.name || COALESCE(' ' || (SELECT GROUP_CONCAT(sa.name, ' ') FROM series_aliases sa WHERE sa.series_id = s.id), ''),
			COALESCE(s.description, ''),
			COALESCE((SELECT GROUP_CONCAT(b.title, ' ') FROM book_series bs JOIN books b ON bs.book_id = b.id WHERE bs.series_id = s.id), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (SELECT DISTINCT p.name FROM book_series bs JOIN books b ON bs.book_id = b.id JOIN authors a ON a.book_id = b.id JOIN persons p ON a.person_id = p.id WHERE bs.series_id = s.id)), '')
		FROM series s
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild persons index (includes person aliases in name column)
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO persons_fts (person_id, library_id, name, sort_name)
		SELECT id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM person_aliases pa WHERE pa.person_id = persons.id), ''),
			sort_name
		FROM persons
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild genres index (includes genre aliases in name column)
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO genres_fts (genre_id, library_id, name)
		SELECT id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(ga.name, ' ') FROM genre_aliases ga WHERE ga.genre_id = genres.id), '')
		FROM genres
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild tags index (includes tag aliases in name column)
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO tags_fts (tag_id, library_id, name)
		SELECT id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(ta.name, ' ') FROM tag_aliases ta WHERE ta.tag_id = tags.id), '')
		FROM tags
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
