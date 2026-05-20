package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

var Migrations = migrate.NewMigrations()

func NewMigrator(db *bun.DB) *migrate.Migrator {
	return newMigrator(db, Migrations)
}

func newMigrator(db *bun.DB, migrations *migrate.Migrations) *migrate.Migrator {
	return migrate.NewMigrator(db, migrations, migrate.WithMarkAppliedOnSuccess(true))
}

func BringUpToDate(ctx context.Context, db *bun.DB) (*migrate.MigrationGroup, error) {
	migrator := NewMigrator(db)
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
