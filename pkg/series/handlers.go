package series

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/merge"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/sortname"
)

type handler struct {
	seriesService  *Service
	aliasService   *aliases.Service
	bookService    *books.Service
	libraryService *libraries.Service
	searchService  *search.Service
	coverCache     *covers.ThumbnailCache
}

// buildSeriesResponse adds the book count and the flat alias list to a series.
// retrieve, list, and update all use it, so a failed lookup fails the request
// instead of rendering a zero count or no aliases.
func (h *handler) buildSeriesResponse(ctx context.Context, series *models.Series) (SeriesResponse, error) {
	bookCount, err := h.seriesService.GetSeriesBookCount(ctx, series.ID)
	if err != nil {
		return SeriesResponse{}, errors.WithStack(err)
	}
	aliasList, err := h.aliasService.ListAliases(ctx, aliases.SeriesConfig, series.ID)
	if err != nil {
		return SeriesResponse{}, errors.WithStack(err)
	}
	return SeriesResponse{Series: *series, BookCount: bookCount, Aliases: aliasList}, nil
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Series")
	if err != nil {
		return err
	}

	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, series.LibraryID); err != nil {
		return err
	}

	seriesFiles, err := h.bookService.GetFirstBooksFilesForSeries(ctx, []int{id})
	if err != nil {
		return errors.WithStack(err)
	}
	aspectRatio := ""
	if series.Library != nil {
		aspectRatio = series.Library.CoverAspectRatio
	}
	series.CoverCacheKey = covers.CacheKey(seriesFiles[id], aspectRatio)

	response, err := h.buildSeriesResponse(ctx, series)
	if err != nil {
		return err
	}

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

	// Filter by the user's library access
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
		opts.LibraryIDs = libraryIDs
	}

	seriesList, total, err := h.seriesService.ListSeriesWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Batch-load cover cache keys for all series.
	seriesIDs := make([]int, len(seriesList))
	for i, s := range seriesList {
		seriesIDs[i] = s.ID
	}
	seriesFiles, err := h.bookService.GetFirstBooksFilesForSeries(ctx, seriesIDs)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with book counts, aliases, and cover cache keys.
	result := make([]SeriesResponse, len(seriesList))
	for i, s := range seriesList {
		aspectRatio := ""
		if s.Library != nil {
			aspectRatio = s.Library.CoverAspectRatio
		}
		s.CoverCacheKey = covers.CacheKey(seriesFiles[s.ID], aspectRatio)
		result[i], err = h.buildSeriesResponse(ctx, s)
		if err != nil {
			return err
		}
	}

	response := ListSeriesResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Series")
	if err != nil {
		return err
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
	if err := auth.RequireLibraryAccessFor(c, series.LibraryID); err != nil {
		return err
	}

	// The name and aliases are copied into series_fts and into books_fts for
	// every member book. The rename commits before SyncAliases runs, so the
	// reindex is deferred to also cover a rejected alias list.
	affected := h.searchService.CollectAffected(ctx, search.Affected{SeriesIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Keep track of what's been changed
	opts := UpdateSeriesOptions{Columns: []string{}}

	if params.Name != nil && *params.Name != series.Name {
		// A rename onto another Series' name is rejected rather than merged,
		// so combining two Series stays an explicit merge.
		existing, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
			Name:      params.Name,
			LibraryID: &series.LibraryID,
		})
		if err == nil && existing.ID != id {
			return errcodes.ValidationError("A series with this name already exists. Merge the two series instead.")
		}
		if err != nil && !errors.Is(err, errcodes.NotFound("Series")) {
			return errors.WithStack(err)
		}

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

	// Sync aliases if provided
	if params.Aliases != nil {
		if err := h.aliasService.SyncAliases(ctx, aliases.SeriesConfig, id, series.LibraryID, params.Aliases); err != nil {
			return errors.WithStack(err)
		}
	}

	// Reload the model
	series, err = h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	response, err := h.buildSeriesResponse(ctx, series)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) seriesBooks(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Series")
	if err != nil {
		return err
	}

	params := SubResourceQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, series.LibraryID); err != nil {
		return err
	}

	booksList, total, err := h.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
		SeriesID: &id,
		Limit:    &params.Limit,
		Offset:   &params.Offset,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	for _, b := range booksList {
		aspectRatio := ""
		if b.Library != nil {
			aspectRatio = b.Library.CoverAspectRatio
		}
		b.CoverCacheKey = covers.CacheKey(b.Files, aspectRatio)
	}

	response := ListSeriesBooksResponse{Items: booksList, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) seriesCover(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Series")
	if err != nil {
		return err
	}

	// Fetch the series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, series.LibraryID); err != nil {
		return err
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

	// The shared helper's ETag bakes in the selected file's ID, so it changes
	// when the series' first book switches to a different file even if the
	// new cover has an older mtime.
	return covers.ServeBookCover(c, book.Files, library.CoverAspectRatio, covers.CacheControlImmutable, "Series cover", h.coverCache)
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Series")
	if err != nil {
		return err
	}

	params := MergeSeriesPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch both sides, so a missing target or source is a 404, then run the
	// shared merge checks.
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	source, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &params.SourceID,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if err := merge.CheckPreconditions(user, "series",
		merge.Side{ID: series.ID, LibraryID: series.LibraryID},
		merge.Side{ID: source.ID, LibraryID: source.LibraryID},
	); err != nil {
		return err
	}

	// Every book of both series lists the target name and the source name,
	// which becomes an alias of the target, and the target's series_fts row
	// gains the moved books. The reindex drops the deleted source's row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{SeriesIDs: []int{id, params.SourceID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	// Merge source series into target (this) series
	if err := h.seriesService.MergeSeries(ctx, id, params.SourceID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteSeries(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Series")
	if err != nil {
		return err
	}

	// Fetch the series to check library access
	series, err := h.seriesService.RetrieveSeries(ctx, RetrieveSeriesOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, series.LibraryID); err != nil {
		return err
	}

	// The member books' books_fts rows list the series name. The delete
	// CASCADEs the links away, so collect the books first. The reindex drops
	// the series' own row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{SeriesIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	affectedBookIDs, err := h.seriesService.DeleteSeries(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Removing the join rows can flip the books' Reviewed completeness state
	// (e.g. when `series` is a required field).
	h.bookService.RecomputeReviewedForBooks(ctx, affectedBookIDs)

	return c.NoContent(http.StatusNoContent)
}
