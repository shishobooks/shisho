package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// Each Device follows its user's default Delivery Rules, follows its
	// type's preset, or has its own list (ADR 0011). Kobo and Kindle Devices
	// start on their presets, as new ones do; the Kobo preset delivers what
	// Kobo sync already sends. Every other existing Device follows the
	// user's default. A following Device has no
	// rows, so a following list is never copied.
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			statements := []string{
				`ALTER TABLE devices ADD COLUMN delivery_rules_mode TEXT NOT NULL DEFAULT 'default'`,
				`UPDATE devices SET delivery_rules_mode = 'preset' WHERE device_type IN ('kobo', 'kindle')`,
				`CREATE TABLE device_delivery_rules (
					device_id TEXT REFERENCES devices (id) ON DELETE CASCADE NOT NULL,
					source_file_type TEXT NOT NULL,
					delivered_file_type TEXT NOT NULL,
					position INTEGER NOT NULL,
					PRIMARY KEY (device_id, source_file_type)
				)`,
			}
			for _, statement := range statements {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			statements := []string{
				`DROP TABLE IF EXISTS device_delivery_rules`,
				`ALTER TABLE devices DROP COLUMN delivery_rules_mode`,
			}
			for _, statement := range statements {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		})
	}

	Migrations.MustRegister(up, down)
}
