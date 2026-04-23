package settings

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// GetViewerSettings retrieves viewer settings for a user, returning defaults if none exist.
func (svc *Service) GetViewerSettings(ctx context.Context, userID int) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	err := svc.db.NewSelect().
		Model(settings).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return defaults if no settings exist
			defaults := models.DefaultUserSettings()
			defaults.UserID = userID
			return defaults, nil
		}
		return nil, errors.WithStack(err)
	}

	return settings, nil
}

// ViewerSettingsUpdate describes a partial update to viewer settings. Any
// field left nil is left untouched; fields set to a non-nil value are
// persisted. This lets clients change one setting without having to read
// and echo every other setting first (which races on concurrent updates).
type ViewerSettingsUpdate struct {
	PreloadCount *int
	FitMode      *string
	EpubFontSize *int
	EpubTheme    *string
	EpubFlow     *string
}

// UpdateViewerSettings applies a partial update to a user's viewer settings,
// creating a row if none exists. It reads the current row (or defaults),
// overlays any non-nil fields from the update, and writes the result back.
func (svc *Service) UpdateViewerSettings(
	ctx context.Context,
	userID int,
	update ViewerSettingsUpdate,
) (*models.UserSettings, error) {
	// Load the current row so missing fields in the partial update keep
	// their current values. GetViewerSettings returns defaults on miss.
	current, err := svc.GetViewerSettings(ctx, userID)
	if err != nil {
		return nil, err
	}

	if update.PreloadCount != nil {
		current.ViewerPreloadCount = *update.PreloadCount
	}
	if update.FitMode != nil {
		current.ViewerFitMode = *update.FitMode
	}
	if update.EpubFontSize != nil {
		current.EpubFontSize = *update.EpubFontSize
	}
	if update.EpubTheme != nil {
		current.EpubTheme = *update.EpubTheme
	}
	if update.EpubFlow != nil {
		current.EpubFlow = *update.EpubFlow
	}

	now := time.Now()
	current.UserID = userID
	current.UpdatedAt = now
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}

	_, err = svc.db.NewInsert().
		Model(current).
		On("CONFLICT (user_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("viewer_preload_count = EXCLUDED.viewer_preload_count").
		Set("viewer_fit_mode = EXCLUDED.viewer_fit_mode").
		Set("viewer_epub_font_size = EXCLUDED.viewer_epub_font_size").
		Set("viewer_epub_theme = EXCLUDED.viewer_epub_theme").
		Set("viewer_epub_flow = EXCLUDED.viewer_epub_flow").
		Returning("*").
		Exec(ctx)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return current, nil
}
