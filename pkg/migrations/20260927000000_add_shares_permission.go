package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Share Links expose a book outside the deployment, so only the Admin
		// role holds the shares permission by default.
		_, err := db.ExecContext(ctx, `
			INSERT INTO permissions (role_id, resource, operation)
			SELECT id, 'shares', operation
			FROM roles, (SELECT 'read' AS operation UNION ALL SELECT 'write')
			WHERE name = 'admin'
		`)
		return errors.WithStack(err)
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DELETE FROM permissions WHERE resource = 'shares'`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
