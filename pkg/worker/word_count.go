package worker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/wordcount"
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
// logged; one whose content is at fault is marked, so later scans don't try
// it again until it changes, while one it couldn't read is tried by the
// next scan. That never fails the scan.
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
		if wordcount.Retryable(err) {
			return
		}
	} else {
		count = &n
	}
	// Only a main File still waiting for a count takes it: one made a
	// supplement meanwhile, or read by a scan that counted it, keeps what
	// it has.
	if err := w.storeWordCount(ctx, file, count, count == nil, "word_count IS NULL AND word_count_failed = FALSE AND file_role = 'main'"); err != nil {
		logWarn("could not store word count", logger.Data{"file_id": file.ID, "error": err.Error()})
	}
}

// storeWordCount writes file's word count, or no count and whether its
// content is at fault, where condition still holds. It writes only those
// columns: a count isn't an edit, so updated_at, which keys the File's cover
// URLs, stays put.
func (w *Worker) storeWordCount(ctx context.Context, file *models.File, count *int, failed bool, condition string) error {
	res, err := w.db.NewUpdate().
		Table("files").
		Set("word_count = ?", count).
		Set("word_count_failed = ?", failed).
		Where("id = ?", file.ID).
		Where(condition).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows > 0 {
		file.WordCount = count
		file.WordCountFailed = failed
	}
	return nil
}

// sameWordCount reports whether file already holds count and failed.
func sameWordCount(file *models.File, count *int, failed bool) bool {
	if (count == nil) != (file.WordCount == nil) || failed != file.WordCountFailed {
		return false
	}
	return count == nil || *count == *file.WordCount
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
