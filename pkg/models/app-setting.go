package models

import (
	"time"

	"github.com/uptrace/bun"
)

type AppSetting struct {
	bun.BaseModel `bun:"table:app_settings,alias:as" tstype:"-"`

	Key       string    `bun:",pk,nullzero" json:"key"`
	Value     string    `bun:",notnull" json:"value"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
