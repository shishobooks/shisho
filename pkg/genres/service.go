package genres

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

type RetrieveGenreOptions struct {
	ID        *int
	Name      *string
	LibraryID *int
}

type ListGenresOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	Search     *string

	includeTotal bool
}

type UpdateGenreOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateGenre(ctx context.Context, genre *models.Genre) error {
	now := time.Now()
	if genre.CreatedAt.IsZero() {
		genre.CreatedAt = now
	}
	genre.UpdatedAt = genre.CreatedAt

	_, err := svc.db.
		NewInsert().
		Model(genre).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RetrieveGenre(ctx context.Context, opts RetrieveGenreOptions) (*models.Genre, error) {
	genre := &models.Genre{}

	q := svc.db.
		NewSelect().
		Model(genre)

	if opts.ID != nil {
		q = q.Where("g.id = ?", *opts.ID)
	}
	if opts.Name != nil && opts.LibraryID != nil {
		// Case-insensitive match
		q = q.Where("LOWER(g.name) = LOWER(?) AND g.library_id = ?", *opts.Name, *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Genre")
		}
		return nil, errors.WithStack(err)
	}

	return genre, nil
}

// FindOrCreateGenre finds an existing genre or creates a new one (case-insensitive match).
func (svc *Service) FindOrCreateGenre(ctx context.Context, name string, libraryID int) (*models.Genre, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("genre name cannot be empty")
	}

	genre, err := svc.RetrieveGenre(ctx, RetrieveGenreOptions{
		Name:      &name,
		LibraryID: &libraryID,
	})
	if err == nil {
		return genre, nil
	}
	if !errors.Is(err, errcodes.NotFound("Genre")) {
		return nil, err
	}

	// Create new genre
	genre = &models.Genre{
		LibraryID: libraryID,
		Name:      name,
	}
	err = svc.CreateGenre(ctx, genre)
	if err != nil {
		return nil, err
	}
	return genre, nil
}

func (svc *Service) ListGenres(ctx context.Context, opts ListGenresOptions) ([]*models.Genre, error) {
	g, _, err := svc.listGenresWithTotal(ctx, opts)
	return g, errors.WithStack(err)
}

func (svc *Service) ListGenresWithTotal(ctx context.Context, opts ListGenresOptions) ([]*models.Genre, int, error) {
	opts.includeTotal = true
	return svc.listGenresWithTotal(ctx, opts)
}

func (svc *Service) listGenresWithTotal(ctx context.Context, opts ListGenresOptions) ([]*models.Genre, int, error) {
	var genres []*models.Genre
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&genres).
		Order("g.name ASC")

	if opts.LibraryID != nil {
		q = q.Where("g.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("g.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	// Search using FTS5
	if opts.Search != nil && *opts.Search != "" {
		ftsQuery := buildFTSPrefixQuery(*opts.Search)
		if ftsQuery != "" {
			q = q.Where("g.id IN (SELECT genre_id FROM genres_fts WHERE genres_fts MATCH ?)", ftsQuery)
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

	return genres, total, nil
}

func (svc *Service) UpdateGenre(ctx context.Context, genre *models.Genre, opts UpdateGenreOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	now := time.Now()
	genre.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(genre).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Genre")
		}
		return errors.WithStack(err)
	}
	return nil
}

// DeleteGenre deletes a genre and all book associations.
func (svc *Service) DeleteGenre(ctx context.Context, genreID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Delete book_genres associations (cascade should handle this, but be explicit)
		_, err := tx.NewDelete().
			Model((*models.BookGenre)(nil)).
			Where("genre_id = ?", genreID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the genre
		_, err = tx.NewDelete().
			Model((*models.Genre)(nil)).
			Where("id = ?", genreID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetBookCount returns the count of books with this genre.
func (svc *Service) GetBookCount(ctx context.Context, genreID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.BookGenre)(nil)).
		Where("genre_id = ?", genreID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// GetBooks returns all books with this genre.
func (svc *Service) GetBooks(ctx context.Context, genreID int) ([]*models.Book, error) {
	var books []*models.Book

	err := svc.db.NewSelect().
		Model(&books).
		Join("INNER JOIN book_genres bg ON bg.book_id = b.id").
		Where("bg.genre_id = ?", genreID).
		Order("b.title ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return books, nil
}

// MergeGenres merges sourceGenre into targetGenre (moves all associations, deletes source).
func (svc *Service) MergeGenres(ctx context.Context, targetID, sourceID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Get all book_ids from source that aren't already in target
		// to avoid unique constraint violations
		_, err := tx.NewRaw(`
			UPDATE book_genres
			SET genre_id = ?
			WHERE genre_id = ?
			AND book_id NOT IN (SELECT book_id FROM book_genres WHERE genre_id = ?)
		`, targetID, sourceID, targetID).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete remaining source associations (duplicates)
		_, err = tx.NewDelete().
			Model((*models.BookGenre)(nil)).
			Where("genre_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the source genre
		_, err = tx.NewDelete().
			Model((*models.Genre)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedGenres deletes genres with no book associations.
func (svc *Service) CleanupOrphanedGenres(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Genre)(nil)).
		Where("id NOT IN (SELECT DISTINCT genre_id FROM book_genres)").
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
