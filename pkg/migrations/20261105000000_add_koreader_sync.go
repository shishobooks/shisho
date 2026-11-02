package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// KOReader names a document by a hash of its bytes, so this maps
			// each hash Shisho produced to the File and the File Type it was
			// delivered as (ADR 0014). It's never pruned while the File
			// exists: a copy on a device keeps its hash.
			`CREATE TABLE koreader_document_hashes (
				hash TEXT NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				delivered_file_type TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (hash, file_id, delivered_file_type)
			)`,
			`CREATE INDEX ix_koreader_document_hashes_file_id ON koreader_document_hashes (file_id)`,

			// The Device that reported a position, when a Device did, so it
			// gets exactly that position back.
			`ALTER TABLE reading_positions ADD COLUMN source_device_id TEXT REFERENCES devices (id) ON DELETE SET NULL`,
			`CREATE INDEX ix_reading_positions_source_device_id ON reading_positions (source_device_id)`,
			// How the source named itself, which KOReader's protocol echoes
			// back so a device can tell its own position.
			`ALTER TABLE reading_positions ADD COLUMN reported_device TEXT`,
			`ALTER TABLE reading_positions ADD COLUMN reported_device_id TEXT`,
			`ALTER TABLE reading_position_updates ADD COLUMN source_device_id TEXT REFERENCES devices (id) ON DELETE SET NULL`,
			`CREATE INDEX ix_reading_position_updates_source_device_id ON reading_position_updates (source_device_id)`,

			// Backfills the document hashes of existing Files.
			`INSERT INTO jobs (type, status, data, progress, library_id, created_at, updated_at)
				SELECT 'hash_generation', 'pending', json_object('library_id', id), 0, id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
				FROM libraries`,
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
			`DELETE FROM file_fingerprints WHERE algorithm = 'koreader_partial_md5'`,
			`DROP TABLE IF EXISTS koreader_document_hashes`,
			`DROP INDEX IF EXISTS ix_reading_position_updates_source_device_id`,
			`DROP INDEX IF EXISTS ix_reading_positions_source_device_id`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		// SQLite refuses to drop a column a foreign key names, so a rebuild
		// removes the reference first.
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			tables := map[string]string{
				"reading_positions": `CREATE TABLE reading_positions_new (
					user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
					file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,` + locatorColumns + `,
					source_device_id TEXT,
					reported_device TEXT,
					reported_device_id TEXT,
					PRIMARY KEY (user_id, file_id),` + oneLocatorForm + `
				)`,
				"reading_position_updates": `CREATE TABLE reading_position_updates_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
					file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
					read_through_id INTEGER REFERENCES read_throughs (id) ON DELETE CASCADE,` + locatorColumns + `,
					at_end BOOLEAN NOT NULL DEFAULT FALSE,
					auto_finished BOOLEAN NOT NULL DEFAULT FALSE,
					opened BOOLEAN NOT NULL DEFAULT FALSE,
					source_device_id TEXT,` + oneLocatorForm + `
				)`,
			}
			names := []string{"reading_positions", "reading_position_updates"}
			for _, table := range names {
				if err := rebuildTableInTx(ctx, tx, table, tables[table]); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" DROP COLUMN source_device_id"); err != nil {
					return errors.WithStack(err)
				}
				if table != "reading_positions" {
					continue
				}
				for _, column := range []string{"reported_device", "reported_device_id"} {
					if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" DROP COLUMN "+column); err != nil {
						return errors.WithStack(err)
					}
				}
			}
			return checkForeignKeys(ctx, tx, names)
		})
	}

	Migrations.MustRegister(up, down)
}

// The columns reading_positions and reading_position_updates share, as
// 20261101000000_create_reading_position_tables.go created them.
const (
	locatorColumns = `
					cfi TEXT,
					xpointer TEXT,
					page INTEGER,
					seconds REAL,
					kobo_span TEXT,
					percentage REAL NOT NULL CHECK (percentage BETWEEN 0 AND 1),
					source TEXT NOT NULL,
					occurred_at TIMESTAMP NOT NULL`
	oneLocatorForm = `
					CHECK ((cfi IS NOT NULL) + (xpointer IS NOT NULL) + (page IS NOT NULL) + (seconds IS NOT NULL) + (kobo_span IS NOT NULL) = 1)`
)
