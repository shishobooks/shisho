package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_epub_font_size INTEGER NOT NULL DEFAULT 100
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_epub_theme TEXT NOT NULL DEFAULT 'light'
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_epub_flow TEXT NOT NULL DEFAULT 'paginated'
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		for _, col := range []string{"viewer_epub_font_size", "viewer_epub_theme", "viewer_epub_flow"} {
			if _, err := db.Exec("ALTER TABLE user_settings DROP COLUMN " + col); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
