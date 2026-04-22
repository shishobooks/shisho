package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/chapters"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/shishobooks/shisho/pkg/pdf"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
)

// RelationshipUpdates holds all relationship data to be updated for a book.
// This enables bulk inserts for better scan performance. When used in parallel
// processing, callers should use ScanCache.LockBook to prevent race conditions.
type RelationshipUpdates struct {
	Authors    []*models.Author
	Narrators  []*models.Narrator
	BookGenres []*models.BookGenre
	BookTags   []*models.BookTag
	BookSeries []*models.BookSeries

	// Delete flags indicate which relationships should be cleared before inserting new ones
	DeleteAuthors   bool
	DeleteNarrators bool
	DeleteGenres    bool
	DeleteTags      bool
	DeleteSeries    bool

	// FileID is required when DeleteNarrators is true (narrators are per-file)
	FileID int
}

// UpdateBookRelationships updates all book relationships using the books service methods.
// It first deletes existing relationships (if flagged) then bulk inserts new ones.
// This method should be called with appropriate locking when used in parallel processing.
func (w *Worker) UpdateBookRelationships(ctx context.Context, bookID int, updates RelationshipUpdates) error {
	// Delete existing relationships if flagged
	if updates.DeleteAuthors {
		if err := w.bookService.DeleteAuthors(ctx, bookID); err != nil {
			return errors.Wrap(err, "failed to delete authors")
		}
	}
	if updates.DeleteNarrators && updates.FileID != 0 {
		if _, err := w.bookService.DeleteNarratorsForFile(ctx, updates.FileID); err != nil {
			return errors.Wrap(err, "failed to delete narrators")
		}
	}
	if updates.DeleteGenres {
		if err := w.bookService.DeleteBookGenres(ctx, bookID); err != nil {
			return errors.Wrap(err, "failed to delete genres")
		}
	}
	if updates.DeleteTags {
		if err := w.bookService.DeleteBookTags(ctx, bookID); err != nil {
			return errors.Wrap(err, "failed to delete tags")
		}
	}
	if updates.DeleteSeries {
		if err := w.bookService.DeleteBookSeries(ctx, bookID); err != nil {
			return errors.Wrap(err, "failed to delete series")
		}
	}

	// Bulk insert new relationships using service methods
	if err := w.bookService.BulkCreateAuthors(ctx, updates.Authors); err != nil {
		return errors.Wrap(err, "failed to insert authors")
	}
	if err := w.bookService.BulkCreateNarrators(ctx, updates.Narrators); err != nil {
		return errors.Wrap(err, "failed to insert narrators")
	}
	if err := w.bookService.BulkCreateBookGenres(ctx, updates.BookGenres); err != nil {
		return errors.Wrap(err, "failed to insert genres")
	}
	if err := w.bookService.BulkCreateBookTags(ctx, updates.BookTags); err != nil {
		return errors.Wrap(err, "failed to insert tags")
	}
	if err := w.bookService.BulkCreateBookSeries(ctx, updates.BookSeries); err != nil {
		return errors.Wrap(err, "failed to insert series")
	}

	return nil
}

// ErrInvalidScanOptions is returned when ScanOptions validation fails.
var ErrInvalidScanOptions = errors.New("exactly one of FilePath, FileID, or BookID must be set")

// ScanOptions configures a scan operation.
//
// Entry points are mutually exclusive - exactly one of FilePath, FileID, or BookID must be set:
//   - FilePath: Batch scan mode - discover or create file/book records by path.
//     Requires LibraryID to be set.
//   - FileID: Single file resync - file already exists in DB. If the file no longer
//     exists on disk, it will be deleted from the database.
//   - BookID: Book resync - scan all files belonging to the book. If the book has
//     no files, it will be deleted.
type ScanOptions struct {
	// Entry points (mutually exclusive - exactly one must be set)
	FilePath string // Batch scan: discover/create by path
	FileID   int    // Single file resync: file already in DB
	BookID   int    // Book resync: scan all files in book

	// Context (required for FilePath mode)
	LibraryID int

	// Behavior
	ForceRefresh  bool // Bypass priority checks, overwrite all metadata
	SkipPlugins   bool // Skip enricher plugins, use only file-embedded metadata
	Reset         bool // Wipe all metadata before scanning (reset to file-only state)
	BookResetDone bool // Book-level wipe already done by scanBook (skip in scanFileByID)

	// Logging (optional, for batch scan job context)
	JobLog *joblogs.JobLogger
}

// ScanResult contains the results of a scan operation.
//
// For single file scans (FilePath or FileID mode), the File and Book fields contain
// the scanned/updated records. FileCreated indicates whether a new file record was
// created (only possible in FilePath mode). FileDeleted and BookDeleted indicate
// whether records were removed because the file no longer exists on disk.
//
// For book scans (BookID mode), the Files slice contains the results for each
// individual file in the book. The top-level Book field contains the updated book
// record (unless BookDeleted is true).
type ScanResult struct {
	// For single file scans
	File        *models.File // The scanned/updated file (nil if deleted)
	Book        *models.Book // The parent book (nil if deleted)
	FileCreated bool         // True if file was newly created (FilePath mode only)
	FileDeleted bool         // True if file was deleted (no longer on disk)
	BookDeleted bool         // True if book was also deleted (was last file)

	// For book scans (multiple files)
	Files []*ScanResult // Results for each file in the book (BookID mode only)
}

// scanInternal is the unified entry point for all scan operations using internal types.
//
// It validates that exactly one of FilePath, FileID, or BookID is set in options,
// then routes to the appropriate internal handler:
//   - FilePath: scanFileByPath (batch scan mode)
//   - FileID: scanFileByID (single file resync)
//   - BookID: scanBook (book resync)
//
// The optional cache parameter enables shared entity lookups across parallel file processing.
// When cache is nil, direct service calls are used (backward compatible).
//
// The public Scan method wraps this to implement books.Scanner.
//
//nolint:unparam // cache will be used in parallel scan mode (Task 2)
func (w *Worker) scanInternal(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
	// Count how many entry points are set
	entryPoints := 0
	if opts.FilePath != "" {
		entryPoints++
	}
	if opts.FileID != 0 {
		entryPoints++
	}
	if opts.BookID != 0 {
		entryPoints++
	}

	// Validate exactly one entry point
	if entryPoints != 1 {
		return nil, ErrInvalidScanOptions
	}

	// Route to appropriate handler
	switch {
	case opts.FilePath != "":
		return w.scanFileByPath(ctx, opts, cache)
	case opts.FileID != 0:
		return w.scanFileByID(ctx, opts, cache)
	case opts.BookID != 0:
		return w.scanBook(ctx, opts, cache)
	default:
		// This should never happen due to validation above
		return nil, ErrInvalidScanOptions
	}
}

// fileContentChanged reports whether a file's on-disk content differs from
// what was last scanned into the DB. It compares size + mtime (truncated to
// seconds, since SQLite drops sub-second precision). ForceRefresh forces a
// "changed" result so the caller re-parses metadata; a missing
// FileModifiedAt on the existing row also forces "changed" since we can't
// reason about it.
//
// Returns an error only if stat fails for a reason other than "not exists".
func fileContentChanged(path string, existing *models.File, forceRefresh bool) (bool, error) {
	if forceRefresh {
		return true, nil
	}
	if existing.FileModifiedAt == nil {
		return true, nil
	}
	stat, err := os.Stat(path)
	if err != nil {
		return true, err
	}
	if stat.Size() != existing.FilesizeBytes {
		return true, nil
	}
	if !stat.ModTime().Truncate(time.Second).Equal(existing.FileModifiedAt.Truncate(time.Second)) {
		return true, nil
	}
	return false, nil
}

// scanFileByPath handles batch scan mode - discovering or creating file/book records by path.
// If the file already exists in DB, delegates to scanFileByID.
// If the file doesn't exist on disk, returns nil (skip silently).
// If the file exists on disk but not in DB, creates a new file/book record.
func (w *Worker) scanFileByPath(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
	// Validate LibraryID is required for path-based scan
	if opts.LibraryID == 0 {
		return nil, errors.New("LibraryID required for FilePath mode")
	}

	// Fast path: check pre-loaded cache for known file (avoids per-file DB query)
	if cache != nil {
		if existingFile := cache.GetKnownFile(opts.FilePath); existingFile != nil {
			// Supplement files share scannable extensions (.pdf/.epub/.cbz/.m4b)
			// with main files, so the walk picks them up — but they have no
			// metadata to rescan and must not be routed through the main-file
			// creation path (which would try to insert a duplicate row and hit
			// UNIQUE(filepath, library_id)). The supplement is already tracked,
			// so we just need to keep its fingerprint in sync with its content:
			// if the supplement's bytes changed on disk, drop the stored
			// fingerprint so the next hash generation job recomputes it.
			if existingFile.FileRole == models.FileRoleSupplement {
				if changed, changedErr := fileContentChanged(opts.FilePath, existingFile, opts.ForceRefresh); changedErr == nil && changed {
					if w.fingerprintService != nil {
						if err := w.fingerprintService.DeleteForFile(ctx, existingFile.ID); err != nil {
							return nil, errors.Wrap(err, "invalidate stale supplement fingerprints")
						}
					}
				}
				return &ScanResult{File: existingFile}, nil
			}
			// File exists in DB — check if it changed on disk. If size/mtime
			// match what's stored, the content hasn't changed and we can skip
			// both re-parsing and fingerprint invalidation.
			changed, changedErr := fileContentChanged(opts.FilePath, existingFile, opts.ForceRefresh)
			if changedErr == nil && !changed {
				return &ScanResult{File: existingFile, Book: existingFile.Book}, nil
			}
			// File content changed (or we can't tell) — invalidate stale
			// fingerprints so the next hash generation job recomputes them
			// against the new content, then delegate to scanFileByID.
			if w.fingerprintService != nil {
				if err := w.fingerprintService.DeleteForFile(ctx, existingFile.ID); err != nil {
					return nil, errors.Wrap(err, "invalidate stale fingerprints")
				}
			}
			return w.scanFileByID(ctx, ScanOptions{
				FileID:       existingFile.ID,
				ForceRefresh: opts.ForceRefresh,
				SkipPlugins:  opts.SkipPlugins,
				JobLog:       opts.JobLog,
			}, cache)
		}
	} else {
		// No cache — fall back to per-file DB query (single-file rescan path)
		existingFile, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
			Filepath:  &opts.FilePath,
			LibraryID: &opts.LibraryID,
		})
		if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
			return nil, errors.Wrap(err, "failed to check if file exists")
		}
		if existingFile != nil {
			// Same change-detection shortcut as the cache-hit path: if the
			// file's size/mtime match what's in the DB, the content is
			// unchanged and we must not invalidate its fingerprint.
			changed, changedErr := fileContentChanged(opts.FilePath, existingFile, opts.ForceRefresh)
			if changedErr == nil && !changed {
				return &ScanResult{File: existingFile, Book: existingFile.Book}, nil
			}
			// File content changed (or we can't tell) — invalidate stale
			// fingerprints before delegating.
			if w.fingerprintService != nil {
				if err := w.fingerprintService.DeleteForFile(ctx, existingFile.ID); err != nil {
					return nil, errors.Wrap(err, "invalidate stale fingerprints")
				}
			}
			return w.scanFileByID(ctx, ScanOptions{
				FileID:       existingFile.ID,
				ForceRefresh: opts.ForceRefresh,
				SkipPlugins:  opts.SkipPlugins,
				JobLog:       opts.JobLog,
			}, cache)
		}
	}

	// File doesn't exist in DB - check if it exists on disk
	_, err := os.Stat(opts.FilePath)
	if os.IsNotExist(err) {
		// File doesn't exist on disk - skip silently
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}

	// File exists on disk but not in DB - parse metadata and create new record
	return w.scanFileCreateNew(ctx, opts, cache)
}

// scanFileByID handles single file resync - file already exists in DB.
// If the file no longer exists on disk, deletes the file record (and book if it was the last file).
func (w *Worker) scanFileByID(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Warn(msg, data)
		}
	}

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Info(msg, data)
		}
	}

	// Patterns for files that should be treated as ignorable during directory cleanup
	// (shisho covers/sidecars + OS junk like .DS_Store).
	cleanupIgnoredPatterns := make([]string, 0, len(fileutils.ShishoSpecialFilePatterns)+len(w.config.SupplementExcludePatterns))
	cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, fileutils.ShishoSpecialFilePatterns...)
	cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, w.config.SupplementExcludePatterns...)

	// Retrieve file with relations from DB
	file, err := w.bookService.RetrieveFileWithRelations(ctx, opts.FileID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve file")
	}

	// Check if file exists on disk
	fileStat, err := os.Stat(file.Filepath)
	if os.IsNotExist(err) {
		logInfo("file no longer exists on disk, deleting record", logger.Data{"file_id": file.ID, "path": file.Filepath})

		// Get parent book to check file count
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve parent book")
		}

		fileDir := filepath.Dir(file.Filepath)
		bookPath := book.Filepath

		// Delete the file record (this also handles primary_file_id promotion)
		if err := w.bookService.DeleteFile(ctx, file.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete file record")
		}

		// Check if any main files remain for this book
		var hasMainFiles bool
		for _, f := range book.Files {
			if f.ID != file.ID && f.FileRole == models.FileRoleMain {
				hasMainFiles = true
				break
			}
		}

		bookDeleted := false
		if hasMainFiles {
			// Other main files exist, just clean up empty dirs
			if fileDir != bookPath {
				if err := fileutils.CleanupEmptyParentDirectories(fileDir, bookPath, cleanupIgnoredPatterns...); err != nil {
					logWarn("failed to cleanup empty directories", logger.Data{"path": fileDir, "error": err.Error()})
				}
			}
		} else {
			// No main files remain - check if we can promote a supplement
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

			// Collect remaining supplements (excluding the deleted file)
			var supplements []*models.File
			for i := range book.Files {
				f := book.Files[i]
				if f.ID != file.ID && f.FileRole == models.FileRoleSupplement {
					supplements = append(supplements, f)
				}
			}

			// Try to promote a supplement with a supported file type
			var promoted bool
			for _, supp := range supplements {
				if _, supported := supportedTypes[supp.FileType]; supported {
					if err := w.bookService.PromoteSupplementToMain(ctx, supp.ID); err != nil {
						logWarn("failed to promote supplement to main", logger.Data{"file_id": supp.ID, "error": err.Error()})
					} else {
						logInfo("promoted supplement to main file", logger.Data{"file_id": supp.ID, "book_id": book.ID})
						promoted = true
					}
					break
				}
			}

			if !promoted {
				// No promotable supplements - delete remaining supplements and the book
				bookDeleted = true
				for _, supp := range supplements {
					if err := w.bookService.DeleteFile(ctx, supp.ID); err != nil {
						logWarn("failed to delete supplement file", logger.Data{"file_id": supp.ID, "error": err.Error()})
					}
				}
			}

			if bookDeleted {
				// Delete from search index before deleting the book
				if w.searchService != nil {
					if err := w.searchService.DeleteFromBookIndex(ctx, book.ID); err != nil {
						logWarn("failed to delete book from search index", logger.Data{"book_id": book.ID, "error": err.Error()})
					}
				}
				if err := w.bookService.DeleteBook(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete orphaned book")
				}
				logInfo("deleted orphaned book", logger.Data{"book_id": book.ID})

				// Clean up empty directories up to library path
				library, libErr := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
					ID: &book.LibraryID,
				})
				if libErr == nil && library != nil {
					for _, libPath := range library.LibraryPaths {
						if strings.HasPrefix(bookPath, libPath.Filepath) {
							if err := fileutils.CleanupEmptyParentDirectories(bookPath, libPath.Filepath, cleanupIgnoredPatterns...); err != nil {
								logWarn("failed to cleanup empty directories", logger.Data{"path": bookPath, "error": err.Error()})
							}
							break
						}
					}
				}
			} else if fileDir != bookPath {
				if err := fileutils.CleanupEmptyParentDirectories(fileDir, bookPath, cleanupIgnoredPatterns...); err != nil {
					logWarn("failed to cleanup empty directories", logger.Data{"path": fileDir, "error": err.Error()})
				}
			}
		}

		// Return a minimal Book stub so callers (e.g. the monitor's
		// search-index pruning loop) can track the deleted book's ID even
		// though the row no longer exists. scanFileByID already pruned the
		// search index above; callers that also look up RetrieveBook for
		// this ID will get NotFound and fall through to DeleteFromBookIndex
		// as a redundant safety net.
		var bookStub *models.Book
		if bookDeleted {
			bookStub = &models.Book{ID: book.ID}
		}
		return &ScanResult{
			FileDeleted: true,
			BookDeleted: bookDeleted,
			Book:        bookStub,
		}, nil
	}

	// If stat returned an error other than NotExist, return it
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}

	// Check and recover missing cover if needed
	if err := w.recoverMissingCover(ctx, file, opts.JobLog); err != nil {
		logWarn("failed to recover missing cover", logger.Data{"file_id": file.ID, "error": err.Error()})
	}

	// File exists on disk - parse metadata
	// For supplements (PDF, txt, etc.), derive minimal metadata from filename since they don't have embedded metadata
	var metadata *mediafile.ParsedMetadata
	if file.FileRole == models.FileRoleSupplement {
		// Supplements get their name from the filename (without extension)
		filename := filepath.Base(file.Filepath)
		ext := filepath.Ext(filename)
		nameWithoutExt := strings.TrimSuffix(filename, ext)
		metadata = &mediafile.ParsedMetadata{
			Title:      nameWithoutExt,
			DataSource: models.DataSourceFilepath,
		}
	} else {
		var err error
		metadata, err = w.parseFileMetadata(ctx, file.Filepath, file.FileType)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse file metadata")
		}
	}

	// Get parent book for scanFileCore
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve parent book")
	}

	// Reset mode: wipe metadata and apply filepath fallbacks
	if opts.Reset {
		// Determine if this is a root-level file.
		// For directory-based books, the file's parent dir equals book.Filepath.
		// For root-level files, the file's parent dir is a library path, not book.Filepath.
		isRootLevelFile := filepath.Dir(file.Filepath) != book.Filepath

		// Apply filepath fallbacks so title/authors are populated even if file has none
		applyFilepathFallbacks(metadata, file.Filepath, book.Filepath, file.FileType, isRootLevelFile)

		// Wipe book and file metadata.
		// If BookResetDone is set (called from scanBook), skip the book-level wipe
		// because scanBook already did it once for all files.
		if err := w.resetBookFileState(ctx, book, file, opts.BookResetDone); err != nil {
			return nil, errors.Wrap(err, "failed to reset book/file state")
		}

		// Reload book and file after wipe (relations were deleted)
		book, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to reload book after reset")
		}
		file, err = w.bookService.RetrieveFileWithRelations(ctx, file.ID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to reload file after reset")
		}

		// Re-extract cover from the already-parsed metadata (resetBookFileState
		// deleted the cover file from disk and cleared the DB columns).
		// We use extractAndSaveCover instead of recoverMissingCover to avoid
		// re-parsing the file — metadata.CoverData is already populated.
		coverFilename, coverMime, _, coverErr := w.extractAndSaveCover(ctx, file.Filepath, book.Filepath, isRootLevelFile, metadata, opts.JobLog)
		if coverErr != nil {
			logWarn("failed to extract cover after reset", logger.Data{"error": coverErr.Error()})
		} else if coverFilename != "" {
			coverSource := metadata.SourceForField("cover")
			file.CoverImageFilename = &coverFilename
			file.CoverMimeType = &coverMime
			file.CoverSource = &coverSource
			if metadata.CoverPage != nil {
				file.CoverPage = metadata.CoverPage
			}
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
				Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source", "cover_page"},
			}); err != nil {
				logWarn("failed to update cover after reset", logger.Data{"error": err.Error()})
			}
		}
	}

	// Run metadata enrichers after parsing
	if !opts.SkipPlugins {
		metadata = w.runMetadataEnrichers(ctx, metadata, file, book, file.LibraryID, opts.JobLog)
	}

	// Apply enricher cover if it's higher resolution than the current cover
	w.upgradeEnricherCover(ctx, metadata, file, book.Filepath, opts.JobLog)

	// Use scanFileCore for all metadata updates, sidecars, and search index
	// This is a resync (FileID mode), so pass isResync=true to enable book organization
	result, err := w.scanFileCore(ctx, file, book, metadata, opts.ForceRefresh, true, opts.JobLog, cache)
	if err != nil {
		return nil, err
	}

	// Update stored mod time and size so future rescans can skip unchanged files
	if fileStat != nil {
		modTime := fileStat.ModTime()
		file.FileModifiedAt = &modTime
		file.FilesizeBytes = fileStat.Size()
		if updateErr := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
			Columns: []string{"file_modified_at", "filesize_bytes"},
		}); updateErr != nil {
			logWarn("failed to update file mod time", logger.Data{"error": updateErr.Error()})
		}
	}

	return result, nil
}

// scanBook handles book resync - scan all files belonging to the book.
// It loops through all files in the book, calling scanFileByID for each.
// If the book has no files, it deletes the book from the database.
// Errors from individual file scans are logged and skipped (don't fail entire book scan).
func (w *Worker) scanBook(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Warn(msg, data)
		}
	}

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Info(msg, data)
		}
	}

	// Fetch book with files from DB
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &opts.BookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve book")
	}

	// If book has no files, delete it
	if len(book.Files) == 0 {
		logInfo("book has no files, deleting", logger.Data{"book_id": book.ID})
		bookPath := book.Filepath

		// Delete from search index before deleting the book
		if w.searchService != nil {
			if err := w.searchService.DeleteFromBookIndex(ctx, book.ID); err != nil {
				logWarn("failed to delete book from search index", logger.Data{"book_id": book.ID, "error": err.Error()})
			}
		}

		// Delete book
		if err := w.bookService.DeleteBook(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete empty book")
		}

		// Clean up empty directories up to library path
		cleanupIgnoredPatterns := make([]string, 0, len(fileutils.ShishoSpecialFilePatterns)+len(w.config.SupplementExcludePatterns))
		cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, fileutils.ShishoSpecialFilePatterns...)
		cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, w.config.SupplementExcludePatterns...)
		library, libErr := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
			ID: &book.LibraryID,
		})
		if libErr == nil && library != nil {
			// Find which library path contains this book
			for _, libPath := range library.LibraryPaths {
				if strings.HasPrefix(bookPath, libPath.Filepath) {
					if err := fileutils.CleanupEmptyParentDirectories(bookPath, libPath.Filepath, cleanupIgnoredPatterns...); err != nil {
						logWarn("failed to cleanup empty directories", logger.Data{"path": bookPath, "error": err.Error()})
					}
					break
				}
			}
		}

		return &ScanResult{BookDeleted: true}, nil
	}

	// When resetting, wipe book-level metadata once before iterating files.
	// Per-file calls below pass BookResetDone: opts.Reset so they skip the
	// book-level wipe (which we've already done here).
	if opts.Reset {
		if err := w.resetBookState(ctx, book); err != nil {
			return nil, errors.Wrap(err, "failed to reset book state")
		}
	}

	// Initialize file results
	fileResults := make([]*ScanResult, 0, len(book.Files))

	// Loop through files and scan each
	for _, file := range book.Files {
		fileResult, err := w.scanFileByID(ctx, ScanOptions{
			FileID:        file.ID,
			ForceRefresh:  opts.ForceRefresh,
			SkipPlugins:   opts.SkipPlugins,
			Reset:         opts.Reset,
			BookResetDone: opts.Reset,
			JobLog:        opts.JobLog,
		}, cache)
		if err != nil {
			logWarn("failed to scan file in book, continuing", logger.Data{
				"file_id": file.ID,
				"book_id": book.ID,
				"error":   err.Error(),
			})
			continue
		}

		// If file deletion caused book deletion, return immediately
		if fileResult.BookDeleted {
			return fileResult, nil
		}

		fileResults = append(fileResults, fileResult)
	}

	// Reload book with updated data
	reloadedBook, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &opts.BookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to reload book after scanning files")
	}

	return &ScanResult{
		Book:  reloadedBook,
		Files: fileResults,
	}, nil
}

// scanFileCore updates a file and its parent book with parsed metadata.
// This handles book scalar field updates (Title, SortTitle, Subtitle, Description)
// and book relationship updates (Authors, Series, Genres, Tags).
// File updates (narrators, identifiers) are handled in separate tasks.
//
// Parameters:
//   - file: The file record to update (must already exist in DB)
//   - book: The parent book record to update (must already exist in DB)
//   - metadata: Parsed metadata from the file (can be nil, in which case no updates are made)
//   - forceRefresh: If true, bypass priority checks and overwrite all fields
//   - isResync: True if this is a single file/book resync (not a full library scan).
//     When true, book organization (folder rename) is performed immediately after
//     title/author changes. When false (batch scan), organization is skipped to
//     avoid renaming directories while other files are still being discovered.
//   - cache: Optional ScanCache for shared entity lookups. When nil, direct service
//     calls are used.
//
// Returns a ScanResult with the updated file and book records.
func (w *Worker) scanFileCore(
	ctx context.Context,
	file *models.File,
	book *models.Book,
	metadata *mediafile.ParsedMetadata,
	forceRefresh bool,
	isResync bool,
	jobLog *joblogs.JobLogger,
	cache *ScanCache,
) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if jobLog != nil {
			jobLog.Warn(msg, data)
		}
	}

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if jobLog != nil {
			jobLog.Info(msg, data)
		}
	}

	// If no metadata, nothing to update
	if metadata == nil {
		return &ScanResult{File: file, Book: book}, nil
	}

	sidecarSource := models.DataSourceSidecar

	// Read sidecar files if they exist (higher priority than file metadata)
	// Sidecars can override file metadata but not manual user edits
	bookSidecarData, err := sidecar.ReadBookSidecar(book.Filepath)
	if err != nil {
		logWarn("failed to read book sidecar", logger.Data{"error": err.Error()})
	}
	fileSidecarData, err := sidecar.ReadFileSidecar(file.Filepath)
	if err != nil {
		logWarn("failed to read file sidecar", logger.Data{"error": err.Error()})
	}

	bookUpdateOpts := books.UpdateBookOptions{Columns: []string{}}
	bookTitleChanged := false
	authorsChanged := false

	// Supplements should not update book-level metadata (title, authors, series, etc.)
	// They only update file-level metadata (name, URL, narrators, etc.)
	isMainFile := file.FileRole != models.FileRoleSupplement

	// Collect relationship updates for batch processing
	var relUpdates RelationshipUpdates
	relUpdates.FileID = file.ID // Needed for narrator updates

	// Book-level updates: only for main files, not supplements
	if isMainFile {
		// Title (from metadata)
		title := strings.TrimSpace(metadata.Title)
		titleSource := metadata.SourceForField("title")
		// Normalize volume indicators (e.g., "#007" -> "v7") for CBZ files only when
		// the title came from the file itself or its path. Plugin/sidecar/manual
		// titles are user-curated and must not be rewritten
		// (e.g., "Naruto v1" must not become "Naruto v001").
		if models.GetDataSourcePriority(titleSource) >= models.DataSourceFileMetadataPriority {
			if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, file.FileType); hasVolume {
				title = normalizedTitle
			}
		}
		if shouldUpdateScalar(title, book.Title, titleSource, book.TitleSource, forceRefresh) {
			logInfo("updating book title", logger.Data{"from": book.Title, "to": title})
			book.Title = title
			book.TitleSource = titleSource
			bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "title", "title_source")
			bookTitleChanged = true

			// Regenerate sort title
			newSortTitle := sortname.ForTitle(title)
			if shouldUpdateScalar(newSortTitle, book.SortTitle, titleSource, book.SortTitleSource, forceRefresh) {
				book.SortTitle = newSortTitle
				book.SortTitleSource = titleSource
				bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "sort_title", "sort_title_source")
			}
		}
		// Title (from sidecar - can override filepath-sourced data)
		if bookSidecarData != nil && bookSidecarData.Title != "" {
			if shouldApplySidecarScalar(bookSidecarData.Title, book.Title, book.TitleSource, forceRefresh) {
				logInfo("updating book title from sidecar", logger.Data{"from": book.Title, "to": bookSidecarData.Title})
				book.Title = bookSidecarData.Title
				book.TitleSource = sidecarSource
				bookUpdateOpts.Columns = appendIfMissing(bookUpdateOpts.Columns, "title", "title_source")
				bookTitleChanged = true

				// Regenerate sort title
				newSortTitle := sortname.ForTitle(bookSidecarData.Title)
				if shouldApplySidecarScalar(newSortTitle, book.SortTitle, book.SortTitleSource, forceRefresh) {
					book.SortTitle = newSortTitle
					book.SortTitleSource = sidecarSource
					bookUpdateOpts.Columns = appendIfMissing(bookUpdateOpts.Columns, "sort_title", "sort_title_source")
				}
			}
		}

		// Subtitle (from metadata)
		subtitle := strings.TrimSpace(metadata.Subtitle)
		if subtitle != "" {
			existingSubtitle := ""
			existingSubtitleSource := ""
			if book.Subtitle != nil {
				existingSubtitle = *book.Subtitle
			}
			if book.SubtitleSource != nil {
				existingSubtitleSource = *book.SubtitleSource
			}
			subtitleSource := metadata.SourceForField("subtitle")
			if shouldUpdateScalar(subtitle, existingSubtitle, subtitleSource, existingSubtitleSource, forceRefresh) {
				logInfo("updating book subtitle", logger.Data{"from": existingSubtitle, "to": subtitle})
				book.Subtitle = &subtitle
				book.SubtitleSource = &subtitleSource
				bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "subtitle", "subtitle_source")
			}
		}
		// Subtitle (from sidecar)
		if bookSidecarData != nil && bookSidecarData.Subtitle != nil && *bookSidecarData.Subtitle != "" {
			existingSubtitle := ""
			existingSubtitleSource := ""
			if book.Subtitle != nil {
				existingSubtitle = *book.Subtitle
			}
			if book.SubtitleSource != nil {
				existingSubtitleSource = *book.SubtitleSource
			}
			if shouldApplySidecarScalar(*bookSidecarData.Subtitle, existingSubtitle, existingSubtitleSource, forceRefresh) {
				logInfo("updating book subtitle from sidecar", logger.Data{"from": existingSubtitle, "to": *bookSidecarData.Subtitle})
				book.Subtitle = bookSidecarData.Subtitle
				book.SubtitleSource = &sidecarSource
				bookUpdateOpts.Columns = appendIfMissing(bookUpdateOpts.Columns, "subtitle", "subtitle_source")
			}
		}

		// Description (from metadata) — strip HTML so enricher-provided markup
		// doesn't leak into the stored description. Matches the sidecar branch
		// below and the identify apply path.
		description := htmlutil.StripTags(strings.TrimSpace(metadata.Description))
		if description != "" {
			existingDescription := ""
			existingDescriptionSource := ""
			if book.Description != nil {
				existingDescription = *book.Description
			}
			if book.DescriptionSource != nil {
				existingDescriptionSource = *book.DescriptionSource
			}
			descSource := metadata.SourceForField("description")
			if shouldUpdateScalar(description, existingDescription, descSource, existingDescriptionSource, forceRefresh) {
				logInfo("updating book description", nil)
				book.Description = &description
				book.DescriptionSource = &descSource
				bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "description", "description_source")
			}
		}
		// Description (from sidecar, strip HTML for clean display)
		if bookSidecarData != nil && bookSidecarData.Description != nil && *bookSidecarData.Description != "" {
			sanitizedDesc := htmlutil.StripTags(*bookSidecarData.Description)
			existingDescription := ""
			existingDescriptionSource := ""
			if book.Description != nil {
				existingDescription = *book.Description
			}
			if book.DescriptionSource != nil {
				existingDescriptionSource = *book.DescriptionSource
			}
			if sanitizedDesc != "" && shouldApplySidecarScalar(sanitizedDesc, existingDescription, existingDescriptionSource, forceRefresh) {
				logInfo("updating book description from sidecar", nil)
				book.Description = &sanitizedDesc
				book.DescriptionSource = &sidecarSource
				bookUpdateOpts.Columns = appendIfMissing(bookUpdateOpts.Columns, "description", "description_source")
			}
		}

		// Apply book column updates if any
		if len(bookUpdateOpts.Columns) > 0 {
			if err := w.bookService.UpdateBook(ctx, book, bookUpdateOpts); err != nil {
				return nil, errors.Wrap(err, "failed to update book")
			}
		}

		// Update authors relationship (from metadata)
		if len(metadata.Authors) > 0 {
			authorNames := make([]string, 0, len(metadata.Authors))
			for _, a := range metadata.Authors {
				authorNames = append(authorNames, a.Name)
			}
			existingAuthorNames := make([]string, 0, len(book.Authors))
			for _, a := range book.Authors {
				if a.Person != nil {
					existingAuthorNames = append(existingAuthorNames, a.Person.Name)
				}
			}

			authorSource := metadata.SourceForField("authors")
			if shouldUpdateRelationship(authorNames, existingAuthorNames, authorSource, book.AuthorSource, forceRefresh) {
				logInfo("updating authors", logger.Data{"new_count": len(metadata.Authors), "old_count": len(book.Authors)})

				// Collect authors for batch insert (replaces immediate delete + create)
				relUpdates.DeleteAuthors = true
				relUpdates.Authors = nil // Clear any previous collection
				for i, parsedAuthor := range metadata.Authors {
					var person *models.Person
					var err error
					if cache != nil {
						person, err = cache.GetOrCreatePerson(ctx, parsedAuthor.Name, book.LibraryID, w.personService)
					} else {
						person, err = w.personService.FindOrCreatePerson(ctx, parsedAuthor.Name, book.LibraryID)
					}
					if err != nil {
						logWarn("failed to find/create person for author", logger.Data{"name": parsedAuthor.Name, "error": err.Error()})
						continue
					}
					var role *string
					if parsedAuthor.Role != "" {
						role = &parsedAuthor.Role
					}
					relUpdates.Authors = append(relUpdates.Authors, &models.Author{
						BookID:    book.ID,
						PersonID:  person.ID,
						Role:      role,
						SortOrder: i + 1,
					})
				}

				// Update author source
				book.AuthorSource = authorSource
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"author_source"}}); err != nil {
					return nil, errors.Wrap(err, "failed to update author source")
				}
				authorsChanged = true
			}
		}
		// Update authors relationship (from sidecar)
		if bookSidecarData != nil && len(bookSidecarData.Authors) > 0 {
			sidecarAuthorNames := make([]string, 0, len(bookSidecarData.Authors))
			for _, a := range bookSidecarData.Authors {
				sidecarAuthorNames = append(sidecarAuthorNames, a.Name)
			}
			existingAuthorNames := make([]string, 0, len(book.Authors))
			for _, a := range book.Authors {
				if a.Person != nil {
					existingAuthorNames = append(existingAuthorNames, a.Person.Name)
				}
			}

			if shouldApplySidecarRelationship(sidecarAuthorNames, existingAuthorNames, book.AuthorSource, forceRefresh) {
				logInfo("updating authors from sidecar", logger.Data{"new_count": len(bookSidecarData.Authors), "old_count": len(book.Authors)})

				// Collect authors for batch insert (replaces any metadata collection)
				relUpdates.DeleteAuthors = true
				relUpdates.Authors = nil // Clear previous collection
				for i, sidecarAuthor := range bookSidecarData.Authors {
					var person *models.Person
					var err error
					if cache != nil {
						person, err = cache.GetOrCreatePerson(ctx, sidecarAuthor.Name, book.LibraryID, w.personService)
					} else {
						person, err = w.personService.FindOrCreatePerson(ctx, sidecarAuthor.Name, book.LibraryID)
					}
					if err != nil {
						logWarn("failed to find/create person for author", logger.Data{"name": sidecarAuthor.Name, "error": err.Error()})
						continue
					}
					relUpdates.Authors = append(relUpdates.Authors, &models.Author{
						BookID:    book.ID,
						PersonID:  person.ID,
						Role:      sidecarAuthor.Role,
						SortOrder: i + 1,
					})
				}

				// Update author source
				book.AuthorSource = sidecarSource
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"author_source"}}); err != nil {
					return nil, errors.Wrap(err, "failed to update author source")
				}
				authorsChanged = true
			}
		}

		// Update series relationship (from metadata)
		if metadata.Series != "" {
			newSeriesNames := []string{metadata.Series}
			existingSeriesNames := make([]string, 0, len(book.BookSeries))
			existingSeriesSource := ""
			for _, bs := range book.BookSeries {
				if bs.Series != nil {
					existingSeriesNames = append(existingSeriesNames, bs.Series.Name)
					// Use the first series's name source for comparison
					if existingSeriesSource == "" {
						existingSeriesSource = bs.Series.NameSource
					}
				}
			}

			seriesSource := metadata.SourceForField("series")
			if shouldUpdateRelationship(newSeriesNames, existingSeriesNames, seriesSource, existingSeriesSource, forceRefresh) {
				logInfo("updating series", logger.Data{"new_count": 1, "old_count": len(book.BookSeries)})

				// Collect series for batch insert (replaces immediate delete + create)
				relUpdates.DeleteSeries = true
				relUpdates.BookSeries = nil // Clear any previous collection
				var seriesRecord *models.Series
				var err error
				if cache != nil {
					seriesRecord, err = cache.GetOrCreateSeries(ctx, metadata.Series, book.LibraryID, seriesSource, w.seriesService)
				} else {
					seriesRecord, err = w.seriesService.FindOrCreateSeries(ctx, metadata.Series, book.LibraryID, seriesSource)
				}
				if err != nil {
					logWarn("failed to find/create series", logger.Data{"name": metadata.Series, "error": err.Error()})
				} else {
					relUpdates.BookSeries = append(relUpdates.BookSeries, &models.BookSeries{
						BookID:       book.ID,
						SeriesID:     seriesRecord.ID,
						SeriesNumber: metadata.SeriesNumber,
						SortOrder:    1,
					})
				}
			}
		}
		// Update series relationship (from sidecar)
		if bookSidecarData != nil && len(bookSidecarData.Series) > 0 {
			sidecarSeriesNames := make([]string, 0, len(bookSidecarData.Series))
			for _, s := range bookSidecarData.Series {
				if s.Name != "" {
					sidecarSeriesNames = append(sidecarSeriesNames, s.Name)
				}
			}
			existingSeriesNames := make([]string, 0, len(book.BookSeries))
			existingSeriesSource := ""
			for _, bs := range book.BookSeries {
				if bs.Series != nil {
					existingSeriesNames = append(existingSeriesNames, bs.Series.Name)
					if existingSeriesSource == "" {
						existingSeriesSource = bs.Series.NameSource
					}
				}
			}

			if len(sidecarSeriesNames) > 0 && shouldApplySidecarRelationship(sidecarSeriesNames, existingSeriesNames, existingSeriesSource, forceRefresh) {
				logInfo("updating series from sidecar", logger.Data{"new_count": len(bookSidecarData.Series), "old_count": len(book.BookSeries)})

				// Collect series for batch insert (replaces any metadata collection)
				relUpdates.DeleteSeries = true
				relUpdates.BookSeries = nil // Clear previous collection
				for i, sidecarSeries := range bookSidecarData.Series {
					if sidecarSeries.Name == "" {
						continue
					}
					var seriesRecord *models.Series
					var err error
					if cache != nil {
						seriesRecord, err = cache.GetOrCreateSeries(ctx, sidecarSeries.Name, book.LibraryID, sidecarSource, w.seriesService)
					} else {
						seriesRecord, err = w.seriesService.FindOrCreateSeries(ctx, sidecarSeries.Name, book.LibraryID, sidecarSource)
					}
					if err != nil {
						logWarn("failed to find/create series", logger.Data{"name": sidecarSeries.Name, "error": err.Error()})
						continue
					}
					relUpdates.BookSeries = append(relUpdates.BookSeries, &models.BookSeries{
						BookID:       book.ID,
						SeriesID:     seriesRecord.ID,
						SeriesNumber: sidecarSeries.Number,
						SortOrder:    i + 1,
					})
				}
			}
		}

		// Update genres relationship (from metadata)
		if len(metadata.Genres) > 0 {
			existingGenreNames := make([]string, 0, len(book.BookGenres))
			existingGenreSource := ""
			if book.GenreSource != nil {
				existingGenreSource = *book.GenreSource
			}
			for _, bg := range book.BookGenres {
				if bg.Genre != nil {
					existingGenreNames = append(existingGenreNames, bg.Genre.Name)
				}
			}

			genreSource := metadata.SourceForField("genres")
			if shouldUpdateRelationship(metadata.Genres, existingGenreNames, genreSource, existingGenreSource, forceRefresh) {
				logInfo("updating genres", logger.Data{"new_count": len(metadata.Genres), "old_count": len(book.BookGenres)})

				// Collect genres for batch insert (replaces immediate delete + create)
				relUpdates.DeleteGenres = true
				relUpdates.BookGenres = nil // Clear any previous collection
				for _, genreName := range metadata.Genres {
					var genreRecord *models.Genre
					var err error
					if cache != nil {
						genreRecord, err = cache.GetOrCreateGenre(ctx, genreName, book.LibraryID, w.genreService)
					} else {
						genreRecord, err = w.genreService.FindOrCreateGenre(ctx, genreName, book.LibraryID)
					}
					if err != nil {
						logWarn("failed to find/create genre", logger.Data{"name": genreName, "error": err.Error()})
						continue
					}
					relUpdates.BookGenres = append(relUpdates.BookGenres, &models.BookGenre{
						BookID:  book.ID,
						GenreID: genreRecord.ID,
					})
				}

				// Update genre source
				book.GenreSource = &genreSource
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"genre_source"}}); err != nil {
					return nil, errors.Wrap(err, "failed to update genre source")
				}
			}
		}
		// Update genres relationship (from sidecar)
		if bookSidecarData != nil && len(bookSidecarData.Genres) > 0 {
			existingGenreNames := make([]string, 0, len(book.BookGenres))
			existingGenreSource := ""
			if book.GenreSource != nil {
				existingGenreSource = *book.GenreSource
			}
			for _, bg := range book.BookGenres {
				if bg.Genre != nil {
					existingGenreNames = append(existingGenreNames, bg.Genre.Name)
				}
			}

			if shouldApplySidecarRelationship(bookSidecarData.Genres, existingGenreNames, existingGenreSource, forceRefresh) {
				logInfo("updating genres from sidecar", logger.Data{"new_count": len(bookSidecarData.Genres), "old_count": len(book.BookGenres)})

				// Collect genres for batch insert (replaces any metadata collection)
				relUpdates.DeleteGenres = true
				relUpdates.BookGenres = nil // Clear previous collection
				for _, genreName := range bookSidecarData.Genres {
					var genreRecord *models.Genre
					var err error
					if cache != nil {
						genreRecord, err = cache.GetOrCreateGenre(ctx, genreName, book.LibraryID, w.genreService)
					} else {
						genreRecord, err = w.genreService.FindOrCreateGenre(ctx, genreName, book.LibraryID)
					}
					if err != nil {
						logWarn("failed to find/create genre", logger.Data{"name": genreName, "error": err.Error()})
						continue
					}
					relUpdates.BookGenres = append(relUpdates.BookGenres, &models.BookGenre{
						BookID:  book.ID,
						GenreID: genreRecord.ID,
					})
				}

				// Update genre source
				book.GenreSource = &sidecarSource
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"genre_source"}}); err != nil {
					return nil, errors.Wrap(err, "failed to update genre source")
				}
			}
		}

		// Update tags relationship (from metadata)
		if len(metadata.Tags) > 0 {
			existingTagNames := make([]string, 0, len(book.BookTags))
			existingTagSource := ""
			if book.TagSource != nil {
				existingTagSource = *book.TagSource
			}
			for _, bt := range book.BookTags {
				if bt.Tag != nil {
					existingTagNames = append(existingTagNames, bt.Tag.Name)
				}
			}

			tagSource := metadata.SourceForField("tags")
			if shouldUpdateRelationship(metadata.Tags, existingTagNames, tagSource, existingTagSource, forceRefresh) {
				logInfo("updating tags", logger.Data{"new_count": len(metadata.Tags), "old_count": len(book.BookTags)})

				// Collect tags for batch insert (replaces immediate delete + create)
				relUpdates.DeleteTags = true
				relUpdates.BookTags = nil // Clear any previous collection
				for _, tagName := range metadata.Tags {
					var tagRecord *models.Tag
					var err error
					if cache != nil {
						tagRecord, err = cache.GetOrCreateTag(ctx, tagName, book.LibraryID, w.tagService)
					} else {
						tagRecord, err = w.tagService.FindOrCreateTag(ctx, tagName, book.LibraryID)
					}
					if err != nil {
						logWarn("failed to find/create tag", logger.Data{"name": tagName, "error": err.Error()})
						continue
					}
					relUpdates.BookTags = append(relUpdates.BookTags, &models.BookTag{
						BookID: book.ID,
						TagID:  tagRecord.ID,
					})
				}

				// Update tag source
				book.TagSource = &tagSource
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"tag_source"}}); err != nil {
					return nil, errors.Wrap(err, "failed to update tag source")
				}
			}
		}
		// Update tags relationship (from sidecar)
		if bookSidecarData != nil && len(bookSidecarData.Tags) > 0 {
			existingTagNames := make([]string, 0, len(book.BookTags))
			existingTagSource := ""
			if book.TagSource != nil {
				existingTagSource = *book.TagSource
			}
			for _, bt := range book.BookTags {
				if bt.Tag != nil {
					existingTagNames = append(existingTagNames, bt.Tag.Name)
				}
			}

			if shouldApplySidecarRelationship(bookSidecarData.Tags, existingTagNames, existingTagSource, forceRefresh) {
				logInfo("updating tags from sidecar", logger.Data{"new_count": len(bookSidecarData.Tags), "old_count": len(book.BookTags)})

				// Collect tags for batch insert (replaces any metadata collection)
				relUpdates.DeleteTags = true
				relUpdates.BookTags = nil // Clear previous collection
				for _, tagName := range bookSidecarData.Tags {
					var tagRecord *models.Tag
					var err error
					if cache != nil {
						tagRecord, err = cache.GetOrCreateTag(ctx, tagName, book.LibraryID, w.tagService)
					} else {
						tagRecord, err = w.tagService.FindOrCreateTag(ctx, tagName, book.LibraryID)
					}
					if err != nil {
						logWarn("failed to find/create tag", logger.Data{"name": tagName, "error": err.Error()})
						continue
					}
					relUpdates.BookTags = append(relUpdates.BookTags, &models.BookTag{
						BookID: book.ID,
						TagID:  tagRecord.ID,
					})
				}

				// Update tag source
				book.TagSource = &sidecarSource
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"tag_source"}}); err != nil {
					return nil, errors.Wrap(err, "failed to update tag source")
				}
			}
		}

		// Reorganize book directory on disk if title or authors changed and library has OrganizeFileStructure enabled
		// Only do this during resyncs - during full scans, organization would rename directories while
		// other files are still being discovered/processed, breaking the scan
		if (bookTitleChanged || authorsChanged) && isResync {
			// Reload book to get fresh author data for organization
			book, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
			if err != nil {
				logWarn("failed to reload book for organization", logger.Data{"error": err.Error()})
			} else {
				// Call UpdateBook with OrganizeFiles flag to trigger file/folder organization
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{OrganizeFiles: true}); err != nil {
					logWarn("failed to organize book files after title/author change", logger.Data{
						"book_id": book.ID,
						"error":   err.Error(),
					})
				} else {
					// Reload book again to get updated file paths
					book, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
					if err != nil {
						logWarn("failed to reload book after organization", logger.Data{"error": err.Error()})
					}
					// Also reload file to get updated filepath
					file, err = w.bookService.RetrieveFileWithRelations(ctx, file.ID)
					if err != nil {
						logWarn("failed to reload file after organization", logger.Data{"error": err.Error()})
					}
				}
			}
		}
	} // end if isMainFile

	// ==========================================================================
	// File updates (applies to both main files and supplements)
	// ==========================================================================

	fileUpdateOpts := books.UpdateFileOptions{Columns: []string{}}

	// File name (from metadata title)
	// For CBZ: use generateCBZFileName which handles series+number formatting
	// For M4B/EPUB: use the title directly as the file name
	var newFileName string
	if file.FileType == models.FileTypeCBZ {
		filename := filepath.Base(file.Filepath)
		newFileName = generateCBZFileName(metadata, filename)
	} else if metadata.Title != "" {
		// For M4B and EPUB, use the title as the file name
		newFileName = strings.TrimSpace(metadata.Title)
	}
	if newFileName != "" {
		existingName := ""
		existingNameSource := ""
		if file.Name != nil {
			existingName = *file.Name
		}
		if file.NameSource != nil {
			existingNameSource = *file.NameSource
		}
		nameSource := metadata.SourceForField("title")
		if shouldUpdateScalar(newFileName, existingName, nameSource, existingNameSource, forceRefresh) {
			logInfo("updating file name", logger.Data{"from": existingName, "to": newFileName})
			file.Name = &newFileName
			file.NameSource = &nameSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "name", "name_source")
		}
	}
	// File name (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Name != nil && *fileSidecarData.Name != "" {
		existingName := ""
		existingNameSource := ""
		if file.Name != nil {
			existingName = *file.Name
		}
		if file.NameSource != nil {
			existingNameSource = *file.NameSource
		}
		if shouldApplySidecarScalar(*fileSidecarData.Name, existingName, existingNameSource, forceRefresh) {
			logInfo("updating file name from sidecar", logger.Data{"from": existingName, "to": *fileSidecarData.Name})
			file.Name = fileSidecarData.Name
			file.NameSource = &sidecarSource
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "name", "name_source")
		}
	}

	// URL (from metadata)
	if metadata.URL != "" {
		existingURL := ""
		existingURLSource := ""
		if file.URL != nil {
			existingURL = *file.URL
		}
		if file.URLSource != nil {
			existingURLSource = *file.URLSource
		}
		urlSource := metadata.SourceForField("url")
		if shouldUpdateScalar(metadata.URL, existingURL, urlSource, existingURLSource, forceRefresh) {
			logInfo("updating file URL", logger.Data{"from": existingURL, "to": metadata.URL})
			file.URL = &metadata.URL
			file.URLSource = &urlSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "url", "url_source")
		}
	}
	// URL (from sidecar)
	if fileSidecarData != nil && fileSidecarData.URL != nil && *fileSidecarData.URL != "" {
		existingURL := ""
		existingURLSource := ""
		if file.URL != nil {
			existingURL = *file.URL
		}
		if file.URLSource != nil {
			existingURLSource = *file.URLSource
		}
		if shouldApplySidecarScalar(*fileSidecarData.URL, existingURL, existingURLSource, forceRefresh) {
			logInfo("updating file URL from sidecar", logger.Data{"from": existingURL, "to": *fileSidecarData.URL})
			file.URL = fileSidecarData.URL
			file.URLSource = &sidecarSource
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "url", "url_source")
		}
	}

	// ReleaseDate (from metadata)
	if metadata.ReleaseDate != nil {
		existingReleaseDateSource := ""
		if file.ReleaseDateSource != nil {
			existingReleaseDateSource = *file.ReleaseDateSource
		}
		// Convert dates to strings for comparison
		newDateStr := metadata.ReleaseDate.Format("2006-01-02")
		existingDateStr := ""
		if file.ReleaseDate != nil {
			existingDateStr = file.ReleaseDate.Format("2006-01-02")
		}
		releaseDateSource := metadata.SourceForField("releaseDate")
		if shouldUpdateScalar(newDateStr, existingDateStr, releaseDateSource, existingReleaseDateSource, forceRefresh) {
			logInfo("updating file release date", logger.Data{"from": existingDateStr, "to": newDateStr})
			file.ReleaseDate = metadata.ReleaseDate
			file.ReleaseDateSource = &releaseDateSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "release_date", "release_date_source")
		}
	}
	// ReleaseDate (from sidecar)
	if fileSidecarData != nil && fileSidecarData.ReleaseDate != nil && *fileSidecarData.ReleaseDate != "" {
		existingReleaseDateSource := ""
		if file.ReleaseDateSource != nil {
			existingReleaseDateSource = *file.ReleaseDateSource
		}
		existingDateStr := ""
		if file.ReleaseDate != nil {
			existingDateStr = file.ReleaseDate.Format("2006-01-02")
		}
		if shouldApplySidecarScalar(*fileSidecarData.ReleaseDate, existingDateStr, existingReleaseDateSource, forceRefresh) {
			// Parse sidecar date string
			if parsedDate, err := time.Parse("2006-01-02", *fileSidecarData.ReleaseDate); err == nil {
				logInfo("updating file release date from sidecar", logger.Data{"from": existingDateStr, "to": *fileSidecarData.ReleaseDate})
				file.ReleaseDate = &parsedDate
				file.ReleaseDateSource = &sidecarSource
				fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "release_date", "release_date_source")
			} else {
				logWarn("failed to parse sidecar release date", logger.Data{"date": *fileSidecarData.ReleaseDate, "error": err.Error()})
			}
		}
	}

	// Language (from metadata)
	if metadata.Language != nil && *metadata.Language != "" {
		existingLanguage := ""
		existingLanguageSource := ""
		if file.Language != nil {
			existingLanguage = *file.Language
		}
		if file.LanguageSource != nil {
			existingLanguageSource = *file.LanguageSource
		}
		langSource := metadata.SourceForField("language")
		if shouldUpdateScalar(*metadata.Language, existingLanguage, langSource, existingLanguageSource, forceRefresh) {
			logInfo("updating file language", logger.Data{"from": existingLanguage, "to": *metadata.Language})
			file.Language = metadata.Language
			file.LanguageSource = &langSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "language", "language_source")
		}
	}
	// Language (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Language != nil && *fileSidecarData.Language != "" {
		existingLanguage := ""
		existingLanguageSource := ""
		if file.Language != nil {
			existingLanguage = *file.Language
		}
		if file.LanguageSource != nil {
			existingLanguageSource = *file.LanguageSource
		}
		if shouldApplySidecarScalar(*fileSidecarData.Language, existingLanguage, existingLanguageSource, forceRefresh) {
			logInfo("updating file language from sidecar", logger.Data{"from": existingLanguage, "to": *fileSidecarData.Language})
			file.Language = fileSidecarData.Language
			file.LanguageSource = &sidecarSource
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "language", "language_source")
		}
	}

	// Abridged (from metadata)
	if metadata.Abridged != nil {
		existingAbridgedSource := ""
		if file.AbridgedSource != nil {
			existingAbridgedSource = *file.AbridgedSource
		}
		newAbridgedStr := "false"
		if *metadata.Abridged {
			newAbridgedStr = "true"
		}
		existingAbridgedStr := ""
		if file.Abridged != nil {
			if *file.Abridged {
				existingAbridgedStr = "true"
			} else {
				existingAbridgedStr = "false"
			}
		}
		abridgedSource := metadata.SourceForField("abridged")
		if shouldUpdateScalar(newAbridgedStr, existingAbridgedStr, abridgedSource, existingAbridgedSource, forceRefresh) {
			logInfo("updating file abridged", logger.Data{"from": existingAbridgedStr, "to": newAbridgedStr})
			file.Abridged = metadata.Abridged
			file.AbridgedSource = &abridgedSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "abridged", "abridged_source")
		}
	}
	// Abridged (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Abridged != nil {
		existingAbridgedSource := ""
		if file.AbridgedSource != nil {
			existingAbridgedSource = *file.AbridgedSource
		}
		newAbridgedStr := "false"
		if *fileSidecarData.Abridged {
			newAbridgedStr = "true"
		}
		existingAbridgedStr := ""
		if file.Abridged != nil {
			if *file.Abridged {
				existingAbridgedStr = "true"
			} else {
				existingAbridgedStr = "false"
			}
		}
		if shouldApplySidecarScalar(newAbridgedStr, existingAbridgedStr, existingAbridgedSource, forceRefresh) {
			logInfo("updating file abridged from sidecar", logger.Data{"from": existingAbridgedStr, "to": newAbridgedStr})
			file.Abridged = fileSidecarData.Abridged
			file.AbridgedSource = &sidecarSource
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "abridged", "abridged_source")
		}
	}

	// Publisher (from metadata)
	publisherName := strings.TrimSpace(metadata.Publisher)
	if publisherName != "" {
		existingPublisherName := ""
		existingPublisherSource := ""
		if file.Publisher != nil {
			existingPublisherName = file.Publisher.Name
		}
		if file.PublisherSource != nil {
			existingPublisherSource = *file.PublisherSource
		}
		pubSource := metadata.SourceForField("publisher")
		if shouldUpdateScalar(publisherName, existingPublisherName, pubSource, existingPublisherSource, forceRefresh) {
			var publisher *models.Publisher
			var err error
			if cache != nil {
				publisher, err = cache.GetOrCreatePublisher(ctx, publisherName, book.LibraryID, w.publisherService)
			} else {
				publisher, err = w.publisherService.FindOrCreatePublisher(ctx, publisherName, book.LibraryID)
			}
			if err != nil {
				logWarn("failed to find/create publisher", logger.Data{"publisher": publisherName, "error": err.Error()})
			} else {
				logInfo("updating file publisher", logger.Data{"from": existingPublisherName, "to": publisherName})
				file.PublisherID = &publisher.ID
				file.PublisherSource = &pubSource
				fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "publisher_id", "publisher_source")
			}
		}
	}
	// Publisher (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Publisher != nil && *fileSidecarData.Publisher != "" {
		existingPublisherName := ""
		existingPublisherSource := ""
		if file.Publisher != nil {
			existingPublisherName = file.Publisher.Name
		}
		if file.PublisherSource != nil {
			existingPublisherSource = *file.PublisherSource
		}
		if shouldApplySidecarScalar(*fileSidecarData.Publisher, existingPublisherName, existingPublisherSource, forceRefresh) {
			var publisher *models.Publisher
			var err error
			if cache != nil {
				publisher, err = cache.GetOrCreatePublisher(ctx, *fileSidecarData.Publisher, book.LibraryID, w.publisherService)
			} else {
				publisher, err = w.publisherService.FindOrCreatePublisher(ctx, *fileSidecarData.Publisher, book.LibraryID)
			}
			if err != nil {
				logWarn("failed to find/create publisher", logger.Data{"publisher": *fileSidecarData.Publisher, "error": err.Error()})
			} else {
				logInfo("updating file publisher from sidecar", logger.Data{"from": existingPublisherName, "to": *fileSidecarData.Publisher})
				file.PublisherID = &publisher.ID
				file.PublisherSource = &sidecarSource
				fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "publisher_id", "publisher_source")
			}
		}
	}

	// Imprint (from metadata)
	imprintName := strings.TrimSpace(metadata.Imprint)
	if imprintName != "" {
		existingImprintName := ""
		existingImprintSource := ""
		if file.Imprint != nil {
			existingImprintName = file.Imprint.Name
		}
		if file.ImprintSource != nil {
			existingImprintSource = *file.ImprintSource
		}
		imprintSource := metadata.SourceForField("imprint")
		if shouldUpdateScalar(imprintName, existingImprintName, imprintSource, existingImprintSource, forceRefresh) {
			var imprint *models.Imprint
			var err error
			if cache != nil {
				imprint, err = cache.GetOrCreateImprint(ctx, imprintName, book.LibraryID, w.imprintService)
			} else {
				imprint, err = w.imprintService.FindOrCreateImprint(ctx, imprintName, book.LibraryID)
			}
			if err != nil {
				logWarn("failed to find/create imprint", logger.Data{"imprint": imprintName, "error": err.Error()})
			} else {
				logInfo("updating file imprint", logger.Data{"from": existingImprintName, "to": imprintName})
				file.ImprintID = &imprint.ID
				file.ImprintSource = &imprintSource
				fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "imprint_id", "imprint_source")
			}
		}
	}
	// Imprint (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Imprint != nil && *fileSidecarData.Imprint != "" {
		existingImprintName := ""
		existingImprintSource := ""
		if file.Imprint != nil {
			existingImprintName = file.Imprint.Name
		}
		if file.ImprintSource != nil {
			existingImprintSource = *file.ImprintSource
		}
		if shouldApplySidecarScalar(*fileSidecarData.Imprint, existingImprintName, existingImprintSource, forceRefresh) {
			var imprint *models.Imprint
			var err error
			if cache != nil {
				imprint, err = cache.GetOrCreateImprint(ctx, *fileSidecarData.Imprint, book.LibraryID, w.imprintService)
			} else {
				imprint, err = w.imprintService.FindOrCreateImprint(ctx, *fileSidecarData.Imprint, book.LibraryID)
			}
			if err != nil {
				logWarn("failed to find/create imprint", logger.Data{"imprint": *fileSidecarData.Imprint, "error": err.Error()})
			} else {
				logInfo("updating file imprint from sidecar", logger.Data{"from": existingImprintName, "to": *fileSidecarData.Imprint})
				file.ImprintID = &imprint.ID
				file.ImprintSource = &sidecarSource
				fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "imprint_id", "imprint_source")
			}
		}
	}

	// Update audiobook-specific fields (M4B) - these always come from file metadata
	if metadata.Duration > 0 {
		durationSeconds := metadata.Duration.Seconds()
		if file.AudiobookDurationSeconds == nil || *file.AudiobookDurationSeconds != durationSeconds {
			file.AudiobookDurationSeconds = &durationSeconds
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "audiobook_duration_seconds")
		}
	}
	if metadata.BitrateBps > 0 {
		if file.AudiobookBitrateBps == nil || *file.AudiobookBitrateBps != metadata.BitrateBps {
			file.AudiobookBitrateBps = &metadata.BitrateBps
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "audiobook_bitrate_bps")
		}
	}
	if metadata.Codec != "" {
		if file.AudiobookCodec == nil || *file.AudiobookCodec != metadata.Codec {
			file.AudiobookCodec = &metadata.Codec
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "audiobook_codec")
		}
	}

	// Update page count (CBZ) - always comes from file metadata
	if metadata.PageCount != nil {
		if file.PageCount == nil || *file.PageCount != *metadata.PageCount {
			file.PageCount = metadata.PageCount
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "page_count")
		}
	}

	// Apply file column updates
	if len(fileUpdateOpts.Columns) > 0 {
		if err := w.bookService.UpdateFile(ctx, file, fileUpdateOpts); err != nil {
			return nil, errors.Wrap(err, "failed to update file")
		}
	}

	// Reorganize file on disk if library has OrganizeFileStructure enabled.
	// Only do this during resyncs - during full scans, organization is deferred to post-scan phase.
	// This handles two cases:
	// 1. fileNameChanged=true: the file.Name in DB changed, so we need to rename the file on disk
	// 2. fileNameChanged=false but current filename differs from expected: e.g., stripping
	//    author prefix from files that still have it (like "[Author] Title.epub" -> "Title.epub")
	if isResync {
		library, err := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
			ID: &book.LibraryID,
		})
		if err != nil {
			logWarn("failed to retrieve library for file organization", logger.Data{"error": err.Error()})
		} else if library.OrganizeFileStructure {
			// Don't include author names in filenames - all files are inside the book folder
			// which already has the author prefix (e.g., "[Author] Book Title/").
			// Including author in the filename would be redundant.

			// Get narrator names if M4B
			narratorNames := make([]string, 0)
			if file.FileType == models.FileTypeM4B {
				for _, n := range file.Narrators {
					if n.Person != nil {
						narratorNames = append(narratorNames, n.Person.Name)
					}
				}
			}

			// Use file.Name for title if available, otherwise book.Title
			title := book.Title
			if file.Name != nil && *file.Name != "" {
				title = *file.Name
			}

			// Generate organized name options (no AuthorNames - see comment above)
			organizeOpts := fileutils.OrganizedNameOptions{
				NarratorNames: narratorNames,
				Title:         title,
				FileType:      file.FileType,
			}

			// Rename the file
			// Use RenameOrganizedFileOnly to avoid renaming the book sidecar.
			// File-level changes (name) should not affect the book's sidecar -
			// only book-level changes (title, author) should rename the book sidecar.
			newPath, err := fileutils.RenameOrganizedFileOnly(file.Filepath, organizeOpts)
			if err != nil {
				logWarn("failed to rename file after name change", logger.Data{
					"file_id": file.ID,
					"path":    file.Filepath,
					"error":   err.Error(),
				})
			} else if newPath != file.Filepath {
				logInfo("renamed file after name change", logger.Data{
					"file_id":  file.ID,
					"old_path": file.Filepath,
					"new_path": newPath,
				})
				// Update cover path if it exists (covers are renamed by rename function)
				fileRenameOpts := books.UpdateFileOptions{Columns: []string{"filepath"}}
				if file.CoverImageFilename != nil {
					newCoverPath := fileutils.ComputeNewCoverFilename(*file.CoverImageFilename, newPath)
					file.CoverImageFilename = &newCoverPath
					fileRenameOpts.Columns = append(fileRenameOpts.Columns, "cover_image_filename")
				}
				file.Filepath = newPath
				if err := w.bookService.UpdateFile(ctx, file, fileRenameOpts); err != nil {
					logWarn("failed to update file path after rename", logger.Data{
						"file_id": file.ID,
						"error":   err.Error(),
					})
				}
			}
		}
	}

	// Update narrators (for M4B files, from metadata)
	if len(metadata.Narrators) > 0 {
		existingNarratorSource := ""
		if file.NarratorSource != nil {
			existingNarratorSource = *file.NarratorSource
		}
		existingNarratorNames := make([]string, 0, len(file.Narrators))
		for _, n := range file.Narrators {
			if n.Person != nil {
				existingNarratorNames = append(existingNarratorNames, n.Person.Name)
			}
		}

		narratorSource := metadata.SourceForField("narrators")
		if shouldUpdateRelationship(metadata.Narrators, existingNarratorNames, narratorSource, existingNarratorSource, forceRefresh) {
			logInfo("updating narrators", logger.Data{"new_count": len(metadata.Narrators), "old_count": len(file.Narrators)})

			// Collect narrators for batch insert (replaces immediate delete + create)
			relUpdates.DeleteNarrators = true
			relUpdates.Narrators = nil // Clear any previous collection
			for i, narratorName := range metadata.Narrators {
				var person *models.Person
				var err error
				if cache != nil {
					person, err = cache.GetOrCreatePerson(ctx, narratorName, book.LibraryID, w.personService)
				} else {
					person, err = w.personService.FindOrCreatePerson(ctx, narratorName, book.LibraryID)
				}
				if err != nil {
					logWarn("failed to find/create person for narrator", logger.Data{"name": narratorName, "error": err.Error()})
					continue
				}
				relUpdates.Narrators = append(relUpdates.Narrators, &models.Narrator{
					FileID:    file.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
				})
			}

			// Update narrator source
			file.NarratorSource = &narratorSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"narrator_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update narrator source")
			}
		}
	}
	// Update narrators (from sidecar)
	if fileSidecarData != nil && len(fileSidecarData.Narrators) > 0 {
		sidecarNarratorNames := make([]string, 0, len(fileSidecarData.Narrators))
		for _, n := range fileSidecarData.Narrators {
			sidecarNarratorNames = append(sidecarNarratorNames, n.Name)
		}
		existingNarratorSource := ""
		if file.NarratorSource != nil {
			existingNarratorSource = *file.NarratorSource
		}
		existingNarratorNames := make([]string, 0, len(file.Narrators))
		for _, n := range file.Narrators {
			if n.Person != nil {
				existingNarratorNames = append(existingNarratorNames, n.Person.Name)
			}
		}

		if shouldApplySidecarRelationship(sidecarNarratorNames, existingNarratorNames, existingNarratorSource, forceRefresh) {
			logInfo("updating narrators from sidecar", logger.Data{"new_count": len(fileSidecarData.Narrators), "old_count": len(file.Narrators)})

			// Collect narrators for batch insert (replaces any metadata collection)
			relUpdates.DeleteNarrators = true
			relUpdates.Narrators = nil // Clear previous collection
			for i, sidecarNarrator := range fileSidecarData.Narrators {
				var person *models.Person
				var err error
				if cache != nil {
					person, err = cache.GetOrCreatePerson(ctx, sidecarNarrator.Name, book.LibraryID, w.personService)
				} else {
					person, err = w.personService.FindOrCreatePerson(ctx, sidecarNarrator.Name, book.LibraryID)
				}
				if err != nil {
					logWarn("failed to find/create person for narrator", logger.Data{"name": sidecarNarrator.Name, "error": err.Error()})
					continue
				}
				relUpdates.Narrators = append(relUpdates.Narrators, &models.Narrator{
					FileID:    file.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
				})
			}

			// Update narrator source
			file.NarratorSource = &sidecarSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"narrator_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update narrator source")
			}
		}
	}

	// Acquire per-book lock to prevent concurrent updates when multiple files
	// belonging to the same book are processed in parallel. This lock covers:
	// - Relationship updates (authors, series, genres, tags, narrators)
	// - Search index updates
	if cache != nil {
		unlock := cache.LockBook(book.ID)
		defer unlock()
	}

	// Batch update all collected relationships (authors, series, genres, tags, narrators)
	if relUpdates.DeleteAuthors || relUpdates.DeleteSeries || relUpdates.DeleteGenres || relUpdates.DeleteTags || relUpdates.DeleteNarrators {
		if err := w.UpdateBookRelationships(ctx, book.ID, relUpdates); err != nil {
			logWarn("failed to update book relationships", logger.Data{"error": err.Error()})
		}
	}

	// Update identifiers (from metadata)
	if len(metadata.Identifiers) > 0 {
		existingIdentifierSource := ""
		if file.IdentifierSource != nil {
			existingIdentifierSource = *file.IdentifierSource
		}
		existingIdentifierValues := fileIdentifierKeys(file.Identifiers)
		newIdentifierValues := parsedIdentifierKeys(metadata.Identifiers)

		identifierSource := metadata.SourceForField("identifiers")
		if shouldUpdateRelationship(newIdentifierValues, existingIdentifierValues, identifierSource, existingIdentifierSource, forceRefresh) {
			logInfo("updating identifiers", logger.Data{"new_count": len(metadata.Identifiers), "old_count": len(file.Identifiers)})

			// Delete existing identifiers
			if err := w.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete existing identifiers")
			}

			// Create new identifiers in bulk
			fileIdentifiers := make([]*models.FileIdentifier, 0, len(metadata.Identifiers))
			for _, id := range metadata.Identifiers {
				fileIdentifiers = append(fileIdentifiers, &models.FileIdentifier{
					FileID: file.ID,
					Type:   id.Type,
					Value:  id.Value,
					Source: identifierSource,
				})
			}
			if err := w.bookService.BulkCreateFileIdentifiers(ctx, fileIdentifiers); err != nil {
				logWarn("failed to create identifiers", logger.Data{"error": err.Error()})
			}

			// Update identifier source
			file.IdentifierSource = &identifierSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"identifier_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update identifier source")
			}
		}
	}
	// Update identifiers (from sidecar)
	if fileSidecarData != nil && len(fileSidecarData.Identifiers) > 0 {
		sidecarIdentifierValues := sidecarIdentifierKeys(fileSidecarData.Identifiers)
		existingIdentifierSource := ""
		if file.IdentifierSource != nil {
			existingIdentifierSource = *file.IdentifierSource
		}
		existingIdentifierValues := fileIdentifierKeys(file.Identifiers)

		if shouldApplySidecarRelationship(sidecarIdentifierValues, existingIdentifierValues, existingIdentifierSource, forceRefresh) {
			logInfo("updating identifiers from sidecar", logger.Data{"new_count": len(fileSidecarData.Identifiers), "old_count": len(file.Identifiers)})

			// Delete existing identifiers
			if err := w.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete existing identifiers")
			}

			// Create new identifiers from sidecar in bulk
			fileIdentifiers := make([]*models.FileIdentifier, 0, len(fileSidecarData.Identifiers))
			for _, id := range fileSidecarData.Identifiers {
				fileIdentifiers = append(fileIdentifiers, &models.FileIdentifier{
					FileID: file.ID,
					Type:   id.Type,
					Value:  id.Value,
					Source: sidecarSource,
				})
			}
			if err := w.bookService.BulkCreateFileIdentifiers(ctx, fileIdentifiers); err != nil {
				logWarn("failed to create identifiers", logger.Data{"error": err.Error()})
			}

			// Update identifier source
			file.IdentifierSource = &sidecarSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"identifier_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update identifier source")
			}
		}
	}

	// ==========================================================================
	// Update chapters (from metadata)
	// ==========================================================================

	if len(metadata.Chapters) > 0 {
		existingChapterSource := file.ChapterSource
		chapterSource := metadata.SourceForField("chapters")

		if chapters.ShouldUpdateChapters(metadata.Chapters, chapterSource, existingChapterSource, forceRefresh) {
			logInfo("updating chapters", logger.Data{"chapter_count": len(metadata.Chapters)})

			// Replace all chapters with new ones from metadata
			if err := w.chapterService.ReplaceChapters(ctx, file.ID, metadata.Chapters); err != nil {
				return nil, errors.Wrap(err, "failed to replace chapters")
			}

			// Update chapter source on file
			file.ChapterSource = &chapterSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"chapter_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update chapter source")
			}
		}
	}

	// Update chapters (from sidecar)
	if fileSidecarData != nil && len(fileSidecarData.Chapters) > 0 {
		// Convert sidecar chapters to ParsedChapter format
		sidecarChapters := convertSidecarChapters(fileSidecarData.Chapters)

		if chapters.ShouldUpdateChapters(sidecarChapters, sidecarSource, file.ChapterSource, forceRefresh) {
			logInfo("updating chapters from sidecar", logger.Data{"chapter_count": len(sidecarChapters)})

			// Replace all chapters with new ones from sidecar
			if err := w.chapterService.ReplaceChapters(ctx, file.ID, sidecarChapters); err != nil {
				return nil, errors.Wrap(err, "failed to replace chapters from sidecar")
			}

			// Update chapter source on file
			file.ChapterSource = &sidecarSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"chapter_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update chapter source")
			}
		}
	}

	// Update cover page (from sidecar) for page-based formats (CBZ, PDF).
	// This restores user-selected cover page from sidecar after library rescans.
	if fileSidecarData != nil && fileSidecarData.CoverPage != nil && models.IsPageBasedFileType(file.FileType) {
		existingCoverSource := ""
		if file.CoverSource != nil {
			existingCoverSource = *file.CoverSource
		}

		// Check if we should apply sidecar (don't override manual selections)
		// Sidecar has priority 1, manual has priority 0 (lower = higher priority)
		sidecarPriority := models.GetDataSourcePriority(models.DataSourceSidecar)
		existingPriority := models.GetDataSourcePriority(existingCoverSource)
		if existingCoverSource == "" {
			existingPriority = models.GetDataSourcePriority(models.DataSourceFilepath)
		}

		// Only apply if sidecar has equal or higher priority than existing source
		// and the cover page is different (or not set)
		shouldApply := !forceRefresh && sidecarPriority <= existingPriority
		isDifferent := file.CoverPage == nil || *file.CoverPage != *fileSidecarData.CoverPage

		if shouldApply && isDifferent {
			fromPage := file.CoverPage
			page := *fileSidecarData.CoverPage
			extractErr, updateErr := w.applyPageCover(ctx, file, book, page, sidecarSource)
			switch {
			case extractErr != nil:
				logWarn("failed to extract cover page from sidecar", logger.Data{
					"error":      extractErr.Error(),
					"cover_page": page,
				})
			case updateErr != nil:
				return nil, errors.Wrap(updateErr, "failed to update cover page from sidecar")
			default:
				logInfo("updating cover page from sidecar", logger.Data{
					"from_page": fromPage,
					"to_page":   page,
				})
			}
		}
	}

	// Update cover page (from plugin-supplied metadata) for page-based formats.
	// Plugin enrichers (and plugin fileParsers) can identify a cover page that
	// isn't the file parser's default (typically page 0); apply their value
	// when the source priority allows.
	//
	// This branch only fires for plugin-sourced CoverPage. The file parser's
	// own default (cbz_metadata / pdf_metadata) is already written by
	// scanFileCreateNew, and re-applying it here would silently revert a
	// user's page-picker selection on any forced rescan — see
	// TestRecoverMissingCover_RespectsCoverPage. Sidecar-sourced CoverPage is
	// handled by the dedicated sidecar branch above.
	metadataCoverSource := metadata.SourceForField("cover")
	isPluginCoverSource := strings.HasPrefix(metadataCoverSource, models.DataSourcePluginPrefix)
	if metadata.CoverPage != nil && models.IsPageBasedFileType(file.FileType) && isPluginCoverSource {
		existingCoverSource := ""
		if file.CoverSource != nil {
			existingCoverSource = *file.CoverSource
		}

		metadataPriority := models.GetDataSourcePriority(metadataCoverSource)
		existingPriority := models.GetDataSourcePriority(existingCoverSource)
		if existingCoverSource == "" {
			existingPriority = models.GetDataSourcePriority(models.DataSourceFilepath)
		}

		shouldApply := metadataPriority <= existingPriority
		isDifferent := file.CoverPage == nil || *file.CoverPage != *metadata.CoverPage

		if shouldApply && isDifferent {
			page := *metadata.CoverPage
			// Bounds guard — mirrors persistMetadata's handling so scan-path
			// warnings are just as actionable as the identify-apply path.
			switch {
			case page < 0:
				logWarn("plugin-provided cover page is negative, skipping", logger.Data{
					"cover_page": page,
					"source":     metadataCoverSource,
				})
			case file.PageCount == nil:
				logWarn("cover page skipped: page count unknown", logger.Data{
					"cover_page": page,
					"source":     metadataCoverSource,
				})
			case page >= *file.PageCount:
				logWarn("plugin-provided cover page is out of range, skipping", logger.Data{
					"cover_page": page,
					"page_count": *file.PageCount,
					"source":     metadataCoverSource,
				})
			default:
				fromPage := file.CoverPage
				extractErr, updateErr := w.applyPageCover(ctx, file, book, page, metadataCoverSource)
				switch {
				case extractErr != nil:
					logWarn("failed to extract cover page from metadata", logger.Data{
						"error":      extractErr.Error(),
						"cover_page": page,
						"source":     metadataCoverSource,
					})
				case updateErr != nil:
					return nil, errors.Wrap(updateErr, "failed to update cover page from metadata")
				default:
					logInfo("updating cover page from metadata", logger.Data{
						"from_page": fromPage,
						"to_page":   page,
						"source":    metadataCoverSource,
					})
				}
			}
		}
	}

	// ==========================================================================
	// Write sidecar files
	// ==========================================================================

	// Reload book and file with full relations before writing sidecars
	reloadedBook, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	if err != nil {
		logWarn("failed to reload book for sidecar", logger.Data{"error": err.Error()})
	} else {
		// Check if the book directory exists. For root-level files with OrganizeFileStructure
		// enabled, we need to create the directory before writing the sidecar.
		// If the directory doesn't exist and org is disabled, we skip writing the book sidecar
		// since the files haven't been organized yet.
		bookDirExists := false
		if info, statErr := os.Stat(reloadedBook.Filepath); statErr == nil && info.IsDir() {
			bookDirExists = true
		}

		if !bookDirExists {
			// Check if the library has OrganizeFileStructure enabled
			lib, libErr := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
				ID: &reloadedBook.LibraryID,
			})
			if libErr != nil {
				logWarn("failed to retrieve library for sidecar", logger.Data{"error": libErr.Error()})
			} else if lib.OrganizeFileStructure {
				// Create the directory for root-level files that will be organized
				if mkdirErr := os.MkdirAll(reloadedBook.Filepath, 0755); mkdirErr != nil {
					logWarn("failed to create book directory for sidecar", logger.Data{"error": mkdirErr.Error()})
				} else {
					bookDirExists = true
				}
			}
			// If org is disabled, skip writing book sidecar (files are at root level)
		}

		if bookDirExists {
			if err := sidecar.WriteBookSidecarFromModel(reloadedBook); err != nil {
				logWarn("failed to write book sidecar", logger.Data{"error": err.Error()})
			}
		}
		book = reloadedBook
	}

	reloadedFile, err := w.bookService.RetrieveFileWithRelations(ctx, file.ID)
	if err != nil {
		logWarn("failed to reload file for sidecar", logger.Data{"error": err.Error()})
	} else {
		if err := sidecar.WriteFileSidecarFromModel(reloadedFile); err != nil {
			logWarn("failed to write file sidecar", logger.Data{"error": err.Error()})
		}
		file = reloadedFile
	}

	// ==========================================================================
	// Update search index
	// ==========================================================================

	// Only update search index for individual resyncs. For full library scans,
	// RebuildAllIndexes is called at the end of ProcessScanJob, making individual
	// IndexBook calls redundant and wasteful.
	if isResync && w.searchService != nil {
		if err := w.searchService.IndexBook(ctx, book); err != nil {
			logWarn("failed to update search index", logger.Data{"book_id": book.ID, "error": err.Error()})
		}
	}

	return &ScanResult{File: file, Book: book, FileCreated: false}, nil
}

// scanFileCreateNew creates a new file and book record for a file that exists on disk
// but not in the database. It handles:
// 1. Determining if this is a root-level file or directory-based file
// 2. Finding or creating the parent book
// 3. Creating the file record
// 4. Extracting and saving the cover image
// 5. Calling scanFileCore to update metadata
//
// This function is called by scanFileByPath when a file exists on disk but not in DB.
func (w *Worker) scanFileCreateNew(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Warn(msg, data)
		}
	}

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if opts.JobLog != nil {
			opts.JobLog.Info(msg, data)
		}
	}

	path := opts.FilePath

	// Get file stats
	stats, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}
	size := stats.Size()
	modTime := stats.ModTime()
	fileType := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	// Parse metadata from file
	metadata, err := w.parseFileMetadata(ctx, path, fileType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse file metadata")
	}

	// Get library to determine book path and check for root-level files
	library, err := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &opts.LibraryID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve library")
	}

	// Determine if this is a root-level file (directly in library path)
	tempBookPath := filepath.Dir(path)
	isRootLevelFile := false
	var containingLibraryPath string
	for _, libraryPath := range library.LibraryPaths {
		if tempBookPath == libraryPath.Filepath {
			isRootLevelFile = true
			containingLibraryPath = libraryPath.Filepath
			break
		}
	}

	// Populate empty metadata fields from filepath (authors, narrators, series).
	// For root-level files, bookPath isn't computed yet so we pass the file path —
	// extractAuthorsFromFilepath uses the filename when isRootLevelFile=true.
	fpBookPath := tempBookPath
	if isRootLevelFile {
		fpBookPath = path
	}
	applyFilepathFallbacks(metadata, path, fpBookPath, fileType, isRootLevelFile)

	// Determine book path
	var bookPath string
	if isRootLevelFile {
		// For root-level files, compute the expected organized folder path so that
		// multiple root-level files with the same title/author will share a book.
		// This ensures "Wind and Truth.epub" and "Wind and Truth.m4b" become one book.
		title := deriveInitialTitle(path, isRootLevelFile, metadata)
		var authorNames []string
		for _, author := range metadata.Authors {
			authorNames = append(authorNames, author.Name)
		}
		organizedFolderName := fileutils.GenerateOrganizedFolderName(fileutils.OrganizedNameOptions{
			AuthorNames: authorNames,
			Title:       title,
			FileType:    fileType,
		})
		bookPath = filepath.Join(containingLibraryPath, organizedFolderName)
	} else {
		// For directory-based files, use the directory path
		bookPath = tempBookPath
	}

	// Acquire per-path lock to prevent concurrent book creation for same path.
	// This is needed when multiple files in the same directory are processed in parallel.
	if cache != nil {
		unlock := cache.LockBookPath(bookPath, opts.LibraryID)
		defer unlock()
	}

	// Check if a book already exists for this path
	existingBook, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		Filepath:  &bookPath,
		LibraryID: &opts.LibraryID,
	})
	if err != nil && !errors.Is(err, errcodes.NotFound("Book")) {
		return nil, errors.Wrap(err, "failed to check for existing book")
	}

	// Create or reuse book
	var book *models.Book
	if existingBook != nil {
		logInfo("using existing book for new file", logger.Data{"book_id": existingBook.ID, "path": path})
		book = existingBook
	} else {
		// Derive initial title from filepath or metadata
		title := deriveInitialTitle(path, isRootLevelFile, metadata)
		titleSource := models.DataSourceFilepath
		if metadata != nil && strings.TrimSpace(metadata.Title) != "" {
			titleSource = metadata.SourceForField("title")
		}

		logInfo("creating new book", logger.Data{"title": title, "path": bookPath})
		book = &models.Book{
			LibraryID:       opts.LibraryID,
			Filepath:        bookPath,
			Title:           title,
			TitleSource:     titleSource,
			SortTitle:       sortname.ForTitle(title),
			SortTitleSource: titleSource,
			AuthorSource:    models.DataSourceFilepath,
		}
		if err := w.bookService.CreateBook(ctx, book); err != nil {
			return nil, errors.Wrap(err, "failed to create book")
		}
		// Authors, series, and narrators from filepath are already populated on metadata
		// by applyFilepathFallbacks above. scanFileCore will create the DB records.
	}

	// Handle cover extraction. extractAndSaveCover also adopts a cover file
	// that already sits next to the source file on disk, so we always call
	// it — even when the parser returned no cover data for the source.
	var coverImagePath *string
	var coverMimeType *string
	var coverSource *string
	var coverPage *int

	coverFilename, extractedMimeType, wasPreExisting, err := w.extractAndSaveCover(ctx, path, bookPath, isRootLevelFile, metadata, opts.JobLog)
	if err != nil {
		logWarn("failed to extract cover", logger.Data{"error": err.Error()})
	} else if coverFilename != "" {
		coverImagePath = &coverFilename
		if extractedMimeType != "" {
			coverMimeType = &extractedMimeType
		}
		if wasPreExisting {
			existingCoverSource := models.DataSourceExistingCover
			coverSource = &existingCoverSource
		} else {
			cs := metadata.SourceForField("cover")
			coverSource = &cs
		}
	}
	if metadata != nil && metadata.CoverPage != nil {
		coverPage = metadata.CoverPage
	}

	// Create file record
	logInfo("creating file", logger.Data{"path": path, "filesize": size})
	file := &models.File{
		LibraryID:          opts.LibraryID,
		BookID:             book.ID,
		Filepath:           path,
		FileType:           fileType,
		FilesizeBytes:      size,
		FileModifiedAt:     &modTime,
		CoverImageFilename: coverImagePath,
		CoverMimeType:      coverMimeType,
		CoverSource:        coverSource,
		CoverPage:          coverPage,
	}

	// Set fields from metadata if provided (parsers only set what's relevant)
	if metadata != nil {
		if metadata.Duration > 0 {
			durationSeconds := metadata.Duration.Seconds()
			file.AudiobookDurationSeconds = &durationSeconds
		}
		if metadata.BitrateBps > 0 {
			file.AudiobookBitrateBps = &metadata.BitrateBps
		}
		if metadata.Codec != "" {
			file.AudiobookCodec = &metadata.Codec
		}
		if metadata.PageCount != nil {
			file.PageCount = metadata.PageCount
		}
	}

	if err := w.bookService.CreateFile(ctx, file); err != nil {
		return nil, errors.Wrap(err, "failed to create file")
	}
	// Narrators from filepath are already populated on metadata by
	// applyFilepathFallbacks above. scanFileCore will create the DB records.

	// Reload file with relations for scanFileCore
	file, err = w.bookService.RetrieveFileWithRelations(ctx, file.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reload file")
	}

	// Reload book with relations for scanFileCore
	book, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to reload book")
	}

	// Run metadata enrichers after parsing
	if !opts.SkipPlugins {
		metadata = w.runMetadataEnrichers(ctx, metadata, file, book, opts.LibraryID, opts.JobLog)
	}

	// Apply enricher cover if it's higher resolution than the current cover
	w.upgradeEnricherCover(ctx, metadata, file, bookPath, opts.JobLog)

	// Use scanFileCore to handle all metadata updates (authors, series, etc.)
	// This is a batch scan (FilePath mode), so pass isResync=false to skip book organization
	result, err := w.scanFileCore(ctx, file, book, metadata, opts.ForceRefresh, false, opts.JobLog, cache)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update metadata")
	}

	// Mark as file created
	result.FileCreated = true

	// Discover and create supplement files
	w.discoverAndCreateSupplements(ctx, book, path, isRootLevelFile, opts.LibraryID, library, opts.JobLog)

	return result, nil
}

// discoverAndCreateSupplements finds and creates supplement files for a book.
// This is called after creating a new book/file to add any supplements in the same directory.
func (w *Worker) discoverAndCreateSupplements(
	ctx context.Context,
	book *models.Book,
	mainFilePath string,
	isRootLevelFile bool,
	libraryID int,
	library *models.Library,
	jobLog *joblogs.JobLogger,
) {
	log := logger.FromContext(ctx)

	logWarn := func(msg string, data logger.Data) {
		if jobLog != nil {
			jobLog.Warn(msg, data)
		} else {
			log.Warn(msg, data)
		}
	}

	logInfo := func(msg string, data logger.Data) {
		if jobLog != nil {
			jobLog.Info(msg, data)
		} else {
			log.Info(msg, data)
		}
	}

	if !isRootLevelFile {
		// Directory-based book: scan directory for supplements
		bookPath := book.Filepath
		supplements, err := discoverSupplements(bookPath, w.config.SupplementExcludePatterns)
		if err != nil {
			logWarn("failed to discover supplements", logger.Data{"error": err.Error()})
			return
		}
		for _, suppPath := range supplements {
			// Check if supplement already exists
			existingSupp, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
				Filepath:  &suppPath,
				LibraryID: &libraryID,
			})
			if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
				logWarn("error checking supplement", logger.Data{"path": suppPath, "error": err.Error()})
				continue
			}
			if existingSupp != nil {
				continue // Already exists
			}

			// Get file info
			suppStat, err := os.Stat(suppPath)
			if err != nil {
				logWarn("can't stat supplement", logger.Data{"path": suppPath, "error": err.Error()})
				continue
			}

			suppExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(suppPath)), ".")
			suppFile := &models.File{
				LibraryID:     libraryID,
				BookID:        book.ID,
				Filepath:      suppPath,
				FileType:      suppExt,
				FileRole:      models.FileRoleSupplement,
				FilesizeBytes: suppStat.Size(),
			}

			if err := w.bookService.CreateFile(ctx, suppFile); err != nil {
				logWarn("failed to create supplement", logger.Data{"path": suppPath, "error": err.Error()})
				continue
			}
			logInfo("created supplement file", logger.Data{"path": suppPath, "file_id": suppFile.ID})
		}
	} else {
		// Root-level book: find supplements by basename matching
		for _, libraryPath := range library.LibraryPaths {
			if filepath.Dir(mainFilePath) == libraryPath.Filepath {
				supplements, err := discoverRootLevelSupplements(mainFilePath, libraryPath.Filepath, w.config.SupplementExcludePatterns)
				if err != nil {
					logWarn("failed to discover root supplements", logger.Data{"error": err.Error()})
					break
				}
				for _, suppPath := range supplements {
					existingSupp, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
						Filepath:  &suppPath,
						LibraryID: &libraryID,
					})
					if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
						continue
					}
					if existingSupp != nil {
						continue
					}

					suppStat, err := os.Stat(suppPath)
					if err != nil {
						continue
					}

					suppExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(suppPath)), ".")
					suppFile := &models.File{
						LibraryID:     libraryID,
						BookID:        book.ID,
						Filepath:      suppPath,
						FileType:      suppExt,
						FileRole:      models.FileRoleSupplement,
						FilesizeBytes: suppStat.Size(),
					}

					if err := w.bookService.CreateFile(ctx, suppFile); err != nil {
						continue
					}
					logInfo("created root-level supplement", logger.Data{"path": suppPath, "file_id": suppFile.ID})
				}
				break
			}
		}
	}
}

// deriveInitialTitle determines the initial title for a new book from the filepath or metadata.
// For CBZ files, volume indicators like "#007" are normalized to "v7", and parenthesized
// metadata like "(2020) (Digital) (group)" is removed.
func deriveInitialTitle(path string, isRootLevelFile bool, metadata *mediafile.ParsedMetadata) string {
	fileType := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	// If metadata has a title, use it
	if metadata != nil {
		if trimmedTitle := strings.TrimSpace(metadata.Title); trimmedTitle != "" {
			// Normalize volume indicators in metadata title
			if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(trimmedTitle, fileType); hasVolume {
				return normalizedTitle
			}
			return trimmedTitle
		}
	}

	// Fall back to filepath-based title
	var filename string
	if isRootLevelFile {
		// Use the file's base name without extension
		filename = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	} else {
		// Use the directory name
		filename = filepath.Base(filepath.Dir(path))
	}

	// Strip author/narrator patterns from filename
	title := strings.TrimSpace(filepathNarratorRE.ReplaceAllString(filepathAuthorRE.ReplaceAllString(filename, ""), ""))

	// Strip parenthesized metadata from CBZ filenames (year, quality, group)
	if fileType == models.FileTypeCBZ {
		title = filepathParensRE.ReplaceAllString(title, "")
		title = strings.TrimSpace(title)
		title = multiSpaceRE.ReplaceAllString(title, " ")
	}

	// If title is empty after stripping, fall back to raw filename
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	// Normalize volume indicators in filepath-based title
	if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, fileType); hasVolume {
		return normalizedTitle
	}

	return title
}

// applyFilepathFallbacks populates empty metadata fields from the filepath.
// This fills in title, authors, narrators, and series using the same logic
// that scanFileCreateNew uses when creating a book for the first time.
// Fields already present in metadata are not overwritten.
func applyFilepathFallbacks(metadata *mediafile.ParsedMetadata, filePath, bookPath, fileType string, isRootLevelFile bool) {
	if metadata == nil {
		return
	}

	setSource := func(field string) {
		if metadata.FieldDataSources == nil {
			metadata.FieldDataSources = make(map[string]string)
		}
		metadata.FieldDataSources[field] = models.DataSourceFilepath
	}

	// Title fallback
	if strings.TrimSpace(metadata.Title) == "" {
		// Pass nil metadata so deriveInitialTitle uses filepath only (we already confirmed Title is empty)
		metadata.Title = deriveInitialTitle(filePath, isRootLevelFile, nil)
		setSource("title")
	}

	// Authors fallback
	if len(metadata.Authors) == 0 {
		filepathAuthors := extractAuthorsFromFilepath(bookPath, isRootLevelFile)
		for _, name := range filepathAuthors {
			metadata.Authors = append(metadata.Authors, mediafile.ParsedAuthor{Name: name})
		}
		if len(metadata.Authors) > 0 {
			setSource("authors")
		}
	}

	// Narrators fallback
	if len(metadata.Narrators) == 0 {
		filepathNarrators := extractNarratorsFromFilepath(filePath, bookPath, isRootLevelFile)
		metadata.Narrators = append(metadata.Narrators, filepathNarrators...)
		if len(metadata.Narrators) > 0 {
			setSource("narrators")
		}
	}

	// Series fallback from title (e.g., "My Series v3" → series="My Series", number=3)
	if metadata.Series == "" {
		title := metadata.Title
		if seriesName, volumeNumber, ok := fileutils.ExtractSeriesFromTitle(title, fileType); ok {
			metadata.Series = seriesName
			metadata.SeriesNumber = volumeNumber
			setSource("series")
		}
	}
}

// extractAuthorsFromFilepath extracts author names from a filepath using the [Author Name] pattern.
// For directory-based books, looks in the directory name.
// For root-level files, looks in the filename.
func extractAuthorsFromFilepath(bookPath string, isRootLevelFile bool) []string {
	var source string
	if isRootLevelFile {
		// For root-level files, the bookPath is the file path itself
		source = strings.TrimSuffix(filepath.Base(bookPath), filepath.Ext(bookPath))
	} else {
		// For directory-based books, the bookPath is the directory
		source = filepath.Base(bookPath)
	}

	// Find [Author Name] pattern
	if !filepathAuthorRE.MatchString(source) {
		return nil
	}

	// Use FindAllStringSubmatch to get the capture group (content inside brackets)
	matches := filepathAuthorRE.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 || len(matches[0]) < 2 {
		return nil
	}

	// matches[0][1] is the first capture group (author name without brackets)
	// Split on common separators to handle multiple authors
	return fileutils.SplitNames(matches[0][1])
}

// extractNarratorsFromFilepath extracts narrator names from a filepath using the {Narrator Name} pattern.
// Checks both the directory name and the actual filename, preferring the filename.
func extractNarratorsFromFilepath(filePath, bookPath string, isRootLevelFile bool) []string {
	// First check the actual filename (without extension)
	actualFilename := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if filepathNarratorRE.MatchString(actualFilename) {
		matches := filepathNarratorRE.FindAllStringSubmatch(actualFilename, -1)
		if len(matches) > 0 && len(matches[0]) > 1 {
			return fileutils.SplitNames(matches[0][1])
		}
	}

	// Fall back to directory name (only for directory-based books)
	if !isRootLevelFile {
		dirName := filepath.Base(bookPath)
		if filepathNarratorRE.MatchString(dirName) {
			matches := filepathNarratorRE.FindAllStringSubmatch(dirName, -1)
			if len(matches) > 0 && len(matches[0]) > 1 {
				return fileutils.SplitNames(matches[0][1])
			}
		}
	}

	return nil
}

// extractAndSaveCover extracts cover data from metadata and saves it to disk.
// Returns the cover filename, mime type, whether it was pre-existing, and any error.
func (w *Worker) extractAndSaveCover(
	ctx context.Context,
	filePath, bookPath string,
	isRootLevelFile bool,
	metadata *mediafile.ParsedMetadata,
	jobLog *joblogs.JobLogger,
) (filename string, mimeType string, wasPreExisting bool, err error) {
	log := logger.FromContext(ctx)

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if jobLog != nil {
			jobLog.Info(msg, data)
		}
	}

	// Determine cover directory
	coverDir := bookPath
	if isRootLevelFile {
		coverDir = filepath.Dir(filePath)
	}

	// Build cover base name: <filename>.cover
	coverBaseName := filepath.Base(filePath) + ".cover"

	// Check if a cover already exists alongside the file. A user may have
	// dropped a `book.epub.cover.jpg` next to an EPUB that has no embedded
	// cover; in that case we want to adopt the existing file instead of
	// leaving the record without a cover, which would otherwise only get
	// picked up on a later "Refresh all metadata".
	existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)
	if existingCoverPath != "" {
		logInfo("cover already exists, using existing", logger.Data{"path": existingCoverPath})
		existingMime := fileutils.MimeTypeFromExtension(filepath.Ext(existingCoverPath))
		return filepath.Base(existingCoverPath), existingMime, true, nil
	}

	// No cover on disk — fall back to whatever the parser extracted from
	// the file itself.
	if metadata == nil || len(metadata.CoverData) == 0 {
		return "", "", false, nil
	}

	// Normalize the cover image
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(metadata.CoverData, metadata.CoverMimeType)
	coverExt := ".png"
	if normalizedMime == metadata.CoverMimeType {
		coverExt = metadata.CoverExtension()
	}

	// Save cover
	coverFilename := coverBaseName + coverExt
	coverFilepath := filepath.Join(coverDir, coverFilename)
	logInfo("saving cover", logger.Data{"path": coverFilepath, "mime": normalizedMime})

	coverFile, err := os.Create(coverFilepath)
	if err != nil {
		return "", "", false, errors.Wrap(err, "failed to create cover file")
	}
	defer coverFile.Close()

	if _, err := io.Copy(coverFile, bytes.NewReader(normalizedData)); err != nil {
		return "", "", false, errors.Wrap(err, "failed to write cover data")
	}

	return coverFilename, normalizedMime, false, nil
}

// upgradeEnricherCover checks if an enricher provided a cover image that is
// higher resolution than the file's current cover. If so, it saves the enricher
// cover to disk and updates the file record.
//
// This is called after runMetadataEnrichers in both scan paths (new file creation
// and rescan). It only applies covers from plugin sources, respects the cover
// field setting (already enforced by filterMetadataFields), and never replaces
// covers for page-based formats (CBZ, PDF).
//
// bookFilepath is the parent book's filepath. The cover directory is determined
// automatically: if bookFilepath is a directory, covers are saved there; otherwise
// (root-level files where the book path may not exist as a directory), covers are
// saved in the file's parent directory.
func (w *Worker) upgradeEnricherCover(
	ctx context.Context,
	metadata *mediafile.ParsedMetadata,
	file *models.File,
	bookFilepath string,
	jobLog *joblogs.JobLogger,
) {
	log := logger.FromContext(ctx)

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if jobLog != nil {
			jobLog.Info(msg, data)
		}
	}

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if jobLog != nil {
			jobLog.Warn(msg, data)
		}
	}

	// 1. Skip if no cover data in enriched metadata
	if metadata == nil || len(metadata.CoverData) == 0 {
		return
	}

	// 2. Skip if the cover source is not from a plugin
	coverSource := metadata.SourceForField("cover")
	if !strings.HasPrefix(coverSource, models.DataSourcePluginPrefix) {
		return
	}

	// 3. Skip for page-based file types — they derive covers from page content
	if models.IsPageBasedFileType(file.FileType) {
		return
	}

	// 4. Determine the cover directory. bookFilepath may be a synthetic
	// organized-folder path that does not yet exist for root-level new
	// files (see scanFileCreateNew around line 2106), so use the
	// write-side helper that falls back to the file's parent dir.
	coverDir := fileutils.ResolveCoverDirForWrite(bookFilepath, file.Filepath)

	coverBaseName := filepath.Base(file.Filepath) + ".cover"
	existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)

	currentResolution := 0
	if existingCoverPath != "" {
		currentResolution = fileutils.ImageFileResolution(existingCoverPath)
	}

	// 5. Resolution gate — enricher cover must be strictly larger
	enricherResolution := fileutils.ImageResolution(metadata.CoverData)
	if enricherResolution == 0 {
		logWarn("enricher cover could not be decoded, skipping", logger.Data{
			"file_id": file.ID,
			"source":  coverSource,
		})
		return
	}
	if enricherResolution <= currentResolution {
		logInfo("enricher cover not larger than current cover, skipping", logger.Data{
			"file_id":             file.ID,
			"enricher_resolution": enricherResolution,
			"current_resolution":  currentResolution,
			"source":              coverSource,
		})
		return
	}

	// 6. Save enricher cover — normalize and write to disk
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(metadata.CoverData, metadata.CoverMimeType)
	coverExt := ".png"
	if normalizedMime == metadata.CoverMimeType {
		coverExt = metadata.CoverExtension()
	}

	coverFilename := coverBaseName + coverExt
	coverFilepath := filepath.Join(coverDir, coverFilename)

	// Remove any existing cover file with a different extension
	if existingCoverPath != "" && existingCoverPath != coverFilepath {
		os.Remove(existingCoverPath)
	}

	coverFile, err := os.Create(coverFilepath)
	if err != nil {
		logWarn("failed to save enricher cover", logger.Data{
			"error": err.Error(),
			"path":  coverFilepath,
		})
		return
	}
	defer coverFile.Close()

	if _, err := io.Copy(coverFile, bytes.NewReader(normalizedData)); err != nil {
		logWarn("failed to write enricher cover data", logger.Data{
			"error": err.Error(),
			"path":  coverFilepath,
		})
		return
	}

	logInfo("upgraded cover from enricher (higher resolution)", logger.Data{
		"file_id":             file.ID,
		"enricher_resolution": enricherResolution,
		"current_resolution":  currentResolution,
		"source":              coverSource,
		"path":                coverFilepath,
	})

	// 7. Update file record
	file.CoverImageFilename = &coverFilename
	file.CoverMimeType = &normalizedMime
	file.CoverSource = &coverSource
	if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
		Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source"},
	}); err != nil {
		logWarn("failed to update file cover after enricher upgrade", logger.Data{
			"error":   err.Error(),
			"file_id": file.ID,
		})
	}
}

// parseFileMetadata extracts metadata from a file based on its type.
// For built-in types (epub, cbz, m4b), uses the native parsers.
// For other types, falls back to plugin file parsers if available.
func (w *Worker) parseFileMetadata(ctx context.Context, path, fileType string) (*mediafile.ParsedMetadata, error) {
	var metadata *mediafile.ParsedMetadata
	var err error

	switch fileType {
	case models.FileTypeEPUB:
		metadata, err = epub.Parse(path)
	case models.FileTypeCBZ:
		metadata, err = cbz.Parse(path)
	case models.FileTypeM4B:
		metadata, err = mp4.Parse(path)
	case models.FileTypePDF:
		metadata, err = pdf.Parse(path)
	default:
		// Check for plugin file parser
		if w.pluginManager != nil {
			rt := w.pluginManager.GetParserForType(fileType)
			if rt != nil {
				// MIME type validation if mimeTypes declared
				declaredMIMEs := rt.Manifest().Capabilities.FileParser.MIMETypes
				if len(declaredMIMEs) > 0 {
					mtype, mErr := mimetype.DetectFile(path)
					if mErr != nil {
						return nil, errors.Wrap(mErr, "failed to detect MIME type")
					}
					detected := strings.ToLower(mtype.String())
					mimeMatch := false
					for _, allowed := range declaredMIMEs {
						if strings.HasPrefix(detected, strings.ToLower(allowed)) {
							mimeMatch = true
							break
						}
					}
					if !mimeMatch {
						return nil, errors.Errorf("file %s: detected MIME type %s does not match parser mimeTypes %v, skipping", path, mtype.String(), declaredMIMEs)
					}
				}
				return w.pluginManager.RunFileParser(ctx, rt, path, fileType)
			}
		}
		return nil, errors.Errorf("unsupported file type: %s", fileType)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to parse file")
	}

	return metadata, nil
}

// runMetadataEnrichers runs metadata enricher plugins on parsed metadata.
// Each enricher's search() is called with the book title as query, and the first
// result is used directly as ParsedMetadata (no conversion needed).
// Enrichers are called in user-defined order; first non-empty value per field wins.
func (w *Worker) runMetadataEnrichers(ctx context.Context, metadata *mediafile.ParsedMetadata, file *models.File, book *models.Book, libraryID int, jobLog *joblogs.JobLogger) *mediafile.ParsedMetadata {
	if w.pluginManager == nil || metadata == nil {
		return metadata
	}

	runtimes, err := w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, libraryID)
	if err != nil || len(runtimes) == 0 {
		return metadata
	}

	log := logger.FromContext(ctx)

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if jobLog != nil {
			jobLog.Warn(msg, data)
		}
	}

	// Determine search query: use book title if available, fall back to parsed title
	query := book.Title
	if query == "" && metadata != nil {
		query = metadata.Title
	}

	// Determine author: first author name if available
	var author string
	if len(book.Authors) > 0 {
		for _, a := range book.Authors {
			if a.Person != nil {
				author = a.Person.Name
				break
			}
		}
	}

	// Collect identifiers from the file being enriched
	var identifiers []map[string]interface{}
	if file != nil && len(file.Identifiers) > 0 {
		identifiers = make([]map[string]interface{}, len(file.Identifiers))
		for i, id := range file.Identifiers {
			identifiers[i] = map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			}
		}
	}

	// Pre-load confidence thresholds for all enricher runtimes to avoid per-file DB queries
	thresholdCache := make(map[string]*float64) // key: "scope/pluginID"
	for _, rt := range runtimes {
		key := rt.Scope() + "/" + rt.PluginID()
		plugin, err := w.pluginService.GetPlugin(ctx, rt.Scope(), rt.PluginID())
		if err == nil && plugin != nil {
			thresholdCache[key] = plugin.ConfidenceThreshold
		}
	}

	var enrichedMeta mediafile.ParsedMetadata
	modified := false

	for _, rt := range runtimes {
		// Check if enricher handles this file type
		enricherCap := rt.Manifest().Capabilities.MetadataEnricher
		if enricherCap == nil {
			continue
		}
		handles := false
		for _, ft := range enricherCap.FileTypes {
			if ft == file.FileType {
				handles = true
				break
			}
		}
		if !handles {
			continue
		}

		// Build flat search context (same shape as interactive identify)
		searchCtx := map[string]interface{}{
			"query": query,
		}
		if author != "" {
			searchCtx["author"] = author
		}
		if len(identifiers) > 0 {
			searchCtx["identifiers"] = identifiers
		}

		// Add file hints (non-modifiable context)
		if file != nil {
			fileCtx := map[string]interface{}{
				"fileType": file.FileType,
			}
			if file.AudiobookDurationSeconds != nil {
				fileCtx["duration"] = *file.AudiobookDurationSeconds
			}
			if file.PageCount != nil {
				fileCtx["pageCount"] = *file.PageCount
			}
			fileCtx["filesizeBytes"] = file.FilesizeBytes
			searchCtx["file"] = fileCtx
		}

		searchResp, sErr := w.pluginManager.RunMetadataSearch(ctx, rt, searchCtx)
		if sErr != nil {
			logWarn("enricher search failed", logger.Data{
				"plugin": rt.Manifest().ID,
				"error":  sErr.Error(),
			})
			continue
		}
		if searchResp == nil || len(searchResp.Results) == 0 {
			continue
		}

		// Take the first result directly as ParsedMetadata (no conversion needed)
		firstResult := searchResp.Results[0]
		searchMeta := &firstResult

		// Check confidence threshold (if result provides a score)
		if searchMeta.Confidence != nil {
			key := rt.Scope() + "/" + rt.PluginID()
			threshold := w.getConfidenceThresholdFromCache(thresholdCache[key])
			if *searchMeta.Confidence < threshold {
				logWarn("enricher result below confidence threshold, skipping", logger.Data{
					"plugin":     rt.PluginID(),
					"confidence": fmt.Sprintf("%.0f%%", *searchMeta.Confidence*100),
					"threshold":  fmt.Sprintf("%.0f%%", threshold*100),
					"book":       book.Title,
				})
				continue
			}
			log.Info("enricher auto-applying result", logger.Data{
				"plugin":     rt.PluginID(),
				"confidence": fmt.Sprintf("%.0f%%", *searchMeta.Confidence*100),
				"book":       book.Title,
			})
		}

		// Get effective field settings for this library + plugin
		declaredFields := enricherCap.Fields
		enabledFields, fErr := w.pluginService.GetEffectiveFieldSettings(ctx, libraryID, rt.Scope(), rt.PluginID(), declaredFields)
		if fErr != nil {
			logWarn("failed to get field settings", logger.Data{
				"plugin": rt.PluginID(),
				"error":  fErr.Error(),
			})
			// Continue with default (all enabled) if settings lookup fails
			enabledFields = make(map[string]bool, len(declaredFields))
			for _, f := range declaredFields {
				enabledFields[f] = true
			}
		}

		// Filter to only declared and enabled fields, log warnings for undeclared
		filteredMetadata := filterMetadataFields(searchMeta, declaredFields, enabledFields, rt.PluginID(), logWarn)

		// Download cover from URL if coverData is empty and coverUrl is set
		if filteredMetadata.CoverURL != "" && len(filteredMetadata.CoverData) == 0 {
			var allowedDomains []string
			if rt.Manifest().Capabilities.HTTPAccess != nil {
				allowedDomains = rt.Manifest().Capabilities.HTTPAccess.Domains
			}
			plugins.DownloadCoverFromURL(ctx, filteredMetadata, allowedDomains, log)
		}

		// Merge: first non-empty wins per field, tracking source per field
		enricherSource := models.PluginDataSource(rt.Scope(), rt.PluginID())
		mergeEnrichedMetadata(&enrichedMeta, filteredMetadata, enricherSource)
		if !modified {
			enrichedMeta.DataSource = enricherSource
		}
		modified = true
	}

	// Merge file-parsed metadata as fallback and apply page-based cover
	// protection. See mergeFileParserFallback for the full policy.
	mergeFileParserFallback(&enrichedMeta, metadata, file.FileType)

	// Copy technical fields that enrichers don't provide
	enrichedMeta.Duration = metadata.Duration
	enrichedMeta.BitrateBps = metadata.BitrateBps
	enrichedMeta.Codec = metadata.Codec
	enrichedMeta.PageCount = metadata.PageCount

	// Use file parser's DataSource as fallback if no enricher modified anything
	if !modified {
		enrichedMeta.DataSource = metadata.DataSource
	}

	return &enrichedMeta
}

// mergeFileParserFallback merges file-parsed metadata into target as a
// fallback for fields no enricher provided, then applies page-based-format
// cover protection.
//
// For page-based formats (CBZ, PDF), plugin-provided image data
// (coverData/coverUrl) must never replace page-derived covers, so CoverData
// and CoverMimeType are reset to the file parser's values. Enricher-supplied
// CoverPage IS honored — the merge already handles "enricher wins if set,
// file parser fills in otherwise".
//
// Source-tracking nuance: FieldDataSources["cover"] is shared between
// CoverData and CoverPage. The file-parser CoverData merge can overwrite the
// "cover" source recorded by the enricher for CoverPage, so we capture the
// pre-merge enricher state and restore it when appropriate.
func mergeFileParserFallback(target, fileParsed *mediafile.ParsedMetadata, fileType string) {
	// Capture enricher state before the file-parser merge possibly overwrites
	// FieldDataSources["cover"] via its CoverData merge.
	enricherSetCoverPage := target.CoverPage != nil
	enricherCoverSource := ""
	if target.FieldDataSources != nil {
		enricherCoverSource = target.FieldDataSources["cover"]
	}

	// Merge file-parsed metadata as fallback for fields no enricher provided.
	// This gives enrichers priority over file metadata (priority 2 > priority 3).
	mergeEnrichedMetadata(target, fileParsed, fileParsed.DataSource)

	// For page-based formats, reset image data to the file parser's values and
	// restore the "cover" source to reflect whoever provided the CoverPage we
	// kept.
	if models.IsPageBasedFileType(fileType) {
		target.CoverData = fileParsed.CoverData
		target.CoverMimeType = fileParsed.CoverMimeType
		if enricherSetCoverPage {
			target.FieldDataSources["cover"] = enricherCoverSource
		} else {
			target.FieldDataSources["cover"] = fileParsed.SourceForField("cover")
		}
	}
}

// mergeEnrichedMetadata applies fields from enrichment result to the target
// only if the target field is currently empty/zero. Tracks which source
// provided each field in target.FieldDataSources.
func mergeEnrichedMetadata(target, enrichment *mediafile.ParsedMetadata, source string) {
	if target.FieldDataSources == nil {
		target.FieldDataSources = make(map[string]string)
	}
	if target.Title == "" && enrichment.Title != "" {
		target.Title = enrichment.Title
		target.FieldDataSources["title"] = source
	}
	if target.Subtitle == "" && enrichment.Subtitle != "" {
		target.Subtitle = enrichment.Subtitle
		target.FieldDataSources["subtitle"] = source
	}
	if len(target.Authors) == 0 && len(enrichment.Authors) > 0 {
		target.Authors = enrichment.Authors
		target.FieldDataSources["authors"] = source
	}
	if len(target.Narrators) == 0 && len(enrichment.Narrators) > 0 {
		target.Narrators = enrichment.Narrators
		target.FieldDataSources["narrators"] = source
	}
	if target.Series == "" && enrichment.Series != "" {
		target.Series = enrichment.Series
		target.FieldDataSources["series"] = source
	}
	if target.SeriesNumber == nil && enrichment.SeriesNumber != nil {
		target.SeriesNumber = enrichment.SeriesNumber
		target.FieldDataSources["series"] = source
	}
	if len(target.Genres) == 0 && len(enrichment.Genres) > 0 {
		target.Genres = enrichment.Genres
		target.FieldDataSources["genres"] = source
	}
	if len(target.Tags) == 0 && len(enrichment.Tags) > 0 {
		target.Tags = enrichment.Tags
		target.FieldDataSources["tags"] = source
	}
	if target.Description == "" && enrichment.Description != "" {
		target.Description = htmlutil.StripTags(enrichment.Description)
		target.FieldDataSources["description"] = source
	}
	if target.Publisher == "" && enrichment.Publisher != "" {
		target.Publisher = enrichment.Publisher
		target.FieldDataSources["publisher"] = source
	}
	if target.Imprint == "" && enrichment.Imprint != "" {
		target.Imprint = enrichment.Imprint
		target.FieldDataSources["imprint"] = source
	}
	if target.URL == "" && enrichment.URL != "" {
		target.URL = enrichment.URL
		target.FieldDataSources["url"] = source
	}
	if target.ReleaseDate == nil && enrichment.ReleaseDate != nil {
		target.ReleaseDate = enrichment.ReleaseDate
		target.FieldDataSources["releaseDate"] = source
	}
	if target.Language == nil && enrichment.Language != nil {
		target.Language = enrichment.Language
		target.FieldDataSources["language"] = source
	}
	if target.Abridged == nil && enrichment.Abridged != nil {
		target.Abridged = enrichment.Abridged
		target.FieldDataSources["abridged"] = source
	}
	if len(target.CoverData) == 0 && len(enrichment.CoverData) > 0 {
		target.CoverData = enrichment.CoverData
		target.CoverMimeType = enrichment.CoverMimeType
		target.FieldDataSources["cover"] = source
	}
	if target.CoverPage == nil && enrichment.CoverPage != nil {
		target.CoverPage = enrichment.CoverPage
		target.FieldDataSources["cover"] = source
	}
	if len(target.Chapters) == 0 && len(enrichment.Chapters) > 0 {
		target.Chapters = enrichment.Chapters
		target.FieldDataSources["chapters"] = source
	}
	// Identifiers are multi-valued by type, so we append new types from the enricher
	// rather than using "first non-empty wins" like other fields.
	if len(enrichment.Identifiers) > 0 {
		existingTypes := make(map[string]bool, len(target.Identifiers))
		for _, id := range target.Identifiers {
			existingTypes[id.Type] = true
		}
		for _, id := range enrichment.Identifiers {
			if !existingTypes[id.Type] {
				target.Identifiers = append(target.Identifiers, id)
				target.FieldDataSources["identifiers"] = source
			}
		}
	}
}

// filterMetadataFields zeros out fields that are undeclared or disabled.
// Undeclared fields (returned but not in manifest) are zeroed with a log warning.
// Disabled fields (declared but user disabled) are zeroed silently.
// Field groupings:
//   - "series" or "seriesNumber" controls both series name and seriesNumber.
//   - "cover" controls coverData, coverMimeType, and coverPage.
func filterMetadataFields(
	md *mediafile.ParsedMetadata,
	declaredFields []string,
	enabledFields map[string]bool,
	pluginID string,
	logWarn func(string, logger.Data),
) *mediafile.ParsedMetadata {
	if md == nil {
		return nil
	}

	// Build a set of declared fields for fast lookup
	declared := make(map[string]bool, len(declaredFields))
	for _, f := range declaredFields {
		declared[f] = true
	}

	// Helper to check if a field is allowed (declared AND enabled)
	// For undeclared fields, log a warning and return false
	// For disabled fields, silently return false
	isFieldAllowed := func(field string) bool {
		if !declared[field] {
			return false
		}
		// Check enabledFields - if not in map, default is enabled (true)
		if enabled, ok := enabledFields[field]; ok {
			return enabled
		}
		return true
	}

	// Helper to check if a field has data and is undeclared (for warning)
	warnIfUndeclared := func(field string, hasData bool) {
		if hasData && !declared[field] {
			logWarn("enricher returned undeclared field", logger.Data{
				"plugin": pluginID,
				"field":  field,
			})
		}
	}

	// Create a copy to avoid mutating the original
	result := *md

	// Handle "series" grouping - both "series" and "seriesNumber" control the series fields
	seriesAllowed := isFieldAllowed("series") || isFieldAllowed("seriesNumber")
	seriesDeclared := declared["series"] || declared["seriesNumber"]
	if !seriesDeclared {
		if result.Series != "" {
			logWarn("enricher returned undeclared field", logger.Data{
				"plugin": pluginID,
				"field":  "series",
			})
		}
		if result.SeriesNumber != nil {
			logWarn("enricher returned undeclared field", logger.Data{
				"plugin": pluginID,
				"field":  "seriesNumber",
			})
		}
	}
	if !seriesAllowed {
		result.Series = ""
		result.SeriesNumber = nil
	}

	// Handle "cover" grouping
	if !isFieldAllowed("cover") {
		warnIfUndeclared("cover", len(result.CoverData) > 0 || result.CoverMimeType != "" || result.CoverPage != nil || result.CoverURL != "")
		result.CoverData = nil
		result.CoverMimeType = ""
		result.CoverPage = nil
		result.CoverURL = ""
	}

	// Handle individual fields
	if !isFieldAllowed("title") {
		warnIfUndeclared("title", result.Title != "")
		result.Title = ""
	}
	if !isFieldAllowed("subtitle") {
		warnIfUndeclared("subtitle", result.Subtitle != "")
		result.Subtitle = ""
	}
	if !isFieldAllowed("authors") {
		warnIfUndeclared("authors", len(result.Authors) > 0)
		result.Authors = nil
	}
	if !isFieldAllowed("narrators") {
		warnIfUndeclared("narrators", len(result.Narrators) > 0)
		result.Narrators = nil
	}
	if !isFieldAllowed("genres") {
		warnIfUndeclared("genres", len(result.Genres) > 0)
		result.Genres = nil
	}
	if !isFieldAllowed("tags") {
		warnIfUndeclared("tags", len(result.Tags) > 0)
		result.Tags = nil
	}
	if !isFieldAllowed("description") {
		warnIfUndeclared("description", result.Description != "")
		result.Description = ""
	}
	if !isFieldAllowed("publisher") {
		warnIfUndeclared("publisher", result.Publisher != "")
		result.Publisher = ""
	}
	if !isFieldAllowed("imprint") {
		warnIfUndeclared("imprint", result.Imprint != "")
		result.Imprint = ""
	}
	if !isFieldAllowed("url") {
		warnIfUndeclared("url", result.URL != "")
		result.URL = ""
	}
	if !isFieldAllowed("releaseDate") {
		warnIfUndeclared("releaseDate", result.ReleaseDate != nil)
		result.ReleaseDate = nil
	}
	if !isFieldAllowed("identifiers") {
		warnIfUndeclared("identifiers", len(result.Identifiers) > 0)
		result.Identifiers = nil
	}
	if !isFieldAllowed("language") {
		warnIfUndeclared("language", result.Language != nil)
		result.Language = nil
	}
	if !isFieldAllowed("abridged") {
		warnIfUndeclared("abridged", result.Abridged != nil)
		result.Abridged = nil
	}

	return &result
}

// convertSidecarChapters converts sidecar ChapterMetadata to mediafile ParsedChapter.
func convertSidecarChapters(chapters []sidecar.ChapterMetadata) []mediafile.ParsedChapter {
	if len(chapters) == 0 {
		return nil
	}

	result := make([]mediafile.ParsedChapter, len(chapters))
	for i, ch := range chapters {
		result[i] = mediafile.ParsedChapter{
			Title:            ch.Title,
			StartPage:        ch.StartPage,
			StartTimestampMs: ch.StartTimestampMs,
			Href:             ch.Href,
			Children:         convertSidecarChapters(ch.Children),
		}
	}
	return result
}

// Scan implements the books.Scanner interface.
// It converts books.ScanOptions to worker.ScanOptions, calls the internal scanInternal method,
// and converts the result back to books.ScanResult.
func (w *Worker) Scan(ctx context.Context, opts books.ScanOptions) (*books.ScanResult, error) {
	// Convert books.ScanOptions to internal ScanOptions
	internalOpts := ScanOptions{
		FileID:       opts.FileID,
		BookID:       opts.BookID,
		ForceRefresh: opts.ForceRefresh,
		SkipPlugins:  opts.SkipPlugins,
		Reset:        opts.Reset,
	}

	// Call internal unified Scan method (no cache for single-file rescans)
	result, err := w.scanInternal(ctx, internalOpts, nil)
	if err != nil {
		return nil, err
	}

	// Convert internal ScanResult to books.ScanResult
	return &books.ScanResult{
		File:        result.File,
		Book:        result.Book,
		FileDeleted: result.FileDeleted,
		BookDeleted: result.BookDeleted,
	}, nil
}

// recoverMissingCover checks if the file's cover image is missing from disk
// and re-extracts it from the media file if needed. This also handles the case
// where a file never had a cover extracted (e.g., a promoted supplement).
func (w *Worker) recoverMissingCover(ctx context.Context, file *models.File, jobLog *joblogs.JobLogger) error {
	log := logger.FromContext(ctx).Data(logger.Data{"file_id": file.ID, "filepath": file.Filepath})

	logInfo := func(msg string, data logger.Data) {
		log.Info(msg, data)
		if jobLog != nil {
			jobLog.Info(msg, data)
		}
	}

	// Determine cover directory. For both root-level and directory-backed
	// books, the cover lives in the same directory as the file, so
	// filepath.Dir(file.Filepath) is always correct — no stat needed.
	coverDir := filepath.Dir(file.Filepath)

	// Check if cover file exists on disk
	filename := filepath.Base(file.Filepath)
	coverBaseName := filename + ".cover"
	existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)

	if existingCoverPath != "" {
		// Cover exists on disk - check if database needs updating
		if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
			// Database doesn't have the cover info, update it
			coverFilename := filepath.Base(existingCoverPath)
			coverMimeType := fileutils.MimeTypeFromExtension(filepath.Ext(existingCoverPath))
			coverSource := models.DataSourceExistingCover
			file.CoverImageFilename = &coverFilename
			file.CoverMimeType = &coverMimeType
			file.CoverSource = &coverSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
				Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source"},
			}); err != nil {
				return errors.WithStack(err)
			}
			logInfo("updated database with existing cover", logger.Data{"cover_path": existingCoverPath})
		}
		return nil
	}

	logWarn := func(msg string, data logger.Data) {
		log.Warn(msg, data)
		if jobLog != nil {
			jobLog.Warn(msg, data)
		}
	}

	// Cover doesn't exist on disk - check if we need to extract one
	// This handles both: 1) missing cover that was previously extracted, and
	// 2) file that never had a cover (e.g., promoted supplement)
	if file.CoverMimeType != nil {
		logInfo("cover file missing, re-extracting", nil)
	} else {
		logInfo("no cover exists, extracting", nil)
	}

	// Page-based formats (CBZ, PDF) may have a user-selected cover page in
	// file.CoverPage (set via the page picker UI). Re-extract from that
	// specific page so recovery preserves the user's choice instead of
	// silently resetting to page 0. If the page extraction fails we bail
	// out rather than falling through to the generic parser — otherwise
	// we'd write a page-0 cover to disk while file.CoverPage still points
	// at the user's selection, leaving the two out of sync.
	if models.IsPageBasedFileType(file.FileType) && file.CoverPage != nil {
		pageNum := *file.CoverPage
		var coverFilename, coverMimeType string
		var err error
		switch file.FileType {
		case models.FileTypeCBZ:
			coverFilename, coverMimeType, err = extractCBZPageCover(file.Filepath, coverDir, coverBaseName, pageNum)
		case models.FileTypePDF:
			coverFilename, coverMimeType, err = extractPDFPageCover(file.Filepath, coverDir, coverBaseName, pageNum)
		}
		if err != nil {
			logWarn("failed to extract cover from selected page", logger.Data{"page": pageNum, "error": err.Error()})
			return nil
		}
		if coverFilename == "" {
			return nil
		}
		logInfo("recovered cover from selected page", logger.Data{"page": pageNum})
		file.CoverImageFilename = &coverFilename
		file.CoverMimeType = &coverMimeType
		// Leave CoverSource alone — the user's selection still stands.
		if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
			Columns: []string{"cover_image_filename", "cover_mime_type"},
		}); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	// Delegate parsing to parseFileMetadata so plugin-registered file parsers
	// are also consulted (previously a hard-coded switch left plugin types
	// stuck with no cover after deletion). Unsupported file types return an
	// error we treat as "nothing to recover".
	metadata, parseErr := w.parseFileMetadata(ctx, file.Filepath, file.FileType)
	if parseErr != nil {
		logInfo("cannot parse file for cover recovery", logger.Data{"error": parseErr.Error()})
		return nil
	}

	if metadata == nil || len(metadata.CoverData) == 0 {
		logInfo("no cover data in media file", nil)
		return nil
	}

	// Normalize the cover image
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(metadata.CoverData, metadata.CoverMimeType)
	coverExt := ".png"
	if normalizedMime == metadata.CoverMimeType {
		coverExt = metadata.CoverExtension()
	}

	// Save the cover
	coverFilepath := filepath.Join(coverDir, coverBaseName+coverExt)
	coverFile, err := os.Create(coverFilepath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer coverFile.Close()

	if _, err := io.Copy(coverFile, bytes.NewReader(normalizedData)); err != nil {
		return errors.WithStack(err)
	}

	logInfo("extracted cover", logger.Data{"cover_path": coverFilepath})

	// Update file's cover info in database
	coverFilename := filepath.Base(coverFilepath)
	coverSource := metadata.SourceForField("cover")
	file.CoverImageFilename = &coverFilename
	file.CoverMimeType = &normalizedMime
	file.CoverSource = &coverSource
	if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
		Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source"},
	}); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// applyPageCover renders `page` from the page-based file, writes it as the
// cover image next to the book, and persists the cover_page /
// cover_image_filename / cover_mime_type / cover_source update to the DB.
// Returns (extractErr, updateErr). Callers typically treat extract errors as
// non-fatal warnings and surface update errors.
func (w *Worker) applyPageCover(ctx context.Context, file *models.File, book *models.Book, page int, source string) (extractErr, updateErr error) {
	coverDir := fileutils.ResolveCoverDirForWrite(book.Filepath, file.Filepath)
	coverBaseName := filepath.Base(file.Filepath) + ".cover"

	var coverFilename, coverMimeType string
	switch file.FileType {
	case models.FileTypePDF:
		coverFilename, coverMimeType, extractErr = extractPDFPageCover(file.Filepath, coverDir, coverBaseName, page)
	case models.FileTypeCBZ:
		coverFilename, coverMimeType, extractErr = extractCBZPageCover(file.Filepath, coverDir, coverBaseName, page)
	default:
		extractErr = errors.Errorf("unsupported page-based file type for cover extraction: %s", file.FileType)
	}
	if extractErr != nil {
		return extractErr, nil
	}

	file.CoverPage = &page
	file.CoverImageFilename = &coverFilename
	file.CoverMimeType = &coverMimeType
	file.CoverSource = &source

	updateErr = w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
		Columns: []string{"cover_page", "cover_image_filename", "cover_mime_type", "cover_source"},
	})
	return nil, updateErr
}

// extractCBZPageCover extracts a specific page from a CBZ file and saves it as the cover.
// Returns the cover filename (relative to coverDir), mime type, and any error.
// pageNum is 0-indexed.
func extractCBZPageCover(cbzPath string, coverDir string, coverBaseName string, pageNum int) (string, string, error) {
	f, err := os.Open(cbzPath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer f.Close()

	stats, err := f.Stat()
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	zipReader, err := zip.NewReader(f, stats.Size())
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// Get sorted image files
	var imageFiles []*zip.File
	for _, file := range zipReader.File {
		ext := strings.ToLower(filepath.Ext(file.Name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
			imageFiles = append(imageFiles, file)
		}
	}
	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	if pageNum < 0 || pageNum >= len(imageFiles) {
		return "", "", errors.Errorf("page %d out of range (0-%d)", pageNum, len(imageFiles)-1)
	}

	targetFile := imageFiles[pageNum]

	// Determine extension and mime type
	ext := strings.ToLower(filepath.Ext(targetFile.Name))
	mimeType := ""
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".png":
		mimeType = "image/png"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	}

	// Delete any existing cover with this base name (regardless of extension)
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, statErr := os.Stat(existingPath); statErr == nil {
			_ = os.Remove(existingPath)
		}
	}

	// Extract the page
	coverFilePath := filepath.Join(coverDir, coverBaseName+ext)

	r, err := targetFile.Open()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer r.Close()

	// Read the image data
	data, err := io.ReadAll(r)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// Normalize the image
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(data, mimeType)
	if normalizedMime != mimeType {
		// Extension changed due to normalization
		ext = ".png"
		if normalizedMime == "image/jpeg" {
			ext = ".jpg"
		}
		coverFilePath = filepath.Join(coverDir, coverBaseName+ext)
		mimeType = normalizedMime
	}

	// Write the cover file
	outFile, err := os.Create(coverFilePath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, bytes.NewReader(normalizedData)); err != nil {
		return "", "", errors.WithStack(err)
	}

	return coverBaseName + ext, mimeType, nil
}

// extractPDFPageCover renders a specific page from a PDF file via pdfium and
// saves it as the cover image. Returns the cover filename (relative to
// coverDir), mime type, and any error. pageNum is 0-indexed.
func extractPDFPageCover(pdfPath string, coverDir string, coverBaseName string, pageNum int) (string, string, error) {
	data, mimeType, err := pdf.RenderPageJPEG(pdfPath, pageNum, 150, 85)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to render pdf page")
	}

	// Delete any existing cover with this base name (regardless of extension).
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, statErr := os.Stat(existingPath); statErr == nil {
			_ = os.Remove(existingPath)
		}
	}

	coverFilename := coverBaseName + ".jpg"
	coverFilePath := filepath.Join(coverDir, coverFilename)
	if err := os.WriteFile(coverFilePath, data, 0644); err != nil { //nolint:gosec // Cover files need to be readable by the HTTP server
		return "", "", errors.WithStack(err)
	}

	return coverFilename, mimeType, nil
}

// resetBookState wipes book-level scanned metadata and all associated
// authors, series, genres, and tags. Identity fields (ID, filepath,
// library_id, primary_file_id) are preserved. Title and SortTitle values
// are preserved (NOT NULL) but their source fields are reset to
// DataSourceFilepath so scanFileCore can set the correct source from
// the re-scanned metadata.
func (w *Worker) resetBookState(ctx context.Context, book *models.Book) error {
	// --- Book-level columns ---
	book.Subtitle = nil
	book.SubtitleSource = nil
	book.Description = nil
	book.DescriptionSource = nil
	book.GenreSource = nil
	book.TagSource = nil

	// Reset NOT NULL source fields to filepath (lowest priority) so that
	// scanFileCore can correct them. Without this, a stale high-priority
	// source (e.g., "plugin:foo") would prevent future scans from updating.
	book.TitleSource = models.DataSourceFilepath
	book.SortTitleSource = models.DataSourceFilepath
	book.AuthorSource = models.DataSourceFilepath

	bookColumns := []string{
		"subtitle", "subtitle_source",
		"description", "description_source",
		"genre_source", "tag_source",
		"title_source", "sort_title_source", "author_source",
	}
	if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: bookColumns}); err != nil {
		return errors.Wrap(err, "failed to clear book metadata")
	}

	// --- Book-level relations ---
	if err := w.bookService.DeleteAuthors(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book authors")
	}
	if err := w.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book series")
	}
	if err := w.bookService.DeleteBookGenres(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book genres")
	}
	if err := w.bookService.DeleteBookTags(ctx, book.ID); err != nil {
		return errors.Wrap(err, "failed to delete book tags")
	}

	return nil
}

// resetBookFileState wipes all scanned metadata from a book and its file,
// preparing them for a fresh scan. It preserves identity fields (IDs, filepath,
// file_type, file_role, library_id, book_id, primary_file_id, filesize, duration,
// bitrate, codec, page_count).
//
// When skipBookWipe is true, only file-level state is reset. This is used by
// scanBook which handles the book-level wipe once for all files rather than
// per-file.
func (w *Worker) resetBookFileState(ctx context.Context, book *models.Book, file *models.File, skipBookWipe bool) error {
	if !skipBookWipe {
		if err := w.resetBookState(ctx, book); err != nil {
			return errors.Wrap(err, "failed to reset book state")
		}
	}

	// --- File-level columns ---
	file.Name = nil
	file.NameSource = nil
	file.URL = nil
	file.URLSource = nil
	file.ReleaseDate = nil
	file.ReleaseDateSource = nil
	file.PublisherID = nil
	file.PublisherSource = nil
	file.ImprintID = nil
	file.ImprintSource = nil
	file.Language = nil
	file.LanguageSource = nil
	file.Abridged = nil
	file.AbridgedSource = nil
	file.ChapterSource = nil
	file.NarratorSource = nil
	file.IdentifierSource = nil

	fileColumns := []string{
		"name", "name_source",
		"url", "url_source",
		"release_date", "release_date_source",
		"publisher_id", "publisher_source",
		"imprint_id", "imprint_source",
		"language", "language_source",
		"abridged", "abridged_source",
		"chapter_source",
		"narrator_source", "identifier_source",
	}

	// Delete cover from disk before clearing cover columns
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		coverPath := filepath.Join(filepath.Dir(file.Filepath), *file.CoverImageFilename)
		_ = os.Remove(coverPath)
	}

	file.CoverImageFilename = nil
	file.CoverMimeType = nil
	file.CoverSource = nil
	file.CoverPage = nil
	fileColumns = append(fileColumns,
		"cover_image_filename", "cover_mime_type", "cover_source", "cover_page",
	)

	if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: fileColumns}); err != nil {
		return errors.Wrap(err, "failed to clear file metadata")
	}

	// --- File-level relations ---
	if _, err := w.bookService.DeleteNarratorsForFile(ctx, file.ID); err != nil {
		return errors.Wrap(err, "failed to delete file narrators")
	}
	if _, err := w.bookService.DeleteIdentifiersForFile(ctx, file.ID); err != nil {
		return errors.Wrap(err, "failed to delete file identifiers")
	}
	if err := w.chapterService.DeleteChaptersForFile(ctx, file.ID); err != nil {
		return errors.Wrap(err, "failed to delete file chapters")
	}

	return nil
}

// getConfidenceThresholdFromCache returns the effective confidence threshold
// using a pre-loaded plugin threshold value, falling back to global config or default.
func (w *Worker) getConfidenceThresholdFromCache(pluginThreshold *float64) float64 {
	if pluginThreshold != nil {
		return *pluginThreshold
	}
	if w.config != nil {
		return w.config.EnrichmentConfidenceThreshold
	}
	return 0.85
}
