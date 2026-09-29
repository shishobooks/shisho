package genres

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/merge"
	"github.com/shishobooks/shisho/pkg/search"
)

type handler struct {
	genreService     *Service
	aliasService     *aliases.Service
	searchService    *search.Service
	reviewRecomputer review.BookReviewRecomputer
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Genre")
	if err != nil {
		return err
	}

	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, genre.LibraryID); err != nil {
		return err
	}

	// Get book count
	bookCount, err := h.genreService.GetBookCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.GenreConfig, id)

	response := GenreResponse{Genre: *genre, BookCount: bookCount, Aliases: aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListGenresQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListGenresOptions{
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

	genres, total, err := h.genreService.ListGenresWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with book counts and aliases
	result := make([]GenreResponse, len(genres))
	for i, g := range genres {
		bookCount, _ := h.genreService.GetBookCount(ctx, g.ID)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.GenreConfig, g.ID)
		result[i] = GenreResponse{Genre: *g, BookCount: bookCount, Aliases: aliasList}
	}

	response := ListGenresResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Genre")
	if err != nil {
		return err
	}

	params := UpdateGenrePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the genre
	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, genre.LibraryID); err != nil {
		return err
	}

	// The rename commits before SyncAliases runs, so the reindex is deferred
	// to also cover a rejected alias list. A rename onto an existing genre
	// merges into it, and the reindex drops the merged row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{GenreIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	if params.Name != nil && *params.Name != genre.Name {
		newName := strings.TrimSpace(*params.Name)
		if newName == "" {
			return errcodes.ValidationError("Genre name cannot be empty")
		}

		// Check if a genre with the new name already exists (case-insensitive)
		existing, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
			Name:      &newName,
			LibraryID: &genre.LibraryID,
		})
		if err == nil && existing.ID != id {
			// Merge into existing genre
			affected.GenreIDs = append(affected.GenreIDs, existing.ID)
			err = h.genreService.MergeGenres(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
			}

			// Return the target genre
			bookCount, _ := h.genreService.GetBookCount(ctx, existing.ID)
			aliasList, _ := h.aliasService.ListAliases(ctx, aliases.GenreConfig, existing.ID)
			response := GenreResponse{Genre: *existing, BookCount: bookCount, Aliases: aliasList}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		genre.Name = newName
		opts := UpdateGenreOptions{Columns: []string{"name"}}
		err = h.genreService.UpdateGenre(ctx, genre, opts)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Sync aliases if provided
	if params.Aliases != nil {
		if err := h.aliasService.SyncAliases(ctx, aliases.GenreConfig, id, genre.LibraryID, params.Aliases); err != nil {
			return errors.WithStack(err)
		}
	}

	// Reload
	genre, err = h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	bookCount, _ := h.genreService.GetBookCount(ctx, id)
	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.GenreConfig, id)
	response := GenreResponse{Genre: *genre, BookCount: bookCount, Aliases: aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Genre")
	if err != nil {
		return err
	}

	params := SubResourceQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the genre to check library access
	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, genre.LibraryID); err != nil {
		return err
	}

	books, total, err := h.genreService.GetBooksPaginated(ctx, id, params.Limit, params.Offset)
	if err != nil {
		return errors.WithStack(err)
	}

	response := ListGenreBooksResponse{Items: books, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Genre")
	if err != nil {
		return err
	}

	params := MergeGenresPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch both sides, so a missing target or source is a 404, then run the
	// shared merge checks.
	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	source, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &params.SourceID,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if err := merge.CheckPreconditions(user, "genre",
		merge.Side{ID: genre.ID, LibraryID: genre.LibraryID},
		merge.Side{ID: source.ID, LibraryID: source.LibraryID},
	); err != nil {
		return err
	}

	// The target's genres_fts row gains the source name as an alias, and the
	// reindex drops the deleted source's row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{GenreIDs: []int{id, params.SourceID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	if err := h.genreService.MergeGenres(ctx, id, params.SourceID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteGenre(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Genre")
	if err != nil {
		return err
	}

	// Fetch the genre to check library access
	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if err := auth.RequireLibraryAccessFor(c, genre.LibraryID); err != nil {
		return err
	}

	affectedBookIDs, err := h.genreService.DeleteGenre(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)

	// Removing the join rows can flip the books' Reviewed completeness state
	// (e.g. when `genres` is a required field), so recompute it for every
	// affected book. Unlike deleteSeries there is no books_fts re-index:
	// books_fts has no genre column. Add ReindexBookByID here if it gains one.
	h.reviewRecomputer.RecomputeReviewedForBooks(ctx, affectedBookIDs)

	// Remove the deleted genre itself from the genre FTS index.
	if err := h.searchService.DeleteFromGenreIndex(ctx, id); err != nil {
		log.Warn("failed to remove genre from search index", logger.Data{"genre_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
