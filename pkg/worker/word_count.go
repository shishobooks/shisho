package worker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
)

// needsWordCount reports whether a scan that skips an unchanged File should
// still count its words: a main reflowable File with no count, as one
// scanned before Shisho kept word counts is. A File whose content couldn't
// be counted waits for its content to change, and one whose last scan
// couldn't parse it for the scan that can.
func needsWordCount(file *models.File) bool {
	return file.WordCount == nil && !file.WordCountFailed && file.ScanError == nil &&
		file.FileRole == models.FileRoleMain && models.IsReflowableFileType(file.FileType)
}

// countUnchangedFileWords counts and stores the words of an unchanged File
// that needsWordCount, reading only its text. A File it can't count is
// logged and marked, so later scans don't try it again until it changes;
// that never fails the scan.
func (w *Worker) countUnchangedFileWords(ctx context.Context, file *models.File, opts ScanOptions) {
	if !needsWordCount(file) {
		return
	}
	logWarn := func(msg string, data logger.Data) {
		logger.FromContext(ctx).Warn(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Warn(msg, data)
		}
	}
	var count *int
	n, err := countWords(ctx, file.Filepath, file.FileType)
	if err != nil {
		logWarn("could not count words", logger.Data{"file_id": file.ID, "filepath": file.Filepath, "error": err.Error()})
	} else {
		count = &n
	}
	// Only a main File still waiting for a count takes it: one made a
	// supplement meanwhile, or read by a scan that counted it, keeps what
	// it has.
	if err := w.storeWordCount(ctx, file, count, "word_count IS NULL AND word_count_failed = FALSE AND file_role = 'main'"); err != nil {
		logWarn("could not store word count", logger.Data{"file_id": file.ID, "error": err.Error()})
	}
}

// storeWordCount writes file's word count, or that its content couldn't be
// counted when count is nil, where condition (if any) still holds. It writes
// only those columns: a count isn't an edit, so updated_at, which keys the
// File's cover URLs, stays put.
func (w *Worker) storeWordCount(ctx context.Context, file *models.File, count *int, condition string) error {
	q := w.db.NewUpdate().
		Table("files").
		Set("word_count = ?", count).
		Set("word_count_failed = ?", count == nil).
		Where("id = ?", file.ID)
	if condition != "" {
		q = q.Where(condition)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows > 0 {
		file.WordCount = count
		file.WordCountFailed = count == nil
	}
	return nil
}

// sameWordCount reports whether file already holds count: the same count,
// or for a nil count, no count with the failure recorded.
func sameWordCount(file *models.File, count *int) bool {
	if count == nil {
		return file.WordCount == nil && file.WordCountFailed
	}
	return file.WordCount != nil && *file.WordCount == *count && !file.WordCountFailed
}

// countWords counts the words of the reflowable ebook at path, as its parser
// does.
func countWords(ctx context.Context, path, fileType string) (int, error) {
	switch fileType {
	case models.FileTypeEPUB, models.FileTypeKePub:
		return epub.WordCount(ctx, path)
	case models.FileTypeMOBI, models.FileTypeAZW3:
		return mobi.WordCount(ctx, path)
	}
	return 0, errors.Errorf("no word count for %s files", fileType)
}
