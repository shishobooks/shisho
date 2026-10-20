package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// Users' default Delivery Rules replace the per-library download
	// setting (ADR 0011).
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE libraries DROP COLUMN download_format_preference")
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE libraries ADD COLUMN download_format_preference TEXT NOT NULL DEFAULT 'original'")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
