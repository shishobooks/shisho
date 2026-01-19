package worker

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

var extensionsToScan = map[string]map[string]struct{}{
	".epub": {"application/epub+zip": {}},
	".m4b":  {"audio/x-m4a": {}, "video/mp4": {}},
	".cbz":  {"application/zip": {}},
}

var (
	// Non-greedy regex to match only the first [Author] pattern, not from first [ to last ].
	filepathAuthorRE   = regexp.MustCompile(`\[(.*?)]`)
	filepathNarratorRE = regexp.MustCompile(`\{(.*?)}`)
	// Regex to collapse multiple whitespace to single space.
	multiSpaceRE = regexp.MustCompile(`\s+`)
)

// generateCBZFileName creates a clean file name for CBZ files.
// Priority: 1) Title from ComicInfo (explicit metadata), 2) Series + Number (inferred), 3) Parse from filename.
func generateCBZFileName(metadata *mediafile.ParsedMetadata, filename string) string {
	// Strategy 1: Use Title if it doesn't look like a filename
	// (i.e., doesn't contain brackets which suggest it's just the raw filename).
	// Title is explicit metadata and should be preferred over inferred values.
	if trimmedTitle := strings.TrimSpace(metadata.Title); trimmedTitle != "" {
		if !filepathAuthorRE.MatchString(trimmedTitle) {
			return trimmedTitle
		}
	}

	// Strategy 2: Use Series + Number if available (inferred name)
	if metadata.Series != "" {
		if metadata.SeriesNumber != nil {
			return metadata.Series + " v" + formatSeriesNumber(*metadata.SeriesNumber)
		}
		return metadata.Series
	}

	// Strategy 3: Parse from filename - strip author brackets and extension
	return cleanCBZFilename(filename)
}

// cleanCBZFilename removes author brackets and extension from a CBZ filename.
// Example: "[Author Name] Comic Title v1.cbz" -> "Comic Title v1".
func cleanCBZFilename(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Remove all bracketed sections (author/creator info)
	name = filepathAuthorRE.ReplaceAllString(name, "")

	// Clean up extra whitespace
	name = strings.TrimSpace(name)
	name = multiSpaceRE.ReplaceAllString(name, " ")

	return name
}

// formatSeriesNumber formats a series number as a clean string.
// Whole numbers are formatted without decimals (e.g., 1.0 -> "1").
// Non-whole numbers keep their decimal (e.g., 1.5 -> "1.5").
func formatSeriesNumber(num float64) string {
	if num == float64(int(num)) {
		return strconv.Itoa(int(num))
	}
	return strconv.FormatFloat(num, 'f', -1, 64)
}

// isMainFileExtension returns true if the extension is a main file type.
func isMainFileExtension(ext string) bool {
	ext = strings.ToLower(ext)
	_, ok := extensionsToScan[ext]
	return ok
}

// isShishoSpecialFile returns true if the filename is a shisho-specific file.
func isShishoSpecialFile(filename string) bool {
	lower := strings.ToLower(filename)
	// Exclude cover files: *.cover.* pattern
	if strings.Contains(lower, ".cover.") {
		return true
	}
	// Exclude metadata files: *.metadata.json
	if strings.HasSuffix(lower, ".metadata.json") {
		return true
	}
	// Exclude canonical cover files (cover.png, cover.jpg, etc.)
	base := strings.TrimSuffix(lower, filepath.Ext(lower))
	if base == "cover" {
		return true
	}
	// Exclude user-provided cover files with common patterns (audiobook_cover.png, book_cover.jpg, etc.)
	if strings.HasSuffix(base, "_cover") || strings.HasSuffix(base, "-cover") {
		return true
	}
	return false
}

// matchesExcludePattern checks if filename matches any exclude pattern.
func matchesExcludePattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		// Simple prefix match for patterns starting with "."
		if strings.HasPrefix(pattern, ".") && strings.HasPrefix(filename, pattern) {
			return true
		}
		// Exact match
		if filename == pattern {
			return true
		}
		// Glob match for more complex patterns
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
	}
	return false
}

// discoverSupplements finds supplement files for a book directory.
func discoverSupplements(bookDir string, excludePatterns []string) ([]string, error) {
	var supplements []string

	err := filepath.WalkDir(bookDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		filename := filepath.Base(path)
		ext := filepath.Ext(path)

		// Skip main file types
		if isMainFileExtension(ext) {
			return nil
		}

		// Skip shisho special files
		if isShishoSpecialFile(filename) {
			return nil
		}

		// Skip files matching exclude patterns
		if matchesExcludePattern(filename, excludePatterns) {
			return nil
		}

		supplements = append(supplements, path)
		return nil
	})

	return supplements, err
}

// discoverRootLevelSupplements finds supplements for root-level books by basename matching.
func discoverRootLevelSupplements(mainFilePath string, libraryPath string, excludePatterns []string) ([]string, error) {
	var supplements []string

	// Get basename without extension
	mainFilename := filepath.Base(mainFilePath)
	mainBasename := strings.TrimSuffix(mainFilename, filepath.Ext(mainFilename))

	// List files in the same directory
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		ext := filepath.Ext(filename)
		basename := strings.TrimSuffix(filename, ext)

		// Skip if it's the main file itself
		if filename == mainFilename {
			continue
		}

		// Skip main file types
		if isMainFileExtension(ext) {
			continue
		}

		// Skip shisho special files
		if isShishoSpecialFile(filename) {
			continue
		}

		// Skip excluded patterns
		if matchesExcludePattern(filename, excludePatterns) {
			continue
		}

		// Match if basename is same or starts with main basename
		// "MyBook.pdf" matches "MyBook.m4b"
		// "MyBook - Guide.txt" matches "MyBook.m4b"
		if basename == mainBasename || strings.HasPrefix(basename, mainBasename) {
			supplements = append(supplements, filepath.Join(libraryPath, filename))
		}
	}

	return supplements, nil
}

func (w *Worker) ProcessScanJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	jobLog.Info("processing scan job", nil)

	allLibraries, err := w.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	// Filter to specific library if set
	if job != nil && job.LibraryID != nil {
		filtered := make([]*models.Library, 0, 1)
		for _, lib := range allLibraries {
			if lib.ID == *job.LibraryID {
				filtered = append(filtered, lib)
				break
			}
		}
		allLibraries = filtered
	}

	jobLog.Info("processing libraries", logger.Data{"count": len(allLibraries)})

	for _, library := range allLibraries {
		jobLog.Info("processing library", logger.Data{"library_id": library.ID})
		filesToScan := make([]string, 0)

		// Go through all the library paths to find all the .cbz files.
		for _, libraryPath := range library.LibraryPaths {
			jobLog.Info("processing library path", logger.Data{"library_path_id": libraryPath.ID, "library_path": libraryPath.Filepath})
			err := filepath.WalkDir(libraryPath.Filepath, func(path string, info fs.DirEntry, err error) error {
				if err != nil {
					return errors.WithStack(err)
				}
				if info.IsDir() {
					// We don't do anything explicitly to directories.
					return nil
				}
				// TODO: support having cover.jpg and cover_audiobook.jpg
				expectedMimeTypes, ok := extensionsToScan[filepath.Ext(path)]
				if !ok {
					// We're only looking for certain files right now.
					return nil
				}
				mtype, err := mimetype.DetectFile(path)
				if err != nil {
					// We can't detect the mime type, so we just skip it.
					jobLog.Warn("can't detect the mime type of a file with a valid extension", logger.Data{"path": path, "err": err.Error()})
					return nil
				}
				if _, ok := expectedMimeTypes[mtype.String()]; !ok {
					// Since files can have any extension, we try to check it against the mime type that we expect it to
					// be. This might be overly restrictive in the future, so it might be something that we remove, but
					// we can keep it for now.
					jobLog.Warn("mime type is not expected for extension", logger.Data{"path": path, "mimetype": mtype.String()})
					return nil
				}

				// This is a file that we care about, so store it in the slice. We do this so that we can know the total
				// number of files that we need to scan before we start doing any real work so that we can accurately
				// update the progress of the job.
				filesToScan = append(filesToScan, path)

				return nil
			})
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Track books that need organization after scan completes.
		// Organization is deferred to avoid breaking file paths during scan.
		booksToOrganize := make(map[int]struct{})

		for _, path := range filesToScan {
			result, err := w.scanInternal(ctx, ScanOptions{
				FilePath:  path,
				LibraryID: library.ID,
				JobLog:    jobLog,
			})
			if err != nil {
				jobLog.Warn("failed to scan file", logger.Data{"path": path, "error": err.Error()})
				continue
			}
			if result != nil && result.Book != nil {
				booksToOrganize[result.Book.ID] = struct{}{}
			}
		}

		// Cleanup orphaned files (in DB but not on disk)
		// Uses the unified Scan() function which handles file deletion properly
		existingFiles, err := w.bookService.ListFilesForLibrary(ctx, library.ID)
		if err != nil {
			jobLog.Warn("failed to list files for orphan cleanup", logger.Data{"error": err.Error()})
		} else {
			// Build a set of all file paths we scanned
			scannedPaths := make(map[string]struct{}, len(filesToScan))
			for _, path := range filesToScan {
				scannedPaths[path] = struct{}{}
			}

			for _, file := range existingFiles {
				if _, seen := scannedPaths[file.Filepath]; !seen {
					jobLog.Info("cleaning up orphaned file", logger.Data{"file_id": file.ID, "filepath": file.Filepath})
					_, err := w.scanInternal(ctx, ScanOptions{FileID: file.ID})
					if err != nil {
						jobLog.Warn("failed to cleanup orphaned file", logger.Data{"file_id": file.ID, "error": err.Error()})
					}
				}
			}
		}

		// Organize files after all scanning is complete
		if library.OrganizeFileStructure && len(booksToOrganize) > 0 {
			jobLog.Info("organizing books after scan", logger.Data{"count": len(booksToOrganize)})
			for bookID := range booksToOrganize {
				book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
				if err != nil {
					jobLog.Warn("failed to retrieve book for organization", logger.Data{
						"book_id": bookID,
						"error":   err.Error(),
					})
					continue
				}

				err = w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{OrganizeFiles: true})
				if err != nil {
					jobLog.Warn("failed to organize book", logger.Data{
						"book_id": bookID,
						"error":   err.Error(),
					})
				}
			}
		}
	}

	// Cleanup orphaned series (soft delete series with no books)
	deletedCount, err := w.seriesService.CleanupOrphanedSeries(ctx)
	if err != nil {
		jobLog.Error("failed to cleanup orphaned series", err, nil)
	} else if deletedCount > 0 {
		jobLog.Info("cleaned up orphaned series", logger.Data{"count": deletedCount})
	}

	// Cleanup orphaned people (delete people with no authors or narrators)
	deletedPeopleCount, err := w.personService.CleanupOrphanedPeople(ctx)
	if err != nil {
		jobLog.Error("failed to cleanup orphaned people", err, nil)
	} else if deletedPeopleCount > 0 {
		jobLog.Info("cleaned up orphaned people", logger.Data{"count": deletedPeopleCount})
	}

	// Cleanup orphaned genres (delete genres with no books)
	deletedGenresCount, err := w.genreService.CleanupOrphanedGenres(ctx)
	if err != nil {
		jobLog.Error("failed to cleanup orphaned genres", err, nil)
	} else if deletedGenresCount > 0 {
		jobLog.Info("cleaned up orphaned genres", logger.Data{"count": deletedGenresCount})
	}

	// Cleanup orphaned tags (delete tags with no books)
	deletedTagsCount, err := w.tagService.CleanupOrphanedTags(ctx)
	if err != nil {
		jobLog.Error("failed to cleanup orphaned tags", err, nil)
	} else if deletedTagsCount > 0 {
		jobLog.Info("cleaned up orphaned tags", logger.Data{"count": deletedTagsCount})
	}

	// Rebuild FTS indexes after scan completes
	if w.searchService != nil {
		jobLog.Info("rebuilding search indexes", nil)
		err = w.searchService.RebuildAllIndexes(ctx)
		if err != nil {
			jobLog.Error("failed to rebuild search indexes", err, nil)
		} else {
			jobLog.Info("search indexes rebuilt successfully", nil)
		}
	}

	jobLog.Info("finished scan job", nil)
	return nil
}
