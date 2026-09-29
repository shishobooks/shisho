package tags

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func createTestLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return lib
}

func TestFindOrCreateTag_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	tag := &models.Tag{LibraryID: lib.ID, Name: "Fantasy"}
	err := svc.CreateTag(ctx, tag)
	require.NoError(t, err)

	found, err := svc.FindOrCreateTag(ctx, "Fantasy", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, tag.ID, found.ID)
}

func TestFindOrCreateTag_AliasMatch(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	tag := &models.Tag{LibraryID: lib.ID, Name: "Science Fiction"}
	err := svc.CreateTag(ctx, tag)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO tag_aliases (created_at, tag_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), tag.ID, "Sci-Fi", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreateTag(ctx, "Sci-Fi", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, tag.ID, found.ID)
	assert.Equal(t, "Science Fiction", found.Name)
}

func TestFindOrCreateTag_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	found, err := svc.FindOrCreateTag(ctx, "Horror", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Horror", found.Name)
	assert.Equal(t, lib.ID, found.LibraryID)
}
