package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE jobs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				type TEXT NOT NULL,
				status TEXT NOT NULL,
				data TEXT NOT NULL,
				progress INTEGER NOT NULL,
				process_id TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE libraries (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				name TEXT NOT NULL,
				organize_file_structure BOOLEAN NOT NULL DEFAULT TRUE,
				deleted_at TIMESTAMPTZ
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE library_paths (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				filepath TEXT NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_library_paths_library_id ON library_paths (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE series (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMPTZ,
				library_id INTEGER REFERENCES libraries (id) NOT NULL,
				name TEXT NOT NULL,
				name_source TEXT NOT NULL,
				description TEXT,
				cover_image_path TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		// Case-insensitive unique constraint (only for non-deleted records)
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_series_library_id ON series (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE books (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER REFERENCES libraries (id) NOT NULL,
				filepath TEXT NOT NULL,
				title TEXT NOT NULL,
				title_source TEXT NOT NULL,
				subtitle TEXT,
				subtitle_source TEXT,
				author_source TEXT NOT NULL,
				series_id INTEGER REFERENCES series (id),
				series_number REAL,
				cover_image_path TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_books_library_id ON books (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_books_filepath_library_id ON books (filepath, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_books_series_id ON books (series_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE files (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				book_id TEXT REFERENCES books (id) NOT NULL,
				filepath TEXT NOT NULL,
				file_type TEXT NOT NULL,
				filesize_bytes INTEGER NOT NULL,
				cover_mime_type TEXT,
				cover_source TEXT,
				audiobook_duration DOUBLE,
				audiobook_bitrate INTEGER,
				narrator_source TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_library_id ON files (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_book_id ON files (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_files_filepath_library_id ON files (filepath, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE book_identifiers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				book_id TEXT REFERENCES books (id) NOT NULL,
				type TEXT NOT NULL,
				value TEXT NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_book_identifiers_book_id ON book_identifiers (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE authors (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				book_id TEXT REFERENCES books (id) NOT NULL,
				name TEXT NOT NULL,
				sort_order INTEGER NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_authors_book_id ON authors (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE narrators (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id TEXT REFERENCES files (id) NOT NULL,
				name TEXT NOT NULL,
				sort_order INTEGER NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_narrators_file_id ON narrators (file_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS narrators")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS authors")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS book_identifiers")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS files")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS books")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS series")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS library_paths")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS libraries")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS jobs")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
