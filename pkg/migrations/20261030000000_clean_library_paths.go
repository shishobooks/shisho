package migrations

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Library paths were stored as entered. The scan compares a file's directory,
// which is always clean, against them, so a path entered as "/books/" never
// matched: root-level files were grouped into one Book keyed by the library
// root instead of a Book each. The libraries service now cleans paths on
// save; this cleans the stored ones and drops a library's repeats.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			var rows []struct {
				ID        int    `bun:"id"`
				LibraryID int    `bun:"library_id"`
				Filepath  string `bun:"filepath"`
			}
			if err := tx.NewRaw(`SELECT id, library_id, filepath FROM library_paths ORDER BY id`).Scan(ctx, &rows); err != nil {
				return errors.Wrap(err, "failed to list library paths")
			}
			type key struct {
				libraryID int
				path      string
			}
			seen := make(map[key]bool, len(rows))
			for _, row := range rows {
				cleaned := row.Filepath
				if cleaned != "" {
					cleaned = filepath.Clean(cleaned)
				}
				k := key{row.LibraryID, cleaned}
				if seen[k] {
					if _, err := tx.ExecContext(ctx, `DELETE FROM library_paths WHERE id = ?`, row.ID); err != nil {
						return errors.Wrap(err, "failed to delete repeated library path")
					}
					continue
				}
				seen[k] = true
				if cleaned != row.Filepath {
					if _, err := tx.ExecContext(ctx, `UPDATE library_paths SET filepath = ? WHERE id = ?`, cleaned, row.ID); err != nil {
						return errors.Wrap(err, "failed to clean library path")
					}
				}
			}
			return nil
		})
	}

	// Down is a no-op: a cleaned path names the same folder as the original.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
