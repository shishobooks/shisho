package genres

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
)

type handler struct {
	genreService  *Service
	searchService *search.Service
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

	response := struct {
		*models.Genre
		BookCount int `json:"book_count"`
	}{genre, bookCount}

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

	// Augment with book counts
	type GenreWithCount struct {
		*models.Genre
		BookCount int `json:"book_count"`
	}
	result := make([]GenreWithCount, len(genres))
	for i, g := range genres {
		bookCount, _ := h.genreService.GetBookCount(ctx, g.ID)
		result[i] = GenreWithCount{g, bookCount}
	}

	response := map[string]any{
		"genres": result,
		"total":  total,
	}

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

	if params.Name == nil || *params.Name == genre.Name {
		// No change, just return current
		bookCount, _ := h.genreService.GetBookCount(ctx, id)
		response := struct {
			*models.Genre
			BookCount int `json:"book_count"`
		}{genre, bookCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

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

		// Remove merged genre from FTS index
		log := logger.FromContext(ctx)
		if err := h.searchService.DeleteFromGenreIndex(ctx, id); err != nil {
			log.Warn("failed to remove merged genre from search index", logger.Data{"genre_id": id, "error": err.Error()})
		}

		// Return the target genre
		bookCount, _ := h.genreService.GetBookCount(ctx, existing.ID)
		response := struct {
			*models.Genre
			BookCount int `json:"book_count"`
		}{existing, bookCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	// Simple rename
	genre.Name = newName
	opts := UpdateGenreOptions{Columns: []string{"name"}}
	err = h.genreService.UpdateGenre(ctx, genre, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload and update FTS index
	genre, err = h.genreService.RetrieveGenre(ctx, RetrieveGenreOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)
	if err := h.searchService.IndexGenre(ctx, genre); err != nil {
		log.Warn("failed to update search index for genre", logger.Data{"genre_id": genre.ID, "error": err.Error()})
	}

	bookCount, _ := h.genreService.GetBookCount(ctx, id)
	response := struct {
		*models.Genre
		BookCount int `json:"book_count"`
	}{genre, bookCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
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

	books, err := h.genreService.GetBooks(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, books))
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

	// Remove the merged (source) genre from FTS index
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromGenreIndex(ctx, params.SourceID); err != nil {
		log.Warn("failed to remove merged genre from search index", logger.Data{"genre_id": params.SourceID, "error": err.Error()})
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

	err = h.genreService.DeleteGenre(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove from FTS index
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromGenreIndex(ctx, id); err != nil {
		log.Warn("failed to remove genre from search index", logger.Data{"genre_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
