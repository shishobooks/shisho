package jobs

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type handler struct {
	jobService *Service
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := CreateJobPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	job := &Job{
		Type:       params.Type,
		Status:     JobStatusPending,
		DataParsed: params.Data,
	}

	err := h.jobService.CreateJob(ctx, job)
	if err != nil {
		return errors.WithStack(err)
	}

	job, err = h.jobService.RetrieveJob(ctx, RetrieveJobOptions{
		ID: &job.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, job))
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	job, err := h.jobService.RetrieveJob(ctx, RetrieveJobOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, job))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListJobsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	jobs, total, err := h.jobService.ListJobsWithTotal(ctx, ListJobsOptions{
		Limit:    &params.Limit,
		Offset:   &params.Offset,
		Statuses: params.Status,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Jobs  []*Job `json:"jobs"`
		Total int    `json:"total"`
	}{jobs, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
