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

	up := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// How many words a reflowable ebook's text holds. NULL until
			// it's counted, and for every other kind of File.
			`ALTER TABLE files ADD COLUMN word_count INTEGER`,

			// The Shisho app shows it beside a File's pages or duration,
			// so a count moves the File's Book in the change feed.
			`DROP TRIGGER change_log_files_update`,
			filesUpdateTrigger(append(comparedBefore, "word_count")...),

			// A scan counts the words of an unchanged File that has none,
			// so one scan of every library counts the Files already there
			// without waiting for the scheduled one.
			`INSERT INTO jobs (type, status, data, progress, created_at, updated_at)
				SELECT 'scan', 'pending', '{}', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
				WHERE EXISTS (SELECT 1 FROM libraries)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			// The trigger goes back first: SQLite won't drop a column a
			// trigger reads.
			`DROP TRIGGER change_log_files_update`,
			filesUpdateTrigger(comparedBefore...),
			`ALTER TABLE files DROP COLUMN word_count`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
