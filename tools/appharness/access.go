package main

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
)

// The second account the flows sign in to: a viewer who can access the
// harness's Library until a flow takes that away, to switch accounts to and
// to see access answered (ADR 0023).
const (
	readerUsername = "harnessreader"
	readerPassword = "harness-reader-password"
)

// setReaderAccess gives the reader access to every Library, or to none,
// writing the server's database as the web app's user editor would.
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
			SELECT u.id, l.id FROM users u, libraries l WHERE u.username = ?`, readerUsername)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return errors.WithStack(tx.Commit())
}
