package migrations

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Splitting a root-level file into a new Book in a library without
// OrganizeFileStructure used to key the new Book by the library root itself.
// Every later split then found the root taken and failed, and deleting the
// Book with organizing on removed the whole library root. Such a Book with
// one main File is keyed by that File instead, as a root-level single-file
// Book is. Library paths are stored as entered, so a trailing slash is
// ignored. One with several main Files is left alone: deleting it no longer
// removes the root, and organizing it moves its Files into a folder instead
// of renaming the root.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			var ids []int
			err := tx.NewRaw(`
				UPDATE books
				SET filepath = (SELECT f.filepath FROM files f WHERE f.book_id = books.id AND f.file_role = 'main')
				WHERE rtrim(filepath, '/') IN (
					SELECT rtrim(lp.filepath, '/') FROM library_paths lp WHERE lp.library_id = books.library_id
				)
				  AND (SELECT COUNT(*) FROM files f WHERE f.book_id = books.id AND f.file_role = 'main') = 1
				  AND NOT EXISTS (
					SELECT 1 FROM books other
					WHERE other.library_id = books.library_id
					  AND other.filepath = (SELECT f.filepath FROM files f WHERE f.book_id = books.id AND f.file_role = 'main')
				  )
				RETURNING id`).Scan(ctx, &ids)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return errors.Wrap(err, "failed to rekey books at library roots")
			}
			if len(ids) == 0 {
				return nil
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE books_fts
				SET filepath = (SELECT b.filepath FROM books b WHERE b.id = books_fts.rowid)
				WHERE rowid IN (?)`, bun.List(ids))
			return errors.Wrap(err, "failed to update search index for rekeyed books")
		})
	}

	// Down is a no-op: the old path was the library root, which no Book
	// should be keyed by.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
