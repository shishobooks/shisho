package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE plugins ADD COLUMN confidence_threshold REAL`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE plugins DROP COLUMN confidence_threshold`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
