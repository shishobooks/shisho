package books

import (
	"context"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanupOrphanedSeries_ReturnsDeletedIDs pins the requirement that
// CleanupOrphanedSeries returns the IDs of deleted series so callers can keep
// series_fts in sync. Without this, orphan-series cleanup silently leaves
// stale FTS rows that surface in search results pointing at non-existent
// series.
func TestCleanupOrphanedSeries_ReturnsDeletedIDs(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Series with a book must NOT be cleaned up.
	keep := &models.Series{
		LibraryID:      library.ID,
		Name:           "Kept Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Kept Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(keep).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	bs := &models.BookSeries{BookID: book.ID, SeriesID: keep.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	// Orphan series with no book must be cleaned up.
	orphan := &models.Series{
		LibraryID:      library.ID,
		Name:           "Orphan Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Orphan Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(orphan).Exec(ctx)
	require.NoError(t, err)

	deletedIDs, err := NewService(db, appsettings.NewService(db)).CleanupOrphanedSeries(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{orphan.ID}, deletedIDs,
		"CleanupOrphanedSeries must return the IDs of deleted series so callers can purge FTS")

	// Orphan row is gone.
	count, err := db.NewSelect().Model((*models.Series)(nil)).
		Where("id = ?", orphan.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "orphan series row should be deleted")

	// Kept series remains.
	count, err = db.NewSelect().Model((*models.Series)(nil)).
		Where("id = ?", keep.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "series with books must not be cleaned up")
}

// CleanupOrphanedEntities deletes every unreferenced Series, Person, Genre,
// Tag, and Publisher, removes each from its FTS table, and leaves referenced
// ones alone.
func TestCleanupOrphanedEntities_DeletesOrphansAndTheirFTSRows(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	searchSvc := search.NewService(db)
	library, book := setupTestLibraryAndBook(t, db)
	insert := func(model any) {
		t.Helper()
		_, err := db.NewInsert().Model(model).Exec(ctx)
		require.NoError(t, err)
	}

	keptSeries := &models.Series{LibraryID: library.ID, Name: "Kept Saga", NameSource: models.DataSourceManual, SortName: "Kept Saga", SortNameSource: models.DataSourceFilepath}
	insert(keptSeries)
	insert(&models.BookSeries{BookID: book.ID, SeriesID: keptSeries.ID, SortOrder: 1})
	keptPerson := &models.Person{LibraryID: library.ID, Name: "Kept Author", SortName: "Author, Kept", SortNameSource: models.DataSourceFilepath}
	insert(keptPerson)
	insert(&models.Author{BookID: book.ID, PersonID: keptPerson.ID, SortOrder: 1})

	orphanSeries := &models.Series{LibraryID: library.ID, Name: "Orphan Saga", NameSource: models.DataSourceManual, SortName: "Orphan Saga", SortNameSource: models.DataSourceFilepath}
	insert(orphanSeries)
	orphanPerson := &models.Person{LibraryID: library.ID, Name: "Orphan Author", SortName: "Author, Orphan", SortNameSource: models.DataSourceFilepath}
	insert(orphanPerson)
	orphanGenre := &models.Genre{LibraryID: library.ID, Name: "Orphan Genre"}
	insert(orphanGenre)
	orphanTag := &models.Tag{LibraryID: library.ID, Name: "Orphan Tag"}
	insert(orphanTag)
	orphanPublisher := &models.Publisher{LibraryID: library.ID, Name: "Orphan Press"}
	insert(orphanPublisher)

	require.NoError(t, searchSvc.IndexSeries(ctx, keptSeries))
	require.NoError(t, searchSvc.IndexPerson(ctx, keptPerson))
	require.NoError(t, searchSvc.IndexSeries(ctx, orphanSeries))
	require.NoError(t, searchSvc.IndexPerson(ctx, orphanPerson))
	require.NoError(t, searchSvc.IndexGenre(ctx, orphanGenre))
	require.NoError(t, searchSvc.IndexTag(ctx, orphanTag))
	require.NoError(t, searchSvc.IndexPublisher(ctx, orphanPublisher))

	CleanupOrphanedEntities(ctx, logger.New(), OrphanCleanupServices{
		Books:      NewService(db, appsettings.NewService(db)),
		People:     people.NewService(db),
		Genres:     genres.NewService(db),
		Tags:       tags.NewService(db),
		Publishers: publishers.NewService(db),
		Search:     searchSvc,
	})

	count := func(query string, id int) int {
		t.Helper()
		var n int
		require.NoError(t, db.NewRaw(query, id).Scan(ctx, &n))
		return n
	}
	orphans := []struct {
		name  string
		table string
		fts   string
		id    int
	}{
		{"series", "series", "series_fts", orphanSeries.ID},
		{"person", "persons", "persons_fts", orphanPerson.ID},
		{"genre", "genres", "genres_fts", orphanGenre.ID},
		{"tag", "tags", "tags_fts", orphanTag.ID},
		{"publisher", "publishers", "publishers_fts", orphanPublisher.ID},
	}
	for _, o := range orphans {
		assert.Equal(t, 0, count("SELECT count(*) FROM "+o.table+" WHERE id = ?", o.id), "the orphaned %s row is deleted", o.name)
		assert.Equal(t, 0, count("SELECT count(*) FROM "+o.fts+" WHERE rowid = ?", o.id), "the orphaned %s leaves its FTS table", o.name)
	}
	assert.Equal(t, 1, count("SELECT count(*) FROM series WHERE id = ?", keptSeries.ID), "a Series with a Book stays")
	assert.Equal(t, 1, count("SELECT count(*) FROM series_fts WHERE rowid = ?", keptSeries.ID), "a Series with a Book stays indexed")
	assert.Equal(t, 1, count("SELECT count(*) FROM persons WHERE id = ?", keptPerson.ID), "a Person who authors a Book stays")
	assert.Equal(t, 1, count("SELECT count(*) FROM persons_fts WHERE rowid = ?", keptPerson.ID), "a Person who authors a Book stays indexed")
}
