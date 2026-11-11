package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// When a KOReader Device last reached Shisho through the KOReader
			// plugin, and the plugin version it reported then.
			`ALTER TABLE devices ADD COLUMN plugin_last_connected_at TIMESTAMP`,
			`ALTER TABLE devices ADD COLUMN plugin_version TEXT`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`ALTER TABLE devices DROP COLUMN plugin_version`,
			`ALTER TABLE devices DROP COLUMN plugin_last_connected_at`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
