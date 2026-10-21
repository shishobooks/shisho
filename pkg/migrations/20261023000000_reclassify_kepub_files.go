package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Retype .kepub.epub files as KePub. Before KePub was a File Type, a scan
// typed one by its last extension, as an EPUB. Download cache entries need
// no cleanup: fingerprints carry the File's own File Type, so these Files
// miss what was cached for them as EPUBs.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `UPDATE files SET file_type = 'kepub' WHERE file_type = 'epub' AND lower(filepath) LIKE '%.kepub.epub'`)
		return errors.Wrap(err, "failed to reclassify KePub files")
	}
	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `UPDATE files SET file_type = 'epub' WHERE file_type = 'kepub'`)
		return errors.Wrap(err, "failed to restore KePub files to EPUB")
	}
	Migrations.MustRegister(up, down)
}
