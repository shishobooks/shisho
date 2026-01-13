package joblogs

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type ListJobLogsOptions struct {
	JobID   int
	AfterID *int
	Levels  []string
}

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

func (svc *Service) CreateJobLog(ctx context.Context, log *models.JobLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	_, err := svc.db.
		NewInsert().
		Model(log).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (svc *Service) ListJobLogs(ctx context.Context, opts ListJobLogsOptions) ([]*models.JobLog, error) {
	logs := []*models.JobLog{}

	q := svc.db.
		NewSelect().
		Model(&logs).
		Where("jl.job_id = ?", opts.JobID).
		Order("jl.id ASC")

	if opts.AfterID != nil {
		q = q.Where("jl.id > ?", *opts.AfterID)
	}

	if len(opts.Levels) > 0 {
		q = q.Where("jl.level IN (?)", bun.In(opts.Levels))
	}

	err := q.Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return logs, nil
}
