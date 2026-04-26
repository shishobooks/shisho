package appsettings

import (
	"context"
	"database/sql"
	"encoding/json"

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

// GetJSON loads the JSON-encoded value for key into out. Returns (false, nil) if no row exists.
func (svc *Service) GetJSON(ctx context.Context, key string, out interface{}) (bool, error) {
	row := &models.AppSetting{}
	err := svc.db.NewSelect().Model(row).Where("key = ?", key).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	if err := json.Unmarshal([]byte(row.Value), out); err != nil {
		return false, errors.WithStack(err)
	}
	return true, nil
}

// SetJSON stores the JSON-encoded value at key. Upserts on conflict.
func (svc *Service) SetJSON(ctx context.Context, key string, value interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return errors.WithStack(err)
	}
	row := &models.AppSetting{Key: key, Value: string(encoded)}
	_, err = svc.db.NewInsert().
		Model(row).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)
	return errors.WithStack(err)
}
