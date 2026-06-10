package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// reviewCriteriaHandler handles the review-criteria settings endpoints.
type reviewCriteriaHandler struct {
	db                 *bun.DB
	appSettingsService *appsettings.Service
}

func (h *reviewCriteriaHandler) getReviewCriteria(c echo.Context) error {
	ctx := c.Request().Context()

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}

	resp := ReviewCriteriaResponse{
		BookFields:          criteria.BookFields,
		AudioFields:         criteria.AudioFields,
		UniversalCandidates: review.UniversalCandidates,
		AudioCandidates:     review.AudioCandidates,
	}

	if err := h.db.NewSelect().
		TableExpr("files").
		ColumnExpr("count(*)").
		Where("file_role = 'main' AND review_override IS NOT NULL").
		Scan(ctx, &resp.OverrideCount); err != nil {
		return errors.WithStack(err)
	}

	if err := h.db.NewSelect().
		TableExpr("files").
		ColumnExpr("count(*)").
		Where("file_role = 'main'").
		Scan(ctx, &resp.MainFileCount); err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *reviewCriteriaHandler) putReviewCriteria(c echo.Context) error {
	var payload PutReviewCriteriaPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	criteria := review.Criteria{BookFields: payload.BookFields, AudioFields: payload.AudioFields}
	if err := review.Validate(criteria); err != nil {
		return errcodes.BadRequest(err.Error())
	}

	ctx := c.Request().Context()

	if err := review.Save(ctx, h.appSettingsService, criteria); err != nil {
		return errors.WithStack(err)
	}

	jobData, err := json.Marshal(models.JobRecomputeReviewData{ClearOverrides: payload.ClearOverrides})
	if err != nil {
		return errors.WithStack(err)
	}

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   string(jobData),
	}
	if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusOK)
}
