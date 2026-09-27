package search

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// ftsEntities holds two of each searchable entity. Each test indexes the
// second one before the first, so an auto-assigned FTS rowid would not match
// the entity id and a missing explicit rowid shows up as a mismatch.
type ftsEntities struct {
	library    *models.Library
	books      [2]*models.Book
	series     [2]*models.Series
	persons    [2]*models.Person
	genres     [2]*models.Genre
	tags       [2]*models.Tag
	publishers [2]*models.Publisher
}

func createFTSEntities(t *testing.T, db *bun.DB) *ftsEntities {
	t.Helper()
	ctx := context.Background()
	e := &ftsEntities{library: &models.Library{Name: "Test Library", CoverAspectRatio: "book"}}
	_, err := db.NewInsert().Model(e.library).Exec(ctx)
	require.NoError(t, err)

	// A deleted first entity of each type leaves the ids starting at 2, so a
	// rebuild into an empty table (whose auto rowids restart at 1) still
	// produces mismatched rowids when rowid is not set explicitly.
	for _, model := range []any{
		&models.Book{LibraryID: e.library.ID, Filepath: "/test/gap", Title: "Gap", TitleSource: "file", SortTitle: "Gap", SortTitleSource: "file", AuthorSource: "file"},
		&models.Series{LibraryID: e.library.ID, Name: "Gap", NameSource: "file", SortName: "Gap", SortNameSource: "file"},
		&models.Person{LibraryID: e.library.ID, Name: "Gap", SortName: "Gap", SortNameSource: "file"},
		&models.Genre{LibraryID: e.library.ID, Name: "Gap"},
		&models.Tag{LibraryID: e.library.ID, Name: "Gap"},
		&models.Publisher{LibraryID: e.library.ID, Name: "Gap"},
	} {
		_, err := db.NewInsert().Model(model).Exec(ctx)
		require.NoError(t, err)
		_, err = db.NewDelete().Model(model).WherePK().Exec(ctx)
		require.NoError(t, err)
	}

	for i, name := range []string{"Alpha", "Beta"} {
		e.books[i] = &models.Book{
			LibraryID: e.library.ID, Filepath: "/test/" + name, Title: name + " Book", TitleSource: "file",
			SortTitle: name + " Book", SortTitleSource: "file", AuthorSource: "file",
		}
		e.series[i] = &models.Series{
			LibraryID: e.library.ID, Name: name + " Series", NameSource: "file",
			SortName: name + " Series", SortNameSource: "file",
		}
		e.persons[i] = &models.Person{LibraryID: e.library.ID, Name: name + " Person", SortName: name + " Person", SortNameSource: "file"}
		e.genres[i] = &models.Genre{LibraryID: e.library.ID, Name: name + " Genre"}
		e.tags[i] = &models.Tag{LibraryID: e.library.ID, Name: name + " Tag"}
		e.publishers[i] = &models.Publisher{LibraryID: e.library.ID, Name: name + " Publisher"}
		for _, model := range []any{e.books[i], e.series[i], e.persons[i], e.genres[i], e.tags[i], e.publishers[i]} {
			_, err := db.NewInsert().Model(model).Exec(ctx)
			require.NoError(t, err)
		}
	}
	return e
}

// ftsTable names an FTS table's entity id column and the ids of the two
// entities created by createFTSEntities.
type ftsTable struct {
	idColumn string
	ids      [2]int
}

// ftsTables maps each FTS table name to its ftsTable.
func (e *ftsEntities) ftsTables() map[string]ftsTable {
	return map[string]ftsTable{
		"books_fts":      {"book_id", [2]int{e.books[0].ID, e.books[1].ID}},
		"series_fts":     {"series_id", [2]int{e.series[0].ID, e.series[1].ID}},
		"persons_fts":    {"person_id", [2]int{e.persons[0].ID, e.persons[1].ID}},
		"genres_fts":     {"genre_id", [2]int{e.genres[0].ID, e.genres[1].ID}},
		"tags_fts":       {"tag_id", [2]int{e.tags[0].ID, e.tags[1].ID}},
		"publishers_fts": {"publisher_id", [2]int{e.publishers[0].ID, e.publishers[1].ID}},
	}
}

// assertFTSRowidsMatchIDs checks that every FTS row is keyed by rowid equal to
// the entity id it indexes, and that each entity has exactly one row.
func assertFTSRowidsMatchIDs(t *testing.T, db *bun.DB, e *ftsEntities) {
	t.Helper()
	ctx := context.Background()
	for table, info := range e.ftsTables() {
		var rows []struct {
			RowID    int `bun:"rowid"`
			EntityID int `bun:"entity_id"`
		}
		err := db.NewRaw("SELECT rowid, "+info.idColumn+" AS entity_id FROM "+table+" ORDER BY "+info.idColumn).Scan(ctx, &rows)
		require.NoError(t, err, table)
		require.Len(t, rows, 2, "%s should hold one row per entity", table)
		for i, row := range rows {
			assert.Equal(t, info.ids[i], row.EntityID, table)
			assert.Equal(t, row.EntityID, row.RowID, "%s row for %s %d should have rowid equal to the entity id", table, info.idColumn, row.EntityID)
		}
	}
}

func TestIndexMethods_KeyFTSRowsByEntityID(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	e := createFTSEntities(t, db)
	svc := NewService(db)

	for _, i := range []int{1, 0} {
		require.NoError(t, svc.IndexBook(ctx, e.books[i]))
		require.NoError(t, svc.IndexSeries(ctx, e.series[i]))
		require.NoError(t, svc.IndexPerson(ctx, e.persons[i]))
		require.NoError(t, svc.IndexGenre(ctx, e.genres[i]))
		require.NoError(t, svc.IndexTag(ctx, e.tags[i]))
		require.NoError(t, svc.IndexPublisher(ctx, e.publishers[i]))
	}
	assertFTSRowidsMatchIDs(t, db, e)

	// Re-indexing replaces the row in place rather than adding a second one.
	require.NoError(t, svc.IndexBook(ctx, e.books[0]))
	require.NoError(t, svc.IndexPublisher(ctx, e.publishers[1]))
	assertFTSRowidsMatchIDs(t, db, e)
}

func TestReindexBookByID_KeysFTSRowByBookID(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	e := createFTSEntities(t, db)
	svc := NewService(db)

	require.NoError(t, svc.ReindexBookByID(ctx, e.books[1].ID))
	require.NoError(t, svc.ReindexBookByID(ctx, e.books[0].ID))
	require.NoError(t, svc.ReindexBookByID(ctx, e.books[0].ID))

	var rows []struct {
		RowID  int `bun:"rowid"`
		BookID int `bun:"book_id"`
	}
	require.NoError(t, db.NewRaw("SELECT rowid, book_id FROM books_fts ORDER BY book_id").Scan(ctx, &rows))
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, row.BookID, row.RowID, "books_fts row for book %d should have rowid equal to the book id", row.BookID)
	}
}

func TestRebuildAllIndexes_KeysFTSRowsByEntityID(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	e := createFTSEntities(t, db)
	svc := NewService(db)

	// Seed rows with auto-assigned rowids in reverse order so a rebuild that
	// does not set rowid would leave them mismatched.
	for _, i := range []int{1, 0} {
		for table, info := range e.ftsTables() {
			textColumn := "name"
			if table == "books_fts" {
				textColumn = "title"
			}
			_, err := db.ExecContext(ctx, "INSERT INTO "+table+" ("+info.idColumn+", library_id, "+textColumn+") VALUES (?, ?, 'stale')", info.ids[i], e.library.ID)
			require.NoError(t, err, table)
		}
	}

	require.NoError(t, svc.RebuildAllIndexes(ctx))
	assertFTSRowidsMatchIDs(t, db, e)
}

// ftsDeleteRecorder captures every DELETE the service runs against an FTS
// table so its query plan can be inspected.
type ftsDeleteRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (r *ftsDeleteRecorder) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (r *ftsDeleteRecorder) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	q := strings.TrimSpace(event.Query)
	if strings.HasPrefix(q, "DELETE") && strings.Contains(q, "_fts") {
		r.mu.Lock()
		r.queries = append(r.queries, q)
		r.mu.Unlock()
	}
}

// Deleting one entity's FTS row must be a rowid lookup, not a scan of the
// whole virtual table. FTS5 reports its plan as "SCAN <table> VIRTUAL TABLE
// INDEX <idxNum>:<idxStr>", where idxStr carries one character per usable
// constraint and "=" is rowid equality. A filter on an UNINDEXED column such
// as book_id leaves idxStr empty, which is a full scan.
func TestDeleteFromIndex_UsesRowidLookup(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	e := createFTSEntities(t, db)
	svc := NewService(db)

	recorder := &ftsDeleteRecorder{}
	db.AddQueryHook(recorder)

	require.NoError(t, svc.DeleteFromBookIndex(ctx, e.books[0].ID))
	require.NoError(t, svc.DeleteFromSeriesIndex(ctx, e.series[0].ID))
	require.NoError(t, svc.DeleteFromPersonIndex(ctx, e.persons[0].ID))
	require.NoError(t, svc.DeleteFromGenreIndex(ctx, e.genres[0].ID))
	require.NoError(t, svc.DeleteFromTagIndex(ctx, e.tags[0].ID))
	require.NoError(t, svc.DeleteFromPublisherIndex(ctx, e.publishers[0].ID))
	require.NoError(t, svc.ReindexBookByID(ctx, e.books[1].ID))

	recorder.mu.Lock()
	queries := append([]string(nil), recorder.queries...)
	recorder.mu.Unlock()
	require.Len(t, queries, 7, "expected one FTS delete per call")

	for _, q := range queries {
		var plan []struct {
			ID      int    `bun:"id"`
			Parent  int    `bun:"parent"`
			NotUsed int    `bun:"notused"`
			Detail  string `bun:"detail"`
		}
		require.NoError(t, db.NewRaw("EXPLAIN QUERY PLAN "+q).Scan(ctx, &plan), q)
		require.Len(t, plan, 1, q)
		detail := plan[0].Detail
		idx := strings.LastIndex(detail, ":")
		require.NotEqual(t, -1, idx, "unexpected plan %q for %q", detail, q)
		assert.Contains(t, detail[idx+1:], "=", "delete should be a rowid lookup, got plan %q for %q", detail, q)
	}
}

// ftsRaceInjector simulates a second writer indexing the same entity: right
// after the service deletes an entity's FTS row, it inserts a row with the
// same rowid, so the service's own insert then meets an existing row. Errors
// from the injected insert are recorded for the test to assert on.
type ftsRaceInjector struct {
	db      *bun.DB
	pending map[string]string // table -> insert to run after its next delete
	errs    []error
}

func (r *ftsRaceInjector) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (r *ftsRaceInjector) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	for table, insert := range r.pending {
		if strings.HasPrefix(strings.TrimSpace(event.Query), "DELETE FROM "+table) {
			delete(r.pending, table)
			if _, err := r.db.ExecContext(ctx, insert); err != nil {
				r.errs = append(r.errs, fmt.Errorf("inject into %s: %w", table, err))
			}
		}
	}
}

// Two writers can index the same entity at once (for example a scan and an
// edit). Each deletes the row and then inserts it, so the second insert can
// find the first writer's row already holding the rowid. The service must
// replace that row rather than fail on the rowid conflict.
func TestIndexMethods_ReplaceRowWrittenConcurrently(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	e := createFTSEntities(t, db)
	svc := NewService(db)

	concurrentRow := func(table, idColumn string, id int) string {
		textColumn := "name"
		if table == "books_fts" {
			textColumn = "title"
		}
		return fmt.Sprintf("INSERT INTO %s (rowid, %s, library_id, %s) VALUES (%d, %d, %d, 'concurrent')",
			table, idColumn, textColumn, id, id, e.library.ID)
	}
	injector := &ftsRaceInjector{db: db, pending: map[string]string{}}
	for table, info := range e.ftsTables() {
		injector.pending[table] = concurrentRow(table, info.idColumn, info.ids[0])
	}
	db.AddQueryHook(injector)

	require.NoError(t, svc.IndexBook(ctx, e.books[0]))
	require.NoError(t, svc.IndexSeries(ctx, e.series[0]))
	require.NoError(t, svc.IndexPerson(ctx, e.persons[0]))
	require.NoError(t, svc.IndexGenre(ctx, e.genres[0]))
	require.NoError(t, svc.IndexTag(ctx, e.tags[0]))
	require.NoError(t, svc.IndexPublisher(ctx, e.publishers[0]))
	require.Empty(t, injector.errs)
	require.Empty(t, injector.pending, "every Index method should have deleted before inserting")

	// ReindexBookByID meets the same race on the other book.
	injector.pending["books_fts"] = concurrentRow("books_fts", "book_id", e.books[1].ID)
	require.NoError(t, svc.ReindexBookByID(ctx, e.books[1].ID))
	require.Empty(t, injector.errs)
	require.Empty(t, injector.pending)

	for table, info := range e.ftsTables() {
		var concurrent, total int
		require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM "+table+" WHERE "+table+" MATCH 'concurrent'").Scan(ctx, &concurrent))
		assert.Zero(t, concurrent, "%s should hold the service's row, not the concurrent one", table)
		require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM "+table+" WHERE "+info.idColumn+" = ?", info.ids[0]).Scan(ctx, &total))
		assert.Equal(t, 1, total, "%s should hold one row for %s %d", table, info.idColumn, info.ids[0])
	}
}
