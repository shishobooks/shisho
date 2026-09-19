package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// Replace only the column, not the table, so child rows, foreign keys,
			// and every existing books index survive. No index uses author_source.
			for _, query := range []string{
				"ALTER TABLE books ADD COLUMN author_source_nullable TEXT",
				"UPDATE books SET author_source_nullable = author_source",
				"ALTER TABLE books DROP COLUMN author_source",
				"ALTER TABLE books RENAME COLUMN author_source_nullable TO author_source",
			} {
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return errors.Wrap(err, "failed to make author source nullable")
				}
			}
			return nil
		})
	}
	down := func(context.Context, *bun.DB) error {
		// A source-free collection has no source to restore. Keep nullability on
		// rollback rather than inventing provenance or breaking cleared books.
		return nil
	}
	Migrations.MustRegister(up, down)
}
