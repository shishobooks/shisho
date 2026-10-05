package books

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
)

// Organizing moves a file on disk and then records its new path, so a failed
// write would leave the database pointing at a path with nothing there. The
// next scan would treat the file as missing and delete its Book, then find the
// moved file and import it again with its metadata gone. Two guards keep disk
// and database in step: a destination another files row claims counts as
// taken (fileutils.OrganizedNameOptions.Claimed), and RecordOrganizedFilepath
// moves the file back when the write fails anyway.

// FilepathClaimedByOtherFile returns a fileutils Claimed check for organizing
// the file fileID: a path is claimed when another files row in the library
// holds it. A row left behind by a deleted Book can hold a path with nothing
// on disk, and files (filepath, library_id) is unique, so moving a file there
// would make recording its new path fail.
func (svc *Service) FilepathClaimedByOtherFile(ctx context.Context, libraryID, fileID int) func(string) bool {
	return func(path string) bool {
		claimed, err := svc.db.NewSelect().
			Model((*models.File)(nil)).
			Where("library_id = ?", libraryID).
			Where("filepath = ?", path).
			Where("id != ?", fileID).
			Exists(ctx)
		if err != nil {
			// Fall back to the disk check. A move onto a claimed path then
			// fails to record and is undone.
			logger.FromContext(ctx).Warn("failed to check whether a path is claimed", logger.Data{"path": path, "error": err.Error()})
			return false
		}
		return claimed
	}
}

// folderClaimedByOtherBook returns a fileutils Claimed check for renaming the
// folder of bookID: a folder is claimed when a files row of another Book in
// the library sits inside it. LIKE cannot use the filepath index and ignores
// ASCII case, so the check scans the library's files and can count a folder
// that differs only in case as claimed. Both are acceptable for a check that
// runs only when a folder rename's target is free on disk, and whose worst
// outcome is a numbered suffix.
func (svc *Service) folderClaimedByOtherBook(ctx context.Context, libraryID, bookID int) func(string) bool {
	return func(path string) bool {
		descendants := escapeLikePattern(path) + escapeLikePattern(string(os.PathSeparator)) + "%"
		claimed, err := svc.db.NewSelect().
			Model((*models.File)(nil)).
			Where("library_id = ?", libraryID).
			Where("book_id != ?", bookID).
			Where("filepath = ? OR filepath LIKE ? ESCAPE '\\'", path, descendants).
			Exists(ctx)
		if err != nil {
			logger.FromContext(ctx).Warn("failed to check whether a folder is claimed", logger.Data{"path": path, "error": err.Error()})
			return false
		}
		return claimed
	}
}

// RecordOrganizedFilepath records that organizing moved file from oldPath to
// newPath on disk, along with its cover filename, which follows the file's
// name. If the write fails, the file and the covers and sidecars that moved
// with it go back to oldPath so disk and database agree, and the write error
// is returned. includeBookSidecar must match the move: see
// fileutils.UndoOrganizedMove. On success file's in-memory path, cover
// filename, and updated_at are updated too.
func (svc *Service) RecordOrganizedFilepath(ctx context.Context, file *models.File, oldPath, newPath string, includeBookSidecar bool) error {
	now := time.Now()
	q := svc.db.NewUpdate().
		Model((*models.File)(nil)).
		Set("filepath = ?, updated_at = ?", newPath, now).
		Where("id = ?", file.ID)

	var newCover *string
	if file.CoverImageFilename != nil {
		cover := fileutils.ComputeNewCoverFilename(*file.CoverImageFilename, newPath)
		newCover = &cover
		q = q.Set("cover_image_filename = ?", cover)
	}

	if _, err := q.Exec(ctx); err != nil {
		log := logger.FromContext(ctx)
		data := logger.Data{
			"file_id":  file.ID,
			"old_path": oldPath,
			"new_path": newPath,
			"error":    err.Error(),
		}
		if undoErr := fileutils.UndoOrganizedMove(oldPath, newPath, includeBookSidecar); undoErr != nil {
			data["undo_error"] = undoErr.Error()
			log.Error("failed to update file path in database and to move the file back; the file is at new_path but the database has old_path", data)
		} else {
			log.Error("failed to update file path in database, moved the file back", data)
		}
		return errors.WithStack(err)
	}

	file.Filepath = newPath
	file.CoverImageFilename = newCover
	file.UpdatedAt = now
	return nil
}
