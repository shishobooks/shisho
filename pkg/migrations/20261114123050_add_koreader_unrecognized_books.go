package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// The columns of reading_positions and reading_position_updates as every
// migration up to this one left them, for rebuilding them with a new check
// on their locator.
const (
	readingPositionsColumns = `
					user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
					file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,` + locatorColumns + `,
					source_device_id TEXT REFERENCES devices (id) ON DELETE SET NULL,
					reported_device TEXT,
					reported_device_id TEXT,
					locator_file_type TEXT,
					chapter_percentage REAL CHECK (chapter_percentage BETWEEN 0 AND 1),
					PRIMARY KEY (user_id, file_id),`
	readingPositionUpdatesColumns = `
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
					file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
					read_through_id INTEGER REFERENCES read_throughs (id) ON DELETE CASCADE,` + locatorColumns + `,
					at_end BOOLEAN NOT NULL DEFAULT FALSE,
					auto_finished BOOLEAN NOT NULL DEFAULT FALSE,
					opened BOOLEAN NOT NULL DEFAULT FALSE,
					source_device_id TEXT REFERENCES devices (id) ON DELETE SET NULL,`
	atMostOneLocatorForm = `
					CHECK ((cfi IS NOT NULL) + (xpointer IS NOT NULL) + (page IS NOT NULL) + (seconds IS NOT NULL) + (kobo_span IS NOT NULL) <= 1)`
)

// rebuildPositionTables rebuilds reading_positions and
// reading_position_updates with check as the rule on their locator.
func rebuildPositionTables(ctx context.Context, db *bun.DB, check string) error {
	return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		tables := map[string]string{
			"reading_positions":        `CREATE TABLE reading_positions_new (` + readingPositionsColumns + check + `)`,
			"reading_position_updates": `CREATE TABLE reading_position_updates_new (` + readingPositionUpdatesColumns + check + `)`,
		}
		names := []string{"reading_positions", "reading_position_updates"}
		for _, table := range names {
			if err := rebuildTableInTx(ctx, tx, table, tables[table]); err != nil {
				return err
			}
		}
		return checkForeignKeys(ctx, tx, names)
	})
}

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// A Reading Position may have no locator, only a percentage: where a
		// copy a user linked was (ADR 0017), whose locators may not line up
		// with the File's.
		if err := rebuildPositionTables(ctx, db, atMostOneLocatorForm); err != nil {
			return err
		}
		statements := []string{
			// The copies each user linked to a File, of bytes Shisho never
			// produced (ADR 0017). A link is the user's own: another user's
			// identical copy stays theirs to link. Its copy syncs by
			// percentage only.
			`CREATE TABLE koreader_document_links (
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				hash TEXT NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				delivered_file_type TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				PRIMARY KEY (user_id, hash)
			)`,
			`CREATE INDEX ix_koreader_document_links_file_id ON koreader_document_links (file_id)`,

			// A page row held for a hash Shisho didn't recognize that came
			// from the history KOReader recorded before signing in, which
			// linking groups by history's rules.
			`ALTER TABLE koreader_page_rows ADD COLUMN history BOOLEAN NOT NULL DEFAULT FALSE`,

			// Copies a KOReader Device holds data for that no server it's
			// signed in to recognized (ADR 0017), with what KOReader knows
			// about each. A dismissed one stays, so the Device's data for it
			// keeps being dropped. A linked one names the File it was linked
			// to.
			`CREATE TABLE koreader_unrecognized_books (
				device_id TEXT REFERENCES devices (id) ON DELETE CASCADE NOT NULL,
				document TEXT NOT NULL,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				title TEXT,
				authors TEXT NOT NULL DEFAULT '[]',
				identifiers TEXT NOT NULL DEFAULT '[]',
				language TEXT,
				file_type TEXT,
				reading_seconds INTEGER NOT NULL DEFAULT 0,
				first_read_at TIMESTAMP,
				last_read_at TIMESTAMP,
				dismissed_at TIMESTAMP,
				linked_at TIMESTAMP,
				linked_file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
				PRIMARY KEY (device_id, document)
			)`,
			`CREATE INDEX ix_koreader_unrecognized_books_user_id_document ON koreader_unrecognized_books (user_id, document)`,
			`CREATE INDEX ix_koreader_unrecognized_books_linked_file_id ON koreader_unrecognized_books (linked_file_id)`,

			// Journal events held for an unrecognized copy until it's linked,
			// with their times already corrected for the device's clock:
			// occurred_at for a position, and changed_day for a status or
			// Rating change.
			`CREATE TABLE koreader_held_events (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMP NOT NULL,
				device_id TEXT REFERENCES devices (id) ON DELETE CASCADE NOT NULL,
				document TEXT NOT NULL,
				event_id TEXT NOT NULL,
				type TEXT NOT NULL,
				history BOOLEAN NOT NULL DEFAULT FALSE,
				occurred_at TIMESTAMP,
				changed_day TEXT,
				percentage REAL,
				status TEXT,
				previous_status TEXT,
				rating INTEGER NOT NULL DEFAULT 0,
				previous_rating INTEGER NOT NULL DEFAULT 0
			)`,
			`CREATE INDEX ix_koreader_held_events_device_id_document ON koreader_held_events (device_id, document)`,
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
			`DROP TABLE IF EXISTS koreader_held_events`,
			`DROP TABLE IF EXISTS koreader_unrecognized_books`,
			`DROP TABLE IF EXISTS koreader_document_links`,
			`ALTER TABLE koreader_page_rows DROP COLUMN history`,
			// Positions with no locator can't be kept by the old check.
			`DELETE FROM reading_positions WHERE cfi IS NULL AND xpointer IS NULL AND page IS NULL AND seconds IS NULL AND kobo_span IS NULL`,
			`DELETE FROM reading_position_updates WHERE cfi IS NULL AND xpointer IS NULL AND page IS NULL AND seconds IS NULL AND kobo_span IS NULL`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return rebuildPositionTables(ctx, db, oneLocatorForm)
	}

	Migrations.MustRegister(up, down)
}
