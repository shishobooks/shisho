package main

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"golang.org/x/crypto/bcrypt"
)

// The second account the flows sign in to: a viewer who can access the
// harness's Library until a flow takes that away, to switch accounts to and
// to see access answered (ADR 0023).
const (
	readerUsername = "harnessreader"
	readerPassword = "harness-reader-password"
)

// setReaderAccess gives the reader access to the harness's first Library,
// as it's seeded, or to none, writing the server's database as the web
// app's user editor would.
func setReaderAccess(ctx context.Context, path string, access bool) error {
	connector, err := sqliteconn.NewConnector("file:"+path, "PRAGMA busy_timeout = 5000", "PRAGMA foreign_keys = ON")
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_library_access WHERE user_id = (SELECT id FROM users WHERE username = ?)`, readerUsername); err != nil {
		return errors.WithStack(err)
	}
	if access {
		_, err := tx.ExecContext(ctx, `INSERT INTO user_library_access (user_id, library_id)
			SELECT u.id, l.id FROM users u, libraries l WHERE u.username = ? AND l.name = 'Harness Library'`, readerUsername)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return errors.WithStack(tx.Commit())
}

// cheapenPasswords re-hashes the flows' two accounts' passwords at bcrypt's
// lowest cost, in the server's database. The server reads the cost from the
// hash, so signing in still checks the password, but in a millisecond rather
// than the quarter second the server's own cost takes here, which a CI runner
// short of CPU stretches past the app's 30 s request timeout.
func cheapenPasswords(ctx context.Context, path string) error {
	connector, err := sqliteconn.NewConnector("file:"+path, "PRAGMA busy_timeout = 5000", "PRAGMA foreign_keys = ON")
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	for username, password := range map[string]string{harnessUsername: harnessPassword, readerUsername: readerPassword} {
		hash, err := cheapPasswordHash(password)
		if err != nil {
			return err
		}
		result, err := db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE username = ?`, hash, username)
		if err != nil {
			return errors.WithStack(err)
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return errors.Errorf("re-hash %s's password: no such user", username)
		}
	}
	return nil
}

// cheapPasswordHash hashes password at bcrypt's lowest cost.
func cheapPasswordHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return string(hash), errors.WithStack(err)
}
