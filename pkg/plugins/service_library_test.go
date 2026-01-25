package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func insertTestLibrary(t *testing.T, db *bun.DB, name string) *models.Library { //nolint:unparam // name is parameterized for flexibility
	t.Helper()
	now := time.Now()
	library := &models.Library{
		Name:                     name,
		CreatedAt:                now,
		UpdatedAt:                now,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(context.Background())
	require.NoError(t, err)
	return library
}

func TestService_IsLibraryCustomized(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	library := insertTestLibrary(t, db, "Test Library")

	// Not customized by default
	customized, err := svc.IsLibraryCustomized(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)

	// Mark as customized
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{})
	require.NoError(t, err)

	customized, err = svc.IsLibraryCustomized(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	assert.True(t, customized)
}

func TestService_GetLibraryOrder(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup: install plugins first
	plugin := &models.Plugin{Scope: "test", ID: "enricher", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	plugin2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Test Enricher 2", Version: "1.0.0", Enabled: true}
	_, err = db.NewInsert().Model(plugin2).Exec(ctx)
	require.NoError(t, err)

	// Set library order with two plugins
	entries := []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher2", Enabled: true},
		{Scope: "test", PluginID: "enricher", Enabled: false},
	}
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", entries)
	require.NoError(t, err)

	// Retrieve - should be ordered by position
	order, err := svc.GetLibraryOrder(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	require.Len(t, order, 2)
	assert.Equal(t, "enricher2", order[0].PluginID)
	assert.True(t, order[0].Enabled)
	assert.Equal(t, 0, order[0].Position)
	assert.Equal(t, "enricher", order[1].PluginID)
	assert.False(t, order[1].Enabled)
	assert.Equal(t, 1, order[1].Position)
}

func TestService_ResetLibraryOrder(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher", Enabled: true},
	})
	require.NoError(t, err)

	// Reset
	err = svc.ResetLibraryOrder(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)

	customized, err := svc.IsLibraryCustomized(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)

	order, err := svc.GetLibraryOrder(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	assert.Empty(t, order)
}

func TestService_ResetAllLibraryOrders(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher", Enabled: true},
	})
	require.NoError(t, err)
	err = svc.SetLibraryOrder(ctx, library.ID, "fileParser", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher", Enabled: true},
	})
	require.NoError(t, err)

	// Reset all
	err = svc.ResetAllLibraryOrders(ctx, library.ID)
	require.NoError(t, err)

	customized, _ := svc.IsLibraryCustomized(ctx, library.ID, "metadataEnricher")
	assert.False(t, customized)
	customized, _ = svc.IsLibraryCustomized(ctx, library.ID, "fileParser")
	assert.False(t, customized)
}
