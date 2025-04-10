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
				id TEXT PRIMARY KEY,
				type TEXT NOT NULL,
				status TEXT NOT NULL,
				data TEXT NOT NULL,
				progress INTEGER NOT NULL,
				process_id TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE libraries (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				deleted_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE library_paths (
				id TEXT PRIMARY KEY,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				filepath TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
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
			CREATE TABLE books (
				id TEXT PRIMARY KEY,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				filepath TEXT NOT NULL,
				title TEXT NOT NULL,
				title_source TEXT NOT NULL,
				subtitle TEXT,
				subtitle_source TEXT,
				author_source TEXT NOT NULL,
				series TEXT,
				series_number REAL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
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
		_, err = db.Exec(`
			CREATE TABLE files (
				id TEXT PRIMARY KEY,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				book_id TEXT REFERENCES books (id) NOT NULL,
				filepath TEXT NOT NULL,
				file_type TEXT NOT NULL,
				filesize_bytes BIGINT NOT NULL,
				cover_mime_type TEXT,
				audiobook_duration DOUBLE,
				audiobook_bitrate INTEGER,
				narrator_source TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
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
				id TEXT PRIMARY KEY,
				book_id TEXT REFERENCES books (id) NOT NULL,
				type TEXT NOT NULL,
				value TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
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
				id TEXT PRIMARY KEY,
				book_id TEXT REFERENCES books (id) NOT NULL,
				name TEXT NOT NULL,
				sequence INTEGER NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
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
				id TEXT PRIMARY KEY,
				file_id TEXT REFERENCES files (id) NOT NULL,
				name TEXT NOT NULL,
				sequence INTEGER NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
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
