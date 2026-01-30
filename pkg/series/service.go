package series

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/uptrace/bun"
)

type RetrieveSeriesOptions struct {
	ID        *int
	Name      *string
	LibraryID *int
}

type ListSeriesOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int   // Filter by multiple library IDs (for access control)
	Search     *string // FTS5 search query

	includeTotal bool
}

type UpdateSeriesOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateSeries(ctx context.Context, series *models.Series) error {
	now := time.Now()
	if series.CreatedAt.IsZero() {
		series.CreatedAt = now
	}
	series.UpdatedAt = series.CreatedAt

	// Generate sort name if not provided
	if series.SortName == "" {
		series.SortName = sortname.ForTitle(series.Name)
		series.SortNameSource = models.DataSourceFilepath // Auto-generated
	}
	// Ensure source is set if not already
	if series.SortNameSource == "" {
		series.SortNameSource = models.DataSourceFilepath
	}

	_, err := svc.db.
		NewInsert().
		Model(series).
		Returning("*").
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) RetrieveSeries(ctx context.Context, opts RetrieveSeriesOptions) (*models.Series, error) {
	series := &models.Series{}

	q := svc.db.
		NewSelect().
		Model(series).
		Relation("Library")

	if opts.ID != nil {
		q = q.Where("s.id = ?", *opts.ID)
	}
	if opts.Name != nil && opts.LibraryID != nil {
		// Case-insensitive match
		q = q.Where("LOWER(s.name) = LOWER(?) AND s.library_id = ?", *opts.Name, *opts.LibraryID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Series")
		}
		return nil, errors.WithStack(err)
	}

	return series, nil
}

// RetrieveSeriesByID retrieves a series by its ID.
func (svc *Service) RetrieveSeriesByID(ctx context.Context, id int) (*models.Series, error) {
	return svc.RetrieveSeries(ctx, RetrieveSeriesOptions{ID: &id})
}

// FindOrCreateSeries finds an existing series or creates a new one (case-insensitive match).
func (svc *Service) FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error) {
	// Normalize the name by trimming whitespace
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("series name cannot be empty")
	}

	series, err := svc.RetrieveSeries(ctx, RetrieveSeriesOptions{
		Name:      &name,
		LibraryID: &libraryID,
	})
	if err == nil {
		// Series exists, check if we should update the source
		if models.GetDataSourcePriority(nameSource) < models.GetDataSourcePriority(series.NameSource) {
			series.NameSource = nameSource
			err = svc.UpdateSeries(ctx, series, UpdateSeriesOptions{
				Columns: []string{"name_source"},
			})
			if err != nil {
				return nil, err
			}
		}
		return series, nil
	}
	if !errors.Is(err, errcodes.NotFound("Series")) {
		return nil, err
	}

	// Create new series
	series = &models.Series{
		LibraryID:      libraryID,
		Name:           name,
		NameSource:     nameSource,
		SortName:       sortname.ForTitle(name),
		SortNameSource: models.DataSourceFilepath,
	}
	err = svc.CreateSeries(ctx, series)
	if err != nil {
		// Handle race condition: if another goroutine created the same series
		// between our retrieve and create, retry the retrieve
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return svc.RetrieveSeries(ctx, RetrieveSeriesOptions{
				Name:      &name,
				LibraryID: &libraryID,
			})
		}
		return nil, err
	}
	return series, nil
}

func (svc *Service) ListSeries(ctx context.Context, opts ListSeriesOptions) ([]*models.Series, error) {
	s, _, err := svc.listSeriesWithTotal(ctx, opts)
	return s, errors.WithStack(err)
}

func (svc *Service) ListSeriesWithTotal(ctx context.Context, opts ListSeriesOptions) ([]*models.Series, int, error) {
	opts.includeTotal = true
	return svc.listSeriesWithTotal(ctx, opts)
}

func (svc *Service) listSeriesWithTotal(ctx context.Context, opts ListSeriesOptions) ([]*models.Series, int, error) {
	var series []*models.Series
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&series).
		Relation("Library").
		ColumnExpr("s.*").
		ColumnExpr("(SELECT COUNT(*) FROM book_series WHERE book_series.series_id = s.id) AS book_count").
		Order("s.sort_name ASC")

	if opts.LibraryID != nil {
		q = q.Where("s.library_id = ?", *opts.LibraryID)
	}
	if len(opts.LibraryIDs) > 0 {
		q = q.Where("s.library_id IN (?)", bun.In(opts.LibraryIDs))
	}
	// Search using FTS5
	if opts.Search != nil && *opts.Search != "" {
		ftsQuery := buildFTSPrefixQuery(*opts.Search)
		if ftsQuery != "" {
			q = q.Where("s.id IN (SELECT series_id FROM series_fts WHERE series_fts MATCH ?)", ftsQuery)
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

	return series, total, nil
}

func (svc *Service) UpdateSeries(ctx context.Context, series *models.Series, opts UpdateSeriesOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	now := time.Now()
	series.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(series).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Series")
		}
		return errors.WithStack(err)
	}
	return nil
}

// DeleteSeries soft-deletes a series.
func (svc *Service) DeleteSeries(ctx context.Context, seriesID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.Series)(nil)).
		Where("id = ?", seriesID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// RestoreSeries restores a soft-deleted series.
func (svc *Service) RestoreSeries(ctx context.Context, seriesID int) error {
	_, err := svc.db.
		NewUpdate().
		Model((*models.Series)(nil)).
		Set("deleted_at = NULL").
		Where("id = ?", seriesID).
		WhereAllWithDeleted().
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// MergeSeries merges sourceSeries into targetSeries (moves all books, soft-deletes source).
func (svc *Service) MergeSeries(ctx context.Context, targetID, sourceID int) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Update all book_series entries from source series to target series
		_, err := tx.NewUpdate().
			Model((*models.BookSeries)(nil)).
			Set("series_id = ?", targetID).
			Where("series_id = ?", sourceID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Soft-delete the source series
		_, err = tx.NewDelete().
			Model((*models.Series)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// CleanupOrphanedSeries soft-deletes series with no books.
func (svc *Service) CleanupOrphanedSeries(ctx context.Context) (int, error) {
	result, err := svc.db.NewDelete().
		Model((*models.Series)(nil)).
		Where("id NOT IN (SELECT DISTINCT series_id FROM book_series)").
		Exec(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// GetSeriesBookCount returns the number of books in a series.
func (svc *Service) GetSeriesBookCount(ctx context.Context, seriesID int) (int, error) {
	count, err := svc.db.NewSelect().
		Model((*models.BookSeries)(nil)).
		Where("series_id = ?", seriesID).
		Count(ctx)
	return count, errors.WithStack(err)
}

// buildFTSPrefixQuery builds an FTS5 query for prefix/typeahead search.
// It sanitizes the input to prevent FTS5 injection and appends a wildcard.
func buildFTSPrefixQuery(input string) string {
	const maxQueryLength = 100

	// Trim and limit length
	input = strings.TrimSpace(input)
	if len(input) > maxQueryLength {
		input = input[:maxQueryLength]
	}
	if input == "" {
		return ""
	}

	// Escape double quotes (used for phrase matching in FTS5)
	input = strings.ReplaceAll(input, `"`, `""`)

	// Wrap in double quotes and add prefix wildcard: "query"*
	return `"` + input + `"*`
}
