package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// The File Type of the copy a position's locator points into,
			// when it isn't the File's own: a KePub's span markup gives its
			// XPointers different paths from its EPUB's.
			`ALTER TABLE reading_positions ADD COLUMN locator_file_type TEXT`,

			// KOReader's latest pull of each document from each Device. The
			// pushes of the same document from the same Device that follow
			// make an estimated Reading Session from it, session_id.
			`CREATE TABLE koreader_pulls (
				device_id TEXT REFERENCES devices (id) ON DELETE CASCADE NOT NULL,
				document TEXT NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				pulled_at TIMESTAMP NOT NULL,
				percentage REAL,
				paired_at TIMESTAMP,
				session_id INTEGER REFERENCES reading_sessions (id) ON DELETE SET NULL,
				PRIMARY KEY (device_id, document)
			)`,
			`CREATE INDEX ix_koreader_pulls_file_id ON koreader_pulls (file_id)`,
			`CREATE INDEX ix_koreader_pulls_session_id ON koreader_pulls (session_id)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`DROP TABLE IF EXISTS koreader_pulls`,
			`ALTER TABLE reading_positions DROP COLUMN locator_file_type`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
