package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN language TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN language_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN abridged INTEGER`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN abridged_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		// Partial index on files.language to speed up:
		//  - the gallery language filter: WHERE language = ? OR language LIKE ?
		//  - the library languages endpoint: SELECT DISTINCT language WHERE language IS NOT NULL
		// Partial because the vast majority of rows will have NULL language and
		// don't need to be included in the index.
		_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_files_language ON files(language) WHERE language IS NOT NULL`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS idx_files_language`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN language`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN language_source`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN abridged`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN abridged_source`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
