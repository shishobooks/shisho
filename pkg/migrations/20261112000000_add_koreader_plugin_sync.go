package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// When a KOReader Device last uploaded through the KOReader
			// plugin. From then on its progress sync pushes are ignored.
			`ALTER TABLE devices ADD COLUMN plugin_last_synced_at TIMESTAMP`,

			// Rows of KOReader's statistics a Device sent through the plugin,
			// unique on KOReader's own key (ADR 0016). file_id is the File
			// the document hash named when the row arrived, or NULL for a
			// hash Shisho didn't recognize. A grouped row was made part of
			// reading_session_id; one grouped whose session is gone had its
			// session deleted, so it's never grouped again.
			`CREATE TABLE koreader_page_rows (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMP NOT NULL,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				device_id TEXT REFERENCES devices (id) ON DELETE CASCADE NOT NULL,
				document TEXT NOT NULL,
				page INTEGER NOT NULL,
				total_pages INTEGER NOT NULL,
				-- start_time is the device's own Unix time, part of the key;
				-- started_at is the instant after the clock correction.
				start_time INTEGER NOT NULL,
				started_at TIMESTAMP NOT NULL,
				duration INTEGER NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
				grouped BOOLEAN NOT NULL DEFAULT FALSE,
				reading_session_id INTEGER REFERENCES reading_sessions (id) ON DELETE SET NULL
			)`,
			`CREATE UNIQUE INDEX ux_koreader_page_rows_key ON koreader_page_rows (device_id, document, page, start_time)`,
			`CREATE INDEX ix_koreader_page_rows_user_id ON koreader_page_rows (user_id)`,
			`CREATE INDEX ix_koreader_page_rows_file_id ON koreader_page_rows (file_id)`,
			`CREATE INDEX ix_koreader_page_rows_reading_session_id ON koreader_page_rows (reading_session_id)`,

			// The journal events each Device sent through the plugin, by
			// their ids, so a repeat is recognized and ignored.
			`CREATE TABLE koreader_plugin_events (
				device_id TEXT REFERENCES devices (id) ON DELETE CASCADE NOT NULL,
				event_id TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				PRIMARY KEY (device_id, event_id)
			)`,
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
			`DROP TABLE IF EXISTS koreader_plugin_events`,
			`DROP TABLE IF EXISTS koreader_page_rows`,
			`ALTER TABLE devices DROP COLUMN plugin_last_synced_at`,
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
