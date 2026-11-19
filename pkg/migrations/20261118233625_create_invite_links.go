package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// An Invite Link goes with its creator and its role. The Libraries it
		// grants are rows in invite_link_libraries, so deleting a Library
		// drops it from every pending invite. The user who accepted it keeps
		// the row when deleted, since the invite stays used.
		statements := []string{
			`CREATE TABLE invite_links (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				token TEXT NOT NULL,
				created_by_user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
				role_id INTEGER NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
				all_library_access BOOLEAN NOT NULL DEFAULT FALSE,
				expires_at TIMESTAMPTZ NOT NULL,
				used_at TIMESTAMPTZ,
				used_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
				revoked_at TIMESTAMPTZ
			)`,
			`CREATE UNIQUE INDEX ux_invite_links_token ON invite_links (token)`,
			`CREATE INDEX ix_invite_links_created_by_user_id ON invite_links (created_by_user_id)`,
			`CREATE INDEX ix_invite_links_role_id ON invite_links (role_id)`,
			`CREATE INDEX ix_invite_links_used_by_user_id ON invite_links (used_by_user_id)`,
			`CREATE TABLE invite_link_libraries (
				invite_link_id INTEGER NOT NULL REFERENCES invite_links (id) ON DELETE CASCADE,
				library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
				PRIMARY KEY (invite_link_id, library_id)
			)`,
			`CREATE INDEX ix_invite_link_libraries_library_id ON invite_link_libraries (library_id)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		for _, statement := range []string{
			`DROP TABLE IF EXISTS invite_link_libraries`,
			`DROP TABLE IF EXISTS invite_links`,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
