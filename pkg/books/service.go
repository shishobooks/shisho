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
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
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
	GenreIDs   []int    // Filter by genre IDs
	TagIDs     []int    // Filter by tag IDs
	IDs        []int    // Filter by specific book IDs
	Search     *string  // Search query for title/author

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

	// Apply ordering
	if opts.orderByRecent {
		q = q.Order("b.updated_at DESC")
	} else if opts.SeriesID != nil {
		// When filtering by series, order by series_number then sort_title
		q = q.Order("bs_filter.series_number ASC", "b.sort_title ASC")
	} else {
		q = q.Order("b.sort_title ASC")
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

	// Filter by specific book IDs
	if len(opts.IDs) > 0 {
		q = q.Where("b.id IN (?)", bun.In(opts.IDs))
	}

	// Filter by file types
	if len(opts.FileTypes) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM files WHERE file_type IN (?))", bun.In(opts.FileTypes))
	}

	// Filter by genre IDs
	if len(opts.GenreIDs) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM book_genres WHERE genre_id IN (?))", bun.In(opts.GenreIDs))
	}

	// Filter by tag IDs
	if len(opts.TagIDs) > 0 {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM book_tags WHERE tag_id IN (?))", bun.In(opts.TagIDs))
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

// CreateFileIdentifier creates a new file identifier record.
func (svc *Service) CreateFileIdentifier(ctx context.Context, identifier *models.FileIdentifier) error {
	now := time.Now()
	identifier.CreatedAt = now
	identifier.UpdatedAt = now
	_, err := svc.db.NewInsert().Model(identifier).Exec(ctx)
	return errors.WithStack(err)
}

// DeleteFileIdentifiers deletes all identifiers for a file.
func (svc *Service) DeleteFileIdentifiers(ctx context.Context, fileID int) error {
	_, err := svc.db.NewDelete().Model((*models.FileIdentifier)(nil)).Where("file_id = ?", fileID).Exec(ctx)
	return errors.WithStack(err)
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

// BulkCreateFileIdentifiers creates multiple file identifier records in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateFileIdentifiers(ctx context.Context, identifiers []*models.FileIdentifier) error {
	if len(identifiers) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&identifiers).Exec(ctx)
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

// DeleteFile deletes a file and its associated records (narrators, identifiers).
// If the deleted file was the book's primary file, promotes another file to primary.
func (svc *Service) DeleteFile(ctx context.Context, fileID int) error {
	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Get the file to find its book_id
		var file models.File
		err := tx.NewSelect().
			Model(&file).
			Where("id = ?", fileID).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete narrators for this file
		_, err = tx.NewDelete().
			Model((*models.Narrator)(nil)).
			Where("file_id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete identifiers for this file
		_, err = tx.NewDelete().
			Model((*models.FileIdentifier)(nil)).
			Where("file_id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the file record
		_, err = tx.NewDelete().
			Model((*models.File)(nil)).
			Where("id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Check if this was the primary file and promote another if needed
		var book models.Book
		err = tx.NewSelect().
			Model(&book).
			Where("id = ?", file.BookID).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if book.PrimaryFileID != nil && *book.PrimaryFileID == fileID {
			// Find the next primary: prefer main files, then oldest
			var newPrimary models.File
			err = tx.NewSelect().
				Model(&newPrimary).
				Where("book_id = ?", file.BookID).
				OrderExpr("CASE WHEN file_role = ? THEN 0 ELSE 1 END", models.FileRoleMain).
				Order("created_at ASC").
				Limit(1).
				Scan(ctx)
			if err == nil {
				// Found a file to promote
				_, err = tx.NewUpdate().
					Model(&book).
					Set("primary_file_id = ?", newPrimary.ID).
					Where("id = ?", book.ID).
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			// If no files remain, the book deletion cascade will handle cleanup
		}

		return nil
	})
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

// DeleteBook deletes a book and all its associated records (files, authors, series, genres, tags).
func (svc *Service) DeleteBook(ctx context.Context, bookID int) error {
	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Get all file IDs for this book
		var fileIDs []int
		err := tx.NewSelect().
			Model((*models.File)(nil)).
			Column("id").
			Where("book_id = ?", bookID).
			Scan(ctx, &fileIDs)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete narrators for all files
		if len(fileIDs) > 0 {
			_, err = tx.NewDelete().
				Model((*models.Narrator)(nil)).
				Where("file_id IN (?)", bun.In(fileIDs)).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			// Delete identifiers for all files
			_, err = tx.NewDelete().
				Model((*models.FileIdentifier)(nil)).
				Where("file_id IN (?)", bun.In(fileIDs)).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Delete files
		_, err = tx.NewDelete().
			Model((*models.File)(nil)).
			Where("book_id = ?", bookID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete authors
		_, err = tx.NewDelete().
			Model((*models.Author)(nil)).
			Where("book_id = ?", bookID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete book series associations
		_, err = tx.NewDelete().
			Model((*models.BookSeries)(nil)).
			Where("book_id = ?", bookID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete book genres
		_, err = tx.NewDelete().
			Model((*models.BookGenre)(nil)).
			Where("book_id = ?", bookID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete book tags
		_, err = tx.NewDelete().
			Model((*models.BookTag)(nil)).
			Where("book_id = ?", bookID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the book record
		_, err = tx.NewDelete().
			Model((*models.Book)(nil)).
			Where("id = ?", bookID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	})
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

	// Delete cover image if exists (best effort)
	// Cover filename may be stored as relative or absolute path
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		coverPath := *file.CoverImageFilename
		if !filepath.IsAbs(coverPath) {
			coverPath = filepath.Join(filepath.Dir(file.Filepath), coverPath)
		}
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
		// Delete book-level sidecar
		bookSidecarPath := sidecar.BookSidecarPath(file.Book.Filepath)
		_ = os.Remove(bookSidecarPath)

		// Clean up empty book directory (ignoredPatterns handles .DS_Store, etc.)
		_, _ = fileutils.CleanupEmptyDirectory(file.Book.Filepath, ignoredPatterns...)
	}

	return result, nil
}
