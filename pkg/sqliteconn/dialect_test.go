package sqliteconn

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type nulRow struct {
	bun.BaseModel `bun:"table:nul_rows"`

	ID    int64  `bun:",pk,autoincrement"`
	Title string `bun:",notnull"`
}

// Metadata read from files, such as M4B tags and PDF Info strings, can carry
// NUL bytes. Writing it must store the text without them rather than fail.
func TestNewDialect_StripsNULFromStrings(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	connector, err := NewConnector("file::memory:")
	require.NoError(t, err)
	sqldb := sql.OpenDB(connector)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, NewDialect())
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.NewCreateTable().Model((*nulRow)(nil)).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&nulRow{Title: "The Hobbit\x00\x00"}).Exec(ctx)
	require.NoError(t, err)

	var got nulRow
	err = db.NewSelect().Model(&got).Where("title = ?", "The\x00 Hobbit").Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, "The Hobbit", got.Title)
}
