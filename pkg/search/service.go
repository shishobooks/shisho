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

// GlobalSearchSections picks the optional sections GlobalSearch fills. Books
// are always searched because the search route requires books:read; a
// section left out comes back as an empty array, not a missing key.
type GlobalSearchSections struct {
	Series bool
	People bool
}

// GlobalSearch searches a library's books, plus the series and people
// that sections asks for. Returns up to 5 results per resource type for
// popover display.
func (svc *Service) GlobalSearch(ctx context.Context, libraryID int, query string, sections GlobalSearchSections) (*GlobalSearchResponse, error) {
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

	series := []SeriesSearchResult{}
	if sections.Series {
		series, err = svc.searchSeriesInternal(ctx, ftsQuery, libraryID, globalSearchLimit, 0)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	people := []PersonSearchResult{}
	if sections.People {
		people, err = svc.searchPeopleInternal(ctx, ftsQuery, libraryID, globalSearchLimit, 0)
		if err != nil {
			return nil, errors.WithStack(err)
		}
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
			TableExpr("books_fts bf").
			ColumnExpr("bf.book_id AS id, bf.library_id, bf.title, bf.subtitle").
			ColumnExpr("(SELECT GROUP_CONCAT(DISTINCT p.name) FROM authors a JOIN persons p ON p.id = a.person_id WHERE a.book_id = bf.book_id) AS authors").
			Where("books_fts MATCH ?", ftsQuery).
			Where("bf.library_id = ?", libraryID).
			Order("bf.rank").
			Limit(remaining + len(seenIDs)). // Fetch extra to account for potential duplicates
			Offset(offset)

		if len(fileTypes) > 0 {
			q = q.Where("bf.book_id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.List(fileTypes))
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

// Each FTS table has one INSERT ... SELECT that computes its rows from the
// source tables. RebuildAllIndexes runs it unfiltered, and reindexRow, behind
// every per-entity method, appends a filter on the entity id, so an edit and a
// scan always write the same row. The Index* methods take a model for their callers' convenience
// but read only its ID: indexing never depends on which relations a caller
// happened to load.
//
// FTS rows are keyed by rowid equal to the entity id (see deleteFTSRow). The
// per-entity methods delete the row and then insert it outside any
// transaction, so every insert uses INSERT OR REPLACE: when another writer
// indexes the same entity in between, the insert finds that writer's row
// already holding the rowid and replaces it instead of failing on the
// conflict.
const (
	// books_fts includes person aliases in authors and narrators, and series
	// aliases in series_names.
	booksFTSInsert = `
		INSERT OR REPLACE INTO books_fts (rowid, book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		SELECT
			b.id AS rowid,
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
		FROM books b`

	// series_fts includes series aliases in name, and the titles and author
	// names (without aliases) of the Series' Books.
	seriesFTSInsert = `
		INSERT OR REPLACE INTO series_fts (rowid, series_id, library_id, name, description, book_titles, book_authors)
		SELECT
			s.id AS rowid,
			s.id,
			s.library_id,
			s.name || COALESCE(' ' || (SELECT GROUP_CONCAT(sa.name, ' ') FROM series_aliases sa WHERE sa.series_id = s.id), ''),
			COALESCE(s.description, ''),
			COALESCE((SELECT GROUP_CONCAT(b.title, ' ') FROM book_series bs JOIN books b ON bs.book_id = b.id WHERE bs.series_id = s.id), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (SELECT DISTINCT p.name FROM book_series bs JOIN books b ON bs.book_id = b.id JOIN authors a ON a.book_id = b.id JOIN persons p ON a.person_id = p.id WHERE bs.series_id = s.id)), '')
		FROM series s`

	personsFTSInsert = `
		INSERT OR REPLACE INTO persons_fts (rowid, person_id, library_id, name, sort_name)
		SELECT p.id AS rowid, p.id, p.library_id,
			p.name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM person_aliases pa WHERE pa.person_id = p.id), ''),
			p.sort_name
		FROM persons p`

	genresFTSInsert = `
		INSERT OR REPLACE INTO genres_fts (rowid, genre_id, library_id, name)
		SELECT g.id AS rowid, g.id, g.library_id,
			g.name || COALESCE(' ' || (SELECT GROUP_CONCAT(ga.name, ' ') FROM genre_aliases ga WHERE ga.genre_id = g.id), '')
		FROM genres g`

	tagsFTSInsert = `
		INSERT OR REPLACE INTO tags_fts (rowid, tag_id, library_id, name)
		SELECT t.id AS rowid, t.id, t.library_id,
			t.name || COALESCE(' ' || (SELECT GROUP_CONCAT(ta.name, ' ') FROM tag_aliases ta WHERE ta.tag_id = t.id), '')
		FROM tags t`

	publishersFTSInsert = `
		INSERT OR REPLACE INTO publishers_fts (rowid, publisher_id, library_id, name)
		SELECT p.id AS rowid, p.id, p.library_id,
			p.name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM publisher_aliases pa WHERE pa.publisher_id = p.id), '')
		FROM publishers p`
)

// ftsSource describes how one FTS table is filled: its insert, and the alias
// of the source table in that insert, which a per-entity reindex filters on.
type ftsSource struct {
	name   string
	insert string
	alias  string
}

var (
	booksFTS      = ftsSource{"books_fts", booksFTSInsert, "b"}
	seriesFTS     = ftsSource{"series_fts", seriesFTSInsert, "s"}
	personsFTS    = ftsSource{"persons_fts", personsFTSInsert, "p"}
	genresFTS     = ftsSource{"genres_fts", genresFTSInsert, "g"}
	tagsFTS       = ftsSource{"tags_fts", tagsFTSInsert, "t"}
	publishersFTS = ftsSource{"publishers_fts", publishersFTSInsert, "p"}

	allFTSTables = []ftsSource{booksFTS, seriesFTS, personsFTS, genresFTS, tagsFTS, publishersFTS}
)

// reindexRow rewrites one entity's row in table from its source tables. When
// the entity no longer exists the insert selects nothing, so the row is only
// deleted.
func (svc *Service) reindexRow(ctx context.Context, table ftsSource, id int) error {
	if err := svc.deleteFTSRow(ctx, table.name, id); err != nil {
		return err
	}
	_, err := svc.db.ExecContext(ctx, table.insert+" WHERE "+table.alias+".id = ?", id)
	return errors.WithStack(err)
}

// IndexBook adds or updates a book in the FTS index.
func (svc *Service) IndexBook(ctx context.Context, book *models.Book) error {
	return svc.ReindexBookByID(ctx, book.ID)
}

// DeleteFromBookIndex removes a book from the FTS index.
func (svc *Service) DeleteFromBookIndex(ctx context.Context, bookID int) error {
	return svc.deleteFTSRow(ctx, booksFTS.name, bookID)
}

// IndexSeries adds or updates a series in the FTS index.
func (svc *Service) IndexSeries(ctx context.Context, series *models.Series) error {
	return svc.ReindexSeriesByID(ctx, series.ID)
}

// DeleteFromSeriesIndex removes a series from the FTS index.
func (svc *Service) DeleteFromSeriesIndex(ctx context.Context, seriesID int) error {
	return svc.deleteFTSRow(ctx, seriesFTS.name, seriesID)
}

// IndexPerson adds or updates a person in the FTS index.
func (svc *Service) IndexPerson(ctx context.Context, person *models.Person) error {
	return svc.reindexRow(ctx, personsFTS, person.ID)
}

// DeleteFromPersonIndex removes a person from the FTS index.
func (svc *Service) DeleteFromPersonIndex(ctx context.Context, personID int) error {
	return svc.deleteFTSRow(ctx, personsFTS.name, personID)
}

// IndexGenre adds or updates a genre in the FTS index.
func (svc *Service) IndexGenre(ctx context.Context, genre *models.Genre) error {
	return svc.reindexRow(ctx, genresFTS, genre.ID)
}

// DeleteFromGenreIndex removes a genre from the FTS index.
func (svc *Service) DeleteFromGenreIndex(ctx context.Context, genreID int) error {
	return svc.deleteFTSRow(ctx, genresFTS.name, genreID)
}

// IndexTag adds or updates a tag in the FTS index.
func (svc *Service) IndexTag(ctx context.Context, tag *models.Tag) error {
	return svc.reindexRow(ctx, tagsFTS, tag.ID)
}

// DeleteFromTagIndex removes a tag from the FTS index.
func (svc *Service) DeleteFromTagIndex(ctx context.Context, tagID int) error {
	return svc.deleteFTSRow(ctx, tagsFTS.name, tagID)
}

// IndexPublisher adds or updates a publisher in the FTS index.
func (svc *Service) IndexPublisher(ctx context.Context, publisher *models.Publisher) error {
	return svc.reindexRow(ctx, publishersFTS, publisher.ID)
}

// DeleteFromPublisherIndex removes a publisher from the FTS index.
func (svc *Service) DeleteFromPublisherIndex(ctx context.Context, publisherID int) error {
	return svc.deleteFTSRow(ctx, publishersFTS.name, publisherID)
}

// ReindexBookByID rewrites one book's books_fts row from the database, or
// deletes it when the book no longer exists.
func (svc *Service) ReindexBookByID(ctx context.Context, bookID int) error {
	return svc.reindexRow(ctx, booksFTS, bookID)
}

// ReindexSeriesByID rewrites one series' series_fts row from the database, or
// deletes it when the series no longer exists.
func (svc *Service) ReindexSeriesByID(ctx context.Context, seriesID int) error {
	return svc.reindexRow(ctx, seriesFTS, seriesID)
}

// deleteFTSRow removes one entity's row from an FTS table. Every insert sets
// rowid to the entity id, so this is a rowid lookup. Filtering on the stored
// id column instead (book_id, series_id, ...) would scan the whole table,
// because those columns are UNINDEXED.
func (svc *Service) deleteFTSRow(ctx context.Context, table string, id int) error {
	_, err := svc.db.NewDelete().
		TableExpr(table).
		Where("rowid = ?", id).
		Exec(ctx)
	return errors.WithStack(err)
}

// RebuildAllIndexes rebuilds all FTS indexes from scratch in one transaction,
// so no search sees half-empty tables and a failure part way through leaves
// the previous index in place. The database pool has a single connection
// (pkg/database), so every other query waits until the transaction commits.
// A scan job calls it when it finishes or fails, and the worker calls it at
// startup when the last scan was cut short by shutdown.
func (svc *Service) RebuildAllIndexes(ctx context.Context) error {
	return errors.WithStack(svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, table := range allFTSTables {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table.name); err != nil {
				return errors.WithStack(err)
			}
		}
		for _, table := range allFTSTables {
			if _, err := tx.ExecContext(ctx, table.insert); err != nil {
				return errors.Wrapf(err, "rebuild %s", table.name)
			}
		}
		return nil
	}))
}
