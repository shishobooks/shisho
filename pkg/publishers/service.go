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

type ListPublishersOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	Search     *string

	includeTotal bool
}

type UpdatePublisherOptions struct {
	Columns []string
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

func (svc *Service) ListPublishers(ctx context.Context, opts ListPublishersOptions) ([]*models.Publisher, error) {
	p, _, err := svc.listPublishersWithTotal(ctx, opts)
	return p, errors.WithStack(err)
}

func (svc *Service) ListPublishersWithTotal(ctx context.Context, opts ListPublishersOptions) ([]*models.Publisher, int, error) {
	opts.includeTotal = true
	return svc.listPublishersWithTotal(ctx, opts)
}

func (svc *Service) listPublishersWithTotal(ctx context.Context, opts ListPublishersOptions) ([]*models.Publisher, int, error) {
	var publishers []*models.Publisher
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&publishers).
		Order("pub.name ASC")

	if opts.LibraryID != nil {
		q = q.Where("pub.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("pub.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	// Search using LIKE (no FTS for publishers)
	if opts.Search != nil && *opts.Search != "" {
		search := "%" + strings.ToLower(*opts.Search) + "%"
		q = q.Where("LOWER(pub.name) LIKE ?", search)
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

	return publishers, total, nil
}

func (svc *Service) UpdatePublisher(ctx context.Context, publisher *models.Publisher, opts UpdatePublisherOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	now := time.Now()
	publisher.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(publisher).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Publisher")
		}
		return errors.WithStack(err)
	}
	return nil
}

// DeletePublisher deletes a publisher and clears publisher_id from all associated files.
func (svc *Service) DeletePublisher(ctx context.Context, publisherID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Clear publisher_id from files
		_, err := tx.NewUpdate().
			Model((*models.File)(nil)).
			Set("publisher_id = NULL").
			Where("publisher_id = ?", publisherID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the publisher
		_, err = tx.NewDelete().
			Model((*models.Publisher)(nil)).
			Where("id = ?", publisherID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetFileCount returns the count of files with this publisher.
func (svc *Service) GetFileCount(ctx context.Context, publisherID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.File)(nil)).
		Where("publisher_id = ?", publisherID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// GetFiles returns all files with this publisher.
func (svc *Service) GetFiles(ctx context.Context, publisherID int) ([]*models.File, error) {
	var files []*models.File

	err := svc.db.NewSelect().
		Model(&files).
		Where("f.publisher_id = ?", publisherID).
		Relation("Book").
		Order("f.filepath ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return files, nil
}

// MergePublishers merges sourcePublisher into targetPublisher (moves all file associations, deletes source).
func (svc *Service) MergePublishers(ctx context.Context, targetID, sourceID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Update all files from source to target
		_, err := tx.NewUpdate().
			Model((*models.File)(nil)).
			Set("publisher_id = ?", targetID).
			Where("publisher_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the source publisher
		_, err = tx.NewDelete().
			Model((*models.Publisher)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedPublishers deletes publishers with no file associations.
func (svc *Service) CleanupOrphanedPublishers(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Publisher)(nil)).
		Where("id NOT IN (SELECT DISTINCT publisher_id FROM files WHERE publisher_id IS NOT NULL)").
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
