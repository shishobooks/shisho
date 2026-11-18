package worker

import (
	"context"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

// ProcessSidecarRewriteJob rewrites the sidecars of every Book a rename or
// merge marked stale (ADR 0027). A Book whose sidecars can't be written stays
// marked, and scans skip its sidecars until they can be.
func (w *Worker) ProcessSidecarRewriteJob(ctx context.Context, _ *models.Job, jobLog *joblogs.JobLogger) error {
	jobLog.Info("rewriting stale sidecars", nil)
	rewritten, err := w.bookService.RewriteStaleSidecars(ctx, jobLog.Warn)
	jobLog.Info("rewrote stale sidecars", logger.Data{"books": rewritten})
	return err
}

// ensureSidecarRewrite queues the sidecar rewrite job at startup when Books
// are still marked, as after a shutdown that cancelled the job or a crash
// between marking and queuing it.
func (w *Worker) ensureSidecarRewrite(ctx context.Context) {
	stale, err := w.bookService.HasStaleSidecars(ctx)
	if err != nil {
		w.log.Err(err).Warn("failed to check for stale sidecars")
		return
	}
	if !stale {
		return
	}
	if err := w.bookService.EnsureSidecarRewriteJob(ctx); err != nil {
		w.log.Err(err).Warn("failed to queue the sidecar rewrite job")
	}
}
