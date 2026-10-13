package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Move the saved cover thumbnail limit under its cache id, the shape that
// holds a limit, and whether it is lifted, for every cache. Rollback keeps
// only the cover thumbnail limit, which is all the old shape can hold; a
// lifted limit becomes the old maximum of 1024 GiB.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `
			UPDATE app_settings
			SET value = json_object('cover_thumbnails', json_object(
				'max_size_gb', json_extract(value, '$.cover_thumbnail_max_size_gb'),
				'unlimited', json('false')))
			WHERE key = 'cache' AND json_extract(value, '$.cover_thumbnail_max_size_gb') IS NOT NULL`)
		return errors.Wrap(err, "failed to nest the cover thumbnail limit")
	}
	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `
			UPDATE app_settings
			SET value = CASE
				WHEN json_extract(value, '$.cover_thumbnails') IS NULL THEN '{}'
				WHEN json_extract(value, '$.cover_thumbnails.unlimited') THEN '{"cover_thumbnail_max_size_gb":1024}'
				ELSE json_object('cover_thumbnail_max_size_gb', json_extract(value, '$.cover_thumbnails.max_size_gb'))
			END
			WHERE key = 'cache'`)
		return errors.Wrap(err, "failed to restore the flat cover thumbnail limit")
	}
	Migrations.MustRegister(up, down)
}
