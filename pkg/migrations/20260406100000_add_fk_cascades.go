package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// recreateTable recreates a table with a new CREATE TABLE statement, preserving data and indexes.
// The newCreateSQL must use a temporary name (table_new), and columns must be compatible with the old table.
func recreateTable(db *bun.DB, table string, newCreateSQL string, indexSQLs []string) error {
	tmpTable := table + "_new"

	// Create new table
	if _, err := db.Exec(newCreateSQL); err != nil {
		return errors.Wrapf(err, "create %s", tmpTable)
	}

	// Copy data
	if _, err := db.Exec("INSERT INTO " + tmpTable + " SELECT * FROM " + table); err != nil {
		return errors.Wrapf(err, "copy data to %s", tmpTable)
	}

	// Drop old table
	if _, err := db.Exec("DROP TABLE " + table); err != nil {
		return errors.Wrapf(err, "drop %s", table)
	}

	// Rename new table
	if _, err := db.Exec("ALTER TABLE " + tmpTable + " RENAME TO " + table); err != nil {
		return errors.Wrapf(err, "rename %s to %s", tmpTable, table)
	}

	// Recreate indexes
	for _, idx := range indexSQLs {
		if _, err := db.Exec(idx); err != nil {
			return errors.Wrapf(err, "create index on %s", table)
		}
	}

	return nil
}

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Add ON DELETE CASCADE / SET NULL to all FK constraints that are missing them.
		// SQLite cannot ALTER constraints, so each table must be recreated.

		// 1. library_paths: library_id → ON DELETE CASCADE
		if err := recreateTable(db, "library_paths",
			`CREATE TABLE library_paths_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id TEXT NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
				filepath TEXT NOT NULL
			)`,
			[]string{
				`CREATE INDEX ix_library_paths_library_id ON library_paths (library_id)`,
			},
		); err != nil {
			return err
		}

		// 2. series: library_id → ON DELETE CASCADE
		if err := recreateTable(db, "series",
			`CREATE TABLE series_new (
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
			[]string{
				`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`,
				`CREATE INDEX ix_series_library_id ON series (library_id)`,
			},
		); err != nil {
			return err
		}

		// 3. persons: library_id → ON DELETE CASCADE
		if err := recreateTable(db, "persons",
			`CREATE TABLE persons_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				sort_name TEXT NOT NULL,
				sort_name_source TEXT NOT NULL
			)`,
			[]string{
				`CREATE UNIQUE INDEX ux_persons_name_library_id ON persons (name COLLATE NOCASE, library_id)`,
				`CREATE INDEX ix_persons_library_id ON persons (library_id)`,
			},
		); err != nil {
			return err
		}

		// 4. books: library_id → CASCADE, primary_file_id → SET NULL, publisher_id/imprint_id → SET NULL
		//    (Drop and re-add FTS triggers if needed — FTS is separate)
		if err := recreateTable(db, "books",
			`CREATE TABLE books_new (
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
				author_source TEXT NOT NULL,
				genre_source TEXT,
				tag_source TEXT,
				primary_file_id INTEGER REFERENCES files (id) ON DELETE SET NULL
			)`,
			[]string{
				`CREATE INDEX ix_books_library_id ON books (library_id)`,
				`CREATE UNIQUE INDEX ux_books_filepath_library_id ON books (filepath, library_id)`,
			},
		); err != nil {
			return err
		}

		// 5. book_series: book_id → CASCADE, series_id → CASCADE
		if err := recreateTable(db, "book_series",
			`CREATE TABLE book_series_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
				series_id INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
				series_number REAL,
				sort_order INTEGER NOT NULL
			)`,
			[]string{
				`CREATE INDEX ix_book_series_book_id ON book_series (book_id)`,
				`CREATE INDEX ix_book_series_series_id ON book_series (series_id)`,
				`CREATE UNIQUE INDEX ux_book_series_book_series ON book_series (book_id, series_id)`,
			},
		); err != nil {
			return err
		}

		// 6. files: book_id → CASCADE, library_id → CASCADE, publisher_id → SET NULL, imprint_id → SET NULL
		if err := recreateTable(db, "files",
			`CREATE TABLE files_new (
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
			[]string{
				`CREATE INDEX ix_files_library_id ON files (library_id)`,
				`CREATE INDEX ix_files_book_id ON files (book_id)`,
				`CREATE UNIQUE INDEX ux_files_filepath_library_id ON files (filepath, library_id)`,
			},
		); err != nil {
			return err
		}

		// 7. authors: book_id → CASCADE
		if err := recreateTable(db, "authors",
			`CREATE TABLE authors_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
				person_id INTEGER NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
				sort_order INTEGER NOT NULL,
				role TEXT
			)`,
			[]string{
				`CREATE INDEX ix_authors_book_id ON authors (book_id)`,
				`CREATE INDEX ix_authors_person_id ON authors (person_id)`,
				`CREATE UNIQUE INDEX ux_authors_book_person_role ON authors (book_id, person_id, role)`,
			},
		); err != nil {
			return err
		}

		// 8. narrators: file_id → CASCADE
		if err := recreateTable(db, "narrators",
			`CREATE TABLE narrators_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
				person_id INTEGER NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
				sort_order INTEGER NOT NULL
			)`,
			[]string{
				`CREATE INDEX ix_narrators_file_id ON narrators (file_id)`,
				`CREATE INDEX ix_narrators_person_id ON narrators (person_id)`,
				`CREATE UNIQUE INDEX ux_narrators_file_person ON narrators (file_id, person_id)`,
			},
		); err != nil {
			return err
		}

		// 9. jobs: library_id → SET NULL
		if err := recreateTable(db, "jobs",
			`CREATE TABLE jobs_new (
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
			[]string{
				`CREATE INDEX ix_jobs_type_library_created ON jobs (type, library_id, created_at DESC)`,
			},
		); err != nil {
			return err
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Reverse all CASCADE/SET NULL changes back to bare REFERENCES.
		// This is safe because removing CASCADE just means the DB won't auto-delete children.

		if err := recreateTable(db, "jobs",
			`CREATE TABLE jobs_new (
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
			[]string{
				`CREATE INDEX ix_jobs_type_library_created ON jobs (type, library_id, created_at DESC)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "narrators",
			`CREATE TABLE narrators_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				file_id INTEGER NOT NULL REFERENCES files (id),
				person_id INTEGER NOT NULL REFERENCES persons (id),
				sort_order INTEGER NOT NULL
			)`,
			[]string{
				`CREATE INDEX ix_narrators_file_id ON narrators (file_id)`,
				`CREATE INDEX ix_narrators_person_id ON narrators (person_id)`,
				`CREATE UNIQUE INDEX ux_narrators_file_person ON narrators (file_id, person_id)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "authors",
			`CREATE TABLE authors_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				book_id INTEGER NOT NULL REFERENCES books (id),
				person_id INTEGER NOT NULL REFERENCES persons (id),
				sort_order INTEGER NOT NULL,
				role TEXT
			)`,
			[]string{
				`CREATE INDEX ix_authors_book_id ON authors (book_id)`,
				`CREATE INDEX ix_authors_person_id ON authors (person_id)`,
				`CREATE UNIQUE INDEX ux_authors_book_person_role ON authors (book_id, person_id, role)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "files",
			`CREATE TABLE files_new (
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
			[]string{
				`CREATE INDEX ix_files_library_id ON files (library_id)`,
				`CREATE INDEX ix_files_book_id ON files (book_id)`,
				`CREATE UNIQUE INDEX ux_files_filepath_library_id ON files (filepath, library_id)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "book_series",
			`CREATE TABLE book_series_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				book_id INTEGER NOT NULL REFERENCES books (id),
				series_id INTEGER NOT NULL REFERENCES series (id),
				series_number REAL,
				sort_order INTEGER NOT NULL
			)`,
			[]string{
				`CREATE INDEX ix_book_series_book_id ON book_series (book_id)`,
				`CREATE INDEX ix_book_series_series_id ON book_series (series_id)`,
				`CREATE UNIQUE INDEX ux_book_series_book_series ON book_series (book_id, series_id)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "books",
			`CREATE TABLE books_new (
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
				author_source TEXT NOT NULL,
				genre_source TEXT,
				tag_source TEXT,
				primary_file_id INTEGER REFERENCES files (id)
			)`,
			[]string{
				`CREATE INDEX ix_books_library_id ON books (library_id)`,
				`CREATE UNIQUE INDEX ux_books_filepath_library_id ON books (filepath, library_id)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "persons",
			`CREATE TABLE persons_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER NOT NULL REFERENCES libraries (id),
				name TEXT NOT NULL,
				sort_name TEXT NOT NULL,
				sort_name_source TEXT NOT NULL
			)`,
			[]string{
				`CREATE UNIQUE INDEX ux_persons_name_library_id ON persons (name COLLATE NOCASE, library_id)`,
				`CREATE INDEX ix_persons_library_id ON persons (library_id)`,
			},
		); err != nil {
			return err
		}

		if err := recreateTable(db, "series",
			`CREATE TABLE series_new (
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
			[]string{
				`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`,
				`CREATE INDEX ix_series_library_id ON series (library_id)`,
			},
		); err != nil {
			return err
		}

		return recreateTable(db, "library_paths",
			`CREATE TABLE library_paths_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id TEXT NOT NULL REFERENCES libraries (id),
				filepath TEXT NOT NULL
			)`,
			[]string{
				`CREATE INDEX ix_library_paths_library_id ON library_paths (library_id)`,
			},
		)
	}

	Migrations.MustRegister(up, down)
}
