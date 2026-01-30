package imprints

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type RetrieveImprintOptions struct {
	ID        *int
	Name      *string
	LibraryID *int
}

type ListImprintsOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	Search     *string

	includeTotal bool
}

type UpdateImprintOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateImprint(ctx context.Context, imprint *models.Imprint) error {
	now := time.Now()
	if imprint.CreatedAt.IsZero() {
		imprint.CreatedAt = now
	}
	imprint.UpdatedAt = imprint.CreatedAt

	_, err := svc.db.
		NewInsert().
		Model(imprint).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RetrieveImprint(ctx context.Context, opts RetrieveImprintOptions) (*models.Imprint, error) {
	imprint := &models.Imprint{}

	q := svc.db.
		NewSelect().
		Model(imprint)

	if opts.ID != nil {
		q = q.Where("imp.id = ?", *opts.ID)
	}
	if opts.Name != nil && opts.LibraryID != nil {
		// Case-insensitive match
		q = q.Where("LOWER(imp.name) = LOWER(?) AND imp.library_id = ?", *opts.Name, *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Imprint")
		}
		return nil, errors.WithStack(err)
	}

	return imprint, nil
}

// FindOrCreateImprint finds an existing imprint or creates a new one (case-insensitive match).
func (svc *Service) FindOrCreateImprint(ctx context.Context, name string, libraryID int) (*models.Imprint, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("imprint name cannot be empty")
	}

	imprint, err := svc.RetrieveImprint(ctx, RetrieveImprintOptions{
		Name:      &name,
		LibraryID: &libraryID,
	})
	if err == nil {
		return imprint, nil
	}
	if !errors.Is(err, errcodes.NotFound("Imprint")) {
		return nil, err
	}

	// Create new imprint
	imprint = &models.Imprint{
		LibraryID: libraryID,
		Name:      name,
	}
	err = svc.CreateImprint(ctx, imprint)
	if err != nil {
		// Handle race condition: if another goroutine created the same imprint
		// between our retrieve and create, retry the retrieve
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return svc.RetrieveImprint(ctx, RetrieveImprintOptions{
				Name:      &name,
				LibraryID: &libraryID,
			})
		}
		return nil, err
	}
	return imprint, nil
}

func (svc *Service) ListImprints(ctx context.Context, opts ListImprintsOptions) ([]*models.Imprint, error) {
	i, _, err := svc.listImprintsWithTotal(ctx, opts)
	return i, errors.WithStack(err)
}

func (svc *Service) ListImprintsWithTotal(ctx context.Context, opts ListImprintsOptions) ([]*models.Imprint, int, error) {
	opts.includeTotal = true
	return svc.listImprintsWithTotal(ctx, opts)
}

func (svc *Service) listImprintsWithTotal(ctx context.Context, opts ListImprintsOptions) ([]*models.Imprint, int, error) {
	var imprints []*models.Imprint
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&imprints).
		Order("imp.name ASC")

	if opts.LibraryID != nil {
		q = q.Where("imp.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("imp.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	// Search using LIKE (no FTS for imprints)
	if opts.Search != nil && *opts.Search != "" {
		search := "%" + strings.ToLower(*opts.Search) + "%"
		q = q.Where("LOWER(imp.name) LIKE ?", search)
	}
	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return imprints, total, nil
}

func (svc *Service) UpdateImprint(ctx context.Context, imprint *models.Imprint, opts UpdateImprintOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	now := time.Now()
	imprint.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(imprint).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Imprint")
		}
		return errors.WithStack(err)
	}
	return nil
}

// DeleteImprint deletes an imprint and clears imprint_id from all associated files.
func (svc *Service) DeleteImprint(ctx context.Context, imprintID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Clear imprint_id from files
		_, err := tx.NewUpdate().
			Model((*models.File)(nil)).
			Set("imprint_id = NULL").
			Where("imprint_id = ?", imprintID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the imprint
		_, err = tx.NewDelete().
			Model((*models.Imprint)(nil)).
			Where("id = ?", imprintID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetFileCount returns the count of files with this imprint.
func (svc *Service) GetFileCount(ctx context.Context, imprintID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.File)(nil)).
		Where("imprint_id = ?", imprintID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// GetFiles returns all files with this imprint.
func (svc *Service) GetFiles(ctx context.Context, imprintID int) ([]*models.File, error) {
	var files []*models.File

	err := svc.db.NewSelect().
		Model(&files).
		Where("f.imprint_id = ?", imprintID).
		Relation("Book").
		Order("f.filepath ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return files, nil
}

// MergeImprints merges sourceImprint into targetImprint (moves all file associations, deletes source).
func (svc *Service) MergeImprints(ctx context.Context, targetID, sourceID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Update all files from source to target
		_, err := tx.NewUpdate().
			Model((*models.File)(nil)).
			Set("imprint_id = ?", targetID).
			Where("imprint_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the source imprint
		_, err = tx.NewDelete().
			Model((*models.Imprint)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedImprints deletes imprints with no file associations.
func (svc *Service) CleanupOrphanedImprints(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Imprint)(nil)).
		Where("id NOT IN (SELECT DISTINCT imprint_id FROM files WHERE imprint_id IS NOT NULL)").
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
