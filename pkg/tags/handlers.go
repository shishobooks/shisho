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
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/merge"
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
	if err := auth.RequireLibraryAccessFor(c, tag.LibraryID); err != nil {
		return err
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

	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
		opts.LibraryIDs = libraryIDs
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

	if err := auth.RequireLibraryAccessFor(c, tag.LibraryID); err != nil {
		return err
	}

	// The rename commits before SyncAliases runs, so the reindex is deferred
	// to also cover a rejected alias list. A rename onto an existing tag
	// merges into it, and the reindex drops the merged row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{TagIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

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
			affected.TagIDs = append(affected.TagIDs, existing.ID)
			err = h.tagService.MergeTags(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
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

	if err := auth.RequireLibraryAccessFor(c, tag.LibraryID); err != nil {
		return err
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

	// Fetch both sides, so a missing target or source is a 404, then run the
	// shared merge checks.
	tag, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	source, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		ID: &params.SourceID,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if err := merge.CheckPreconditions(user, "tag",
		merge.Side{ID: tag.ID, LibraryID: tag.LibraryID},
		merge.Side{ID: source.ID, LibraryID: source.LibraryID},
	); err != nil {
		return err
	}

	// The target's tags_fts row gains the source name as an alias, and the
	// reindex drops the deleted source's row.
	affected := h.searchService.CollectAffected(ctx, search.Affected{TagIDs: []int{id, params.SourceID}})
	defer h.searchService.ReindexAffected(ctx, affected)

	if err := h.tagService.MergeTags(ctx, id, params.SourceID); err != nil {
		return errors.WithStack(err)
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

	if err := auth.RequireLibraryAccessFor(c, tag.LibraryID); err != nil {
		return err
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
