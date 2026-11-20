package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// usernameKey20261119 is models.UsernameKey as it was when this migration
// was written, copied so a later change to that function can't change what
// this migration wrote.
func usernameKey20261119(username string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFD.String(username)))
}

// Adds users.username_key, the username with every letter's case folded
// (models.UsernameKey), under a unique index. users.username stays UNIQUE
// COLLATE NOCASE, which folds only English letters; the key is what makes
// "Émile" and "émile" one username.
//
// That old uniqueness let users in whose usernames differ only in the case
// of other letters. The upgrade can't fail on them or lock any of them out,
// so users whose keys collide get no key (NULL, which the unique index
// allows any number of). auth.MatchUsername still signs each of them in by
// their exact username, and users.Service refuses a new username that folds
// to theirs.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return errors.WithStack(db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE users ADD COLUMN username_key TEXT`); err != nil {
				return errors.WithStack(err)
			}

			var users []struct {
				ID       int    `bun:"id"`
				Username string `bun:"username"`
			}
			if err := tx.NewRaw(`SELECT id, username FROM users`).Scan(ctx, &users); err != nil {
				return errors.WithStack(err)
			}
			byKey := map[string][]int{}
			for _, user := range users {
				key := usernameKey20261119(user.Username)
				byKey[key] = append(byKey[key], user.ID)
			}
			for key, ids := range byKey {
				if len(ids) > 1 {
					continue
				}
				if _, err := tx.ExecContext(ctx, `UPDATE users SET username_key = ? WHERE id = ?`, key, ids[0]); err != nil {
					return errors.WithStack(err)
				}
			}

			_, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX ux_users_username_key ON users (username_key)`)
			return errors.WithStack(err)
		}))
	}

	down := func(ctx context.Context, db *bun.DB) error {
		for _, statement := range []string{
			`DROP INDEX IF EXISTS ux_users_username_key`,
			`ALTER TABLE users DROP COLUMN username_key`,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
