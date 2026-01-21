package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Insert editor role
		_, err := db.Exec(`INSERT INTO roles (name, is_system) VALUES ('editor', TRUE)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Get editor role ID
		var editorRoleID int
		err = db.QueryRow(`SELECT id FROM roles WHERE name = 'editor'`).Scan(&editorRoleID)
		if err != nil {
			return errors.WithStack(err)
		}

		// Editor gets read+write on libraries, books, series, people
		// (same as viewer but with write permissions added)
		editorResources := []string{"libraries", "books", "series", "people"}
		operations := []string{"read", "write"}

		for _, resource := range editorResources {
			for _, operation := range operations {
				_, err = db.Exec(`INSERT INTO permissions (role_id, resource, operation) VALUES (?, ?, ?)`,
					editorRoleID, resource, operation)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Delete permissions for editor role first (foreign key constraint)
		_, err := db.Exec(`DELETE FROM permissions WHERE role_id = (SELECT id FROM roles WHERE name = 'editor')`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete editor role
		_, err = db.Exec(`DELETE FROM roles WHERE name = 'editor'`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
