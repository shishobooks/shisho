package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE VIRTUAL TABLE publishers_fts USING fts5(
				publisher_id UNINDEXED,
				library_id UNINDEXED,
				name,
				tokenize='unicode61',
				prefix='2,3'
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

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

		// Populate from existing data (including aliases)
		_, err = db.Exec(`
			INSERT INTO publishers_fts (publisher_id, library_id, name)
			SELECT id, library_id,
				name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM publisher_aliases pa WHERE pa.publisher_id = publishers.id), '')
			FROM publishers
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			INSERT INTO imprints_fts (imprint_id, library_id, name)
			SELECT id, library_id,
				name || COALESCE(' ' || (SELECT GROUP_CONCAT(ia.name, ' ') FROM imprint_aliases ia WHERE ia.imprint_id = imprints.id), '')
			FROM imprints
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS imprints_fts")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS publishers_fts")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
