package tags

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/merge"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
)

type handler struct {
	tagService       *Service
	aliasService     *aliases.Service
	searchService    *search.Service
	reviewRecomputer review.BookReviewRecomputer
}

// buildTagResponse adds the book count and the flat alias list to a tag.
// retrieve, list, and update all use it, so a failed lookup fails the request
// instead of rendering a zero count or no aliases.
func (h *handler) buildTagResponse(ctx context.Context, tag *models.Tag) (TagResponse, error) {
	bookCount, err := h.tagService.GetBookCount(ctx, tag.ID)
	if err != nil {
		return TagResponse{}, errors.WithStack(err)
	}
	aliasList, err := h.aliasService.ListAliases(ctx, aliases.TagConfig, tag.ID)
	if err != nil {
		return TagResponse{}, errors.WithStack(err)
	}
	return TagResponse{Tag: *tag, BookCount: bookCount, Aliases: aliasList}, nil
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Tag")
	if err != nil {
		return err
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

	response, err := h.buildTagResponse(ctx, tag)
	if err != nil {
		return err
	}

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
		result[i], err = h.buildTagResponse(ctx, t)
		if err != nil {
			return err
		}
	}

	response := ListTagsResponse{Items: result, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Tag")
	if err != nil {
		return err
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

			response, err := h.buildTagResponse(ctx, existing)
			if err != nil {
				return err
			}
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

	response, err := h.buildTagResponse(ctx, tag)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := httputil.ParamID(c, "id", "Tag")
	if err != nil {
		return err
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
	id, err := httputil.ParamID(c, "id", "Tag")
	if err != nil {
		return err
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
	id, err := httputil.ParamID(c, "id", "Tag")
	if err != nil {
		return err
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

	// The reindex drops the deleted tag's tags_fts row. books_fts has no
	// tag column, so the affected Books need no reindex.
	affected := h.searchService.CollectAffected(ctx, search.Affected{TagIDs: []int{id}})
	defer h.searchService.ReindexAffected(ctx, affected)

	affectedBookIDs, err := h.tagService.DeleteTag(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Removing the join rows can flip the books' Reviewed completeness state
	// (e.g. when `tags` is a required field), so recompute it for every
	// affected book.
	h.reviewRecomputer.RecomputeReviewedForBooks(ctx, affectedBookIDs)

	return c.NoContent(http.StatusNoContent)
}
