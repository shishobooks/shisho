package settings

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserSettings_ReturnsReflowableDefaults(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	svc := NewService(db)

	settings, err := svc.GetUserSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, settings.ReflowableFontSize)
	assert.Equal(t, models.ReflowableThemeLight, settings.ReflowableTheme)
	assert.Equal(t, models.ReflowableFlowPaginated, settings.ReflowableFlow)
}

func TestUpdateUserSettings_PersistsReflowableFields(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "bob")
	svc := NewService(db)

	preload := 5
	fitMode := "fit-width"
	fontSize := 140
	theme := models.ReflowableThemeSepia
	flow := models.ReflowableFlowScrolled

	updated, err := svc.UpdateUserSettings(
		context.Background(),
		user.ID,
		UserSettingsUpdate{
			PreloadCount:       &preload,
			FitMode:            &fitMode,
			ReflowableFontSize: &fontSize,
			ReflowableTheme:    &theme,
			ReflowableFlow:     &flow,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 140, updated.ReflowableFontSize)
	assert.Equal(t, models.ReflowableThemeSepia, updated.ReflowableTheme)
	assert.Equal(t, models.ReflowableFlowScrolled, updated.ReflowableFlow)

	// Re-read to confirm persistence
	reloaded, err := svc.GetUserSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 140, reloaded.ReflowableFontSize)
	assert.Equal(t, models.ReflowableThemeSepia, reloaded.ReflowableTheme)
	assert.Equal(t, models.ReflowableFlowScrolled, reloaded.ReflowableFlow)
}

func TestGetUserSettings_DefaultsPlaybackSpeedToNormal(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "frank")
	svc := NewService(db)

	settings, err := svc.GetUserSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.InDelta(t, 1.0, settings.PlaybackSpeed, 0)
}

func TestUpdateUserSettings_PersistsPlaybackSpeed(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "grace")
	svc := NewService(db)

	speed := 1.5
	updated, err := svc.UpdateUserSettings(
		context.Background(),
		user.ID,
		UserSettingsUpdate{PlaybackSpeed: &speed},
	)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, updated.PlaybackSpeed, 0)

	// Re-read to confirm persistence
	reloaded, err := svc.GetUserSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, reloaded.PlaybackSpeed, 0)
}

// TestUpdateUserSettings_PartialUpdateDoesNotClobber verifies the core
// reason UserSettingsUpdate uses pointer fields: a client that only
// changes one setting should leave others alone, not overwrite them with
// defaults.
func TestUpdateUserSettings_PartialUpdateDoesNotClobber(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "carol")
	svc := NewService(db)

	// Seed all five fields to known non-default values.
	preload := 7
	fitMode := "fit-width"
	fontSize := 130
	theme := models.ReflowableThemeDark
	flow := models.ReflowableFlowScrolled
	_, err := svc.UpdateUserSettings(context.Background(), user.ID, UserSettingsUpdate{
		PreloadCount:       &preload,
		FitMode:            &fitMode,
		ReflowableFontSize: &fontSize,
		ReflowableTheme:    &theme,
		ReflowableFlow:     &flow,
	})
	require.NoError(t, err)

	// Now update just the theme; all other fields must keep their seeded value.
	newTheme := models.ReflowableThemeSepia
	updated, err := svc.UpdateUserSettings(context.Background(), user.ID, UserSettingsUpdate{
		ReflowableTheme: &newTheme,
	})
	require.NoError(t, err)
	assert.Equal(t, 7, updated.ViewerPreloadCount)
	assert.Equal(t, "fit-width", updated.ViewerFitMode)
	assert.Equal(t, 130, updated.ReflowableFontSize)
	assert.Equal(t, models.ReflowableThemeSepia, updated.ReflowableTheme)
	assert.Equal(t, models.ReflowableFlowScrolled, updated.ReflowableFlow)
}
