package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// ftsFixture is a small library that exercises every FTS column: shared
// authors across the Books of one Series, narrators, aliases on every
// resource, a Book in two Series, and a Book in none.
type ftsFixture struct {
	library    *models.Library
	books      []*models.Book
	series     []*models.Series
	persons    []*models.Person
	genres     []*models.Genre
	tags       []*models.Tag
	publishers []*models.Publisher
}

func createFTSFixture(t *testing.T, db *bun.DB) *ftsFixture {
	t.Helper()
	ctx := context.Background()
	insert := func(model any) {
		t.Helper()
		_, err := db.NewInsert().Model(model).Exec(ctx)
		require.NoError(t, err)
	}
	f := &ftsFixture{library: &models.Library{Name: "Library", CoverAspectRatio: "book"}}
	insert(f.library)
	lib := f.library.ID

	for _, name := range []string{"Harbor Lights", "Quiet Tides", "Ember Garden"} {
		subtitle := name + " Subtitle"
		b := &models.Book{
			LibraryID: lib, Filepath: "/library/" + name, Title: name, TitleSource: "file",
			SortTitle: name, SortTitleSource: "file", AuthorSource: "file", Subtitle: &subtitle,
		}
		insert(b)
		f.books = append(f.books, b)
	}
	for _, name := range []string{"Lanternfall Cycle", "Moonwake Saga"} {
		description := name + " description"
		s := &models.Series{LibraryID: lib, Name: name, NameSource: "file", SortName: name, SortNameSource: "file", Description: &description}
		insert(s)
		f.series = append(f.series, s)
	}
	for _, name := range []string{"Zephyrine Quillfeather", "Ottoline Brackenridge", "Marigold Ashcombe"} {
		p := &models.Person{LibraryID: lib, Name: name, SortName: name, SortNameSource: "file"}
		insert(p)
		f.persons = append(f.persons, p)
	}
	g := &models.Genre{LibraryID: lib, Name: "Solarpunk"}
	insert(g)
	f.genres = append(f.genres, g)
	tg := &models.Tag{LibraryID: lib, Name: "Wistful"}
	insert(tg)
	f.tags = append(f.tags, tg)
	pub := &models.Publisher{LibraryID: lib, Name: "Driftwood Press"}
	insert(pub)
	f.publishers = append(f.publishers, pub)

	for _, alias := range []struct {
		model any
	}{
		{&models.PersonAlias{PersonID: f.persons[0].ID, Name: "Zeph Quill", LibraryID: lib}},
		{&models.PersonAlias{PersonID: f.persons[1].ID, Name: "Otto Bracken", LibraryID: lib}},
		{&models.SeriesAlias{SeriesID: f.series[0].ID, Name: "Lantern Books", LibraryID: lib}},
		{&models.GenreAlias{GenreID: g.ID, Name: "Hopepunk", LibraryID: lib}},
		{&models.TagAlias{TagID: tg.ID, Name: "Melancholy", LibraryID: lib}},
		{&models.PublisherAlias{PublisherID: pub.ID, Name: "Driftwood", LibraryID: lib}},
	} {
		insert(alias.model)
	}

	// The first two Books share an author and sit in the first Series; the
	// second is also in the second Series. The third has no Series.
	insert(&models.Author{BookID: f.books[0].ID, PersonID: f.persons[0].ID, SortOrder: 1})
	insert(&models.Author{BookID: f.books[1].ID, PersonID: f.persons[0].ID, SortOrder: 1})
	insert(&models.Author{BookID: f.books[1].ID, PersonID: f.persons[2].ID, SortOrder: 2})
	insert(&models.Author{BookID: f.books[2].ID, PersonID: f.persons[2].ID, SortOrder: 1})
	insert(&models.BookSeries{BookID: f.books[0].ID, SeriesID: f.series[0].ID, SortOrder: 1})
	insert(&models.BookSeries{BookID: f.books[1].ID, SeriesID: f.series[0].ID, SortOrder: 1})
	insert(&models.BookSeries{BookID: f.books[1].ID, SeriesID: f.series[1].ID, SortOrder: 2})

	for i, b := range f.books {
		for j, ext := range []string{"m4b", "epub"} {
			file := &models.File{
				LibraryID: lib, BookID: b.ID, Filepath: fmt.Sprintf("%s/part%d.%s", b.Filepath, j, ext),
				FileType: ext, FileRole: models.FileRoleMain, FilesizeBytes: 1, PublisherID: &pub.ID,
			}
			insert(file)
			if ext == "m4b" && i < 2 {
				insert(&models.Narrator{FileID: file.ID, PersonID: f.persons[1].ID, SortOrder: 1})
			}
		}
	}
	return f
}

// ftsSnapshot reads every FTS table into rowid -> column -> sorted tokens, so
// two indexes compare equal when they hold the same words per column,
// whatever order GROUP_CONCAT emitted them in.
func ftsSnapshot(t *testing.T, db *bun.DB) map[string]map[int]map[string]string {
	t.Helper()
	ctx := context.Background()
	tables := map[string][]string{
		"books_fts":      {"book_id", "library_id", "title", "filepath", "subtitle", "authors", "filenames", "narrators", "series_names"},
		"series_fts":     {"series_id", "library_id", "name", "description", "book_titles", "book_authors"},
		"persons_fts":    {"person_id", "library_id", "name", "sort_name"},
		"genres_fts":     {"genre_id", "library_id", "name"},
		"tags_fts":       {"tag_id", "library_id", "name"},
		"publishers_fts": {"publisher_id", "library_id", "name"},
	}
	out := map[string]map[int]map[string]string{}
	for table, columns := range tables {
		rows, err := db.QueryContext(ctx, "SELECT rowid, "+strings.Join(columns, ", ")+" FROM "+table)
		require.NoError(t, err)
		out[table] = map[int]map[string]string{}
		for rows.Next() {
			var rowid int
			values := make([]any, len(columns))
			ptrs := []any{&rowid}
			for i := range values {
				ptrs = append(ptrs, &values[i])
			}
			require.NoError(t, rows.Scan(ptrs...))
			row := map[string]string{}
			for i, column := range columns {
				tokens := strings.Fields(fmt.Sprint(values[i]))
				sort.Strings(tokens)
				row[column] = strings.Join(tokens, " ")
			}
			out[table][rowid] = row
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
	return out
}

// The per-entity index methods, ReindexAffected, and RebuildAllIndexes must
// write the same rows.
// A scan rebuilds every table, so any difference is search that changes
// meaning depending on whether an edit or a scan wrote the row last.
func TestIndexMethods_MatchRebuildAllIndexes(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)

	for _, b := range f.books {
		require.NoError(t, svc.IndexBook(ctx, loadBookForIndex(t, db, b.ID)))
	}
	for _, s := range f.series {
		require.NoError(t, svc.IndexSeries(ctx, s))
	}
	for _, p := range f.persons {
		require.NoError(t, svc.IndexPerson(ctx, p))
	}
	require.NoError(t, svc.IndexGenre(ctx, f.genres[0]))
	require.NoError(t, svc.IndexTag(ctx, f.tags[0]))
	require.NoError(t, svc.IndexPublisher(ctx, f.publishers[0]))
	incremental := ftsSnapshot(t, db)

	require.NoError(t, svc.RebuildAllIndexes(ctx))
	rebuilt := ftsSnapshot(t, db)

	for table := range rebuilt {
		assert.Equal(t, rebuilt[table], incremental[table], "%s differs between the Index methods and RebuildAllIndexes", table)
	}

	// ReindexAffected reaches every row from the People, Genres, Tags, and
	// Publishers alone: People expand to their Books and those to their
	// Series.
	clearFTS(t, db)
	affected := Affected{GenreIDs: []int{f.genres[0].ID}, TagIDs: []int{f.tags[0].ID}, PublisherIDs: []int{f.publishers[0].ID}}
	for _, p := range f.persons {
		affected.PersonIDs = append(affected.PersonIDs, p.ID)
	}
	svc.ReindexAffected(ctx, &affected)
	reindexed := ftsSnapshot(t, db)
	for table := range rebuilt {
		assert.Equal(t, rebuilt[table], reindexed[table], "%s differs between ReindexAffected and RebuildAllIndexes", table)
	}
}

// loadBookForIndex loads a Book with the relations the handlers pass to
// IndexBook.
func loadBookForIndex(t *testing.T, db *bun.DB, id int) *models.Book {
	t.Helper()
	book := &models.Book{}
	require.NoError(t, db.NewSelect().Model(book).
		Relation("Authors.Person").
		Relation("BookSeries.Series").
		Relation("Files.Narrators.Person").
		Where("b.id = ?", id).
		Scan(context.Background()))
	return book
}

// RebuildAllIndexes clears every FTS table before refilling it. Run outside a
// transaction, a failure part way through leaves the tables empty, and
// searches that land mid-rebuild find nothing.
func TestRebuildAllIndexes_FailureKeepsPreviousIndex(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	f := createFTSFixture(t, db)
	svc := NewService(db)
	require.NoError(t, svc.RebuildAllIndexes(ctx))
	before := ftsSnapshot(t, db)
	require.NotEmpty(t, before["books_fts"])

	// The books insert reads series_aliases, so dropping it makes the rebuild
	// fail right after the DELETEs.
	_, err := db.ExecContext(ctx, "DROP TABLE series_aliases")
	require.NoError(t, err)

	require.Error(t, svc.RebuildAllIndexes(ctx))

	assert.Equal(t, before, ftsSnapshot(t, db), "a failed rebuild leaves the previous index in place")
	results, _, err := svc.SearchBooks(ctx, f.library.ID, "Harbor", nil, 10, 0)
	require.NoError(t, err)
	assert.Len(t, results, 1, "search still finds the Book")
}
