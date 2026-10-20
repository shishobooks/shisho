package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Kobo sync entries get ids of their own, so an entry can be handed to
// another File of the same book (ADR 0012). Every entry recorded so far went
// out under its File's Shisho id as a KePub, which is what Kobo sync always
// sent, so it keeps that id and RevisionId. An entry's File and book are set
// to NULL when deleted rather than taking the entry with them: the next sync
// hands the entry to another File of the book, or removes it.
//
// Sync point snapshots record the entry each File went out under, its
// delivered File Type, and its RevisionId.

const koboSyncEntriesWithIDs = `CREATE TABLE kobo_sync_entries_new (
	device_id TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
	file_id INTEGER REFERENCES files (id) ON DELETE SET NULL,
	revision_id TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	entry_id TEXT NOT NULL,
	book_id INTEGER REFERENCES books (id) ON DELETE SET NULL,
	delivered_file_type TEXT NOT NULL,
	PRIMARY KEY (device_id, entry_id),
	UNIQUE (device_id, file_id)
)`

// koboSyncEntriesByFile is the table before this migration, with the new
// columns still on it and book_id no longer a foreign key, so they can be
// dropped.
const koboSyncEntriesByFile = `CREATE TABLE kobo_sync_entries_new (
	device_id TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
	file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
	revision_id TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	entry_id TEXT NOT NULL,
	book_id INTEGER,
	delivered_file_type TEXT NOT NULL,
	PRIMARY KEY (device_id, file_id)
)`

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			for _, stmt := range []string{
				`ALTER TABLE kobo_sync_entries ADD COLUMN entry_id TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE kobo_sync_entries ADD COLUMN book_id INTEGER`,
				`ALTER TABLE kobo_sync_entries ADD COLUMN delivered_file_type TEXT NOT NULL DEFAULT ''`,
				`UPDATE kobo_sync_entries SET
					entry_id = 'shisho-' || file_id,
					delivered_file_type = 'kepub',
					book_id = (SELECT f.book_id FROM files f WHERE f.id = kobo_sync_entries.file_id)`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}
			if err := rebuildTableInTx(ctx, tx, "kobo_sync_entries", koboSyncEntriesWithIDs); err != nil {
				return err
			}
			for _, stmt := range []string{
				`CREATE INDEX ix_kobo_sync_entries_book_id ON kobo_sync_entries (book_id)`,
				`ALTER TABLE kobo_sync_point_books ADD COLUMN entry_id TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE kobo_sync_point_books ADD COLUMN delivered_file_type TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE kobo_sync_point_books ADD COLUMN revision_id TEXT NOT NULL DEFAULT ''`,
				`UPDATE kobo_sync_point_books SET
					entry_id = 'shisho-' || file_id,
					delivered_file_type = 'kepub',
					revision_id = COALESCE((
						SELECT e.revision_id FROM kobo_sync_entries e
						JOIN kobo_sync_points sp ON sp.device_id = e.device_id
						WHERE sp.id = kobo_sync_point_books.sync_point_id AND e.file_id = kobo_sync_point_books.file_id
					), '')`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}
			return checkForeignKeys(ctx, tx, []string{"kobo_sync_entries"})
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			// Only an entry still holding a File fits the old table, under
			// that File.
			for _, stmt := range []string{
				`ALTER TABLE kobo_sync_point_books DROP COLUMN entry_id`,
				`ALTER TABLE kobo_sync_point_books DROP COLUMN delivered_file_type`,
				`ALTER TABLE kobo_sync_point_books DROP COLUMN revision_id`,
				`DROP INDEX ix_kobo_sync_entries_book_id`,
				`DELETE FROM kobo_sync_entries WHERE file_id IS NULL`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}
			if err := rebuildTableInTx(ctx, tx, "kobo_sync_entries", koboSyncEntriesByFile); err != nil {
				return err
			}
			for _, stmt := range []string{
				`ALTER TABLE kobo_sync_entries DROP COLUMN entry_id`,
				`ALTER TABLE kobo_sync_entries DROP COLUMN book_id`,
				`ALTER TABLE kobo_sync_entries DROP COLUMN delivered_file_type`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}
			return checkForeignKeys(ctx, tx, []string{"kobo_sync_entries"})
		})
	}

	Migrations.MustRegister(up, down)
}
