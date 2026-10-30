package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// The flow one user chose for a Book's Page-based reader. NULL means
		// they never chose, and the reader picks one from the page shapes.
		_, err := db.Exec(`
			ALTER TABLE user_books ADD COLUMN page_flow TEXT CHECK (page_flow IN ('paginated', 'scrolled'))
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE user_books DROP COLUMN page_flow")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
