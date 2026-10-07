package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// These tests enforce the schema rules in "Database" in the
// root AGENTS.md against the fully migrated schema. Each checker returns one
// message per violation; TestSchemaInvariantCheckers_CatchViolations runs them
// against a deliberately broken schema so a checker that silently stops
// reporting anything fails too.

// nonPluralTables are entity tables whose names do not end in "s" but are
// plural anyway (a "children" table, say). Keep this list short and give each
// entry a reason. The check is only a suffix test: uncountable names such as
// user_library_access and series already pass.
var nonPluralTables = map[string]string{}

// fkWithoutOnDelete lists foreign keys, as "table(col, ...)", that are allowed
// to keep SQLite's default NO ACTION. Empty: every foreign key must say what
// happens when its parent row is deleted.
var fkWithoutOnDelete = map[string]string{}

// unindexedFKs lists foreign keys, as "table(col, ...)", that are allowed to
// have no index leading with their columns.
var unindexedFKs = map[string]string{}

// skippedTables are tables the plural-name rule does not apply to because
// Shisho does not name them.
var skippedTables = map[string]string{
	"bun_migrations":      "created by Bun's migrator",
	"bun_migration_locks": "created by Bun's migrator",
}

type schemaTable struct {
	name    string
	virtual bool
}

func listTables(ctx context.Context, t *testing.T, db *bun.DB) []schemaTable {
	t.Helper()
	rows, err := db.QueryContext(ctx,
		`SELECT name, COALESCE(sql, '') FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()

	var all []schemaTable
	for rows.Next() {
		var name, ddl string
		require.NoError(t, rows.Scan(&name, &ddl))
		all = append(all, schemaTable{
			name:    name,
			virtual: strings.HasPrefix(strings.ToUpper(strings.TrimSpace(ddl)), "CREATE VIRTUAL TABLE"),
		})
	}
	require.NoError(t, rows.Err())

	// Drop the shadow tables a virtual table (FTS5) creates for itself, such
	// as books_fts_data and books_fts_config, along with the virtual tables.
	var virtualNames []string
	for _, tbl := range all {
		if tbl.virtual {
			virtualNames = append(virtualNames, tbl.name)
		}
	}
	var tables []schemaTable
	for _, tbl := range all {
		shadow := false
		for _, v := range virtualNames {
			if strings.HasPrefix(tbl.name, v+"_") {
				shadow = true
				break
			}
		}
		if !shadow {
			tables = append(tables, tbl)
		}
	}
	return tables
}

type foreignKey struct {
	table    string
	parent   string
	columns  []string
	onDelete string
}

func (fk foreignKey) key() string {
	return fk.table + "(" + strings.Join(fk.columns, ", ") + ")"
}

func listForeignKeys(ctx context.Context, t *testing.T, db *bun.DB, table string) []foreignKey {
	t.Helper()
	rows, err := db.QueryContext(ctx,
		`SELECT id, "table", "from", on_delete FROM pragma_foreign_key_list(?) ORDER BY id, seq`, table)
	require.NoError(t, err)
	defer rows.Close()

	byID := map[int]*foreignKey{}
	var ids []int
	for rows.Next() {
		var (
			id                     int
			parent, from, onDelete string
		)
		require.NoError(t, rows.Scan(&id, &parent, &from, &onDelete))
		fk, ok := byID[id]
		if !ok {
			fk = &foreignKey{table: table, parent: parent, onDelete: onDelete}
			byID[id] = fk
			ids = append(ids, id)
		}
		fk.columns = append(fk.columns, from)
	}
	require.NoError(t, rows.Err())

	fks := make([]foreignKey, 0, len(ids))
	for _, id := range ids {
		fks = append(fks, *byID[id])
	}
	return fks
}

// indexPrefixes returns the column lists of every non-partial index on table,
// including the autoindexes behind PRIMARY KEY and UNIQUE constraints, plus
// the INTEGER PRIMARY KEY column, which is the rowid and needs no index.
func indexPrefixes(ctx context.Context, t *testing.T, db *bun.DB, table string) [][]string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT name, partial FROM pragma_index_list(?)`, table)
	require.NoError(t, err)
	var names []string
	for rows.Next() {
		var (
			name    string
			partial int
		)
		require.NoError(t, rows.Scan(&name, &partial))
		// A partial index only covers some rows, so it cannot serve every
		// lookup of children by parent.
		if partial == 0 {
			names = append(names, name)
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	var prefixes [][]string
	for _, name := range names {
		cols, err := db.QueryContext(ctx, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, name)
		require.NoError(t, err)
		var columns []string
		for cols.Next() {
			var col sql.NullString
			require.NoError(t, cols.Scan(&col))
			// Expression columns have no name; nothing after them can count
			// as a plain column prefix.
			if !col.Valid {
				break
			}
			columns = append(columns, col.String)
		}
		require.NoError(t, cols.Err())
		require.NoError(t, cols.Close())
		prefixes = append(prefixes, columns)
	}

	info, err := db.QueryContext(ctx, `SELECT name, type, pk FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer info.Close()
	var pkCols []string
	var pkType string
	for info.Next() {
		var (
			name, typ string
			pk        int
		)
		require.NoError(t, info.Scan(&name, &typ, &pk))
		if pk > 0 {
			pkCols = append(pkCols, name)
			pkType = typ
		}
	}
	require.NoError(t, info.Err())
	if len(pkCols) == 1 && strings.EqualFold(pkType, "INTEGER") {
		prefixes = append(prefixes, pkCols)
	}
	return prefixes
}

// hasLeadingIndex reports whether some index starts with exactly the foreign
// key's columns, in any order (equality lookups on all of them can use it).
func hasLeadingIndex(fkCols []string, prefixes [][]string) bool {
	want := append([]string(nil), fkCols...)
	sort.Strings(want)
	for _, cols := range prefixes {
		if len(cols) < len(want) {
			continue
		}
		got := append([]string(nil), cols[:len(want)]...)
		sort.Strings(got)
		if strings.Join(got, "\x00") == strings.Join(want, "\x00") {
			return true
		}
	}
	return false
}

func checkForeignKeyOnDelete(ctx context.Context, t *testing.T, db *bun.DB) []string {
	t.Helper()
	var violations []string
	for _, tbl := range listTables(ctx, t, db) {
		if tbl.virtual {
			continue
		}
		for _, fk := range listForeignKeys(ctx, t, db, tbl.name) {
			if fk.onDelete != "NO ACTION" {
				continue
			}
			if _, ok := fkWithoutOnDelete[fk.key()]; ok {
				continue
			}
			violations = append(violations, fmt.Sprintf(
				"%s references %s with no ON DELETE action: add ON DELETE CASCADE (child rows meaningless without the parent) or ON DELETE SET NULL (nullable reference that should survive) in a migration",
				fk.key(), fk.parent))
		}
	}
	return violations
}

func checkForeignKeyIndexes(ctx context.Context, t *testing.T, db *bun.DB) []string {
	t.Helper()
	var violations []string
	for _, tbl := range listTables(ctx, t, db) {
		if tbl.virtual {
			continue
		}
		fks := listForeignKeys(ctx, t, db, tbl.name)
		if len(fks) == 0 {
			continue
		}
		prefixes := indexPrefixes(ctx, t, db, tbl.name)
		for _, fk := range fks {
			if hasLeadingIndex(fk.columns, prefixes) {
				continue
			}
			if _, ok := unindexedFKs[fk.key()]; ok {
				continue
			}
			violations = append(violations, fmt.Sprintf(
				"%s references %s but no index on %s leads with those columns: add CREATE INDEX ix_%s_%s ON %s (%s) in a migration (a composite index that leads with another column does not count)",
				fk.key(), fk.parent, tbl.name, tbl.name, strings.Join(fk.columns, "_"), tbl.name, strings.Join(fk.columns, ", ")))
		}
	}
	return violations
}

func checkPluralTableNames(ctx context.Context, t *testing.T, db *bun.DB) []string {
	t.Helper()
	var violations []string
	for _, tbl := range listTables(ctx, t, db) {
		if tbl.virtual || strings.HasSuffix(tbl.name, "s") {
			continue
		}
		if _, ok := skippedTables[tbl.name]; ok {
			continue
		}
		if _, ok := nonPluralTables[tbl.name]; ok {
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"table %q is not plural: table names must be plural (plugins, plugin_configs); rename it, or add it to nonPluralTables with a reason if the name is plural without ending in s",
			tbl.name))
	}
	return violations
}

func TestSchema_ForeignKeysHaveOnDelete(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	assert.Empty(t, checkForeignKeyOnDelete(context.Background(), t, db))
}

func TestSchema_ForeignKeysAreIndexed(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	assert.Empty(t, checkForeignKeyIndexes(context.Background(), t, db))
}

func TestSchema_TableNamesArePlural(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	assert.Empty(t, checkPluralTableNames(context.Background(), t, db))
}

// TestSchemaInvariantCheckers_CatchViolations proves each checker reports a
// violation it is meant to catch, and accepts the shapes it is meant to
// accept, so the schema tests above cannot pass by checking nothing.
func TestSchemaInvariantCheckers_CatchViolations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connector, err := sqliteconn.NewConnector(":memory:", "PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	sqldb := sql.OpenDB(connector)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE parents (id INTEGER PRIMARY KEY)`,
		// Violates ON DELETE and the index rule.
		`CREATE TABLE orphans (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents (id))`,
		// Indexed only by a composite index that leads with another column.
		`CREATE TABLE misindexed_rows (id INTEGER PRIMARY KEY, other INTEGER, parent_id INTEGER REFERENCES parents (id) ON DELETE CASCADE)`,
		`CREATE INDEX ix_misindexed ON misindexed_rows (other, parent_id)`,
		// Fine: a plain index, a composite index leading with the key, and a
		// composite primary key leading with the key.
		`CREATE TABLE good_rows (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents (id) ON DELETE SET NULL)`,
		`CREATE INDEX ix_good ON good_rows (parent_id)`,
		`CREATE TABLE composite_rows (parent_id INTEGER REFERENCES parents (id) ON DELETE CASCADE, other INTEGER, PRIMARY KEY (parent_id, other))`,
		`CREATE TABLE leading_rows (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents (id) ON DELETE RESTRICT, other INTEGER)`,
		`CREATE INDEX ix_leading ON leading_rows (parent_id, other)`,
		// Not plural.
		`CREATE TABLE child (id INTEGER PRIMARY KEY)`,
		// FTS virtual table and its shadow tables are skipped.
		`CREATE VIRTUAL TABLE things_fts USING fts5 (name)`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}

	onDelete := checkForeignKeyOnDelete(ctx, t, db)
	require.Len(t, onDelete, 1, onDelete)
	assert.Contains(t, onDelete[0], "orphans(parent_id)")

	indexes := checkForeignKeyIndexes(ctx, t, db)
	require.Len(t, indexes, 2, indexes)
	assert.Contains(t, indexes[0], "misindexed_rows(parent_id)")
	assert.Contains(t, indexes[1], "orphans(parent_id)")

	plural := checkPluralTableNames(ctx, t, db)
	require.Len(t, plural, 1, plural)
	assert.Contains(t, plural[0], `"child"`)
}

// lowerNamePattern matches SQL that wraps a name column in LOWER(), such as
// `LOWER(name)` or `lower(p.sort_name)`. The word boundary keeps Go's
// strings.ToLower(x.Name) from matching.
var lowerNamePattern = regexp.MustCompile(`(?i)\blower\(\s*(\w+\.)?\w*name\s*\)`)

// TestSource_NoLowerNameComparisons enforces the root AGENTS.md rule that
// case-insensitive name lookups use `name = ? COLLATE NOCASE`, which can
// search the (name COLLATE NOCASE, library_id) unique indexes, and never
// `LOWER(name) = LOWER(?)`, which scans. Migrations are skipped: they run
// once, and rewriting applied migrations changes nothing.
func TestSource_NoLowerNameComparisons(t *testing.T) {
	t.Parallel()

	pkgDir, err := filepath.Abs("..")
	require.NoError(t, err)

	var hits []string
	err = filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "migrations" || d.Name() == "testdata" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if lowerNamePattern.MatchString(line) {
				rel, _ := filepath.Rel(pkgDir, path)
				hits = append(hits, fmt.Sprintf("pkg/%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, hits,
		"case-insensitive name lookups must use `name = ? COLLATE NOCASE` (keeping `library_id = ?` where library-scoped), not LOWER(name) = LOWER(?), which cannot use the NOCASE unique index")
}

func TestLowerNamePattern(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		`WHERE LOWER(name) = LOWER(?)`,
		`JOIN publishers p ON LOWER(p.name) = LOWER(i.name)`,
		`where lower( sort_name ) = ?`,
	} {
		assert.True(t, lowerNamePattern.MatchString(s), s)
	}
	for _, s := range []string{
		`if strings.ToLower(file.Name) == "comicinfo.xml" {`,
		`WHERE name = ? COLLATE NOCASE`,
		`WHERE LOWER(title) = LOWER(?)`,
	} {
		assert.False(t, lowerNamePattern.MatchString(s), s)
	}
}
