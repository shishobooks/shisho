package books

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func relationsFixture(t *testing.T) (*Service, *bun.DB) {
	t.Helper()
	db := testdb.New(t)
	for _, stmt := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'L', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source) VALUES (1, 1, '/b', 'B', 'file', 'B', 'file')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type) VALUES (1, 1, 1, '/b/b.m4b', 'm4b')`,
		`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'A', 'A', 'file'), (2, 1, 'B', 'B', 'file'), (3, 1, 'C', 'C', 'file')`,
		`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source) VALUES (1, 1, 'S', 'file', 'S', 'file'), (2, 1, 'T', 'file', 'T', 'file')`,
		`INSERT INTO genres (id, library_id, name) VALUES (1, 1, 'G'), (2, 1, 'H')`,
		`INSERT INTO tags (id, library_id, name) VALUES (1, 1, 'X'), (2, 1, 'Y')`,
	} {
		_, err := db.ExecContext(context.Background(), stmt)
		require.NoError(t, err, stmt)
	}
	return NewService(db, appsettings.NewService(db)), db
}

// rows lists a join table's rows as "id:columns", so a test sees both what
// changed and which rows kept their ids.
func rows(t *testing.T, db *bun.DB, query string) []string {
	t.Helper()
	var out []string
	require.NoError(t, db.NewRaw(query).Scan(context.Background(), &out))
	return out
}

func changeLog(t *testing.T, db *bun.DB) []string {
	t.Helper()
	return rows(t, db, `SELECT kind || ':' || record_id || ':' || seq FROM change_log_entries ORDER BY seq`)
}

func TestReplaceAuthors_UpdatesInPlace(t *testing.T) {
	t.Parallel()
	svc, db := relationsFixture(t)
	ctx := context.Background()
	writer := models.AuthorRoleWriter
	authors := func(specs ...[2]int) []*models.Author {
		out := []*models.Author{}
		for i, spec := range specs {
			a := &models.Author{BookID: 1, PersonID: spec[0], SortOrder: i + 1}
			if spec[1] == 1 {
				a.Role = &writer
			}
			out = append(out, a)
		}
		return out
	}
	query := `SELECT id || ':' || person_id || ':' || COALESCE(role, '') || ':' || sort_order FROM authors ORDER BY id`

	require.NoError(t, svc.ReplaceAuthors(ctx, 1, authors([2]int{1, 0}, [2]int{2, 1})))
	assert.Equal(t, []string{"1:1::1", "2:2:writer:2"}, rows(t, db, query))

	// The same authors again write nothing.
	log := changeLog(t, db)
	require.NoError(t, svc.ReplaceAuthors(ctx, 1, authors([2]int{1, 0}, [2]int{2, 1})))
	assert.Equal(t, []string{"1:1::1", "2:2:writer:2"}, rows(t, db, query))
	assert.Equal(t, log, changeLog(t, db))

	// Reordered, they keep their rows.
	require.NoError(t, svc.ReplaceAuthors(ctx, 1, authors([2]int{2, 1}, [2]int{1, 0})))
	assert.Equal(t, []string{"1:1::2", "2:2:writer:1"}, rows(t, db, query))

	// One replaced, one kept.
	require.NoError(t, svc.ReplaceAuthors(ctx, 1, authors([2]int{2, 1}, [2]int{3, 0})))
	assert.Equal(t, []string{"2:2:writer:1", "3:3::2"}, rows(t, db, query))

	require.NoError(t, svc.ReplaceAuthors(ctx, 1, nil))
	assert.Empty(t, rows(t, db, query))
}

func TestReplaceOtherRelations_UpdateInPlace(t *testing.T) {
	t.Parallel()
	svc, db := relationsFixture(t)
	ctx := context.Background()
	two := 2.0

	require.NoError(t, svc.ReplaceBookSeries(ctx, 1, []*models.BookSeries{{BookID: 1, SeriesID: 1, SortOrder: 1}, {BookID: 1, SeriesID: 2, SortOrder: 2}}))
	require.NoError(t, svc.ReplaceBookGenres(ctx, 1, []*models.BookGenre{{BookID: 1, GenreID: 1}, {BookID: 1, GenreID: 2}}))
	require.NoError(t, svc.ReplaceBookTags(ctx, 1, []*models.BookTag{{BookID: 1, TagID: 1}}))
	require.NoError(t, svc.ReplaceNarrators(ctx, 1, []*models.Narrator{{FileID: 1, PersonID: 1, SortOrder: 1}, {FileID: 1, PersonID: 2, SortOrder: 2}}))
	log := changeLog(t, db)

	require.NoError(t, svc.ReplaceBookSeries(ctx, 1, []*models.BookSeries{{BookID: 1, SeriesID: 1, SortOrder: 1}, {BookID: 1, SeriesID: 2, SortOrder: 2}}))
	assert.Equal(t, log, changeLog(t, db), "the same series again")

	require.NoError(t, svc.ReplaceBookSeries(ctx, 1, []*models.BookSeries{{BookID: 1, SeriesID: 2, SortOrder: 1, SeriesNumber: &two}}))
	assert.Equal(t, []string{"2:2:2.0:1"}, rows(t, db, `SELECT id || ':' || series_id || ':' || COALESCE(series_number, '') || ':' || sort_order FROM book_series`))

	require.NoError(t, svc.ReplaceBookGenres(ctx, 1, []*models.BookGenre{{BookID: 1, GenreID: 2}}))
	assert.Equal(t, []string{"2:2"}, rows(t, db, `SELECT id || ':' || genre_id FROM book_genres`))
	require.NoError(t, svc.ReplaceBookTags(ctx, 1, []*models.BookTag{{BookID: 1, TagID: 1}, {BookID: 1, TagID: 2}}))
	assert.Equal(t, []string{"1:1", "2:2"}, rows(t, db, `SELECT id || ':' || tag_id FROM book_tags ORDER BY id`))
	require.NoError(t, svc.ReplaceNarrators(ctx, 1, []*models.Narrator{{FileID: 1, PersonID: 2, SortOrder: 1}, {FileID: 1, PersonID: 1, SortOrder: 2}}))
	assert.Equal(t, []string{"1:1:2", "2:2:1"}, rows(t, db, `SELECT id || ':' || person_id || ':' || sort_order FROM narrators ORDER BY id`))
}
