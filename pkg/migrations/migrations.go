package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

var Migrations = migrate.NewMigrations()

func BringUpToDate(ctx context.Context, db *bun.DB) (*migrate.MigrationGroup, error) {
	migrator := migrate.NewMigrator(db, Migrations)
	err := migrator.Init(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	group, err := migrator.Migrate(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return group, nil
}
