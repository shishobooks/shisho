package publishers

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

type RetrievePublisherOptions struct {
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

func (svc *Service) CreatePublisher(ctx context.Context, publisher *models.Publisher) error {
	now := time.Now()
	if publisher.CreatedAt.IsZero() {
		publisher.CreatedAt = now
	}
	publisher.UpdatedAt = publisher.CreatedAt

	_, err := svc.db.
		NewInsert().
		Model(publisher).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RetrievePublisher(ctx context.Context, opts RetrievePublisherOptions) (*models.Publisher, error) {
	publisher := &models.Publisher{}

	q := svc.db.
		NewSelect().
		Model(publisher)

	if opts.ID != nil {
		q = q.Where("pub.id = ?", *opts.ID)
	}
	if opts.Name != nil && opts.LibraryID != nil {
		// Case-insensitive match
		q = q.Where("LOWER(pub.name) = LOWER(?) AND pub.library_id = ?", *opts.Name, *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Publisher")
		}
		return nil, errors.WithStack(err)
	}

	return publisher, nil
}

// FindOrCreatePublisher finds an existing publisher or creates a new one (case-insensitive match).
func (svc *Service) FindOrCreatePublisher(ctx context.Context, name string, libraryID int) (*models.Publisher, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("publisher name cannot be empty")
	}

	publisher, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{
		Name:      &name,
		LibraryID: &libraryID,
	})
	if err == nil {
		return publisher, nil
	}
	if !errors.Is(err, errcodes.NotFound("Publisher")) {
		return nil, err
	}

	// Create new publisher
	publisher = &models.Publisher{
		LibraryID: libraryID,
		Name:      name,
	}
	err = svc.CreatePublisher(ctx, publisher)
	if err != nil {
		return nil, err
	}
	return publisher, nil
}
