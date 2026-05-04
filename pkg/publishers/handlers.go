package publishers

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
	publisherService *Service
	aliasService     *aliases.Service
	searchService    *search.Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	fileCount, err := h.publisherService.GetFileCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, id)

	response := struct {
		*models.Publisher
		FileCount int      `json:"file_count"`
		Aliases   []string `json:"aliases"`
	}{publisher, fileCount, aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListPublishersQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListPublishersOptions{
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

	publishers, total, err := h.publisherService.ListPublishersWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	type PublisherWithCount struct {
		*models.Publisher
		FileCount int      `json:"file_count"`
		Aliases   []string `json:"aliases"`
	}
	result := make([]PublisherWithCount, len(publishers))
	for i, p := range publishers {
		fileCount, _ := h.publisherService.GetFileCount(ctx, p.ID)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, p.ID)
		result[i] = PublisherWithCount{p, fileCount, aliasList}
	}

	response := map[string]any{
		"publishers": result,
		"total":      total,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	params := UpdatePublisherPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	nameChanged := false
	var oldName string
	if params.Name != nil && *params.Name != publisher.Name {
		newName := strings.TrimSpace(*params.Name)
		if newName == "" {
			return errcodes.ValidationError("Publisher name cannot be empty")
		}

		existing, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
			Name:      &newName,
			LibraryID: &publisher.LibraryID,
		})
		if err == nil && existing.ID != id {
			err = h.publisherService.MergePublishers(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
			}

			log := logger.FromContext(ctx)
			if err := h.searchService.DeleteFromPublisherIndex(ctx, id); err != nil {
				log.Warn("failed to remove merged publisher from search index", logger.Data{"publisher_id": id, "error": err.Error()})
			}
			if err := h.searchService.IndexPublisher(ctx, existing); err != nil {
				log.Warn("failed to re-index target publisher after merge", logger.Data{"publisher_id": existing.ID, "error": err.Error()})
			}

			fileCount, _ := h.publisherService.GetFileCount(ctx, existing.ID)
			aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, existing.ID)
			response := struct {
				*models.Publisher
				FileCount int      `json:"file_count"`
				Aliases   []string `json:"aliases"`
			}{existing, fileCount, aliasList}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		oldName = publisher.Name
		publisher.Name = newName
		opts := UpdatePublisherOptions{Columns: []string{"name"}}
		err = h.publisherService.UpdatePublisher(ctx, publisher, opts)
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
		if err := h.aliasService.SyncAliases(ctx, aliases.PublisherConfig, id, publisher.LibraryID, syncList); err != nil {
			return errors.WithStack(err)
		}
	} else if nameChanged {
		_ = h.aliasService.RemoveAlias(ctx, aliases.PublisherConfig, id, publisher.Name)
		log := logger.FromContext(ctx)
		if err := h.aliasService.AddAlias(ctx, aliases.PublisherConfig, id, oldName, publisher.LibraryID); err != nil {
			log.Warn("failed to add old name as alias after rename", logger.Data{"publisher_id": id, "old_name": oldName, "error": err.Error()})
		}
	}

	publisher, err = h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	if nameChanged || params.Aliases != nil {
		log := logger.FromContext(ctx)
		if err := h.searchService.IndexPublisher(ctx, publisher); err != nil {
			log.Warn("failed to update search index for publisher", logger.Data{"publisher_id": publisher.ID, "error": err.Error()})
		}
	}

	fileCount, _ := h.publisherService.GetFileCount(ctx, id)
	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, id)
	response := struct {
		*models.Publisher
		FileCount int      `json:"file_count"`
		Aliases   []string `json:"aliases"`
	}{publisher, fileCount, aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) files(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	files, err := h.publisherService.GetFiles(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, files))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	params := MergePublishersPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.publisherService.MergePublishers(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromPublisherIndex(ctx, params.SourceID); err != nil {
		log.Warn("failed to remove merged publisher from search index", logger.Data{"publisher_id": params.SourceID, "error": err.Error()})
	}
	if err := h.searchService.IndexPublisher(ctx, publisher); err != nil {
		log.Warn("failed to re-index target publisher after merge", logger.Data{"publisher_id": publisher.ID, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deletePublisher(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.publisherService.DeletePublisher(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	log := logger.FromContext(ctx)
	if err := h.searchService.DeleteFromPublisherIndex(ctx, id); err != nil {
		log.Warn("failed to remove publisher from search index", logger.Data{"publisher_id": id, "error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
