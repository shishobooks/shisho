package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FitMode = typeof FitModeHeight | typeof FitModeOriginal;
	FitModeHeight   = "fit-height"
	FitModeOriginal = "original"
)

type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings,alias:us" tstype:"-"`

	ID                 int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID             int       `bun:",notnull,unique" json:"user_id"`
	ViewerPreloadCount int       `bun:",notnull,default:3" json:"viewer_preload_count"`
	ViewerFitMode      string    `bun:",notnull,default:'fit-height'" json:"viewer_fit_mode" tstype:"FitMode"`
}

// DefaultUserSettings returns a UserSettings with default values.
func DefaultUserSettings() *UserSettings {
	return &UserSettings{
		ViewerPreloadCount: 3,
		ViewerFitMode:      FitModeHeight,
	}
}
