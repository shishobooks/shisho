package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// A user with no rows follows Shisho's built-in Delivery Rules, so
		// a following list is never copied. A customized list can't be
		// empty, so "no rows" never means "deliver nothing".
		_, err := db.Exec(`
			CREATE TABLE user_delivery_rules (
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				source_file_type TEXT NOT NULL,
				delivered_file_type TEXT NOT NULL,
				position INTEGER NOT NULL,
				PRIMARY KEY (user_id, source_file_type)
			)
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS user_delivery_rules")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
