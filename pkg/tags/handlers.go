package tags

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
// imports pkg/tags, so the handler takes this interface to avoid an import
// cycle.
type BookReviewRecomputer interface {
	RecomputeReviewedForBooks(ctx context.Context, bookIDs []int)
}

type handler struct {
	tagService       *Service
	aliasService     *aliases.Service
	searchService    *search.Service
	reviewRecomputer BookReviewRecomputer
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	tag, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(tag.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	bookCount, err := h.tagService.GetBookCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.TagConfig, id)

	response := TagResponse{Tag: *tag, BookCount: bookCount, Aliases: aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListTagsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListTagsOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
		Search:    params.Search,
	}

	if user, ok := c.Get("user").(*models.User); ok {
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
	}

	tags, total, err := h.tagService.ListTagsWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	result := make([]TagResponse, len(tags))
	for i, t := range tags {
		bookCount, _ := h.tagService.GetBookCount(ctx, t.ID)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.TagConfig, t.ID)
		result[i] = TagResponse{Tag: *t, BookCount: bookCount, Aliases: aliasList}
	}

	response := ListTagsResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	params := UpdateTagPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	tag, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(tag.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	nameChanged := false
	if params.Name != nil && *params.Name != tag.Name {
		newName := strings.TrimSpace(*params.Name)
		if newName == "" {
			return errcodes.ValidationError("Tag name cannot be empty")
		}

		existing, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
			Name:      &newName,
			LibraryID: &tag.LibraryID,
		})
		if err == nil && existing.ID != id {
			err = h.tagService.MergeTags(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
			}

			log := logger.FromContext(ctx)
			if err := h.searchService.DeleteFromTagIndex(ctx, id); err != nil {
				log.Warn("failed to remove merged tag from search index", logger.Data{"tag_id": id, "error": err.Error()})
			}
			if err := h.searchService.IndexTag(ctx, existing); err != nil {
				log.Warn("failed to re-index target tag after merge", logger.Data{"tag_id": existing.ID, "error": err.Error()})
			}

			bookCount, _ := h.tagService.GetBookCount(ctx, existing.ID)
			aliasList, _ := h.aliasService.ListAliases(ctx, aliases.TagConfig, existing.ID)
			response := TagResponse{Tag: *existing, BookCount: bookCount, Aliases: aliasList}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		tag.Name = newName
		opts := UpdateTagOptions{Columns: []string{"name"}}
		err = h.tagService.UpdateTag(ctx, tag, opts)
		if err != nil {
			return errors.WithStack(err)
		}
		nameChanged = true
	}

	if params.Aliases != nil {
		if err := h.aliasService.SyncAliases(ctx, aliases.TagConfig, id, tag.LibraryID, params.Aliases); err != nil {
			return errors.WithStack(err)
		}
	}

	tag, err = h.tagService.RetrieveTag(ctx, RetrieveTagOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	if nameChanged || params.Aliases != nil {
		log := logger.FromContext(ctx)
		if err := h.searchService.IndexTag(ctx, tag); err != nil {
			log.Warn("failed to update search index for tag", logger.Data{"tag_id": tag.ID, "error": err.Error()})
		}
	}

	bookCount, _ := h.tagService.GetBookCount(ctx, id)
	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.TagConfig, id)
	response := TagResponse{Tag: *tag, BookCount: bookCount, Aliases: aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	params := SubResourceQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	tag, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(tag.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	books, total, err := h.tagService.GetBooksPaginated(ctx, id, params.Limit, params.Offset)
	if err != nil {
		return errors.WithStack(err)
	}

	response := ListTagBooksResponse{Items: books, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	params := MergeTagsPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	tag, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(tag.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.tagService.MergeTags(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromTagIndex(ctx, params.SourceID); err != nil {
		log.Warn("failed to remove merged tag from search index", logger.Data{"tag_id": params.SourceID, "error": err.Error()})
	}
	if err := h.searchService.IndexTag(ctx, tag); err != nil {
		log.Warn("failed to re-index target tag after merge", logger.Data{"tag_id": tag.ID, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteTag(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	tag, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(tag.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	affectedBookIDs, err := h.tagService.DeleteTag(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)

	// Removing the join rows can flip the books' Reviewed completeness state
	// (e.g. when `tags` is a required field), so recompute it for every
	// affected book. Unlike deleteSeries there is no books_fts re-index:
	// books_fts has no tag column. Add ReindexBookByID here if it gains one.
	h.reviewRecomputer.RecomputeReviewedForBooks(ctx, affectedBookIDs)

	// Remove the deleted tag itself from the tag FTS index.
	if err := h.searchService.DeleteFromTagIndex(ctx, id); err != nil {
		log.Warn("failed to remove tag from search index", logger.Data{"tag_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
