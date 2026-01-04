package series

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/sortname"
)

type handler struct {
	seriesService  *Service
	bookService    *books.Service
	libraryService *libraries.Service
	searchService  *search.Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(series.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get book count
	bookCount, err := h.seriesService.GetSeriesBookCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	response := struct {
		*models.Series
		BookCount int `json:"book_count"`
	}{series, bookCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListSeriesQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListSeriesOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
		Search:    params.Search,
	}

	// Filter by user's library access if user is in context
	if user, ok := c.Get("user").(*models.User); ok {
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
	}

	seriesList, total, err := h.seriesService.ListSeriesWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with book counts
	type SeriesWithCount struct {
		*models.Series
		BookCount int `json:"book_count"`
	}
	result := make([]SeriesWithCount, len(seriesList))
	for i, s := range seriesList {
		count, _ := h.seriesService.GetSeriesBookCount(ctx, s.ID)
		result[i] = SeriesWithCount{s, count}
	}

	response := map[string]interface{}{
		"series": result,
		"total":  total,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	params := UpdateSeriesPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the series
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(series.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Keep track of what's been changed
	opts := UpdateSeriesOptions{Columns: []string{}}

	if params.Name != nil && *params.Name != series.Name {
		series.Name = *params.Name
		series.NameSource = models.DataSourceManual
		opts.Columns = append(opts.Columns, "name", "name_source")
		// Regenerate sort name when name changes (unless sort_name_source is manual)
		if series.SortNameSource != models.DataSourceManual {
			series.SortName = sortname.ForTitle(*params.Name)
			series.SortNameSource = models.DataSourceFilepath
			opts.Columns = append(opts.Columns, "sort_name", "sort_name_source")
		}
	}

	// Update sort name
	if params.SortName != nil && *params.SortName != series.SortName {
		if *params.SortName == "" {
			// Empty string means regenerate from name
			series.SortName = sortname.ForTitle(series.Name)
			series.SortNameSource = models.DataSourceFilepath
		} else {
			series.SortName = *params.SortName
			series.SortNameSource = models.DataSourceManual
		}
		opts.Columns = append(opts.Columns, "sort_name", "sort_name_source")
	}

	if params.Description != nil {
		series.Description = params.Description
		opts.Columns = append(opts.Columns, "description")
	}

	// Update the model
	err = h.seriesService.UpdateSeries(ctx, series, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload the model
	series, err = h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Update FTS index for this series
	log := logger.FromContext(ctx)
	if err := h.searchService.IndexSeries(ctx, series); err != nil {
		log.Warn("failed to update search index for series", logger.Data{"series_id": series.ID, "error": err.Error()})
	}

	// Get book count
	bookCount, _ := h.seriesService.GetSeriesBookCount(ctx, id)

	response := struct {
		*models.Series
		BookCount int `json:"book_count"`
	}{series, bookCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) seriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	// Fetch the series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(series.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	booksList, err := h.bookService.ListBooks(ctx, books.ListBooksOptions{
		SeriesID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, booksList))
}

func (h *handler) seriesCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	// Fetch the series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(series.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get the library to determine cover aspect ratio preference
	library, err := h.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
		ID: &series.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Get the first book in the series for cover
	book, err := h.bookService.GetFirstBookInSeriesByID(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Select the appropriate file based on the library's cover aspect ratio setting
	coverFile := selectCoverFile(book.Files, library.CoverAspectRatio)
	if coverFile == nil || coverFile.CoverImagePath == nil || *coverFile.CoverImagePath == "" {
		return errcodes.NotFound("Series cover")
	}

	// Determine if this is a root-level book by checking if book.Filepath is a file
	isRootLevelBook := false
	if info, err := os.Stat(book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}

	// Determine the directory where covers are located
	var coverDir string
	if isRootLevelBook {
		coverDir = filepath.Dir(book.Filepath)
	} else {
		coverDir = book.Filepath
	}

	coverImagePath := filepath.Join(coverDir, *coverFile.CoverImagePath)

	// Set appropriate headers
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")

	return errors.WithStack(c.File(coverImagePath))
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

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	params := MergeSeriesPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the target series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(series.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Merge source series into target (this) series
	err = h.seriesService.MergeSeries(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteSeries(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Series")
	}

	// Fetch the series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(series.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.seriesService.DeleteSeries(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove from FTS index
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromSeriesIndex(ctx, id); err != nil {
		log.Warn("failed to remove series from search index", logger.Data{"series_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
