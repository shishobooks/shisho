package books

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/uptrace/bun"
)

type RetrieveBookOptions struct {
	ID        *int
	Filepath  *string
	LibraryID *int
}

type ListBooksOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	SeriesID   *int
	PersonID   *int     // Filter to books authored by this person (joins through authors)
	FileTypes  []string // Filter by file types (e.g., ["epub", "cbz"])
	GenreIDs   []int    // Filter by genre IDs
	TagIDs     []int    // Filter by tag IDs
	Language   *string  // Filter by language tag (matches exact tag and subtag variants, e.g. "en" matches "en-US")
	IDs        []int    // Filter by specific book IDs
	Search     *string  // Search query for title/author

	// Sort overrides the default ordering. When nil and SeriesID is set,
	// books are ordered by series_number ASC + sort_title ASC. When nil
	// and SeriesID is not set, the service falls back to
	// sortspec.BuiltinDefault (date_added DESC) so all surfaces (REST,
	// OPDS, eReader, gallery) share the same "newest first" default.
	Sort []sortspec.SortLevel

	includeTotal  bool
	orderByRecent bool // Order by updated_at DESC instead of created_at ASC
}

type UpdateBookOptions struct {
	Columns       []string
	UpdateAuthors bool
	Authors       []mediafile.ParsedAuthor // Authors with roles for updating (requires UpdateAuthors to be true)
	OrganizeFiles bool                     // Whether to rename files when metadata changes
}

type RetrieveFileOptions struct {
	ID        *int
	Filepath  *string
	LibraryID *int
}

type ListFilesOptions struct {
	Limit          *int
	Offset         *int
	BookID         *int
	LibraryID      *int
	FilepathPrefix *string // Matches files whose filepath equals this value or is a descendant (prefix + "/")

	includeTotal bool
}

type UpdateFileOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateBook(ctx context.Context, book *models.Book) error {
	now := time.Now()
	if book.CreatedAt.IsZero() {
		book.CreatedAt = now
	}
	book.UpdatedAt = book.CreatedAt

	// Generate sort title if not provided
	if book.SortTitle == "" {
		book.SortTitle = sortname.ForTitle(book.Title)
		book.SortTitleSource = models.DataSourceFilepath // Auto-generated
	}
	// Ensure source is set if not already
	if book.SortTitleSource == "" {
		book.SortTitleSource = models.DataSourceFilepath
	}

	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Insert book.
		_, err := tx.
			NewInsert().
			Model(book).
			Returning("*").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Note: Authors are created separately via CreateAuthor after person creation

		// Insert files.
		for _, file := range book.Files {
			file.BookID = book.ID
			file.CreatedAt = book.CreatedAt
			file.UpdatedAt = book.UpdatedAt
		}
		if len(book.Files) > 0 {
			_, err := tx.
				NewInsert().
				Model(&book.Files).
				Returning("*").
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			// Set the first file as the primary file
			book.PrimaryFileID = &book.Files[0].ID
			_, err = tx.NewUpdate().
				Model(book).
				Column("primary_file_id").
				Where("id = ?", book.ID).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Note: Narrators are created separately via CreateNarrator after person creation

		return nil
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (svc *Service) RetrieveBook(ctx context.Context, opts RetrieveBookOptions) (*models.Book, error) {
	book := &models.Book{}

	q := svc.db.
		NewSelect().
		Model(book).
		Relation("Library").
		Relation("Authors", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("a.sort_order ASC")
		}).
		Relation("Authors.Person").
		Relation("BookSeries", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("bs.sort_order ASC")
		}).
		Relation("BookSeries.Series").
		Relation("BookGenres").
		Relation("BookGenres.Genre").
		Relation("BookTags").
		Relation("BookTags.Tag").
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.file_type ASC")
		}).
		Relation("Files.Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person").
		Relation("Files.Publisher").
		Relation("Files.Imprint").
		Relation("Files.Identifiers").
		Relation("Files.Chapters", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("ch.sort_order ASC")
		}).
		Relation("Files.Chapters.Children", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("ch.sort_order ASC")
		})

	if opts.ID != nil {
		q = q.Where("b.id = ?", *opts.ID)
	}
	if opts.Filepath != nil {
		q = q.Where("b.filepath = ?", *opts.Filepath)
	}
	if opts.LibraryID != nil {
		q = q.Where("b.library_id = ?", *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Book")
		}
		return nil, errors.WithStack(err)
	}

	return book, nil
}

// RetrieveBookByFilePath finds a book that contains a file with the specified filepath.
func (svc *Service) RetrieveBookByFilePath(ctx context.Context, filepath string, libraryID int) (*models.Book, error) {
	book := &models.Book{}

	q := svc.db.
		NewSelect().
		Model(book).
		Relation("Library").
		Relation("Authors", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("a.sort_order ASC")
		}).
		Relation("Authors.Person").
		Relation("BookSeries", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("bs.sort_order ASC")
		}).
		Relation("BookSeries.Series").
		Relation("BookGenres").
		Relation("BookGenres.Genre").
		Relation("BookTags").
		Relation("BookTags.Tag").
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.filepath ASC")
		}).
		Relation("Files.Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person").
		Relation("Files.Publisher").
		Relation("Files.Imprint").
		Relation("Files.Identifiers").
		Relation("Files.Chapters", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("ch.sort_order ASC")
		}).
		Relation("Files.Chapters.Children", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("ch.sort_order ASC")
		}).
		Join("INNER JOIN files fil ON fil.book_id = b.id").
		Where("fil.filepath = ? AND b.library_id = ?", filepath, libraryID)

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Book")
		}
		return nil, errors.WithStack(err)
	}

	return book, nil
}

func (svc *Service) ListBooks(ctx context.Context, opts ListBooksOptions) ([]*models.Book, error) {
	b, _, err := svc.listBooksWithTotal(ctx, opts)
	return b, errors.WithStack(err)
}

func (svc *Service) ListBooksWithTotal(ctx context.Context, opts ListBooksOptions) ([]*models.Book, int, error) {
	opts.includeTotal = true
	return svc.listBooksWithTotal(ctx, opts)
}

func (svc *Service) listBooksWithTotal(ctx context.Context, opts ListBooksOptions) ([]*models.Book, int, error) {
	books := []*models.Book{}
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&books).
		Relation("Library").
		Relation("Authors", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("a.sort_order ASC")
		}).
		Relation("Authors.Person").
		Relation("BookSeries", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("bs.sort_order ASC")
		}).
		Relation("BookSeries.Series").
		Relation("BookGenres").
		Relation("BookGenres.Genre").
		Relation("BookTags").
		Relation("BookTags.Tag").
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.file_type ASC")
		}).
		Relation("Files.Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person").
		Relation("Files.Publisher").
		Relation("Files.Imprint").
		Relation("Files.Identifiers")

	// Apply series filter first (affects ordering)
	if opts.SeriesID != nil {
		q = q.Join("INNER JOIN book_series bs_filter ON bs_filter.book_id = b.id").
			Where("bs_filter.series_id = ?", *opts.SeriesID)
	}

	// Apply ordering.
	// Precedence: orderByRecent (internal flag) > explicit Sort >
	// series-number (when SeriesID is set) > sortspec.BuiltinDefault.
	switch {
	case opts.orderByRecent:
		q = q.Order("b.updated_at DESC")

	case len(opts.Sort) > 0:
		for _, clause := range sortspec.OrderClauses(opts.Sort) {
			q = q.OrderExpr(clause.Expression)
		}
		// Stable tiebreaker: ensures deterministic pagination when the
		// user-specified sort levels have ties. Without this, SQLite's
		// order for tied rows is not guaranteed, which can cause books
		// to shift between pages.
		q = q.Order("b.id ASC")

	case opts.SeriesID != nil:
		// When filtering by series, order by series_number then sort_title.
		// Series listings are a distinct use case from the general "newest
		// first" fallback — readers of a series expect #1, #2, #3 order,
		// not most-recently-added.
		q = q.Order("bs_filter.series_number ASC", "b.sort_title ASC")

	default:
		// No explicit caller Sort, not a series listing, not the internal
		// recent-flag path — fall back to the builtin default ("date_added
		// DESC"). Applying it here means the /books REST handler, OPDS
		// feeds, and the eReader browser all show the same "newest first"
		// ordering when neither the URL nor a stored preference overrides
		// it. Keep a stable id tiebreaker for the same pagination reason
		// as the explicit-sort branch.
		for _, clause := range sortspec.OrderClauses(sortspec.BuiltinDefault()) {
			q = q.OrderExpr(clause.Expression)
		}
		q = q.Order("b.id ASC")
	}

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}
	if opts.LibraryID != nil {
		q = q.Where("b.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("b.library_id IN (?)", bun.List(opts.LibraryIDs))
	}

	// Filter by specific book IDs
	if len(opts.IDs) > 0 {
		q = q.Where("b.id IN (?)", bun.List(opts.IDs))
	}

	// Filter by author (person). Subquery rather than JOIN so it composes
	// cleanly with the user-specified Sort — joining `authors` into the
	// outer query would require disambiguating ORDER BY references and
	// risks duplicate rows when a person appears multiple times on the
	// same book. The `authors` UNIQUE index is on
	// (book_id, person_id, role), so a single person CAN legitimately
	// appear on one book under multiple roles (e.g., writer + inker on
	// a comic). The `SELECT DISTINCT book_id` — not the UNIQUE index —
	// is what guarantees each book appears at most once in the result.
	if opts.PersonID != nil {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM authors WHERE person_id = ?)", *opts.PersonID)
	}

	// Filter by file types
	if len(opts.FileTypes) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.List(opts.FileTypes))
	}

	// Filter by genre IDs
	if len(opts.GenreIDs) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM book_genres WHERE genre_id IN (?))", bun.List(opts.GenreIDs))
	}

	// Filter by tag IDs
	if len(opts.TagIDs) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM book_tags WHERE tag_id IN (?))", bun.List(opts.TagIDs))
	}

	// Filter by language (exact match + subtag variants, e.g., "en" matches "en-US", "en-GB")
	if opts.Language != nil && *opts.Language != "" {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM files WHERE language = ? OR language LIKE ?)", *opts.Language, *opts.Language+"-%")
	}

	// Search using FTS5
	if opts.Search != nil && *opts.Search != "" {
		ftsQuery := buildFTSPrefixQuery(*opts.Search)
		if ftsQuery != "" {
			q = q.Where("b.id IN (SELECT book_id FROM books_fts WHERE books_fts MATCH ?)", ftsQuery)
		}
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return books, total, nil
}

func (svc *Service) UpdateBook(ctx context.Context, book *models.Book, opts UpdateBookOptions) error {
	// Check if there's any actual database work to do
	hasDBWork := len(opts.Columns) > 0 || opts.UpdateAuthors

	if !hasDBWork && !opts.OrganizeFiles {
		return nil
	}

	// Only run transaction if there's database work to do
	if hasDBWork {
		err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
			if opts.UpdateAuthors {
				// Delete all previous authors associations.
				// Note: The actual Person records are not deleted - they may be referenced by other books.
				// AuthorNames in opts should be used by the caller to create new Author entries
				// after calling personService.FindOrCreatePerson.
				_, err := tx.
					NewDelete().
					Model((*models.Author)(nil)).
					Where("book_id = ?", book.ID).
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}

			// Update updated_at.
			now := time.Now()
			book.UpdatedAt = now
			columns := append(opts.Columns, "updated_at")

			_, err := tx.
				NewUpdate().
				Model(book).
				Column(columns...).
				WherePK().
				Exec(ctx)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return errcodes.NotFound("Book")
				}
				return errors.WithStack(err)
			}

			return nil
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Handle file organization if requested
	if opts.OrganizeFiles {
		err := svc.organizeBookFiles(ctx, book)
		if err != nil {
			// Log error but don't fail the update
			log := logger.FromContext(ctx)
			log.Error("failed to organize book files after update", logger.Data{
				"book_id": book.ID,
				"error":   err.Error(),
			})
		}
	}

	return nil
}

func (svc *Service) CreateFile(ctx context.Context, file *models.File) error {
	now := time.Now()
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	file.UpdatedAt = file.CreatedAt

	// Insert file.
	_, err := svc.db.
		NewInsert().
		Model(file).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// If this is the first file for the book, set it as primary
	var book models.Book
	err = svc.db.NewSelect().
		Model(&book).
		Where("id = ?", file.BookID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if book.PrimaryFileID == nil {
		book.PrimaryFileID = &file.ID
		_, err = svc.db.NewUpdate().
			Model(&book).
			Column("primary_file_id").
			Where("id = ?", book.ID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Note: FileNarrators are created separately via CreateFileNarrator after person creation

	return nil
}

// DeleteFileIdentifiers deletes all identifiers for a file.
func (svc *Service) DeleteFileIdentifiers(ctx context.Context, fileID int) error {
	_, err := svc.db.NewDelete().Model((*models.FileIdentifier)(nil)).Where("file_id = ?", fileID).Exec(ctx)
	return errors.WithStack(err)
}

// escapeLikePattern escapes the SQLite LIKE wildcards (%, _) and the escape
// character (\) so a raw filesystem path can be used as a literal prefix in a
// LIKE clause via `ESCAPE '\'`.
func escapeLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

func (svc *Service) RetrieveFile(ctx context.Context, opts RetrieveFileOptions) (*models.File, error) {
	file := &models.File{}

	q := svc.db.
		NewSelect().
		Model(file).
		Relation("Book").
		Relation("Identifiers").
		Relation("Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Narrators.Person")

	if opts.ID != nil {
		q = q.Where("f.id = ?", *opts.ID)
	}
	if opts.Filepath != nil {
		q = q.Where("f.filepath = ?", *opts.Filepath)
	}
	if opts.LibraryID != nil {
		q = q.Where("f.library_id = ?", *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("File")
		}
		return nil, errors.WithStack(err)
	}

	return file, nil
}

// RetrieveFileWithRelations retrieves a file with all relations needed for
// sidecar writing and fingerprint generation (Narrators, Identifiers, Publisher, Imprint).
// Use this instead of RetrieveFile when you need to call WriteFileSidecarFromModel
// or ComputeFingerprint.
func (svc *Service) RetrieveFileWithRelations(ctx context.Context, fileID int) (*models.File, error) {
	file := &models.File{}

	err := svc.db.
		NewSelect().
		Model(file).
		Relation("Book").
		Relation("Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Narrators.Person").
		Relation("Identifiers").
		Relation("Publisher").
		Relation("Imprint").
		Relation("Chapters", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("ch.sort_order ASC")
		}).
		Relation("Chapters.Children", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("ch.sort_order ASC")
		}).
		Where("f.id = ?", fileID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("File")
		}
		return nil, errors.WithStack(err)
	}

	return file, nil
}

func (svc *Service) ListFiles(ctx context.Context, opts ListFilesOptions) ([]*models.File, error) {
	b, _, err := svc.listFilesWithTotal(ctx, opts)
	return b, errors.WithStack(err)
}

func (svc *Service) ListFilesWithTotal(ctx context.Context, opts ListFilesOptions) ([]*models.File, int, error) {
	opts.includeTotal = true
	return svc.listFilesWithTotal(ctx, opts)
}

func (svc *Service) listFilesWithTotal(ctx context.Context, opts ListFilesOptions) ([]*models.File, int, error) {
	files := []*models.File{}
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&files).
		Relation("Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Narrators.Person").
		Order("f.created_at ASC")

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}
	if opts.BookID != nil {
		q = q.Where("f.book_id = ?", *opts.BookID)
	}
	if opts.LibraryID != nil {
		q = q.Where("f.library_id = ?", *opts.LibraryID)
	}
	if opts.FilepathPrefix != nil {
		// Match the directory itself (should not happen for files) or any descendant.
		// Escape LIKE wildcards so paths containing % or _ don't over-match. The
		// path separator is escaped too — on Windows it's `\`, which is our
		// ESCAPE char, so an unescaped separator would turn the trailing % into
		// a literal and silently match nothing.
		escaped := escapeLikePattern(*opts.FilepathPrefix) + escapeLikePattern(string(os.PathSeparator)) + "%"
		q = q.Where("f.filepath = ? OR f.filepath LIKE ? ESCAPE '\\'", *opts.FilepathPrefix, escaped)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return files, total, nil
}

func (svc *Service) UpdateFile(ctx context.Context, file *models.File, opts UpdateFileOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	// Update updated_at.
	now := time.Now()
	file.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(file).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("File")
		}
		return errors.WithStack(err)
	}

	return nil
}

// GetFirstBookInSeriesByID returns the first book in a series, ordered by series number.
func (svc *Service) GetFirstBookInSeriesByID(ctx context.Context, seriesID int) (*models.Book, error) {
	var book models.Book

	err := svc.db.
		NewSelect().
		Model(&book).
		Relation("Files").
		Join("INNER JOIN book_series bs ON bs.book_id = b.id").
		Where("bs.series_id = ?", seriesID).
		Order("bs.series_number ASC", "b.title ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Series")
		}
		return nil, errors.WithStack(err)
	}

	return &book, nil
}

// OrganizeBookFiles is the public entry point for triggering file organization.
// It checks the library's OrganizeFileStructure setting internally and returns
// early if disabled.
func (svc *Service) OrganizeBookFiles(ctx context.Context, book *models.Book) error {
	return svc.organizeBookFiles(ctx, book)
}

// organizeBookFiles renames files and folders based on updated book metadata.
func (svc *Service) organizeBookFiles(ctx context.Context, book *models.Book) error {
	log := logger.FromContext(ctx)
	now := time.Now()

	// Get the library with paths to check if organize_file_structure is enabled
	// and to determine if files are at root level
	var library models.Library
	err := svc.db.NewSelect().
		Model(&library).
		Relation("LibraryPaths", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("filepath ASC")
		}).
		Where("l.id = ?", book.LibraryID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// Only proceed if organize_file_structure is enabled
	if !library.OrganizeFileStructure {
		return nil
	}

	// Get all files for this book
	files, err := svc.ListFiles(ctx, ListFilesOptions{
		BookID: &book.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if len(files) == 0 {
		return nil
	}

	// Get author names from Authors
	authorNames := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		if a.Person != nil {
			authorNames = append(authorNames, a.Person.Name)
		}
	}

	// Get series number from first BookSeries entry (if any)
	var seriesNumber *float64
	if len(book.BookSeries) > 0 {
		seriesNumber = book.BookSeries[0].SeriesNumber
	}

	// Create organized name options from current book metadata
	organizeOpts := fileutils.OrganizedNameOptions{
		AuthorNames:  authorNames,
		Title:        book.Title,
		SeriesNumber: seriesNumber,
	}

	// Track path updates for database
	var pathUpdates []struct {
		fileID         int
		oldPath        string
		newPath        string
		coverImagePath *string // old cover image path (filename only), nil if none
	}

	// Check if this is a directory-based book or root-level files
	isDirectoryBased := filepath.Dir(files[0].Filepath) == book.Filepath

	// Check if this is a root-level book (files directly in a library path, not yet organized into folders)
	isRootLevelBook := false
	for _, libraryPath := range library.LibraryPaths {
		if filepath.Dir(files[0].Filepath) == libraryPath.Filepath {
			isRootLevelBook = true
			break
		}
	}

	if isDirectoryBased {
		// For directory-based books, rename the folder and update all file paths
		newFolderPath, err := fileutils.RenameOrganizedFolder(book.Filepath, organizeOpts)
		if err != nil {
			return errors.WithStack(err)
		}

		folderRenamed := newFolderPath != book.Filepath
		if folderRenamed {
			log.Info("renamed book folder", logger.Data{
				"old_path": book.Filepath,
				"new_path": newFolderPath,
			})

			// Delete old sidecar file (it has the old folder name in its filename)
			// The old sidecar is now at: newFolderPath/oldFolderName.metadata.json
			oldFolderName := filepath.Base(book.Filepath)
			oldSidecarPath := filepath.Join(newFolderPath, oldFolderName+".metadata.json")
			if err := os.Remove(oldSidecarPath); err != nil && !os.IsNotExist(err) {
				log.Warn("failed to remove old sidecar", logger.Data{
					"path":  oldSidecarPath,
					"error": err.Error(),
				})
			}

			// Update book filepath
			book.Filepath = newFolderPath
			book.UpdatedAt = now

			// Update book in database
			_, err = svc.db.NewUpdate().
				Model(book).
				Column("filepath", "updated_at").
				WherePK().
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			// Write a fresh book sidecar at the renamed folder so the
			// organized book is never left sidecar-less — the old-named
			// sidecar was just deleted above.
			if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
				log.Warn("failed to write book sidecar after folder rename", logger.Data{
					"book_id": book.ID,
					"path":    book.Filepath,
					"error":   err.Error(),
				})
			}
		}

		// Rename files inside the folder (whether or not folder was renamed)
		for _, file := range files {
			// Calculate the current path (after potential folder rename)
			currentPath := file.Filepath
			if folderRenamed {
				currentPath = strings.Replace(file.Filepath, filepath.Dir(file.Filepath), newFolderPath, 1)
			}

			// Set file type, title, and narrator names for proper naming
			organizeOpts.FileType = file.FileType
			// Use file.Name for title if available, otherwise book.Title
			if file.Name != nil && *file.Name != "" {
				organizeOpts.Title = *file.Name
			} else {
				organizeOpts.Title = book.Title
			}
			organizeOpts.NarratorNames = nil
			for _, n := range file.Narrators {
				if n.Person != nil {
					organizeOpts.NarratorNames = append(organizeOpts.NarratorNames, n.Person.Name)
				}
			}

			// Rename the file to the organized name
			newPath, err := fileutils.RenameOrganizedFile(currentPath, organizeOpts)
			if err != nil {
				log.Error("failed to rename file in folder", logger.Data{
					"file_id": file.ID,
					"path":    currentPath,
					"error":   err.Error(),
				})
				// If file rename failed but folder was renamed, still track the folder path change
				if folderRenamed && currentPath != file.Filepath {
					pathUpdates = append(pathUpdates, struct {
						fileID         int
						oldPath        string
						newPath        string
						coverImagePath *string
					}{file.ID, file.Filepath, currentPath, file.CoverImageFilename})
				}
				continue
			}

			// Track path update if anything changed
			if newPath != file.Filepath {
				log.Info("renamed file", logger.Data{
					"file_id":  file.ID,
					"old_path": file.Filepath,
					"new_path": newPath,
				})
				pathUpdates = append(pathUpdates, struct {
					fileID         int
					oldPath        string
					newPath        string
					coverImagePath *string
				}{file.ID, file.Filepath, newPath, file.CoverImageFilename})
			}
		}
	} else if isRootLevelBook {
		// For root-level files that need folder creation, organize each file into a new folder
		log.Info("organizing root-level files into folder", logger.Data{"file_count": len(files)})

		var newBookPath string
		for _, file := range files {
			// Set file type for proper volume formatting
			organizeOpts.FileType = file.FileType

			// Prefer file.Name for the per-file title (consistent with the
			// isDirectoryBased and else branches below). Without this, a
			// user-edited file name on a root-level file would be dropped
			// when organize moves the file into its folder.
			if file.Name != nil && *file.Name != "" {
				organizeOpts.Title = *file.Name
			} else {
				organizeOpts.Title = book.Title
			}

			// Populate narrator names from file's narrators for M4B files
			organizeOpts.NarratorNames = nil
			for _, n := range file.Narrators {
				if n.Person != nil {
					organizeOpts.NarratorNames = append(organizeOpts.NarratorNames, n.Person.Name)
				}
			}

			result, err := fileutils.OrganizeRootLevelFile(file.Filepath, organizeOpts)
			if err != nil {
				log.Error("failed to organize root-level file", logger.Data{
					"file_id": file.ID,
					"path":    file.Filepath,
					"error":   err.Error(),
				})
				continue
			}

			if result.Moved {
				log.Info("organized file into folder", logger.Data{
					"file_id":  file.ID,
					"old_path": result.OriginalPath,
					"new_path": result.NewPath,
				})

				pathUpdates = append(pathUpdates, struct {
					fileID         int
					oldPath        string
					newPath        string
					coverImagePath *string
				}{file.ID, result.OriginalPath, result.NewPath, file.CoverImageFilename})

				// Track the new folder path (should be same for all files)
				if newBookPath == "" {
					newBookPath = filepath.Dir(result.NewPath)
				}
			}
		}

		// Update book filepath to the new folder
		if newBookPath != "" && newBookPath != book.Filepath {
			oldBookPath := book.Filepath
			book.Filepath = newBookPath
			book.UpdatedAt = now

			_, err = svc.db.NewUpdate().
				Model(book).
				Column("filepath", "updated_at").
				WherePK().
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			// Clean up the synthetic pre-organize folder that scan_unified.go
			// created just to hold an early book sidecar. If an enricher
			// changed book.Title between that sidecar write and now, the
			// folder is orphaned (contains only the stale sidecar) because
			// OrganizeRootLevelFile computed the new folder from the current
			// title and moved the media file + associated covers there.
			// Then write a fresh sidecar at the new folder so the organized
			// book is never left sidecar-less (same pattern as the
			// isDirectoryBased branch above).
			svc.cleanUpStaleRootLevelBookFolder(ctx, oldBookPath)
			if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
				log.Warn("failed to write book sidecar after organize", logger.Data{
					"book_id": book.ID,
					"path":    book.Filepath,
					"error":   err.Error(),
				})
			}
		}
	} else {
		// For files already in a subfolder, just rename them in place
		log.Info("renaming files in place", logger.Data{"file_count": len(files)})

		for _, file := range files {
			// Set file type for proper volume formatting
			organizeOpts.FileType = file.FileType

			// Use file.Name for title if available, otherwise book.Title
			if file.Name != nil && *file.Name != "" {
				organizeOpts.Title = *file.Name
			} else {
				organizeOpts.Title = book.Title
			}

			// Populate narrator names from file's narrators for M4B files
			organizeOpts.NarratorNames = nil
			for _, n := range file.Narrators {
				if n.Person != nil {
					organizeOpts.NarratorNames = append(organizeOpts.NarratorNames, n.Person.Name)
				}
			}

			newPath, err := fileutils.RenameOrganizedFile(file.Filepath, organizeOpts)
			if err != nil {
				log.Error("failed to rename file", logger.Data{
					"file_id": file.ID,
					"path":    file.Filepath,
					"error":   err.Error(),
				})
				continue
			}

			if newPath != file.Filepath {
				log.Info("renamed file", logger.Data{
					"file_id":  file.ID,
					"old_path": file.Filepath,
					"new_path": newPath,
				})

				pathUpdates = append(pathUpdates, struct {
					fileID         int
					oldPath        string
					newPath        string
					coverImagePath *string
				}{file.ID, file.Filepath, newPath, file.CoverImageFilename})
			}
		}
	}

	// Update file paths in database
	for _, update := range pathUpdates {
		q := svc.db.NewUpdate().
			Model((*models.File)(nil)).
			Set("filepath = ?, updated_at = ?", update.newPath, now).
			Where("id = ?", update.fileID)

		// Also update cover image path if the file has a cover
		if update.coverImagePath != nil {
			newCoverPath := fileutils.ComputeNewCoverFilename(*update.coverImagePath, update.newPath)
			q = q.Set("cover_image_filename = ?", newCoverPath)
		}

		_, err = q.Exec(ctx)
		if err != nil {
			log.Error("failed to update file path in database", logger.Data{
				"file_id":  update.fileID,
				"old_path": update.oldPath,
				"new_path": update.newPath,
				"error":    err.Error(),
			})
		}
	}

	return nil
}

// cleanUpStaleRootLevelBookFolder removes the synthetic book folder that
// scan_unified.go may have created to hold an early book sidecar. It's only
// safe to call after root-level file organization has relocated the media
// files (and any cover sidecars) to the new folder — at that point the old
// folder should contain nothing but the stale book metadata.json.
//
// Missing directory or non-empty directory (e.g. user dropped extra files
// in there) are both logged and tolerated: we only clean up what we know we
// put there.
func (svc *Service) cleanUpStaleRootLevelBookFolder(ctx context.Context, oldBookPath string) {
	log := logger.FromContext(ctx)

	if oldBookPath == "" {
		return
	}

	info, err := os.Stat(oldBookPath)
	if err != nil || !info.IsDir() {
		return
	}

	staleSidecarPath := sidecar.BookSidecarPath(oldBookPath)
	if staleSidecarPath != "" {
		if err := os.Remove(staleSidecarPath); err != nil && !os.IsNotExist(err) {
			log.Warn("failed to remove stale book sidecar", logger.Data{
				"path":  staleSidecarPath,
				"error": err.Error(),
			})
		}
	}

	if err := os.Remove(oldBookPath); err != nil && !os.IsNotExist(err) {
		log.Warn("failed to remove stale book folder", logger.Data{
			"path":  oldBookPath,
			"error": err.Error(),
		})
	}
}

// CreateAuthor creates a book-author association.
func (svc *Service) CreateAuthor(ctx context.Context, author *models.Author) error {
	_, err := svc.db.
		NewInsert().
		Model(author).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

// CreateNarrator creates a file-narrator association.
func (svc *Service) CreateNarrator(ctx context.Context, narrator *models.Narrator) error {
	_, err := svc.db.
		NewInsert().
		Model(narrator).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteAuthors deletes all author associations for a book.
func (svc *Service) DeleteAuthors(ctx context.Context, bookID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.Author)(nil)).
		Where("book_id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// CreateBookSeries creates a book-series association.
func (svc *Service) CreateBookSeries(ctx context.Context, bookSeries *models.BookSeries) error {
	_, err := svc.db.
		NewInsert().
		Model(bookSeries).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteBookSeries deletes all series associations for a book.
func (svc *Service) DeleteBookSeries(ctx context.Context, bookID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.BookSeries)(nil)).
		Where("book_id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// GetBookSeriesForBook returns all series associations for a book.
func (svc *Service) GetBookSeriesForBook(ctx context.Context, bookID int) ([]*models.BookSeries, error) {
	var bookSeries []*models.BookSeries
	err := svc.db.
		NewSelect().
		Model(&bookSeries).
		Relation("Series").
		Where("bs.book_id = ?", bookID).
		Order("bs.sort_order ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return bookSeries, nil
}

// buildFTSPrefixQuery builds an FTS5 query for prefix/typeahead search.
// It sanitizes the input to prevent FTS5 injection and appends a wildcard.
func buildFTSPrefixQuery(input string) string {
	const maxQueryLength = 100

	// Trim and limit length
	input = strings.TrimSpace(input)
	if len(input) > maxQueryLength {
		input = input[:maxQueryLength]
	}
	if input == "" {
		return ""
	}

	// Escape double quotes (used for phrase matching in FTS5)
	input = strings.ReplaceAll(input, `"`, `""`)

	// Wrap in double quotes and add prefix wildcard: "query"*
	return `"` + input + `"*`
}

// FindOrCreateSeries finds an existing series or creates a new one (case-insensitive match).
// This is duplicated from series service to avoid import cycles.
func (svc *Service) FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("series name cannot be empty")
	}

	// Try to find existing series (case-insensitive)
	series := &models.Series{}
	err := svc.db.
		NewSelect().
		Model(series).
		Where("LOWER(s.name) = LOWER(?) AND s.library_id = ?", name, libraryID).
		Scan(ctx)
	if err == nil {
		// Series exists, check if we should update the source
		if models.GetDataSourcePriority(nameSource) < models.GetDataSourcePriority(series.NameSource) {
			series.NameSource = nameSource
			series.UpdatedAt = time.Now()
			_, err = svc.db.
				NewUpdate().
				Model(series).
				Column("name_source", "updated_at").
				WherePK().
				Exec(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
		}
		return series, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}

	// Create new series
	now := time.Now()
	series = &models.Series{
		LibraryID:      libraryID,
		Name:           name,
		NameSource:     nameSource,
		SortName:       sortname.ForTitle(name),
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = svc.db.
		NewInsert().
		Model(series).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return series, nil
}

// CleanupOrphanedSeries soft-deletes series with no books.
// This is duplicated from series service to avoid import cycles.
func (svc *Service) CleanupOrphanedSeries(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Series)(nil)).
		Where("id NOT IN (SELECT DISTINCT series_id FROM book_series)").
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// RetrieveSeriesByID retrieves a series by its ID.
func (svc *Service) RetrieveSeriesByID(ctx context.Context, id int) (*models.Series, error) {
	series := &models.Series{}
	err := svc.db.
		NewSelect().
		Model(series).
		Relation("Library").
		Where("s.id = ?", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Series")
		}
		return nil, errors.WithStack(err)
	}
	return series, nil
}

// CreateBookGenre creates a book-genre association.
func (svc *Service) CreateBookGenre(ctx context.Context, bookGenre *models.BookGenre) error {
	_, err := svc.db.
		NewInsert().
		Model(bookGenre).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteBookGenres deletes all genre associations for a book.
func (svc *Service) DeleteBookGenres(ctx context.Context, bookID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.BookGenre)(nil)).
		Where("book_id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// CreateBookTag creates a book-tag association.
func (svc *Service) CreateBookTag(ctx context.Context, bookTag *models.BookTag) error {
	_, err := svc.db.
		NewInsert().
		Model(bookTag).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteBookTags deletes all tag associations for a book.
func (svc *Service) DeleteBookTags(ctx context.Context, bookID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.BookTag)(nil)).
		Where("book_id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateAuthors creates multiple book-author associations in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateAuthors(ctx context.Context, authors []*models.Author) error {
	if len(authors) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&authors).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateNarrators creates multiple file-narrator associations in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateNarrators(ctx context.Context, narrators []*models.Narrator) error {
	if len(narrators) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&narrators).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateBookGenres creates multiple book-genre associations in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateBookGenres(ctx context.Context, bookGenres []*models.BookGenre) error {
	if len(bookGenres) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&bookGenres).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateBookTags creates multiple book-tag associations in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateBookTags(ctx context.Context, bookTags []*models.BookTag) error {
	if len(bookTags) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&bookTags).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateBookSeries creates multiple book-series associations in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateBookSeries(ctx context.Context, bookSeries []*models.BookSeries) error {
	if len(bookSeries) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&bookSeries).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateFileIdentifiers creates multiple file identifier records in a
// single query. Identifier values are canonicalized via
// identifiers.NormalizeValue before insert. Defensively dedupes by type
// (last-wins) so a misbehaving caller never trips the UNIQUE(file_id, type)
// constraint; each dropped duplicate is logged at warn level so upstream
// bugs are visible. Returns nil if the slice is empty after dedupe.
func (svc *Service) BulkCreateFileIdentifiers(ctx context.Context, fileIdentifiers []*models.FileIdentifier) error {
	if len(fileIdentifiers) == 0 {
		return nil
	}
	log := logger.FromContext(ctx)
	type key struct {
		FileID int
		Type   string
	}
	indexByKey := make(map[key]int, len(fileIdentifiers))
	deduped := make([]*models.FileIdentifier, 0, len(fileIdentifiers))
	for _, fi := range fileIdentifiers {
		clone := *fi
		clone.Type = strings.TrimSpace(fi.Type)
		clone.Value = identifiers.NormalizeValue(clone.Type, fi.Value)
		k := key{FileID: clone.FileID, Type: clone.Type}
		if existingIdx, ok := indexByKey[k]; ok {
			dropped := deduped[existingIdx]
			log.Warn("dropping duplicate file identifier of same type", logger.Data{
				"file_id":        clone.FileID,
				"type":           clone.Type,
				"dropped_value":  dropped.Value,
				"dropped_source": dropped.Source,
				"kept_value":     clone.Value,
				"kept_source":    clone.Source,
			})
			deduped[existingIdx] = &clone
			continue
		}
		indexByKey[k] = len(deduped)
		deduped = append(deduped, &clone)
	}
	if len(deduped) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&deduped).Exec(ctx)
	return errors.WithStack(err)
}

// DeleteNarratorsForFile deletes all narrators for a file.
func (svc *Service) DeleteNarratorsForFile(ctx context.Context, fileID int) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Narrator)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// DeleteIdentifiersForFile deletes all identifiers for a file.
func (svc *Service) DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.FileIdentifier)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// DeleteFile deletes a file and its associated records (narrators, identifiers, chapters cascade via FK).
// If the deleted file was the book's primary file (auto-nulled via ON DELETE SET NULL), promotes another.
func (svc *Service) DeleteFile(ctx context.Context, fileID int) error {
	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Get the file to find its book_id and check if it's the primary
		var file models.File
		err := tx.NewSelect().
			Model(&file).
			Where("id = ?", fileID).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Check if this file is the book's primary before deleting
		var book models.Book
		err = tx.NewSelect().Model(&book).
			Column("primary_file_id").
			Where("id = ?", file.BookID).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		wasPrimary := book.PrimaryFileID != nil && *book.PrimaryFileID == fileID

		// Delete the file — narrators, identifiers, and chapters cascade via FK.
		// books.primary_file_id is auto-nulled via ON DELETE SET NULL.
		_, err = tx.NewDelete().
			Model((*models.File)(nil)).
			Where("id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Promote a new primary file only if we just deleted the primary
		if wasPrimary {
			var newPrimary models.File
			err = tx.NewSelect().
				Model(&newPrimary).
				Where("book_id = ?", file.BookID).
				OrderExpr("CASE WHEN file_role = ? THEN 0 ELSE 1 END", models.FileRoleMain).
				Order("created_at ASC").
				Limit(1).
				Scan(ctx)
			if err == nil {
				_, err = tx.NewUpdate().
					Model((*models.Book)(nil)).
					Set("primary_file_id = ?", newPrimary.ID).
					Where("id = ?", file.BookID).
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}

		return nil
	})
}

// DeleteFilesByIDs batch-deletes files and their associated records (cascade via FK).
// Unlike DeleteFile, it does NOT handle primary file promotion — the caller manages that separately.
// Returns nil if fileIDs is empty.
func (svc *Service) DeleteFilesByIDs(ctx context.Context, fileIDs []int) error {
	if len(fileIDs) == 0 {
		return nil
	}
	// Narrators, identifiers, chapters cascade via FK.
	// books.primary_file_id is auto-nulled via ON DELETE SET NULL.
	_, err := svc.db.NewDelete().
		Model((*models.File)(nil)).
		Where("id IN (?)", bun.List(fileIDs)).
		Exec(ctx)
	return errors.WithStack(err)
}

// PromoteSupplementToMain promotes a supplement file to a main file.
// Used during scan cleanup when the last main file is deleted but a promotable supplement exists.
func (svc *Service) PromoteSupplementToMain(ctx context.Context, fileID int) error {
	_, err := svc.db.NewUpdate().
		Model((*models.File)(nil)).
		Set("file_role = ?", models.FileRoleMain).
		Where("id = ?", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}

// ListFilesForLibrary returns all main files for a library.
// Used for orphan cleanup during batch scans - only main files are tracked,
// supplements don't need orphan cleanup.
func (svc *Service) ListFilesForLibrary(ctx context.Context, libraryID int) ([]*models.File, error) {
	var files []*models.File
	err := svc.db.NewSelect().
		Model(&files).
		Where("library_id = ?", libraryID).
		Where("file_role = ?", models.FileRoleMain).
		Scan(ctx)
	return files, errors.WithStack(err)
}

// ListAllFilesForLibrary returns all files (main and supplement) for a library.
// Used to preload the scan cache so the path-based scan walk can detect
// supplement files that share scannable extensions (e.g. .pdf) with main files
// and skip them instead of trying to re-create them as main files.
func (svc *Service) ListAllFilesForLibrary(ctx context.Context, libraryID int) ([]*models.File, error) {
	var files []*models.File
	err := svc.db.NewSelect().
		Model(&files).
		Where("library_id = ?", libraryID).
		Scan(ctx)
	return files, errors.WithStack(err)
}

// DeleteBook deletes a book and all its associated records.
// All child records (files, authors, book_series, book_genres, book_tags) cascade via FK.
// File children (narrators, identifiers, chapters) cascade from files via FK.
func (svc *Service) DeleteBook(ctx context.Context, bookID int) error {
	_, err := svc.db.NewDelete().
		Model((*models.Book)(nil)).
		Where("id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteBooksByIDs deletes multiple books and all their associated records.
// All child records cascade via FK. Used during scan cleanup.
func (svc *Service) DeleteBooksByIDs(ctx context.Context, bookIDs []int) error {
	if len(bookIDs) == 0 {
		return nil
	}

	_, err := svc.db.NewDelete().
		Model((*models.Book)(nil)).
		Where("id IN (?)", bun.List(bookIDs)).
		Exec(ctx)
	return errors.WithStack(err)
}

// PromoteNextPrimaryFile selects the best remaining file for a book and sets it as
// primary_file_id. Main files are preferred over supplements; among equal roles,
// the oldest file (by created_at) wins. If no files remain, primary_file_id is set
// to NULL. This is called after a batch file deletion completes its transaction.
func (svc *Service) PromoteNextPrimaryFile(ctx context.Context, bookID int) error {
	// Find the best candidate: prefer main over supplement, then oldest first.
	var candidate models.File
	err := svc.db.NewSelect().
		Model(&candidate).
		Where("book_id = ?", bookID).
		OrderExpr("CASE WHEN file_role = ? THEN 0 ELSE 1 END", models.FileRoleMain).
		Order("created_at ASC").
		Limit(1).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.WithStack(err)
	}

	if err == nil {
		// Found a candidate — promote it.
		_, err = svc.db.NewUpdate().
			Model((*models.Book)(nil)).
			Set("primary_file_id = ?", candidate.ID).
			Where("id = ?", bookID).
			Exec(ctx)
		return errors.WithStack(err)
	}

	// No files remain — clear the primary pointer.
	_, err = svc.db.NewUpdate().
		Model((*models.Book)(nil)).
		Set("primary_file_id = NULL").
		Where("id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}

// DeleteBookAndFilesResult contains the results of deleting a book and its files.
type DeleteBookAndFilesResult struct {
	FilesDeleted int
}

// DeleteBookAndFiles deletes a book and all its files from both disk and database.
// If library.OrganizeFileStructure is true, the entire book directory is deleted.
// Otherwise, each file is deleted individually with its cover and sidecar.
func (svc *Service) DeleteBookAndFiles(ctx context.Context, bookID int, library *models.Library) (*DeleteBookAndFilesResult, error) {
	result := &DeleteBookAndFilesResult{}

	// Load book with files
	var book models.Book
	err := svc.db.NewSelect().
		Model(&book).
		Relation("Files").
		Where("b.id = ?", bookID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Book")
		}
		return nil, errors.WithStack(err)
	}

	result.FilesDeleted = len(book.Files)

	// Delete files from disk first (before DB transaction)
	if library.OrganizeFileStructure && book.Filepath != "" {
		// Organized structure: delete entire book directory
		if err := os.RemoveAll(book.Filepath); err != nil && !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "failed to delete book directory")
		}
	} else {
		// Root-level files: delete each file individually
		for _, file := range book.Files {
			if err := deleteFileFromDisk(file); err != nil {
				return nil, errors.Wrap(err, "failed to delete file from disk")
			}
		}
	}

	// Delete book from database (cascades to files and associations)
	if err := svc.DeleteBook(ctx, bookID); err != nil {
		return nil, err
	}

	return result, nil
}

// deleteFileFromDisk deletes a file and its associated cover and sidecar from disk.
func deleteFileFromDisk(file *models.File) error {
	// Delete the main file
	if err := os.Remove(file.Filepath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to delete main file")
	}

	// Delete cover image if exists (best effort). The cover lives alongside
	// the file for both root-level and directory-backed books, so
	// filepath.Dir(file.Filepath) is always the cover dir regardless of
	// whether the main file exists on disk.
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		coverPath := filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)
		_ = os.Remove(coverPath)
	}

	// Delete sidecar file if exists (best effort)
	sidecarPath := file.Filepath + ".metadata.json"
	_ = os.Remove(sidecarPath)

	return nil
}

// DeleteBooksAndFilesResult contains the results of bulk book deletion.
type DeleteBooksAndFilesResult struct {
	BooksDeleted int
	FilesDeleted int
}

// DistinctFileLanguages returns distinct non-null language values for files in a library.
func (svc *Service) DistinctFileLanguages(ctx context.Context, libraryID int) ([]string, error) {
	var languages []string
	err := svc.db.NewSelect().
		TableExpr("files AS f").
		ColumnExpr("DISTINCT f.language").
		Where("f.library_id = ?", libraryID).
		Where("f.language IS NOT NULL").
		Where("f.language != ''").
		OrderExpr("f.language ASC").
		Scan(ctx, &languages)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return languages, nil
}

// DeleteBooksAndFiles deletes multiple books and all their files from disk and database.
func (svc *Service) DeleteBooksAndFiles(ctx context.Context, bookIDs []int, library *models.Library) (*DeleteBooksAndFilesResult, error) {
	result := &DeleteBooksAndFilesResult{}

	for _, bookID := range bookIDs {
		bookResult, err := svc.DeleteBookAndFiles(ctx, bookID, library)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to delete book %d", bookID)
		}
		result.BooksDeleted++
		result.FilesDeleted += bookResult.FilesDeleted
	}

	return result, nil
}

// DeleteFileAndCleanupResult contains the results of deleting a file.
type DeleteFileAndCleanupResult struct {
	BookDeleted    bool
	BookID         int
	PromotedFileID *int // ID of supplement file that was promoted to main, if any
}

// DeleteFileAndCleanup deletes a file from disk and database.
// If this was the last main file in the book, it checks if any supplement files can be
// promoted to main (based on supportedTypes). If a promotable supplement exists, the oldest
// one is promoted. If no promotable supplements exist, the book and remaining files are deleted.
// ignoredPatterns are glob patterns for files to ignore during directory cleanup (e.g., ".DS_Store", ".*").
func (svc *Service) DeleteFileAndCleanup(ctx context.Context, fileID int, library *models.Library, supportedTypes map[string]struct{}, ignoredPatterns []string) (*DeleteFileAndCleanupResult, error) {
	result := &DeleteFileAndCleanupResult{}

	// Load file with book
	var file models.File
	err := svc.db.NewSelect().
		Model(&file).
		Relation("Book").
		Where("f.id = ?", fileID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("File")
		}
		return nil, errors.WithStack(err)
	}

	result.BookID = file.BookID

	// Delete file from disk
	if err := deleteFileFromDisk(&file); err != nil {
		return nil, err
	}

	// Delete file from database
	if err := svc.DeleteFile(ctx, fileID); err != nil {
		return nil, err
	}

	// Check if book has any remaining main files
	mainCount, err := svc.db.NewSelect().
		Model((*models.File)(nil)).
		Where("book_id = ?", file.BookID).
		Where("file_role = ?", models.FileRoleMain).
		Count(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if mainCount > 0 {
		// Other main files exist, nothing more to do
		return result, nil
	}

	// No main files remain - check if we can promote a supplement
	var supplements []models.File
	err = svc.db.NewSelect().
		Model(&supplements).
		Where("book_id = ?", file.BookID).
		Where("file_role = ?", models.FileRoleSupplement).
		Order("created_at ASC"). // Oldest first
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(supplements) > 0 {
		// Find the first supplement with a supported file type
		var toPromote *models.File
		for i := range supplements {
			if _, supported := supportedTypes[supplements[i].FileType]; supported {
				toPromote = &supplements[i]
				break
			}
		}

		if toPromote != nil {
			// Promote this supplement to main
			_, err = svc.db.NewUpdate().
				Model(toPromote).
				Set("file_role = ?", models.FileRoleMain).
				WherePK().
				Exec(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			result.PromotedFileID = &toPromote.ID
			return result, nil
		}

		// No promotable supplements - delete all remaining files
		for i := range supplements {
			if err := deleteFileFromDisk(&supplements[i]); err != nil {
				return nil, err
			}
			if err := svc.DeleteFile(ctx, supplements[i].ID); err != nil {
				return nil, err
			}
		}
	}

	// Delete the book
	if err := svc.DeleteBook(ctx, file.BookID); err != nil {
		return nil, err
	}
	result.BookDeleted = true

	// Clean up book directory if organized structure
	if library.OrganizeFileStructure && file.Book != nil && file.Book.Filepath != "" {
		// Combine caller's ignored patterns with shisho special file patterns
		// so covers (*.cover.*) and sidecars (*.metadata.json) are cleaned up too
		allIgnoredPatterns := make([]string, 0, len(ignoredPatterns)+len(fileutils.ShishoSpecialFilePatterns))
		allIgnoredPatterns = append(allIgnoredPatterns, ignoredPatterns...)
		allIgnoredPatterns = append(allIgnoredPatterns, fileutils.ShishoSpecialFilePatterns...)

		// Clean up book directory (removes covers, sidecars, and OS junk files)
		_, _ = fileutils.CleanupEmptyDirectory(file.Book.Filepath, allIgnoredPatterns...)
	}

	return result, nil
}
