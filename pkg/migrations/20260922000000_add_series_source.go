package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// books.series_source is aggregate provenance for a Book's ordered Series
// membership collection and its Series Number groups (ADR 0006). It never
// describes a Series resource's name; series.name_source keeps that meaning.
//
// Before this column existed the scanner used the first membership's
// series.name_source as the effective membership source. The backfill copies
// exactly that value so existing Books keep their current Scan protection
// without inventing history. Books without memberships stay NULL.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// Adding a column leaves the table, its indexes, and child rows intact.
			for _, query := range []string{
				"ALTER TABLE books ADD COLUMN series_source TEXT",
				`UPDATE books
				 SET series_source = (
				   SELECT s.name_source
				   FROM book_series bs
				   JOIN series s ON s.id = bs.series_id
				   WHERE bs.book_id = books.id
				   ORDER BY bs.sort_order ASC, bs.id ASC
				   LIMIT 1
				 )
				 WHERE EXISTS (SELECT 1 FROM book_series bs WHERE bs.book_id = books.id)`,
			} {
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return errors.Wrap(err, "failed to add series source")
				}
			}
			return nil
		})
	}
	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, "ALTER TABLE books DROP COLUMN series_source")
		return errors.Wrap(err, "failed to drop series source")
	}
	Migrations.MustRegister(up, down)
}
