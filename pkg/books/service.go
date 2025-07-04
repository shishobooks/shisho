package books

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
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

	includeTotal bool
}

type UpdateBookOptions struct {
	Columns       []string
	UpdateAuthors bool
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
