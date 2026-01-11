package tags

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

type RetrieveTagOptions struct {
	ID        *int
	Name      *string
	LibraryID *int
}

type ListTagsOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	Search     *string

	includeTotal bool
}

type UpdateTagOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateTag(ctx context.Context, tag *models.Tag) error {
	now := time.Now()
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = now
	}
	tag.UpdatedAt = tag.CreatedAt

	_, err := svc.db.
		NewInsert().
		Model(tag).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RetrieveTag(ctx context.Context, opts RetrieveTagOptions) (*models.Tag, error) {
	tag := &models.Tag{}

	q := svc.db.
		NewSelect().
		Model(tag)

	if opts.ID != nil {
		q = q.Where("t.id = ?", *opts.ID)
	}
	if opts.Name != nil && opts.LibraryID != nil {
		// Case-insensitive match
		q = q.Where("LOWER(t.name) = LOWER(?) AND t.library_id = ?", *opts.Name, *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Tag")
		}
		return nil, errors.WithStack(err)
	}

	return tag, nil
}

// FindOrCreateTag finds an existing tag or creates a new one (case-insensitive match).
func (svc *Service) FindOrCreateTag(ctx context.Context, name string, libraryID int) (*models.Tag, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("tag name cannot be empty")
	}

	tag, err := svc.RetrieveTag(ctx, RetrieveTagOptions{
		Name:      &name,
		LibraryID: &libraryID,
	})
	if err == nil {
		return tag, nil
	}
	if !errors.Is(err, errcodes.NotFound("Tag")) {
		return nil, err
	}

	// Create new tag
	tag = &models.Tag{
		LibraryID: libraryID,
		Name:      name,
	}
	err = svc.CreateTag(ctx, tag)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

func (svc *Service) ListTags(ctx context.Context, opts ListTagsOptions) ([]*models.Tag, error) {
	t, _, err := svc.listTagsWithTotal(ctx, opts)
	return t, errors.WithStack(err)
}

func (svc *Service) ListTagsWithTotal(ctx context.Context, opts ListTagsOptions) ([]*models.Tag, int, error) {
	opts.includeTotal = true
	return svc.listTagsWithTotal(ctx, opts)
}

func (svc *Service) listTagsWithTotal(ctx context.Context, opts ListTagsOptions) ([]*models.Tag, int, error) {
	var tags []*models.Tag
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&tags).
		Order("t.name ASC")

	if opts.LibraryID != nil {
		q = q.Where("t.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("t.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	// Search using FTS5
	if opts.Search != nil && *opts.Search != "" {
		ftsQuery := buildFTSPrefixQuery(*opts.Search)
		if ftsQuery != "" {
			q = q.Where("t.id IN (SELECT tag_id FROM tags_fts WHERE tags_fts MATCH ?)", ftsQuery)
		}
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

	return tags, total, nil
}

func (svc *Service) UpdateTag(ctx context.Context, tag *models.Tag, opts UpdateTagOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	now := time.Now()
	tag.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(tag).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Tag")
		}
		return errors.WithStack(err)
	}
	return nil
}

// DeleteTag deletes a tag and all book associations.
func (svc *Service) DeleteTag(ctx context.Context, tagID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Delete book_tags associations (cascade should handle this, but be explicit)
		_, err := tx.NewDelete().
			Model((*models.BookTag)(nil)).
			Where("tag_id = ?", tagID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the tag
		_, err = tx.NewDelete().
			Model((*models.Tag)(nil)).
			Where("id = ?", tagID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetBookCount returns the count of books with this tag.
func (svc *Service) GetBookCount(ctx context.Context, tagID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.BookTag)(nil)).
		Where("tag_id = ?", tagID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// GetBooks returns all books with this tag.
func (svc *Service) GetBooks(ctx context.Context, tagID int) ([]*models.Book, error) {
	var books []*models.Book

	err := svc.db.NewSelect().
		Model(&books).
		Join("INNER JOIN book_tags bt ON bt.book_id = b.id").
		Where("bt.tag_id = ?", tagID).
		Order("b.title ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return books, nil
}

// MergeTags merges sourceTag into targetTag (moves all associations, deletes source).
func (svc *Service) MergeTags(ctx context.Context, targetID, sourceID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Get all book_ids from source that aren't already in target
		// to avoid unique constraint violations
		_, err := tx.NewRaw(`
			UPDATE book_tags
			SET tag_id = ?
			WHERE tag_id = ?
			AND book_id NOT IN (SELECT book_id FROM book_tags WHERE tag_id = ?)
		`, targetID, sourceID, targetID).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete remaining source associations (duplicates)
		_, err = tx.NewDelete().
			Model((*models.BookTag)(nil)).
			Where("tag_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the source tag
		_, err = tx.NewDelete().
			Model((*models.Tag)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedTags deletes tags with no book associations.
func (svc *Service) CleanupOrphanedTags(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Tag)(nil)).
		Where("id NOT IN (SELECT DISTINCT tag_id FROM book_tags)").
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// buildFTSPrefixQuery builds an FTS5 query for prefix/typeahead search.
func buildFTSPrefixQuery(input string) string {
	const maxQueryLength = 100

	input = strings.TrimSpace(input)
	if len(input) > maxQueryLength {
		input = input[:maxQueryLength]
	}
	if input == "" {
		return ""
	}

	input = strings.ReplaceAll(input, `"`, `""`)
	return `"` + input + `"*`
}
