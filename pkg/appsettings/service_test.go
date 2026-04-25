package appsettings

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type sampleConfig struct {
	BookFields  []string `json:"book_fields"`
	AudioFields []string `json:"audio_fields"`
}

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Enable foreign keys to match production behavior
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestGetSetJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	svc := NewService(db)

	want := sampleConfig{
		BookFields:  []string{"authors", "description"},
		AudioFields: []string{"narrators"},
	}
	require.NoError(t, svc.SetJSON(ctx, "review_criteria", want))

	var got sampleConfig
	ok, err := svc.GetJSON(ctx, "review_criteria", &got)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestGetJSON_Missing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	svc := NewService(db)

	var got sampleConfig
	ok, err := svc.GetJSON(ctx, "missing_key", &got)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSetJSON_Overwrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	svc := NewService(db)

	require.NoError(t, svc.SetJSON(ctx, "k", sampleConfig{BookFields: []string{"a"}}))
	require.NoError(t, svc.SetJSON(ctx, "k", sampleConfig{BookFields: []string{"b"}}))

	var got sampleConfig
	_, err := svc.GetJSON(ctx, "k", &got)
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, got.BookFields)
}
