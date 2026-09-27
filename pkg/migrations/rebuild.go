package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Helpers for SQLite table-rebuild migrations. SQLite cannot ALTER a column's
// type or constraints, so a migration creates "<table>_new", copies the rows,
// drops the old table, and renames the new one. Wrap every rebuild in
// withForeignKeysOff, run rebuildTableInTx per table, then checkForeignKeys.

// withForeignKeysOff runs fn in a transaction on one pinned connection with
// foreign key enforcement off, then restores the connection's prior setting.
// PRAGMA foreign_keys is a no-op inside a transaction, so it is switched
// before BEGIN. With enforcement on, DROP TABLE on a parent table runs an
// implicit DELETE that fires ON DELETE CASCADE into every child table.
func withForeignKeysOff(ctx context.Context, db *bun.DB, fn func(context.Context, bun.Tx) error) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()

	var enabled int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return errors.Wrap(err, "read foreign_keys pragma")
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return errors.Wrap(err, "disable foreign keys")
	}
	defer func() {
		if enabled == 0 {
			return
		}
		if _, restoreErr := conn.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON"); restoreErr != nil && err == nil {
			err = errors.Wrap(restoreErr, "re-enable foreign keys")
		}
	}()

	return conn.RunInTx(ctx, nil, fn)
}

// rebuildTableInTx replaces table with the definition in createSQL, which
// must create "<table>_new". It keeps every row, every index and trigger
// that existed on the table (read from sqlite_master, so ones added by any
// earlier migration survive), and the AUTOINCREMENT high-water mark so
// deleted ids are not reused. The copy fails if the old and new column sets
// differ, which catches a column added to the table after createSQL was
// written.
func rebuildTableInTx(ctx context.Context, tx bun.Tx, table, createSQL string) error {
	newTable := table + "_new"

	var schemaSQL []string
	if err := tx.NewRaw(
		`SELECT sql FROM sqlite_master
		WHERE tbl_name = ? AND type IN ('index', 'trigger') AND sql IS NOT NULL
		ORDER BY type = 'trigger', name`,
		table,
	).Scan(ctx, &schemaSQL); err != nil {
		return errors.Wrapf(err, "list indexes and triggers on %s", table)
	}

	var oldSeq sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT seq FROM sqlite_sequence WHERE name = ?", table).Scan(&oldSeq); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.Wrapf(err, "read sqlite_sequence for %s", table)
	}

	oldColumns, err := tableColumns(ctx, tx, table)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, createSQL); err != nil {
		return errors.Wrapf(err, "create %s", newTable)
	}

	newColumns, err := tableColumns(ctx, tx, newTable)
	if err != nil {
		return err
	}
	if !slices.Equal(sortedCopy(oldColumns), sortedCopy(newColumns)) {
		return errors.Errorf("%s columns %v do not match %s columns %v", table, oldColumns, newTable, newColumns)
	}

	quoted := make([]string, len(oldColumns))
	for i, column := range oldColumns {
		quoted[i] = `"` + column + `"`
	}
	columnList := strings.Join(quoted, ", ")
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s", newTable, columnList, columnList, table,
	)); err != nil {
		return errors.Wrapf(err, "copy %s into %s", table, newTable)
	}

	if _, err := tx.ExecContext(ctx, "DROP TABLE "+table); err != nil {
		return errors.Wrapf(err, "drop %s", table)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", newTable, table)); err != nil {
		return errors.Wrapf(err, "rename %s to %s", newTable, table)
	}

	for _, stmt := range schemaSQL {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return errors.Wrapf(err, "recreate on %s: %s", table, stmt)
		}
	}

	// DROP TABLE deleted the old sqlite_sequence row, and the copy only
	// advanced the new one to the highest surviving id.
	if oldSeq.Valid {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name = ?", table); err != nil {
			return errors.Wrapf(err, "reset sqlite_sequence for %s", table)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)", table, oldSeq.Int64); err != nil {
			return errors.Wrapf(err, "restore sqlite_sequence for %s", table)
		}
	}

	return nil
}

func tableColumns(ctx context.Context, tx bun.Tx, table string) ([]string, error) {
	var columns []string
	if err := tx.NewRaw("SELECT name FROM pragma_table_info(?) ORDER BY cid", table).Scan(ctx, &columns); err != nil {
		return nil, errors.Wrapf(err, "list columns of %s", table)
	}
	if len(columns) == 0 {
		return nil, errors.Errorf("table %s has no columns", table)
	}
	return columns, nil
}

func sortedCopy(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return out
}

// checkForeignKeys fails the migration, rolling back the transaction, if any
// row in the rebuilt tables references a missing parent. The copy keeps every
// value, so a violation here is a row that was already broken and that the
// migration did not repair.
func checkForeignKeys(ctx context.Context, tx bun.Tx, tables []string) error {
	for _, table := range tables {
		var violations []struct {
			Table  string        `bun:"table"`
			RowID  sql.NullInt64 `bun:"rowid"`
			Parent string        `bun:"parent"`
		}
		if err := tx.NewRaw(`SELECT "table", rowid, parent FROM pragma_foreign_key_check(?)`, table).Scan(ctx, &violations); err != nil {
			return errors.Wrapf(err, "check foreign keys on %s", table)
		}
		if len(violations) > 0 {
			return errors.Errorf(
				"%d foreign key violations in %s (first: rowid %d references a missing %s row); fix or delete those rows and restart",
				len(violations), table, violations[0].RowID.Int64, violations[0].Parent,
			)
		}
	}
	return nil
}
