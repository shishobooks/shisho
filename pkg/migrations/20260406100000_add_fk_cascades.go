package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Each rebuild below is applied with rebuildTableInTx, which copies by column
// name and keeps the table's indexes, so a later migration that reorders or
// indexes these tables does not break the down.
type fkCascadeRebuild struct {
	table string
	// up adds ON DELETE CASCADE / SET NULL; down restores bare REFERENCES.
	up, down string
}

// books.author_source is nullable in both directions, although it was NOT
// NULL when this migration was written: 20260919000001 made it nullable and
// its down keeps it that way, so rows rolled back past it can hold NULL.
var fkCascadeRebuilds = []fkCascadeRebuild{
	{
		table: "library_paths",
		up: `CREATE TABLE library_paths_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id TEXT NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
			filepath TEXT NOT NULL
		)`,
		down: `CREATE TABLE library_paths_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id TEXT NOT NULL REFERENCES libraries (id),
			filepath TEXT NOT NULL
		)`,
	},
	{
		table: "series",
		up: `CREATE TABLE series_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			name_source TEXT NOT NULL,
			sort_name TEXT NOT NULL,
			sort_name_source TEXT NOT NULL,
			description TEXT,
			cover_image_filename TEXT
		)`,
		down: `CREATE TABLE series_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			library_id INTEGER NOT NULL REFERENCES libraries (id),
			name TEXT NOT NULL,
			name_source TEXT NOT NULL,
			sort_name TEXT NOT NULL,
			sort_name_source TEXT NOT NULL,
			description TEXT,
			cover_image_filename TEXT
		)`,
	},
	{
		table: "persons",
		up: `CREATE TABLE persons_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			sort_name TEXT NOT NULL,
			sort_name_source TEXT NOT NULL
		)`,
		down: `CREATE TABLE persons_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id INTEGER NOT NULL REFERENCES libraries (id),
			name TEXT NOT NULL,
			sort_name TEXT NOT NULL,
			sort_name_source TEXT NOT NULL
		)`,
	},
	{
		table: "books",
		up: `CREATE TABLE books_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
			filepath TEXT NOT NULL,
			title TEXT NOT NULL,
			title_source TEXT NOT NULL,
			sort_title TEXT NOT NULL,
			sort_title_source TEXT NOT NULL,
			subtitle TEXT,
			subtitle_source TEXT,
			description TEXT,
			description_source TEXT,
			author_source TEXT,
			genre_source TEXT,
			tag_source TEXT,
			primary_file_id INTEGER REFERENCES files (id) ON DELETE SET NULL
		)`,
		down: `CREATE TABLE books_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id INTEGER NOT NULL REFERENCES libraries (id),
			filepath TEXT NOT NULL,
			title TEXT NOT NULL,
			title_source TEXT NOT NULL,
			sort_title TEXT NOT NULL,
			sort_title_source TEXT NOT NULL,
			subtitle TEXT,
			subtitle_source TEXT,
			description TEXT,
			description_source TEXT,
			author_source TEXT,
			genre_source TEXT,
			tag_source TEXT,
			primary_file_id INTEGER REFERENCES files (id)
		)`,
	},
	{
		table: "book_series",
		up: `CREATE TABLE book_series_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
			series_id INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
			series_number REAL,
			sort_order INTEGER NOT NULL
		)`,
		down: `CREATE TABLE book_series_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id INTEGER NOT NULL REFERENCES books (id),
			series_id INTEGER NOT NULL REFERENCES series (id),
			series_number REAL,
			sort_order INTEGER NOT NULL
		)`,
	},
	{
		table: "files",
		up: `CREATE TABLE files_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id TEXT NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
			book_id TEXT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
			filepath TEXT NOT NULL,
			file_type TEXT NOT NULL,
			file_role TEXT NOT NULL DEFAULT 'main',
			filesize_bytes INTEGER NOT NULL,
			cover_image_filename TEXT,
			cover_mime_type TEXT,
			cover_source TEXT,
			cover_page INTEGER,
			name TEXT,
			name_source TEXT,
			page_count INTEGER,
			audiobook_duration_seconds DOUBLE,
			audiobook_bitrate_bps INTEGER,
			narrator_source TEXT,
			url TEXT,
			url_source TEXT,
			release_date DATE,
			release_date_source TEXT,
			publisher_id INTEGER REFERENCES publishers (id) ON DELETE SET NULL,
			publisher_source TEXT,
			imprint_id INTEGER REFERENCES imprints (id) ON DELETE SET NULL,
			imprint_source TEXT,
			identifier_source TEXT,
			chapter_source TEXT,
			audiobook_codec TEXT,
			file_modified_at DATETIME
		)`,
		down: `CREATE TABLE files_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id TEXT NOT NULL REFERENCES libraries (id),
			book_id TEXT NOT NULL REFERENCES books (id),
			filepath TEXT NOT NULL,
			file_type TEXT NOT NULL,
			file_role TEXT NOT NULL DEFAULT 'main',
			filesize_bytes INTEGER NOT NULL,
			cover_image_filename TEXT,
			cover_mime_type TEXT,
			cover_source TEXT,
			cover_page INTEGER,
			name TEXT,
			name_source TEXT,
			page_count INTEGER,
			audiobook_duration_seconds DOUBLE,
			audiobook_bitrate_bps INTEGER,
			narrator_source TEXT,
			url TEXT,
			url_source TEXT,
			release_date DATE,
			release_date_source TEXT,
			publisher_id INTEGER REFERENCES publishers (id),
			publisher_source TEXT,
			imprint_id INTEGER REFERENCES imprints (id),
			imprint_source TEXT,
			identifier_source TEXT,
			chapter_source TEXT,
			audiobook_codec TEXT,
			file_modified_at DATETIME
		)`,
	},
	{
		table: "authors",
		up: `CREATE TABLE authors_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
			person_id INTEGER NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
			sort_order INTEGER NOT NULL,
			role TEXT
		)`,
		down: `CREATE TABLE authors_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id INTEGER NOT NULL REFERENCES books (id),
			person_id INTEGER NOT NULL REFERENCES persons (id),
			sort_order INTEGER NOT NULL,
			role TEXT
		)`,
	},
	{
		table: "narrators",
		up: `CREATE TABLE narrators_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
			person_id INTEGER NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
			sort_order INTEGER NOT NULL
		)`,
		down: `CREATE TABLE narrators_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES files (id),
			person_id INTEGER NOT NULL REFERENCES persons (id),
			sort_order INTEGER NOT NULL
		)`,
	},
	{
		table: "jobs",
		up: `CREATE TABLE jobs_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			data TEXT NOT NULL,
			progress INTEGER NOT NULL,
			process_id TEXT,
			library_id INTEGER REFERENCES libraries (id) ON DELETE SET NULL
		)`,
		down: `CREATE TABLE jobs_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			data TEXT NOT NULL,
			progress INTEGER NOT NULL,
			process_id TEXT,
			library_id INTEGER REFERENCES libraries (id)
		)`,
	},
}

func fkCascadeTables() []string {
	tables := make([]string, len(fkCascadeRebuilds))
	for i, rebuild := range fkCascadeRebuilds {
		tables[i] = rebuild.table
	}
	return tables
}

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			// The hand-written rebuild this migration first shipped with
			// recreated only the indexes it listed, which dropped this one.
			// Drop it here too, so new installs match upgraded ones.
			if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS ix_jobs_status_created_at"); err != nil {
				return errors.WithStack(err)
			}
			for _, rebuild := range fkCascadeRebuilds {
				if err := rebuildTableInTx(ctx, tx, rebuild.table, rebuild.up); err != nil {
					return err
				}
			}
			// No checkForeignKeys: foreign keys were not enforced before this
			// migration, so upgraded databases can hold rows with a missing
			// parent. 20261005000000 repairs them.
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			for _, rebuild := range fkCascadeRebuilds {
				if err := rebuildTableInTx(ctx, tx, rebuild.table, rebuild.down); err != nil {
					return err
				}
			}
			return checkForeignKeys(ctx, tx, fkCascadeTables())
		})
	}

	Migrations.MustRegister(up, down)
}
