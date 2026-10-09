package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// The font size, theme, and flow reader settings apply to every Reflowable
// format (EPUB, MOBI, AZW3), so their columns lose the EPUB prefix. RENAME
// COLUMN keeps the saved values.
func init() {
	renames := [][2]string{
		{"viewer_epub_font_size", "viewer_reflowable_font_size"},
		{"viewer_epub_theme", "viewer_reflowable_theme"},
		{"viewer_epub_flow", "viewer_reflowable_flow"},
	}

	rename := func(db *bun.DB, from, to string) error {
		_, err := db.Exec("ALTER TABLE user_settings RENAME COLUMN " + from + " TO " + to)
		return errors.WithStack(err)
	}

	up := func(_ context.Context, db *bun.DB) error {
		for _, r := range renames {
			if err := rename(db, r[0], r[1]); err != nil {
				return err
			}
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		for _, r := range renames {
			if err := rename(db, r[1], r[0]); err != nil {
				return err
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
