package genres

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
)

// BookReviewRecomputer refreshes files.reviewed for every file of each book,
// loading the review criteria once. *books.Service satisfies it. pkg/books
// imports pkg/genres, so the handler takes this interface to avoid an import
// cycle.
type BookReviewRecomputer interface {
	RecomputeReviewedForBooks(ctx context.Context, bookIDs []int)
}

type handler struct {
	genreService     *Service
	aliasService     *aliases.Service
	searchService    *search.Service
	reviewRecomputer BookReviewRecomputer
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Genre")
	}

	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(genre.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
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

	// Filter by user's library access if user is in context
	if user, ok := c.Get("user").(*models.User); ok {
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Genre")
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
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(genre.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	nameChanged := false
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
			err = h.genreService.MergeGenres(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
			}

			// Remove merged genre from FTS index and re-index the target
			log := logger.FromContext(ctx)
			if err := h.searchService.DeleteFromGenreIndex(ctx, id); err != nil {
				log.Warn("failed to remove merged genre from search index", logger.Data{"genre_id": id, "error": err.Error()})
			}
			if err := h.searchService.IndexGenre(ctx, existing); err != nil {
				log.Warn("failed to re-index target genre after merge", logger.Data{"genre_id": existing.ID, "error": err.Error()})
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
		nameChanged = true
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

	if nameChanged || params.Aliases != nil {
		log := logger.FromContext(ctx)
		if err := h.searchService.IndexGenre(ctx, genre); err != nil {
			log.Warn("failed to update search index for genre", logger.Data{"genre_id": genre.ID, "error": err.Error()})
		}
	}

	bookCount, _ := h.genreService.GetBookCount(ctx, id)
	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.GenreConfig, id)
	response := GenreResponse{Genre: *genre, BookCount: bookCount, Aliases: aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Genre")
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
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(genre.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Genre")
	}

	params := MergeGenresPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the target genre to check library access
	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(genre.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Merge source genre into target (this) genre
	err = h.genreService.MergeGenres(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove the merged (source) genre from FTS index and re-index the target
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromGenreIndex(ctx, params.SourceID); err != nil {
		log.Warn("failed to remove merged genre from search index", logger.Data{"genre_id": params.SourceID, "error": err.Error()})
	}
	if err := h.searchService.IndexGenre(ctx, genre); err != nil {
		log.Warn("failed to re-index target genre after merge", logger.Data{"genre_id": genre.ID, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteGenre(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Genre")
	}

	// Fetch the genre to check library access
	genre, err := h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(genre.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
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
