package worker

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fileutils"
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
	// Matches parenthesized metadata sections like (2020), (Digital), (group).
	filepathParensRE = regexp.MustCompile(`\([^)]*\)`)
	// Regex to collapse multiple whitespace to single space.
	multiSpaceRE = regexp.MustCompile(`\s+`)
)

// scanResult holds the result of a single file scan for the worker pool.
type scanResult struct {
	BookID int
	Path   string
	Err    error
}

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

// cleanCBZFilename removes author brackets, parenthesized metadata, and extension from a CBZ filename,
// then normalizes volume indicators (e.g., "v02" -> "v2").
// Example: "[Author Name] Comic Title v01 (2020) (Digital).cbz" -> "Comic Title v1".
func cleanCBZFilename(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Remove all bracketed sections (author/creator info)
	name = filepathAuthorRE.ReplaceAllString(name, "")

	// Remove all parenthesized sections (year, quality, group)
	name = filepathParensRE.ReplaceAllString(name, "")

	// Clean up extra whitespace
	name = strings.TrimSpace(name)
	name = multiSpaceRE.ReplaceAllString(name, " ")

	// Normalize volume indicators (v02 -> v002)
	if normalized, hasVolume := fileutils.NormalizeVolumeInTitle(name, models.FileTypeCBZ); hasVolume {
		return normalized
	}

	return name
}

// formatSeriesNumber formats a series number as a zero-padded string for
// lexicographic sorting (e.g., 1 -> "001", 42 -> "042", 1.5 -> "001.5").
func formatSeriesNumber(num float64) string {
	if num == float64(int(num)) {
		return fmt.Sprintf("%03d", int(num))
	}
	intPart := int(num)
	fracStr := strconv.FormatFloat(num-float64(intPart), 'f', -1, 64)
	// fracStr is like "0.5", strip the leading "0"
	return fmt.Sprintf("%03d%s", intPart, fracStr[1:])
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
	// Exclude individual cover files: *.cover.* pattern
	if strings.Contains(lower, ".cover.") {
		return true
	}
	// Exclude metadata files: *.metadata.json
	if strings.HasSuffix(lower, ".metadata.json") {
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
				ext := filepath.Ext(path)
				expectedMimeTypes, ok := extensionsToScan[ext]
				if !ok {
					// Check plugin-registered extensions (file parsers and converter source types)
					if w.pluginManager != nil {
						extNoDot := strings.TrimPrefix(ext, ".")
						pluginExts := w.pluginManager.RegisteredFileExtensions()
						converterExts := w.pluginManager.RegisteredConverterExtensions()
						if _, isParser := pluginExts[extNoDot]; isParser {
							filesToScan = append(filesToScan, path)
							return nil
						}
						if _, isConverter := converterExts[extNoDot]; isConverter {
							filesToScan = append(filesToScan, path)
							return nil
						}
					}
					// Not a built-in or plugin-registered extension, skip.
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

		// Run input converters on discovered files
		if w.pluginManager != nil {
			convertedFiles := w.runInputConverters(ctx, filesToScan, jobLog, library.ID)
			filesToScan = append(filesToScan, convertedFiles...)
		}

		// Track books that need organization after scan completes.
		// Organization is deferred to avoid breaking file paths during scan.
		booksToOrganize := make(map[int]struct{})

		// Parallel file processing with worker pool
		workerCount := max(runtime.NumCPU(), 4)
		jobLog.Info("starting parallel scan", logger.Data{
			"worker_count":  workerCount,
			"files_to_scan": len(filesToScan),
		})

		cache := NewScanCache()
		fileChan := make(chan string, len(filesToScan))
		resultChan := make(chan scanResult, len(filesToScan))

		// Start workers
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for path := range fileChan {
					result, err := w.scanInternal(ctx, ScanOptions{
						FilePath:  path,
						LibraryID: library.ID,
						JobLog:    jobLog,
					}, cache)

					sr := scanResult{Path: path}
					if err != nil {
						sr.Err = err
					} else if result != nil && result.Book != nil {
						sr.BookID = result.Book.ID
					}
					resultChan <- sr
				}
			}()
		}

		// Dispatch files
		for _, path := range filesToScan {
			fileChan <- path
		}
		close(fileChan)

		// Collect results in background
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Process results
		for result := range resultChan {
			if result.Err != nil {
				jobLog.Warn("failed to scan file", logger.Data{"path": result.Path, "error": result.Err.Error()})
				continue
			}
			if result.BookID != 0 {
				booksToOrganize[result.BookID] = struct{}{}
			}
		}

		jobLog.Info("parallel scan complete", logger.Data{
			"persons_cached":    cache.PersonCount(),
			"genres_cached":     cache.GenreCount(),
			"tags_cached":       cache.TagCount(),
			"series_cached":     cache.SeriesCount(),
			"publishers_cached": cache.PublisherCount(),
			"imprints_cached":   cache.ImprintCount(),
		})

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
					_, err := w.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
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

// runInputConverters runs input converter plugins on discovered files.
// It returns a list of newly converted files that should also be scanned.
func (w *Worker) runInputConverters(ctx context.Context, filesToScan []string, jobLog *joblogs.JobLogger, libraryID int) []string {
	runtimes, err := w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookInputConverter, libraryID)
	if err != nil || len(runtimes) == 0 {
		return nil
	}

	var convertedFiles []string
	// Track which source+target pairs have been processed to avoid duplicate conversions
	processedPairs := make(map[string]struct{})

	for _, path := range filesToScan {
		ext := strings.TrimPrefix(filepath.Ext(path), ".")

		for _, rt := range runtimes {
			converterCap := rt.Manifest().Capabilities.InputConverter
			if converterCap == nil {
				continue
			}

			// Check if this converter handles this extension
			handles := false
			for _, srcType := range converterCap.SourceTypes {
				if srcType == ext {
					handles = true
					break
				}
			}
			if !handles {
				continue
			}

			// Check for duplicate source+target pair
			pairKey := path + "\x00" + converterCap.TargetType
			if _, done := processedPairs[pairKey]; done {
				continue
			}
			processedPairs[pairKey] = struct{}{}

			// MIME validation if mimeTypes declared
			if len(converterCap.MIMETypes) > 0 {
				mtype, mErr := mimetype.DetectFile(path)
				if mErr != nil {
					jobLog.Warn("converter: failed to detect MIME type", logger.Data{"path": path, "error": mErr.Error()})
					continue
				}
				mimeMatch := false
				for _, allowed := range converterCap.MIMETypes {
					if mtype.String() == allowed {
						mimeMatch = true
						break
					}
				}
				if !mimeMatch {
					continue
				}
			}

			// Create temp dir for conversion output
			targetDir, tErr := os.MkdirTemp("", "shisho-convert-*")
			if tErr != nil {
				jobLog.Warn("converter: failed to create temp dir", logger.Data{"error": tErr.Error()})
				continue
			}

			result, cErr := w.pluginManager.RunInputConverter(ctx, rt, path, targetDir)
			if cErr != nil {
				jobLog.Warn("converter failed", logger.Data{
					"plugin": rt.Manifest().ID,
					"path":   path,
					"error":  cErr.Error(),
				})
				os.RemoveAll(targetDir)
				continue
			}

			if !result.Success || result.TargetPath == "" {
				os.RemoveAll(targetDir)
				continue
			}

			// Move converted file to library alongside source
			sourceDir := filepath.Dir(path)
			destFilename := filepath.Base(result.TargetPath)
			destPath := filepath.Join(sourceDir, destFilename)

			if _, sErr := os.Stat(destPath); sErr == nil {
				// Destination already exists, skip
				jobLog.Info("converter output already exists, skipping", logger.Data{"dest": destPath})
				os.RemoveAll(targetDir)
				continue
			}

			if mErr := moveFile(result.TargetPath, destPath); mErr != nil {
				jobLog.Warn("converter: failed to move output", logger.Data{
					"source": result.TargetPath,
					"dest":   destPath,
					"error":  mErr.Error(),
				})
				os.RemoveAll(targetDir)
				continue
			}

			os.RemoveAll(targetDir)
			convertedFiles = append(convertedFiles, destPath)
			jobLog.Info("converter produced file", logger.Data{"source": path, "dest": destPath})
		}
	}

	return convertedFiles
}

// moveFile moves a file from src to dst. If os.Rename fails (e.g., cross-device),
// it falls back to copy+remove.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fallback: copy + remove
	srcFile, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "failed to open source")
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "failed to create destination")
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return errors.Wrap(err, "failed to copy file")
	}

	if err := dstFile.Close(); err != nil {
		os.Remove(dst)
		return errors.Wrap(err, "failed to close destination")
	}

	srcFile.Close()
	return os.Remove(src)
}
