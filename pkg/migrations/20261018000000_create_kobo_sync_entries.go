package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE kobo_sync_entries (
				api_key_id TEXT NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
				file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
				revision_id TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (api_key_id, file_id)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_kobo_sync_entries_file_id ON kobo_sync_entries (file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Record the RevisionId each book already on a Kobo went out with:
		// its Shisho id plus the first 8 characters of the file hash in the
		// key's latest completed sync point. Then, for a sync still paging out
		// when the server upgraded, record the files that only its snapshot
		// holds, since earlier pages may already have sent them. Two cases
		// stay uncovered and may add a second copy: a file whose hash changed
		// in that unfinished snapshot and whose page was already sent, and a
		// key whose sync history was cleared with no sync before the upgrade.
		_, err = db.Exec(`
			INSERT OR IGNORE INTO kobo_sync_entries (api_key_id, file_id, revision_id)
			SELECT sp.api_key_id, b.file_id, 'shisho-' || b.file_id || '-' || substr(b.file_hash, 1, 8)
			FROM kobo_sync_point_books b
			JOIN kobo_sync_points sp ON sp.id = b.sync_point_id
			JOIN files f ON f.id = b.file_id
			WHERE sp.id = (
				SELECT latest.id FROM kobo_sync_points latest
				WHERE latest.api_key_id = sp.api_key_id AND latest.completed_at IS NOT NULL
				ORDER BY latest.completed_at DESC, latest.created_at DESC
				LIMIT 1
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			INSERT OR IGNORE INTO kobo_sync_entries (api_key_id, file_id, revision_id)
			SELECT sp.api_key_id, b.file_id, 'shisho-' || b.file_id || '-' || substr(b.file_hash, 1, 8)
			FROM kobo_sync_point_books b
			JOIN kobo_sync_points sp ON sp.id = b.sync_point_id
			JOIN files f ON f.id = b.file_id
			WHERE sp.id = (
				SELECT ongoing.id FROM kobo_sync_points ongoing
				WHERE ongoing.api_key_id = sp.api_key_id AND ongoing.completed_at IS NULL
				AND ongoing.created_at > COALESCE((
					SELECT MAX(done.created_at) FROM kobo_sync_points done
					WHERE done.api_key_id = sp.api_key_id AND done.completed_at IS NOT NULL
				), '')
				ORDER BY ongoing.created_at DESC
				LIMIT 1
			)
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP TABLE IF EXISTS kobo_sync_entries`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
