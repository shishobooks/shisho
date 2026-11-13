package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/demo"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seedDemoSession records one Reading Session ending at end.
func seedDemoSession(t *testing.T, db *bun.DB, end time.Time) {
	t.Helper()
	ctx := context.Background()
	user := &models.User{Username: "demo", PasswordHash: "test", RoleID: 2, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.ReadingSession{
		CreatedAt: end, UserID: user.ID, Source: models.ReadingSourceWebReader, StartedAt: end.Add(-time.Hour),
		EndedAt: end, Measured: true, Kind: models.ReadingKindReading, Duration: 3600,
	}).Exec(ctx)
	require.NoError(t, err)
}

func lastSessionDay(t *testing.T, db *bun.DB) string {
	t.Helper()
	var day string
	require.NoError(t, db.NewRaw(`SELECT substr(MAX(ended_at), 1, 10) FROM reading_sessions`).Scan(context.Background(), &day))
	return day
}

func TestNew_DemoModeMovesTheReadingToYesterdayBeforeServing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemoSession(t, db, time.Date(2025, 6, 1, 21, 0, 0, 0, time.UTC))
	cfg := config.NewForTest()
	cfg.DemoMode = true
	cfg.CacheDir = t.TempDir()

	_, err := New(cfg, db, &worker.Worker{}, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly), lastSessionDay(t, db))
}

// The demo's Machine suspends when idle and resumes without restarting, so
// the first request of each day moves the reading again.
func TestDemoDateShiftMiddleware_MovesTheReadingOnTheFirstRequestOfADay(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seedDemoSession(t, db, time.Date(2026, 3, 10, 21, 0, 0, 0, time.UTC))
	now := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	shifter := demo.NewDateShifter(db, func() time.Time { return now })

	e := echo.New()
	e.Use(demoDateShiftMiddleware(shifter))
	e.GET("/api/books", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	request := func() {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books", nil))
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	request()
	assert.Equal(t, "2026-11-12", lastSessionDay(t, db))
	now = now.AddDate(0, 0, 1)
	request()
	assert.Equal(t, "2026-11-13", lastSessionDay(t, db))
}
