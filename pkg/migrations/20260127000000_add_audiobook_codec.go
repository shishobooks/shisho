package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE files ADD COLUMN audiobook_codec TEXT")
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE files DROP COLUMN audiobook_codec")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
