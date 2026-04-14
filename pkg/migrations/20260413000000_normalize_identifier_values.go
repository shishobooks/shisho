package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Backfill existing file_identifiers rows to the canonical form that
		// new writes now use (identifiers.NormalizeValue). Without this, legacy
		// rows stored as e.g. "978-0-316-76948-8" would not match exact-value
		// lookups against newly written "9780316769488" rows. The (file_id,
		// type) unique constraint is unaffected because normalization never
		// changes the type column.
		rows, err := db.QueryContext(ctx, `SELECT id, type, value FROM file_identifiers`)
		if err != nil {
			return errors.WithStack(err)
		}
		type pendingUpdate struct {
			id       int
			newValue string
		}
		var updates []pendingUpdate
		for rows.Next() {
			var (
				id       int
				idType   string
				rawValue string
			)
			if err := rows.Scan(&id, &idType, &rawValue); err != nil {
				_ = rows.Close()
				return errors.WithStack(err)
			}
			normalized := identifiers.NormalizeValue(idType, rawValue)
			if normalized != rawValue {
				updates = append(updates, pendingUpdate{id: id, newValue: normalized})
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return errors.WithStack(err)
		}
		if err := rows.Close(); err != nil {
			return errors.WithStack(err)
		}

		for _, u := range updates {
			if _, err := db.ExecContext(ctx, `UPDATE file_identifiers SET value = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, u.newValue, u.id); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(_ context.Context, _ *bun.DB) error {
		// The normalized values are canonicalized representations; we cannot
		// reconstruct the original cosmetic formatting. No-op rollback.
		return nil
	}

	Migrations.MustRegister(up, down)
}
