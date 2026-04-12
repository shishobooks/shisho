package worker

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

// cleanupOrphanedFiles batch-cleans files that exist in the database but were not found on disk
// during the scan. This replaces the previous sequential scanInternal loop with batch operations.
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

	// Step 1: Collect orphans and group by book.
	// existingFiles only contains main files (from ListFilesForLibrary).
	totalFilesByBook := make(map[int]int)         // bookID → total main file count
	orphansByBook := make(map[int][]*models.File) // bookID → orphaned files

	for _, file := range existingFiles {
		totalFilesByBook[file.BookID]++
		if _, seen := scannedPaths[file.Filepath]; !seen {
			// Skip files that were already reconciled as moves — they have a
			// valid updated filepath and must not be deleted.
			if sc != nil && sc.IsMovedOrphan(file.ID) {
				continue
			}
			orphansByBook[file.BookID] = append(orphansByBook[file.BookID], file)
		}
	}

	if len(orphansByBook) == 0 {
		return
	}

	jobLog.Info("batch orphan cleanup starting", logger.Data{
		"orphaned_books": len(orphansByBook),
	})

	// Collect directories for cleanup at the end
	orphanDirs := make(map[string]struct{})

	// Step 2 & 3: Handle partial orphan books.
	// Collect file IDs from books where only SOME main files are orphaned.
	var partialOrphanFileIDs []int
	partialOrphanBookIDs := make(map[int]struct{}) // books that need primary file check

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
			partialOrphanBookIDs[bookID] = struct{}{}
		}
	}

	// Batch-delete partial orphan files
	if len(partialOrphanFileIDs) > 0 {
		if err := w.bookService.DeleteFilesByIDs(ctx, partialOrphanFileIDs); err != nil {
			jobLog.Warn("failed to batch-delete partial orphan files", logger.Data{"error": err.Error()})
		} else {
			// Promote primary file for affected books
			for bookID := range partialOrphanBookIDs {
				if err := w.bookService.PromoteNextPrimaryFile(ctx, bookID); err != nil {
					jobLog.Warn("failed to promote primary file", logger.Data{"book_id": bookID, "error": err.Error()})
				}
			}
		}
	}

	// Step 4: Handle full orphan books.
	// Build supported types set for supplement promotion.
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
		models.FileTypePDF:  {},
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
			if err := w.bookService.PromoteNextPrimaryFile(ctx, bookID); err != nil {
				jobLog.Warn("failed to promote primary file", logger.Data{"book_id": bookID, "error": err.Error()})
			}
			continue
		}

		// Collect supplements (files with supplement role)
		var supplements []*models.File
		for i := range book.Files {
			if book.Files[i].FileRole == models.FileRoleSupplement {
				supplements = append(supplements, book.Files[i])
			}
		}

		// Try to promote a supplement
		var promoted bool
		for _, supp := range supplements {
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
			// Promote primary file to the newly promoted supplement
			if err := w.bookService.PromoteNextPrimaryFile(ctx, bookID); err != nil {
				jobLog.Warn("failed to promote primary file after supplement promotion", logger.Data{"book_id": bookID, "error": err.Error()})
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

	// Step 5: Directory cleanup.
	cleanupIgnoredPatterns := make([]string, 0, len(fileutils.ShishoSpecialFilePatterns)+len(w.config.SupplementExcludePatterns))
	cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, fileutils.ShishoSpecialFilePatterns...)
	cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, w.config.SupplementExcludePatterns...)

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
		"partial_files_attempted":  len(partialOrphanFileIDs),
		"promoted_files_attempted": len(promotedBookOrphanFileIDs),
		"books_attempted":          len(bookIDsToDelete),
	})
}
