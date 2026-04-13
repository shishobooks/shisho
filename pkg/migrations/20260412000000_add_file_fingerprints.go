package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE file_fingerprints (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				algorithm TEXT NOT NULL,
				value TEXT NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// One fingerprint per algorithm per file.
		_, err = db.Exec(`
			CREATE UNIQUE INDEX ux_file_fingerprints_file_algorithm
				ON file_fingerprints (file_id, algorithm)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Fast lookup for "find file by hash" (move detection).
		_, err = db.Exec(`
			CREATE INDEX ix_file_fingerprints_algorithm_value
				ON file_fingerprints (algorithm, value)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS file_fingerprints")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
