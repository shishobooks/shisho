package joblogs

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/jobs"
)

type handler struct {
	jobLogService *Service
	jobService    *jobs.Service
}

func (h *handler) listLogs(c echo.Context) error {
	ctx := c.Request().Context()

	jobID, err := httputil.ParamID(c, "id", "Job")
	if err != nil {
		return err
	}

	// Verify job exists. The job itself is fetched separately by the client via
	// GET /jobs/:id; this is purely an existence check so unknown jobs 404.
	if _, err := h.jobService.RetrieveJob(ctx, jobs.RetrieveJobOptions{
		ID: &jobID,
	}); err != nil {
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
		Search:  params.Search,
		Plugin:  params.Plugin,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	resp := ListJobLogsResponse{Items: logs, Total: len(logs)}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
