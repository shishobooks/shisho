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
	".pdf":  {"application/pdf": {}},
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
			prefix := "v"
			if metadata.SeriesNumberUnit != nil && *metadata.SeriesNumberUnit == models.SeriesNumberUnitChapter {
				prefix = "c"
			}
			return metadata.Series + " " + prefix + formatSeriesNumber(*metadata.SeriesNumber)
		}
		return metadata.Series
	}

	// Strategy 3: Parse from filename - strip author brackets and extension
	return cleanCBZFilename(filename)
}

// cleanCBZFilename removes author brackets, parenthesized metadata, and extension from a CBZ filename,
// then normalizes series number indicators (e.g., "v02" -> "v002", "Ch.5" -> "c005").
// Example: "[Author Name] Comic Title v01 (2020) (Digital).cbz" -> "Comic Title v001".
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

	// Normalize series number indicators (v02 -> v002, Ch.5 -> c005)
	if normalized, _, hasNumber := fileutils.NormalizeSeriesNumberInTitle(name, models.FileTypeCBZ); hasNumber {
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
	for _, pattern := range fileutils.ShishoSpecialFilePatterns {
		if matched, _ := filepath.Match(pattern, lower); matched {
			return true
		}
	}
	return false
}

// looksLikePDFSupplement returns true if filename has a .pdf extension and its
// basename (without extension, trimmed, lowercased) is an exact case-insensitive
// match for any entry in names. Returns false when names is empty/nil.
func looksLikePDFSupplement(filename string, names []string) bool {
	if len(names) == 0 {
		return false
	}
	filename = filepath.Base(filename)
	rawExt := filepath.Ext(filename)
	if strings.ToLower(rawExt) != ".pdf" {
		return false
	}
	basename := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(filename, rawExt)))
	if basename == "" {
		return false
	}
	for _, name := range names {
		if strings.ToLower(strings.TrimSpace(name)) == basename {
			return true
		}
	}
	return false
}

// hasNonPDFMainSibling returns true if dir (recursive) contains at least one
// file with a non-PDF main-eligible extension. Main-eligible means EPUB / CBZ /
// M4B or any extension in pluginExts (which comes from
// pluginManager.RegisteredFileExtensions() — keys are extensions without the
// leading dot, lowercase). pluginExts may be nil. Hidden subdirectories
// (e.g. .git, .calibre, .stversions) are skipped so a stray ebook inside an
// app-state directory doesn't accidentally qualify as a sibling.
func hasNonPDFMainSibling(dir string, pluginExts map[string]struct{}) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Skip hidden directories so stray files inside app-state dirs
			// (.git, .calibre, .stversions, etc.) don't qualify as siblings.
			// The walk root itself may have a leading dot in odd setups, so
			// only skip when we're not at the root.
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if ext == models.FileTypePDF {
			return nil
		}
		switch ext {
		case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypeM4B:
			found = true
			return filepath.SkipAll
		}
		if pluginExts != nil {
			if _, ok := pluginExts[ext]; ok {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}

// partitionSupplementPDFsLast returns paths reordered so that PDFs whose
// basename matches the supplement name list appear after every other path.
// Order within each partition is preserved (stable). The input slice is not
// mutated. When names is empty/nil, the input is returned unchanged.
func partitionSupplementPDFsLast(paths []string, names []string) []string {
	if len(names) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	var deferred []string
	for _, p := range paths {
		if looksLikePDFSupplement(filepath.Base(p), names) {
			deferred = append(deferred, p)
			continue
		}
		out = append(out, p)
	}
	return append(out, deferred...)
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
		// Honor cancellation between libraries — if the worker is shutting
		// down mid-scan, don't start a fresh per-library walk.
		if err := ctx.Err(); err != nil {
			return err
		}
		jobLog.Info("processing library", logger.Data{"library_id": library.ID})
		filesToScan := make([]string, 0)

		// Pre-load all known files (main + supplement) for fast lookup during
		// discovery and scan. The cache must include supplements so the scan
		// walk can detect a supplement sharing a scannable extension (e.g. a
		// .pdf companion next to a .epub) and skip it instead of trying to
		// recreate it as a main file and hitting UNIQUE(filepath, library_id).
		cache := NewScanCache()
		cache.SetAliasLister(NewAliasServiceAdapter(w.aliasService))
		allFiles, err := w.bookService.ListAllFilesForLibrary(ctx, library.ID)
		if err != nil {
			jobLog.Warn("failed to pre-load files", logger.Data{"error": err.Error()})
		} else {
			cache.LoadKnownFiles(allFiles)
			jobLog.Info("pre-loaded known files", logger.Data{"count": len(allFiles)})
		}
		// Orphan cleanup only considers main files — supplements don't need
		// orphan cleanup (their lifecycle follows the parent book).
		var existingFiles []*models.File
		for _, f := range allFiles {
			if f.FileRole == models.FileRoleMain {
				existingFiles = append(existingFiles, f)
			}
		}

		// Go through all the library paths to find all the .cbz files.
		for _, libraryPath := range library.LibraryPaths {
			jobLog.Info("processing library path", logger.Data{"library_path_id": libraryPath.ID, "library_path": libraryPath.Filepath})
			err := filepath.WalkDir(libraryPath.Filepath, func(path string, info fs.DirEntry, err error) error {
				// Stop walking the tree if the worker is shutting down. Returning
				// the cancellation error aborts the outer WalkDir call so we bail
				// out of the library rather than enumerating thousands more paths.
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
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
				// Skip MIME detection for files we already know about — they were
				// validated when first imported, so re-checking is redundant I/O.
				if cache.GetKnownFile(path) != nil {
					filesToScan = append(filesToScan, path)
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

		// Defer supplement-named PDFs so non-supplement files in the same
		// directory get processed first by the parallel worker pool. This
		// makes the supplement classification ordering-independent in the
		// common case where a sibling main file exists, even though the
		// on-disk sibling check in scanFileCreateNew is the actual
		// correctness mechanism.
		filesToScan = partitionSupplementPDFsLast(filesToScan, w.config.PDFSupplementFilenames)

		// Run input converters on discovered files
		if w.pluginManager != nil {
			convertedFiles := w.runInputConverters(ctx, filesToScan, jobLog, library.ID)
			filesToScan = append(filesToScan, convertedFiles...)
		}

		// --- Move reconciliation ---
		// Detect files that were moved/renamed while the server was offline by
		// matching candidate-orphan DB rows (known path missing from disk) against
		// unknown-new on-disk paths (disk path missing from the cache) using
		// size+sha256 comparison. Must run BEFORE the parallel worker pool so that
		// the cache is up to date — moved orphans are registered at their new paths
		// and won't be double-processed.
		//
		// Prime the library root paths on the cache so syncBookFilepathAfterMove
		// (invoked per reconciled move) can enforce its "no library-root Book.Filepath"
		// guard without a per-call DB lookup.
		if len(library.LibraryPaths) > 0 {
			roots := make([]string, 0, len(library.LibraryPaths))
			for _, lp := range library.LibraryPaths {
				roots = append(roots, lp.Filepath)
			}
			cache.SetLibraryRootPaths(roots)
		}
		if err := w.reconcileMoves(ctx, existingFiles, filesToScan, cache, jobLog); err != nil {
			jobLog.Warn("move reconciliation encountered an error", logger.Data{"error": err.Error()})
			// Non-fatal: proceed with the scan; moved files fall through to orphan cleanup.
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

		fileChan := make(chan string, len(filesToScan))
		resultChan := make(chan scanResult, len(filesToScan))

		// Start workers. Each worker checks ctx.Err() at the top of its
		// loop so that once the worker is shutting down, queued paths
		// still in fileChan are drained without running scanInternal.
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for path := range fileChan {
					if ctx.Err() != nil {
						continue
					}
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

		// Dispatch files; bail early on shutdown so queued workers can drain.
	dispatchLoop:
		for _, path := range filesToScan {
			select {
			case <-ctx.Done():
				break dispatchLoop
			case fileChan <- path:
			}
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

		// If we were cancelled mid-scan, return now rather than running orphan
		// cleanup/organize/hash-gen queuing on a partial result set.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Books whose files were reconciled as moves should also be organized
		// so organize_file_structure can rename their folders back into the
		// structured layout. Only merge these when the library actually has
		// organize enabled — otherwise it's wasted work since the organize
		// step below is gated on the same setting.
		if library.OrganizeFileStructure {
			for bookID := range cache.MovedBookIDs() {
				booksToOrganize[bookID] = struct{}{}
			}
		}

		// Cleanup orphaned files (in DB but not on disk) using batch operations.
		// Uses the pre-loaded files from before the scan to avoid a second DB query.
		// The cache is passed so that files already reconciled as moves are skipped.
		if existingFiles != nil {
			scannedPaths := make(map[string]struct{}, len(filesToScan))
			for _, path := range filesToScan {
				scannedPaths[path] = struct{}{}
			}
			w.cleanupOrphanedFiles(ctx, existingFiles, scannedPaths, library, jobLog, cache)
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

		// Queue async sha256 hash generation for files that still lack a fingerprint.
		// Handles both initial backfill and newly-discovered files from this scan.
		if err := EnsureHashGenerationJob(ctx, w.jobService, library.ID); err != nil {
			jobLog.Warn("failed to ensure hash generation job", logger.Data{"error": err.Error()})
		}
	}

	// Cleanup orphaned entities (series, people, genres, tags)
	w.cleanupOrphanedEntities(ctx, logger.FromContext(ctx))

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
