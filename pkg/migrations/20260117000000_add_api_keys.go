package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create api_keys table
		_, err := db.Exec(`
			CREATE TABLE api_keys (
				id TEXT PRIMARY KEY,
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				key TEXT NOT NULL UNIQUE,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL,
				last_accessed_at DATETIME
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_api_keys_user_id ON api_keys(user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_api_keys_key ON api_keys(key)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create api_key_permissions table
		_, err = db.Exec(`
			CREATE TABLE api_key_permissions (
				id TEXT PRIMARY KEY,
				api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
				permission TEXT NOT NULL,
				created_at DATETIME NOT NULL,
				UNIQUE(api_key_id, permission)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_api_key_permissions_api_key_id ON api_key_permissions(api_key_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create api_key_short_urls table
		_, err = db.Exec(`
			CREATE TABLE api_key_short_urls (
				id TEXT PRIMARY KEY,
				api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
				short_code TEXT NOT NULL UNIQUE,
				expires_at DATETIME NOT NULL,
				created_at DATETIME NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_short_urls_code ON api_key_short_urls(short_code)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			DROP TABLE IF EXISTS api_key_short_urls;
			DROP TABLE IF EXISTS api_key_permissions;
			DROP TABLE IF EXISTS api_keys;
		`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
