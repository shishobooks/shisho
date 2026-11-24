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
// scanned before Shisho kept word counts or whose count failed at parse
// time is. A File whose last scan couldn't parse it is left to the scan
// that can.
func needsWordCount(file *models.File) bool {
	if file.WordCount != nil || file.ScanError != nil || file.FileRole != models.FileRoleMain {
		return false
	}
	switch file.FileType {
	case models.FileTypeEPUB, models.FileTypeKePub, models.FileTypeMOBI, models.FileTypeAZW3:
		return true
	}
	return false
}

// countUnchangedFileWords counts and stores the words of an unchanged File
// that needsWordCount, reading only its text. It writes only the count, so
// the File's updated_at, which cover and page URLs are keyed by, stays put.
// A File it can't count is logged and keeps no count, for the next scan to
// try; that never fails the scan.
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
	n, err := countWords(ctx, file.Filepath, file.FileType)
	if err != nil {
		logWarn("could not count words", logger.Data{"file_id": file.ID, "filepath": file.Filepath, "error": err.Error()})
		return
	}
	_, err = w.db.NewUpdate().
		Table("files").
		Set("word_count = ?", n).
		Where("id = ?", file.ID).
		Exec(ctx)
	if err != nil {
		logWarn("could not store word count", logger.Data{"file_id": file.ID, "error": err.Error()})
		return
	}
	file.WordCount = &n
}

// countWords counts the words of the reflowable ebook at path, as its parser
// does.
func countWords(ctx context.Context, path, fileType string) (int, error) {
	switch fileType {
	case models.FileTypeEPUB, models.FileTypeKePub:
		return epub.WordCount(path)
	case models.FileTypeMOBI, models.FileTypeAZW3:
		return mobi.WordCount(ctx, path)
	}
	return 0, errors.Errorf("no word count for %s files", fileType)
}
