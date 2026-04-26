package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Hard-delete any soft-deleted series. CASCADE on book_series.series_id
		// (set in 20260406100000_add_fk_cascades.go) cleans up join rows.
		// Done before the schema change so the unconditional unique index in
		// the final step can't collide on a name shared between a soft-deleted
		// and a live row in the same library.
		if _, err := db.Exec(`DELETE FROM series WHERE deleted_at IS NOT NULL`); err != nil {
			return errors.WithStack(err)
		}
		// Defensive: purge any series_fts rows whose series_id is gone.
		// Handlers already call DeleteFromSeriesIndex on soft-delete today,
		// so this should be a no-op in practice.
		if _, err := db.Exec(`DELETE FROM series_fts WHERE series_id NOT IN (SELECT id FROM series)`); err != nil {
			return errors.WithStack(err)
		}
		// The partial unique index references deleted_at; drop before the column.
		if _, err := db.Exec(`DROP INDEX ux_series_name_library_id`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`ALTER TABLE series DROP COLUMN deleted_at`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id)`); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Lossy: does not restore deleted rows. Matches the precedent set by
		// 20260423000000_drop_libraries_deleted_at.go.
		if _, err := db.Exec(`DROP INDEX ux_series_name_library_id`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`ALTER TABLE series ADD COLUMN deleted_at TIMESTAMPTZ`); err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.Exec(`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
