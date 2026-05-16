package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// 1. Add nullable parent_id to publishers (self-referential FK)
		_, err := db.Exec(`ALTER TABLE publishers ADD COLUMN parent_id INTEGER REFERENCES publishers(id) ON DELETE SET NULL`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 2. Copy all imprint rows into publishers (skip on name conflict)
		_, err = db.Exec(`
			INSERT OR IGNORE INTO publishers (created_at, updated_at, library_id, name)
			SELECT created_at, updated_at, library_id, name FROM imprints
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 3. Copy imprint_aliases into publisher_aliases (skip on conflict)
		_, err = db.Exec(`
			INSERT OR IGNORE INTO publisher_aliases (created_at, publisher_id, name, library_id)
			SELECT ia.created_at, p.id, ia.name, ia.library_id
			FROM imprint_aliases ia
			JOIN imprints i ON ia.imprint_id = i.id
			JOIN publishers p ON LOWER(p.name) = LOWER(i.name) AND p.library_id = i.library_id
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 4. For files with imprint_id: resolve the imprint name -> find corresponding
		//    publisher row, set publisher_id to it. Imprint (more specific) wins over
		//    existing publisher.
		_, err = db.Exec(`
			UPDATE files
			SET publisher_id = (
				SELECT p.id FROM imprints i
				JOIN publishers p ON LOWER(p.name) = LOWER(i.name) AND p.library_id = i.library_id
				WHERE i.id = files.imprint_id
			),
			publisher_source = COALESCE(imprint_source, publisher_source)
			WHERE imprint_id IS NOT NULL
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 5. Drop imprints_fts, imprint_aliases, imprints tables
		_, err = db.Exec(`DROP TABLE IF EXISTS imprints_fts`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS imprint_aliases`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS imprints`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 6. Drop imprint_id and imprint_source columns from files.
		// SQLite requires table recreation to drop columns.
		_, err = db.Exec(`
			CREATE TABLE files_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER NOT NULL,
				book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
				filepath TEXT NOT NULL,
				file_type TEXT NOT NULL,
				file_role TEXT NOT NULL DEFAULT 'main',
				filesize_bytes INTEGER NOT NULL DEFAULT 0,
				file_modified_at TIMESTAMPTZ,
				cover_image_filename TEXT,
				cover_mime_type TEXT,
				cover_source TEXT,
				cover_page INTEGER,
				name TEXT,
				name_source TEXT,
				page_count INTEGER,
				audiobook_duration_seconds REAL,
				audiobook_bitrate_bps INTEGER,
				audiobook_codec TEXT,
				narrator_source TEXT,
				identifier_source TEXT,
				url TEXT,
				url_source TEXT,
				release_date TIMESTAMPTZ,
				release_date_source TEXT,
				publisher_id INTEGER REFERENCES publishers(id) ON DELETE SET NULL,
				publisher_source TEXT,
				chapter_source TEXT,
				language TEXT,
				language_source TEXT,
				abridged BOOLEAN,
				abridged_source TEXT,
				review_override TEXT,
				review_overridden_at TIMESTAMPTZ,
				reviewed BOOLEAN
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			INSERT INTO files_new (
				id, created_at, updated_at, library_id, book_id, filepath, file_type, file_role,
				filesize_bytes, file_modified_at, cover_image_filename, cover_mime_type, cover_source, cover_page,
				name, name_source, page_count, audiobook_duration_seconds, audiobook_bitrate_bps, audiobook_codec,
				narrator_source, identifier_source, url, url_source, release_date, release_date_source,
				publisher_id, publisher_source, chapter_source, language, language_source,
				abridged, abridged_source, review_override, review_overridden_at, reviewed
			)
			SELECT
				id, created_at, updated_at, library_id, book_id, filepath, file_type, file_role,
				filesize_bytes, file_modified_at, cover_image_filename, cover_mime_type, cover_source, cover_page,
				name, name_source, page_count, audiobook_duration_seconds, audiobook_bitrate_bps, audiobook_codec,
				narrator_source, identifier_source, url, url_source, release_date, release_date_source,
				publisher_id, publisher_source, chapter_source, language, language_source,
				abridged, abridged_source, review_override, review_overridden_at, reviewed
			FROM files
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`DROP TABLE files`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE files_new RENAME TO files`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recreate indexes on files
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_files_filepath_library_id ON files (filepath, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_book_id ON files (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_library_id ON files (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_publisher_id ON files (publisher_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rebuild publishers_fts to include any newly-migrated imprint data
		_, err = db.Exec(`DELETE FROM publishers_fts`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			INSERT INTO publishers_fts (publisher_id, library_id, name)
			SELECT id, library_id,
				name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM publisher_aliases pa WHERE pa.publisher_id = publishers.id), '')
			FROM publishers
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Recreate imprints table
		_, err := db.Exec(`
			CREATE TABLE imprints (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER NOT NULL,
				name TEXT NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_imprints_name_library_id ON imprints (name COLLATE NOCASE, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_imprints_library_id ON imprints (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recreate imprint_aliases table
		_, err = db.Exec(`
			CREATE TABLE imprint_aliases (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				imprint_id INTEGER NOT NULL REFERENCES imprints(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				library_id INTEGER NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recreate imprints_fts
		_, err = db.Exec(`
			CREATE VIRTUAL TABLE imprints_fts USING fts5(
				imprint_id UNINDEXED,
				library_id UNINDEXED,
				name,
				tokenize='unicode61',
				prefix='2,3'
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Add imprint columns back to files (re-create table)
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN imprint_id INTEGER REFERENCES imprints(id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN imprint_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Drop parent_id from publishers
		// SQLite can't drop columns easily, but ALTER TABLE DROP COLUMN works in 3.35.0+
		_, err = db.Exec(`ALTER TABLE publishers DROP COLUMN parent_id`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
