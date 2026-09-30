package database

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// The tests in this file swap the global logger output, so none of them may
// run in parallel.

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs sends every logger built after this call to a buffer.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	original := logger.Output()
	buf := &syncBuffer{}
	logger.SetOutput(buf)
	t.Cleanup(func() { logger.SetOutput(original) })
	return buf
}

type widget struct {
	bun.BaseModel `bun:"table:widgets"`

	ID     int64  `bun:",pk,autoincrement"`
	Secret string `bun:",notnull"`
}

func openTestDB(t *testing.T, debug bool) *bun.DB {
	t.Helper()
	cfg := config.NewForTest()
	cfg.DatabaseDebug = debug
	db, err := New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNew_DatabaseDebugLogsQueries(t *testing.T) {
	logs := captureLogs(t)
	db := openTestDB(t, true)

	var got string
	require.NoError(t, db.NewSelect().ColumnExpr("? AS marker", "debug-marker").Scan(context.Background(), &got))

	assert.Contains(t, logs.String(), "SELECT 'debug-marker' AS marker")
	assert.Contains(t, logs.String(), `"level":"debug"`)
}

func TestNew_DatabaseDebugOffLogsNoQueries(t *testing.T) {
	logs := captureLogs(t)
	db := openTestDB(t, false)

	var got string
	require.NoError(t, db.NewSelect().ColumnExpr("? AS marker", "debug-marker").Scan(context.Background(), &got))

	// Only the debug tier would log this; a slow-query warning (possible on a
	// loaded runner) names the operation but never the SQL.
	assert.NotContains(t, logs.String(), "debug-marker")
	assert.NotContains(t, logs.String(), "sql query")
	assert.NotContains(t, logs.String(), `"level":"debug"`)
}

func TestQueryLogger_TruncatesLongStatements(t *testing.T) {
	logs := captureLogs(t)
	db := openTestDB(t, true)

	long := strings.Repeat("x", 5000)
	var got string
	require.NoError(t, db.NewSelect().ColumnExpr("? AS marker", long).Scan(context.Background(), &got))

	assert.Contains(t, logs.String(), strings.Repeat("x", 1000))
	assert.NotContains(t, logs.String(), long, "statements over 2 KB are truncated")
}

// sleepHook delays one INSERT so the query logger sees it as slow. bun sets
// the event's start time before calling any BeforeQuery hook.
type sleepHook struct{}

func (sleepHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	if event.Operation() == "INSERT" {
		time.Sleep(slowQueryThreshold + 50*time.Millisecond)
	}
	return ctx
}

func (sleepHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestNew_SlowQueryWarnsWithoutValues(t *testing.T) {
	logs := captureLogs(t)
	db := openTestDB(t, false)
	ctx := context.Background()

	_, err := db.NewCreateTable().Model((*widget)(nil)).Exec(ctx)
	require.NoError(t, err)
	db.AddQueryHook(sleepHook{})

	_, err = db.NewInsert().Model(&widget{Secret: "hunter2-password-hash"}).Exec(ctx)
	require.NoError(t, err)

	out := logs.String()
	assert.Contains(t, out, `"level":"warn"`)
	assert.Contains(t, out, "slow query")
	assert.Contains(t, out, `"operation":"INSERT"`)
	assert.Contains(t, out, `"table":"widgets"`)
	assert.Contains(t, out, `"duration_ms":`)
	assert.NotContains(t, out, "hunter2-password-hash", "slow query warnings must not include values")
	assert.NotContains(t, out, "INSERT INTO", "slow query warnings must not include SQL text")
}
