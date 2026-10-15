package tags

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tagNames(tags []*models.Tag) []string {
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return names
}

func TestListTags_DefaultsToNameAscending(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	seedTagWithBooks(t, db, lib, "Beta", []string{"B1"})
	seedTagWithBooks(t, db, lib, "Alpha", []string{"A1", "A2"})

	tags, err := NewService(db).ListTags(context.Background(), ListTagsOptions{LibraryID: &lib.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{"Alpha", "Beta"}, tagNames(tags))
}

func TestListTags_SortsByBookCount(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	seedTagWithBooks(t, db, lib, "One", []string{"A"})
	seedTagWithBooks(t, db, lib, "Three", []string{"B", "C", "D"})
	seedTagWithBooks(t, db, lib, "Two", []string{"E", "F"})
	seedTagWithBooks(t, db, lib, "Also Two", []string{"G", "H"})

	tags, err := NewService(db).ListTags(context.Background(), ListTagsOptions{
		LibraryID: &lib.ID,
		Sort: []sortspec.SortLevel{
			{Field: sortspec.FieldBookCount, Direction: sortspec.DirDesc},
			{Field: sortspec.FieldName, Direction: sortspec.DirAsc},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Three", "Also Two", "Two", "One"}, tagNames(tags))
}

func TestListTags_SortsByDateAdded(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	ctx := context.Background()
	svc := NewService(db)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, seed := range []struct {
		name string
		age  time.Duration
	}{{"Oldest", 0}, {"Newest", 2 * time.Hour}, {"Middle", time.Hour}} {
		require.NoError(t, svc.CreateTag(ctx, &models.Tag{LibraryID: lib.ID, Name: seed.name, CreatedAt: base.Add(seed.age)}))
	}

	tags, err := svc.ListTags(ctx, ListTagsOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, tagNames(tags))
}
