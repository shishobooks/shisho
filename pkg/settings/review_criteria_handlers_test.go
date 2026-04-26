package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func buildReviewCriteriaGetRequest(t *testing.T, e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/settings/review-criteria", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func buildReviewCriteriaPutRequest(t *testing.T, e *echo.Echo, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/settings/review-criteria", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func newReviewCriteriaHandler(t *testing.T, db *bun.DB) *reviewCriteriaHandler {
	t.Helper()
	return &reviewCriteriaHandler{
		db:                 db,
		appSettingsService: appsettings.NewService(db),
	}
}

func TestGetReviewCriteria_ReturnsDefault(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	h := newReviewCriteriaHandler(t, db)

	e := newTestEcho(t)
	c, rec := buildReviewCriteriaGetRequest(t, e)

	require.NoError(t, h.getReviewCriteria(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body reviewCriteriaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	// Default criteria seeded by review.Default()
	defaults := review.Default()
	assert.Equal(t, defaults.BookFields, body.BookFields)
	assert.Equal(t, defaults.AudioFields, body.AudioFields)

	// Candidate slices are always present
	assert.Equal(t, review.UniversalCandidates, body.UniversalCandidates)
	assert.Equal(t, review.AudioCandidates, body.AudioCandidates)

	// No files in the test DB yet
	assert.Equal(t, 0, body.OverrideCount)
	assert.Equal(t, 0, body.MainFileCount)
}

func TestPutReviewCriteria_PersistsAndEnqueuesJob(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	h := newReviewCriteriaHandler(t, db)

	body := `{"book_fields":["authors","cover"],"audio_fields":["narrators"],"clear_overrides":true}`

	e := newTestEcho(t)
	c, rec := buildReviewCriteriaPutRequest(t, e, body)

	require.NoError(t, h.putReviewCriteria(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify criteria persisted in app_settings
	criteria, err := review.Load(context.Background(), appsettings.NewService(db))
	require.NoError(t, err)
	assert.Equal(t, []string{"authors", "cover"}, criteria.BookFields)
	assert.Equal(t, []string{"narrators"}, criteria.AudioFields)

	// Verify a recompute_review job was inserted
	var job models.Job
	err = db.NewSelect().
		Model(&job).
		Where("type = ?", models.JobTypeRecomputeReview).
		OrderExpr("id DESC").
		Limit(1).
		Scan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, models.JobStatusPending, job.Status)

	require.NoError(t, job.UnmarshalData())
	data, ok := job.DataParsed.(*models.JobRecomputeReviewData)
	require.True(t, ok)
	assert.True(t, data.ClearOverrides)
}

func TestPutReviewCriteria_RejectsInvalidField(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	h := newReviewCriteriaHandler(t, db)

	// "narrators" is an audio-only field, not valid in book_fields
	body := `{"book_fields":["narrators"],"audio_fields":[]}`

	e := newTestEcho(t)
	c, _ := buildReviewCriteriaPutRequest(t, e, body)

	err := h.putReviewCriteria(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}
