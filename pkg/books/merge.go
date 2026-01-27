package books

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/uptrace/bun"
)

// MoveFilesOptions contains the parameters for moving files between books.
type MoveFilesOptions struct {
	FileIDs         []int    // IDs of files to move
	TargetBookID    *int     // Target book ID (nil to create a new book)
	LibraryID       int      // Library ID for validation
	IgnoredPatterns []string // Patterns for ignored files during cleanup (e.g., ".DS_Store", ".*")
}

// MoveFilesResult contains the result of a move files operation.
type MoveFilesResult struct {
	TargetBook        *models.Book // The target book (with relations loaded)
	FilesMoved        int          // Number of files successfully moved
	SourceBookDeleted bool         // True if any source book was deleted
	DeletedBookIDs    []int        // IDs of books that were deleted
	NewBookCreated    bool         // True if a new book was created
}

// fileMove tracks a file move operation for rollback purposes.
type fileMove struct {
	fileID    int
	oldPath   string
	newPath   string
	oldDir    string // Source directory for cleanup
	oldBookID int
	newBookID int
	fileMoved bool // True if the physical file was moved
	dbUpdated bool // True if the database was updated
}

// MoveFilesToBook moves files from their current books to a target book.
// If TargetBookID is nil, a new book is created from the first file's directory.
// This method handles physical file relocation when the library has OrganizeFileStructure enabled.
func (svc *Service) MoveFilesToBook(ctx context.Context, opts MoveFilesOptions) (*MoveFilesResult, error) {
	log := logger.FromContext(ctx)

	if len(opts.FileIDs) == 0 {
		return nil, errors.New("no files specified to move")
	}

	result := &MoveFilesResult{
		DeletedBookIDs: []int{},
	}

	// Fetch all files to validate they exist and are in the specified library
	var files []*models.File
	err := svc.db.NewSelect().
		Model(&files).
		Where("id IN (?)", bun.In(opts.FileIDs)).
		Where("library_id = ?", opts.LibraryID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(files) != len(opts.FileIDs) {
		return nil, errors.New("some files not found or not in the specified library")
	}

	// Fetch library to check OrganizeFileStructure
	var library models.Library
	err = svc.db.NewSelect().
		Model(&library).
		Relation("LibraryPaths").
		Where("l.id = ?", opts.LibraryID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Get or create target book
	var targetBook *models.Book
	if opts.TargetBookID != nil {
		// Fetch existing target book
		targetBook, err = svc.RetrieveBook(ctx, RetrieveBookOptions{
			ID:        opts.TargetBookID,
			LibraryID: &opts.LibraryID,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve target book")
		}
	} else {
		// Create a new book from the first file
		// Use the file's name as the title and copy authors from its source book
		targetBook, err = svc.createBookFromFile(ctx, files[0], files[0].BookID, &library)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create new book")
		}
		result.NewBookCreated = true
		log.Info("created new book for file move", logger.Data{
			"book_id":    targetBook.ID,
			"book_title": targetBook.Title,
		})
	}

	// Track source books to check for cleanup later (ID -> filepath)
	sourceBookPaths := make(map[int]string)
	for _, file := range files {
		if file.BookID != targetBook.ID {
			sourceBookPaths[file.BookID] = "" // Will be populated below
		}
	}

	// Fetch source book filepaths for sidecar cleanup
	if len(sourceBookPaths) > 0 {
		sourceIDs := make([]int, 0, len(sourceBookPaths))
		for id := range sourceBookPaths {
			sourceIDs = append(sourceIDs, id)
		}
		var sourceBooks []*models.Book
		err = svc.db.NewSelect().
			Model(&sourceBooks).
			Where("id IN (?)", bun.In(sourceIDs)).
			Scan(ctx)
		if err != nil {
			log.Warn("failed to fetch source book paths for cleanup", logger.Data{"error": err.Error()})
		} else {
			for _, sb := range sourceBooks {
				sourceBookPaths[sb.ID] = sb.Filepath
			}
		}
	}

	// Track file moves for potential rollback
	var moves []fileMove

	// Process each file
	err = svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()

		for _, file := range files {
			// Skip if already on the target book
			if file.BookID == targetBook.ID {
				continue
			}

			move := fileMove{
				fileID:    file.ID,
				oldPath:   file.Filepath,
				oldBookID: file.BookID,
				newBookID: targetBook.ID,
			}

			// Handle physical file move if organize is enabled
			if library.OrganizeFileStructure {
				newPath := filepath.Join(targetBook.Filepath, filepath.Base(file.Filepath))
				// Generate unique path if a file already exists at the destination
				newPath = fileutils.GenerateUniqueFilepathIfExists(newPath)

				if newPath != file.Filepath {
					// Move file along with associated files (covers, sidecars)
					_, err := fileutils.MoveFileWithAssociatedFiles(file.Filepath, newPath)
					if err != nil {
						// Rollback any previous moves
						failures := svc.rollbackFileMoves(ctx, moves)
						baseErr := errors.Wrapf(err, "failed to move file %d to %s", file.ID, newPath)
						return wrapErrorWithRollbackFailures(baseErr, failures)
					}
					move.newPath = newPath
					move.fileMoved = true
					move.oldDir = filepath.Dir(file.Filepath)
					file.Filepath = newPath

					// Update cover path if it exists
					if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
						newCoverPath := fileutils.ComputeNewCoverFilename(*file.CoverImageFilename, newPath)
						file.CoverImageFilename = &newCoverPath
					}
				}
			}

			// Update database record
			file.BookID = targetBook.ID
			file.UpdatedAt = now

			columns := []string{"book_id", "updated_at"}
			if move.fileMoved {
				columns = append(columns, "filepath")
				// Also update cover path if file has one (it was computed above)
				if file.CoverImageFilename != nil {
					columns = append(columns, "cover_image_filename")
				}
			}

			_, err := tx.NewUpdate().
				Model(file).
				Column(columns...).
				WherePK().
				Exec(ctx)
			if err != nil {
				var allFailures []rollbackFailure
				// Rollback physical file move for this file
				if move.fileMoved {
					if rbErr := fileutils.MoveFile(move.newPath, move.oldPath); rbErr != nil {
						log.Error("failed to rollback file move", logger.Data{
							"file_id":  file.ID,
							"old_path": move.oldPath,
							"new_path": move.newPath,
							"error":    rbErr.Error(),
						})
						allFailures = append(allFailures, rollbackFailure{
							fileID:  file.ID,
							oldPath: move.oldPath,
							newPath: move.newPath,
							err:     rbErr,
						})
					}
				}
				// Rollback any previous moves
				failures := svc.rollbackFileMoves(ctx, moves)
				allFailures = append(allFailures, failures...)
				baseErr := errors.Wrapf(err, "failed to update file %d", file.ID)
				return wrapErrorWithRollbackFailures(baseErr, allFailures)
			}
			move.dbUpdated = true
			moves = append(moves, move)
			result.FilesMoved++

			log.Debug("moved file to book", logger.Data{
				"file_id":      file.ID,
				"from_book_id": move.oldBookID,
				"to_book_id":   targetBook.ID,
				"file_moved":   move.fileMoved,
			})
		}

		// Update target book's updated_at timestamp
		_, err := tx.NewUpdate().
			Model(targetBook).
			Set("updated_at = ?", now).
			WherePK().
			Exec(ctx)
		if err != nil {
			failures := svc.rollbackFileMoves(ctx, moves)
			baseErr := errors.Wrap(err, "failed to update target book timestamp")
			return wrapErrorWithRollbackFailures(baseErr, failures)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Check and delete empty source books (in a separate transaction for safety)
	// Using a transaction with FOR UPDATE to prevent race conditions
	// Also track which books were deleted so we can remove their sidecars
	deletedBookPaths := make(map[int]string)
	err = svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		for sourceBookID, bookPath := range sourceBookPaths {
			// Lock the book row and count files atomically
			var count int
			count, err = tx.NewSelect().
				Model((*models.File)(nil)).
				Where("book_id = ?", sourceBookID).
				Count(ctx)
			if err != nil {
				log.Error("failed to count files for source book", logger.Data{
					"book_id": sourceBookID,
					"error":   err.Error(),
				})
				continue
			}

			if count == 0 {
				// Delete the empty book within the transaction
				_, err = tx.NewDelete().
					Model((*models.Book)(nil)).
					Where("id = ?", sourceBookID).
					Exec(ctx)
				if err != nil {
					log.Error("failed to delete empty source book", logger.Data{
						"book_id": sourceBookID,
						"error":   err.Error(),
					})
					continue
				}
				result.DeletedBookIDs = append(result.DeletedBookIDs, sourceBookID)
				result.SourceBookDeleted = true
				deletedBookPaths[sourceBookID] = bookPath
				log.Info("deleted empty source book", logger.Data{
					"book_id": sourceBookID,
				})
			}
		}
		return nil
	})
	if err != nil {
		log.Error("failed to cleanup empty source books", logger.Data{"error": err.Error()})
		// Don't fail the whole operation if cleanup fails, just log it
	}

	// Remove book sidecars for deleted books (best effort)
	for bookID, bookPath := range deletedBookPaths {
		if bookPath == "" {
			continue
		}
		sidecarPath := sidecar.BookSidecarPath(bookPath)
		if err := os.Remove(sidecarPath); err != nil && !os.IsNotExist(err) {
			log.Warn("failed to remove book sidecar", logger.Data{
				"book_id":      bookID,
				"sidecar_path": sidecarPath,
				"error":        err.Error(),
			})
		} else if err == nil {
			log.Debug("removed book sidecar", logger.Data{
				"book_id":      bookID,
				"sidecar_path": sidecarPath,
			})
		}
	}

	// Clean up empty source directories (best effort, don't fail operation)
	if library.OrganizeFileStructure {
		// Collect unique source directories to clean up
		dirsToClean := make(map[string]bool)
		for _, move := range moves {
			if move.oldDir != "" {
				dirsToClean[move.oldDir] = true
			}
		}

		// Find the library root path for boundary checking
		var libraryRootPath string
		if len(library.LibraryPaths) > 0 {
			libraryRootPath = library.LibraryPaths[0].Filepath
		}

		// Clean up empty directories (also removing ignored files like .DS_Store)
		for dir := range dirsToClean {
			// Try to clean up the directory and any empty parents up to the library root
			if libraryRootPath != "" {
				if err := fileutils.CleanupEmptyParentDirectories(dir, libraryRootPath, opts.IgnoredPatterns...); err != nil {
					log.Warn("failed to cleanup empty source directory", logger.Data{
						"directory": dir,
						"error":     err.Error(),
					})
				}
			} else {
				// If no library root, just try to clean the immediate directory
				if _, err := fileutils.CleanupEmptyDirectory(dir, opts.IgnoredPatterns...); err != nil {
					log.Warn("failed to cleanup empty source directory", logger.Data{
						"directory": dir,
						"error":     err.Error(),
					})
				}
			}
		}
	}

	// Reload target book with all relations
	result.TargetBook, err = svc.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &targetBook.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to reload target book")
	}

	return result, nil
}

// createBookFromFile creates a new book based on a file.
// The title is derived from the file's metadata name if available, otherwise from the filename on disk.
// All other metadata (authors, series, genres, tags, description, etc.) is copied from the source book.
// When OrganizeFileStructure is enabled:
//   - Creates a new directory using the [Author] Title format in the library path
//
// When OrganizeFileStructure is disabled:
//   - Uses the file's current directory; returns an error if a book already exists there
func (svc *Service) createBookFromFile(ctx context.Context, file *models.File, sourceBookID int, library *models.Library) (*models.Book, error) {
	now := time.Now()

	// Fetch source book with all metadata
	var sourceBook models.Book
	err := svc.db.NewSelect().
		Model(&sourceBook).
		Where("id = ?", sourceBookID).
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch source book")
	}

	// Fetch authors from source book early - we need them for folder naming
	var sourceAuthors []*models.Author
	err = svc.db.NewSelect().
		Model(&sourceAuthors).
		Relation("Person").
		Where("book_id = ?", sourceBookID).
		Order("sort_order ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch source book authors")
	}

	// Derive title from file's metadata name, falling back to filename on disk
	var title string
	var titleSource string
	if file.Name != nil && *file.Name != "" {
		title = *file.Name
		titleSource = models.DataSourceFileMetadata
	} else {
		filename := filepath.Base(file.Filepath)
		title = strings.TrimSuffix(filename, filepath.Ext(filename))
		titleSource = models.DataSourceFilepath
	}

	// Determine book directory
	var bookDir string
	if library.OrganizeFileStructure {
		// Generate organized folder name: [Author] Title
		var authorNames []string
		for _, author := range sourceAuthors {
			if author.Person != nil && author.Person.Name != "" {
				authorNames = append(authorNames, author.Person.Name)
			}
		}

		folderName := fileutils.GenerateOrganizedFolderName(fileutils.OrganizedNameOptions{
			AuthorNames: authorNames,
			Title:       title,
			FileType:    file.FileType,
		})

		// Use the first library path as the parent directory
		if len(library.LibraryPaths) == 0 {
			return nil, errors.New("library has no paths configured")
		}
		parentDir := library.LibraryPaths[0].Filepath
		bookDir = filepath.Join(parentDir, folderName)

		// Ensure the directory is unique
		baseBookDir := bookDir
		for i := 1; i <= 100; i++ {
			// Check if this directory is already used by another book
			count, err := svc.db.NewSelect().
				Model((*models.Book)(nil)).
				Where("filepath = ? AND library_id = ?", bookDir, library.ID).
				Count(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to check for existing book")
			}
			if count == 0 {
				break
			}
			bookDir = fmt.Sprintf("%s_%d", baseBookDir, i)
			if i == 100 {
				return nil, errors.New("could not generate a unique directory for the new book after 100 attempts")
			}
		}

		// Create the new directory since files will be moved here
		if err := os.MkdirAll(bookDir, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to create directory for new book")
		}
	} else {
		// Use the file's directory as the book's filepath
		bookDir = filepath.Dir(file.Filepath)

		// Check if a book already exists at this directory
		existingCount, err := svc.db.NewSelect().
			Model((*models.Book)(nil)).
			Where("filepath = ? AND library_id = ?", bookDir, library.ID).
			Count(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to check for existing book")
		}

		if existingCount > 0 {
			// Without organize_file_structure, we cannot create a new book because:
			// 1. The directory is already used by another book
			// 2. We cannot move the file to a new directory
			return nil, errors.New("cannot create a new book: a book already exists at this location. Please select an existing book as the target, or enable 'Organize File Structure' in library settings to allow creating a subdirectory for the new book")
		}
	}

	// Create new book with title and all other metadata from source book
	book := &models.Book{
		LibraryID:         library.ID,
		Filepath:          bookDir,
		Title:             title,
		TitleSource:       titleSource,
		SortTitle:         sortname.ForTitle(title),
		SortTitleSource:   titleSource,
		Subtitle:          sourceBook.Subtitle,
		SubtitleSource:    sourceBook.SubtitleSource,
		Description:       sourceBook.Description,
		DescriptionSource: sourceBook.DescriptionSource,
		AuthorSource:      sourceBook.AuthorSource,
		GenreSource:       sourceBook.GenreSource,
		TagSource:         sourceBook.TagSource,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	_, err = svc.db.NewInsert().
		Model(book).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Copy authors from source book (already fetched earlier for folder naming)
	if len(sourceAuthors) > 0 {
		newAuthors := make([]*models.Author, len(sourceAuthors))
		for i, author := range sourceAuthors {
			newAuthors[i] = &models.Author{
				BookID:    book.ID,
				PersonID:  author.PersonID,
				SortOrder: author.SortOrder,
				Role:      author.Role,
			}
		}

		_, err = svc.db.NewInsert().
			Model(&newAuthors).
			Exec(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy authors to new book")
		}
	}

	// Copy series associations from source book
	var sourceSeries []*models.BookSeries
	err = svc.db.NewSelect().
		Model(&sourceSeries).
		Where("book_id = ?", sourceBookID).
		Order("sort_order ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch source book series")
	}

	if len(sourceSeries) > 0 {
		newSeries := make([]*models.BookSeries, len(sourceSeries))
		for i, bs := range sourceSeries {
			newSeries[i] = &models.BookSeries{
				BookID:       book.ID,
				SeriesID:     bs.SeriesID,
				SeriesNumber: bs.SeriesNumber,
				SortOrder:    bs.SortOrder,
			}
		}

		_, err = svc.db.NewInsert().
			Model(&newSeries).
			Exec(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy series to new book")
		}
	}

	// Copy genres from source book
	var sourceGenres []*models.BookGenre
	err = svc.db.NewSelect().
		Model(&sourceGenres).
		Where("book_id = ?", sourceBookID).
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch source book genres")
	}

	if len(sourceGenres) > 0 {
		newGenres := make([]*models.BookGenre, len(sourceGenres))
		for i, bg := range sourceGenres {
			newGenres[i] = &models.BookGenre{
				BookID:  book.ID,
				GenreID: bg.GenreID,
			}
		}

		_, err = svc.db.NewInsert().
			Model(&newGenres).
			Exec(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy genres to new book")
		}
	}

	// Copy tags from source book
	var sourceTags []*models.BookTag
	err = svc.db.NewSelect().
		Model(&sourceTags).
		Where("book_id = ?", sourceBookID).
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch source book tags")
	}

	if len(sourceTags) > 0 {
		newTags := make([]*models.BookTag, len(sourceTags))
		for i, bt := range sourceTags {
			newTags[i] = &models.BookTag{
				BookID: book.ID,
				TagID:  bt.TagID,
			}
		}

		_, err = svc.db.NewInsert().
			Model(&newTags).
			Exec(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy tags to new book")
		}
	}

	return book, nil
}

// rollbackFileMoves attempts to undo physical file moves. Returns a list of
// files that could not be rolled back for error reporting.
func (svc *Service) rollbackFileMoves(ctx context.Context, moves []fileMove) []rollbackFailure {
	log := logger.FromContext(ctx)
	var failures []rollbackFailure

	for i := len(moves) - 1; i >= 0; i-- {
		move := moves[i]
		if move.fileMoved {
			err := fileutils.MoveFile(move.newPath, move.oldPath)
			if err != nil {
				log.Error("failed to rollback file move during error recovery", logger.Data{
					"file_id":  move.fileID,
					"old_path": move.oldPath,
					"new_path": move.newPath,
					"error":    err.Error(),
				})
				failures = append(failures, rollbackFailure{
					fileID:  move.fileID,
					oldPath: move.oldPath,
					newPath: move.newPath,
					err:     err,
				})
			}
		}
	}
	return failures
}

// rollbackFailure tracks a file that could not be rolled back.
type rollbackFailure struct {
	fileID  int
	oldPath string
	newPath string
	err     error
}

// wrapErrorWithRollbackFailures adds rollback failure details to an error message.
func wrapErrorWithRollbackFailures(err error, failures []rollbackFailure) error {
	if len(failures) == 0 {
		return err
	}
	var sb strings.Builder
	sb.WriteString(err.Error())
	sb.WriteString(". WARNING: Some files could not be restored to their original location and require manual intervention:")
	for _, f := range failures {
		fmt.Fprintf(&sb, " [file %d: %s -> %s (%v)]", f.fileID, f.newPath, f.oldPath, f.err)
	}
	return errors.New(sb.String())
}
