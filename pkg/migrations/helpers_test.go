package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// Schema inspection helpers shared by migration tests.

type foreignKey struct {
	From     string `bun:"from"`
	Table    string `bun:"table"`
	To       string `bun:"to"`
	OnDelete string `bun:"on_delete"`
}

func foreignKeys(ctx context.Context, t *testing.T, db *bun.DB, table string) []foreignKey {
	t.Helper()
	var fks []foreignKey
	require.NoError(t, db.NewRaw(`SELECT "from", "table", "to", on_delete FROM pragma_foreign_key_list(?)`, table).Scan(ctx, &fks))
	return fks
}

func columnDecl(ctx context.Context, t *testing.T, db *bun.DB, table, column string) (string, bool) {
	t.Helper()
	var info struct {
		Type    string `bun:"type"`
		NotNull bool   `bun:"column:notnull"`
	}
	require.NoError(t, db.NewRaw("SELECT type, \"notnull\" FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(ctx, &info))
	return info.Type, info.NotNull
}

func columnNames(ctx context.Context, t *testing.T, db *bun.DB, table string) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.NewRaw("SELECT name FROM pragma_table_info(?) ORDER BY cid", table).Scan(ctx, &names))
	return names
}

func foreignKeyViolations(ctx context.Context, t *testing.T, db *bun.DB) int {
	t.Helper()
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	require.NoError(t, err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())
	return count
}
