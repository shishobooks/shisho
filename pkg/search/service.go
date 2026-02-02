package search

import (
	"context"
	"strings"

	"github.com/pkg/errors"
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
			q = q.Where("book_id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.In(fileTypes))
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
		Where("book_id IN (?)", bun.In(bookIDs)).
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
		q = q.Where("book_id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.In(fileTypes))
	}

	var count int
	err := q.Scan(ctx, &count)
	return count, errors.WithStack(err)
}

// searchBooksByIdentifier searches for books with matching file identifier values (exact match).
func (svc *Service) searchBooksByIdentifier(ctx context.Context, query string, libraryID int, fileTypes []string, limit int) ([]BookSearchResult, error) {
	// Clean query for identifier search
	query = strings.TrimSpace(query)
	if query == "" {
		return []BookSearchResult{}, nil
	}

	q := svc.db.NewSelect().
		TableExpr("file_identifiers fi").
		ColumnExpr("DISTINCT b.id, b.library_id, b.title, b.subtitle").
		ColumnExpr("(SELECT GROUP_CONCAT(p.name, ', ') FROM authors a JOIN persons p ON p.id = a.person_id WHERE a.book_id = b.id ORDER BY a.sort_order) AS authors").
		Join("JOIN files f ON f.id = fi.file_id").
		Join("JOIN books b ON b.id = f.book_id").
		Where("fi.value = ?", query).
		Where("b.library_id = ?", libraryID).
		Limit(limit)

	if len(fileTypes) > 0 {
		q = q.Where("f.file_type IN (?)", bun.In(fileTypes))
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
		TableExpr("series_fts").
		ColumnExpr("series_id AS id, library_id, name").
		ColumnExpr("(SELECT COUNT(*) FROM book_series WHERE series_id = series_fts.series_id) AS book_count").
		Where("series_fts MATCH ?", ftsQuery).
		Where("library_id = ?", libraryID).
		Order("rank").
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
		TableExpr("persons_fts").
		ColumnExpr("person_id AS id, library_id, name, sort_name").
		Where("persons_fts MATCH ?", ftsQuery).
		Where("library_id = ?", libraryID).
		Order("rank").
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

	// Collect author names
	var authorNames []string
	for _, author := range book.Authors {
		if author.Person != nil {
			authorNames = append(authorNames, author.Person.Name)
		}
	}

	// Collect file names and narrators
	var filenames []string
	var narratorNames []string
	for _, file := range book.Files {
		filenames = append(filenames, file.Filepath)
		for _, narrator := range file.Narrators {
			if narrator.Person != nil {
				narratorNames = append(narratorNames, narrator.Person.Name)
			}
		}
	}

	// Collect series names
	var seriesNames []string
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			seriesNames = append(seriesNames, bs.Series.Name)
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
		ColumnExpr("(SELECT GROUP_CONCAT(p.name, ' ') FROM authors a JOIN persons p ON a.person_id = p.id WHERE a.book_id = b.id) AS authors").
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

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO series_fts (series_id, library_id, name, description, book_titles, book_authors)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		series.ID,
		series.LibraryID,
		series.Name,
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

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO persons_fts (person_id, library_id, name, sort_name)
		 VALUES (?, ?, ?, ?)`,
		person.ID, person.LibraryID, person.Name, person.SortName,
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

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO genres_fts (genre_id, library_id, name)
		 VALUES (?, ?, ?)`,
		genre.ID, genre.LibraryID, genre.Name,
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

	_, err = svc.db.ExecContext(ctx,
		`INSERT INTO tags_fts (tag_id, library_id, name)
		 VALUES (?, ?, ?)`,
		tag.ID, tag.LibraryID, tag.Name,
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

	// Rebuild books index
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		SELECT
			b.id,
			b.library_id,
			b.title,
			b.filepath,
			COALESCE(b.subtitle, ''),
			COALESCE((SELECT GROUP_CONCAT(p.name, ' ') FROM authors a JOIN persons p ON a.person_id = p.id WHERE a.book_id = b.id), ''),
			COALESCE((SELECT GROUP_CONCAT(f.filepath, ' ') FROM files f WHERE f.book_id = b.id), ''),
			COALESCE((SELECT GROUP_CONCAT(p.name, ' ') FROM files f JOIN narrators n ON n.file_id = f.id JOIN persons p ON n.person_id = p.id WHERE f.book_id = b.id), ''),
			COALESCE((SELECT GROUP_CONCAT(s.name, ' ') FROM book_series bs JOIN series s ON bs.series_id = s.id WHERE bs.book_id = b.id AND s.deleted_at IS NULL), '')
		FROM books b
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild series index
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO series_fts (series_id, library_id, name, description, book_titles, book_authors)
		SELECT
			s.id,
			s.library_id,
			s.name,
			COALESCE(s.description, ''),
			COALESCE((SELECT GROUP_CONCAT(b.title, ' ') FROM book_series bs JOIN books b ON bs.book_id = b.id WHERE bs.series_id = s.id), ''),
			COALESCE((SELECT GROUP_CONCAT(p.name, ' ') FROM book_series bs JOIN books b ON bs.book_id = b.id JOIN authors a ON a.book_id = b.id JOIN persons p ON a.person_id = p.id WHERE bs.series_id = s.id), '')
		FROM series s
		WHERE s.deleted_at IS NULL
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild persons index
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO persons_fts (person_id, library_id, name, sort_name)
		SELECT id, library_id, name, sort_name
		FROM persons
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild genres index
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO genres_fts (genre_id, library_id, name)
		SELECT id, library_id, name
		FROM genres
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	// Rebuild tags index
	_, err = svc.db.ExecContext(ctx, `
		INSERT INTO tags_fts (tag_id, library_id, name)
		SELECT id, library_id, name
		FROM tags
	`)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
