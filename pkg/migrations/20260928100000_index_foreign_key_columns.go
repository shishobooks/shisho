package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Index foreign key columns that had no index leading with the column.
// Without one, reading a resource's aliases (search reindexing, merges, alias
// lists) and the ON DELETE action on every parent delete scan the child
// table. The alias tables' library_id, api_key_short_urls.api_key_id, and the
// plugin tables' composite keys are left out: their tables are small or only
// a rare delete touches the column.
func init() {
	columns := []struct{ table, column string }{
		{"person_aliases", "person_id"},
		{"series_aliases", "series_id"},
		{"genre_aliases", "genre_id"},
		{"tag_aliases", "tag_id"},
		{"publisher_aliases", "publisher_id"},
		{"publishers", "parent_id"},
		{"jobs", "library_id"},
		{"list_books", "added_by_user_id"},
		{"list_shares", "shared_by_user_id"},
		{"user_library_settings", "library_id"},
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, c := range columns {
				query := "CREATE INDEX ix_" + c.table + "_" + c.column + " ON " + c.table + " (" + c.column + ")"
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return errors.Wrapf(err, "failed to index %s.%s", c.table, c.column)
				}
			}
			return nil
		})
	}
	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, c := range columns {
				if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS ix_"+c.table+"_"+c.column); err != nil {
					return errors.Wrapf(err, "failed to drop index on %s.%s", c.table, c.column)
				}
			}
			return nil
		})
	}
	Migrations.MustRegister(up, down)
}
