package tags

import (
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

type handler struct {
	tagService    *Service
	aliasService  *aliases.Service
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

	bookCount, err := h.tagService.GetBookCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.TagConfig, id)

	response := struct {
		*models.Tag
		BookCount int      `json:"book_count"`
		Aliases   []string `json:"aliases"`
	}{tag, bookCount, aliasList}

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

	type TagWithCount struct {
		*models.Tag
		BookCount int      `json:"book_count"`
		Aliases   []string `json:"aliases"`
	}
	result := make([]TagWithCount, len(tags))
	for i, t := range tags {
		bookCount, _ := h.tagService.GetBookCount(ctx, t.ID)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.TagConfig, t.ID)
		result[i] = TagWithCount{t, bookCount, aliasList}
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
	var oldName string
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
			response := struct {
				*models.Tag
				BookCount int      `json:"book_count"`
				Aliases   []string `json:"aliases"`
			}{existing, bookCount, aliasList}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		oldName = tag.Name
		tag.Name = newName
		opts := UpdateTagOptions{Columns: []string{"name"}}
		err = h.tagService.UpdateTag(ctx, tag, opts)
		if err != nil {
			return errors.WithStack(err)
		}
		nameChanged = true
	}

	if params.Aliases != nil {
		syncList := params.Aliases
		if nameChanged {
			syncList = append(syncList, oldName)
		}
		if err := h.aliasService.SyncAliases(ctx, aliases.TagConfig, id, tag.LibraryID, syncList); err != nil {
			return errors.WithStack(err)
		}
	} else if nameChanged {
		_ = h.aliasService.RemoveAlias(ctx, aliases.TagConfig, id, tag.Name)
		log := logger.FromContext(ctx)
		if err := h.aliasService.AddAlias(ctx, aliases.TagConfig, id, oldName, tag.LibraryID); err != nil {
			log.Warn("failed to add old name as alias after rename", logger.Data{"tag_id": id, "old_name": oldName, "error": err.Error()})
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
	response := struct {
		*models.Tag
		BookCount int      `json:"book_count"`
		Aliases   []string `json:"aliases"`
	}{tag, bookCount, aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) books(c echo.Context) error {
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

	err = h.tagService.DeleteTag(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromTagIndex(ctx, id); err != nil {
		log.Warn("failed to remove tag from search index", logger.Data{"tag_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
