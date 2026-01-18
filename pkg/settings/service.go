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

// UpdateViewerSettings updates viewer settings for a user, creating if not exists.
func (svc *Service) UpdateViewerSettings(ctx context.Context, userID int, preloadCount int, fitMode string) (*models.UserSettings, error) {
	now := time.Now()

	settings := &models.UserSettings{
		CreatedAt:          now,
		UpdatedAt:          now,
		UserID:             userID,
		ViewerPreloadCount: preloadCount,
		ViewerFitMode:      fitMode,
	}

	_, err := svc.db.NewInsert().
		Model(settings).
		On("CONFLICT (user_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("viewer_preload_count = EXCLUDED.viewer_preload_count").
		Set("viewer_fit_mode = EXCLUDED.viewer_fit_mode").
		Returning("*").
		Exec(ctx)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return settings, nil
}
