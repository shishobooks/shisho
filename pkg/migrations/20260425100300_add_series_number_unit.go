package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE book_series ADD COLUMN series_number_unit TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE book_series DROP COLUMN series_number_unit`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
