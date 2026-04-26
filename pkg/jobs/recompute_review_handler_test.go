package jobs

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newValidator returns a validator instance that mirrors the one used by the
// binder (same tag-name function so field names in errors use JSON names).
func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return v
}

// TestCreateJob_RecomputeReview verifies that the service layer accepts a
// recompute_review job and persists it with the correct type and status.
func TestCreateJob_RecomputeReview(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	job := &models.Job{
		Type:       models.JobTypeRecomputeReview,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobRecomputeReviewData{ClearOverrides: true},
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)
	assert.Positive(t, job.ID)

	// Retrieve and verify the persisted job.
	retrieved, err := svc.RetrieveJob(ctx, RetrieveJobOptions{ID: &job.ID})
	require.NoError(t, err)
	assert.Equal(t, models.JobTypeRecomputeReview, retrieved.Type)
	assert.Equal(t, models.JobStatusPending, retrieved.Status)
	data, ok := retrieved.DataParsed.(*models.JobRecomputeReviewData)
	require.True(t, ok, "expected DataParsed to be *models.JobRecomputeReviewData")
	assert.True(t, data.ClearOverrides)
}

// TestCreateJobPayload_RecomputeReview_Valid verifies that the CreateJobPayload
// validation (the oneof= tag on the Type field) accepts "recompute_review".
func TestCreateJobPayload_RecomputeReview_Valid(t *testing.T) {
	t.Parallel()
	v := newValidator()

	payload := CreateJobPayload{
		Type: models.JobTypeRecomputeReview,
		Data: &models.JobRecomputeReviewData{ClearOverrides: false},
	}
	err := v.Struct(payload)
	assert.NoError(t, err, "recompute_review should be a valid job type in CreateJobPayload")
}

// TestListJobsQuery_RecomputeReview_Valid verifies that the ListJobsQuery
// validation accepts "recompute_review" as a filter type.
func TestListJobsQuery_RecomputeReview_Valid(t *testing.T) {
	t.Parallel()
	v := newValidator()

	jobType := models.JobTypeRecomputeReview
	query := ListJobsQuery{
		Limit:  10,
		Offset: 0,
		Type:   &jobType,
	}
	err := v.Struct(query)
	assert.NoError(t, err, "recompute_review should be a valid type in ListJobsQuery")
}
