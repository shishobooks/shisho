package settings

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetViewerSettings_ReturnsEpubDefaults(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "alice")
	svc := NewService(db)

	settings, err := svc.GetViewerSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, settings.EpubFontSize)
	assert.Equal(t, models.EpubThemeLight, settings.EpubTheme)
	assert.Equal(t, models.EpubFlowPaginated, settings.EpubFlow)
}

func TestUpdateViewerSettings_PersistsEpubFields(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "bob")
	svc := NewService(db)

	updated, err := svc.UpdateViewerSettings(
		context.Background(),
		user.ID,
		5, "original",
		140, models.EpubThemeSepia, models.EpubFlowScrolled,
	)
	require.NoError(t, err)
	assert.Equal(t, 140, updated.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, updated.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, updated.EpubFlow)

	// Re-read to confirm persistence
	reloaded, err := svc.GetViewerSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 140, reloaded.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, reloaded.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, reloaded.EpubFlow)
}
