package books

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/uptrace/bun"
)

// ScanOptions configures a scan operation.
// Entry points are mutually exclusive - exactly one of FileID or BookID must be set.
type ScanOptions struct {
	FileID       int  // Single file resync: file already in DB
	BookID       int  // Book resync: scan all files in book
	ForceRefresh bool // Bypass priority checks, overwrite all metadata
	SkipPlugins  bool // Skip enricher plugins, use only file-embedded metadata
	Reset        bool // Wipe all metadata before scanning (reset to file-only state)
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

// ErrFileUnreadable marks a scan error for a file that exists on disk but
// could not be parsed, such as a corrupt EPUB. The scanner wraps its parse
// failure with it so the resync routes can report it as a 422 the user can
// act on, while any other scan failure is a server fault.
var ErrFileUnreadable = errors.New("file could not be parsed")

type handler struct {
	config             *config.Config
	bookService        *Service
	libraryService     *libraries.Service
	personService      *people.Service
	searchService      *search.Service
	genreService       *genres.Service
	tagService         *tags.Service
	publisherService   *publishers.Service
	listsService       *lists.Service
	settingsService    *settings.Service
	appSettingsService *appsettings.Service
	downloadCache      *downloadcache.Cache
	pageCache          *cbzpages.Cache
	pdfPageCache       *pdfpages.Cache
	scanner            Scanner
	pluginManager      pluginManager
}

// pluginManager defines the interface for plugin operations needed by the books handler.
type pluginManager interface {
	RegisteredFileExtensions() map[string]struct{}
}

func seriesNumberGroupChangedForOrganization(oldMembership *models.BookSeries, newMembership *SeriesInput) bool {
	var oldStart, oldEnd *float64
	var oldUnit *string
	if oldMembership != nil {
		oldStart = oldMembership.SeriesNumber
		oldEnd = oldMembership.SeriesNumberEnd
		oldUnit = oldMembership.SeriesNumberUnit
	}
	var newStart, newEnd *float64
	var newUnit *string
	if newMembership != nil {
		newStart = newMembership.Number
		newEnd = newMembership.NumberEnd
		newUnit = newMembership.SeriesNumberUnit
	}
	return !equalOptionalFloat(oldStart, newStart) || !equalOptionalFloat(oldEnd, newEnd) || !equalOptionalString(oldUnit, newUnit)
}

func equalOptionalFloat(a, b *float64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func equalOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	aspectRatio := ""
	if book.Library != nil {
		aspectRatio = book.Library.CoverAspectRatio
	}
	book.CoverCacheKey = covers.CacheKey(book.Files, aspectRatio)

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Normalize the language filter tag (e.g., "en-us" → "en-US") so the LIKE
	// query matches consistently regardless of how the user passed the tag.
	// Invalid tags are ignored (treated as no filter) rather than returning an
	// error, so a malformed query param doesn't break the gallery page.
	languageFilter := params.Language
	if languageFilter != nil && *languageFilter != "" {
		if normalized := mediafile.NormalizeLanguage(*languageFilter); normalized != nil {
			languageFilter = normalized
		} else {
			languageFilter = nil
		}
	}

	// Parse the explicit sort param. Validation errors here surface as 422
	// so a typo in a stored URL doesn't silently fall back to the default.
	var explicitSort []sortspec.SortLevel
	if params.Sort != "" {
		parsed, err := sortspec.Parse(params.Sort)
		if err != nil {
			return errcodes.ValidationError(err.Error())
		}
		explicitSort = parsed
	}

	reviewedFilter := params.ReviewedFilter
	if reviewedFilter == "all" {
		reviewedFilter = ""
	}

	opts := ListBooksOptions{
		Limit:          &params.Limit,
		Offset:         &params.Offset,
		LibraryID:      params.LibraryID,
		SeriesID:       params.SeriesID,
		Search:         params.Search,
		FileTypes:      params.FileTypes,
		GenreIDs:       params.GenreIDs,
		TagIDs:         params.TagIDs,
		Language:       languageFilter,
		IDs:            params.IDs,
		ReviewedFilter: reviewedFilter,
	}

	// Filter by the user's library access.
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
		opts.LibraryIDs = libraryIDs
	}

	// Resolve the sort: explicit param wins, then stored per-(user, library)
	// preference, then nil (service applies its hard-coded default). The
	// resolver is only consulted when scoped to a single library — there is
	// no natural "stored sort" for a multi-library or all-libraries listing.
	if params.LibraryID != nil {
		opts.Sort = sortspec.ResolveForLibrary(ctx, h.settingsService, user.ID, *params.LibraryID, explicitSort)
	} else {
		opts.Sort = explicitSort
	}

	books, total, err := h.bookService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, b := range books {
		aspectRatio := ""
		if b.Library != nil {
			aspectRatio = b.Library.CoverAspectRatio
		}
		b.CoverCacheKey = covers.CacheKey(b.Files, aspectRatio)
	}

	resp := ListBooksResponse{Items: books, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	// Bind params.
	params := UpdateBookPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := validateSeriesInputs(params.Series); err != nil {
		return errcodes.ValidationError(err.Error())
	}

	// Fetch the book.
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	// The edit can change the Book's title, authors, series, and file paths,
	// which books_fts and the series_fts rows of every Series the Book is in,
	// or was in, copy. Collect those Series before relinking.
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: []int{book.ID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Keep track of what's been changed.
	opts := UpdateBookOptions{Columns: []string{}}
	authorsChanged := false
	seriesChanged := false
	shouldOrganizeFiles := false

	// Update title
	if params.Title != nil && *params.Title != book.Title {
		oldTitle := book.Title
		newTitle := *params.Title
		book.Title = newTitle
		book.TitleSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "title", "title_source")
		shouldOrganizeFiles = true
		// Regenerate sort title when title changes (unless sort_title_source is manual)
		if book.SortTitleSource != models.DataSourceManual {
			book.SortTitle = sortname.ForTitle(newTitle)
			book.SortTitleSource = models.DataSourceFilepath
			opts.Columns = append(opts.Columns, "sort_title", "sort_title_source")
		}
		// Sync file.Name on each main file whose current Name is unset or
		// matches the old title (trim + case-insensitive). Custom filenames
		// that deliberately differ from the book title are preserved.
		for _, f := range book.Files {
			if f.FileRole != models.FileRoleMain {
				continue
			}
			currentEmpty := f.Name == nil || *f.Name == ""
			currentMatches := false
			if f.Name != nil {
				currentMatches = strings.EqualFold(strings.TrimSpace(*f.Name), strings.TrimSpace(oldTitle))
			}
			if !currentEmpty && !currentMatches {
				continue
			}
			nameCopy := newTitle
			manualSource := models.DataSourceManual
			f.Name = &nameCopy
			f.NameSource = &manualSource
			if err := h.bookService.UpdateFile(ctx, f, UpdateFileOptions{Columns: []string{"name", "name_source"}}); err != nil {
				log.Warn("failed to update file name on title change", logger.Data{"file_id": f.ID, "error": err.Error()})
			}
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
		// Check if description actually changed (strip HTML for clean display)
		sanitizedDescription := htmlutil.StripTags(*params.Description)
		currentDescription := ""
		if book.Description != nil {
			currentDescription = *book.Description
		}
		if sanitizedDescription != currentDescription {
			if sanitizedDescription == "" {
				book.Description = nil
			} else {
				book.Description = &sanitizedDescription
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

		// Create new author associations. A Person is listed once per role:
		// the unique index treats NULL roles as distinct, so it would not
		// stop a second generic row for the same Person.
		type authorKey struct {
			personID int
			role     string
		}
		seenAuthors := make(map[authorKey]bool, len(params.Authors))
		// sortOrder counts stored rows, so a skipped entry leaves no gap.
		sortOrder := 0
		for _, authorInput := range params.Authors {
			if authorInput.Name == "" {
				continue
			}
			person, err := h.personService.FindOrCreatePerson(ctx, authorInput.Name, book.LibraryID)
			if err != nil {
				log.Error("failed to find/create person", logger.Data{"author": authorInput.Name, "error": err.Error()})
				continue
			}
			// A nil role and an empty-string role share one key, so both
			// count as the generic author entry.
			key := authorKey{personID: person.ID}
			if authorInput.Role != nil {
				key.role = *authorInput.Role
			}
			if seenAuthors[key] {
				continue
			}
			seenAuthors[key] = true
			author := &models.Author{
				BookID:    book.ID,
				PersonID:  person.ID,
				SortOrder: sortOrder + 1,
				Role:      authorInput.Role,
			}
			if err := h.bookService.CreateAuthor(ctx, author); err != nil {
				log.Error("failed to create author", logger.Data{"book_id": book.ID, "person_id": person.ID, "error": err.Error()})
				continue
			}
			sortOrder++
		}
	}

	// Update series
	if params.Series != nil {
		seriesChanged = true
		// Membership provenance follows the Edit form convention for genres and
		// tags: the whole collection becomes manual, including an emptied one.
		seriesSource := models.DataSourceManual
		book.SeriesSource = &seriesSource
		opts.Columns = append(opts.Columns, "series_source")

		// Check if series number changed for CBZ files (triggers file organization)
		hasCBZFiles := false
		for _, file := range book.Files {
			if file.FileType == models.FileTypeCBZ {
				hasCBZFiles = true
				break
			}
		}
		if hasCBZFiles {
			var oldMembership *models.BookSeries
			if len(book.BookSeries) > 0 {
				oldMembership = book.BookSeries[0]
			}
			var newMembership *SeriesInput
			if len(params.Series) > 0 {
				newMembership = &params.Series[0]
			}
			shouldOrganizeFiles = shouldOrganizeFiles || seriesNumberGroupChangedForOrganization(oldMembership, newMembership)
		}

		// Delete existing series associations
		if err := h.bookService.DeleteBookSeries(ctx, book.ID); err != nil {
			return errors.WithStack(err)
		}

		// Create new series associations. A name and its Alias resolve to one
		// Series, which a book may belong to at most once.
		attachedSeries := make(map[int]struct{}, len(params.Series))
		for _, seriesInput := range params.Series {
			if seriesInput.Name == "" {
				continue
			}
			seriesRecord, err := h.bookService.FindOrCreateSeries(ctx, seriesInput.Name, book.LibraryID, models.DataSourceManual)
			if err != nil {
				log.Error("failed to find/create series", logger.Data{"series": seriesInput.Name, "error": err.Error()})
				continue
			}
			if _, dup := attachedSeries[seriesRecord.ID]; dup {
				log.Warn("skipping series listed more than once", logger.Data{"book_id": book.ID, "series_id": seriesRecord.ID})
				continue
			}
			attachedSeries[seriesRecord.ID] = struct{}{}
			bookSeries := &models.BookSeries{
				BookID:           book.ID,
				SeriesID:         seriesRecord.ID,
				SeriesNumber:     seriesInput.Number,
				SeriesNumberEnd:  seriesInput.NumberEnd,
				SeriesNumberUnit: seriesInput.SeriesNumberUnit,
				SortOrder:        len(attachedSeries),
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

		// Cleanup orphaned genres after deletion and purge their FTS rows.
		orphanedGenreIDs, err := h.genreService.CleanupOrphanedGenres(ctx)
		if err != nil {
			log.Warn("failed to cleanup orphaned genres", logger.Data{"error": err.Error()})
		}
		for _, id := range orphanedGenreIDs {
			if err := h.searchService.DeleteFromGenreIndex(ctx, id); err != nil {
				log.Warn("failed to remove orphaned genre from search index", logger.Data{"genre_id": id, "error": err.Error()})
			}
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

		// Cleanup orphaned tags after deletion and purge their FTS rows.
		orphanedTagIDs, err := h.tagService.CleanupOrphanedTags(ctx)
		if err != nil {
			log.Warn("failed to cleanup orphaned tags", logger.Data{"error": err.Error()})
		}
		for _, id := range orphanedTagIDs {
			if err := h.searchService.DeleteFromTagIndex(ctx, id); err != nil {
				log.Warn("failed to remove orphaned tag from search index", logger.Data{"tag_id": id, "error": err.Error()})
			}
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

	// Index the book's authors, which may be newly created. The deferred
	// ReindexAffected covers the book and its series.
	if authorsChanged {
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
		orphanedPersonIDs, err := h.personService.CleanupOrphanedPeople(ctx)
		if err != nil {
			log.Warn("failed to cleanup orphaned people", logger.Data{"error": err.Error()})
		}
		for _, personID := range orphanedPersonIDs {
			if err := h.searchService.DeleteFromPersonIndex(ctx, personID); err != nil {
				log.Warn("failed to remove orphaned person from search index", logger.Data{"person_id": personID, "error": err.Error()})
			}
		}
	}
	if seriesChanged {
		orphanedSeriesIDs, err := h.bookService.CleanupOrphanedSeries(ctx)
		if err != nil {
			log.Warn("failed to cleanup orphaned series", logger.Data{"error": err.Error()})
		}
		for _, seriesID := range orphanedSeriesIDs {
			if err := h.searchService.DeleteFromSeriesIndex(ctx, seriesID); err != nil {
				log.Warn("failed to remove orphaned series from search index", logger.Data{"series_id": seriesID, "error": err.Error()})
			}
		}
	}

	// Recompute reviewed for the book — covers the cases where relations
	// changed but no source column did (e.g. series-only edits don't touch
	// any *_source column, so UpdateBook short-circuits its own recompute).
	h.bookService.RecomputeReviewedForBook(ctx, book.ID)

	aspectRatio := ""
	if book.Library != nil {
		aspectRatio = book.Library.CoverAspectRatio
	}
	book.CoverCacheKey = covers.CacheKey(book.Files, aspectRatio)

	return errors.WithStack(c.JSON(http.StatusOK, book))
}

// strPtr is a helper to get a pointer to a string.
func strPtr(s string) *string {
	return &s
}

func (h *handler) updateFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
	}

	// Bind params.
	params := UpdateFilePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the file with all relations (including Publisher for change detection)
	file, err := h.bookService.RetrieveFileWithRelations(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
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

	// The book's books_fts row lists the file's path and narrators. Narrator
	// writes commit before later validation can fail the request, so the
	// reindex is deferred to cover those error paths too.
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: []int{file.BookID}})
	defer h.searchService.ReindexAffected(ctx, affected)

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
				models.FileTypePDF:  true,
			}
			if !supportedTypes[file.FileType] {
				return errcodes.InvalidState(fmt.Sprintf("Cannot upgrade to main file: file type '%s' is not supported as a main file.", file.FileType))
			}
		}

		// When downgrading from main to supplement, clear all main-file-only metadata
		if oldRole == models.FileRoleMain && newRole == models.FileRoleSupplement {
			// Delete cover image file if it exists. The cover always lives
			// alongside the file for both root-level and directory-backed
			// books, so filepath.Dir(file.Filepath) is always correct.
			if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
				coverPath := covers.FileCoverPath(file)
				if err := os.Remove(coverPath); err != nil && !os.IsNotExist(err) {
					log.Warn("failed to delete cover image on downgrade", logger.Data{
						"error":   err.Error(),
						"path":    coverPath,
						"file_id": file.ID,
					})
				}
			}

			// Clear cover fields
			file.CoverImageFilename = nil
			file.CoverMimeType = nil
			file.CoverSource = nil
			file.CoverPage = nil
			file.IsPreferredCover = false
			opts.Columns = append(opts.Columns, "cover_image_filename", "cover_mime_type", "cover_source", "cover_page", "is_preferred_cover")

			// Clear audiobook fields
			file.AudiobookDurationSeconds = nil
			file.AudiobookBitrateBps = nil
			opts.Columns = append(opts.Columns, "audiobook_duration_seconds", "audiobook_bitrate_bps")

			// Clear publisher
			file.PublisherID = nil
			file.PublisherSource = nil
			opts.Columns = append(opts.Columns, "publisher_id", "publisher_source")

			// Clear release date
			file.ReleaseDate = nil
			file.ReleaseDateSource = nil
			opts.Columns = append(opts.Columns, "release_date", "release_date_source")

			// Clear URL
			file.URL = nil
			file.URLSource = nil
			opts.Columns = append(opts.Columns, "url", "url_source")

			// Clear abridged (language is preserved — it's intrinsic to file content)
			file.Abridged = nil
			file.AbridgedSource = nil
			opts.Columns = append(opts.Columns, "abridged", "abridged_source")

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

	// narratorNames tracks the authoritative narrator list for this file —
	// either the freshly-written names from this request, or (when narrators
	// weren't touched) the in-memory relation from the initial file load.
	// Any downstream branch that reorganizes the file must use this variable
	// so that a name-triggered reorg following a narrator update doesn't
	// fall back to stale file.Narrators.
	var narratorNames []string
	if file.FileType == models.FileTypeM4B && params.Narrators == nil {
		for _, n := range file.Narrators {
			if n.Person != nil {
				narratorNames = append(narratorNames, n.Person.Name)
			}
		}
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
		narratorNames = make([]string, 0, len(params.Narrators))
		for i, narratorName := range params.Narrators {
			if narratorName == "" {
				continue
			}
			person, err := h.personService.FindOrCreatePerson(ctx, narratorName, file.LibraryID)
			if err != nil {
				log.Error("failed to find/create person", logger.Data{"narrator": narratorName, "error": err.Error()})
				continue
			}
			// Index the person as it is attached, since it may be newly
			// created and the request can still fail further down.
			if err := h.searchService.IndexPerson(ctx, person); err != nil {
				log.Warn("failed to update search index for narrator", logger.Data{"person_id": person.ID, "error": err.Error()})
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

		// For M4B files with OrganizeFileStructure enabled, reorganize so the
		// new narrator appears in the filename/path.
		if file.FileType == models.FileTypeM4B && library.OrganizeFileStructure && len(narratorNames) > 0 {
			file = h.reorganizeFileAfterMetadataChange(ctx, library, book, file, narratorNames, &opts)
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
		// narratorNames is already authoritative — either set from the new
		// narrator request above or populated from the pre-update in-memory
		// relation (which is still current if narrators weren't touched).
		file = h.reorganizeFileAfterMetadataChange(ctx, library, book, file, narratorNames, &opts)
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
					if err := h.searchService.IndexPublisher(ctx, publisher); err != nil {
						log.Warn("failed to update search index for publisher", logger.Data{"publisher_id": publisher.ID, "error": err.Error()})
					}
				}
			}
			file.PublisherSource = strPtr(models.DataSourceManual)
			opts.Columns = append(opts.Columns, "publisher_id", "publisher_source")
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

	// Update language
	if params.Language != nil {
		currentLanguage := ""
		if file.Language != nil {
			currentLanguage = *file.Language
		}
		if *params.Language != currentLanguage {
			if *params.Language == "" {
				file.Language = nil
				file.LanguageSource = nil
			} else {
				normalized := mediafile.NormalizeLanguage(*params.Language)
				if normalized == nil {
					return errcodes.ValidationError("Invalid language tag: " + *params.Language)
				}
				file.Language = normalized
				file.LanguageSource = strPtr(models.DataSourceManual)
			}
			opts.Columns = append(opts.Columns, "language", "language_source")
		}
	}

	// Update abridged
	if params.Abridged != nil {
		switch *params.Abridged {
		case "":
			// Clear
			if file.Abridged != nil {
				file.Abridged = nil
				file.AbridgedSource = nil
				opts.Columns = append(opts.Columns, "abridged", "abridged_source")
			}
		case "true":
			if file.Abridged == nil || !*file.Abridged {
				b := true
				file.Abridged = &b
				file.AbridgedSource = strPtr(models.DataSourceManual)
				opts.Columns = append(opts.Columns, "abridged", "abridged_source")
			}
		case "false":
			if file.Abridged == nil || *file.Abridged {
				b := false
				file.Abridged = &b
				file.AbridgedSource = strPtr(models.DataSourceManual)
				opts.Columns = append(opts.Columns, "abridged", "abridged_source")
			}
		}
	}

	// Update identifiers
	if params.Identifiers != nil {
		// Reject duplicate types before any DB mutation. The DB enforces
		// UNIQUE(file_id, type); surface the contract violation explicitly
		// instead of silently dropping the second insert. The binder has already
		// trimmed id.Type via mod:"dive" + mod:"trim", so keying on the raw
		// field is equivalent to keying on the trimmed value.
		seen := make(map[string]struct{}, len(*params.Identifiers))
		for _, id := range *params.Identifiers {
			if _, dup := seen[id.Type]; dup {
				return identifiers.DuplicateTypeError(id.Type)
			}
			seen[id.Type] = struct{}{}
		}

		// Read existing identifiers so an entry whose (type, normalized value)
		// is unchanged keeps its source. Replacements and net-new entries get
		// DataSourceManual since the user explicitly applied them here.
		existingFile, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
		if err != nil {
			return errors.WithStack(err)
		}
		toInsert := make([]*models.FileIdentifier, 0, len(*params.Identifiers))
		for _, id := range *params.Identifiers {
			toInsert = append(toInsert, &models.FileIdentifier{
				FileID: file.ID,
				Type:   id.Type,
				Value:  id.Value,
				Source: models.DataSourceManual,
			})
		}
		identifiers.ReconcileSources(existingFile.Identifiers, toInsert)

		if err := h.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
			return errors.WithStack(err)
		}
		if err := h.bookService.BulkCreateFileIdentifiers(ctx, toInsert); err != nil {
			return errors.WithStack(err)
		}
		file.IdentifierSource = strPtr(models.DataSourceManual)
		opts.Columns = append(opts.Columns, "identifier_source")
	}

	// Handle is_preferred_cover
	if params.IsPreferredCover != nil {
		if *params.IsPreferredCover {
			// Validate file has a cover
			if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
				return errcodes.InvalidState("Cannot set preferred cover: file has no cover image.")
			}
			// Clear is_preferred_cover on other files of the same type category
			// in the same book. EPUB/CBZ/PDF = ebook, M4B = audiobook.
			var sameCategory []string
			switch file.FileType {
			case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF:
				sameCategory = []string{models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF}
			case models.FileTypeM4B:
				sameCategory = []string{models.FileTypeM4B}
			}
			if len(sameCategory) > 0 {
				_, err := h.bookService.DB().NewUpdate().
					TableExpr("files").
					Set("is_preferred_cover = ?", false).
					Where("book_id = ?", file.BookID).
					Where("id != ?", file.ID).
					Where("file_type IN (?)", bun.List(sameCategory)).
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			file.IsPreferredCover = true
		} else {
			file.IsPreferredCover = false
		}
		opts.Columns = append(opts.Columns, "is_preferred_cover")
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

	// Cleanup orphaned people
	if narratorsChanged {
		orphanedPersonIDs, err := h.personService.CleanupOrphanedPeople(ctx)
		if err != nil {
			log.Warn("failed to cleanup orphaned people", logger.Data{"error": err.Error()})
		}
		for _, personID := range orphanedPersonIDs {
			if err := h.searchService.DeleteFromPersonIndex(ctx, personID); err != nil {
				log.Warn("failed to remove orphaned person from search index", logger.Data{"person_id": personID, "error": err.Error()})
			}
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, file))
}

// reorganizeFileAfterMetadataChange reorganizes a file on disk after a
// file-level metadata change (name or narrator) when the library has
// OrganizeFileStructure enabled.
//
// For files that already live inside their organized folder this does an
// in-folder rename via RenameOrganizedFileOnly. For root-level files —
// which can happen when a library was previously organize=false or when
// organize hasn't finished running after a toggle — it delegates to
// OrganizeBookFiles so the file is moved into its organized folder and
// book.Filepath is kept in sync. Rename-in-place at the library root
// would otherwise leave book.Filepath pointing at a synthetic path that
// diverges from the actual file location.
//
// Returns the (possibly reloaded) file model so the caller can pick up any
// path/cover updates persisted by the book-organize path.
func (h *handler) reorganizeFileAfterMetadataChange(
	ctx context.Context,
	library *models.Library,
	book *models.Book,
	file *models.File,
	narratorNames []string,
	opts *UpdateFileOptions,
) *models.File {
	log := logger.FromContext(ctx)

	isRootLevel := false
	fileDir := filepath.Dir(file.Filepath)
	for _, lp := range library.LibraryPaths {
		if fileDir == lp.Filepath {
			isRootLevel = true
			break
		}
	}

	if isRootLevel {
		// OrganizeBookFiles reads files fresh from the DB, so flush any
		// pending in-memory updates (name, cover_image_filename, etc.) on
		// this file first. Otherwise a just-changed file.Name would not be
		// reflected in the organized path. If the subsequent organize fails
		// we roll back those DB writes so DB and disk stay consistent.
		var preFlushFile *models.File
		var flushedColumns []string
		if len(opts.Columns) > 0 {
			// Snapshot the pre-flush DB state so we can revert if organize
			// fails and DB+disk would otherwise diverge.
			snapshot, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
			if err != nil {
				log.Error("failed to snapshot file before organize", logger.Data{
					"file_id": file.ID,
					"error":   err.Error(),
				})
				return file
			}
			preFlushFile = snapshot

			if err := h.bookService.UpdateFile(ctx, file, *opts); err != nil {
				log.Error("failed to flush file updates before organize", logger.Data{
					"file_id": file.ID,
					"error":   err.Error(),
				})
				return file
			}
			// Already persisted — prevent the outer UpdateFile call from
			// writing the same columns again. Keep a copy for revert.
			flushedColumns = append([]string{}, opts.Columns...)
			opts.Columns = opts.Columns[:0]
		}
		// Delegate to book-level organize so the file is moved into its
		// organized folder and book.Filepath is updated in lockstep.
		organizeErr := h.bookService.OrganizeBookFiles(ctx, book)
		if organizeErr != nil {
			log.Error("failed to organize book files after file metadata change", logger.Data{
				"file_id": file.ID,
				"book_id": book.ID,
				"error":   organizeErr.Error(),
			})
		}

		// Reload the file so we can detect whether organize actually moved
		// it. OrganizeBookFiles logs and continues when an individual file's
		// organize fails (e.g. the source was deleted out from under us), so
		// a nil error doesn't guarantee the move happened.
		updated, reloadErr := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
		if reloadErr != nil {
			log.Error("failed to reload file after organize", logger.Data{
				"file_id": file.ID,
				"error":   reloadErr.Error(),
			})
			return file
		}

		// If the file didn't actually move (organize failed explicitly OR
		// silently), revert the pre-organize flush so the DB doesn't show
		// new metadata while disk still has the file at its old path.
		if updated.Filepath == file.Filepath {
			if preFlushFile != nil && len(flushedColumns) > 0 {
				if rbErr := h.bookService.UpdateFile(ctx, preFlushFile, UpdateFileOptions{Columns: flushedColumns}); rbErr != nil {
					log.Error("failed to revert file update after organize failure", logger.Data{
						"file_id": file.ID,
						"error":   rbErr.Error(),
					})
				}
			}
			return file
		}
		return updated
	}

	// Same-folder rename. Book.Filepath already points at the book's
	// directory for directory-backed books, so there's nothing to sync.
	authorNames := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		if a.Person != nil {
			authorNames = append(authorNames, a.Person.Name)
		}
	}
	title := book.Title
	if file.Name != nil && *file.Name != "" {
		title = *file.Name
	}
	// File-level renames intentionally omit SeriesNumber/SeriesNumberUnit:
	// GenerateOrganizedFileName clears them before generating the filename
	// since the series number is already encoded in the parent folder name.
	organizeOpts := fileutils.OrganizedNameOptions{
		AuthorNames:   authorNames,
		NarratorNames: narratorNames,
		Title:         title,
		FileType:      file.FileType,
	}
	// RenameOrganizedFileOnly leaves the book sidecar untouched — file-level
	// changes must not rename the book sidecar.
	newPath, err := fileutils.RenameOrganizedFileOnly(file.Filepath, organizeOpts)
	if err != nil {
		log.Error("failed to rename file after metadata change", logger.Data{
			"file_id": file.ID,
			"path":    file.Filepath,
			"error":   err.Error(),
		})
		// Revert the pending name/name_source DB writes so the outer
		// UpdateFile doesn't persist the new name while the file stays at
		// its old path on disk. Narrator-driven renames don't need reverting
		// — narrators are their own table rows, already persisted before
		// this call, and narrator_source reflects the user's manual intent
		// regardless of whether the filename update succeeded.
		h.revertRenameDrivenColumns(ctx, file, opts, log)
		return file
	}
	if newPath != file.Filepath {
		log.Info("renamed file after metadata change", logger.Data{
			"file_id":  file.ID,
			"old_path": file.Filepath,
			"new_path": newPath,
		})
		if file.CoverImageFilename != nil {
			newCoverPath := fileutils.ComputeNewCoverFilename(*file.CoverImageFilename, newPath)
			file.CoverImageFilename = &newCoverPath
			opts.Columns = append(opts.Columns, "cover_image_filename")
		}
		file.Filepath = newPath
		opts.Columns = append(opts.Columns, "filepath")
	}
	return file
}

// revertRenameDrivenColumns undoes in-memory file.Name / file.NameSource
// changes queued for the outer UpdateFile when a same-folder rename failed,
// so the DB doesn't end up carrying new metadata while the file sits at its
// old path on disk. Reads the pre-update values from the DB. Silently
// returns if there's nothing to revert.
func (h *handler) revertRenameDrivenColumns(
	ctx context.Context,
	file *models.File,
	opts *UpdateFileOptions,
	log logger.Logger,
) {
	hasName := containsColumn(opts.Columns, "name")
	hasNameSource := containsColumn(opts.Columns, "name_source")
	if !hasName && !hasNameSource {
		return
	}
	snapshot, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	if err != nil {
		log.Error("failed to snapshot file for rename-failure revert", logger.Data{
			"file_id": file.ID,
			"error":   err.Error(),
		})
		return
	}
	if hasName {
		file.Name = snapshot.Name
		opts.Columns = removeColumn(opts.Columns, "name")
	}
	if hasNameSource {
		file.NameSource = snapshot.NameSource
		opts.Columns = removeColumn(opts.Columns, "name_source")
	}
}

func containsColumn(cols []string, target string) bool {
	for _, c := range cols {
		if c == target {
			return true
		}
	}
	return false
}

// removeColumn returns a new slice with the first occurrence of target
// removed. Allocates a fresh backing array — does not alias cols.
// Assumes target appears at most once (guaranteed by the single-append
// contract at each column-queuing site in the handler).
func removeColumn(cols []string, target string) []string {
	out := make([]string, 0, len(cols))
	removed := false
	for _, c := range cols {
		if !removed && c == target {
			removed = true
			continue
		}
		out = append(out, c)
	}
	return out
}

func (h *handler) fileCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
	}

	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	coverPath := covers.FileCoverPath(file)

	// Stat first so a missing cover returns the errcodes 404 that the book and
	// series cover routes return, not echo.HTTPError's generic "Not Found".
	if _, err := os.Stat(coverPath); err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
	}

	c.Response().Header().Set("Cache-Control", covers.CacheControlImmutable)
	return errors.WithStack(c.File(coverPath))
}

func (h *handler) uploadFileCover(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
	}

	fileHeader, err := c.FormFile("cover")
	if err != nil {
		return coverFormFileError(err)
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
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	// Page-based formats (CBZ, PDF) derive their cover from page content and
	// cannot have it replaced by upload.
	if models.IsPageBasedFileType(file.FileType) {
		return errcodes.InvalidState("Cover upload is not supported for this file type.")
	}

	// The cover always lives next to the file — using book.Filepath here
	// would fail for root-level books where book.Filepath is a synthetic
	// path that doesn't exist on disk.
	coverDir := filepath.Dir(file.Filepath)

	// Generate the cover filename: {filename}.cover.{ext}
	filename := filepath.Base(file.Filepath)
	coverBaseName := filename + ".cover"

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

	// Normalize the image to strip problematic metadata. The full decode is
	// also the validation: a truncated upload keeps a valid header, so this
	// is the only check that catches it, and it runs before the previous
	// cover is touched.
	normalizedData, normalizedMime, err := fileutils.NormalizeImage(uploadedData, contentType)
	if err != nil {
		log.Warn("uploaded cover is not a decodable image", logger.Data{"file_id": file.ID, "content_type": contentType, "error": err.Error()})
		return errcodes.ValidationError("The uploaded file is not a decodable image")
	}

	// Determine final extension based on normalized MIME type
	finalExt := getExtensionFromMimeType(normalizedMime)
	if finalExt == "" {
		finalExt = ext // fallback to original extension
	}

	// Install the replacement atomically. Previous covers at other
	// extensions are removed only after the database names the replacement,
	// so neither a failed write nor a failed update destroys a working cover.
	coverFilePath := filepath.Join(coverDir, coverBaseName+finalExt)
	if err := fileutils.WriteFileAtomic(coverFilePath, normalizedData, 0644); err != nil {
		return errors.WithStack(err)
	}
	staleCovers := fileutils.OtherCoverExtensions(coverDir, coverBaseName, finalExt)

	log.Info("uploaded file cover", logger.Data{
		"file_id":       file.ID,
		"cover_path":    coverFilePath,
		"normalized_to": normalizedMime,
	})

	// Write the complete cover state in one column set, so a failed update
	// can never leave the row naming the old file with the new provenance.
	coverFilename := coverBaseName + finalExt
	file.CoverImageFilename = &coverFilename
	file.CoverMimeType = &normalizedMime
	file.CoverSource = strPtr(models.DataSourceManual)
	if err := h.bookService.UpdateFile(ctx, file, UpdateFileOptions{
		Columns: []string{"cover_image_filename", "cover_mime_type", "cover_source"},
	}); err != nil {
		return errors.WithStack(err)
	}
	RemoveStaleCovers(staleCovers, log)

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
	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	// Get the library to determine cover aspect ratio preference
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &book.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return covers.ServeBookCover(c, book.Files, library.CoverAspectRatio, covers.CacheControlImmutable, "Cover")
}

// downloadFile handles downloading a file with generated metadata embedded.
func (h *handler) downloadFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
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
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
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
	if err := RequireFileOnDisk(c, fileWithRelations, "Source file"); err != nil {
		return err
	}

	// Try to generate/get from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerate(ctx, book, fileWithRelations)
	if err != nil {
		// Every generation failure is a server fault; the device routes
		// fall back to the original file instead.
		return errors.WithStack(err)
	}

	httputil.SetAttachmentFilename(c.Response(), downloadFilename)
	c.Response().Header().Set("Cache-Control", "private, no-store")

	return c.File(cachedPath)
}

// downloadOriginalFile handles downloading the original file without any modifications.
func (h *handler) downloadOriginalFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
	}

	// Fetch the file
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	// Check if the file exists
	if err := RequireFileOnDisk(c, file, "File"); err != nil {
		return err
	}

	filename := filepath.Base(file.Filepath)
	httputil.SetAttachmentFilename(c.Response(), filename)
	c.Response().Header().Set("Cache-Control", "private, no-store")

	return c.File(file.Filepath)
}

// downloadKepubFile handles downloading a file converted to KePub format.
// KePub conversion is only supported for EPUB and CBZ files.
func (h *handler) downloadKepubFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
	}

	// Fetch the file with its book
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
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
	if err := RequireFileOnDisk(c, fileWithRelations, "Source file"); err != nil {
		return err
	}

	// Try to generate/get from cache
	cachedPath, downloadFilename, err := h.downloadCache.GetOrGenerateKepub(ctx, book, fileWithRelations)
	if err != nil {
		// A file type KePub cannot convert is the one failure the user can
		// work around; any other generation failure is a server fault.
		if errors.Is(err, filegen.ErrKepubNotSupported) {
			return errcodes.InvalidState("KePub conversion is not supported for " + file.FileType + " files")
		}
		return errors.WithStack(err)
	}

	httputil.SetAttachmentFilename(c.Response(), downloadFilename)
	c.Response().Header().Set("Cache-Control", "private, no-store")

	return c.File(cachedPath)
}

func (h *handler) resyncFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
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
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	// Perform resync
	forceRefresh, skipPlugins, reset := params.resolveScanMode()
	result, err := h.scanner.Scan(ctx, ScanOptions{
		FileID:       id,
		ForceRefresh: forceRefresh,
		SkipPlugins:  skipPlugins,
		Reset:        reset,
	})
	if err != nil {
		log.Error("failed to resync file", logger.Data{"file_id": id, "error": err.Error()})
		return resyncError(err)
	}

	// Handle deletion case
	if result.FileDeleted {
		return c.JSON(http.StatusOK, ResyncFileResponse{
			FileDeleted: true,
			BookDeleted: result.BookDeleted,
		})
	}

	return errors.WithStack(c.JSON(http.StatusOK, result.File))
}

func (h *handler) resyncBook(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
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
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	// Perform resync
	forceRefresh, skipPlugins, reset := params.resolveScanMode()
	result, err := h.scanner.Scan(ctx, ScanOptions{
		BookID:       id,
		ForceRefresh: forceRefresh,
		SkipPlugins:  skipPlugins,
		Reset:        reset,
	})
	if err != nil {
		log.Error("failed to resync book", logger.Data{"book_id": id, "error": err.Error()})
		return resyncError(err)
	}

	// Handle deletion case
	if result.BookDeleted {
		return c.JSON(http.StatusOK, ResyncBookResponse{
			BookDeleted: true,
		})
	}

	return errors.WithStack(c.JSON(http.StatusOK, result.Book))
}

// resyncError renders a scan failure: a file that could not be parsed is a
// 422, and anything else is a server fault. The parse error names library
// paths, so the 422 carries a fixed message; the caller logs the detail and
// the scanner records it on the file as its scan error.
func resyncError(err error) error {
	if errors.Is(err, ErrFileUnreadable) {
		return errcodes.ValidationError("The file could not be parsed. Its scan error has the details.")
	}
	return errors.WithStack(err)
}

// coverFormFileError renders a failure to read the uploaded cover. A request
// without a multipart body or without a cover part carries no cover (422). A
// failure on the server's filesystem, such as spilling a large upload to a
// temporary file, is a server fault. Anything else is a multipart body that
// is malformed or cut short, the binder's 400.
func coverFormFileError(err error) error {
	var pathErr *fs.PathError
	switch {
	case errors.Is(err, http.ErrMissingFile), errors.Is(err, http.ErrNotMultipart), errors.Is(err, http.ErrMissingBoundary):
		return errcodes.ValidationError("Cover image is required")
	case errors.Is(err, context.Canceled), errors.As(err, &pathErr):
		return errors.WithStack(err)
	}
	return errcodes.MalformedPayload()
}

func (h *handler) getPage(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
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
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	// Only CBZ and PDF files have pages
	if file.FileType != models.FileTypeCBZ && file.FileType != models.FileTypePDF {
		return errcodes.InvalidState("Only CBZ and PDF files have pages")
	}

	// Validate page number against page count
	if file.PageCount != nil && pageNum >= *file.PageCount {
		return errcodes.NotFound("Page")
	}
	if pageNum < 0 {
		return errcodes.NotFound("Page")
	}

	// Check the source before the page cache so a missing file returns the same
	// 404 as the download routes instead of a 500 on a cache miss or a stale
	// page on a cache hit.
	if err := RequireFileOnDisk(c, file, "File"); err != nil {
		return err
	}

	// Get or render the page from the appropriate cache
	var cachedPath, mimeType string
	switch file.FileType {
	case models.FileTypeCBZ:
		cachedPath, mimeType, err = h.pageCache.GetPage(file.Filepath, file.ID, pageNum)
	case models.FileTypePDF:
		cachedPath, mimeType, err = h.pdfPageCache.GetPage(file.Filepath, file.ID, pageNum)
	}
	if err != nil {
		return errors.WithStack(err)
	}

	// Cache for 1 year since page content doesn't change. The route is
	// authenticated, so private keeps shared caches from storing it. A PDF
	// page also depends on the render settings, which the URL carries as r.
	// A tab loaded before a restart with new settings asks for the old key;
	// caching the new render under that URL would show it if the settings
	// were reverted, so only the current key is cached.
	cacheControl := "private, max-age=31536000, immutable"
	if file.FileType == models.FileTypePDF && c.QueryParam("r") != h.pdfPageCache.RenderKey() {
		cacheControl = "private, no-store"
	}
	c.Response().Header().Set("Cache-Control", cacheControl)
	c.Response().Header().Set("Content-Type", mimeType)

	return c.File(cachedPath)
}

// streamFile streams an M4B audio file with support for Range headers (seeking).
// This endpoint enables audio playback with seek functionality in the browser.
func (h *handler) streamFile(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
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
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	// Check if file exists on disk
	if err := RequireFileOnDisk(c, file, "File"); err != nil {
		return err
	}

	// Set Accept-Ranges header to indicate we support range requests
	c.Response().Header().Set("Accept-Ranges", "bytes")
	c.Response().Header().Set("Content-Type", "audio/mp4")
	c.Response().Header().Set("Cache-Control", "private, no-store")

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
		if err != nil || start < 0 {
			// A malformed or suffix range ("bytes=-500") is not one this
			// handler serves. RFC 9110 lets a server ignore a Range header,
			// so serve the whole open file with 200 instead of failing.
			// Dropping the header keeps ServeContent from reading it again.
			c.Request().Header.Del("Range")
			http.ServeContent(c.Response(), c.Request(), fileInfo.Name(), fileInfo.ModTime(), f)
			return nil
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

	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
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

	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
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

// moveFiles moves files from this book to another book (or a new book).
func (h *handler) moveFiles(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse book ID from URL param
	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	// Get source book to determine library
	sourceBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, sourceBook.LibraryID); err != nil {
		return err
	}

	// Bind payload
	params := MoveFilesPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Validate files belong to this book
	for _, fileID := range params.FileIDs {
		found := false
		for _, file := range sourceBook.Files {
			if file.ID == fileID {
				found = true
				break
			}
		}
		if !found {
			return errcodes.ValidationError("File does not belong to this book")
		}
	}

	// If target book specified, verify it exists and is in same library
	if params.TargetBookID != nil {
		targetBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
			ID: params.TargetBookID,
		})
		var codeErr *errcodes.Error
		if errors.As(err, &codeErr) && codeErr.HTTPCode == http.StatusNotFound {
			return errcodes.NotFound("Target book")
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if targetBook.LibraryID != sourceBook.LibraryID {
			return errcodes.ValidationError("Target book must be in the same library")
		}
	}

	// The move changes both Books' files, may delete the source (whose
	// book_series rows CASCADE away), and may create a new Book that copies
	// the source's Series memberships.
	affectedBooks := []int{id}
	if params.TargetBookID != nil {
		affectedBooks = append(affectedBooks, *params.TargetBookID)
	}
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: affectedBooks})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Call service method
	result, err := h.bookService.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      params.FileIDs,
		TargetBookID: params.TargetBookID,
		LibraryID:    sourceBook.LibraryID,
	})
	if err != nil {
		return moveFilesError(err)
	}
	if result.TargetBook != nil {
		affected.BookIDs = append(affected.BookIDs, result.TargetBook.ID)
	}

	// Return MoveFilesResponse
	return c.JSON(http.StatusOK, MoveFilesResponse{
		TargetBook:        result.TargetBook,
		FilesMoved:        result.FilesMoved,
		SourceBookDeleted: result.SourceBookDeleted,
	})
}

// moveFilesError renders a MoveFilesToBook failure. A request it cannot carry
// out is a 422; a missing target book keeps its 404, and anything else is a
// 500.
func moveFilesError(err error) error {
	if errors.Is(err, ErrNoFilesToMove) || errors.Is(err, ErrFilesNotInLibrary) {
		return errcodes.ValidationError(err.Error())
	}
	return errors.WithStack(err)
}

// mergeBooks merges multiple books into a single target book.
func (h *handler) mergeBooks(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	// Bind payload
	params := MergeBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// A source listed twice would collect its files twice, and the file move
	// would then report them as missing.
	seenSources := make(map[int]bool, len(params.SourceBookIDs))
	for _, sourceBookID := range params.SourceBookIDs {
		if seenSources[sourceBookID] {
			return errcodes.ValidationError(fmt.Sprintf("Source book %d is listed more than once", sourceBookID))
		}
		seenSources[sourceBookID] = true
	}

	// Get target book to determine library
	targetBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &params.TargetBookID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, targetBook.LibraryID); err != nil {
		return err
	}

	// Validate source books exist and are in same library, collect all file IDs
	var allFileIDs []int
	for _, sourceBookID := range params.SourceBookIDs {
		// Skip if target book is in sources
		if sourceBookID == params.TargetBookID {
			continue
		}

		// RetrieveBook returns NotFound("Book") for a missing row, and any
		// other failure is a server fault.
		sourceBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
			ID: &sourceBookID,
		})
		if err != nil {
			return errors.WithStack(err)
		}
		if sourceBook.LibraryID != targetBook.LibraryID {
			return errcodes.ValidationError("All source books must be in the same library as target book")
		}

		// Collect file IDs from this source book
		for _, file := range sourceBook.Files {
			allFileIDs = append(allFileIDs, file.ID)
		}
	}

	// If no files to move (e.g., target was the only source), return early
	if len(allFileIDs) == 0 {
		return c.JSON(http.StatusOK, MergeBooksResponse{
			TargetBook:   targetBook,
			FilesMoved:   0,
			BooksDeleted: 0,
		})
	}

	// The merge deletes the emptied sources, whose book_series rows CASCADE
	// away, so collect their Series first.
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: append([]int{params.TargetBookID}, params.SourceBookIDs...)})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Call service method to move all files to target book
	result, err := h.bookService.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      allFileIDs,
		TargetBookID: &params.TargetBookID,
		LibraryID:    targetBook.LibraryID,
	})
	if err != nil {
		return moveFilesError(err)
	}

	// The deleted sources' People, Genres, Tags, Series, and Publishers may
	// be used nowhere else now, as after a Book delete.
	if len(result.DeletedBookIDs) > 0 {
		CleanupOrphanedEntities(ctx, log, h.orphanCleanupServices())
	}

	// Return MergeBooksResponse
	return c.JSON(http.StatusOK, MergeBooksResponse{
		TargetBook:   result.TargetBook,
		FilesMoved:   result.FilesMoved,
		BooksDeleted: len(result.DeletedBookIDs),
	})
}

// orphanCleanupServices returns the handler's services for
// CleanupOrphanedEntities.
func (h *handler) orphanCleanupServices() OrphanCleanupServices {
	return OrphanCleanupServices{
		Books:      h.bookService,
		People:     h.personService,
		Genres:     h.genreService,
		Tags:       h.tagService,
		Publishers: h.publisherService,
		Search:     h.searchService,
	}
}

// deleteBook handles DELETE /books/:id.
func (h *handler) deleteBook(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	// Parse book ID
	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return err
	}

	// Load book to get library ID
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	// Load library for deletion config
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{ID: &book.LibraryID})
	if err != nil {
		return errors.WithStack(err)
	}

	// The delete CASCADEs the book's book_series rows away, so collect its
	// Series first. The reindex drops the book's own row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Delete book and files
	result, err := h.bookService.DeleteBookAndFiles(ctx, id, library)
	if err != nil {
		return errors.WithStack(err)
	}

	CleanupOrphanedEntities(ctx, log, h.orphanCleanupServices())

	return c.JSON(http.StatusOK, DeleteBookResponse{
		FilesDeleted: result.FilesDeleted,
	})
}

// deleteFile handles DELETE /books/files/:id.
func (h *handler) deleteFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	// Parse file ID
	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return err
	}

	// Load file to get library ID and book ID
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check user has library access
	if err := auth.RequireLibraryAccessFor(c, file.LibraryID); err != nil {
		return err
	}

	// Load library
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{ID: &file.LibraryID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Build supported types map (native + plugin-registered)
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
		models.FileTypePDF:  {},
	}
	if h.pluginManager != nil {
		for ext := range h.pluginManager.RegisteredFileExtensions() {
			supportedTypes[ext] = struct{}{}
		}
	}

	// The book's books_fts row lists the file's path and narrators. When the
	// last main file goes the book is deleted too, and its book_series rows
	// CASCADE away, so collect its Series first.
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: []int{file.BookID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Delete file
	result, err := h.bookService.DeleteFileAndCleanup(ctx, id, library, supportedTypes)
	if err != nil {
		return errors.WithStack(err)
	}

	if result.BookDeleted {
		CleanupOrphanedEntities(ctx, log, h.orphanCleanupServices())
	} else {
		// The deleted file's narrators may have narrated nothing else. Only
		// the people kind runs, so a file delete does not start sweeping up
		// other kinds of orphans that the full cleanup would remove.
		CleanupOrphanedPeople(ctx, log, h.orphanCleanupServices())
	}

	// If a supplement was promoted, scan it to extract cover and update metadata
	if result.PromotedFileID != nil {
		if _, err := h.scanner.Scan(ctx, ScanOptions{FileID: *result.PromotedFileID}); err != nil {
			log.Warn("failed to scan promoted file", logger.Data{"file_id": *result.PromotedFileID, "error": err.Error()})
		}
	}

	return c.JSON(http.StatusOK, DeleteFileResponse{
		BookDeleted: result.BookDeleted,
	})
}

// deleteBooks handles POST /books/delete for bulk book deletion.
func (h *handler) deleteBooks(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	var req DeleteBooksPayload
	if err := c.Bind(&req); err != nil {
		return errors.WithStack(err)
	}

	if len(req.BookIDs) == 0 {
		return errcodes.ValidationError("book_ids is required")
	}

	// Load first book to get library (all books must be in same library)
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &req.BookIDs[0]})
	if err != nil {
		return errors.WithStack(err)
	}
	if book == nil {
		return errcodes.NotFound("Book")
	}

	// Check user has library access
	if err := auth.RequireLibraryAccessFor(c, book.LibraryID); err != nil {
		return err
	}

	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{ID: &book.LibraryID})
	if err != nil {
		return errors.WithStack(err)
	}

	// Verify all books belong to same library
	for _, bookID := range req.BookIDs[1:] {
		b, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
		if err != nil {
			return errors.WithStack(err)
		}
		if b == nil {
			return errcodes.NotFound("Book")
		}
		if b.LibraryID != library.ID {
			return errcodes.ValidationError("All books must belong to the same library")
		}
	}

	// The delete CASCADEs the books' book_series rows away, so collect their
	// Series first. The reindex drops the books' own rows.
	affected := h.searchService.CollectAffected(ctx, search.Affected{BookIDs: req.BookIDs})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Delete books
	result, err := h.bookService.DeleteBooksAndFiles(ctx, req.BookIDs, library)
	if err != nil {
		return errors.WithStack(err)
	}
	CleanupOrphanedEntities(ctx, log, h.orphanCleanupServices())

	return c.JSON(http.StatusOK, DeleteBooksResponse{
		BooksDeleted: result.BooksDeleted,
		FilesDeleted: result.FilesDeleted,
	})
}

func (h *handler) listLibraryLanguages(c echo.Context) error {
	libraryID, err := httputil.ParamID(c, "id", "Library")
	if err != nil {
		return err
	}

	languages, err := h.bookService.DistinctFileLanguages(c.Request().Context(), libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, languages)
}
