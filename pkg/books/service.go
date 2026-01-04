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
	"github.com/shishobooks/shisho/pkg/models"
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
	FileTypes  []string // Filter by file types (e.g., ["epub", "cbz"])
	Search     *string  // Search query for title/author

	includeTotal  bool
	orderByRecent bool // Order by updated_at DESC instead of created_at ASC
}

type UpdateBookOptions struct {
	Columns       []string
	UpdateAuthors bool
	AuthorNames   []string // Author names for updating (requires UpdateAuthors to be true)
	OrganizeFiles bool     // Whether to rename files when metadata changes
}

type RetrieveFileOptions struct {
	ID        *int
	Filepath  *string
	LibraryID *int
}

type ListFilesOptions struct {
	Limit  *int
	Offset *int
	BookID *int

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
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.file_type ASC")
		}).
		Relation("Files.Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person")

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
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.filepath ASC")
		}).
		Relation("Files.Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person").
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
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.file_type ASC")
		}).
		Relation("Files.Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Files.Narrators.Person")

	// Apply ordering
	if opts.orderByRecent {
		q = q.Order("b.updated_at DESC")
	} else {
		q = q.Order("b.created_at ASC")
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
		q = q.Where("b.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	if opts.SeriesID != nil {
		q = q.Where("b.id IN (SELECT book_id FROM book_series WHERE series_id = ?)", *opts.SeriesID)
	}

	// Filter by file types
	if len(opts.FileTypes) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.In(opts.FileTypes))
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

	// Note: FileNarrators are created separately via CreateFileNarrator after person creation

	return nil
}

func (svc *Service) RetrieveFile(ctx context.Context, opts RetrieveFileOptions) (*models.File, error) {
	file := &models.File{}

	q := svc.db.
		NewSelect().
		Model(file).
		Relation("Book")

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

// RetrieveFileWithNarrators retrieves a file with its narrators loaded.
func (svc *Service) RetrieveFileWithNarrators(ctx context.Context, fileID int) (*models.File, error) {
	file := &models.File{}

	err := svc.db.
		NewSelect().
		Model(file).
		Relation("Book").
		Relation("Narrators", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("n.sort_order ASC")
		}).
		Relation("Narrators.Person").
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

// organizeBookFiles renames files and folders based on updated book metadata.
func (svc *Service) organizeBookFiles(ctx context.Context, book *models.Book) error {
	log := logger.FromContext(ctx)
	now := time.Now()

	// Get the library to check if organize_file_structure is enabled
	var library models.Library
	err := svc.db.NewSelect().
		Model(&library).
		Where("id = ?", book.LibraryID).
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
		fileID  int
		oldPath string
		newPath string
	}

	// Check if this is a directory-based book or root-level files
	isDirectoryBased := filepath.Dir(files[0].Filepath) == book.Filepath

	if isDirectoryBased {
		// For directory-based books, rename the folder and update all file paths
		newFolderPath, err := fileutils.RenameOrganizedFolder(book.Filepath, organizeOpts)
		if err != nil {
			return errors.WithStack(err)
		}

		if newFolderPath != book.Filepath {
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

			// Update all file paths
			for _, file := range files {
				oldPath := file.Filepath
				newPath := strings.Replace(file.Filepath, book.Filepath, newFolderPath, 1)
				pathUpdates = append(pathUpdates, struct {
					fileID  int
					oldPath string
					newPath string
				}{file.ID, oldPath, newPath})
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
		}
	} else {
		// For root-level files, rename each file individually
		log.Info("organizing root-level files", logger.Data{"file_count": len(files)})

		for _, file := range files {
			// Set file type for proper volume formatting
			organizeOpts.FileType = file.FileType

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
					fileID  int
					oldPath string
					newPath string
				}{file.ID, file.Filepath, newPath})
			}
		}
	}

	// Update file paths in database
	for _, update := range pathUpdates {
		_, err = svc.db.NewUpdate().
			Model((*models.File)(nil)).
			Set("filepath = ?, updated_at = ?", update.newPath, now).
			Where("id = ?", update.fileID).
			Exec(ctx)
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

// DeleteNarrators deletes all narrator associations for a file.
func (svc *Service) DeleteNarrators(ctx context.Context, fileID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.Narrator)(nil)).
		Where("file_id = ?", fileID).
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
		if models.DataSourcePriority[nameSource] < models.DataSourcePriority[series.NameSource] {
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
		LibraryID:  libraryID,
		Name:       name,
		NameSource: nameSource,
		CreatedAt:  now,
		UpdatedAt:  now,
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
