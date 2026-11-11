package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// GET /user/libraries gives every signed-in user what the reader
		// pages need from their libraries, so Libraries Read only showed the
		// viewer role Administration > Libraries and library folder paths.
		// System roles' permissions are editable, so delete only this row
		// and leave whatever else an admin set on the role.
		_, err := db.ExecContext(ctx, `
			DELETE FROM permissions
			WHERE resource = 'libraries' AND operation = 'read'
				AND role_id = (SELECT id FROM roles WHERE name = 'viewer' AND is_system = TRUE)
		`)
		return errors.WithStack(err)
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `
			INSERT OR IGNORE INTO permissions (role_id, resource, operation)
			SELECT id, 'libraries', 'read' FROM roles WHERE name = 'viewer' AND is_system = TRUE
		`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
