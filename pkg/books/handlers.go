package books

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/shishobooks/shisho/pkg/tags"
)

// ScanOptions configures a scan operation.
// Entry points are mutually exclusive - exactly one of FileID or BookID must be set.
type ScanOptions struct {
	FileID       int  // Single file resync: file already in DB
	BookID       int  // Book resync: scan all files in book
	ForceRefresh bool // Bypass priority checks, overwrite all metadata
}

// ScanResult contains the results of a scan operation.
type ScanResult struct {
	File        *models.File // The scanned/updated file (nil if deleted)
	Book        *models.Book // The parent book (nil if deleted)
	FileDeleted bool         // True if file was deleted (no longer on disk)
	BookDeleted bool         // True if book was also deleted (was last file)
}

// Scanner defines the interface for scanning file and book metadata.
// This interface is implemented by the worker package to avoid circular dependencies.
type Scanner interface {
	Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error)
}

type handler struct {
	bookService      *Service
	libraryService   *libraries.Service
	personService    *people.Service
	searchService    *search.Service
	genreService     *genres.Service
	tagService       *tags.Service
	publisherService *publishers.Service
	imprintService   *imprints.Service
	listsService     *lists.Service
	downloadCache    *downloadcache.Cache
	pageCache        *cbzpages.Cache
	scanner          Scanner
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
		GenreIDs:  params.GenreIDs,
		TagIDs:    params.TagIDs,
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

	// Track old author person IDs for FTS re-indexing
	oldPersonIDs := make([]int, 0)
	for _, author := range book.Authors {
		oldPersonIDs = append(oldPersonIDs, author.PersonID)
	}

	// Update title
	if params.Title != nil && *params.Title != book.Title {
		book.Title = *params.Title
		book.TitleSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "title", "title_source")
		shouldOrganizeFiles = true
		// Regenerate sort title when title changes (unless sort_title_source is manual)
		if book.SortTitleSource != models.DataSourceManual {
			book.SortTitle = sortname.ForTitle(*params.Title)
			book.SortTitleSource = models.DataSourceFilepath
			opts.Columns = append(opts.Columns, "sort_title", "sort_title_source")
		}
	}

	// Update sort title
	if params.SortTitle != nil && *params.SortTitle != book.SortTitle {
		if *params.SortTitle == "" {
			// Empty string means regenerate from title
			book.SortTitle = sortname.ForTitle(book.Title)
			book.SortTitleSource = models.DataSourceFilepath
		} else {
			book.SortTitle = *params.SortTitle
			book.SortTitleSource = models.DataSourceManual
		}
		opts.Columns = append(opts.Columns, "sort_title", "sort_title_source")
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

	// Update description
	if params.Description != nil {
		// Check if description actually changed
		currentDescription := ""
		if book.Description != nil {
			currentDescription = *book.Description
		}
		if *params.Description != currentDescription {
			if *params.Description == "" {
				book.Description = nil
			} else {
				book.Description = params.Description
			}
			book.DescriptionSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "description", "description_source")
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
		for i, authorInput := range params.Authors {
			if authorInput.Name == "" {
				continue
			}
			person, err := h.personService.FindOrCreatePerson(ctx, authorInput.Name, book.LibraryID)
			if err != nil {
				log.Error("failed to find/create person", logger.Data{"author": authorInput.Name, "error": err.Error()})
				continue
			}
			author := &models.Author{
				BookID:    book.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
				Role:      authorInput.Role,
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

	// Update genres
	genresChanged := false
	if params.Genres != nil {
		genresChanged = true

		// Track old genre IDs for FTS re-indexing
		oldGenreIDs := make([]int, 0)
		for _, bg := range book.BookGenres {
			oldGenreIDs = append(oldGenreIDs, bg.GenreID)
		}

		// Delete existing genre associations
		if err := h.bookService.DeleteBookGenres(ctx, book.ID); err != nil {
			return errors.WithStack(err)
		}

		// Create new genre associations and track new genre IDs
		newGenreIDs := make([]int, 0)
		for _, genreName := range params.Genres {
			if genreName == "" {
				continue
			}
			genreRecord, err := h.genreService.FindOrCreateGenre(ctx, genreName, book.LibraryID)
			if err != nil {
				log.Error("failed to find/create genre", logger.Data{"genre": genreName, "error": err.Error()})
				continue
			}
			newGenreIDs = append(newGenreIDs, genreRecord.ID)
			bookGenre := &models.BookGenre{
				BookID:  book.ID,
				GenreID: genreRecord.ID,
			}
			if err := h.bookService.CreateBookGenre(ctx, bookGenre); err != nil {
				log.Error("failed to create book genre", logger.Data{"book_id": book.ID, "genre_id": genreRecord.ID, "error": err.Error()})
			}
		}

		// Update genre source to manual
		genreSource := models.DataSourceManual
		book.GenreSource = &genreSource
		opts.Columns = append(opts.Columns, "genre_source")

		// Cleanup orphaned genres after deletion
		if _, err := h.genreService.CleanupOrphanedGenres(ctx); err != nil {
			log.Warn("failed to cleanup orphaned genres", logger.Data{"error": err.Error()})
		}

		// Re-index old genres (some may have been deleted/orphaned)
		for _, genreID := range oldGenreIDs {
			genre, err := h.genreService.RetrieveGenre(ctx, genres.RetrieveGenreOptions{ID: &genreID})
			if err == nil {
				if err := h.searchService.IndexGenre(ctx, genre); err != nil {
					log.Warn("failed to update search index for old genre", logger.Data{"genre_id": genreID, "error": err.Error()})
				}
			}
		}

		// Index new genres (including newly created ones)
		for _, genreID := range newGenreIDs {
			genre, err := h.genreService.RetrieveGenre(ctx, genres.RetrieveGenreOptions{ID: &genreID})
			if err == nil {
				if err := h.searchService.IndexGenre(ctx, genre); err != nil {
					log.Warn("failed to update search index for new genre", logger.Data{"genre_id": genreID, "error": err.Error()})
				}
			}
		}
	}

	// Update tags
	tagsChanged := false
	if params.Tags != nil {
		tagsChanged = true

		// Track old tag IDs for FTS re-indexing
		oldTagIDs := make([]int, 0)
		for _, bt := range book.BookTags {
			oldTagIDs = append(oldTagIDs, bt.TagID)
		}

		// Delete existing tag associations
		if err := h.bookService.DeleteBookTags(ctx, book.ID); err != nil {
			return errors.WithStack(err)
		}

		// Create new tag associations and track new tag IDs
		newTagIDs := make([]int, 0)
		for _, tagName := range params.Tags {
			if tagName == "" {
				continue
			}
			tagRecord, err := h.tagService.FindOrCreateTag(ctx, tagName, book.LibraryID)
			if err != nil {
				log.Error("failed to find/create tag", logger.Data{"tag": tagName, "error": err.Error()})
				continue
			}
			newTagIDs = append(newTagIDs, tagRecord.ID)
			bookTag := &models.BookTag{
				BookID: book.ID,
				TagID:  tagRecord.ID,
			}
			if err := h.bookService.CreateBookTag(ctx, bookTag); err != nil {
				log.Error("failed to create book tag", logger.Data{"book_id": book.ID, "tag_id": tagRecord.ID, "error": err.Error()})
			}
		}

		// Update tag source to manual
		tagSource := models.DataSourceManual
		book.TagSource = &tagSource
		opts.Columns = append(opts.Columns, "tag_source")

		// Cleanup orphaned tags after deletion
		if _, err := h.tagService.CleanupOrphanedTags(ctx); err != nil {
			log.Warn("failed to cleanup orphaned tags", logger.Data{"error": err.Error()})
		}

		// Re-index old tags (some may have been deleted/orphaned)
		for _, tagID := range oldTagIDs {
			tag, err := h.tagService.RetrieveTag(ctx, tags.RetrieveTagOptions{ID: &tagID})
			if err == nil {
				if err := h.searchService.IndexTag(ctx, tag); err != nil {
					log.Warn("failed to update search index for old tag", logger.Data{"tag_id": tagID, "error": err.Error()})
				}
			}
		}

		// Index new tags (including newly created ones)
		for _, tagID := range newTagIDs {
			tag, err := h.tagService.RetrieveTag(ctx, tags.RetrieveTagOptions{ID: &tagID})
			if err == nil {
				if err := h.searchService.IndexTag(ctx, tag); err != nil {
					log.Warn("failed to update search index for new tag", logger.Data{"tag_id": tagID, "error": err.Error()})
				}
			}
		}
	}

	// Silence unused variable warnings
	_ = genresChanged
	_ = tagsChanged

	// Update the model first (without file organization).
	err = h.bookService.UpdateBook(ctx, book, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload the model with all relations (including newly created authors/series/genres/tags)
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

	// Update FTS index for affected people (old and new)
	if authorsChanged {
		// Re-index new people (they may be newly created)
		for _, author := range book.Authors {
			if author.Person != nil {
				if err := h.searchService.IndexPerson(ctx, author.Person); err != nil {
					log.Warn("failed to update search index for new person", logger.Data{"person_id": author.PersonID, "error": err.Error()})
				}
			}
		}
	}

	// Cleanup orphaned records
	if authorsChanged {
		// Get list of orphaned people before deleting them (for FTS cleanup)
		orphanedPersonIDs := make([]int, 0)
		for _, personID := range oldPersonIDs {
			// Check if this old person is still associated with any books
			isOrphaned := true
			for _, author := range book.Authors {
				if author.PersonID == personID {
					isOrphaned = false
					break
				}
			}
			if isOrphaned {
				orphanedPersonIDs = append(orphanedPersonIDs, personID)
			}
		}

		if _, err := h.personService.CleanupOrphanedPeople(ctx); err != nil {
			log.Warn("failed to cleanup orphaned people", logger.Data{"error": err.Error()})
		}

		// Remove orphaned people from FTS index
		for _, personID := range orphanedPersonIDs {
			if err := h.searchService.DeleteFromPersonIndex(ctx, personID); err != nil {
				log.Warn("failed to remove orphaned person from search index", logger.Data{"person_id": personID, "error": err.Error()})
			}
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

	// Handle file role change
	if params.FileRole != nil && *params.FileRole != file.FileRole {
		oldRole := file.FileRole
		newRole := *params.FileRole

		// When upgrading from supplement to main, validate file type is supported
		if oldRole == models.FileRoleSupplement && newRole == models.FileRoleMain {
			supportedTypes := map[string]bool{
				models.FileTypeCBZ:  true,
				models.FileTypeEPUB: true,
				models.FileTypeM4B:  true,
			}
			if !supportedTypes[file.FileType] {
				return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("cannot upgrade to main file: file type '%s' is not supported as a main file (only cbz, epub, m4b)", file.FileType))
			}
		}

		// When downgrading from main to supplement, clear all main-file-only metadata
		if oldRole == models.FileRoleMain && newRole == models.FileRoleSupplement {
			// Delete cover image file if it exists
			if file.CoverImagePath != nil {
				if err := os.Remove(*file.CoverImagePath); err != nil && !os.IsNotExist(err) {
					log.Warn("failed to delete cover image on downgrade", logger.Data{"error": err.Error(), "path": *file.CoverImagePath})
				}
			}

			// Clear cover fields
			file.CoverImagePath = nil
			file.CoverMimeType = nil
			file.CoverSource = nil
			file.CoverPage = nil
			opts.Columns = append(opts.Columns, "cover_image_path", "cover_mime_type", "cover_source", "cover_page")

			// Clear audiobook fields
			file.AudiobookDurationSeconds = nil
			file.AudiobookBitrateBps = nil
			opts.Columns = append(opts.Columns, "audiobook_duration_seconds", "audiobook_bitrate_bps")

			// Clear publisher/imprint
			file.PublisherID = nil
			file.PublisherSource = nil
			file.ImprintID = nil
			file.ImprintSource = nil
			opts.Columns = append(opts.Columns, "publisher_id", "publisher_source", "imprint_id", "imprint_source")

			// Clear release date
			file.ReleaseDate = nil
			file.ReleaseDateSource = nil
			opts.Columns = append(opts.Columns, "release_date", "release_date_source")

			// Clear URL
			file.URL = nil
			file.URLSource = nil
			opts.Columns = append(opts.Columns, "url", "url_source")

			// Clear narrator source (narrators will be handled separately)
			file.NarratorSource = nil
			opts.Columns = append(opts.Columns, "narrator_source")

			// Clear identifier source
			file.IdentifierSource = nil
			opts.Columns = append(opts.Columns, "identifier_source")

			// Delete narrators for this file
			_, err := h.bookService.DeleteNarratorsForFile(ctx, file.ID)
			if err != nil {
				log.Warn("failed to delete narrators on downgrade", logger.Data{"error": err.Error()})
			}

			// Delete identifiers for this file
			_, err = h.bookService.DeleteIdentifiersForFile(ctx, file.ID)
			if err != nil {
				log.Warn("failed to delete identifiers on downgrade", logger.Data{"error": err.Error()})
			}

			// Delete sidecar file
			sidecarPath := sidecar.FileSidecarPath(file.Filepath)
			if err := os.Remove(sidecarPath); err != nil && !os.IsNotExist(err) {
				log.Warn("failed to delete sidecar on downgrade", logger.Data{"error": err.Error(), "path": sidecarPath})
			}
		}

		file.FileRole = newRole
		opts.Columns = append(opts.Columns, "file_role")
	}

	// Track old narrator person IDs for FTS re-indexing
	oldNarratorPersonIDs := make([]int, 0)
	for _, narrator := range file.Narrators {
		oldNarratorPersonIDs = append(oldNarratorPersonIDs, narrator.PersonID)
	}

	// Update narrators
	if params.Narrators != nil {
		narratorsChanged = true
		file.NarratorSource = strPtr(models.DataSourceManual)
		opts.Columns = append(opts.Columns, "narrator_source")

		// Delete existing narrator associations
		if _, err := h.bookService.DeleteNarratorsForFile(ctx, file.ID); err != nil {
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

			// Use file.Name for title if available, otherwise book.Title
			title := book.Title
			if file.Name != nil && *file.Name != "" {
				title = *file.Name
			}

			// Generate organized name options
			organizeOpts := fileutils.OrganizedNameOptions{
				AuthorNames:   authorNames,
				NarratorNames: narratorNames,
				Title:         title,
				FileType:      file.FileType,
			}

			// Rename the file
			// Use RenameOrganizedFileForSupplement to avoid renaming the book sidecar.
			// File-level changes (narrator, name) should not affect the book's sidecar -
			// only book-level changes (title, author) should rename the book sidecar.
			newPath, err := fileutils.RenameOrganizedFileOnly(file.Filepath, organizeOpts)
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
				// Update cover path if it exists (covers are renamed by rename function)
				if file.CoverImagePath != nil {
					// CoverImagePath stores just the filename, so extract Base from the computed path
					newCoverPath := filepath.Base(fileutils.ComputeNewCoverPath(*file.CoverImagePath, newPath))
					file.CoverImagePath = &newCoverPath
					opts.Columns = append(opts.Columns, "cover_image_path")
				}
				file.Filepath = newPath
				opts.Columns = append(opts.Columns, "filepath")
			}
		}
	}

	// Handle name update
	nameChanged := false
	if params.Name != nil {
		currentName := ""
		if file.Name != nil {
			currentName = *file.Name
		}
		if *params.Name != currentName {
			nameChanged = true
			if *params.Name == "" {
				file.Name = nil
				file.NameSource = nil
			} else {
				file.Name = params.Name
				file.NameSource = strPtr(models.DataSourceManual)
			}
			opts.Columns = append(opts.Columns, "name", "name_source")
		}
	}

	// Reorganize file if name changed and library has OrganizeFileStructure enabled
	if nameChanged && library.OrganizeFileStructure {
		// Get author names from the book
		authorNames := make([]string, 0, len(book.Authors))
		for _, a := range book.Authors {
			if a.Person != nil {
				authorNames = append(authorNames, a.Person.Name)
			}
		}

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

		// Generate organized name options
		organizeOpts := fileutils.OrganizedNameOptions{
			AuthorNames:   authorNames,
			NarratorNames: narratorNames,
			Title:         title,
			FileType:      file.FileType,
		}

		// Rename the file
		// Use RenameOrganizedFileForSupplement to avoid renaming the book sidecar.
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
			if file.CoverImagePath != nil {
				// CoverImagePath stores just the filename, so extract Base from the computed path
				newCoverPath := filepath.Base(fileutils.ComputeNewCoverPath(*file.CoverImagePath, newPath))
				file.CoverImagePath = &newCoverPath
				opts.Columns = append(opts.Columns, "cover_image_path")
			}
			file.Filepath = newPath
			opts.Columns = append(opts.Columns, "filepath")
		}
	}

	// Update URL
	if params.URL != nil {
		currentURL := ""
		if file.URL != nil {
			currentURL = *file.URL
		}
		if *params.URL != currentURL {
			if *params.URL == "" {
				file.URL = nil
			} else {
				file.URL = params.URL
			}
			file.URLSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "url", "url_source")
		}
	}

	// Update publisher
	if params.Publisher != nil {
		currentPublisher := ""
		if file.Publisher != nil {
			currentPublisher = file.Publisher.Name
		}
		if *params.Publisher != currentPublisher {
			if *params.Publisher == "" {
				file.PublisherID = nil
				file.Publisher = nil
			} else {
				publisher, err := h.publisherService.FindOrCreatePublisher(ctx, *params.Publisher, file.LibraryID)
				if err != nil {
					log.Error("failed to find/create publisher", logger.Data{"publisher": *params.Publisher, "error": err.Error()})
				} else {
					file.PublisherID = &publisher.ID
					file.Publisher = publisher
				}
			}
			file.PublisherSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "publisher_id", "publisher_source")
		}
	}

	// Update imprint
	if params.Imprint != nil {
		currentImprint := ""
		if file.Imprint != nil {
			currentImprint = file.Imprint.Name
		}
		if *params.Imprint != currentImprint {
			if *params.Imprint == "" {
				file.ImprintID = nil
				file.Imprint = nil
			} else {
				imprint, err := h.imprintService.FindOrCreateImprint(ctx, *params.Imprint, file.LibraryID)
				if err != nil {
					log.Error("failed to find/create imprint", logger.Data{"imprint": *params.Imprint, "error": err.Error()})
				} else {
					file.ImprintID = &imprint.ID
					file.Imprint = imprint
				}
			}
			file.ImprintSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "imprint_id", "imprint_source")
		}
	}

	// Update release date
	if params.ReleaseDate != nil {
		var currentReleaseDate string
		if file.ReleaseDate != nil {
			currentReleaseDate = file.ReleaseDate.Format(time.RFC3339)
		}
		if *params.ReleaseDate != currentReleaseDate {
			if *params.ReleaseDate == "" {
				file.ReleaseDate = nil
			} else {
				// Try parsing the date
				parsedDate, err := time.Parse("2006-01-02", *params.ReleaseDate)
				if err != nil {
					// Try RFC3339 format as well
					parsedDate, err = time.Parse(time.RFC3339, *params.ReleaseDate)
				}
				if err != nil {
					log.Error("failed to parse release date", logger.Data{"release_date": *params.ReleaseDate, "error": err.Error()})
				} else {
					file.ReleaseDate = &parsedDate
				}
			}
			file.ReleaseDateSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "release_date", "release_date_source")
		}
	}

	// Update identifiers
	if params.Identifiers != nil {
		// Delete existing identifiers
		if err := h.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
			return errors.WithStack(err)
		}
		// Create new identifiers
		for _, id := range *params.Identifiers {
			fileID := &models.FileIdentifier{
				FileID: file.ID,
				Type:   id.Type,
				Value:  id.Value,
				Source: models.DataSourceManual,
			}
			if err := h.bookService.CreateFileIdentifier(ctx, fileID); err != nil {
				log.Error("failed to create file identifier", logger.Data{"file_id": file.ID, "type": id.Type, "error": err.Error()})
			}
		}
		file.IdentifierSource = strPtr(models.DataSourceManual)
		opts.Columns = append(opts.Columns, "identifier_source")
	}

	// Update the file
	if err := h.bookService.UpdateFile(ctx, file, opts); err != nil {
		return errors.WithStack(err)
	}

	// Reload the file with narrators
	file, err = h.bookService.RetrieveFileWithRelations(ctx, file.ID)
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

	// Update FTS index for affected people (new narrators)
	if narratorsChanged {
		// Re-index new people (they may be newly created)
		for _, narrator := range file.Narrators {
			if narrator.Person != nil {
				if err := h.searchService.IndexPerson(ctx, narrator.Person); err != nil {
					log.Warn("failed to update search index for new person", logger.Data{"person_id": narrator.PersonID, "error": err.Error()})
				}
			}
		}
	}

	// Cleanup orphaned people
	if narratorsChanged {
		// Get list of orphaned people before deleting them (for FTS cleanup)
		orphanedPersonIDs := make([]int, 0)
		for _, personID := range oldNarratorPersonIDs {
			// Check if this old person is still associated with any files as narrator
			isOrphaned := true
			for _, narrator := range file.Narrators {
				if narrator.PersonID == personID {
					isOrphaned = false
					break
				}
			}
			if isOrphaned {
				orphanedPersonIDs = append(orphanedPersonIDs, personID)
			}
		}

		if _, err := h.personService.CleanupOrphanedPeople(ctx); err != nil {
			log.Warn("failed to cleanup orphaned people", logger.Data{"error": err.Error()})
		}

		// Remove orphaned people from FTS index
		for _, personID := range orphanedPersonIDs {
			if err := h.searchService.DeleteFromPersonIndex(ctx, personID); err != nil {
				log.Warn("failed to remove orphaned person from search index", logger.Data{"person_id": personID, "error": err.Error()})
			}
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

	// Cover filename is stored in CoverImagePath, or fallback to {filename}.cover.{ext}
	var coverPath string
	if file.CoverImagePath != nil && *file.CoverImagePath != "" {
		coverPath = filepath.Join(coverDir, *file.CoverImagePath)
	} else {
		filename := filepath.Base(file.Filepath)
		coverPath = filepath.Join(coverDir, filename+".cover"+file.CoverExtension())
	}

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
	file, err = h.bookService.RetrieveFileWithRelations(ctx, file.ID)
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

// downloadFile handles downloading a file with generated metadata embedded.
func (h *handler) downloadFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Fetch the file with its book
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Supplements download as-is, no processing
	if file.FileRole == models.FileRoleSupplement {
		return h.downloadOriginalFile(c)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get the full book with relations for generation
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Find the file with all relations from the book's files (includes identifiers for fingerprinting)
	var fileWithRelations *models.File
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}
	if fileWithRelations == nil {
		fileWithRelations = file // Fallback to original file if not found
	}

	// Check if the source file exists
	if _, err := os.Stat(fileWithRelations.Filepath); os.IsNotExist(err) {
		return errcodes.NotFound("Source file not found on disk")
	}

	// Try to generate/get from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerate(ctx, book, fileWithRelations)
	if err != nil {
		// Check if it's a "not implemented" error for M4B/CBZ
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			if errors.Is(genErr.Err, filegen.ErrNotImplemented) {
				// Return a specific error that tells the user this format isn't supported yet
				return errcodes.ValidationError("File generation for " + file.FileType + " is not yet supported. Use 'Download Original' instead.")
			}
			// Other generation error - return details
			return errcodes.ValidationError("Failed to generate file: " + genErr.Message)
		}
		return errors.WithStack(err)
	}

	// Set content disposition for download with the formatted filename
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+downloadFilename+"\"")

	return c.File(cachedPath)
}

// downloadOriginalFile handles downloading the original file without any modifications.
func (h *handler) downloadOriginalFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
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

	// Check if the file exists
	if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
		return errcodes.NotFound("File not found on disk")
	}

	// Set content disposition for download with the original filename
	filename := filepath.Base(file.Filepath)
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	return c.File(file.Filepath)
}

// downloadKepubFile handles downloading a file converted to KePub format.
// KePub conversion is only supported for EPUB and CBZ files.
func (h *handler) downloadKepubFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
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

	// Get the full book with relations for generation
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &file.BookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Find the file with all relations from the book's files (includes identifiers for fingerprinting)
	var fileWithRelations *models.File
	for _, f := range book.Files {
		if f.ID == file.ID {
			fileWithRelations = f
			break
		}
	}
	if fileWithRelations == nil {
		fileWithRelations = file // Fallback to original file if not found
	}

	// Check if the source file exists
	if _, err := os.Stat(fileWithRelations.Filepath); os.IsNotExist(err) {
		return errcodes.NotFound("Source file not found on disk")
	}

	// Try to generate/get from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerateKepub(ctx, book, fileWithRelations)
	if err != nil {
		// Check if this file type doesn't support KePub conversion
		if errors.Is(err, filegen.ErrKepubNotSupported) {
			return errcodes.ValidationError("KePub conversion is not supported for " + file.FileType + " files")
		}
		// Check for other generation errors
		var genErr *filegen.GenerationError
		if errors.As(err, &genErr) {
			return errcodes.ValidationError("Failed to generate KePub file: " + genErr.Message)
		}
		return errors.WithStack(err)
	}

	// Set content disposition for download with the formatted filename
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+downloadFilename+"\"")

	return c.File(cachedPath)
}

func (h *handler) resyncFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Bind params
	params := ResyncPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the file to check library access
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

	// Perform resync
	result, err := h.scanner.Scan(ctx, ScanOptions{
		FileID:       id,
		ForceRefresh: params.Refresh,
	})
	if err != nil {
		log.Error("failed to resync file", logger.Data{"file_id": id, "error": err.Error()})
		return errcodes.ValidationError(err.Error())
	}

	// Handle deletion case
	if result.FileDeleted {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"file_deleted": true,
			"book_deleted": result.BookDeleted,
		})
	}

	return errors.WithStack(c.JSON(http.StatusOK, result.File))
}

func (h *handler) resyncBook(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	// Bind params
	params := ResyncPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the book to check library access
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

	// Perform resync
	result, err := h.scanner.Scan(ctx, ScanOptions{
		BookID:       id,
		ForceRefresh: params.Refresh,
	})
	if err != nil {
		log.Error("failed to resync book", logger.Data{"book_id": id, "error": err.Error()})
		return errcodes.ValidationError(err.Error())
	}

	// Handle deletion case
	if result.BookDeleted {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"book_deleted": true,
		})
	}

	return errors.WithStack(c.JSON(http.StatusOK, result.Book))
}

func (h *handler) getPage(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	pageNum, err := strconv.Atoi(c.Param("pageNum"))
	if err != nil {
		return errcodes.ValidationError("Invalid page number")
	}

	// Retrieve file with access check
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Only CBZ files have pages
	if file.FileType != models.FileTypeCBZ {
		return errcodes.ValidationError("Only CBZ files have pages")
	}

	// Validate page number against page count
	if file.PageCount != nil && pageNum >= *file.PageCount {
		return errcodes.NotFound("Page")
	}
	if pageNum < 0 {
		return errcodes.NotFound("Page")
	}

	// Get or extract the page
	cachedPath, mimeType, err := h.pageCache.GetPage(file.Filepath, file.ID, pageNum)
	if err != nil {
		return errors.WithStack(err)
	}

	// Set cache headers (cache for 1 year since page content doesn't change)
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Response().Header().Set("Content-Type", mimeType)

	return c.File(cachedPath)
}

// streamFile streams an M4B audio file with support for Range headers (seeking).
// This endpoint enables audio playback with seek functionality in the browser.
func (h *handler) streamFile(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Retrieve file
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Only M4B files can be streamed
	if file.FileType != models.FileTypeM4B {
		return errcodes.NotFound("File")
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Check if file exists on disk
	if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
		return errcodes.NotFound("File not found on disk")
	}

	// Set Accept-Ranges header to indicate we support range requests
	c.Response().Header().Set("Accept-Ranges", "bytes")
	c.Response().Header().Set("Content-Type", "audio/mp4")

	// Check for Range header
	rangeHeader := c.Request().Header.Get("Range")
	if rangeHeader == "" {
		// No range requested - serve full file
		return c.File(file.Filepath)
	}

	// Parse Range header (format: "bytes=start-end")
	return h.serveRangeRequest(c, file.Filepath, rangeHeader)
}

// serveRangeRequest handles HTTP Range requests for partial content delivery.
func (h *handler) serveRangeRequest(c echo.Context, filePath, rangeHeader string) error {
	// Open the file
	f, err := os.Open(filePath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()

	// Get file size
	fileInfo, err := f.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	fileSize := fileInfo.Size()

	// Parse the range header (expecting format: "bytes=start-end")
	var start, end int64
	_, err = fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
	if err != nil {
		// Try parsing just start (e.g., "bytes=0-")
		_, err = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
		if err != nil {
			return errcodes.ValidationError("Invalid Range header")
		}
		end = fileSize - 1
	}

	// Validate range
	if start < 0 || start >= fileSize || end < start || end >= fileSize {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
		return c.NoContent(http.StatusRequestedRangeNotSatisfiable)
	}

	// Calculate content length
	contentLength := end - start + 1

	// Set response headers
	c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	c.Response().Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))

	// Seek to start position
	_, err = f.Seek(start, io.SeekStart)
	if err != nil {
		return errors.WithStack(err)
	}

	// Create a limited reader for the requested range
	limitedReader := io.LimitReader(f, contentLength)

	// Stream the content with 206 Partial Content status
	return c.Stream(http.StatusPartialContent, "audio/mp4", limitedReader)
}

func (h *handler) bookLists(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Verify book exists and user has library access
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.NotFound("Book")
	}

	bookLists, err := h.listsService.GetBookLists(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, bookLists))
}

func (h *handler) updateBookLists(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Verify book exists and user has library access
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.NotFound("Book")
	}

	// Parse payload
	params := lists.UpdateBookListsPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Update the book's list memberships
	err = h.listsService.UpdateBookListMemberships(ctx, id, user.ID, params.ListIDs)
	if err != nil {
		return errors.WithStack(err)
	}

	// Return updated lists
	bookLists, err := h.listsService.GetBookLists(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, bookLists))
}
