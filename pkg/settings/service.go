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

// GetUserSettings retrieves user settings for a user, returning defaults if none exist.
func (svc *Service) GetUserSettings(ctx context.Context, userID int) (*models.UserSettings, error) {
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

// UserSettingsUpdate describes a partial update to user settings. Any
// field left nil is left untouched; fields set to a non-nil value are
// persisted. This lets clients change one setting without having to read
// and echo every other setting first.
type UserSettingsUpdate struct {
	PreloadCount *int
	FitMode      *string
	EpubFontSize *int
	EpubTheme    *string
	EpubFlow     *string
}

// UpdateUserSettings applies a partial update to a user's settings,
// creating a row if none exists. Runs in a transaction so that the
// read-current-then-write-merged sequence is atomic — otherwise two
// concurrent updates from the same user (rapid toggles in one tab, or two
// tabs racing) could lose one of the writes.
func (svc *Service) UpdateUserSettings(
	ctx context.Context,
	userID int,
	update UserSettingsUpdate,
) (*models.UserSettings, error) {
	var result *models.UserSettings
	err := svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Load the current row inside the tx so a concurrent update can't
		// slip in between the read and the write. GetUserSettings doesn't
		// take a tx, so inline the select here.
		current := &models.UserSettings{}
		err := tx.NewSelect().
			Model(current).
			Where("user_id = ?", userID).
			Scan(ctx)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return errors.WithStack(err)
			}
			// No row yet — start from defaults.
			current = models.DefaultUserSettings()
			current.UserID = userID
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

		_, err = tx.NewInsert().
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
			return errors.WithStack(err)
		}
		result = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
