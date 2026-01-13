package joblogs

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/jobs"
)

type handler struct {
	jobLogService *Service
	jobService    *jobs.Service
}

func (h *handler) listLogs(c echo.Context) error {
	ctx := c.Request().Context()

	jobID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Job")
	}

	// Verify job exists
	job, err := h.jobService.RetrieveJob(ctx, jobs.RetrieveJobOptions{
		ID: &jobID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Bind query params
	params := ListJobLogsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	logs, err := h.jobLogService.ListJobLogs(ctx, ListJobLogsOptions{
		JobID:   jobID,
		AfterID: params.AfterID,
		Levels:  params.Level,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Logs interface{} `json:"logs"`
		Job  interface{} `json:"job"`
	}{logs, job}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
