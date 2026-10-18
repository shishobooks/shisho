package migrations

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Foreign keys stay off for the whole migration: with them on, DROP
		// TABLE files in step 6 deletes every row that references a file.
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			for _, q := range []string{
				// 1. Add nullable parent_id to publishers (self-referential FK)
				`ALTER TABLE publishers ADD COLUMN parent_id INTEGER REFERENCES publishers(id) ON DELETE SET NULL`,

				// 2. Copy all imprint rows into publishers (skip on name conflict)
				`INSERT OR IGNORE INTO publishers (created_at, updated_at, library_id, name)
				SELECT created_at, updated_at, library_id, name FROM imprints`,

				// 3. Copy imprint_aliases into publisher_aliases (skip on conflict)
				`INSERT OR IGNORE INTO publisher_aliases (created_at, publisher_id, name, library_id)
				SELECT ia.created_at, p.id, ia.name, ia.library_id
				FROM imprint_aliases ia
				JOIN imprints i ON ia.imprint_id = i.id
				JOIN publishers p ON LOWER(p.name) = LOWER(i.name) AND p.library_id = i.library_id`,

				// 4. For files with imprint_id: resolve the imprint name -> find corresponding
				//    publisher row, set publisher_id to it. Imprint (more specific) wins over
				//    existing publisher.
				`UPDATE files
				SET publisher_id = (
					SELECT p.id FROM imprints i
					JOIN publishers p ON LOWER(p.name) = LOWER(i.name) AND p.library_id = i.library_id
					WHERE i.id = files.imprint_id
				),
				publisher_source = COALESCE(imprint_source, publisher_source)
				WHERE imprint_id IS NOT NULL`,

				// 5. Drop imprints_fts, imprint_aliases, imprints tables
				`DROP TABLE IF EXISTS imprints_fts`,
				`DROP TABLE IF EXISTS imprint_aliases`,
				`DROP TABLE IF EXISTS imprints`,

				// 6. Drop imprint_id and imprint_source columns from files.
				// SQLite requires table recreation to drop columns.
				// rebuildTableInTx needs matching column sets, so the copy is
				// spelled out here, inside the foreign-keys-off transaction.
				`CREATE TABLE files_new (
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
				)`,
				`INSERT INTO files_new (
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
				FROM files`,
				`DROP TABLE files`,
				`ALTER TABLE files_new RENAME TO files`,

				// Recreate indexes on files
				`CREATE UNIQUE INDEX ux_files_filepath_library_id ON files (filepath, library_id)`,
				`CREATE INDEX ix_files_book_id ON files (book_id)`,
				`CREATE INDEX ix_files_library_id ON files (library_id)`,
				`CREATE INDEX ix_files_publisher_id ON files (publisher_id)`,

				// Rebuild publishers_fts to include any newly-migrated imprint data
				`DELETE FROM publishers_fts`,
				`INSERT INTO publishers_fts (publisher_id, library_id, name)
				SELECT id, library_id,
					name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM publisher_aliases pa WHERE pa.publisher_id = publishers.id), '')
				FROM publishers`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}

			// 7. Remove "imprint" from review_criteria JSON in app_settings.
			// If a user had "imprint" in their criteria, the isPresent() switch
			// would fall through and permanently flag files as incomplete.
			if err := cleanReviewCriteriaImprint(ctx, tx); err != nil {
				return err
			}

			// No checkForeignKeys, for the reason given in 20260406100000.
			// 20260928110000 repairs files with a missing parent.
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, q := range []string{
				// Recreate imprints table
				`CREATE TABLE imprints (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					library_id INTEGER NOT NULL,
					name TEXT NOT NULL
				)`,
				`CREATE UNIQUE INDEX ux_imprints_name_library_id ON imprints (name COLLATE NOCASE, library_id)`,
				`CREATE INDEX ix_imprints_library_id ON imprints (library_id)`,

				// Recreate imprint_aliases table
				`CREATE TABLE imprint_aliases (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					imprint_id INTEGER NOT NULL REFERENCES imprints(id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					library_id INTEGER NOT NULL
				)`,

				// Recreate imprints_fts
				`CREATE VIRTUAL TABLE imprints_fts USING fts5(
					imprint_id UNINDEXED,
					library_id UNINDEXED,
					name,
					tokenize='unicode61',
					prefix='2,3'
				)`,

				// Add imprint columns back to files
				`ALTER TABLE files ADD COLUMN imprint_id INTEGER REFERENCES imprints(id)`,
				`ALTER TABLE files ADD COLUMN imprint_source TEXT`,

				// Drop parent_id from publishers
				`ALTER TABLE publishers DROP COLUMN parent_id`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}
			return nil
		})
	}

	Migrations.MustRegister(up, down)
}

// cleanReviewCriteriaImprint removes "imprint" from the review_criteria JSON
// stored in app_settings. The value schema is:
//
//	{"book_fields":["publisher","imprint",...],"audio_fields":[...]}
//
// "imprint" is no longer a valid field after this migration.
func cleanReviewCriteriaImprint(ctx context.Context, tx bun.Tx) error {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'review_criteria'`).Scan(&value)
	if err != nil {
		// No row means the user never customized criteria — nothing to clean.
		return nil
	}

	var criteria map[string][]string
	if err := json.Unmarshal([]byte(value), &criteria); err != nil {
		// Malformed JSON — skip silently, don't fail the migration.
		return nil
	}

	changed := false
	for key, fields := range criteria {
		filtered := make([]string, 0, len(fields))
		for _, f := range fields {
			if f == "imprint" {
				changed = true
				continue
			}
			filtered = append(filtered, f)
		}
		criteria[key] = filtered
	}

	if !changed {
		return nil
	}

	encoded, err := json.Marshal(criteria)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = tx.ExecContext(ctx, `UPDATE app_settings SET value = ? WHERE key = 'review_criteria'`, string(encoded))
	return errors.WithStack(err)
}
