package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Delete hook-order rows for the outputGenerator hook, which was removed from
// the plugin contract, so the API never returns a hook type the frontend no
// longer knows. The hook never ran, so there is nothing to restore on
// rollback.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, table := range []string{"plugin_hook_configs", "library_plugin_customizations", "library_plugin_hook_configs"} {
				if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE hook_type = 'outputGenerator'`); err != nil {
					return errors.Wrapf(err, "failed to delete outputGenerator rows from %s", table)
				}
			}
			return nil
		})
	}
	down := func(_ context.Context, _ *bun.DB) error {
		return nil
	}
	Migrations.MustRegister(up, down)
}
