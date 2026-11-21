package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// A Shisho app Device records the installation that signed in and the
	// app version it last reported. The installation id is made when the
	// app is installed and kept across signing out and back in, so the
	// journal (ADR 0024) can recognize events an installation resends after
	// signing in again as a new Device. Other Device types leave both NULL.
	up := func(_ context.Context, db *bun.DB) error {
		statements := []string{
			`ALTER TABLE devices ADD COLUMN installation_id TEXT`,
			`ALTER TABLE devices ADD COLUMN app_version TEXT`,
		}
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		statements := []string{
			`DELETE FROM devices WHERE device_type = 'shisho_app'`,
			`ALTER TABLE devices DROP COLUMN app_version`,
			`ALTER TABLE devices DROP COLUMN installation_id`,
		}
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
