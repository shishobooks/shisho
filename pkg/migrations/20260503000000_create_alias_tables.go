package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		tables := []struct {
			name   string
			fk     string
			parent string
		}{
			{"genre_aliases", "genre_id", "genres"},
			{"tag_aliases", "tag_id", "tags"},
			{"series_aliases", "series_id", "series"},
			{"person_aliases", "person_id", "persons"},
			{"publisher_aliases", "publisher_id", "publishers"},
			{"imprint_aliases", "imprint_id", "imprints"},
		}

		for _, t := range tables {
			create := `CREATE TABLE ` + t.name + ` (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				` + t.fk + ` INTEGER NOT NULL REFERENCES ` + t.parent + `(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE
			)`
			if _, err := db.Exec(create); err != nil {
				return errors.Wrapf(err, "creating table %s", t.name)
			}

			idx := `CREATE UNIQUE INDEX ux_` + t.name + `_name_library_id ON ` + t.name + ` (name COLLATE NOCASE, library_id)`
			if _, err := db.Exec(idx); err != nil {
				return errors.Wrapf(err, "creating unique index on %s", t.name)
			}
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		tables := []string{
			"imprint_aliases",
			"publisher_aliases",
			"person_aliases",
			"series_aliases",
			"tag_aliases",
			"genre_aliases",
		}
		for _, t := range tables {
			if _, err := db.Exec("DROP TABLE IF EXISTS " + t); err != nil {
				return errors.Wrapf(err, "dropping table %s", t)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
