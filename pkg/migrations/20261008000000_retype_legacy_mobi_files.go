package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Retype .azw and .prc files as MOBI. Before MOBI was a built-in type, a
// plugin parser or supplement discovery typed a file by its raw extension, so
// these rows say "azw" or "prc". Plugins can no longer claim those
// extensions, and only type "mobi" reaches the built-in parser, the ebook
// cover category, and supplement promotion.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `UPDATE files SET file_type = 'mobi' WHERE file_type IN ('azw', 'prc')`)
		return errors.Wrap(err, "failed to retype legacy MOBI files")
	}
	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, ext := range []string{"azw", "prc"} {
				if _, err := tx.ExecContext(ctx,
					`UPDATE files SET file_type = ? WHERE file_type = 'mobi' AND lower(filepath) LIKE ?`,
					ext, "%."+ext,
				); err != nil {
					return errors.Wrapf(err, "failed to restore file type %s", ext)
				}
			}
			return nil
		})
	}
	Migrations.MustRegister(up, down)
}
