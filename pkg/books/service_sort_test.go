package books

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupBooksTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	// :memory: SQLite is per-connection — multiple connections each have
	// their own (empty) database. Pinning to a single connection ensures
	// the migrated schema is visible to every operation in the test.
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })
	return db
}

func seedLibrary(t *testing.T, db *bun.DB, name string) *models.Library {
	t.Helper()
	l := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(l).Exec(context.Background())
	require.NoError(t, err)
	return l
}

func seedBook(t *testing.T, db *bun.DB, lib *models.Library, title, sortTitle string, createdAt time.Time) *models.Book {
	t.Helper()
	b := &models.Book{
		LibraryID:       lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       sortTitle,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/test/" + title + ".epub",
		CreatedAt:       createdAt,
	}
	_, err := db.NewInsert().Model(b).Exec(context.Background())
	require.NoError(t, err)
	return b
}

// TestListBooks_SortByTitleAsc confirms an explicit Sort overrides the
// default sort_title ASC ordering.
func TestListBooks_SortByTitleAsc(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	// Titles intentionally in non-alphabetic insertion order.
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now.Add(-2*time.Hour))
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))
	banana := seedBook(t, db, lib, "Banana", "Banana", now)

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldTitle, Direction: sortspec.DirAsc}},
	})
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, apple.ID, got[0].ID)
	assert.Equal(t, banana.ID, got[1].ID)
	assert.Equal(t, cheese.ID, got[2].ID)
}

// TestListBooks_SortByDateAddedDesc confirms the primary use case for the
// frontend's builtin default.
func TestListBooks_SortByDateAddedDesc(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	oldest := seedBook(t, db, lib, "Oldest", "Oldest", now.Add(-3*time.Hour))
	middle := seedBook(t, db, lib, "Middle", "Middle", now.Add(-time.Hour))
	newest := seedBook(t, db, lib, "Newest", "Newest", now)

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, newest.ID, got[0].ID)
	assert.Equal(t, middle.ID, got[1].ID)
	assert.Equal(t, oldest.ID, got[2].ID)
}

// TestListBooks_NilSortUsesDefault preserves backward compatibility.
func TestListBooks_NilSortUsesDefault(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      nil,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	// Default is sort_title ASC → Apple before Cheese.
	assert.Equal(t, apple.ID, got[0].ID)
	assert.Equal(t, cheese.ID, got[1].ID)
}
