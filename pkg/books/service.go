package books

import (
	"context"
	"database/sql"
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
	Limit     *int
	Offset    *int
	LibraryID *int
	SeriesID  *int

	includeTotal bool
}

type UpdateBookOptions struct {
	Columns       []string
	UpdateAuthors bool
	OrganizeFiles bool // Whether to rename files when metadata changes
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

		// Insert authors.
		for i, author := range book.Authors {
			author.BookID = book.ID
			if author.SortOrder == 0 {
				author.SortOrder = i + 1
			}
			author.CreatedAt = book.CreatedAt
			author.UpdatedAt = book.UpdatedAt
		}
		if len(book.Authors) > 0 {
			_, err := tx.
				NewInsert().
				Model(&book.Authors).
				Returning("*").
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

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

		// Insert narrators.
		narrators := make([]*models.Narrator, 0)
		for _, file := range book.Files {
			for i, narrator := range file.Narrators {
				narrator.FileID = file.ID
				if narrator.SortOrder == 0 {
					narrator.SortOrder = i + 1
				}
				narrator.CreatedAt = file.CreatedAt
				narrator.UpdatedAt = file.UpdatedAt
				narrators = append(narrators, narrator)
			}
		}
		if len(narrators) > 0 {
			_, err := tx.
				NewInsert().
				Model(&narrators).
				Returning("*").
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

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
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.file_type ASC")
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
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.filepath ASC")
		}).
		Join("INNER JOIN files f ON f.book_id = b.id").
		Where("f.filepath = ? AND b.library_id = ?", filepath, libraryID)

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
		Relation("Files", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("f.file_type ASC")
		}).
		Order("b.created_at ASC")

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}
	if opts.LibraryID != nil {
		q = q.Where("b.library_id = ?", *opts.LibraryID)
	}
	if opts.SeriesID != nil {
		q = q.Where("b.series_id = ?", *opts.SeriesID)
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
	if len(opts.Columns) == 0 && !opts.UpdateAuthors {
		return nil
	}

	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if opts.UpdateAuthors {
			// Delete all previous authors and save these new ones.
			_, err := tx.
				NewDelete().
				Model((*models.Author)(nil)).
				Where("book_id = ?", book.ID).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			for i, author := range book.Authors {
				author.BookID = book.ID
				if author.SortOrder == 0 {
					author.SortOrder = i + 1
				}
				author.CreatedAt = book.CreatedAt
				author.UpdatedAt = book.UpdatedAt
			}

			if len(book.Authors) > 0 {
				_, err = tx.
					NewInsert().
					Model(&book.Authors).
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
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

	// Handle file organization if requested
	if opts.OrganizeFiles {
		err = svc.organizeBookFiles(ctx, book)
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

	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Insert file.
		_, err := tx.
			NewInsert().
			Model(file).
			Returning("*").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Insert narrators.
		for i, narrator := range file.Narrators {
			narrator.FileID = file.ID
			if narrator.SortOrder == 0 {
				narrator.SortOrder = i + 1
			}
			narrator.CreatedAt = file.CreatedAt
			narrator.UpdatedAt = file.UpdatedAt
		}
		if len(file.Narrators) > 0 {
			_, err := tx.
				NewInsert().
				Model(&file.Narrators).
				Returning("*").
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		return nil
	})
	if err != nil {
		return errors.WithStack(err)
	}

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
		Where("series_id = ?", seriesID).
		Order("series_number ASC", "title ASC").
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

	// Create organized name options from current book metadata
	organizeOpts := fileutils.OrganizedNameOptions{
		Authors:      book.Authors,
		Title:        book.Title,
		SeriesNumber: book.SeriesNumber,
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
		log.Info("organizing directory-based book", logger.Data{"book_path": book.Filepath})

		newFolderPath, err := fileutils.RenameOrganizedFolder(book.Filepath, organizeOpts)
		if err != nil {
			return errors.WithStack(err)
		}

		if newFolderPath != book.Filepath {
			log.Info("renamed book folder", logger.Data{
				"old_path": book.Filepath,
				"new_path": newFolderPath,
			})

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
