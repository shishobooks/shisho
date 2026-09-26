package jobs

import (
	"context"
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
	"github.com/shishobooks/shisho/pkg/httputil"
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

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// A bulk download packages files the user could already download one at a
	// time, so it needs Books Read and library access instead of Jobs
	// permissions. Every other job type needs Jobs Read and Jobs Write.
	if params.Type == models.JobTypeBulkDownload {
		if !user.HasPermission(models.ResourceBooks, models.OperationRead) {
			return errcodes.Forbidden("Bulk download requires the books:read permission.")
		}

		// Validate file_ids by marshaling the data and checking the field.
		dataBytes, err := json.Marshal(params.Data)
		if err != nil {
			return errcodes.BadRequest("Invalid bulk download data")
		}
		var bulkData models.JobBulkDownloadData
		if err := json.Unmarshal(dataBytes, &bulkData); err != nil {
			return errcodes.BadRequest("Invalid bulk download data")
		}
		if len(bulkData.FileIDs) == 0 {
			return errcodes.BadRequest("No file IDs provided for bulk download")
		}
		if err := h.checkFileLibraryAccess(ctx, user, bulkData.FileIDs); err != nil {
			return err
		}
		// Store only the validated input. Result fields are the worker's to
		// set, and a bulk download never belongs to one library.
		params.Data = &models.JobBulkDownloadData{
			FileIDs:            bulkData.FileIDs,
			EstimatedSizeBytes: bulkData.EstimatedSizeBytes,
		}
		params.LibraryID = nil
	} else if !user.HasPermission(models.ResourceJobs, models.OperationRead) ||
		!user.HasPermission(models.ResourceJobs, models.OperationWrite) {
		// Other job types keep the jobs:read plus jobs:write requirement the
		// jobs group and route used to enforce together.
		return errcodes.Forbidden("Creating this job requires the jobs:read and jobs:write permissions.")
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
		Type:            params.Type,
		Status:          models.JobStatusPending,
		DataParsed:      params.Data,
		LibraryID:       params.LibraryID,
		CreatedByUserID: &user.ID,
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
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
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

	// Answer like a missing job so IDs of other users' jobs cannot be probed.
	if !canReadJob(user, job) {
		return errcodes.NotFound("Job")
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

	resp := ListJobsResponse{Items: jobs, Total: total}

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
		return errcodes.Forbidden("Downloading requires the books:read permission.")
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

	// Answer like a missing job so IDs of other users' jobs cannot be probed.
	if !canReadJob(user, job) {
		return errcodes.NotFound("Job")
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

	// Library access may have been revoked since the job was created.
	if err := h.checkFileLibraryAccess(ctx, user, data.FileIDs); err != nil {
		return err
	}

	zipPath := h.downloadCache.BulkZipPath(data.FingerprintHash)
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return errcodes.NotFound("Download file has expired from cache")
	}

	filename := fmt.Sprintf("shisho-download-%d-books.zip", data.FileCount)
	httputil.SetAttachmentFilename(c.Response(), filename)
	c.Response().Header().Set("Cache-Control", "private, no-store")

	return c.File(zipPath)
}

// canReadJob reports whether the user may read a job. Jobs Read grants every
// job. Without it, a user may read only a bulk download they created.
func canReadJob(user *models.User, job *models.Job) bool {
	if user.HasPermission(models.ResourceJobs, models.OperationRead) {
		return true
	}
	return job.Type == models.JobTypeBulkDownload &&
		job.CreatedByUserID != nil &&
		*job.CreatedByUserID == user.ID
}

// fileLibraryAccessChunkSize keeps each IN clause well below SQLite's bound
// parameter limit.
const fileLibraryAccessChunkSize = 500

// checkFileLibraryAccess rejects the request when any of the files belongs to
// a library the user cannot access. Missing files are skipped because the
// worker skips them too and a file may be deleted after the job is created.
// It queries files directly to avoid an import cycle (jobs cannot import books).
func (h *handler) checkFileLibraryAccess(ctx context.Context, user *models.User, fileIDs []int) error {
	if user.HasAllLibraryAccess() {
		return nil
	}
	for start := 0; start < len(fileIDs); start += fileLibraryAccessChunkSize {
		end := min(start+fileLibraryAccessChunkSize, len(fileIDs))
		var libraryIDs []int
		err := h.db.NewSelect().
			Table("files").
			ColumnExpr("DISTINCT library_id").
			Where("id IN (?)", bun.List(fileIDs[start:end])).
			Scan(ctx, &libraryIDs)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, libraryID := range libraryIDs {
			if !user.HasLibraryAccess(libraryID) {
				return errcodes.Forbidden("This download includes libraries you don't have access to.")
			}
		}
	}
	return nil
}
