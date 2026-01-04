package jobs

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type RetrieveJobOptions struct {
	ID *int
}

type ListJobsOptions struct {
	Limit              *int
	Offset             *int
	Statuses           []string
	ProcessIDToExclude *string

	includeTotal bool
}

type UpdateJobOptions struct {
	Columns []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateJob(ctx context.Context, job *models.Job) error {
	now := time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = job.CreatedAt

	if job.Data == "" && job.DataParsed != nil {
		// Marshal the data into a JSON string to save into the database.
		data, err := json.Marshal(job.DataParsed)
		if err != nil {
			return errors.WithStack(err)
		}
		job.Data = string(data)
	}

	_, err := svc.db.
		NewInsert().
		Model(job).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (svc *Service) RetrieveJob(ctx context.Context, opts RetrieveJobOptions) (*models.Job, error) {
	job := &models.Job{}

	q := svc.db.
		NewSelect().
		Model(job)

	if opts.ID != nil {
		q = q.Where("j.id = ?", *opts.ID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Job")
		}
		return nil, errors.WithStack(err)
	}

	if job.Data != "" {
		// Unmarshal the data into a struct to be returned.
		err := job.UnmarshalData()
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return job, nil
}

func (svc *Service) ListJobs(ctx context.Context, opts ListJobsOptions) ([]*models.Job, error) {
	j, _, err := svc.listJobsWithTotal(ctx, opts)
	return j, errors.WithStack(err)
}

func (svc *Service) ListJobsWithTotal(ctx context.Context, opts ListJobsOptions) ([]*models.Job, int, error) {
	opts.includeTotal = true
	return svc.listJobsWithTotal(ctx, opts)
}

func (svc *Service) listJobsWithTotal(ctx context.Context, opts ListJobsOptions) ([]*models.Job, int, error) {
	jobs := []*models.Job{}
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&jobs).
		Order("j.created_at ASC")

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}
	if opts.Statuses != nil {
		q = q.WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			for _, s := range opts.Statuses {
				sq = sq.WhereOr("j.status = ?", s)
			}
			return sq
		})
	}
	if opts.ProcessIDToExclude != nil {
		q = q.WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.
				Where("j.process_id IS NULL").
				WhereOr("j.process_id != ?", *opts.ProcessIDToExclude)
		})
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	for _, job := range jobs {
		err := job.UnmarshalData()
		if err != nil {
			return nil, 0, errors.WithStack(err)
		}
	}

	return jobs, total, nil
}

// HasActiveJobByType checks if there's a pending or in-progress job of the given type.
func (svc *Service) HasActiveJobByType(ctx context.Context, jobType string) (bool, error) {
	count, err := svc.db.NewSelect().
		Model((*models.Job)(nil)).
		Where("type = ?", jobType).
		WhereGroup(" AND ", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Where("status = ?", models.JobStatusPending).
				WhereOr("status = ?", models.JobStatusInProgress)
		}).
		Count(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return count > 0, nil
}

func (svc *Service) UpdateJob(ctx context.Context, job *models.Job, opts UpdateJobOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	// Update updated_at.
	now := time.Now()
	job.UpdatedAt = now
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(job).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Job")
		}
		return errors.WithStack(err)
	}

	return nil
}
