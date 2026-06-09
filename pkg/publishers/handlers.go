package publishers

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

type handler struct {
	publisherService *Service
	aliasService     *aliases.Service
	searchService    *search.Service
}

// buildPublisherResponse assembles the full single-publisher API response
// (PublisherResponse) for the given publisher: rolled-up file counts, aliases
// as a flat []string, the ancestor chain, descendant ids, and flattened direct
// children. retrieve, update, and merge all use this so a mutation returns the
// same full shape the detail page reads (enabling setQueryData on the client).
func (h *handler) buildPublisherResponse(ctx context.Context, publisher *models.Publisher) (PublisherResponse, error) {
	id := publisher.ID

	fileCount, err := h.publisherService.GetFileCount(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, id)

	ancestors, err := h.publisherService.GetAncestors(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}
	ancestorList := make([]AncestorResponse, len(ancestors))
	for i, a := range ancestors {
		ancestorList[i] = AncestorResponse{ID: a.ID, Name: a.Name}
	}

	descendantIDs, err := h.publisherService.GetDescendantIDs(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	children, err := h.publisherService.GetChildren(ctx, id)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}
	childList := make([]ChildResponse, len(children))
	for i, ch := range children {
		childList[i] = ChildResponse{ID: ch.ID, Name: ch.Name, FileCount: ch.FileCount}
	}

	descendantFileCount, err := h.publisherService.GetFileCountForPublisherIDs(ctx, descendantIDs)
	if err != nil {
		return PublisherResponse{}, errors.WithStack(err)
	}

	return PublisherResponse{
		Publisher:           *publisher,
		FileCount:           fileCount,
		DescendantFileCount: descendantFileCount,
		Aliases:             aliasList,
		Ancestors:           ancestorList,
		DescendantIDs:       descendantIDs,
		Children:            childList,
	}, nil
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

	response, err := h.buildPublisherResponse(ctx, publisher)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListPublishersQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListPublishersOptions{
		Limit:      &params.Limit,
		Offset:     &params.Offset,
		LibraryID:  params.LibraryID,
		Search:     params.Search,
		ExcludeIDs: params.ExcludeIDs,
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

	// Build a lookup map of publisher ID -> name for parent resolution
	publisherNameMap := make(map[int]string, len(publishers))
	for _, p := range publishers {
		publisherNameMap[p.ID] = p.Name
	}

	result := make([]PublisherListItem, len(publishers))
	for i, p := range publishers {
		fileCount, _ := h.publisherService.GetFileCount(ctx, p.ID)
		descendantIDs, _ := h.publisherService.GetDescendantIDs(ctx, p.ID)
		descendantFileCount, _ := h.publisherService.GetFileCountForPublisherIDs(ctx, descendantIDs)
		descendantPublisherCount := len(descendantIDs)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.PublisherConfig, p.ID)

		var parentName *string
		if p.ParentID != nil {
			if name, ok := publisherNameMap[*p.ParentID]; ok {
				parentName = &name
			} else {
				// Parent might not be in the current page; look it up
				parent, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: p.ParentID})
				if err == nil {
					parentName = &parent.Name
				}
			}
		}

		result[i] = PublisherListItem{
			Publisher:                *p,
			FileCount:                fileCount,
			DescendantFileCount:      descendantFileCount,
			DescendantPublisherCount: descendantPublisherCount,
			ParentName:               parentName,
			Aliases:                  aliasList,
		}
	}

	response := ListPublishersResponse{Items: result, Total: total}

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

	// Handle parent update before name-change/merge so that the parent change
	// applies even when a rename triggers a merge (which returns early).
	// resolvedParentID tracks the parent ID resolved from either path so the
	// merge-transfer block below can use it.
	var resolvedParentID *int
	var parentWasSet bool
	if params.ParentID.Set {
		resolvedParentID = params.ParentID.Value
		parentWasSet = true
		if err := h.publisherService.SetParent(ctx, id, params.ParentID.Value); err != nil {
			if strings.Contains(err.Error(), "cycle") || strings.Contains(err.Error(), "invalid parent") || strings.Contains(err.Error(), "same library") || strings.Contains(err.Error(), "not found") {
				return errcodes.ValidationError(err.Error())
			}
			return errors.WithStack(err)
		}
	} else if params.ParentName != nil {
		// Resolve parent by name: find or create a publisher with the given name
		// in the same library, then set it as the parent.
		parentPublisher, err := h.publisherService.FindOrCreatePublisher(ctx, *params.ParentName, publisher.LibraryID)
		if err != nil {
			return errors.WithStack(err)
		}
		resolvedParentID = &parentPublisher.ID
		parentWasSet = true
		// Index the parent publisher in case it was just created
		if indexErr := h.searchService.IndexPublisher(ctx, parentPublisher); indexErr != nil {
			log := logger.FromContext(ctx)
			log.Warn("failed to index new parent publisher", logger.Data{"publisher_id": parentPublisher.ID, "error": indexErr.Error()})
		}
		if err := h.publisherService.SetParent(ctx, id, &parentPublisher.ID); err != nil {
			if strings.Contains(err.Error(), "cycle") || strings.Contains(err.Error(), "invalid parent") || strings.Contains(err.Error(), "same library") || strings.Contains(err.Error(), "not found") {
				return errcodes.ValidationError(err.Error())
			}
			return errors.WithStack(err)
		}
	}

	nameChanged := false
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
			// If a parent was set on the source publisher above, transfer it to
			// the merge target so the intent is preserved.
			if parentWasSet {
				if err := h.publisherService.SetParent(ctx, existing.ID, resolvedParentID); err != nil {
					// Non-fatal: merge succeeded, log and continue
					log := logger.FromContext(ctx)
					log.Warn("failed to set parent on merge target", logger.Data{"publisher_id": existing.ID, "error": err.Error()})
				}
			}

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

			// Re-retrieve to pick up parent_id change
			existing, _ = h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &existing.ID})
			response, err := h.buildPublisherResponse(ctx, existing)
			if err != nil {
				return err
			}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		publisher.Name = newName
		opts := UpdatePublisherOptions{Columns: []string{"name"}}
		err = h.publisherService.UpdatePublisher(ctx, publisher, opts)
		if err != nil {
			return errors.WithStack(err)
		}
		nameChanged = true
	}

	if params.Aliases != nil {
		if err := h.aliasService.SyncAliases(ctx, aliases.PublisherConfig, id, publisher.LibraryID, params.Aliases); err != nil {
			return errors.WithStack(err)
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

	response, err := h.buildPublisherResponse(ctx, publisher)
	if err != nil {
		return err
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) files(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	params := SubResourceQuery{}
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

	files, total, err := h.publisherService.GetFilesPaginated(ctx, id, params.Limit, params.Offset)
	if err != nil {
		return errors.WithStack(err)
	}

	response := ListPublisherFilesResponse{Items: files, Total: total}

	return errors.WithStack(c.JSON(http.StatusOK, response))
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

func (h *handler) setChild(c echo.Context) error {
	ctx := c.Request().Context()
	parentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	params := SetChildPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	parent, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &parentID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(parent.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// SetParent validates same-library, cycle detection, and sets the parent
	if err := h.publisherService.SetParent(ctx, params.ChildID, &parentID); err != nil {
		if strings.Contains(err.Error(), "cycle") || strings.Contains(err.Error(), "invalid parent") || strings.Contains(err.Error(), "same library") || strings.Contains(err.Error(), "not found") {
			return errcodes.ValidationError(err.Error())
		}
		return errors.WithStack(err)
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
