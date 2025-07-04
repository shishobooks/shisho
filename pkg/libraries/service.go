package libraries

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type RetrieveLibraryOptions struct {
	ID *int
}

type ListLibrariesOptions struct {
	Limit          *int
	Offset         *int
	IncludeDeleted bool

	includeTotal bool
}

type UpdateLibraryOptions struct {
	Columns            []string
	UpdateLibraryPaths bool
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateLibrary(ctx context.Context, library *models.Library) error {
	now := time.Now()
	if library.CreatedAt.IsZero() {
		library.CreatedAt = now
	}
	library.UpdatedAt = library.CreatedAt

	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.
			NewInsert().
			Model(library).
			Returning("*").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		for _, path := range library.LibraryPaths {
			path.LibraryID = library.ID
			path.CreatedAt = library.CreatedAt
		}

		if len(library.LibraryPaths) > 0 {
			_, err := tx.
				NewInsert().
				Model(&library.LibraryPaths).
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

func (svc *Service) RetrieveLibrary(ctx context.Context, opts RetrieveLibraryOptions) (*models.Library, error) {
	library := &models.Library{}

	q := svc.db.
		NewSelect().
		Model(library).
		Column("l.*").
		Relation("LibraryPaths", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("filepath ASC")
		}).
		Group("l.id")

	if opts.ID != nil {
		q = q.Where("l.id = ?", *opts.ID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Library")
		}
		return nil, errors.WithStack(err)
	}

	return library, nil
}

func (svc *Service) ListLibraries(ctx context.Context, opts ListLibrariesOptions) ([]*models.Library, error) {
	l, _, err := svc.listLibrariesWithTotal(ctx, opts)
	return l, errors.WithStack(err)
}

func (svc *Service) ListLibrariesWithTotal(ctx context.Context, opts ListLibrariesOptions) ([]*models.Library, int, error) {
	opts.includeTotal = true
	return svc.listLibrariesWithTotal(ctx, opts)
}

func (svc *Service) listLibrariesWithTotal(ctx context.Context, opts ListLibrariesOptions) ([]*models.Library, int, error) {
	libraries := []*models.Library{}
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&libraries).
		Column("l.*").
		Relation("LibraryPaths", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("filepath ASC")
		}).
		Group("l.id").
		Order("l.name ASC")

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}
	if !opts.IncludeDeleted {
		q = q.Where("deleted_at IS NULL")
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return libraries, total, nil
}

func (svc *Service) UpdateLibrary(ctx context.Context, library *models.Library, opts UpdateLibraryOptions) error {
	if len(opts.Columns) == 0 && !opts.UpdateLibraryPaths {
		return nil
	}

	// Update updated_at.
	now := time.Now()
	library.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.
			NewUpdate().
			Model(library).
			Column(columns...).
			WherePK().
			Exec(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errcodes.NotFound("Library")
			}
			return errors.WithStack(err)
		}

		if opts.UpdateLibraryPaths {
			// Delete all existing library paths.
			_, err := tx.
				NewDelete().
				Model((*models.LibraryPath)(nil)).
				Where("library_id = ?", library.ID).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			for _, path := range library.LibraryPaths {
				path.LibraryID = library.ID
				path.CreatedAt = now
			}

			// Insert new library paths.
			if len(library.LibraryPaths) > 0 {
				_, err := tx.
					NewInsert().
					Model(&library.LibraryPaths).
					Returning("*").
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
