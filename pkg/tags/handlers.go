package tags

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
	tagService    *Service
	searchService *search.Service
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

	// Get book count
	bookCount, err := h.tagService.GetBookCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	response := struct {
		*models.Tag
		BookCount int `json:"book_count"`
	}{tag, bookCount}

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

	// Filter by user's library access if user is in context
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

	// Augment with book counts
	type TagWithCount struct {
		*models.Tag
		BookCount int `json:"book_count"`
	}
	result := make([]TagWithCount, len(tags))
	for i, t := range tags {
		bookCount, _ := h.tagService.GetBookCount(ctx, t.ID)
		result[i] = TagWithCount{t, bookCount}
	}

	response := map[string]any{
		"tags":  result,
		"total": total,
	}

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

	// Fetch the tag
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

	if params.Name == nil || *params.Name == tag.Name {
		// No change, just return current
		bookCount, _ := h.tagService.GetBookCount(ctx, id)
		response := struct {
			*models.Tag
			BookCount int `json:"book_count"`
		}{tag, bookCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	newName := strings.TrimSpace(*params.Name)
	if newName == "" {
		return errcodes.ValidationError("Tag name cannot be empty")
	}

	// Check if a tag with the new name already exists (case-insensitive)
	existing, err := h.tagService.RetrieveTag(ctx, RetrieveTagOptions{
		Name:      &newName,
		LibraryID: &tag.LibraryID,
	})
	if err == nil && existing.ID != id {
		// Merge into existing tag
		err = h.tagService.MergeTags(ctx, existing.ID, id)
		if err != nil {
			return errors.WithStack(err)
		}

		// Remove merged tag from FTS index
		log := logger.FromContext(ctx)
		if err := h.searchService.DeleteFromTagIndex(ctx, id); err != nil {
			log.Warn("failed to remove merged tag from search index", logger.Data{"tag_id": id, "error": err.Error()})
		}

		// Return the target tag
		bookCount, _ := h.tagService.GetBookCount(ctx, existing.ID)
		response := struct {
			*models.Tag
			BookCount int `json:"book_count"`
		}{existing, bookCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	// Simple rename
	tag.Name = newName
	opts := UpdateTagOptions{Columns: []string{"name"}}
	err = h.tagService.UpdateTag(ctx, tag, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload and update FTS index
	tag, err = h.tagService.RetrieveTag(ctx, RetrieveTagOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)
	if err := h.searchService.IndexTag(ctx, tag); err != nil {
		log.Warn("failed to update search index for tag", logger.Data{"tag_id": tag.ID, "error": err.Error()})
	}

	bookCount, _ := h.tagService.GetBookCount(ctx, id)
	response := struct {
		*models.Tag
		BookCount int `json:"book_count"`
	}{tag, bookCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	// Fetch the tag to check library access
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

	books, err := h.tagService.GetBooks(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, books))
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

	// Fetch the target tag to check library access
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

	// Merge source tag into target (this) tag
	err = h.tagService.MergeTags(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove the merged (source) tag from FTS index
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromTagIndex(ctx, params.SourceID); err != nil {
		log.Warn("failed to remove merged tag from search index", logger.Data{"tag_id": params.SourceID, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteTag(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Tag")
	}

	// Fetch the tag to check library access
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

	err = h.tagService.DeleteTag(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Remove from FTS index
	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromTagIndex(ctx, id); err != nil {
		log.Warn("failed to remove tag from search index", logger.Data{"tag_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
