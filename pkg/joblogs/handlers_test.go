package joblogs

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	// Pin to a single connection so writes are visible to subsequent reads (a
	// bare :memory: DSN gives each pooled connection its own empty database).
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// TestListLogs_ResponseUsesItemsTotalEnvelopeWithoutJob asserts the joblogs list
// response returns the standard { items, total } envelope and no longer bundles
// the job. The job is now fetched separately by the client via GET /jobs/:id.
func TestListLogs_ResponseUsesItemsTotalEnvelopeWithoutJob(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	// Seed a job and two logs for it.
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	require.NoError(t, jobs.NewService(db).CreateJob(ctx, job))

	svc := NewService(db)
	require.NoError(t, svc.CreateJobLog(ctx, &models.JobLog{JobID: job.ID, Level: models.LogLevelInfo, Message: "first"}))
	require.NoError(t, svc.CreateJobLog(ctx, &models.JobLog{JobID: job.ID, Level: models.LogLevelWarn, Message: "second"}))

	h := &handler{jobLogService: svc, jobService: jobs.NewService(db)}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+strconv.Itoa(job.ID)+"/logs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(job.ID))

	require.NoError(t, h.listLogs(c))
	require.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only — no "job", no "logs".
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasJob := raw["job"]
	_, hasLogs := raw["logs"]
	assert.True(t, hasItems, "response must have 'items' key")
	assert.True(t, hasTotal, "response must have 'total' key")
	assert.False(t, hasJob, "response must NOT bundle the 'job' anymore")
	assert.False(t, hasLogs, "response must NOT use legacy 'logs' key")
	assert.Len(t, raw, 2, "response must have exactly 'items' and 'total' keys")

	var resp struct {
		Items []struct {
			ID      int    `json:"id"`
			Message string `json:"message"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, "first", resp.Items[0].Message)
}
