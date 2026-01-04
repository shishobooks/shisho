package books

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

type handler struct {
	bookService    *Service
	libraryService *libraries.Service
	personService  *people.Service
	searchService  *search.Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListBooksOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
		SeriesID:  params.SeriesID,
		Search:    params.Search,
		FileTypes: params.FileTypes,
	}

	// Filter by user's library access if user is in context
	if user, ok := c.Get("user").(*models.User); ok {
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
	}

	books, total, err := h.bookService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Books []*models.Book `json:"books"`
		Total int            `json:"total"`
	}{books, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	// Bind params.
	params := UpdateBookPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the book.
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Keep track of what's been changed.
	opts := UpdateBookOptions{Columns: []string{}}
	authorsChanged := false
	seriesChanged := false
	shouldOrganizeFiles := false

	// Track old series IDs for FTS re-indexing
	oldSeriesIDs := make([]int, 0)
	for _, bs := range book.BookSeries {
		oldSeriesIDs = append(oldSeriesIDs, bs.SeriesID)
	}

	// Update title
	if params.Title != nil && *params.Title != book.Title {
		book.Title = *params.Title
		book.TitleSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "title", "title_source")
		shouldOrganizeFiles = true
	}

	// Update subtitle
	if params.Subtitle != nil {
		// Check if subtitle actually changed
		currentSubtitle := ""
		if book.Subtitle != nil {
			currentSubtitle = *book.Subtitle
		}
		if *params.Subtitle != currentSubtitle {
			if *params.Subtitle == "" {
				book.Subtitle = nil
			} else {
				book.Subtitle = params.Subtitle
			}
			book.SubtitleSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "subtitle", "subtitle_source")
		}
	}

	// Update authors
	if params.Authors != nil {
		authorsChanged = true
		shouldOrganizeFiles = true
		book.AuthorSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "author_source")

		// Delete existing author associations
		if err := h.bookService.DeleteAuthors(ctx, book.ID); err != nil {
			return errors.WithStack(err)
		}

		// Create new author associations
		for i, authorName := range params.Authors {
			if authorName == "" {
				continue
			}
			person, err := h.personService.FindOrCreatePerson(ctx, authorName, book.LibraryID)
			if err != nil {
				log.Error("failed to find/create person", logger.Data{"author": authorName, "error": err.Error()})
				continue
			}
			author := &models.Author{
				BookID:    book.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
			}
			if err := h.bookService.CreateAuthor(ctx, author); err != nil {
				log.Error("failed to create author", logger.Data{"book_id": book.ID, "person_id": person.ID, "error": err.Error()})
			}
		}
	}

	// Update series
	if params.Series != nil {
		seriesChanged = true

		// Check if series number changed for CBZ files (triggers file organization)
		hasCBZFiles := false
		for _, file := range book.Files {
			if file.FileType == models.FileTypeCBZ {
				hasCBZFiles = true
				break
			}
		}
		if hasCBZFiles {
			// Compare old and new series numbers
			oldSeriesNum := float64(0)
			if len(book.BookSeries) > 0 && book.BookSeries[0].SeriesNumber != nil {
				oldSeriesNum = *book.BookSeries[0].SeriesNumber
			}
			newSeriesNum := float64(0)
			if len(params.Series) > 0 && params.Series[0].Number != nil {
				newSeriesNum = *params.Series[0].Number
			}
			if oldSeriesNum != newSeriesNum {
				shouldOrganizeFiles = true
			}
		}

		// Delete existing series associations
		if err := h.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
			return errors.WithStack(err)
		}

		// Create new series associations
		for i, seriesInput := range params.Series {
			if seriesInput.Name == "" {
				continue
			}
			seriesRecord, err := h.bookService.FindOrCreateSeries(ctx, seriesInput.Name, book.LibraryID, models.DataSourceManual)
			if err != nil {
				log.Error("failed to find/create series", logger.Data{"series": seriesInput.Name, "error": err.Error()})
				continue
			}
			bookSeries := &models.BookSeries{
				BookID:       book.ID,
				SeriesID:     seriesRecord.ID,
				SeriesNumber: seriesInput.Number,
				SortOrder:    i + 1,
			}
			if err := h.bookService.CreateBookSeries(ctx, bookSeries); err != nil {
				log.Error("failed to create book series", logger.Data{"book_id": book.ID, "series_id": seriesRecord.ID, "error": err.Error()})
			}
		}
	}

	// Update the model first (without file organization).
	err = h.bookService.UpdateBook(ctx, book, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload the model with all relations (including newly created authors/series)
	// This is needed BEFORE organizing files so that organizeBookFiles has fresh data
	book, err = h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Now organize files if needed (after reloading to get fresh author/series data)
	if shouldOrganizeFiles {
		organizeOpts := UpdateBookOptions{OrganizeFiles: true}
		if err := h.bookService.UpdateBook(ctx, book, organizeOpts); err != nil {
			log.Warn("failed to organize book files", logger.Data{"book_id": book.ID, "error": err.Error()})
		}

		// Reload again to get updated file paths
		book, err = h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
			ID: &id,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Write sidecar files to keep them in sync with the database
	if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
		log.Warn("failed to write book sidecar", logger.Data{"error": err.Error()})
	}
	// Also write file sidecars for all files in the book
	for _, file := range book.Files {
		if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
			log.Warn("failed to write file sidecar", logger.Data{"file_id": file.ID, "error": err.Error()})
		}
	}

	// Update FTS index for this book
	if err := h.searchService.IndexBook(ctx, book); err != nil {
		log.Warn("failed to update search index for book", logger.Data{"book_id": book.ID, "error": err.Error()})
	}

	// Update FTS index for affected series (old and new)
	if seriesChanged {
		// Re-index old series
		for _, seriesID := range oldSeriesIDs {
			seriesRecord, err := h.bookService.RetrieveSeriesByID(ctx, seriesID)
			if err == nil {
				if err := h.searchService.IndexSeries(ctx, seriesRecord); err != nil {
					log.Warn("failed to update search index for old series", logger.Data{"series_id": seriesID, "error": err.Error()})
				}
			}
		}
		// Re-index new series
		for _, bs := range book.BookSeries {
			if bs.Series != nil {
				if err := h.searchService.IndexSeries(ctx, bs.Series); err != nil {
					log.Warn("failed to update search index for new series", logger.Data{"series_id": bs.SeriesID, "error": err.Error()})
				}
			}
		}
	}

	// Cleanup orphaned records
	if authorsChanged {
		if _, err := h.personService.CleanupOrphanedPeople(ctx); err != nil {
			log.Warn("failed to cleanup orphaned people", logger.Data{"error": err.Error()})
		}
	}
	if seriesChanged {
		if _, err := h.bookService.CleanupOrphanedSeries(ctx); err != nil {
			log.Warn("failed to cleanup orphaned series", logger.Data{"error": err.Error()})
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

// strPtr is a helper to get a pointer to a string.
func strPtr(s string) *string {
	return &s
}

func (h *handler) updateFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Bind params.
	params := UpdateFilePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the file with its book
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get the library to check OrganizeFileStructure
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &file.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Get the parent book for author names (needed for file renaming)
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	narratorsChanged := false
	opts := UpdateFileOptions{Columns: []string{}}

	// Update narrators
	if params.Narrators != nil {
		narratorsChanged = true
		file.NarratorSource = strPtr(models.DataSourceManual)
		opts.Columns = append(opts.Columns, "narrator_source")

		// Delete existing narrator associations
		if err := h.bookService.DeleteNarrators(ctx, file.ID); err != nil {
			return errors.WithStack(err)
		}

		// Create new narrator associations
		narratorNames := make([]string, 0, len(params.Narrators))
		for i, narratorName := range params.Narrators {
			if narratorName == "" {
				continue
			}
			person, err := h.personService.FindOrCreatePerson(ctx, narratorName, file.LibraryID)
			if err != nil {
				log.Error("failed to find/create person", logger.Data{"narrator": narratorName, "error": err.Error()})
				continue
			}
			narrator := &models.Narrator{
				FileID:    file.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
			}
			if err := h.bookService.CreateNarrator(ctx, narrator); err != nil {
				log.Error("failed to create narrator", logger.Data{"file_id": file.ID, "person_id": person.ID, "error": err.Error()})
			}
			narratorNames = append(narratorNames, narratorName)
		}

		// For M4B files with OrganizeFileStructure enabled, rename the file to include narrator
		if file.FileType == models.FileTypeM4B && library.OrganizeFileStructure && len(narratorNames) > 0 {
			// Get author names from the book
			authorNames := make([]string, 0, len(book.Authors))
			for _, a := range book.Authors {
				if a.Person != nil {
					authorNames = append(authorNames, a.Person.Name)
				}
			}

			// Generate organized name options
			organizeOpts := fileutils.OrganizedNameOptions{
				AuthorNames:   authorNames,
				NarratorNames: narratorNames,
				Title:         book.Title,
				FileType:      file.FileType,
			}

			// Rename the file
			newPath, err := fileutils.RenameOrganizedFile(file.Filepath, organizeOpts)
			if err != nil {
				log.Error("failed to rename file with narrator", logger.Data{
					"file_id": file.ID,
					"path":    file.Filepath,
					"error":   err.Error(),
				})
			} else if newPath != file.Filepath {
				log.Info("renamed file with narrator", logger.Data{
					"file_id":  file.ID,
					"old_path": file.Filepath,
					"new_path": newPath,
				})
				file.Filepath = newPath
				opts.Columns = append(opts.Columns, "filepath")
			}
		}
	}

	// Update the file
	if err := h.bookService.UpdateFile(ctx, file, opts); err != nil {
		return errors.WithStack(err)
	}

	// Reload the file with narrators
	file, err = h.bookService.RetrieveFileWithNarrators(ctx, file.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Write file sidecar
	if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
		log.Warn("failed to write file sidecar", logger.Data{"file_id": file.ID, "error": err.Error()})
	}

	// Re-index the parent book (narrators are indexed in books_fts)
	book, err = h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err == nil {
		if err := h.searchService.IndexBook(ctx, book); err != nil {
			log.Warn("failed to update search index for book", logger.Data{"book_id": book.ID, "error": err.Error()})
		}
	}

	// Cleanup orphaned people
	if narratorsChanged {
		if _, err := h.personService.CleanupOrphanedPeople(ctx); err != nil {
			log.Warn("failed to cleanup orphaned people", logger.Data{"error": err.Error()})
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, file))
}

func (h *handler) fileCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Determine cover directory (same logic as scan worker)
	isRootLevelBook := false
	if info, err := os.Stat(file.Book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}
	var coverDir string
	if isRootLevelBook {
		coverDir = filepath.Dir(file.Book.Filepath)
	} else {
		coverDir = file.Book.Filepath
	}

	// Cover filename is {filename}.cover.{ext}
	filename := filepath.Base(file.Filepath)
	coverPath := filepath.Join(coverDir, filename+".cover"+file.CoverExtension())

	return errors.WithStack(c.File(coverPath))
}

func (h *handler) uploadFileCover(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Get the uploaded file
	fileHeader, err := c.FormFile("cover")
	if err != nil {
		return errcodes.ValidationError("Cover image is required")
	}

	// Validate file type
	contentType := fileHeader.Header.Get("Content-Type")
	if !isValidImageType(contentType) {
		return errcodes.ValidationError("Invalid image type. Allowed types: JPEG, PNG, WebP")
	}

	// Get extension from content type
	ext := getExtensionFromMimeType(contentType)
	if ext == "" {
		return errcodes.ValidationError("Could not determine image extension")
	}

	// Fetch the file
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get the parent book for the cover directory
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Determine cover directory
	isRootLevelBook := false
	if info, err := os.Stat(book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}
	var coverDir string
	if isRootLevelBook {
		coverDir = filepath.Dir(book.Filepath)
	} else {
		coverDir = book.Filepath
	}

	// Generate the cover filename: {filename}.cover.{ext}
	filename := filepath.Base(file.Filepath)
	coverBaseName := filename + ".cover"

	// Delete any existing cover with this base name (regardless of extension)
	for _, existingExt := range fileutils.CoverImageExtensions {
		existingPath := filepath.Join(coverDir, coverBaseName+existingExt)
		if _, err := os.Stat(existingPath); err == nil {
			if err := os.Remove(existingPath); err != nil {
				log.Warn("failed to remove existing cover", logger.Data{"path": existingPath, "error": err.Error()})
			}
		}
	}

	// Read the uploaded file data
	src, err := fileHeader.Open()
	if err != nil {
		return errors.WithStack(err)
	}
	defer src.Close()

	uploadedData, err := io.ReadAll(src)
	if err != nil {
		return errors.WithStack(err)
	}

	// Normalize the image to strip problematic metadata
	normalizedData, normalizedMime, _ := fileutils.NormalizeImage(uploadedData, contentType)

	// Determine final extension based on normalized MIME type
	finalExt := getExtensionFromMimeType(normalizedMime)
	if finalExt == "" {
		finalExt = ext // fallback to original extension
	}

	// Save the normalized cover
	coverFilePath := filepath.Join(coverDir, coverBaseName+finalExt)
	dst, err := os.Create(coverFilePath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer dst.Close()

	if _, err := dst.Write(normalizedData); err != nil {
		return errors.WithStack(err)
	}

	log.Info("uploaded file cover", logger.Data{
		"file_id":       file.ID,
		"cover_path":    coverFilePath,
		"normalized_to": normalizedMime,
	})

	// Update file's cover metadata with normalized MIME type
	file.CoverMimeType = &normalizedMime
	file.CoverSource = strPtr(models.DataSourceManual)

	if err := h.bookService.UpdateFile(ctx, file, UpdateFileOptions{
		Columns: []string{"cover_mime_type", "cover_source"},
	}); err != nil {
		return errors.WithStack(err)
	}

	// Update the file's cover_image_path
	coverFilename := coverBaseName + finalExt
	file.CoverImagePath = &coverFilename
	if err := h.bookService.UpdateFile(ctx, file, UpdateFileOptions{
		Columns: []string{"cover_image_path"},
	}); err != nil {
		return errors.WithStack(err)
	}

	// Reload the file
	file, err = h.bookService.RetrieveFileWithNarrators(ctx, file.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, file))
}

// isValidImageType checks if the content type is a valid image type for covers.
func isValidImageType(contentType string) bool {
	validTypes := []string{"image/jpeg", "image/png", "image/webp"}
	for _, t := range validTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// getExtensionFromMimeType returns the file extension for a given MIME type.
func getExtensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func (h *handler) bookCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get the library to determine cover aspect ratio preference
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &book.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Select the appropriate file based on the library's cover aspect ratio setting
	coverFile := selectCoverFile(book.Files, library.CoverAspectRatio)
	if coverFile == nil || coverFile.CoverImagePath == nil || *coverFile.CoverImagePath == "" {
		return errcodes.NotFound("Cover")
	}

	// Determine if this is a root-level book by checking if book.Filepath is a file
	isRootLevelBook := false
	if info, err := os.Stat(book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}

	// Determine the directory where covers are located
	var coverDir string
	if isRootLevelBook {
		// For root-level books, covers are in the same directory as the file
		coverDir = filepath.Dir(book.Filepath)
	} else {
		// For directory-based books, covers are in the book directory
		coverDir = book.Filepath
	}

	coverPath := filepath.Join(coverDir, *coverFile.CoverImagePath)
	return errors.WithStack(c.File(coverPath))
}

// selectCoverFile selects the appropriate file for cover display based on the library's
// cover aspect ratio setting.
func selectCoverFile(files []*models.File, coverAspectRatio string) *models.File {
	var bookFiles, audiobookFiles []*models.File
	for _, f := range files {
		if f.CoverImagePath == nil || *f.CoverImagePath == "" {
			continue
		}
		switch f.FileType {
		case models.FileTypeEPUB, models.FileTypeCBZ:
			bookFiles = append(bookFiles, f)
		case models.FileTypeM4B:
			audiobookFiles = append(audiobookFiles, f)
		}
	}

	switch coverAspectRatio {
	case "audiobook", "audiobook_fallback_book":
		if len(audiobookFiles) > 0 {
			return audiobookFiles[0]
		}
		if len(bookFiles) > 0 {
			return bookFiles[0]
		}
	default: // "book", "book_fallback_audiobook"
		if len(bookFiles) > 0 {
			return bookFiles[0]
		}
		if len(audiobookFiles) > 0 {
			return audiobookFiles[0]
		}
	}
	return nil
}
