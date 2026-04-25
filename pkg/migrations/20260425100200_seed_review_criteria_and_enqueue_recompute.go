package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Seed default review criteria.
		defaultJSON, err := json.Marshal(map[string]interface{}{
			"book_fields":  []string{"authors", "description", "cover", "genres"},
			"audio_fields": []string{"narrators"},
		})
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO app_settings (key, value, updated_at)
			VALUES ('review_criteria', ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO NOTHING
		`, string(defaultJSON)); err != nil {
			return errors.WithStack(err)
		}

		// Enqueue a recompute job so the worker fills in files.reviewed
		// asynchronously after migrations finish. Until then, files.reviewed
		// is NULL and the books/list query treats NULL as needs-review.
		jobData, err := json.Marshal(map[string]interface{}{"clear_overrides": false})
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO jobs (type, status, data, progress, created_at, updated_at)
			VALUES ('recompute_review', 'pending', ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, string(jobData)); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`DELETE FROM jobs WHERE type = 'recompute_review'`,
			`DELETE FROM app_settings WHERE key = 'review_criteria'`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
