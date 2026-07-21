package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE book_series ADD COLUMN series_number_end REAL`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE book_series DROP COLUMN series_number_end`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
