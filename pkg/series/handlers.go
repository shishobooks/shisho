package series

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
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
	coverFile := covers.SelectFile(book.Files, library.CoverAspectRatio)
	if coverFile == nil || coverFile.CoverImageFilename == nil || *coverFile.CoverImageFilename == "" {
		return errcodes.NotFound("Series cover")
	}

	// Resolve via the file's parent dir — book.Filepath may be a synthetic
	// organized-folder path that doesn't exist on disk for root-level files.
	coverImagePath := filepath.Join(filepath.Dir(coverFile.Filepath), *coverFile.CoverImageFilename)

	coverStat, err := os.Stat(coverImagePath)
	if err != nil {
		return errcodes.NotFound("Series cover")
	}
	modTime := coverStat.ModTime().UTC().Truncate(time.Second)

	// ETag bakes in the selected file's identity, not just mtime, so it changes
	// when the series' first book switches to a different file — even if the new
	// cover happens to have an older mtime than the previous first book's cover.
	etag := fmt.Sprintf(`"%d-%d"`, coverFile.ID, modTime.Unix())

	c.Response().Header().Set("Cache-Control", "private, no-cache")
	c.Response().Header().Set("ETag", etag)

	// Conditional GET uses ETag only. If-Modified-Since is intentionally not
	// honored: file mtime doesn't capture changes in which file is selected,
	// so IMS-based revalidation would serve stale bytes when the first book
	// switches to one whose cover file has an older mtime.
	if inm := c.Request().Header.Get("If-None-Match"); inm != "" && inm == etag {
		c.Response().WriteHeader(http.StatusNotModified)
		return nil
	}

	fh, err := os.Open(coverImagePath)
	if err != nil {
		return errcodes.NotFound("Series cover")
	}
	defer fh.Close()

	// Zero modtime suppresses Last-Modified and IMS handling inside ServeContent.
	http.ServeContent(c.Response(), c.Request(), filepath.Base(coverImagePath), time.Time{}, fh)
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
	movedBookIDs, err := h.seriesService.MergeSeries(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)

	// Remove the merged (source) series from FTS index
	if err := h.searchService.DeleteFromSeriesIndex(ctx, params.SourceID); err != nil {
		log.Warn("failed to remove merged series from search index", logger.Data{"series_id": params.SourceID, "error": err.Error()})
	}

	// Books that moved from source to target carry the source series name in
	// their books_fts row; re-index them so the target series name is what
	// shows up in search.
	for _, bookID := range movedBookIDs {
		book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
		if err != nil {
			log.Warn("failed to retrieve book for FTS reindex after series merge", logger.Data{"book_id": bookID, "error": err.Error()})
			continue
		}
		if err := h.searchService.IndexBook(ctx, book); err != nil {
			log.Warn("failed to update book search index after series merge", logger.Data{"book_id": bookID, "error": err.Error()})
		}
	}

	// Re-index the target series since it now has more books
	series, err = h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err == nil {
		if err := h.searchService.IndexSeries(ctx, series); err != nil {
			log.Warn("failed to update search index for target series", logger.Data{"series_id": id, "error": err.Error()})
		}
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

	affectedBookIDs, err := h.seriesService.DeleteSeries(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)

	// Removing the join rows can flip the books' Reviewed completeness state
	// (e.g. when `series` is a required field) and stales their books_fts
	// rows, which still reference the deleted series name. Recompute review
	// state and re-index each affected book.
	for _, bookID := range affectedBookIDs {
		h.bookService.RecomputeReviewedForBook(ctx, bookID)

		book, err := h.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
		if err != nil {
			log.Warn("failed to retrieve book for FTS reindex after series delete", logger.Data{"book_id": bookID, "error": err.Error()})
			continue
		}
		if err := h.searchService.IndexBook(ctx, book); err != nil {
			log.Warn("failed to update book search index after series delete", logger.Data{"book_id": bookID, "error": err.Error()})
		}
	}

	// Remove the deleted series itself from the series FTS index.
	if err := h.searchService.DeleteFromSeriesIndex(ctx, id); err != nil {
		log.Warn("failed to remove series from search index", logger.Data{"series_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
