package worker

import (
	"archive/zip"
	"bytes"
	"context"
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
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
)

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
	ForceRefresh bool // Bypass priority checks, overwrite all metadata

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
// The public Scan method wraps this to implement books.Scanner.
func (w *Worker) scanInternal(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
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
		return w.scanFileByPath(ctx, opts)
	case opts.FileID != 0:
		return w.scanFileByID(ctx, opts)
	case opts.BookID != 0:
		return w.scanBook(ctx, opts)
	default:
		// This should never happen due to validation above
		return nil, ErrInvalidScanOptions
	}
}

// scanFileByPath handles batch scan mode - discovering or creating file/book records by path.
// If the file already exists in DB, delegates to scanFileByID.
// If the file doesn't exist on disk, returns nil (skip silently).
// If the file exists on disk but not in DB, creates a new file/book record.
func (w *Worker) scanFileByPath(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	// Validate LibraryID is required for path-based scan
	if opts.LibraryID == 0 {
		return nil, errors.New("LibraryID required for FilePath mode")
	}

	// Check if file already exists in DB
	existingFile, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
		Filepath:  &opts.FilePath,
		LibraryID: &opts.LibraryID,
	})
	if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
		return nil, errors.Wrap(err, "failed to check if file exists")
	}

	// If file exists in DB, delegate to scanFileByID
	if existingFile != nil {
		return w.scanFileByID(ctx, ScanOptions{
			FileID:       existingFile.ID,
			ForceRefresh: opts.ForceRefresh,
			JobLog:       opts.JobLog,
		})
	}

	// File doesn't exist in DB - check if it exists on disk
	_, err = os.Stat(opts.FilePath)
	if os.IsNotExist(err) {
		// File doesn't exist on disk - skip silently
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}

	// File exists on disk but not in DB - parse metadata and create new record
	return w.scanFileCreateNew(ctx, opts)
}

// scanFileByID handles single file resync - file already exists in DB.
// If the file no longer exists on disk, deletes the file record (and book if it was the last file).
func (w *Worker) scanFileByID(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	// Retrieve file with relations from DB
	file, err := w.bookService.RetrieveFileWithRelations(ctx, opts.FileID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve file")
	}

	// Check if file exists on disk
	_, err = os.Stat(file.Filepath)
	if os.IsNotExist(err) {
		log.Info("file no longer exists on disk, deleting record", logger.Data{"file_id": file.ID, "path": file.Filepath})

		// Get parent book to check file count
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve parent book")
		}

		bookDeleted := len(book.Files) == 1
		fileDir := filepath.Dir(file.Filepath)
		bookPath := book.Filepath

		// Delete the file
		if err := w.bookService.DeleteFile(ctx, file.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete file record")
		}

		// If last file, delete the book too
		if bookDeleted {
			// Delete from search index before deleting the book
			if w.searchService != nil {
				if err := w.searchService.DeleteFromBookIndex(ctx, book.ID); err != nil {
					log.Warn("failed to delete book from search index", logger.Data{"book_id": book.ID, "error": err.Error()})
				}
			}
			if err := w.bookService.DeleteBook(ctx, book.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete orphaned book")
			}
			log.Info("deleted orphaned book", logger.Data{"book_id": book.ID})

			// Clean up empty directories up to library path
			library, libErr := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
				ID: &book.LibraryID,
			})
			if libErr == nil && library != nil {
				// Find which library path contains this book
				for _, libPath := range library.LibraryPaths {
					if strings.HasPrefix(bookPath, libPath.Filepath) {
						if err := fileutils.CleanupEmptyParentDirectories(bookPath, libPath.Filepath); err != nil {
							log.Warn("failed to cleanup empty directories", logger.Data{"path": bookPath, "error": err.Error()})
						}
						break
					}
				}
			}
		} else if fileDir != bookPath {
			// Clean up empty directories up to book folder
			if err := fileutils.CleanupEmptyParentDirectories(fileDir, bookPath); err != nil {
				log.Warn("failed to cleanup empty directories", logger.Data{"path": fileDir, "error": err.Error()})
			}
		}

		return &ScanResult{
			FileDeleted: true,
			BookDeleted: bookDeleted,
		}, nil
	}

	// If stat returned an error other than NotExist, return it
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}

	// Check and recover missing cover if needed
	if err := w.recoverMissingCover(ctx, file); err != nil {
		log.Warn("failed to recover missing cover", logger.Data{"file_id": file.ID, "error": err.Error()})
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

	// Run metadata enrichers after parsing
	metadata = w.runMetadataEnrichers(ctx, metadata, file, book, file.LibraryID)

	// Use scanFileCore for all metadata updates, sidecars, and search index
	// This is a resync (FileID mode), so pass isResync=true to enable book organization
	return w.scanFileCore(ctx, file, book, metadata, opts.ForceRefresh, true)
}

// scanBook handles book resync - scan all files belonging to the book.
// It loops through all files in the book, calling scanFileByID for each.
// If the book has no files, it deletes the book from the database.
// Errors from individual file scans are logged and skipped (don't fail entire book scan).
func (w *Worker) scanBook(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	// Fetch book with files from DB
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &opts.BookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve book")
	}

	// If book has no files, delete it
	if len(book.Files) == 0 {
		log.Info("book has no files, deleting", logger.Data{"book_id": book.ID})
		bookPath := book.Filepath

		// Delete from search index before deleting the book
		if w.searchService != nil {
			if err := w.searchService.DeleteFromBookIndex(ctx, book.ID); err != nil {
				log.Warn("failed to delete book from search index", logger.Data{"book_id": book.ID, "error": err.Error()})
			}
		}

		// Delete book
		if err := w.bookService.DeleteBook(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete empty book")
		}

		// Clean up empty directories up to library path
		library, libErr := w.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
			ID: &book.LibraryID,
		})
		if libErr == nil && library != nil {
			// Find which library path contains this book
			for _, libPath := range library.LibraryPaths {
				if strings.HasPrefix(bookPath, libPath.Filepath) {
					if err := fileutils.CleanupEmptyParentDirectories(bookPath, libPath.Filepath); err != nil {
						log.Warn("failed to cleanup empty directories", logger.Data{"path": bookPath, "error": err.Error()})
					}
					break
				}
			}
		}

		return &ScanResult{BookDeleted: true}, nil
	}

	// Initialize file results
	fileResults := make([]*ScanResult, 0, len(book.Files))

	// Loop through files and scan each
	for _, file := range book.Files {
		fileResult, err := w.scanFileByID(ctx, ScanOptions{
			FileID:       file.ID,
			ForceRefresh: opts.ForceRefresh,
			JobLog:       opts.JobLog,
		})
		if err != nil {
			log.Warn("failed to scan file in book, continuing", logger.Data{
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
//
// Returns a ScanResult with the updated file and book records.
func (w *Worker) scanFileCore(
	ctx context.Context,
	file *models.File,
	book *models.Book,
	metadata *mediafile.ParsedMetadata,
	forceRefresh bool,
	isResync bool,
) (*ScanResult, error) {
	log := logger.FromContext(ctx)

	// If no metadata, nothing to update
	if metadata == nil {
		return &ScanResult{File: file, Book: book}, nil
	}

	sidecarSource := models.DataSourceSidecar

	// Read sidecar files if they exist (higher priority than file metadata)
	// Sidecars can override file metadata but not manual user edits
	bookSidecarData, err := sidecar.ReadBookSidecar(book.Filepath)
	if err != nil {
		log.Warn("failed to read book sidecar", logger.Data{"error": err.Error()})
	}
	fileSidecarData, err := sidecar.ReadFileSidecar(file.Filepath)
	if err != nil {
		log.Warn("failed to read file sidecar", logger.Data{"error": err.Error()})
	}

	bookUpdateOpts := books.UpdateBookOptions{Columns: []string{}}
	bookTitleChanged := false
	authorsChanged := false

	// Supplements should not update book-level metadata (title, authors, series, etc.)
	// They only update file-level metadata (name, URL, narrators, etc.)
	isMainFile := file.FileRole != models.FileRoleSupplement

	// Book-level updates: only for main files, not supplements
	if isMainFile {
		// Title (from metadata)
		title := strings.TrimSpace(metadata.Title)
		// Normalize volume indicators (e.g., "#007" -> "v7") for CBZ files
		if normalizedTitle, hasVolume := fileutils.NormalizeVolumeInTitle(title, file.FileType); hasVolume {
			title = normalizedTitle
		}
		titleSource := metadata.SourceForField("title")
		if shouldUpdateScalar(title, book.Title, titleSource, book.TitleSource, forceRefresh) {
			log.Info("updating book title", logger.Data{"from": book.Title, "to": title})
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
				log.Info("updating book title from sidecar", logger.Data{"from": book.Title, "to": bookSidecarData.Title})
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
				log.Info("updating book subtitle", logger.Data{"from": existingSubtitle, "to": subtitle})
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
				log.Info("updating book subtitle from sidecar", logger.Data{"from": existingSubtitle, "to": *bookSidecarData.Subtitle})
				book.Subtitle = bookSidecarData.Subtitle
				book.SubtitleSource = &sidecarSource
				bookUpdateOpts.Columns = appendIfMissing(bookUpdateOpts.Columns, "subtitle", "subtitle_source")
			}
		}

		// Description (from metadata)
		description := strings.TrimSpace(metadata.Description)
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
				log.Info("updating book description", nil)
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
				log.Info("updating book description from sidecar", nil)
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
				log.Info("updating authors", logger.Data{"new_count": len(metadata.Authors), "old_count": len(book.Authors)})

				// Delete existing authors
				if err := w.bookService.DeleteAuthors(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing authors")
				}

				// Create new authors
				for i, parsedAuthor := range metadata.Authors {
					person, err := w.personService.FindOrCreatePerson(ctx, parsedAuthor.Name, book.LibraryID)
					if err != nil {
						log.Warn("failed to find/create person for author", logger.Data{"name": parsedAuthor.Name, "error": err.Error()})
						continue
					}
					var role *string
					if parsedAuthor.Role != "" {
						role = &parsedAuthor.Role
					}
					author := &models.Author{
						BookID:    book.ID,
						PersonID:  person.ID,
						Role:      role,
						SortOrder: i + 1,
					}
					if err := w.bookService.CreateAuthor(ctx, author); err != nil {
						log.Warn("failed to create author", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating authors from sidecar", logger.Data{"new_count": len(bookSidecarData.Authors), "old_count": len(book.Authors)})

				// Delete existing authors
				if err := w.bookService.DeleteAuthors(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing authors")
				}

				// Create new authors from sidecar
				for i, sidecarAuthor := range bookSidecarData.Authors {
					person, err := w.personService.FindOrCreatePerson(ctx, sidecarAuthor.Name, book.LibraryID)
					if err != nil {
						log.Warn("failed to find/create person for author", logger.Data{"name": sidecarAuthor.Name, "error": err.Error()})
						continue
					}
					author := &models.Author{
						BookID:    book.ID,
						PersonID:  person.ID,
						Role:      sidecarAuthor.Role,
						SortOrder: i + 1,
					}
					if err := w.bookService.CreateAuthor(ctx, author); err != nil {
						log.Warn("failed to create author", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating series", logger.Data{"new_count": 1, "old_count": len(book.BookSeries)})

				// Delete existing series
				if err := w.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing series")
				}

				// Create new series
				seriesRecord, err := w.seriesService.FindOrCreateSeries(ctx, metadata.Series, book.LibraryID, seriesSource)
				if err != nil {
					log.Warn("failed to find/create series", logger.Data{"name": metadata.Series, "error": err.Error()})
				} else {
					bookSeries := &models.BookSeries{
						BookID:       book.ID,
						SeriesID:     seriesRecord.ID,
						SeriesNumber: metadata.SeriesNumber,
						SortOrder:    1,
					}
					if err := w.bookService.CreateBookSeries(ctx, bookSeries); err != nil {
						log.Warn("failed to create book series", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating series from sidecar", logger.Data{"new_count": len(bookSidecarData.Series), "old_count": len(book.BookSeries)})

				// Delete existing series
				if err := w.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing series")
				}

				// Create new series from sidecar
				for i, sidecarSeries := range bookSidecarData.Series {
					if sidecarSeries.Name == "" {
						continue
					}
					seriesRecord, err := w.seriesService.FindOrCreateSeries(ctx, sidecarSeries.Name, book.LibraryID, sidecarSource)
					if err != nil {
						log.Warn("failed to find/create series", logger.Data{"name": sidecarSeries.Name, "error": err.Error()})
						continue
					}
					bookSeries := &models.BookSeries{
						BookID:       book.ID,
						SeriesID:     seriesRecord.ID,
						SeriesNumber: sidecarSeries.Number,
						SortOrder:    i + 1,
					}
					if err := w.bookService.CreateBookSeries(ctx, bookSeries); err != nil {
						log.Warn("failed to create book series", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating genres", logger.Data{"new_count": len(metadata.Genres), "old_count": len(book.BookGenres)})

				// Delete existing genres
				if err := w.bookService.DeleteBookGenres(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing genres")
				}

				// Create new genres
				for _, genreName := range metadata.Genres {
					genreRecord, err := w.genreService.FindOrCreateGenre(ctx, genreName, book.LibraryID)
					if err != nil {
						log.Warn("failed to find/create genre", logger.Data{"name": genreName, "error": err.Error()})
						continue
					}
					bookGenre := &models.BookGenre{
						BookID:  book.ID,
						GenreID: genreRecord.ID,
					}
					if err := w.bookService.CreateBookGenre(ctx, bookGenre); err != nil {
						log.Warn("failed to create book genre", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating genres from sidecar", logger.Data{"new_count": len(bookSidecarData.Genres), "old_count": len(book.BookGenres)})

				// Delete existing genres
				if err := w.bookService.DeleteBookGenres(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing genres")
				}

				// Create new genres from sidecar
				for _, genreName := range bookSidecarData.Genres {
					genreRecord, err := w.genreService.FindOrCreateGenre(ctx, genreName, book.LibraryID)
					if err != nil {
						log.Warn("failed to find/create genre", logger.Data{"name": genreName, "error": err.Error()})
						continue
					}
					bookGenre := &models.BookGenre{
						BookID:  book.ID,
						GenreID: genreRecord.ID,
					}
					if err := w.bookService.CreateBookGenre(ctx, bookGenre); err != nil {
						log.Warn("failed to create book genre", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating tags", logger.Data{"new_count": len(metadata.Tags), "old_count": len(book.BookTags)})

				// Delete existing tags
				if err := w.bookService.DeleteBookTags(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing tags")
				}

				// Create new tags
				for _, tagName := range metadata.Tags {
					tagRecord, err := w.tagService.FindOrCreateTag(ctx, tagName, book.LibraryID)
					if err != nil {
						log.Warn("failed to find/create tag", logger.Data{"name": tagName, "error": err.Error()})
						continue
					}
					bookTag := &models.BookTag{
						BookID: book.ID,
						TagID:  tagRecord.ID,
					}
					if err := w.bookService.CreateBookTag(ctx, bookTag); err != nil {
						log.Warn("failed to create book tag", logger.Data{"error": err.Error()})
					}
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
				log.Info("updating tags from sidecar", logger.Data{"new_count": len(bookSidecarData.Tags), "old_count": len(book.BookTags)})

				// Delete existing tags
				if err := w.bookService.DeleteBookTags(ctx, book.ID); err != nil {
					return nil, errors.Wrap(err, "failed to delete existing tags")
				}

				// Create new tags from sidecar
				for _, tagName := range bookSidecarData.Tags {
					tagRecord, err := w.tagService.FindOrCreateTag(ctx, tagName, book.LibraryID)
					if err != nil {
						log.Warn("failed to find/create tag", logger.Data{"name": tagName, "error": err.Error()})
						continue
					}
					bookTag := &models.BookTag{
						BookID: book.ID,
						TagID:  tagRecord.ID,
					}
					if err := w.bookService.CreateBookTag(ctx, bookTag); err != nil {
						log.Warn("failed to create book tag", logger.Data{"error": err.Error()})
					}
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
				log.Warn("failed to reload book for organization", logger.Data{"error": err.Error()})
			} else {
				// Call UpdateBook with OrganizeFiles flag to trigger file/folder organization
				if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{OrganizeFiles: true}); err != nil {
					log.Warn("failed to organize book files after title/author change", logger.Data{
						"book_id": book.ID,
						"error":   err.Error(),
					})
				} else {
					// Reload book again to get updated file paths
					book, err = w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
					if err != nil {
						log.Warn("failed to reload book after organization", logger.Data{"error": err.Error()})
					}
					// Also reload file to get updated filepath
					file, err = w.bookService.RetrieveFileWithRelations(ctx, file.ID)
					if err != nil {
						log.Warn("failed to reload file after organization", logger.Data{"error": err.Error()})
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
			log.Info("updating file name", logger.Data{"from": existingName, "to": newFileName})
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
			log.Info("updating file name from sidecar", logger.Data{"from": existingName, "to": *fileSidecarData.Name})
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
			log.Info("updating file URL", logger.Data{"from": existingURL, "to": metadata.URL})
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
			log.Info("updating file URL from sidecar", logger.Data{"from": existingURL, "to": *fileSidecarData.URL})
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
			log.Info("updating file release date", logger.Data{"from": existingDateStr, "to": newDateStr})
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
				log.Info("updating file release date from sidecar", logger.Data{"from": existingDateStr, "to": *fileSidecarData.ReleaseDate})
				file.ReleaseDate = &parsedDate
				file.ReleaseDateSource = &sidecarSource
				fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "release_date", "release_date_source")
			} else {
				log.Warn("failed to parse sidecar release date", logger.Data{"date": *fileSidecarData.ReleaseDate, "error": err.Error()})
			}
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
			publisher, err := w.publisherService.FindOrCreatePublisher(ctx, publisherName, book.LibraryID)
			if err != nil {
				log.Warn("failed to find/create publisher", logger.Data{"publisher": publisherName, "error": err.Error()})
			} else {
				log.Info("updating file publisher", logger.Data{"from": existingPublisherName, "to": publisherName})
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
			publisher, err := w.publisherService.FindOrCreatePublisher(ctx, *fileSidecarData.Publisher, book.LibraryID)
			if err != nil {
				log.Warn("failed to find/create publisher", logger.Data{"publisher": *fileSidecarData.Publisher, "error": err.Error()})
			} else {
				log.Info("updating file publisher from sidecar", logger.Data{"from": existingPublisherName, "to": *fileSidecarData.Publisher})
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
			imprint, err := w.imprintService.FindOrCreateImprint(ctx, imprintName, book.LibraryID)
			if err != nil {
				log.Warn("failed to find/create imprint", logger.Data{"imprint": imprintName, "error": err.Error()})
			} else {
				log.Info("updating file imprint", logger.Data{"from": existingImprintName, "to": imprintName})
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
			imprint, err := w.imprintService.FindOrCreateImprint(ctx, *fileSidecarData.Imprint, book.LibraryID)
			if err != nil {
				log.Warn("failed to find/create imprint", logger.Data{"imprint": *fileSidecarData.Imprint, "error": err.Error()})
			} else {
				log.Info("updating file imprint from sidecar", logger.Data{"from": existingImprintName, "to": *fileSidecarData.Imprint})
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
			log.Warn("failed to retrieve library for file organization", logger.Data{"error": err.Error()})
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
				log.Error("failed to rename file after name change", logger.Data{
					"file_id": file.ID,
					"path":    file.Filepath,
					"error":   err.Error(),
				})
			} else if newPath != file.Filepath {
				log.Info("renamed file after name change", logger.Data{
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
					log.Error("failed to update file path after rename", logger.Data{
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
			log.Info("updating narrators", logger.Data{"new_count": len(metadata.Narrators), "old_count": len(file.Narrators)})

			// Delete existing narrators
			if _, err := w.bookService.DeleteNarratorsForFile(ctx, file.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete existing narrators")
			}

			// Create new narrators
			for i, narratorName := range metadata.Narrators {
				person, err := w.personService.FindOrCreatePerson(ctx, narratorName, book.LibraryID)
				if err != nil {
					log.Warn("failed to find/create person for narrator", logger.Data{"name": narratorName, "error": err.Error()})
					continue
				}
				narrator := &models.Narrator{
					FileID:    file.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
				}
				if err := w.bookService.CreateNarrator(ctx, narrator); err != nil {
					log.Warn("failed to create narrator", logger.Data{"error": err.Error()})
				}
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
			log.Info("updating narrators from sidecar", logger.Data{"new_count": len(fileSidecarData.Narrators), "old_count": len(file.Narrators)})

			// Delete existing narrators
			if _, err := w.bookService.DeleteNarratorsForFile(ctx, file.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete existing narrators")
			}

			// Create new narrators from sidecar
			for i, sidecarNarrator := range fileSidecarData.Narrators {
				person, err := w.personService.FindOrCreatePerson(ctx, sidecarNarrator.Name, book.LibraryID)
				if err != nil {
					log.Warn("failed to find/create person for narrator", logger.Data{"name": sidecarNarrator.Name, "error": err.Error()})
					continue
				}
				narrator := &models.Narrator{
					FileID:    file.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
				}
				if err := w.bookService.CreateNarrator(ctx, narrator); err != nil {
					log.Warn("failed to create narrator", logger.Data{"error": err.Error()})
				}
			}

			// Update narrator source
			file.NarratorSource = &sidecarSource
			if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"narrator_source"}}); err != nil {
				return nil, errors.Wrap(err, "failed to update narrator source")
			}
		}
	}

	// Update identifiers (from metadata)
	if len(metadata.Identifiers) > 0 {
		existingIdentifierSource := ""
		if file.IdentifierSource != nil {
			existingIdentifierSource = *file.IdentifierSource
		}
		existingIdentifierValues := make([]string, 0, len(file.Identifiers))
		for _, id := range file.Identifiers {
			existingIdentifierValues = append(existingIdentifierValues, id.Type+":"+id.Value)
		}
		newIdentifierValues := make([]string, 0, len(metadata.Identifiers))
		for _, id := range metadata.Identifiers {
			newIdentifierValues = append(newIdentifierValues, id.Type+":"+id.Value)
		}

		identifierSource := metadata.SourceForField("identifiers")
		if shouldUpdateRelationship(newIdentifierValues, existingIdentifierValues, identifierSource, existingIdentifierSource, forceRefresh) {
			log.Info("updating identifiers", logger.Data{"new_count": len(metadata.Identifiers), "old_count": len(file.Identifiers)})

			// Delete existing identifiers
			if err := w.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete existing identifiers")
			}

			// Create new identifiers
			for _, id := range metadata.Identifiers {
				identifier := &models.FileIdentifier{
					FileID: file.ID,
					Type:   id.Type,
					Value:  id.Value,
					Source: identifierSource,
				}
				if err := w.bookService.CreateFileIdentifier(ctx, identifier); err != nil {
					log.Warn("failed to create identifier", logger.Data{"error": err.Error()})
				}
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
		sidecarIdentifierValues := make([]string, 0, len(fileSidecarData.Identifiers))
		for _, id := range fileSidecarData.Identifiers {
			sidecarIdentifierValues = append(sidecarIdentifierValues, id.Type+":"+id.Value)
		}
		existingIdentifierSource := ""
		if file.IdentifierSource != nil {
			existingIdentifierSource = *file.IdentifierSource
		}
		existingIdentifierValues := make([]string, 0, len(file.Identifiers))
		for _, id := range file.Identifiers {
			existingIdentifierValues = append(existingIdentifierValues, id.Type+":"+id.Value)
		}

		if shouldApplySidecarRelationship(sidecarIdentifierValues, existingIdentifierValues, existingIdentifierSource, forceRefresh) {
			log.Info("updating identifiers from sidecar", logger.Data{"new_count": len(fileSidecarData.Identifiers), "old_count": len(file.Identifiers)})

			// Delete existing identifiers
			if err := w.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete existing identifiers")
			}

			// Create new identifiers from sidecar
			for _, id := range fileSidecarData.Identifiers {
				identifier := &models.FileIdentifier{
					FileID: file.ID,
					Type:   id.Type,
					Value:  id.Value,
					Source: sidecarSource,
				}
				if err := w.bookService.CreateFileIdentifier(ctx, identifier); err != nil {
					log.Warn("failed to create identifier", logger.Data{"error": err.Error()})
				}
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
			log.Info("updating chapters", logger.Data{"chapter_count": len(metadata.Chapters)})

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
			log.Info("updating chapters from sidecar", logger.Data{"chapter_count": len(sidecarChapters)})

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

	// Update cover page (from sidecar) for CBZ files
	// This restores user-selected cover page from sidecar after library rescans
	if fileSidecarData != nil && fileSidecarData.CoverPage != nil && file.FileType == models.FileTypeCBZ {
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
			// Determine cover directory
			var coverDir string
			if file.Book != nil {
				if info, statErr := os.Stat(file.Book.Filepath); statErr == nil && info.IsDir() {
					coverDir = file.Book.Filepath
				} else {
					coverDir = filepath.Dir(file.Book.Filepath)
				}
			} else {
				coverDir = filepath.Dir(file.Filepath)
			}

			// Generate cover filename
			filename := filepath.Base(file.Filepath)
			coverBaseName := filename + ".cover"

			// Extract the cover page from CBZ
			coverFilename, coverMimeType, err := extractCBZPageCover(file.Filepath, coverDir, coverBaseName, *fileSidecarData.CoverPage)
			if err != nil {
				log.Warn("failed to extract cover page from sidecar", logger.Data{
					"error":      err.Error(),
					"cover_page": *fileSidecarData.CoverPage,
				})
			} else {
				log.Info("updating cover page from sidecar", logger.Data{
					"from_page": file.CoverPage,
					"to_page":   *fileSidecarData.CoverPage,
				})

				file.CoverPage = fileSidecarData.CoverPage
				file.CoverImageFilename = &coverFilename
				file.CoverMimeType = &coverMimeType
				file.CoverSource = &sidecarSource

				if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
					Columns: []string{"cover_page", "cover_image_filename", "cover_mime_type", "cover_source"},
				}); err != nil {
					return nil, errors.Wrap(err, "failed to update cover page from sidecar")
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
		log.Warn("failed to reload book for sidecar", logger.Data{"error": err.Error()})
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
				log.Warn("failed to retrieve library for sidecar", logger.Data{"error": libErr.Error()})
			} else if lib.OrganizeFileStructure {
				// Create the directory for root-level files that will be organized
				if mkdirErr := os.MkdirAll(reloadedBook.Filepath, 0755); mkdirErr != nil {
					log.Warn("failed to create book directory for sidecar", logger.Data{"error": mkdirErr.Error()})
				} else {
					bookDirExists = true
				}
			}
			// If org is disabled, skip writing book sidecar (files are at root level)
		}

		if bookDirExists {
			if err := sidecar.WriteBookSidecarFromModel(reloadedBook); err != nil {
				log.Warn("failed to write book sidecar", logger.Data{"error": err.Error()})
			}
		}
		book = reloadedBook
	}

	reloadedFile, err := w.bookService.RetrieveFileWithRelations(ctx, file.ID)
	if err != nil {
		log.Warn("failed to reload file for sidecar", logger.Data{"error": err.Error()})
	} else {
		if err := sidecar.WriteFileSidecarFromModel(reloadedFile); err != nil {
			log.Warn("failed to write file sidecar", logger.Data{"error": err.Error()})
		}
		file = reloadedFile
	}

	// ==========================================================================
	// Update search index
	// ==========================================================================

	if w.searchService != nil {
		if err := w.searchService.IndexBook(ctx, book); err != nil {
			log.Warn("failed to update search index", logger.Data{"book_id": book.ID, "error": err.Error()})
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
func (w *Worker) scanFileCreateNew(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	log := logger.FromContext(ctx)
	path := opts.FilePath

	// Get file stats
	stats, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}
	size := stats.Size()
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

	// Determine book path
	var bookPath string
	if isRootLevelFile {
		// For root-level files, compute the expected organized folder path so that
		// multiple root-level files with the same title/author will share a book.
		// This ensures "Wind and Truth.epub" and "Wind and Truth.m4b" become one book.
		title := deriveInitialTitle(path, isRootLevelFile, metadata)
		var authorNames []string
		if metadata != nil && len(metadata.Authors) > 0 {
			for _, author := range metadata.Authors {
				authorNames = append(authorNames, author.Name)
			}
		} else {
			authorNames = extractAuthorsFromFilepath(path, isRootLevelFile)
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
		log.Info("using existing book for new file", logger.Data{"book_id": existingBook.ID, "path": path})
		book = existingBook
	} else {
		// Derive initial title from filepath or metadata
		title := deriveInitialTitle(path, isRootLevelFile, metadata)
		titleSource := models.DataSourceFilepath
		if metadata != nil && strings.TrimSpace(metadata.Title) != "" {
			titleSource = metadata.SourceForField("title")
		}

		log.Info("creating new book", logger.Data{"title": title, "path": bookPath})
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

		// Extract and create authors from filepath if metadata doesn't have authors
		// Format: [Author Name] in directory or filename
		filepathAuthors := extractAuthorsFromFilepath(bookPath, isRootLevelFile)
		if len(filepathAuthors) > 0 && (metadata == nil || len(metadata.Authors) == 0) {
			for i, authorName := range filepathAuthors {
				person, err := w.personService.FindOrCreatePerson(ctx, authorName, opts.LibraryID)
				if err != nil {
					log.Warn("failed to create person for filepath author", logger.Data{"author": authorName, "error": err.Error()})
					continue
				}
				author := &models.Author{
					BookID:    book.ID,
					PersonID:  person.ID,
					SortOrder: i + 1,
				}
				if err := w.bookService.CreateAuthor(ctx, author); err != nil {
					log.Warn("failed to create author", logger.Data{"book_id": book.ID, "person_id": person.ID, "error": err.Error()})
				}
			}
		}
		// Infer series from title if it contains a volume indicator and no series from metadata
		if metadata == nil || metadata.Series == "" {
			if seriesName, volumeNumber, ok := fileutils.ExtractSeriesFromTitle(book.Title, fileType); ok {
				seriesRecord, err := w.seriesService.FindOrCreateSeries(ctx, seriesName, opts.LibraryID, models.DataSourceFilepath)
				if err != nil {
					log.Warn("failed to create series for inferred title", logger.Data{"series": seriesName, "error": err.Error()})
				} else {
					bookSeries := &models.BookSeries{
						BookID:       book.ID,
						SeriesID:     seriesRecord.ID,
						SeriesNumber: volumeNumber,
						SortOrder:    1,
					}
					if err := w.bookService.CreateBookSeries(ctx, bookSeries); err != nil {
						log.Warn("failed to create book series", logger.Data{"book_id": book.ID, "series_id": seriesRecord.ID, "error": err.Error()})
					}
				}
			}
		}
	}

	// Handle cover extraction
	var coverImagePath *string
	var coverMimeType *string
	var coverSource *string
	var coverPage *int

	if metadata != nil && len(metadata.CoverData) > 0 {
		coverFilename, extractedMimeType, wasPreExisting, err := w.extractAndSaveCover(ctx, path, bookPath, isRootLevelFile, metadata)
		if err != nil {
			log.Warn("failed to extract cover", logger.Data{"error": err.Error()})
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
		if metadata.CoverPage != nil {
			coverPage = metadata.CoverPage
		}
	}

	// Create file record
	log.Info("creating file", logger.Data{"path": path, "filesize": size})
	file := &models.File{
		LibraryID:          opts.LibraryID,
		BookID:             book.ID,
		Filepath:           path,
		FileType:           fileType,
		FilesizeBytes:      size,
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

	// Extract and create narrators from filepath if metadata doesn't have narrators
	// Check both directory name and actual filename for {Narrator Name} pattern
	filepathNarrators := extractNarratorsFromFilepath(path, bookPath, isRootLevelFile)
	if len(filepathNarrators) > 0 && (metadata == nil || len(metadata.Narrators) == 0) {
		narratorSource := models.DataSourceFilepath
		for i, narratorName := range filepathNarrators {
			person, err := w.personService.FindOrCreatePerson(ctx, narratorName, opts.LibraryID)
			if err != nil {
				log.Warn("failed to create person for filepath narrator", logger.Data{"narrator": narratorName, "error": err.Error()})
				continue
			}
			narrator := &models.Narrator{
				FileID:    file.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
			}
			if err := w.bookService.CreateNarrator(ctx, narrator); err != nil {
				log.Warn("failed to create narrator", logger.Data{"file_id": file.ID, "person_id": person.ID, "error": err.Error()})
			}
		}
		file.NarratorSource = &narratorSource
		if err := w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: []string{"narrator_source"}}); err != nil {
			log.Warn("failed to update narrator source", logger.Data{"error": err.Error()})
		}
	}

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
	metadata = w.runMetadataEnrichers(ctx, metadata, file, book, opts.LibraryID)

	// Use scanFileCore to handle all metadata updates (authors, series, etc.)
	// This is a batch scan (FilePath mode), so pass isResync=false to skip book organization
	result, err := w.scanFileCore(ctx, file, book, metadata, opts.ForceRefresh, false)
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
) (filename string, mimeType string, wasPreExisting bool, err error) {
	log := logger.FromContext(ctx)

	if metadata == nil || len(metadata.CoverData) == 0 {
		return "", "", false, nil
	}

	// Determine cover directory
	coverDir := bookPath
	if isRootLevelFile {
		coverDir = filepath.Dir(filePath)
	}

	// Build cover base name: <filename>.cover
	coverBaseName := filepath.Base(filePath) + ".cover"

	// Check if cover already exists
	existingCoverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)
	if existingCoverPath != "" {
		log.Info("cover already exists, using existing", logger.Data{"path": existingCoverPath})
		// Detect MIME type from file extension
		existingMime := fileutils.MimeTypeFromExtension(filepath.Ext(existingCoverPath))
		return filepath.Base(existingCoverPath), existingMime, true, nil
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
	log.Info("saving cover", logger.Data{"path": coverFilepath, "mime": normalizedMime})

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
// Each enricher is called in user-defined order; first non-empty value per field wins.
func (w *Worker) runMetadataEnrichers(ctx context.Context, metadata *mediafile.ParsedMetadata, file *models.File, book *models.Book, libraryID int) *mediafile.ParsedMetadata {
	if w.pluginManager == nil || metadata == nil {
		return metadata
	}

	runtimes, err := w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, libraryID)
	if err != nil || len(runtimes) == 0 {
		return metadata
	}

	log := logger.FromContext(ctx)

	// Build enrichment context
	enrichCtx := map[string]interface{}{
		"parsedMetadata": buildParsedMetadataContext(metadata),
		"file":           buildFileContext(file),
		"book":           buildBookContext(book, file),
	}

	enrichedMeta := *metadata // copy
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

		result, eErr := w.pluginManager.RunMetadataEnricher(ctx, rt, enrichCtx)
		if eErr != nil {
			log.Warn("enricher failed", logger.Data{
				"plugin": rt.Manifest().ID,
				"error":  eErr.Error(),
			})
			continue
		}
		if !result.Modified || result.Metadata == nil {
			continue
		}

		// Get effective field settings for this library + plugin
		declaredFields := enricherCap.Fields
		enabledFields, fErr := w.pluginService.GetEffectiveFieldSettings(ctx, libraryID, rt.Scope(), rt.PluginID(), declaredFields)
		if fErr != nil {
			log.Warn("failed to get field settings", logger.Data{
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
		filteredMetadata := filterMetadataFields(result.Metadata, declaredFields, enabledFields, rt.PluginID(), log)

		// Merge: first non-empty wins per field, tracking source per field
		enricherSource := models.PluginDataSource(rt.Scope(), rt.PluginID())
		mergeEnrichedMetadata(&enrichedMeta, filteredMetadata, enricherSource)
		if !modified {
			enrichedMeta.DataSource = enricherSource
		}
		modified = true
	}

	return &enrichedMeta
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
	if len(target.CoverData) == 0 && len(enrichment.CoverData) > 0 {
		target.CoverData = enrichment.CoverData
		target.CoverMimeType = enrichment.CoverMimeType
		target.FieldDataSources["cover"] = source
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
	log logger.Logger,
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
			log.Warn("enricher returned undeclared field", logger.Data{
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
			log.Warn("enricher returned undeclared field", logger.Data{
				"plugin": pluginID,
				"field":  "series",
			})
		}
		if result.SeriesNumber != nil {
			log.Warn("enricher returned undeclared field", logger.Data{
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
		warnIfUndeclared("cover", len(result.CoverData) > 0 || result.CoverMimeType != "" || result.CoverPage != nil)
		result.CoverData = nil
		result.CoverMimeType = ""
		result.CoverPage = nil
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

	return &result
}

// buildParsedMetadataContext converts ParsedMetadata to a map for plugin enrichment context.
func buildParsedMetadataContext(md *mediafile.ParsedMetadata) map[string]interface{} {
	if md == nil {
		return nil
	}

	ctx := map[string]interface{}{
		"title":       md.Title,
		"subtitle":    md.Subtitle,
		"series":      md.Series,
		"description": md.Description,
		"publisher":   md.Publisher,
		"imprint":     md.Imprint,
		"url":         md.URL,
		"dataSource":  md.DataSource,
	}

	if md.SeriesNumber != nil {
		ctx["seriesNumber"] = *md.SeriesNumber
	}

	if len(md.Authors) > 0 {
		authors := make([]map[string]interface{}, len(md.Authors))
		for i, a := range md.Authors {
			authors[i] = map[string]interface{}{
				"name": a.Name,
				"role": a.Role,
			}
		}
		ctx["authors"] = authors
	}

	if len(md.Narrators) > 0 {
		ctx["narrators"] = md.Narrators
	}

	if len(md.Genres) > 0 {
		ctx["genres"] = md.Genres
	}

	if len(md.Tags) > 0 {
		ctx["tags"] = md.Tags
	}

	if md.ReleaseDate != nil {
		ctx["releaseDate"] = md.ReleaseDate.Format("2006-01-02")
	}

	if len(md.Identifiers) > 0 {
		identifiers := make([]map[string]interface{}, len(md.Identifiers))
		for i, id := range md.Identifiers {
			identifiers[i] = map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			}
		}
		ctx["identifiers"] = identifiers
	}

	return ctx
}

// buildFileContext converts a File model to a map for plugin enrichment context.
func buildFileContext(file *models.File) map[string]interface{} {
	if file == nil {
		return nil
	}

	ctx := map[string]interface{}{
		"id":            file.ID,
		"filepath":      file.Filepath,
		"fileType":      file.FileType,
		"fileRole":      file.FileRole,
		"filesizeBytes": file.FilesizeBytes,
	}

	if file.Name != nil {
		ctx["name"] = *file.Name
	}

	if file.URL != nil {
		ctx["url"] = *file.URL
	}

	return ctx
}

// buildBookContext converts a Book model to a map for plugin enrichment context.
// It provides the current DB state of the book, including manually-edited fields.
// The file parameter is used to find the specific file's identifiers and publisher.
func buildBookContext(book *models.Book, file *models.File) map[string]interface{} {
	if book == nil {
		return nil
	}

	ctx := map[string]interface{}{
		"id":    book.ID,
		"title": book.Title,
	}

	if book.Subtitle != nil {
		ctx["subtitle"] = *book.Subtitle
	}

	if book.Description != nil {
		ctx["description"] = *book.Description
	}

	// Series (from BookSeries relations)
	if len(book.BookSeries) > 0 {
		seriesList := make([]map[string]interface{}, 0, len(book.BookSeries))
		for _, bs := range book.BookSeries {
			if bs.Series == nil {
				continue
			}
			entry := map[string]interface{}{
				"name": bs.Series.Name,
			}
			if bs.SeriesNumber != nil {
				entry["number"] = *bs.SeriesNumber
			}
			seriesList = append(seriesList, entry)
		}
		if len(seriesList) > 0 {
			ctx["series"] = seriesList
		}
	}

	// Authors
	if len(book.Authors) > 0 {
		authors := make([]map[string]interface{}, 0, len(book.Authors))
		for _, a := range book.Authors {
			if a.Person == nil {
				continue
			}
			entry := map[string]interface{}{
				"name": a.Person.Name,
			}
			if a.Role != nil {
				entry["role"] = *a.Role
			}
			authors = append(authors, entry)
		}
		if len(authors) > 0 {
			ctx["authors"] = authors
		}
	}

	// Genres
	if len(book.BookGenres) > 0 {
		genres := make([]string, 0, len(book.BookGenres))
		for _, bg := range book.BookGenres {
			if bg.Genre != nil {
				genres = append(genres, bg.Genre.Name)
			}
		}
		if len(genres) > 0 {
			ctx["genres"] = genres
		}
	}

	// Tags
	if len(book.BookTags) > 0 {
		tags := make([]string, 0, len(book.BookTags))
		for _, bt := range book.BookTags {
			if bt.Tag != nil {
				tags = append(tags, bt.Tag.Name)
			}
		}
		if len(tags) > 0 {
			ctx["tags"] = tags
		}
	}

	// File-level fields: identifiers and publisher from the specific file being enriched
	if file != nil {
		if len(file.Identifiers) > 0 {
			identifiers := make([]map[string]interface{}, len(file.Identifiers))
			for i, id := range file.Identifiers {
				identifiers[i] = map[string]interface{}{
					"type":  id.Type,
					"value": id.Value,
				}
			}
			ctx["identifiers"] = identifiers
		}

		if file.Publisher != nil {
			ctx["publisher"] = file.Publisher.Name
		}
	}

	return ctx
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
	}

	// Call internal unified Scan method
	result, err := w.scanInternal(ctx, internalOpts)
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
// and re-extracts it from the media file if needed.
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
