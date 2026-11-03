package worker

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

func (w *Worker) ProcessRecomputeReviewJob(ctx context.Context, job *models.Job, _ *joblogs.JobLogger) error {
	var data models.JobRecomputeReviewData
	if err := json.Unmarshal([]byte(job.Data), &data); err != nil {
		return errors.WithStack(err)
	}

	if data.ClearOverrides {
		if _, err := w.db.NewUpdate().
			Model((*models.File)(nil)).
			Set("review_override = NULL").
			Set("review_overridden_at = NULL").
			Where("file_role = ?", models.FileRoleMain).
			Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	// Bail out cleanly between stages if shutdown was requested. Each bulk
	// statement above already respects ctx via Exec(ctx); these checks just
	// avoid starting the next stage when cancellation is already in flight.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Reset reviewed for supplements in a single statement, which also covers
	// books with no main file that the batches below never reach.
	if _, err := w.db.NewUpdate().
		Model((*models.File)(nil)).
		Set("reviewed = NULL").
		Where("file_role = ?", models.FileRoleSupplement).
		Exec(ctx); err != nil {
		return errors.WithStack(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	criteria, err := review.Load(ctx, w.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}

	// Recompute the books that have main files, in batches. Order by id for
	// stable progress %.
	var bookIDs []int
	if err := w.db.NewSelect().
		Model((*models.File)(nil)).
		ColumnExpr("DISTINCT f.book_id").
		Where("f.file_role = ?", models.FileRoleMain).
		Order("f.book_id ASC").
		Scan(ctx, &bookIDs); err != nil {
		return errors.WithStack(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	done := 0
	for batch := range slices.Chunk(bookIDs, review.RecomputeBatchSize) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := review.RecomputeForBooks(ctx, w.db, batch, criteria); err != nil {
			return errors.WithStack(err)
		}
		done += len(batch)
		pct := int(float64(done) / float64(len(bookIDs)) * 100)
		if _, err := w.db.NewUpdate().
			Model((*models.Job)(nil)).
			Set("progress = ?", pct).
			Where("id = ?", job.ID).
			Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
