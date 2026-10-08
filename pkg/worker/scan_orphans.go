package worker

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

// cleanupOrphanedFiles batch-cleans files that exist in the database but were not found on disk
// during the scan. This replaces the previous sequential scanInternal loop with batch operations.
//
// existingFiles holds the library's main files from before the scan; a main
// file is orphaned when its path is missing from scannedPaths. Supplements
// are loaded fresh here, so a row whose file moved during the scan is checked
// at its current path, and one is missing when stat says its file does not
// exist (see missingSupplements). Missing supplement rows are deleted first,
// so a missing supplement is never promoted. Deleting one never deletes its
// Book, and a surviving Book that lost one is searched for supplements again.
// The search index is not touched here: a full scan rebuilds every index when
// it finishes.
//
// The method is non-fatal: all errors are logged as warnings and execution continues.
//
// cache is optional (may be nil). When provided, files whose IDs appear in
// cache.movedOrphanIDs are skipped — they were already reconciled by the move
// reconciliation phase and must not be deleted.
func (w *Worker) cleanupOrphanedFiles(
	ctx context.Context,
	existingFiles []*models.File,
	scannedPaths map[string]struct{},
	library *models.Library,
	jobLog *joblogs.JobLogger,
	cache ...*ScanCache,
) {
	// Resolve optional cache argument.
	var sc *ScanCache
	if len(cache) > 0 {
		sc = cache[0]
	}

	// Step 1: Delete supplements whose files are gone.
	missing := w.missingSupplements(ctx, library.ID, jobLog)
	missingSupplementIDs := make(map[int]struct{}, len(missing))
	missingIDs := make([]int, 0, len(missing))
	for _, f := range missing {
		missingSupplementIDs[f.ID] = struct{}{}
		missingIDs = append(missingIDs, f.ID)
		jobLog.Info("missing supplement", logger.Data{"file_id": f.ID, "book_id": f.BookID, "filepath": f.Filepath})
	}
	// Books that lost a supplement are searched for supplements again at the
	// end, since discovery otherwise runs only when a main file is imported.
	// That finds a supplement that was renamed or moved with its book folder.
	rediscoverBookIDs := make(map[int]struct{})
	if len(missingIDs) > 0 {
		if err := w.bookService.DeleteFilesByIDs(ctx, missingIDs); err != nil {
			jobLog.Warn("failed to batch-delete missing supplements", logger.Data{"error": err.Error()})
		} else {
			for _, f := range missing {
				rediscoverBookIDs[f.BookID] = struct{}{}
			}
		}
	}

	// Step 2: Collect orphaned main files and group by book.
	totalFilesByBook := make(map[int]int)         // bookID → total main file count
	orphansByBook := make(map[int][]*models.File) // bookID → orphaned files

	for _, file := range existingFiles {
		totalFilesByBook[file.BookID]++
		if _, seen := scannedPaths[file.Filepath]; !seen {
			// Skip files that were already reconciled as moves — they have a
			// valid updated filepath and must not be deleted.
			if sc != nil && sc.IsMovedOrphan(file.ID) {
				jobLog.Info("orphan cleanup: skipping moved file", logger.Data{
					"file_id":  file.ID,
					"filepath": file.Filepath,
				})
				continue
			}
			orphansByBook[file.BookID] = append(orphansByBook[file.BookID], file)
		}
	}

	if len(orphansByBook) == 0 && len(missingIDs) == 0 {
		return
	}

	jobLog.Info("batch orphan cleanup starting", logger.Data{
		"orphaned_books": len(orphansByBook),
	})

	// Collect directories for cleanup at the end
	orphanDirs := make(map[string]struct{})

	// Step 3: Handle partial orphan books.
	// Collect file IDs from books where only SOME main files are orphaned.
	var partialOrphanFileIDs []int

	// Also collect file IDs from full-orphan books where a supplement was promoted
	var promotedBookOrphanFileIDs []int

	// Collect book IDs for full deletion
	var bookIDsToDelete []int

	for bookID, orphans := range orphansByBook {
		// Track directories for all orphans
		for _, f := range orphans {
			orphanDirs[filepath.Dir(f.Filepath)] = struct{}{}
		}

		if len(orphans) < totalFilesByBook[bookID] {
			// Partial orphan: some main files remain
			for _, f := range orphans {
				partialOrphanFileIDs = append(partialOrphanFileIDs, f.ID)
				jobLog.Info("orphaned file (partial)", logger.Data{"file_id": f.ID, "filepath": f.Filepath})
			}
		}
	}

	// Batch-delete partial orphan files
	if len(partialOrphanFileIDs) > 0 {
		if err := w.bookService.DeleteFilesByIDs(ctx, partialOrphanFileIDs); err != nil {
			jobLog.Warn("failed to batch-delete partial orphan files", logger.Data{"error": err.Error()})
		}
	}

	// Step 4: Handle full orphan books.
	// Build supported types set for supplement promotion.
	supportedTypes := make(map[string]struct{})
	for _, fileType := range models.BuiltInFileTypes {
		supportedTypes[fileType] = struct{}{}
	}
	if w.pluginManager != nil {
		for ext := range w.pluginManager.RegisteredFileExtensions() {
			supportedTypes[ext] = struct{}{}
		}
	}

	for bookID, orphans := range orphansByBook {
		if len(orphans) < totalFilesByBook[bookID] {
			continue // Already handled as partial orphan
		}

		// Full orphan: all main files are gone
		jobLog.Info("all main files orphaned for book", logger.Data{"book_id": bookID})

		// Load book with files to check current state.
		// The parallel scan may have added new files to this book since existingFiles was loaded.
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
		if err != nil {
			// If the book row is already gone (e.g., deleted without FK cascade before
			// FK enforcement was enabled), clean up the orphaned file rows and any
			// remaining book-scoped children directly.
			var errCode *errcodes.Error
			if errors.As(err, &errCode) && errCode.Code == "not_found" {
				jobLog.Info("book row already missing, cleaning up orphaned children", logger.Data{"book_id": bookID})
				if w.searchService != nil {
					if delErr := w.searchService.DeleteFromBookIndex(ctx, bookID); delErr != nil {
						jobLog.Warn("failed to remove missing book from search index", logger.Data{"book_id": bookID, "error": delErr.Error()})
					}
				}
				if delErr := w.bookService.DeleteOrphanedBookChildren(ctx, bookID); delErr != nil {
					jobLog.Warn("failed to delete orphaned book children", logger.Data{"book_id": bookID, "error": delErr.Error()})
				}
				continue
			}
			jobLog.Warn("failed to retrieve orphaned book", logger.Data{"book_id": bookID, "error": err.Error()})
			continue
		}

		// Check if the book gained new main files during the parallel scan.
		// If new main files exist, this is actually a partial orphan — not a full deletion.
		// We only check for main files here; supplements are handled separately below.
		orphanIDs := make(map[int]struct{}, len(orphans))
		for _, f := range orphans {
			orphanIDs[f.ID] = struct{}{}
		}
		hasNewMainFiles := false
		for _, f := range book.Files {
			if _, isOrphan := orphanIDs[f.ID]; !isOrphan && f.FileRole == models.FileRoleMain {
				hasNewMainFiles = true
				break
			}
		}
		if hasNewMainFiles {
			// New files were added during scan — delete only the orphaned files, keep the book
			for _, f := range orphans {
				promotedBookOrphanFileIDs = append(promotedBookOrphanFileIDs, f.ID)
				jobLog.Info("orphaned file (scan-updated book)", logger.Data{"file_id": f.ID, "filepath": f.Filepath})
			}
			continue
		}

		// Collect supplements (files with supplement role). Missing ones were
		// deleted in step 1; skipping them here also covers a failed delete.
		var bookSupplements []*models.File
		for i := range book.Files {
			if book.Files[i].FileRole != models.FileRoleSupplement {
				continue
			}
			if _, isMissing := missingSupplementIDs[book.Files[i].ID]; isMissing {
				continue
			}
			bookSupplements = append(bookSupplements, book.Files[i])
		}

		// Try to promote a supplement
		var promoted bool
		for _, supp := range bookSupplements {
			if _, supported := supportedTypes[supp.FileType]; supported {
				if err := w.bookService.PromoteSupplementToMain(ctx, supp.ID); err != nil {
					jobLog.Warn("failed to promote supplement", logger.Data{"file_id": supp.ID, "error": err.Error()})
				} else {
					jobLog.Info("promoted supplement to main", logger.Data{"file_id": supp.ID, "book_id": bookID})
					promoted = true
				}
				break
			}
		}

		if promoted {
			// Delete only the orphaned main files; book and supplements survive
			for _, f := range orphans {
				promotedBookOrphanFileIDs = append(promotedBookOrphanFileIDs, f.ID)
			}
		} else {
			// No promotable supplement — delete the entire book
			// Remove from search index first
			if w.searchService != nil {
				if err := w.searchService.DeleteFromBookIndex(ctx, bookID); err != nil {
					jobLog.Warn("failed to remove book from search index", logger.Data{"book_id": bookID, "error": err.Error()})
				}
			}
			bookIDsToDelete = append(bookIDsToDelete, bookID)
			// Track book directory for cleanup
			orphanDirs[book.Filepath] = struct{}{}
			jobLog.Info("deleting orphaned book", logger.Data{"book_id": bookID})
		}
	}

	// Batch-delete orphaned files from promoted books
	if len(promotedBookOrphanFileIDs) > 0 {
		if err := w.bookService.DeleteFilesByIDs(ctx, promotedBookOrphanFileIDs); err != nil {
			jobLog.Warn("failed to batch-delete promoted book orphan files", logger.Data{"error": err.Error()})
		}
	}

	// Batch-delete fully orphaned books (cascades to all their files and relations)
	if len(bookIDsToDelete) > 0 {
		if err := w.bookService.DeleteBooksByIDs(ctx, bookIDsToDelete); err != nil {
			jobLog.Warn("failed to batch-delete orphaned books", logger.Data{"error": err.Error()})
		}
	}

	// Step 5: Rediscover supplements for books that lost one. A book deleted
	// above is gone, so the retrieve skips it.
	for bookID := range rediscoverBookIDs {
		w.rediscoverSupplements(ctx, bookID, library, jobLog)
	}

	// Step 6: Directory cleanup.
	cleanupIgnoredPatterns := fileutils.DirectoryCleanupPatterns()

	for dir := range orphanDirs {
		for _, libPath := range library.LibraryPaths {
			if strings.HasPrefix(dir, libPath.Filepath) {
				if err := fileutils.CleanupEmptyParentDirectories(dir, libPath.Filepath, cleanupIgnoredPatterns...); err != nil {
					jobLog.Warn("failed to cleanup empty directories", logger.Data{"path": dir, "error": err.Error()})
				}
				break
			}
		}
	}

	jobLog.Info("batch orphan cleanup complete", logger.Data{
		"missing_supplements_attempted": len(missingIDs),
		"partial_files_attempted":       len(partialOrphanFileIDs),
		"promoted_files_attempted":      len(promotedBookOrphanFileIDs),
		"books_attempted":               len(bookIDsToDelete),
	})
}

// rediscoverSupplements runs supplement discovery for an existing book, as a
// main file import does.
func (w *Worker) rediscoverSupplements(ctx context.Context, bookID int, library *models.Library, jobLog *joblogs.JobLogger) {
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
	if err != nil {
		var errCode *errcodes.Error
		if !errors.As(err, &errCode) || errCode.Code != "not_found" {
			jobLog.Warn("failed to retrieve book for supplement discovery", logger.Data{"book_id": bookID, "error": err.Error()})
		}
		return
	}
	for _, f := range book.Files {
		if f.FileRole != models.FileRoleMain {
			continue
		}
		isRootLevelFile := false
		for _, libraryPath := range library.LibraryPaths {
			if filepath.Dir(f.Filepath) == libraryPath.Filepath {
				isRootLevelFile = true
				break
			}
		}
		w.discoverAndCreateSupplements(ctx, book, f.Filepath, isRootLevelFile, library.ID, library, jobLog)
		return
	}
}

// missingSupplements returns the library's supplements whose files are gone
// from disk. It stats each one: the scannable-file list cannot answer this,
// since most supplements (.txt, .jpg, and so on) never appear in it. Only a
// definite not-exist counts as missing; another stat error leaves the row
// alone and logs a warning.
func (w *Worker) missingSupplements(ctx context.Context, libraryID int, jobLog *joblogs.JobLogger) []*models.File {
	files, err := w.bookService.ListAllFilesForLibrary(ctx, libraryID)
	if err != nil {
		jobLog.Warn("failed to list supplements for orphan cleanup", logger.Data{"error": err.Error()})
		return nil
	}
	stat := w.statFile
	if stat == nil {
		stat = os.Stat
	}
	var missing []*models.File
	for _, f := range files {
		if f.FileRole != models.FileRoleSupplement {
			continue
		}
		_, err := stat(f.Filepath)
		if err == nil {
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			jobLog.Warn("could not check whether supplement exists, keeping it", logger.Data{
				"file_id":  f.ID,
				"filepath": f.Filepath,
				"error":    err.Error(),
			})
			continue
		}
		missing = append(missing, f)
	}
	return missing
}
