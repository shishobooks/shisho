package search

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// aliasQueryRecorder captures every query the service runs against an alias
// table so its query plan can be inspected.
type aliasQueryRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (r *aliasQueryRecorder) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (r *aliasQueryRecorder) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	if strings.Contains(event.Query, "_aliases") {
		r.mu.Lock()
		r.queries = append(r.queries, event.Query)
		r.mu.Unlock()
	}
}

var aliasTablePattern = regexp.MustCompile(`\b(person|series|genre|tag|publisher)_aliases\b`)

// Indexing reads a resource's aliases by the alias table's parent column,
// once per book in ReindexBookByID and once per resource in the Index*
// methods. Each of those reads must search the parent column's index rather
// than scan the alias table or build a temporary automatic index.
func TestIndexing_AliasLookupsUseParentIndex(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	e := createFTSEntities(t, db)
	svc := NewService(db)

	lib := e.library.ID
	book := e.books[0].ID
	person := e.persons[0].ID
	series := e.series[0].ID
	_, err := db.NewRaw("INSERT INTO authors (book_id, person_id, sort_order) VALUES (?, ?, 0)", book, person).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewRaw("INSERT INTO book_series (book_id, series_id, sort_order) VALUES (?, ?, 0)", book, series).Exec(ctx)
	require.NoError(t, err)
	var fileID int
	require.NoError(t, db.NewRaw(
		"INSERT INTO files (library_id, book_id, filepath, file_type) VALUES (?, ?, '/test/alpha.m4b', 'm4b') RETURNING id",
		lib, book,
	).Scan(ctx, &fileID))
	_, err = db.NewRaw("INSERT INTO narrators (file_id, person_id, sort_order) VALUES (?, ?, 0)", fileID, person).Exec(ctx)
	require.NoError(t, err)
	for _, alias := range []struct {
		table, fk string
		parentID  int
	}{
		{"person_aliases", "person_id", person},
		{"series_aliases", "series_id", series},
		{"genre_aliases", "genre_id", e.genres[0].ID},
		{"tag_aliases", "tag_id", e.tags[0].ID},
		{"publisher_aliases", "publisher_id", e.publishers[0].ID},
	} {
		_, err := db.NewRaw(
			"INSERT INTO "+alias.table+" ("+alias.fk+", name, library_id) VALUES (?, ?, ?)",
			alias.parentID, "Nickname "+alias.fk, lib,
		).Exec(ctx)
		require.NoError(t, err)
	}

	recorder := &aliasQueryRecorder{}
	db.AddQueryHook(recorder)

	require.NoError(t, svc.ReindexBookByID(ctx, book))
	require.NoError(t, svc.IndexSeries(ctx, e.series[0]))
	require.NoError(t, svc.IndexPerson(ctx, e.persons[0]))
	require.NoError(t, svc.IndexGenre(ctx, e.genres[0]))
	require.NoError(t, svc.IndexTag(ctx, e.tags[0]))
	require.NoError(t, svc.IndexPublisher(ctx, e.publishers[0]))

	recorder.mu.Lock()
	queries := append([]string(nil), recorder.queries...)
	recorder.mu.Unlock()
	require.NotEmpty(t, queries)

	seen := map[string]bool{}
	for _, q := range queries {
		joined := strings.Join(queryPlan(ctx, t, db, q), "\n")
		assert.NotContains(t, joined, "AUTOMATIC", "alias lookup should not build an automatic index:\n%s\n%s", q, joined)
		for _, match := range aliasTablePattern.FindAllStringSubmatch(q, -1) {
			resource := match[1]
			seen[resource] = true
			index := "INDEX ix_" + resource + "_aliases_" + resource + "_id (" + resource + "_id=?)"
			assert.Contains(t, joined, index, "alias lookup should search %s:\n%s\n%s", index, q, joined)
		}
	}
	for _, resource := range []string{"person", "series", "genre", "tag", "publisher"} {
		assert.True(t, seen[resource], "expected a %s_aliases lookup", resource)
	}
}

// queryPlan returns the detail column of each EXPLAIN QUERY PLAN row for q.
func queryPlan(ctx context.Context, t *testing.T, db *bun.DB, q string) []string {
	t.Helper()
	var plan []struct {
		ID      int    `bun:"id"`
		Parent  int    `bun:"parent"`
		NotUsed int    `bun:"notused"`
		Detail  string `bun:"detail"`
	}
	require.NoError(t, db.NewRaw("EXPLAIN QUERY PLAN "+q).Scan(ctx, &plan), q)
	details := make([]string, 0, len(plan))
	for _, row := range plan {
		details = append(details, row.Detail)
	}
	return details
}
