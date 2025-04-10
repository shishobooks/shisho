package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/uptrace/bun/migrate"
	"github.com/urfave/cli/v2"
)

func main() {
	log := logger.New()

	cfg, err := config.New()
	if err != nil {
		log.Err(err).Fatal("config error")
	}

	db, err := database.New(cfg)
	if err != nil {
		log.Err(err).Fatal("database error")
	}

	app := &cli.App{
		Name:        "migrations",
		Usage:       "CLI to interact with migrations",
		Description: "CLI to interact with migrations",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "create migration tables",
				Action: func(c *cli.Context) error {
					migrator := migrate.NewMigrator(db, migrations.Migrations)
					return migrator.Init(c.Context)
				},
			},
			{
				Name:  "migrate",
				Usage: "migrate database",
				Action: func(c *cli.Context) error {
					migrator := migrate.NewMigrator(db, migrations.Migrations)

					group, err := migrator.Migrate(c.Context)
					if err != nil {
						return err
					}

					if group.ID == 0 {
						fmt.Printf("There are no new migrations to run\n")
						return nil
					}

					fmt.Printf("Migrated to %s\n", group)
					return nil
				},
			},
			{
				Name:  "rollback",
				Usage: "rollback the last migration group",
				Action: func(c *cli.Context) error {
					migrator := migrate.NewMigrator(db, migrations.Migrations)

					group, err := migrator.Rollback(c.Context)
					if err != nil {
						return err
					}

					if group.ID == 0 {
						fmt.Printf("There are no groups to roll back\n")
						return nil
					}

					fmt.Printf("Rolled back %s\n", group)
					return nil
				},
			},
			{
				Name:  "create",
				Usage: "create Go migration",
				Action: func(c *cli.Context) error {
					migrator := migrate.NewMigrator(db, migrations.Migrations)

					name := strings.Join(c.Args().Slice(), "_")
					mf, err := migrator.CreateGoMigration(
						c.Context,
						name,
						migrate.WithGoTemplate(migrationTemplate),
					)
					if err != nil {
						return err
					}
					fmt.Printf("Created migration %s (%s)\n", mf.Name, mf.Path)

					return nil
				},
			},
			{
				Name:  "status",
				Usage: "print migrations status",
				Action: func(c *cli.Context) error {
					migrator := migrate.NewMigrator(db, migrations.Migrations)

					ms, err := migrator.MigrationsWithStatus(c.Context)
					if err != nil {
						return err
					}
					fmt.Printf("Migrations: %s\n", ms)
					fmt.Printf("Unapplied migrations: %s\n", ms.Unapplied())
					fmt.Printf("Last migration group: %s\n", ms.LastGroup())

					return nil
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Err(err).Fatal("app run error")
	}
}

const migrationTemplate = `package %s

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("")
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
`
