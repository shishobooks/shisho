package worker

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

// orphanEntry holds a candidate-orphan file ID with its stored sha256 hash.
type orphanEntry struct {
	fileID int
	hash   string
}

// reconcileMoves detects files that were moved/renamed while the server was
// offline by matching candidate-orphan DB rows against unknown-new on-disk
// paths using file size as a cheap first filter followed by sha256 comparison.
//
// The function mutates three data structures in-place:
//   - orphan DB rows whose paths are matched get their filepath updated to the
//     new on-disk path
//   - matched file IDs are stored in the cache via SetMovedOrphanIDs so that
//     orphan cleanup skips them
//   - matched new paths are registered in the cache via AddKnownFile so the
//     subsequent parallel processing loop treats them as already-known and
//     doesn't create a duplicate file row
//
// Preconditions:
//   - existingFiles contains all main-file rows loaded before the walk
//   - filesToScan contains all paths discovered by the walk (includes both
//     already-known paths and unknown-new paths)
//   - cache.knownFiles is already populated from LoadKnownFiles
//
//nolint:unparam // error return reserved for future DB error propagation
func (w *Worker) reconcileMoves(
	ctx context.Context,
	existingFiles []*models.File,
	filesToScan []string,
	cache *ScanCache,
	jobLog *joblogs.JobLogger,
) error {
	// Reconciliation requires the fingerprint service. If it's not wired up
	// (e.g. in certain test contexts), skip silently.
	if w.fingerprintService == nil {
		return nil
	}

	// Build a set of scanned paths for quick membership testing.
	scannedSet := make(map[string]struct{}, len(filesToScan))
	for _, p := range filesToScan {
		scannedSet[p] = struct{}{}
	}

	// Identify candidate orphans (DB files whose paths were not found on disk).
	// Only main files are considered — supplement lifecycle is tied to the parent book.
	type sizeKey = int64
	// orphanIndex maps filesize_bytes → list of orphanEntry for orphans that
	// have a stored sha256 fingerprint. Orphans without a fingerprint are
	// excluded from reconciliation (we can't match them safely).
	orphanIndex := make(map[sizeKey][]orphanEntry)

	for _, file := range existingFiles {
		if file.FileRole != models.FileRoleMain {
			continue
		}
		if _, seen := scannedSet[file.Filepath]; seen {
			continue // file still exists at its known path — not an orphan
		}
		if file.FilesizeBytes == 0 {
			continue // no size recorded; can't use for size-based pruning
		}

		// Look up stored sha256 fingerprint for this orphan.
		fps, err := w.fingerprintService.ListForFile(ctx, file.ID, models.FingerprintAlgorithmSHA256)
		if err != nil {
			// Non-fatal: log and skip — this orphan will follow the normal deletion path.
			jobLog.Warn("reconcile: failed to list fingerprints for orphan", logger.Data{
				"file_id": file.ID,
				"error":   err.Error(),
			})
			continue
		}
		if len(fps) == 0 {
			// No fingerprint — can't reconcile this orphan.
			continue
		}

		orphanIndex[file.FilesizeBytes] = append(orphanIndex[file.FilesizeBytes], orphanEntry{
			fileID: file.ID,
			hash:   fps[0].Value,
		})
	}

	if len(orphanIndex) == 0 {
		// No fingerprinted orphans — nothing to reconcile.
		return nil
	}

	// Build a reverse map: fileID → *models.File for quick mutation.
	fileByID := make(map[int]*models.File, len(existingFiles))
	for _, f := range existingFiles {
		fileByID[f.ID] = f
	}

	// Walk unknown-new paths: paths on disk that are NOT in the known-files cache.
	movedOrphanIDs := make(map[int]struct{})

	for _, newPath := range filesToScan {
		if cache.GetKnownFile(newPath) != nil {
			continue // already known — not an unknown-new file
		}

		// Size-based pruning: stat the file to get its size.
		info, err := os.Stat(newPath)
		if err != nil {
			continue // can't stat — skip
		}
		newSize := info.Size()

		candidates, ok := orphanIndex[newSize]
		if !ok {
			continue // no orphan with matching size
		}

		// Compute sha256 for the new file (only done when sizes match).
		newHash, err := computeFileSHA256(newPath)
		if err != nil {
			jobLog.Warn("reconcile: failed to compute sha256 for candidate new file", logger.Data{
				"path":  newPath,
				"error": err.Error(),
			})
			continue
		}

		// Find the first orphan whose hash matches.
		matchIdx := -1
		for i, candidate := range candidates {
			if candidate.hash == newHash {
				matchIdx = i
				break
			}
		}
		if matchIdx < 0 {
			continue // hash mismatch — different content
		}

		matchedEntry := candidates[matchIdx]
		orphanFile := fileByID[matchedEntry.fileID]
		if orphanFile == nil {
			continue // shouldn't happen, but be defensive
		}

		// Match found — update the orphan's filepath to the new path.
		oldPath := orphanFile.Filepath
		orphanFile.Filepath = newPath
		if err := w.bookService.UpdateFile(ctx, orphanFile, books.UpdateFileOptions{
			Columns: []string{"filepath"},
		}); err != nil {
			jobLog.Warn("reconcile: failed to update filepath for moved orphan", logger.Data{
				"file_id":  orphanFile.ID,
				"old_path": oldPath,
				"new_path": newPath,
				"error":    err.Error(),
			})
			// Revert in-memory change so the file still goes through orphan cleanup.
			orphanFile.Filepath = oldPath
			continue
		}

		// Bring the book row along with the file so cover serving,
		// supplement detection, and organize all resolve against the
		// new directory. No-op for multi-file books where this single
		// file's move doesn't reflect the whole book's new location.
		if err := w.syncBookFilepathAfterMove(ctx, orphanFile, oldPath, newPath); err != nil {
			jobLog.Warn("reconcile: failed to sync book filepath after move", logger.Data{
				"file_id":  orphanFile.ID,
				"old_path": oldPath,
				"new_path": newPath,
				"error":    err.Error(),
			})
		}

		jobLog.Info("reconcile: detected file move", logger.Data{
			"file_id":  orphanFile.ID,
			"old_path": oldPath,
			"new_path": newPath,
		})

		// Record as moved orphan so cleanup skips it.
		movedOrphanIDs[orphanFile.ID] = struct{}{}

		// Register new path in the cache so the parallel processing loop sees
		// the file as already-known and doesn't create a duplicate row.
		cache.AddKnownFile(orphanFile)

		// Remove the matched entry from orphanIndex so it can't match again
		// if another file on disk has the same size+hash (duplicate content).
		orphanIndex[newSize] = append(candidates[:matchIdx], candidates[matchIdx+1:]...)
	}

	if len(movedOrphanIDs) > 0 {
		cache.SetMovedOrphanIDs(movedOrphanIDs)
		jobLog.Info("reconcile: move reconciliation complete", logger.Data{
			"moved_files": len(movedOrphanIDs),
		})
	}

	return nil
}

// computeFileSHA256 returns the lowercase hex-encoded sha256 of the file at path.
// It delegates to the fingerprint package so the algorithm is consistent with
// what the hash generation job stores.
func computeFileSHA256(path string) (string, error) {
	hash, err := fingerprint.ComputeSHA256(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return hash, nil
}

// syncBookFilepathAfterMove updates a book's Filepath to match a moved file's
// new location so that cover serving, supplement detection, and file
// organization keep resolving correctly after a rename.
//
// Books come in two flavors:
//   - Directory-based: Book.Filepath is the directory that contains the book's
//     files. We update Book.Filepath when all of the book's (main) files share
//     a single common directory after the move.
//   - Root-level: Book.Filepath is the file path itself (a single-file book
//     sitting directly in a library path). We detect this by the old
//     Book.Filepath matching the pre-move file path and update accordingly.
//
// The "all files share a common directory" rule is what lets this recover
// from books whose Filepath was already out of sync (for instance, a previous
// broken rename before this sync logic existed): if the single file moves to
// a new directory and that directory is the common home for all the book's
// files, the book follows — regardless of where its stale Filepath pointed.
//
// Multi-file books whose files are spread across multiple directories are
// left alone; later moves of their other files can reconcile the book
// independently.
func (w *Worker) syncBookFilepathAfterMove(ctx context.Context, file *models.File, oldFilePath, newFilePath string) error {
	if file.BookID == 0 {
		return nil
	}
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.Wrap(err, "retrieve book for filepath sync")
	}

	// Root-level single-file book: Book.Filepath points at the file itself.
	// Bring it along to the new file path.
	if book.Filepath == oldFilePath {
		book.Filepath = newFilePath
		if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{
			Columns: []string{"filepath"},
		}); err != nil {
			return errors.Wrap(err, "update book filepath (root-level)")
		}
		return nil
	}

	// Directory-based book: look at where ALL the book's main files live
	// (using the freshly-updated location of the moved file) and, if they
	// share a single directory, set Book.Filepath to that directory.
	mainFiles, err := w.bookService.ListFiles(ctx, books.ListFilesOptions{
		BookID: &file.BookID,
	})
	if err != nil {
		return errors.Wrap(err, "list files for filepath sync")
	}
	commonDir := ""
	for _, f := range mainFiles {
		if f.FileRole != models.FileRoleMain {
			continue
		}
		// Use the in-memory new path for the file we just moved since
		// the ListFiles result might reflect a slightly stale snapshot
		// depending on transaction visibility.
		fPath := f.Filepath
		if f.ID == file.ID {
			fPath = newFilePath
		}
		dir := filepath.Dir(fPath)
		if commonDir == "" {
			commonDir = dir
			continue
		}
		if dir != commonDir {
			// Files are spread across multiple directories — leave
			// Book.Filepath alone and let other moves reconcile it.
			return nil
		}
	}
	if commonDir == "" || commonDir == book.Filepath {
		return nil
	}

	book.Filepath = commonDir
	if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{
		Columns: []string{"filepath"},
	}); err != nil {
		return errors.Wrap(err, "update book filepath")
	}
	return nil
}
