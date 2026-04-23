package libraries

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })

	return db
}

// seedLibraryWithContent creates a library with one book, one file, one series,
// one person, one genre, one tag, and corresponding FTS entries. Returns the
// library ID and the seeded entity IDs for assertions.
type seededIDs struct {
	LibraryID int
	BookID    int
	FileID    int
	SeriesID  int
	PersonID  int
	GenreID   int
	TagID     int
}

func seedLibraryWithContent(t *testing.T, ctx context.Context, db *bun.DB, name string) seededIDs {
	t.Helper()

	library := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Returning("*").Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Seeded Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Seeded Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/seeded",
	}
	_, err = db.NewInsert().Model(book).Returning("*").Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/seeded/book.epub",
		FileType:      "epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Returning("*").Exec(ctx)
	require.NoError(t, err)

	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Seeded Series",
		NameSource:     models.DataSourceFilepath,
		SortName:       "Seeded Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(series).Returning("*").Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{
		LibraryID: library.ID,
		Name:      "Seeded Person",
		SortName:  "Person, Seeded",
	}
	_, err = db.NewInsert().Model(person).Returning("*").Exec(ctx)
	require.NoError(t, err)

	genre := &models.Genre{
		LibraryID: library.ID,
		Name:      "Seeded Genre",
	}
	_, err = db.NewInsert().Model(genre).Returning("*").Exec(ctx)
	require.NoError(t, err)

	tag := &models.Tag{
		LibraryID: library.ID,
		Name:      "Seeded Tag",
	}
	_, err = db.NewInsert().Model(tag).Returning("*").Exec(ctx)
	require.NoError(t, err)

	// Seed minimal FTS rows directly (Index* methods have relation loading
	// requirements that are overkill for this test's needs).
	_, err = db.ExecContext(ctx,
		`INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		 VALUES (?, ?, ?, ?, '', '', '', '', '')`,
		book.ID, library.ID, book.Title, book.Filepath)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO series_fts (series_id, library_id, name, description, book_titles, book_authors)
		 VALUES (?, ?, ?, '', '', '')`,
		series.ID, library.ID, series.Name)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO persons_fts (person_id, library_id, name, sort_name) VALUES (?, ?, ?, ?)`,
		person.ID, library.ID, person.Name, person.SortName)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO genres_fts (genre_id, library_id, name) VALUES (?, ?, ?)`,
		genre.ID, library.ID, genre.Name)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO tags_fts (tag_id, library_id, name) VALUES (?, ?, ?)`,
		tag.ID, library.ID, tag.Name)
	require.NoError(t, err)

	return seededIDs{
		LibraryID: library.ID,
		BookID:    book.ID,
		FileID:    file.ID,
		SeriesID:  series.ID,
		PersonID:  person.ID,
		GenreID:   genre.ID,
		TagID:     tag.ID,
	}
}

func TestDeleteLibrary_RemovesRowAndCascades(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	seeded := seedLibraryWithContent(t, ctx, db, "Test Library")

	err := svc.DeleteLibrary(ctx, seeded.LibraryID)
	require.NoError(t, err)

	// Library row removed.
	libCount, err := db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", seeded.LibraryID).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, libCount, "library row should be gone")

	// CASCADE removed children.
	for name, count := range map[string]func() (int, error){
		"books": func() (int, error) {
			return db.NewSelect().Model((*models.Book)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx)
		},
		"files": func() (int, error) {
			return db.NewSelect().Model((*models.File)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx)
		},
		"series": func() (int, error) {
			return db.NewSelect().Model((*models.Series)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx)
		},
		"persons": func() (int, error) {
			return db.NewSelect().Model((*models.Person)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx)
		},
		"genres": func() (int, error) {
			return db.NewSelect().Model((*models.Genre)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx)
		},
		"tags": func() (int, error) {
			return db.NewSelect().Model((*models.Tag)(nil)).Where("library_id = ?", seeded.LibraryID).Count(ctx)
		},
	} {
		n, err := count()
		require.NoError(t, err, name)
		assert.Zero(t, n, "%s rows should be cascaded", name)
	}
}

func TestDeleteLibrary_PurgesFTS(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	seeded := seedLibraryWithContent(t, ctx, db, "FTS Library")

	err := svc.DeleteLibrary(ctx, seeded.LibraryID)
	require.NoError(t, err)

	for _, table := range []string{"books_fts", "series_fts", "persons_fts", "genres_fts", "tags_fts"} {
		var count int
		err := db.NewSelect().TableExpr(table).ColumnExpr("COUNT(*)").Where("library_id = ?", seeded.LibraryID).Scan(ctx, &count)
		require.NoError(t, err, table)
		assert.Zero(t, count, "%s rows for deleted library should be purged", table)
	}
}

func TestDeleteLibrary_CancelsActiveJobs(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Target library.
	lib := &models.Library{Name: "Target", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Returning("*").Exec(ctx)
	require.NoError(t, err)

	// Second library whose jobs should not be touched.
	other := &models.Library{Name: "Other", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err = db.NewInsert().Model(other).Returning("*").Exec(ctx)
	require.NoError(t, err)

	insertJob := func(status string, libraryID *int) int {
		j := &models.Job{
			Type:      models.JobTypeScan,
			Status:    status,
			LibraryID: libraryID,
			Data:      "{}",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err := db.NewInsert().Model(j).Returning("*").Exec(ctx)
		require.NoError(t, err)
		return j.ID
	}

	pendingID := insertJob(models.JobStatusPending, &lib.ID)
	runningID := insertJob(models.JobStatusInProgress, &lib.ID)
	completedID := insertJob(models.JobStatusCompleted, &lib.ID)
	otherLibraryJobID := insertJob(models.JobStatusPending, &other.ID)
	globalJobID := insertJob(models.JobStatusPending, nil)

	err = svc.DeleteLibrary(ctx, lib.ID)
	require.NoError(t, err)

	loadStatus := func(id int) string {
		var j models.Job
		err := db.NewSelect().Model(&j).Where("id = ?", id).Scan(ctx)
		require.NoError(t, err)
		return j.Status
	}

	assert.Equal(t, models.JobStatusFailed, loadStatus(pendingID), "pending job for deleted library should be failed")
	assert.Equal(t, models.JobStatusFailed, loadStatus(runningID), "running job for deleted library should be failed")
	assert.Equal(t, models.JobStatusCompleted, loadStatus(completedID), "completed job must not change")
	assert.Equal(t, models.JobStatusPending, loadStatus(otherLibraryJobID), "other library's job must not change")
	assert.Equal(t, models.JobStatusPending, loadStatus(globalJobID), "global job (no library_id) must not change")
}

func TestDeleteLibrary_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	err := svc.DeleteLibrary(ctx, 99999)
	require.Error(t, err)

	var codeErr *errcodes.Error
	require.True(t, errors.As(err, &codeErr), "error must wrap errcodes.Error, got %T: %v", err, err)
	assert.Equal(t, "not_found", codeErr.Code)
}
