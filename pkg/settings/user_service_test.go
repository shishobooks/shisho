package settings

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserSettings_ReturnsEpubDefaults(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "alice")
	svc := NewService(db)

	settings, err := svc.GetUserSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, settings.EpubFontSize)
	assert.Equal(t, models.EpubThemeLight, settings.EpubTheme)
	assert.Equal(t, models.EpubFlowPaginated, settings.EpubFlow)
}

func TestUpdateUserSettings_PersistsEpubFields(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "bob")
	svc := NewService(db)

	preload := 5
	fitMode := "original"
	fontSize := 140
	theme := models.EpubThemeSepia
	flow := models.EpubFlowScrolled

	updated, err := svc.UpdateUserSettings(
		context.Background(),
		user.ID,
		UserSettingsUpdate{
			PreloadCount: &preload,
			FitMode:      &fitMode,
			EpubFontSize: &fontSize,
			EpubTheme:    &theme,
			EpubFlow:     &flow,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 140, updated.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, updated.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, updated.EpubFlow)

	// Re-read to confirm persistence
	reloaded, err := svc.GetUserSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 140, reloaded.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, reloaded.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, reloaded.EpubFlow)
}

// TestUpdateUserSettings_PartialUpdateDoesNotClobber verifies the core
// reason UserSettingsUpdate uses pointer fields: a client that only
// changes one setting should leave others alone, not overwrite them with
// defaults.
func TestUpdateUserSettings_PartialUpdateDoesNotClobber(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "carol")
	svc := NewService(db)

	// Seed all five fields to known non-default values.
	preload := 7
	fitMode := "original"
	fontSize := 130
	theme := models.EpubThemeDark
	flow := models.EpubFlowScrolled
	_, err := svc.UpdateUserSettings(context.Background(), user.ID, UserSettingsUpdate{
		PreloadCount: &preload,
		FitMode:      &fitMode,
		EpubFontSize: &fontSize,
		EpubTheme:    &theme,
		EpubFlow:     &flow,
	})
	require.NoError(t, err)

	// Now update just the theme; all other fields must keep their seeded value.
	newTheme := models.EpubThemeSepia
	updated, err := svc.UpdateUserSettings(context.Background(), user.ID, UserSettingsUpdate{
		EpubTheme: &newTheme,
	})
	require.NoError(t, err)
	assert.Equal(t, 7, updated.ViewerPreloadCount)
	assert.Equal(t, "original", updated.ViewerFitMode)
	assert.Equal(t, 130, updated.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, updated.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, updated.EpubFlow)
}
