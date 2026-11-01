package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// A position is held in exactly one form: a text location (EPUB CFI
		// or KOReader XPointer), a 0-based page, seconds into the audio, or a
		// Kobo span. Every source writes the one it reads, so the check keeps
		// the forms exclusive.
		const locator = `
				cfi TEXT,
				xpointer TEXT,
				page INTEGER,
				seconds REAL,
				kobo_span TEXT,
				percentage REAL NOT NULL CHECK (percentage BETWEEN 0 AND 1),
				source TEXT NOT NULL,
				occurred_at TIMESTAMP NOT NULL`
		const oneForm = `
				CHECK ((cfi IS NOT NULL) + (xpointer IS NOT NULL) + (page IS NOT NULL) + (seconds IS NOT NULL) + (kobo_span IS NOT NULL) = 1)`

		// One Reading Position per user and File: the latest update wins, by
		// the instant it happened, not the order it arrived.
		statements := []string{
			`CREATE TABLE reading_positions (
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,` + locator + `,
				PRIMARY KEY (user_id, file_id),` + oneForm + `
			)`,
			`CREATE INDEX ix_reading_positions_file_id ON reading_positions (file_id)`,

			// Every accepted update, kept for stats. It belongs to the
			// Read-through underway when it arrived and goes with it, and it
			// outlives its File so removing a File keeps the history.
			// auto_finished marks the update that finished its Read-through,
			// which the next update for the File may undo.
			`CREATE TABLE reading_position_updates (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
				read_through_id INTEGER REFERENCES read_throughs (id) ON DELETE CASCADE,` + locator + `,
				at_end BOOLEAN NOT NULL DEFAULT FALSE,
				auto_finished BOOLEAN NOT NULL DEFAULT FALSE,` + oneForm + `
			)`,
			`CREATE INDEX ix_reading_position_updates_user_id_file_id ON reading_position_updates (user_id, file_id)`,
			`CREATE INDEX ix_reading_position_updates_file_id ON reading_position_updates (file_id)`,
			`CREATE INDEX ix_reading_position_updates_read_through_id ON reading_position_updates (read_through_id)`,

			// Reading Sessions, owned like the update log. measured is false
			// for a session a source estimated rather than timed.
			`CREATE TABLE reading_sessions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
				read_through_id INTEGER REFERENCES read_throughs (id) ON DELETE CASCADE,
				source TEXT NOT NULL,
				started_at TIMESTAMP NOT NULL,
				ended_at TIMESTAMP NOT NULL,
				start_percentage REAL NOT NULL CHECK (start_percentage BETWEEN 0 AND 1),
				end_percentage REAL NOT NULL CHECK (end_percentage BETWEEN 0 AND 1),
				measured BOOLEAN NOT NULL,
				CHECK (ended_at >= started_at)
			)`,
			`CREATE INDEX ix_reading_sessions_user_id ON reading_sessions (user_id)`,
			`CREATE INDEX ix_reading_sessions_file_id ON reading_sessions (file_id)`,
			`CREATE INDEX ix_reading_sessions_read_through_id ON reading_sessions (read_through_id)`,

			// The IANA zone that dates a user's automatic Read-through days.
			// NULL until the browser fills it in.
			`ALTER TABLE user_settings ADD COLUMN time_zone TEXT`,

			// The To Read position a Read-through took its Book from when it
			// started, so deleting it while underway puts the Book back.
			`ALTER TABLE read_throughs ADD COLUMN left_to_read_position INTEGER`,
		}
		for _, s := range statements {
			if _, err := db.Exec(s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		statements := []string{
			"ALTER TABLE read_throughs DROP COLUMN left_to_read_position",
			"ALTER TABLE user_settings DROP COLUMN time_zone",
			"DROP TABLE IF EXISTS reading_sessions",
			"DROP TABLE IF EXISTS reading_position_updates",
			"DROP TABLE IF EXISTS reading_positions",
		}
		for _, s := range statements {
			if _, err := db.Exec(s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
