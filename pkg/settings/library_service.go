package settings

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// GetLibrarySettings returns the (user, library) settings row, or nil
// when no row exists. The nil-return form (rather than a zero-valued
// struct) makes callers' "no preference" checks explicit.
func (svc *Service) GetLibrarySettings(ctx context.Context, userID, libraryID int) (*models.UserLibrarySettings, error) {
	row := &models.UserLibrarySettings{}
	err := svc.db.NewSelect().
		Model(row).
		Where("user_id = ? AND library_id = ?", userID, libraryID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return row, nil
}

// UpsertLibrarySort writes just the sort_spec column for (userID,
// libraryID). sortSpec may be nil to clear the saved default. Other
// columns on the row (when they exist in a future version) are left
// untouched by the ON CONFLICT update.
func (svc *Service) UpsertLibrarySort(ctx context.Context, userID, libraryID int, sortSpec *string) (*models.UserLibrarySettings, error) {
	now := time.Now()

	row := &models.UserLibrarySettings{
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userID,
		LibraryID: libraryID,
		SortSpec:  sortSpec,
	}

	_, err := svc.db.NewInsert().
		Model(row).
		On("CONFLICT (user_id, library_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("sort_spec = EXCLUDED.sort_spec").
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return row, nil
}
