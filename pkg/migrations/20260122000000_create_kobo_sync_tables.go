package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create kobo_sync_points table
		_, err := db.Exec(`
			CREATE TABLE kobo_sync_points (
				id TEXT PRIMARY KEY,
				api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
				created_at DATETIME NOT NULL,
				completed_at DATETIME
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_kobo_sync_points_api_key ON kobo_sync_points(api_key_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create kobo_sync_point_books table
		_, err = db.Exec(`
			CREATE TABLE kobo_sync_point_books (
				id TEXT PRIMARY KEY,
				sync_point_id TEXT NOT NULL REFERENCES kobo_sync_points(id) ON DELETE CASCADE,
				file_id INTEGER NOT NULL,
				file_hash TEXT NOT NULL,
				file_size INTEGER NOT NULL,
				metadata_hash TEXT NOT NULL,
				synced BOOLEAN NOT NULL DEFAULT FALSE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_sync_point_books_sync_point ON kobo_sync_point_books(sync_point_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_sync_point_books_file ON kobo_sync_point_books(file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP TABLE IF EXISTS kobo_sync_point_books`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS kobo_sync_points`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
