package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/uptrace/bun"
)

// Three table rebuilds that fix foreign key declarations SQLite cannot ALTER:
//
//   - files.library_id regains REFERENCES libraries (id) ON DELETE CASCADE,
//     which the files_new rebuild in 20260516000000 dropped.
//   - users.role_id becomes ON DELETE RESTRICT instead of the implicit
//     NO ACTION. roles.Service.Delete already refuses to delete a role that
//     users hold; this states that intent in the schema.
//   - library_paths.library_id is declared INTEGER instead of TEXT, matching
//     libraries.id. Copying the rows converts the stored '1' text values to
//     integers through the new column's affinity.
//
// Each CREATE TABLE below is the table's current definition with only that
// change, and with columns added by later ALTER TABLE statements folded in.
var foreignKeyRebuilds = []struct {
	table     string
	createSQL string
}{
	{
		table: "files",
		createSQL: `CREATE TABLE files_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
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
			reviewed BOOLEAN,
			is_preferred_cover BOOLEAN NOT NULL DEFAULT FALSE,
			scan_error TEXT
		)`,
	},
	{
		table: "users",
		createSQL: `CREATE TABLE users_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			email TEXT COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role_id INTEGER NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			must_change_password BOOLEAN NOT NULL DEFAULT FALSE
		)`,
	},
	{
		table: "library_paths",
		createSQL: `CREATE TABLE library_paths_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
			filepath TEXT NOT NULL
		)`,
	},
}

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			if err := repairOrphans(ctx, tx); err != nil {
				return err
			}
			tables := make([]string, 0, len(foreignKeyRebuilds))
			for _, rebuild := range foreignKeyRebuilds {
				if err := rebuildTableInTx(ctx, tx, rebuild.table, rebuild.createSQL); err != nil {
					return err
				}
				tables = append(tables, rebuild.table)
			}
			return checkForeignKeys(ctx, tx, tables)
		})
	}

	// Down is a no-op. Reversing would take three more rebuilds to restore a
	// missing foreign key, an implicit NO ACTION, and a mistyped column. Code
	// from before this migration runs against the corrected schema unchanged:
	// files already cascade through book_id, role deletes are already refused
	// while users hold the role, and library ids are already written as
	// integers. Re-running up after a rollback rebuilds into the same schema.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}

// Files that CASCADE would have removed had enforcement been on: the Book is
// gone, or the file's library is gone and its Book's library is too.
const orphanFileIDs = `SELECT f.id FROM files f
	WHERE NOT EXISTS (SELECT 1 FROM books b WHERE b.id = f.book_id)
	OR (
		NOT EXISTS (SELECT 1 FROM libraries l WHERE l.id = f.library_id)
		AND NOT EXISTS (
			SELECT 1 FROM books b JOIN libraries l ON l.id = b.library_id
			WHERE b.id = f.book_id
		)
	)`

// repairOrphans fixes rows that already pointed at a missing parent before
// this migration, applying each column's ON DELETE action by hand. Foreign
// keys were not enforced before 20260406, and files.library_id had no
// foreign key at all, so such rows can exist. Without this, the check after
// the rebuild would fail startup on every boot. A users row with a missing
// role is left for that check to report, since no action says which role
// the user should get.
func repairOrphans(ctx context.Context, tx bun.Tx) error {
	repairs := []struct {
		label string
		query string
	}{
		// Enforcement is off, so delete the children CASCADE would remove.
		// These are every table with a foreign key to files at this point.
		{"chapters_deleted", "DELETE FROM chapters WHERE file_id IN (" + orphanFileIDs + ")"},
		{"file_identifiers_deleted", "DELETE FROM file_identifiers WHERE file_id IN (" + orphanFileIDs + ")"},
		{"file_fingerprints_deleted", "DELETE FROM file_fingerprints WHERE file_id IN (" + orphanFileIDs + ")"},
		{"narrators_deleted", "DELETE FROM narrators WHERE file_id IN (" + orphanFileIDs + ")"},
		{"files_deleted", "DELETE FROM files WHERE id IN (" + orphanFileIDs + ")"},
		// Every remaining file has a Book in a live library, which is the
		// library the file belongs to.
		{"files_library_id_reset", `UPDATE files SET library_id = (SELECT b.library_id FROM books b WHERE b.id = files.book_id)
			WHERE NOT EXISTS (SELECT 1 FROM libraries l WHERE l.id = files.library_id)`},
		{"files_publisher_id_cleared", `UPDATE files SET publisher_id = NULL
			WHERE publisher_id IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM publishers p WHERE p.id = files.publisher_id)`},
		{"library_paths_deleted", `DELETE FROM library_paths
			WHERE NOT EXISTS (SELECT 1 FROM libraries l WHERE l.id = library_paths.library_id)`},
	}

	counts := logger.Data{}
	for _, repair := range repairs {
		result, err := tx.ExecContext(ctx, repair.query)
		if err != nil {
			return errors.Wrapf(err, "repair rows with a missing parent (%s)", repair.label)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return errors.WithStack(err)
		}
		if affected > 0 {
			counts[repair.label] = affected
		}
	}
	if len(counts) > 0 {
		logger.New().Warn("repaired rows that referenced a missing parent before rebuilding files, users, and library_paths", counts)
	}
	return nil
}
