package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestList_ResponseUsesItemsTotalEnvelope asserts that GET /jobs returns the
// standard { items, total } envelope (not the legacy { jobs, total } shape).
func TestList_ResponseUsesItemsTotalEnvelope(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	// Seed two jobs.
	for i := 0; i < 2; i++ {
		job := &models.Job{
			Type:       models.JobTypeScan,
			Status:     models.JobStatusPending,
			DataParsed: &models.JobScanData{},
		}
		require.NoError(t, NewService(db).CreateJob(ctx, job))
	}

	h := &handler{jobService: NewService(db), db: db}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/jobs?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.list(c))
	require.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasJobs := raw["jobs"]
	assert.True(t, hasItems, "list response must have 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.False(t, hasJobs, "list response must NOT use legacy 'jobs' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")

	var resp struct {
		Items []struct {
			ID int `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// At least the two jobs we seeded are present (other migration-seeded jobs
	// may also exist), and total counts them.
	assert.GreaterOrEqual(t, len(resp.Items), 2)
	assert.GreaterOrEqual(t, resp.Total, 2)
}
