package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// filesUpdateTrigger is the change log's trigger on files, comparing
	// columns: a change to any of them moves the Books the File was and is
	// in, so the Shisho app downloads them again.
	filesUpdateTrigger := func(columns ...string) string {
		terms := make([]string, len(columns))
		for i, column := range columns {
			terms[i] = fmt.Sprintf("OLD.%[1]s IS NOT NEW.%[1]s", column)
		}
		return fmt.Sprintf(`CREATE TRIGGER change_log_files_update AFTER UPDATE ON files WHEN %s BEGIN
			DELETE FROM change_log_entries WHERE kind = 'book' AND record_id IN (SELECT id FROM books WHERE id IN (OLD.book_id, NEW.book_id));
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'book', id, library_id, FALSE, CURRENT_TIMESTAMP FROM books WHERE id IN (OLD.book_id, NEW.book_id);
		END`, strings.Join(terms, " OR "))
	}
	comparedBefore := []string{
		"book_id", "file_type", "file_role", "filesize_bytes", "name", "name_source", "filepath",
		"release_date", "page_count", "audiobook_duration_seconds",
		"cover_image_filename", "is_preferred_cover", "updated_at", "publisher_id",
		"audiobook_codec", "audiobook_bitrate_bps", "language", "abridged", "url", "reading_direction",
		"file_modified_at", "created_at",
	}

	// Bun records a migration after it commits, so a server stopped between
	// the two runs it again on its next start: each step checks whether it
	// already ran.
	hasColumn := func(ctx context.Context, tx bun.Tx, column string) (bool, error) {
		var n int
		err := tx.NewRaw("SELECT COUNT(*) FROM pragma_table_info('files') WHERE name = ?", column).Scan(ctx, &n)
		return n > 0, errors.WithStack(err)
	}
	run := func(ctx context.Context, tx bun.Tx, statements ...string) error {
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return errors.Wrapf(err, "run %s", statement)
			}
		}
		return nil
	}
	columns := []struct{ name, definition string }{
		// How many words a reflowable ebook's text holds. NULL until it's
		// counted, and for every other kind of File.
		{"word_count", "INTEGER"},
		// Whether the File's current content couldn't be counted, so a scan
		// that skips the unchanged File doesn't try again.
		{"word_count_failed", "BOOLEAN NOT NULL DEFAULT FALSE"},
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, column := range columns {
				has, err := hasColumn(ctx, tx, column.name)
				if err != nil {
					return err
				}
				if !has {
					if err := run(ctx, tx, fmt.Sprintf("ALTER TABLE files ADD COLUMN %s %s", column.name, column.definition)); err != nil {
						return err
					}
				}
			}
			return run(ctx, tx,
				// The Shisho app shows it beside a File's pages or duration,
				// so a count moves the File's Book in the change feed.
				`DROP TRIGGER IF EXISTS change_log_files_update`,
				filesUpdateTrigger(append(comparedBefore, "word_count")...),

				// A scan counts the words of an unchanged File that has
				// none, so one scan of every library counts the Files
				// already there without waiting for the scheduled one.
				`INSERT INTO jobs (type, status, data, progress, created_at, updated_at)
					SELECT 'scan', 'pending', '{}', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
					WHERE EXISTS (SELECT 1 FROM libraries)
					AND NOT EXISTS (SELECT 1 FROM jobs WHERE type = 'scan' AND status IN ('pending', 'in_progress') AND library_id IS NULL)`,
			)
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// The trigger goes back first: SQLite won't drop a column a
			// trigger reads.
			if err := run(ctx, tx, `DROP TRIGGER IF EXISTS change_log_files_update`, filesUpdateTrigger(comparedBefore...)); err != nil {
				return err
			}
			for _, column := range columns {
				has, err := hasColumn(ctx, tx, column.name)
				if err != nil {
					return err
				}
				if has {
					if err := run(ctx, tx, "ALTER TABLE files DROP COLUMN "+column.name); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}

	Migrations.MustRegister(up, down)
}
