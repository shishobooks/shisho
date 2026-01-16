package worker

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/shishobooks/shisho/pkg/sidecar"
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
			err := w.scanFile(ctx, path, library.ID, booksToOrganize, jobLog)
			if err != nil {
				return errors.WithStack(err)
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

	// TODO: go through and delete files/books that have been deleted

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

func (w *Worker) scanFile(ctx context.Context, path string, libraryID int, booksToOrganize map[int]struct{}, jobLog *joblogs.JobLogger) error {
	jobLog.Info("processing file", logger.Data{"path": path})

	// Check if this file already exists based on its filepath.
	existingFile, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		Filepath:  &path,
		LibraryID: &libraryID,
	})
	if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
		return errors.WithStack(err)
	}
	if existingFile != nil {
		jobLog.Info("file already exists, will check for metadata updates", logger.Data{"file_id": existingFile.ID})
		// Check if cover is missing and recover it if needed
		if err := w.recoverMissingCover(ctx, existingFile); err != nil {
			jobLog.Warn("failed to recover missing cover", logger.Data{"file_id": existingFile.ID, "error": err.Error()})
		}
		// Continue to metadata extraction - we'll compare and update later
	}

	// Get the size of the file.
	stats, err := os.Stat(path)
	if err != nil {
		// File may have been moved by concurrent API-triggered organization
		jobLog.Warn("file not accessible, skipping", logger.Data{"path": path, "error": err.Error()})
		return nil
	}
	size := stats.Size()
	fileType := strings.ToLower(strings.ReplaceAll(filepath.Ext(path), ".", ""))

	// Determine if this is a root-level file by checking if the file is directly in a library path
	tempBookPath := filepath.Dir(path)
	filename := filepath.Base(tempBookPath)
	isRootLevelFile := false

	// Get the library to check its paths
	library, err := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &libraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check if file is directly in any library path
	for _, libraryPath := range library.LibraryPaths {
		if tempBookPath == libraryPath.Filepath {
			isRootLevelFile = true
			break
		}
	}

	// Set book path based on whether this is a root-level file
	var bookPath string
	if isRootLevelFile {
		// For root-level files, each file is its own book - use the full file path
		bookPath = path
		filename = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	} else {
		// For directory-based files, use the directory path (multiple files can belong to same book)
		bookPath = tempBookPath
	}

	title := strings.TrimSpace(filepathNarratorRE.ReplaceAllString(filepathAuthorRE.ReplaceAllString(filename, ""), ""))
	// If title is empty after stripping author/narrator patterns, fall back to the raw filename
	// (the file base name without extension, which is guaranteed to be non-empty for valid files)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	titleSource := models.DataSourceFilepath
	authors := make([]mediafile.ParsedAuthor, 0)
	authorSource := models.DataSourceFilepath
	narratorNames := make([]string, 0)
	narratorSource := models.DataSourceFilepath
	identifiers := make([]mediafile.ParsedIdentifier, 0)
	identifierSource := models.DataSourceFilepath
	// Series data: supports multiple series per book
	type seriesData struct {
		name      string
		number    *float64
		sortOrder int
	}
	var seriesList []seriesData
	seriesSource := models.DataSourceFilepath
	var genreNames []string
	genreSource := models.DataSourceFilepath
	var tagNames []string
	tagSource := models.DataSourceFilepath
	var coverMimeType *string
	var coverSource *string
	var subtitle *string
	subtitleSource := models.DataSourceFilepath
	var description *string
	descriptionSource := models.DataSourceFilepath

	// File-level metadata
	var fileURL *string
	fileURLSource := models.DataSourceFilepath
	var publisherName *string
	publisherSource := models.DataSourceFilepath
	var imprintName *string
	imprintSource := models.DataSourceFilepath
	var releaseDate *time.Time
	releaseDateSource := models.DataSourceFilepath
	var fileName *string
	fileNameSource := models.DataSourceFilepath

	// Extract metadata from each file based on its file type.
	var metadata *mediafile.ParsedMetadata
	switch fileType {
	case models.FileTypeEPUB:
		jobLog.Info("parsing file as epub", logger.Data{"file_type": fileType})
		metadata, err = epub.Parse(path)
		if err != nil {
			return errors.WithStack(err)
		}
	case models.FileTypeCBZ:
		jobLog.Info("parsing file as cbz", logger.Data{"file_type": fileType})
		metadata, err = cbz.Parse(path)
		if err != nil {
			return errors.WithStack(err)
		}
	case models.FileTypeM4B:
		jobLog.Info("parsing file as m4b", logger.Data{"file_type": fileType})
		metadata, err = mp4.Parse(path)
		if err != nil {
			jobLog.Error("failed to parse as m4b", err, logger.Data{"file_type": fileType})
			return nil
		}
	}

	// Track cover page for CBZ files
	var coverPage *int

	if metadata != nil {
		// Only use metadata values if they're non-empty (after trimming whitespace), otherwise keep filepath-based values
		if trimmedTitle := strings.TrimSpace(metadata.Title); trimmedTitle != "" {
			title = trimmedTitle
			titleSource = metadata.DataSource
		}
		if len(metadata.Authors) > 0 {
			authorSource = metadata.DataSource
			authors = append(authors, metadata.Authors...)
		}
		// Capture cover page for CBZ files
		if metadata.CoverPage != nil {
			coverPage = metadata.CoverPage
		}
		if len(metadata.Narrators) > 0 {
			narratorSource = metadata.DataSource
			narratorNames = append(narratorNames, metadata.Narrators...)
		}
		if metadata.Series != "" {
			seriesList = append(seriesList, seriesData{
				name:      metadata.Series,
				number:    metadata.SeriesNumber,
				sortOrder: 1,
			})
			seriesSource = metadata.DataSource
		}
		if len(metadata.Genres) > 0 {
			genreNames = append(genreNames, metadata.Genres...)
			genreSource = metadata.DataSource
		}
		if len(metadata.Tags) > 0 {
			tagNames = append(tagNames, metadata.Tags...)
			tagSource = metadata.DataSource
		}
		if len(metadata.Identifiers) > 0 {
			identifiers = metadata.Identifiers
			identifierSource = metadata.DataSource
		}
		if metadata.CoverMimeType != "" {
			coverMimeType = &metadata.CoverMimeType
		}
		if trimmedSubtitle := strings.TrimSpace(metadata.Subtitle); trimmedSubtitle != "" {
			subtitle = &trimmedSubtitle
			subtitleSource = metadata.DataSource
		}
		if trimmedDescription := strings.TrimSpace(metadata.Description); trimmedDescription != "" {
			description = &trimmedDescription
			descriptionSource = metadata.DataSource
		}
		// File-level metadata
		if trimmedURL := strings.TrimSpace(metadata.URL); trimmedURL != "" {
			fileURL = &trimmedURL
			fileURLSource = metadata.DataSource
		}
		if trimmedPublisher := strings.TrimSpace(metadata.Publisher); trimmedPublisher != "" {
			publisherName = &trimmedPublisher
			publisherSource = metadata.DataSource
		}
		if trimmedImprint := strings.TrimSpace(metadata.Imprint); trimmedImprint != "" {
			imprintName = &trimmedImprint
			imprintSource = metadata.DataSource
		}
		if metadata.ReleaseDate != nil {
			releaseDate = metadata.ReleaseDate
			releaseDateSource = metadata.DataSource
		}
		// Populate file name from metadata
		// For CBZ files, use special logic to generate a clean file name
		if fileType == models.FileTypeCBZ {
			cbzFileName := generateCBZFileName(metadata, filename)
			if cbzFileName != "" {
				fileName = &cbzFileName
				fileNameSource = metadata.DataSource
			}
		} else if trimmedTitle := strings.TrimSpace(metadata.Title); trimmedTitle != "" {
			fileName = &trimmedTitle
			fileNameSource = metadata.DataSource
		}
	}

	// If we didn't find any authors in the metadata, try getting it from the filename.
	if len(authors) == 0 && filepathAuthorRE.MatchString(filename) {
		jobLog.Info("no authors found in metadata; parsing filename", logger.Data{"filename": filename})
		// Use FindAllStringSubmatch to get the capture group (content inside brackets)
		matches := filepathAuthorRE.FindAllStringSubmatch(filename, -1)
		if len(matches) > 0 && len(matches[0]) > 1 {
			// matches[0][1] is the first capture group (author name without brackets)
			names := fileutils.SplitNames(matches[0][1])
			for _, author := range names {
				// Filepath-based authors have no specific role (generic author)
				authors = append(authors, mediafile.ParsedAuthor{Name: author, Role: ""})
			}
		}
	}

	// If we didn't find any narrators in the metadata, try getting it from the filename.
	// For directory-based files, `filename` is the directory name, so also check the actual file name.
	if len(narratorNames) == 0 {
		actualFilename := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		// Check directory name first, then fall back to actual filename
		nameToCheck := filename
		if !filepathNarratorRE.MatchString(filename) && filepathNarratorRE.MatchString(actualFilename) {
			nameToCheck = actualFilename
		}
		if filepathNarratorRE.MatchString(nameToCheck) {
			jobLog.Info("no narrators found in metadata; parsing filename", logger.Data{"filename": nameToCheck})
			// Use FindAllStringSubmatch to get the capture group (content inside braces)
			matches := filepathNarratorRE.FindAllStringSubmatch(nameToCheck, -1)
			if len(matches) > 0 && len(matches[0]) > 1 {
				// matches[0][1] is the first capture group (narrator name without braces)
				names := fileutils.SplitNames(matches[0][1])
				narratorNames = append(narratorNames, names...)
			}
		}
	}

	// Normalize volume indicators in title for CBZ files (after all metadata extraction)
	if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, fileType); hasVolume {
		title = normalizedTitle
	}

	// Infer series from title if it contains a volume indicator and no series is set from a higher priority source
	if len(seriesList) == 0 {
		if seriesName, volumeNumber, ok := fileutils.ExtractSeriesFromTitle(title, fileType); ok {
			seriesList = append(seriesList, seriesData{
				name:      seriesName,
				number:    volumeNumber,
				sortOrder: 1,
			})
			seriesSource = models.DataSourceFilepath
		}
	}

	// Read sidecar files if they exist (sidecar has priority 1, higher than file metadata)
	var fileSidecarData *sidecar.FileSidecar
	bookSidecarData, err := sidecar.ReadBookSidecar(bookPath)
	if err != nil {
		jobLog.Warn("failed to read book sidecar", logger.Data{"error": err.Error()})
	}
	fileSidecarData, err = sidecar.ReadFileSidecar(path)
	if err != nil {
		jobLog.Warn("failed to read file sidecar", logger.Data{"error": err.Error()})
	}

	// Apply book sidecar data (higher priority than file metadata)
	if bookSidecarData != nil {
		jobLog.Info("applying book sidecar data", nil)
		if bookSidecarData.Title != "" && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[titleSource] {
			title = bookSidecarData.Title
			titleSource = models.DataSourceSidecar
		}
		if len(bookSidecarData.Authors) > 0 && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[authorSource] {
			authorSource = models.DataSourceSidecar
			authors = make([]mediafile.ParsedAuthor, 0)
			for _, a := range bookSidecarData.Authors {
				role := ""
				if a.Role != nil {
					role = *a.Role
				}
				authors = append(authors, mediafile.ParsedAuthor{Name: a.Name, Role: role})
			}
		}
		if len(bookSidecarData.Series) > 0 && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[seriesSource] {
			seriesSource = models.DataSourceSidecar
			seriesList = make([]seriesData, 0, len(bookSidecarData.Series))
			for _, s := range bookSidecarData.Series {
				if s.Name != "" {
					seriesList = append(seriesList, seriesData{
						name:      s.Name,
						number:    s.Number,
						sortOrder: s.SortOrder,
					})
				}
			}
		}
		if len(bookSidecarData.Genres) > 0 && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[genreSource] {
			genreSource = models.DataSourceSidecar
			genreNames = bookSidecarData.Genres
		}
		if len(bookSidecarData.Tags) > 0 && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[tagSource] {
			tagSource = models.DataSourceSidecar
			tagNames = bookSidecarData.Tags
		}
		if bookSidecarData.Subtitle != nil && *bookSidecarData.Subtitle != "" && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[subtitleSource] {
			subtitle = bookSidecarData.Subtitle
			subtitleSource = models.DataSourceSidecar
		}
		if bookSidecarData.Description != nil && *bookSidecarData.Description != "" && models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[descriptionSource] {
			description = bookSidecarData.Description
			descriptionSource = models.DataSourceSidecar
		}
	}

	// Final safety check: ensure title is never empty after all processing.
	// This catches any edge case where metadata/sidecar provided an empty/whitespace title.
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		titleSource = models.DataSourceFilepath
		jobLog.Warn("title was empty after all processing, falling back to filename", logger.Data{"title": title})
	}

	// First, check if there's already a book for this file path
	// This covers the case where a file was previously organized but moved back to root level
	existingBookByFile, err := w.bookService.RetrieveBookByFilePath(ctx, path, libraryID)
	if err != nil && !errors.Is(err, errcodes.NotFound("Book")) {
		return errors.WithStack(err)
	}

	// Check if this is a root-level file that needs organization.
	// Organization is deferred to post-scan phase to avoid breaking file paths during scan.
	needsOrganization := false
	if library.OrganizeFileStructure && isRootLevelFile {
		// If there's already a book for this exact file path, skip organization to avoid duplicates
		if existingBookByFile != nil {
			jobLog.Info("skipping organization - book already exists for this file", logger.Data{
				"book_id":   existingBookByFile.ID,
				"file_path": path,
			})
		} else {
			needsOrganization = true
		}
	}

	// Determine which existing book to use
	var existingBook *models.Book
	if existingBookByFile != nil {
		// If we found a book by the original file path, use that
		existingBook = existingBookByFile
		jobLog.Info("using existing book found by file path", logger.Data{"book_id": existingBook.ID})
	} else {
		// Otherwise, check for existing book by the final book path (after potential organization)
		existingBook, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
			Filepath:  &bookPath,
			LibraryID: &libraryID,
		})
		if err != nil && !errors.Is(err, errcodes.NotFound("Book")) {
			return errors.WithStack(err)
		}
	}

	if existingBook != nil {
		jobLog.Info("book already exists", logger.Data{"book_id": existingBook.ID})

		// Check to see if we need to update any of the metadata on the book.
		// Important: We only update metadata if:
		// 1. The new source has higher or equal priority (lower or equal number)
		// 2. The new value is non-empty (we always prefer having some data over no data)
		// 3. The new value differs from the existing value
		updateOptions := books.UpdateBookOptions{Columns: make([]string, 0)}
		metadataChanged := false

		// Update title only if the new title is non-empty and from a higher or equal priority source with different value
		if shouldUpdateScalar(strings.TrimSpace(title), existingBook.Title, titleSource, existingBook.TitleSource) {
			jobLog.Info("updating title", logger.Data{"new_title": title, "old_title": existingBook.Title})
			existingBook.Title = title
			existingBook.TitleSource = titleSource
			updateOptions.Columns = append(updateOptions.Columns, "title", "title_source")
			metadataChanged = true
		}

		// Update subtitle only if the new subtitle is non-empty and from a higher or equal priority source with different value
		existingSubtitleSource := ""
		if existingBook.SubtitleSource != nil {
			existingSubtitleSource = *existingBook.SubtitleSource
		}
		existingSubtitle := ""
		if existingBook.Subtitle != nil {
			existingSubtitle = *existingBook.Subtitle
		}
		newSubtitle := ""
		if subtitle != nil {
			newSubtitle = *subtitle
		}
		if shouldUpdateScalar(newSubtitle, existingSubtitle, subtitleSource, existingSubtitleSource) {
			jobLog.Info("updating subtitle", logger.Data{"new_subtitle": newSubtitle})
			existingBook.Subtitle = subtitle
			existingBook.SubtitleSource = &subtitleSource
			updateOptions.Columns = append(updateOptions.Columns, "subtitle", "subtitle_source")
			metadataChanged = true
		}

		// Update description only if the new description is non-empty and from a higher or equal priority source with different value
		existingDescriptionSource := ""
		if existingBook.DescriptionSource != nil {
			existingDescriptionSource = *existingBook.DescriptionSource
		}
		existingDescription := ""
		if existingBook.Description != nil {
			existingDescription = *existingBook.Description
		}
		newDescription := ""
		if description != nil {
			newDescription = *description
		}
		if shouldUpdateScalar(newDescription, existingDescription, descriptionSource, existingDescriptionSource) {
			jobLog.Info("updating description", nil)
			existingBook.Description = description
			existingBook.DescriptionSource = &descriptionSource
			updateOptions.Columns = append(updateOptions.Columns, "description", "description_source")
			metadataChanged = true
		}

		// Update authors only if we have authors and they're from a higher or equal priority source with different values
		existingAuthorNames := make([]string, len(existingBook.Authors))
		for i, a := range existingBook.Authors {
			if a.Person != nil {
				existingAuthorNames[i] = a.Person.Name
			}
		}
		newAuthorNames := make([]string, len(authors))
		for i, a := range authors {
			newAuthorNames[i] = a.Name
		}
		if shouldUpdateRelationship(newAuthorNames, existingAuthorNames, authorSource, existingBook.AuthorSource) {
			jobLog.Info("updating authors", logger.Data{"new_author_count": len(authors), "old_author_count": len(existingBook.Authors)})
			existingBook.AuthorSource = authorSource
			updateOptions.UpdateAuthors = true
			updateOptions.Authors = authors
			metadataChanged = true
		}
		// Update series if we have a higher or equal priority source with different values
		// Get existing series source for comparison
		var existingSeriesSource string
		if len(existingBook.BookSeries) > 0 && existingBook.BookSeries[0].Series != nil {
			existingSeriesSource = existingBook.BookSeries[0].Series.NameSource
		}
		existingSeriesNames := make([]string, len(existingBook.BookSeries))
		for i, bs := range existingBook.BookSeries {
			if bs.Series != nil {
				existingSeriesNames[i] = bs.Series.Name
			}
		}
		newSeriesNames := make([]string, len(seriesList))
		for i, s := range seriesList {
			newSeriesNames[i] = s.name
		}
		if shouldUpdateRelationship(newSeriesNames, existingSeriesNames, seriesSource, existingSeriesSource) {
			jobLog.Info("updating series", logger.Data{"new_series_count": len(seriesList), "old_series_count": len(existingBook.BookSeries)})
			// Delete existing book series entries
			err = w.bookService.DeleteBookSeries(ctx, existingBook.ID)
			if err != nil {
				return errors.WithStack(err)
			}
			// Create new book series entries for each series
			for i, s := range seriesList {
				seriesRecord, err := w.seriesService.FindOrCreateSeries(ctx, s.name, libraryID, seriesSource)
				if err != nil {
					jobLog.Error("failed to find/create series", nil, logger.Data{"series": s.name, "error": err.Error()})
					continue
				}
				sortOrder := s.sortOrder
				if sortOrder == 0 {
					sortOrder = i + 1
				}
				bookSeriesEntry := &models.BookSeries{
					BookID:       existingBook.ID,
					SeriesID:     seriesRecord.ID,
					SeriesNumber: s.number,
					SortOrder:    sortOrder,
				}
				err = w.bookService.CreateBookSeries(ctx, bookSeriesEntry)
				if err != nil {
					jobLog.Error("failed to create book series", nil, logger.Data{"book_id": existingBook.ID, "series_id": seriesRecord.ID, "error": err.Error()})
				}
			}
		}

		// Update genres if we have a higher or equal priority source with different values
		existingGenreSource := ""
		if existingBook.GenreSource != nil {
			existingGenreSource = *existingBook.GenreSource
		}
		existingGenreNames := make([]string, len(existingBook.BookGenres))
		for i, bg := range existingBook.BookGenres {
			if bg.Genre != nil {
				existingGenreNames[i] = bg.Genre.Name
			}
		}
		if shouldUpdateRelationship(genreNames, existingGenreNames, genreSource, existingGenreSource) {
			jobLog.Info("updating genres", logger.Data{"new_genre_count": len(genreNames), "old_genre_count": len(existingBook.BookGenres)})
			// Delete existing book genre entries
			err = w.bookService.DeleteBookGenres(ctx, existingBook.ID)
			if err != nil {
				return errors.WithStack(err)
			}
			// Create new book genre entries
			for _, genreName := range genreNames {
				genreRecord, err := w.genreService.FindOrCreateGenre(ctx, genreName, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create genre", nil, logger.Data{"genre": genreName, "error": err.Error()})
					continue
				}
				bookGenreEntry := &models.BookGenre{
					BookID:  existingBook.ID,
					GenreID: genreRecord.ID,
				}
				err = w.bookService.CreateBookGenre(ctx, bookGenreEntry)
				if err != nil {
					jobLog.Error("failed to create book genre", nil, logger.Data{"book_id": existingBook.ID, "genre_id": genreRecord.ID, "error": err.Error()})
				}
			}
			existingBook.GenreSource = &genreSource
			updateOptions.Columns = append(updateOptions.Columns, "genre_source")
		}

		// Update tags if we have a higher or equal priority source with different values
		existingTagSource := ""
		if existingBook.TagSource != nil {
			existingTagSource = *existingBook.TagSource
		}
		existingTagNames := make([]string, len(existingBook.BookTags))
		for i, bt := range existingBook.BookTags {
			if bt.Tag != nil {
				existingTagNames[i] = bt.Tag.Name
			}
		}
		if shouldUpdateRelationship(tagNames, existingTagNames, tagSource, existingTagSource) {
			jobLog.Info("updating tags", logger.Data{"new_tag_count": len(tagNames), "old_tag_count": len(existingBook.BookTags)})
			// Delete existing book tag entries
			err = w.bookService.DeleteBookTags(ctx, existingBook.ID)
			if err != nil {
				return errors.WithStack(err)
			}
			// Create new book tag entries
			for _, tagName := range tagNames {
				tagRecord, err := w.tagService.FindOrCreateTag(ctx, tagName, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create tag", nil, logger.Data{"tag": tagName, "error": err.Error()})
					continue
				}
				bookTagEntry := &models.BookTag{
					BookID: existingBook.ID,
					TagID:  tagRecord.ID,
				}
				err = w.bookService.CreateBookTag(ctx, bookTagEntry)
				if err != nil {
					jobLog.Error("failed to create book tag", nil, logger.Data{"book_id": existingBook.ID, "tag_id": tagRecord.ID, "error": err.Error()})
				}
			}
			existingBook.TagSource = &tagSource
			updateOptions.Columns = append(updateOptions.Columns, "tag_source")
		}

		err := w.bookService.UpdateBook(ctx, existingBook, updateOptions)
		if err != nil {
			return errors.WithStack(err)
		}

		// If authors were updated, create new Author entries
		// (UpdateBook deletes old authors, we need to create the new ones)
		if updateOptions.UpdateAuthors {
			for i, parsedAuthor := range updateOptions.Authors {
				person, err := w.personService.FindOrCreatePerson(ctx, parsedAuthor.Name, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create person", nil, logger.Data{"author": parsedAuthor.Name, "error": err.Error()})
					continue
				}
				var role *string
				if parsedAuthor.Role != "" {
					role = &parsedAuthor.Role
				}
				author := &models.Author{
					BookID:    existingBook.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
					Role:      role,
				}
				err = w.bookService.CreateAuthor(ctx, author)
				if err != nil {
					jobLog.Error("failed to create author", nil, logger.Data{"book_id": existingBook.ID, "person_id": person.ID, "error": err.Error()})
				}
			}
		}

		// Track for post-scan organization if this is a root-level file or metadata changed
		if library.OrganizeFileStructure && (needsOrganization || metadataChanged) {
			booksToOrganize[existingBook.ID] = struct{}{}
		}
	} else {
		jobLog.Info("creating book", logger.Data{"title": title})
		existingBook = &models.Book{
			LibraryID:    libraryID,
			Filepath:     bookPath,
			Title:        title,
			TitleSource:  titleSource,
			AuthorSource: authorSource,
			Subtitle:     subtitle,
			Description:  description,
		}
		// Set subtitle source only if we have a subtitle
		if subtitle != nil && *subtitle != "" {
			existingBook.SubtitleSource = &subtitleSource
		}
		// Set description source only if we have a description
		if description != nil && *description != "" {
			existingBook.DescriptionSource = &descriptionSource
		}
		err := w.bookService.CreateBook(ctx, existingBook)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create Author entries for each author
		for i, parsedAuthor := range authors {
			person, err := w.personService.FindOrCreatePerson(ctx, parsedAuthor.Name, libraryID)
			if err != nil {
				jobLog.Error("failed to find/create person", nil, logger.Data{"author": parsedAuthor.Name, "error": err.Error()})
				continue
			}
			var role *string
			if parsedAuthor.Role != "" {
				role = &parsedAuthor.Role
			}
			author := &models.Author{
				BookID:    existingBook.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
				Role:      role,
			}
			err = w.bookService.CreateAuthor(ctx, author)
			if err != nil {
				jobLog.Error("failed to create author", nil, logger.Data{"book_id": existingBook.ID, "person_id": person.ID, "error": err.Error()})
			}
		}

		// Create BookSeries entries for each series
		for i, s := range seriesList {
			seriesRecord, err := w.seriesService.FindOrCreateSeries(ctx, s.name, libraryID, seriesSource)
			if err != nil {
				jobLog.Error("failed to find/create series", nil, logger.Data{"series": s.name, "error": err.Error()})
				continue
			}
			sortOrder := s.sortOrder
			if sortOrder == 0 {
				sortOrder = i + 1
			}
			bookSeriesEntry := &models.BookSeries{
				BookID:       existingBook.ID,
				SeriesID:     seriesRecord.ID,
				SeriesNumber: s.number,
				SortOrder:    sortOrder,
			}
			err = w.bookService.CreateBookSeries(ctx, bookSeriesEntry)
			if err != nil {
				jobLog.Error("failed to create book series", nil, logger.Data{"book_id": existingBook.ID, "series_id": seriesRecord.ID, "error": err.Error()})
			}
		}

		// Create BookGenre entries for each genre
		if len(genreNames) > 0 {
			existingBook.GenreSource = &genreSource
			err = w.bookService.UpdateBook(ctx, existingBook, books.UpdateBookOptions{
				Columns: []string{"genre_source"},
			})
			if err != nil {
				jobLog.Error("failed to update book genre source", nil, logger.Data{"book_id": existingBook.ID, "error": err.Error()})
			}
			for _, genreName := range genreNames {
				genreRecord, err := w.genreService.FindOrCreateGenre(ctx, genreName, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create genre", nil, logger.Data{"genre": genreName, "error": err.Error()})
					continue
				}
				bookGenreEntry := &models.BookGenre{
					BookID:  existingBook.ID,
					GenreID: genreRecord.ID,
				}
				err = w.bookService.CreateBookGenre(ctx, bookGenreEntry)
				if err != nil {
					jobLog.Error("failed to create book genre", nil, logger.Data{"book_id": existingBook.ID, "genre_id": genreRecord.ID, "error": err.Error()})
				}
			}
		}

		// Create BookTag entries for each tag
		if len(tagNames) > 0 {
			existingBook.TagSource = &tagSource
			err = w.bookService.UpdateBook(ctx, existingBook, books.UpdateBookOptions{
				Columns: []string{"tag_source"},
			})
			if err != nil {
				jobLog.Error("failed to update book tag source", nil, logger.Data{"book_id": existingBook.ID, "error": err.Error()})
			}
			for _, tagName := range tagNames {
				tagRecord, err := w.tagService.FindOrCreateTag(ctx, tagName, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create tag", nil, logger.Data{"tag": tagName, "error": err.Error()})
					continue
				}
				bookTagEntry := &models.BookTag{
					BookID: existingBook.ID,
					TagID:  tagRecord.ID,
				}
				err = w.bookService.CreateBookTag(ctx, bookTagEntry)
				if err != nil {
					jobLog.Error("failed to create book tag", nil, logger.Data{"book_id": existingBook.ID, "tag_id": tagRecord.ID, "error": err.Error()})
				}
			}
		}

		// Track for post-scan organization if this is a root-level file
		// or a new directory-based book (to apply [Author] Title naming convention)
		if library.OrganizeFileStructure {
			booksToOrganize[existingBook.ID] = struct{}{}
		}
	}

	// Handle cover extraction before creating the file so we can set the cover source
	var coverImagePath *string
	if metadata != nil && len(metadata.CoverData) > 0 {
		// Save the cover image as a separate file using filename.cover.ext format
		// This includes the original file extension to avoid conflicts when files
		// with the same name but different extensions exist (e.g., mybook.epub and mybook.m4b)
		filename := filepath.Base(path)
		// For root-level files, bookPath is the file path itself, so use the directory
		// For directory-based files, bookPath is already the directory
		coverDir := bookPath
		if isRootLevelFile {
			coverDir = filepath.Dir(path)
		}
		coverBaseName := filename + ".cover"

		// Check if any cover already exists with this base name (regardless of extension)
		// This allows users to provide custom covers that won't be overwritten
		existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)
		if existingCoverPath == "" {
			// Normalize the cover image to strip problematic metadata (like gAMA without sRGB)
			// that can cause color rendering issues in browsers
			normalizedData, normalizedMime, _ := fileutils.NormalizeImage(metadata.CoverData, metadata.CoverMimeType)
			coverExt := ".png" // normalizeImage always outputs PNG
			if normalizedMime == metadata.CoverMimeType {
				// Normalization didn't change the format, use original extension
				coverExt = metadata.CoverExtension()
			}

			coverFilename := coverBaseName + coverExt
			coverFilepath := filepath.Join(coverDir, coverFilename)
			jobLog.Info("saving cover", logger.Data{"original_mime": metadata.CoverMimeType, "normalized_mime": normalizedMime})
			coverFile, err := os.Create(coverFilepath)
			if err != nil {
				return errors.WithStack(err)
			}
			defer coverFile.Close()
			_, err = io.Copy(coverFile, bytes.NewReader(normalizedData))
			if err != nil {
				return errors.WithStack(err)
			}
			// Update metadata with normalized values for the file record
			metadata.CoverData = normalizedData
			metadata.CoverMimeType = normalizedMime
			// Set cover source to the metadata source since we extracted it
			coverSource = &metadata.DataSource
			// Store the cover filename for the file record
			coverImagePath = &coverFilename
		} else {
			jobLog.Info("cover already exists, skipping extraction", logger.Data{"existing_cover": existingCoverPath})
			// Set cover source to existing cover since we're using the existing one
			existingCoverSource := models.DataSourceExistingCover
			coverSource = &existingCoverSource
			// Extract the filename from the existing cover path
			existingCoverFilename := filepath.Base(existingCoverPath)
			coverImagePath = &existingCoverFilename
		}
	}

	jobLog.Info("creating file", logger.Data{"filesize": size})
	file := &models.File{
		LibraryID:      libraryID,
		BookID:         existingBook.ID,
		Filepath:       path,
		FileType:       fileType,
		FilesizeBytes:  size,
		CoverImagePath: coverImagePath,
		CoverMimeType:  coverMimeType,
		CoverSource:    coverSource,
		CoverPage:      coverPage,
	}

	// Set audiobook-specific metadata for M4B files
	if metadata != nil && fileType == models.FileTypeM4B {
		if metadata.Duration > 0 {
			durationSeconds := metadata.Duration.Seconds()
			file.AudiobookDurationSeconds = &durationSeconds
		}
		if metadata.BitrateBps > 0 {
			bitrateBps := metadata.BitrateBps
			file.AudiobookBitrateBps = &bitrateBps
		}
	}

	// Set page count for CBZ files
	if metadata != nil && fileType == models.FileTypeCBZ {
		if metadata.PageCount != nil {
			file.PageCount = metadata.PageCount
		}
	}

	// Apply file sidecar data for narrators (higher priority than file metadata)
	if fileSidecarData != nil && len(fileSidecarData.Narrators) > 0 {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[narratorSource] {
			jobLog.Info("applying file sidecar data for narrators", logger.Data{"narrator_count": len(fileSidecarData.Narrators)})
			narratorSource = models.DataSourceSidecar
			narratorNames = make([]string, 0)
			for _, n := range fileSidecarData.Narrators {
				narratorNames = append(narratorNames, n.Name)
			}
		}
	}

	// Apply file sidecar data for identifiers (higher priority than file metadata)
	if fileSidecarData != nil && len(fileSidecarData.Identifiers) > 0 {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[identifierSource] {
			jobLog.Info("applying file sidecar data for identifiers", logger.Data{"identifier_count": len(fileSidecarData.Identifiers)})
			identifierSource = models.DataSourceSidecar
			identifiers = make([]mediafile.ParsedIdentifier, 0)
			for _, id := range fileSidecarData.Identifiers {
				identifiers = append(identifiers, mediafile.ParsedIdentifier{
					Type:  id.Type,
					Value: id.Value,
				})
			}
		}
	}

	// Apply file sidecar data for URL (higher priority than file metadata)
	if fileSidecarData != nil && fileSidecarData.URL != nil && *fileSidecarData.URL != "" {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[fileURLSource] {
			fileURL = fileSidecarData.URL
			fileURLSource = models.DataSourceSidecar
		}
	}

	// Apply file sidecar data for publisher (higher priority than file metadata)
	if fileSidecarData != nil && fileSidecarData.Publisher != nil && *fileSidecarData.Publisher != "" {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[publisherSource] {
			publisherName = fileSidecarData.Publisher
			publisherSource = models.DataSourceSidecar
		}
	}

	// Apply file sidecar data for imprint (higher priority than file metadata)
	if fileSidecarData != nil && fileSidecarData.Imprint != nil && *fileSidecarData.Imprint != "" {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[imprintSource] {
			imprintName = fileSidecarData.Imprint
			imprintSource = models.DataSourceSidecar
		}
	}

	// Apply file sidecar data for release date (higher priority than file metadata)
	if fileSidecarData != nil && fileSidecarData.ReleaseDate != nil && *fileSidecarData.ReleaseDate != "" {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[releaseDateSource] {
			// Parse the ISO 8601 date string from sidecar
			if t, err := time.Parse("2006-01-02", *fileSidecarData.ReleaseDate); err == nil {
				releaseDate = &t
				releaseDateSource = models.DataSourceSidecar
			}
		}
	}

	// Apply file sidecar data for name (higher priority than file metadata)
	if fileSidecarData != nil && fileSidecarData.Name != nil && *fileSidecarData.Name != "" {
		if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[fileNameSource] {
			jobLog.Info("applying file sidecar data for name", logger.Data{"name": *fileSidecarData.Name})
			fileName = fileSidecarData.Name
			fileNameSource = models.DataSourceSidecar
		}
	}

	// Set narrator source on file if we have narrators
	if len(narratorNames) > 0 {
		file.NarratorSource = &narratorSource
	}

	// Set identifier source on file if we have identifiers
	if len(identifiers) > 0 {
		file.IdentifierSource = &identifierSource
	}

	// Set file-level metadata
	if fileURL != nil {
		file.URL = fileURL
		file.URLSource = &fileURLSource
	}
	if releaseDate != nil {
		file.ReleaseDate = releaseDate
		file.ReleaseDateSource = &releaseDateSource
	}
	if fileName != nil {
		file.Name = fileName
		file.NameSource = &fileNameSource
	}

	// Create publisher entity if we have a publisher name
	if publisherName != nil && *publisherName != "" {
		publisher, err := w.publisherService.FindOrCreatePublisher(ctx, *publisherName, libraryID)
		if err != nil {
			jobLog.Error("failed to find/create publisher", nil, logger.Data{"publisher": *publisherName, "error": err.Error()})
		} else {
			file.PublisherID = &publisher.ID
			file.PublisherSource = &publisherSource
		}
	}

	// Create imprint entity if we have an imprint name
	if imprintName != nil && *imprintName != "" {
		imprint, err := w.imprintService.FindOrCreateImprint(ctx, *imprintName, libraryID)
		if err != nil {
			jobLog.Error("failed to find/create imprint", nil, logger.Data{"imprint": *imprintName, "error": err.Error()})
		} else {
			file.ImprintID = &imprint.ID
			file.ImprintSource = &imprintSource
		}
	}

	// For existing files, update metadata and return (don't create duplicate)
	if existingFile != nil {
		// Reload existing file with full relations for metadata comparison
		existingFile, err = w.bookService.RetrieveFileWithRelations(ctx, existingFile.ID)
		if err != nil {
			return errors.WithStack(err)
		}

		fileUpdated := false
		fileUpdateColumns := make([]string, 0)

		// Update narrators
		existingNarratorSource := ""
		if existingFile.NarratorSource != nil {
			existingNarratorSource = *existingFile.NarratorSource
		}
		existingNarratorNames := make([]string, len(existingFile.Narrators))
		for i, n := range existingFile.Narrators {
			if n.Person != nil {
				existingNarratorNames[i] = n.Person.Name
			}
		}
		if shouldUpdateRelationship(narratorNames, existingNarratorNames, narratorSource, existingNarratorSource) {
			jobLog.Info("updating narrators", logger.Data{"new_count": len(narratorNames), "old_count": len(existingNarratorNames)})
			// Delete existing narrators
			if err := w.bookService.DeleteNarrators(ctx, existingFile.ID); err != nil {
				return errors.WithStack(err)
			}
			// Create new narrators
			for i, name := range narratorNames {
				person, err := w.personService.FindOrCreatePerson(ctx, name, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create narrator person", nil, logger.Data{"narrator": name, "error": err.Error()})
					continue
				}
				narrator := &models.Narrator{
					FileID:    existingFile.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
				}
				if err := w.bookService.CreateNarrator(ctx, narrator); err != nil {
					jobLog.Error("failed to create narrator", nil, logger.Data{"file_id": existingFile.ID, "person_id": person.ID, "error": err.Error()})
				}
			}
			existingFile.NarratorSource = &narratorSource
			fileUpdateColumns = append(fileUpdateColumns, "narrator_source")
			fileUpdated = true
		}

		// Update identifiers
		existingIdentifierSource := ""
		if existingFile.IdentifierSource != nil {
			existingIdentifierSource = *existingFile.IdentifierSource
		}
		existingIdentifierValues := make([]string, len(existingFile.Identifiers))
		for i, id := range existingFile.Identifiers {
			existingIdentifierValues[i] = id.Type + ":" + id.Value
		}
		newIdentifierValues := make([]string, len(identifiers))
		for i, id := range identifiers {
			newIdentifierValues[i] = id.Type + ":" + id.Value
		}
		if shouldUpdateRelationship(newIdentifierValues, existingIdentifierValues, identifierSource, existingIdentifierSource) {
			jobLog.Info("updating identifiers", logger.Data{"new_count": len(identifiers), "old_count": len(existingFile.Identifiers)})
			// Delete existing identifiers
			if err := w.bookService.DeleteFileIdentifiers(ctx, existingFile.ID); err != nil {
				return errors.WithStack(err)
			}
			// Create new identifiers
			for _, id := range identifiers {
				fileIdentifier := &models.FileIdentifier{
					FileID: existingFile.ID,
					Type:   id.Type,
					Value:  id.Value,
					Source: identifierSource,
				}
				if err := w.bookService.CreateFileIdentifier(ctx, fileIdentifier); err != nil {
					jobLog.Error("failed to create identifier", nil, logger.Data{"file_id": existingFile.ID, "type": id.Type, "error": err.Error()})
				}
			}
			existingFile.IdentifierSource = &identifierSource
			fileUpdateColumns = append(fileUpdateColumns, "identifier_source")
			fileUpdated = true
		}

		// Update URL
		existingURLSource := ""
		if existingFile.URLSource != nil {
			existingURLSource = *existingFile.URLSource
		}
		existingURL := ""
		if existingFile.URL != nil {
			existingURL = *existingFile.URL
		}
		newURL := ""
		if fileURL != nil {
			newURL = *fileURL
		}
		if shouldUpdateScalar(newURL, existingURL, fileURLSource, existingURLSource) {
			jobLog.Info("updating URL", logger.Data{"new_url": newURL})
			existingFile.URL = fileURL
			existingFile.URLSource = &fileURLSource
			fileUpdateColumns = append(fileUpdateColumns, "url", "url_source")
			fileUpdated = true
		}

		// Update publisher
		existingPublisherSource := ""
		if existingFile.PublisherSource != nil {
			existingPublisherSource = *existingFile.PublisherSource
		}
		existingPublisherName := ""
		if existingFile.Publisher != nil {
			existingPublisherName = existingFile.Publisher.Name
		}
		newPublisherName := ""
		if publisherName != nil {
			newPublisherName = *publisherName
		}
		if shouldUpdateScalar(newPublisherName, existingPublisherName, publisherSource, existingPublisherSource) {
			jobLog.Info("updating publisher", logger.Data{"new_publisher": newPublisherName})
			if newPublisherName != "" {
				publisher, err := w.publisherService.FindOrCreatePublisher(ctx, newPublisherName, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create publisher", nil, logger.Data{"publisher": newPublisherName, "error": err.Error()})
				} else {
					existingFile.PublisherID = &publisher.ID
					existingFile.PublisherSource = &publisherSource
					fileUpdateColumns = append(fileUpdateColumns, "publisher_id", "publisher_source")
					fileUpdated = true
				}
			} else {
				existingFile.PublisherID = nil
				existingFile.PublisherSource = &publisherSource
				fileUpdateColumns = append(fileUpdateColumns, "publisher_id", "publisher_source")
				fileUpdated = true
			}
		}

		// Update imprint
		existingImprintSource := ""
		if existingFile.ImprintSource != nil {
			existingImprintSource = *existingFile.ImprintSource
		}
		existingImprintName := ""
		if existingFile.Imprint != nil {
			existingImprintName = existingFile.Imprint.Name
		}
		newImprintName := ""
		if imprintName != nil {
			newImprintName = *imprintName
		}
		if shouldUpdateScalar(newImprintName, existingImprintName, imprintSource, existingImprintSource) {
			jobLog.Info("updating imprint", logger.Data{"new_imprint": newImprintName})
			if newImprintName != "" {
				imprint, err := w.imprintService.FindOrCreateImprint(ctx, newImprintName, libraryID)
				if err != nil {
					jobLog.Error("failed to find/create imprint", nil, logger.Data{"imprint": newImprintName, "error": err.Error()})
				} else {
					existingFile.ImprintID = &imprint.ID
					existingFile.ImprintSource = &imprintSource
					fileUpdateColumns = append(fileUpdateColumns, "imprint_id", "imprint_source")
					fileUpdated = true
				}
			} else {
				existingFile.ImprintID = nil
				existingFile.ImprintSource = &imprintSource
				fileUpdateColumns = append(fileUpdateColumns, "imprint_id", "imprint_source")
				fileUpdated = true
			}
		}

		// Update release date
		existingReleaseDateSource := ""
		if existingFile.ReleaseDateSource != nil {
			existingReleaseDateSource = *existingFile.ReleaseDateSource
		}
		existingReleaseDate := ""
		if existingFile.ReleaseDate != nil {
			existingReleaseDate = existingFile.ReleaseDate.Format(time.RFC3339)
		}
		newReleaseDate := ""
		if releaseDate != nil {
			newReleaseDate = releaseDate.Format(time.RFC3339)
		}
		if shouldUpdateScalar(newReleaseDate, existingReleaseDate, releaseDateSource, existingReleaseDateSource) {
			jobLog.Info("updating release date", logger.Data{"new_release_date": newReleaseDate})
			existingFile.ReleaseDate = releaseDate
			existingFile.ReleaseDateSource = &releaseDateSource
			fileUpdateColumns = append(fileUpdateColumns, "release_date", "release_date_source")
			fileUpdated = true
		}

		// Update name
		existingNameSource := ""
		if existingFile.NameSource != nil {
			existingNameSource = *existingFile.NameSource
		}
		existingName := ""
		if existingFile.Name != nil {
			existingName = *existingFile.Name
		}
		newName := ""
		if fileName != nil {
			newName = *fileName
		}
		if shouldUpdateScalar(newName, existingName, fileNameSource, existingNameSource) {
			jobLog.Info("updating name", logger.Data{"from": existingName, "to": newName})
			existingFile.Name = fileName
			existingFile.NameSource = &fileNameSource
			fileUpdateColumns = append(fileUpdateColumns, "name", "name_source")
			fileUpdated = true
		}

		// Update file size if changed (no source tracking, always update if different)
		if existingFile.FilesizeBytes != size {
			jobLog.Info("updating file size", logger.Data{"new_size": size, "old_size": existingFile.FilesizeBytes})
			existingFile.FilesizeBytes = size
			fileUpdateColumns = append(fileUpdateColumns, "filesize_bytes")
			fileUpdated = true
		}

		// Apply file updates if any changes
		if fileUpdated {
			if err := w.bookService.UpdateFile(ctx, existingFile, books.UpdateFileOptions{Columns: fileUpdateColumns}); err != nil {
				return errors.WithStack(err)
			}
			jobLog.Info("file metadata updated", logger.Data{"columns": fileUpdateColumns})
		} else {
			jobLog.Info("no file metadata changes detected", nil)
		}

		return nil
	}

	err = w.bookService.CreateFile(ctx, file)
	if err != nil {
		return errors.WithStack(err)
	}

	// Create Narrator entries for each narrator
	for i, narratorName := range narratorNames {
		person, err := w.personService.FindOrCreatePerson(ctx, narratorName, libraryID)
		if err != nil {
			jobLog.Error("failed to find/create person for narrator", nil, logger.Data{"narrator": narratorName, "error": err.Error()})
			continue
		}
		narrator := &models.Narrator{
			FileID:    file.ID,
			PersonID:  person.ID,
			SortOrder: i + 1,
		}
		err = w.bookService.CreateNarrator(ctx, narrator)
		if err != nil {
			jobLog.Error("failed to create narrator", nil, logger.Data{"file_id": file.ID, "person_id": person.ID, "error": err.Error()})
		}
	}

	// Create FileIdentifier entries for each identifier
	for _, parsedID := range identifiers {
		fileID := &models.FileIdentifier{
			FileID: file.ID,
			Type:   parsedID.Type,
			Value:  parsedID.Value,
			Source: identifierSource,
		}
		err = w.bookService.CreateFileIdentifier(ctx, fileID)
		if err != nil {
			jobLog.Error("failed to create file identifier", nil, logger.Data{"file_id": file.ID, "type": parsedID.Type, "error": err.Error()})
		}
	}

	// Write sidecar files to keep them in sync with the database
	// Reload the book with all relations to get complete data for the sidecar
	existingBook, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
		ID: &existingBook.ID,
	})
	if err != nil {
		jobLog.Warn("failed to reload book for sidecar", logger.Data{"error": err.Error()})
	} else {
		if err := sidecar.WriteBookSidecarFromModel(existingBook); err != nil {
			jobLog.Warn("failed to write book sidecar", logger.Data{"error": err.Error()})
		}

		// Write file sidecar using the reloaded file from the book (has all relations including identifiers)
		for _, reloadedFile := range existingBook.Files {
			if reloadedFile.ID == file.ID {
				if err := sidecar.WriteFileSidecarFromModel(reloadedFile); err != nil {
					jobLog.Warn("failed to write file sidecar", logger.Data{"error": err.Error()})
				}
				break
			}
		}
	}

	// Discover and create supplement files
	if !isRootLevelFile {
		// Directory-based book: scan directory for supplements
		supplements, err := discoverSupplements(bookPath, w.config.SupplementExcludePatterns)
		if err != nil {
			jobLog.Warn("failed to discover supplements", logger.Data{"error": err.Error()})
		} else {
			for _, suppPath := range supplements {
				// Check if supplement already exists
				existingSupp, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
					Filepath:  &suppPath,
					LibraryID: &libraryID,
				})
				if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
					jobLog.Warn("error checking supplement", logger.Data{"path": suppPath, "error": err.Error()})
					continue
				}
				if existingSupp != nil {
					continue // Already exists
				}

				// Get file info
				suppStat, err := os.Stat(suppPath)
				if err != nil {
					jobLog.Warn("can't stat supplement", logger.Data{"path": suppPath, "error": err.Error()})
					continue
				}

				suppExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(suppPath)), ".")
				suppFile := &models.File{
					LibraryID:     libraryID,
					BookID:        existingBook.ID,
					Filepath:      suppPath,
					FileType:      suppExt,
					FileRole:      models.FileRoleSupplement,
					FilesizeBytes: suppStat.Size(),
				}

				if err := w.bookService.CreateFile(ctx, suppFile); err != nil {
					jobLog.Warn("failed to create supplement", logger.Data{"path": suppPath, "error": err.Error()})
					continue
				}
				jobLog.Info("created supplement file", logger.Data{"path": suppPath, "file_id": suppFile.ID})
			}
		}
	} else {
		// Root-level book: find supplements by basename matching
		for _, libraryPath := range library.LibraryPaths {
			if filepath.Dir(path) == libraryPath.Filepath {
				supplements, err := discoverRootLevelSupplements(path, libraryPath.Filepath, w.config.SupplementExcludePatterns)
				if err != nil {
					jobLog.Warn("failed to discover root supplements", logger.Data{"error": err.Error()})
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
						BookID:        existingBook.ID,
						Filepath:      suppPath,
						FileType:      suppExt,
						FileRole:      models.FileRoleSupplement,
						FilesizeBytes: suppStat.Size(),
					}

					if err := w.bookService.CreateFile(ctx, suppFile); err != nil {
						continue
					}
					jobLog.Info("created root-level supplement", logger.Data{"path": suppPath, "file_id": suppFile.ID})
				}
				break
			}
		}
	}

	return nil
}

// recoverMissingCover checks if a file's cover is missing on disk and re-extracts it if needed.
func (w *Worker) recoverMissingCover(ctx context.Context, file *models.File) error {
	log := logger.FromContext(ctx).Data(logger.Data{"file_id": file.ID, "filepath": file.Filepath})

	// If file has no cover mime type, nothing to recover
	if file.CoverMimeType == nil {
		return nil
	}

	// Determine cover directory
	var coverDir string
	if file.Book != nil {
		// Check if book filepath is a directory or file
		if info, err := os.Stat(file.Book.Filepath); err == nil && info.IsDir() {
			coverDir = file.Book.Filepath
		} else {
			coverDir = filepath.Dir(file.Book.Filepath)
		}
	} else {
		coverDir = filepath.Dir(file.Filepath)
	}

	// Check if cover file exists
	filename := filepath.Base(file.Filepath)
	coverBaseName := filename + ".cover"
	existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)

	if existingCoverPath != "" {
		// Cover exists, nothing to do
		return nil
	}

	log.Info("cover file missing, re-extracting")

	// Re-extract cover from the media file
	var metadata *mediafile.ParsedMetadata
	var parseErr error

	switch file.FileType {
	case models.FileTypeM4B:
		metadata, parseErr = mp4.Parse(file.Filepath)
	case models.FileTypeEPUB:
		metadata, parseErr = epub.Parse(file.Filepath)
	case models.FileTypeCBZ:
		metadata, parseErr = cbz.Parse(file.Filepath)
	default:
		return nil // Unknown file type, skip
	}

	if parseErr != nil {
		return errors.WithStack(parseErr)
	}

	if metadata == nil || len(metadata.CoverData) == 0 {
		log.Info("no cover data in media file")
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

	log.Info("recovered missing cover", logger.Data{"cover_path": coverFilepath})

	// Update file's cover mime type if it changed due to normalization
	if normalizedMime != *file.CoverMimeType {
		file.CoverMimeType = &normalizedMime
		if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
			Columns: []string{"cover_mime_type"},
		}); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
