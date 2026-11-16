package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Dismissing an unrecognized KOReader copy now only sets it aside:
		// its reading history stays held, to link later (ADR 0017).
		// Deleting that history is a step of its own, marked by
		// reading_deleted_at, after which the Device's data for it is
		// dropped. Every copy dismissed before had its history deleted, so
		// it's marked that way too.
		statements := []string{
			`ALTER TABLE koreader_unrecognized_books ADD COLUMN reading_deleted_at TIMESTAMP`,
			`UPDATE koreader_unrecognized_books SET reading_deleted_at = dismissed_at WHERE dismissed_at IS NOT NULL`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		// A copy dismissed with its reading still held goes back to waiting,
		// since a dismissal used to drop its reading.
		statements := []string{
			`UPDATE koreader_unrecognized_books SET dismissed_at = NULL WHERE reading_deleted_at IS NULL`,
			`ALTER TABLE koreader_unrecognized_books DROP COLUMN reading_deleted_at`,
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
