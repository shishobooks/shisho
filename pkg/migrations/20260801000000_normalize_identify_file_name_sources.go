package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func normalizeIdentifyFileNameSources(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
		UPDATE files
		SET name_source = 'manual'
		WHERE name_source = 'user'
	`)
	return errors.WithStack(err)
}

func init() {
	up := normalizeIdentifyFileNameSources

	// This data migration is intentionally irreversible. After normalization,
	// historical Identify edits cannot be distinguished from other manual edits.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
