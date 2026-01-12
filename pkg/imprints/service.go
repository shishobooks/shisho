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
		return nil, err
	}
	return imprint, nil
}
