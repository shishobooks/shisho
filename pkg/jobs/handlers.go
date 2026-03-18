package jobs

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type handler struct {
	jobService    *Service
	db            *bun.DB
	broker        *events.Broker
	downloadCache *downloadcache.Cache
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := CreateJobPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Check if a scan job is already running or pending.
	if params.Type == models.JobTypeScan {
		hasActive, err := h.jobService.HasActiveJob(ctx, models.JobTypeScan, params.LibraryID)
		if err != nil {
			return errors.WithStack(err)
		}
		if hasActive {
			return errcodes.Conflict("A scan job is already running or pending.")
		}
	}

	job := &models.Job{
		Type:       params.Type,
		Status:     models.JobStatusPending,
		DataParsed: params.Data,
		LibraryID:  params.LibraryID,
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

	if h.broker != nil {
		h.broker.Publish(events.NewJobEvent("job.created", job.ID, job.Status, job.Type, job.LibraryID))
	}

	return errors.WithStack(c.JSON(http.StatusOK, job))
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Job")
	}

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
		Limit:             &params.Limit,
		Offset:            &params.Offset,
		Statuses:          params.Status,
		Type:              params.Type,
		LibraryIDOrGlobal: params.LibraryIDOrGlobal,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Jobs  []*models.Job `json:"jobs"`
		Total int           `json:"total"`
	}{jobs, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) download(c echo.Context) error {
	ctx := c.Request().Context()

	// Verify the user has books:read permission
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}
	if !user.HasPermission(models.ResourceBooks, models.OperationRead) {
		return errcodes.Forbidden("You need books:read permission to download")
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Job")
	}

	job, err := h.jobService.RetrieveJob(ctx, RetrieveJobOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if job.Type != models.JobTypeBulkDownload {
		return errcodes.BadRequest("Job is not a bulk download")
	}

	if job.Status != models.JobStatusCompleted {
		return errcodes.BadRequest("Job is not completed yet")
	}

	var data models.JobBulkDownloadData
	if err := json.Unmarshal([]byte(job.Data), &data); err != nil {
		return errors.WithStack(err)
	}

	if data.FingerprintHash == "" {
		return errcodes.BadRequest("Job has no download data")
	}

	// Verify the user has library access for the files in this download.
	// Query the DB directly to avoid an import cycle (jobs cannot import books).
	for _, fileID := range data.FileIDs {
		var file models.File
		err := h.db.NewSelect().Model(&file).Column("library_id").Where("id = ?", fileID).Scan(ctx)
		if err != nil {
			continue // File may have been deleted since job was created
		}
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to one or more libraries in this download")
		}
	}

	zipPath := h.downloadCache.BulkZipPath(data.FingerprintHash)
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return errcodes.NotFound("Download file has expired from cache")
	}

	filename := fmt.Sprintf("shisho-download-%d-books.zip", data.FileCount)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	return c.File(zipPath)
}
