package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/uptrace/bun"
)

// Before pkg/sqliteconn, PRAGMA foreign_keys was set once at startup.
// database/sql replaces a connection whose query is canceled mid-statement,
// and the replacement ran with foreign keys off until the process restarted.
// Deletes in that window skipped their ON DELETE actions: deleting a Book
// left its files, authors, and series links behind, and the orphaned file row
// kept its path claimed so a reimport of the same file could not be organized.
//
// This applies each violated foreign key's ON DELETE action by hand. Any
// delete could have run in that window, so it works from PRAGMA
// foreign_key_check rather than a fixed table list.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		var repaired, remaining logger.Data
		err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			var err error
			repaired, remaining, err = repairForeignKeyViolations(ctx, tx)
			if err != nil {
				return err
			}
			if len(repaired) == 0 {
				return nil
			}
			// Deleting a Book's children changes what other rows index (a
			// Series lists its Books' titles), and the search service never
			// saw these deletes, so rebuild every search table.
			return rebuildFTSTablesInTx(ctx, tx)
		})
		if err != nil {
			return err
		}
		// Logged only after commit, so a rolled back attempt never reports
		// repairs that did not stick.
		if len(repaired) > 0 {
			logger.New().Warn("repaired rows that referenced a missing parent", repaired)
		}
		if len(remaining) > 0 {
			logger.New().Warn("rows still reference a missing parent after repair, counted per table (see PRAGMA foreign_key_check)", remaining)
		}
		return nil
	}

	// Down is a no-op: the deleted rows pointed at parents that no longer
	// exist, and nothing reads them.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}

type foreignKeyViolation struct {
	Table string `bun:"table"`
	FKID  int    `bun:"fkid"`
}

type foreignKeyColumn struct {
	Parent   string  `bun:"table"`
	From     string  `bun:"from"`
	To       *string `bun:"to"`
	OnDelete string  `bun:"on_delete"`
}

// maxRepairPasses bounds the loop. Each pass removes a level of the foreign
// key graph (files, then the files' children when enforcement is off), and
// the deepest chain in the schema is a handful of tables.
const maxRepairPasses = 10

// repairForeignKeyViolations deletes rows whose CASCADE parent is missing and
// clears columns whose SET NULL parent is missing, repeating until a pass
// changes nothing. It returns the rows changed per table and column, and the
// violations still left per table: ones whose foreign key has no action that
// removes them (RESTRICT, NO ACTION, SET DEFAULT; users.role_id is the only
// one, and roles.Service.Delete refuses to delete a role users hold), or
// ones the repair could not clear.
func repairForeignKeyViolations(ctx context.Context, tx bun.Tx) (repaired, remaining logger.Data, err error) {
	repaired = logger.Data{}
	for pass := 0; pass < maxRepairPasses; pass++ {
		var violations []foreignKeyViolation
		if err := tx.NewRaw(`SELECT DISTINCT "table", fkid FROM pragma_foreign_key_check ORDER BY "table", fkid`).Scan(ctx, &violations); err != nil {
			return nil, nil, errors.Wrap(err, "list foreign key violations")
		}

		progressed := false
		for _, violation := range violations {
			label, affected, err := repairForeignKey(ctx, tx, violation)
			if err != nil {
				return nil, nil, err
			}
			if affected > 0 {
				prior, _ := repaired[label].(int64)
				repaired[label] = prior + affected
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}

	var left []struct {
		Table string `bun:"table"`
		Count int    `bun:"count"`
	}
	if err := tx.NewRaw(`SELECT "table", COUNT(*) AS count FROM pragma_foreign_key_check GROUP BY "table" ORDER BY "table"`).Scan(ctx, &left); err != nil {
		return nil, nil, errors.Wrap(err, "count remaining foreign key violations")
	}
	remaining = logger.Data{}
	for _, row := range left {
		remaining[row.Table] = row.Count
	}
	return repaired, remaining, nil
}

// repairForeignKey applies one foreign key's ON DELETE action to the rows
// whose parent is missing. It changes nothing when the action does not
// remove the violation.
func repairForeignKey(ctx context.Context, tx bun.Tx, violation foreignKeyViolation) (string, int64, error) {
	var columns []foreignKeyColumn
	if err := tx.NewRaw(
		`SELECT "table", "from", "to", on_delete FROM pragma_foreign_key_list(?) WHERE id = ? ORDER BY seq`,
		violation.Table, violation.FKID,
	).Scan(ctx, &columns); err != nil {
		return "", 0, errors.Wrapf(err, "read foreign key %d on %s", violation.FKID, violation.Table)
	}
	if len(columns) == 0 {
		return "", 0, errors.Errorf("foreign key %d on %s not found", violation.FKID, violation.Table)
	}

	parent := columns[0].Parent
	parentKey, err := parentKeyColumns(ctx, tx, parent, columns)
	if err != nil {
		return "", 0, err
	}

	fromNames := make([]string, len(columns))
	notNull := make([]string, len(columns))
	matches := make([]string, len(columns))
	for i, column := range columns {
		fromNames[i] = column.From
		notNull[i] = fmt.Sprintf(`c.%q IS NOT NULL`, column.From)
		matches[i] = fmt.Sprintf(`p.%q = c.%q`, parentKey[i], column.From)
	}
	label := violation.Table + "." + strings.Join(fromNames, ",")
	missingParent := fmt.Sprintf(
		`%s AND NOT EXISTS (SELECT 1 FROM %q p WHERE %s)`,
		strings.Join(notNull, " AND "), parent, strings.Join(matches, " AND "),
	)

	var query string
	switch columns[0].OnDelete {
	case "CASCADE":
		label += "_deleted"
		query = fmt.Sprintf(`DELETE FROM %q AS c WHERE %s`, violation.Table, missingParent)
	case "SET NULL":
		label += "_cleared"
		sets := make([]string, len(columns))
		for i, column := range columns {
			sets[i] = fmt.Sprintf(`%q = NULL`, column.From)
		}
		query = fmt.Sprintf(`UPDATE %q AS c SET %s WHERE %s`, violation.Table, strings.Join(sets, ", "), missingParent)
	default:
		return label, 0, nil
	}

	result, err := tx.ExecContext(ctx, query)
	if err != nil {
		return "", 0, errors.Wrapf(err, "repair %s", label)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", 0, errors.WithStack(err)
	}
	return label, affected, nil
}

// parentKeyColumns returns the parent columns each child column refers to.
// A foreign key that names no parent columns refers to the primary key.
func parentKeyColumns(ctx context.Context, tx bun.Tx, parent string, columns []foreignKeyColumn) ([]string, error) {
	keys := make([]string, len(columns))
	explicit := true
	for i, column := range columns {
		if column.To == nil || *column.To == "" {
			explicit = false
			break
		}
		keys[i] = *column.To
	}
	if explicit {
		return keys, nil
	}

	var primaryKey []string
	if err := tx.NewRaw(`SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk`, parent).Scan(ctx, &primaryKey); err != nil {
		return nil, errors.Wrapf(err, "read the primary key of %s", parent)
	}
	if len(primaryKey) != len(columns) {
		return nil, errors.Errorf("foreign key into %s has %d columns but its primary key has %d", parent, len(columns), len(primaryKey))
	}
	return primaryKey, nil
}
