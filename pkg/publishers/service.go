package publishers

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
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
	ExcludeIDs []int // Exclude specific publisher IDs from results

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

	// Check aliases
	if resourceID, aliasErr := aliases.FindResourceIDByAlias(ctx, svc.db, aliases.PublisherConfig, name, libraryID); aliasErr == nil {
		return svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &resourceID})
	}

	// Create new publisher
	publisher = &models.Publisher{
		LibraryID: libraryID,
		Name:      name,
	}
	err = svc.CreatePublisher(ctx, publisher)
	if err != nil {
		// Handle race condition: if another goroutine created the same publisher
		// between our retrieve and create, retry the retrieve
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return svc.RetrievePublisher(ctx, RetrievePublisherOptions{
				Name:      &name,
				LibraryID: &libraryID,
			})
		}
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
		q = q.Where("pub.library_id IN (?)", bun.List(opts.LibraryIDs))
	}
	if opts.Search != nil && *opts.Search != "" {
		ftsQuery := search.BuildPrefixQuery(*opts.Search)
		if ftsQuery != "" {
			q = q.Where("pub.id IN (SELECT publisher_id FROM publishers_fts WHERE publishers_fts MATCH ?)", ftsQuery)
		}
	}
	if len(opts.ExcludeIDs) > 0 {
		q = q.Where("pub.id NOT IN (?)", bun.List(opts.ExcludeIDs))
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

// GetFilesPaginated returns a paginated list of files with this publisher.
func (svc *Service) GetFilesPaginated(ctx context.Context, publisherID, limit, offset int) ([]*models.File, int, error) {
	var files []*models.File

	total, err := svc.db.NewSelect().
		Model(&files).
		Where("f.publisher_id = ?", publisherID).
		Relation("Book").
		Order("f.filepath ASC").
		Limit(limit).
		Offset(offset).
		ScanAndCount(ctx)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return files, total, nil
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

		// Re-parent children of source to target (exclude target itself to avoid self-reference)
		_, err = tx.NewUpdate().
			Model((*models.Publisher)(nil)).
			Set("parent_id = ?", targetID).
			Where("parent_id = ? AND id != ?", sourceID, targetID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// If target was a child of source, clear target's parent_id to avoid dangling reference
		_, err = tx.NewUpdate().
			Model((*models.Publisher)(nil)).
			Set("parent_id = NULL").
			Where("id = ? AND parent_id = ?", targetID, sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// If source was a child of target, clear that to avoid issues during deletion
		_, err = tx.NewUpdate().
			Model((*models.Publisher)(nil)).
			Set("parent_id = NULL").
			Where("id = ? AND parent_id = ?", sourceID, targetID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if err := aliases.TransferAliasesOnMerge(ctx, tx, aliases.PublisherConfig, sourceID, targetID); err != nil {
			return err
		}

		// Delete the source publisher
		_, err = tx.NewDelete().
			Model((*models.Publisher)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedPublishers deletes publishers with no file associations and
// no children, returning the IDs of deleted publishers. Callers must pass the
// returned IDs to searchService.DeleteFromPublisherIndex to keep publishers_fts
// in sync. Publishers that serve as parents in the hierarchy are preserved even
// if they have no direct file associations.
func (svc *Service) CleanupOrphanedPublishers(ctx context.Context) ([]int, error) {
	deletedIDs := []int{}
	err := svc.db.NewDelete().
		Model((*models.Publisher)(nil)).
		Where("id NOT IN (SELECT DISTINCT publisher_id FROM files WHERE publisher_id IS NOT NULL)").
		Where("id NOT IN (SELECT DISTINCT parent_id FROM publishers WHERE parent_id IS NOT NULL)").
		Returning("id").
		Scan(ctx, &deletedIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return deletedIDs, nil
}

// SetParent sets or clears the parent of a publisher. If parentID is non-nil,
// it validates that setting the parent would not create a cycle.
func (svc *Service) SetParent(ctx context.Context, publisherID int, parentID *int) error {
	if parentID != nil {
		if *parentID <= 0 {
			return errors.New("invalid parent: parent_id must be a positive integer")
		}

		// Verify parent is in the same library as the child
		child, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &publisherID})
		if err != nil {
			return err
		}
		parent, err := svc.RetrievePublisher(ctx, RetrievePublisherOptions{ID: parentID})
		if err != nil {
			return err
		}
		if child.LibraryID != parent.LibraryID {
			return errors.New("parent publisher must be in the same library")
		}

		if err := svc.ValidateNoCycle(ctx, publisherID, *parentID); err != nil {
			return err
		}
	}

	publisher := &models.Publisher{ID: publisherID, ParentID: parentID}
	_, err := svc.db.NewUpdate().
		Model(publisher).
		Column("parent_id").
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// ValidateNoCycle walks the ancestor chain of proposedParentID and rejects the
// operation if publisherID is found (which would create a cycle). It also
// rejects self-references (publisherID == proposedParentID).
func (svc *Service) ValidateNoCycle(ctx context.Context, publisherID, proposedParentID int) error {
	if publisherID == proposedParentID {
		return errors.New("cannot set parent: would create a cycle")
	}

	// Walk up from proposedParentID to check for cycles
	currentID := proposedParentID
	visited := map[int]bool{publisherID: true}

	for {
		if visited[currentID] {
			return errors.New("cannot set parent: would create a cycle")
		}
		visited[currentID] = true

		var parentID *int
		err := svc.db.NewSelect().
			Model((*models.Publisher)(nil)).
			Column("parent_id").
			Where("id = ?", currentID).
			Scan(ctx, &parentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("parent publisher not found")
			}
			return errors.WithStack(err)
		}
		if parentID == nil {
			break
		}
		currentID = *parentID
	}

	return nil
}

// GetAncestors returns the ancestor chain for a publisher, ordered from
// immediate parent to root.
func (svc *Service) GetAncestors(ctx context.Context, publisherID int) ([]*models.Publisher, error) {
	// First get the publisher's parent_id
	var parentID *int
	err := svc.db.NewSelect().
		Model((*models.Publisher)(nil)).
		Column("parent_id").
		Where("id = ?", publisherID).
		Scan(ctx, &parentID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var ancestors []*models.Publisher
	visited := map[int]bool{publisherID: true}

	currentID := parentID
	for currentID != nil {
		if visited[*currentID] {
			break // Safety: prevent infinite loop on corrupt data
		}
		visited[*currentID] = true

		ancestor := &models.Publisher{}
		err := svc.db.NewSelect().
			Model(ancestor).
			Where("pub.id = ?", *currentID).
			Scan(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ancestors = append(ancestors, ancestor)
		currentID = ancestor.ParentID
	}

	return ancestors, nil
}

// GetChildren returns the direct children of a publisher with their file counts.
func (svc *Service) GetChildren(ctx context.Context, publisherID int) ([]*models.Publisher, error) {
	var children []*models.Publisher

	err := svc.db.NewSelect().
		Model(&children).
		Where("pub.parent_id = ?", publisherID).
		ColumnExpr("pub.*").
		ColumnExpr("(SELECT COUNT(*) FROM files WHERE files.publisher_id = pub.id) AS file_count").
		Order("pub.name ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return children, nil
}

// GetDescendantFileCount returns the count of files attached to any descendant
// of the given publisher (not including the publisher's own direct files).
func (svc *Service) GetDescendantFileCount(ctx context.Context, publisherID int) (int, error) {
	descendantIDs, err := svc.GetDescendantIDs(ctx, publisherID)
	if err != nil {
		return 0, err
	}
	if len(descendantIDs) == 0 {
		return 0, nil
	}

	count, err := svc.db.NewSelect().
		Model((*models.File)(nil)).
		Where("publisher_id IN (?)", bun.List(descendantIDs)).
		Count(ctx)
	return count, errors.WithStack(err)
}

// GetDescendantPublisherCount returns the count of all publishers in the
// subtree rooted at publisherID (not including the publisher itself).
func (svc *Service) GetDescendantPublisherCount(ctx context.Context, publisherID int) (int, error) {
	descendantIDs, err := svc.GetDescendantIDs(ctx, publisherID)
	if err != nil {
		return 0, err
	}
	return len(descendantIDs), nil
}

// GetDescendantIDs returns all descendant IDs of a publisher (children,
// grandchildren, etc.) using a breadth-first traversal.
func (svc *Service) GetDescendantIDs(ctx context.Context, publisherID int) ([]int, error) {
	var descendants []int
	queue := []int{publisherID}
	visited := map[int]bool{publisherID: true}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		var childIDs []int
		err := svc.db.NewSelect().
			Model((*models.Publisher)(nil)).
			Column("id").
			Where("parent_id = ?", currentID).
			Scan(ctx, &childIDs)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, childID := range childIDs {
			if visited[childID] {
				continue // Safety: prevent infinite loop on corrupt circular data
			}
			visited[childID] = true
			descendants = append(descendants, childID)
			queue = append(queue, childID)
		}
	}

	return descendants, nil
}
