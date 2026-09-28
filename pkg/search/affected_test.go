package search

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// clearFTS empties every FTS table, so a test can see exactly which rows a
// reindex writes.
func clearFTS(t *testing.T, db *bun.DB) {
	t.Helper()
	for _, table := range allFTSTables {
		_, err := db.ExecContext(context.Background(), "DELETE FROM "+table.name)
		require.NoError(t, err)
	}
}

// ftsRowIDs returns the rowids present in an FTS table.
func ftsRowIDs(t *testing.T, db *bun.DB, table string) []int {
	t.Helper()
	ids := []int{}
	require.NoError(t, db.NewRaw("SELECT rowid FROM "+table+" ORDER BY rowid").Scan(context.Background(), &ids))
	return ids
}

func TestReindexAffected_ExpandsPeopleToBooksAndSeries(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)
	clearFTS(t, db)

	// The narrator narrates the first two Books; those Books sit in both
	// Series. The third Book and the other People are untouched.
	svc.ReindexAffected(ctx, &Affected{PersonIDs: []int{f.persons[1].ID}})

	assert.Equal(t, []int{f.persons[1].ID}, ftsRowIDs(t, db, "persons_fts"))
	assert.Equal(t, []int{f.books[0].ID, f.books[1].ID}, ftsRowIDs(t, db, "books_fts"))
	assert.Equal(t, []int{f.series[0].ID, f.series[1].ID}, ftsRowIDs(t, db, "series_fts"))
	assert.Empty(t, ftsRowIDs(t, db, "genres_fts"))
}

func TestReindexAffected_ExpandsSeriesToBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)
	clearFTS(t, db)

	svc.ReindexAffected(ctx, &Affected{SeriesIDs: []int{f.series[1].ID}})

	assert.Equal(t, []int{f.books[1].ID}, ftsRowIDs(t, db, "books_fts"), "the Series' only Book")
	assert.Equal(t, []int{f.series[0].ID, f.series[1].ID}, ftsRowIDs(t, db, "series_fts"), "the Series, and the other Series holding its Book")
	assert.Empty(t, ftsRowIDs(t, db, "persons_fts"))
}

func TestReindexAffected_OwnRowsOnlyForGenresTagsPublishers(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)
	clearFTS(t, db)

	svc.ReindexAffected(ctx, &Affected{GenreIDs: []int{f.genres[0].ID}, TagIDs: []int{f.tags[0].ID}, PublisherIDs: []int{f.publishers[0].ID}})

	assert.Equal(t, []int{f.genres[0].ID}, ftsRowIDs(t, db, "genres_fts"))
	assert.Equal(t, []int{f.tags[0].ID}, ftsRowIDs(t, db, "tags_fts"))
	assert.Equal(t, []int{f.publishers[0].ID}, ftsRowIDs(t, db, "publishers_fts"))
	assert.Empty(t, ftsRowIDs(t, db, "books_fts"))
}

// A deleted Book's book_series rows CASCADE away, so only the ids collected
// before the delete still reach the Series that held it.
func TestCollectAffected_KeepsSeriesOfDeletedBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)
	require.NoError(t, svc.RebuildAllIndexes(ctx))
	doomed := f.books[0]

	affected := svc.CollectAffected(ctx, Affected{BookIDs: []int{doomed.ID}})
	_, err := db.NewDelete().Model((*models.Book)(nil)).Where("id = ?", doomed.ID).Exec(ctx)
	require.NoError(t, err)
	svc.ReindexAffected(ctx, affected)

	assert.NotContains(t, ftsRowIDs(t, db, "books_fts"), doomed.ID, "the deleted Book's row is dropped")
	series, _, err := svc.SearchSeries(ctx, f.library.ID, "Harbor", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, series, "the Series no longer lists the deleted Book's title")
	series, _, err = svc.SearchSeries(ctx, f.library.ID, "Quiet", 10, 0)
	require.NoError(t, err)
	assert.Len(t, series, 2, "both Series still list the surviving Book")
}

// ReindexAffected picks up ids added to the collected set after the call, as
// a handler does for a Book its mutation creates, and drops rows of entities
// that no longer exist.
func TestReindexAffected_ReadsIDsAddedAfterCollect(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)
	clearFTS(t, db)

	affected := svc.CollectAffected(ctx, Affected{})
	affected.BookIDs = append(affected.BookIDs, f.books[2].ID)
	affected.PersonIDs = append(affected.PersonIDs, 9999)
	svc.ReindexAffected(ctx, affected)

	assert.Equal(t, []int{f.books[2].ID}, ftsRowIDs(t, db, "books_fts"))
	assert.Empty(t, ftsRowIDs(t, db, "persons_fts"), "an id with no entity writes no row")
}

// ReindexAffected runs after the change has committed, so a cancelled request
// context must not stop it.
func TestReindexAffected_IgnoresCancellation(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	f := createFTSFixture(t, db)
	svc := NewService(db)
	clearFTS(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.ReindexAffected(ctx, &Affected{BookIDs: []int{f.books[2].ID}})

	assert.Equal(t, []int{f.books[2].ID}, ftsRowIDs(t, db, "books_fts"))
}

// A nil service is a no-op, so callers built without search (some tests)
// need no guard.
func TestReindexAffected_NilServiceIsNoOp(t *testing.T) {
	t.Parallel()
	var svc *Service
	affected := svc.CollectAffected(context.Background(), Affected{BookIDs: []int{1}})
	assert.Equal(t, []int{1}, affected.BookIDs)
	svc.ReindexAffected(context.Background(), affected)
}
