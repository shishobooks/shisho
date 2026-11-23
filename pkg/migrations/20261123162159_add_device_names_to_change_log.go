package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// A Reading Position in the change feed (ADR 0023) names the Device that
	// sent it, so the Shisho app can say where another device's place came
	// from. Renaming a Device moves the entries of its positions, so the app
	// gets the new name. Deleting one clears the positions' source_device_id,
	// which their own update trigger already compares.
	statements := []string{
		`CREATE TRIGGER change_log_devices_update AFTER UPDATE ON devices WHEN OLD.name IS NOT NEW.name BEGIN
			DELETE FROM change_log_entries WHERE kind = 'reading_position' AND EXISTS (
				SELECT 1 FROM reading_positions rp WHERE rp.source_device_id = NEW.id
				AND rp.file_id = change_log_entries.record_id AND rp.user_id = change_log_entries.user_id);
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
				SELECT 'reading_position', file_id, user_id, FALSE, CURRENT_TIMESTAMP
				FROM reading_positions WHERE source_device_id = NEW.id ORDER BY occurred_at;
		END`,

		// Every position a Device sent moves, so an installation that
		// already synced gets its name too.
		`DELETE FROM change_log_entries WHERE kind = 'reading_position' AND EXISTS (
			SELECT 1 FROM reading_positions rp WHERE rp.source_device_id IS NOT NULL
			AND rp.file_id = change_log_entries.record_id AND rp.user_id = change_log_entries.user_id)`,
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'reading_position', file_id, user_id, FALSE, CURRENT_TIMESTAMP
			FROM reading_positions WHERE source_device_id IS NOT NULL ORDER BY occurred_at`,
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, statement := range statements {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.Wrapf(err, "run %s", statement)
				}
			}
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TRIGGER IF EXISTS change_log_devices_update`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
