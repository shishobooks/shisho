package publishers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	publisherService *Service
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

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get file count
	fileCount, err := h.publisherService.GetFileCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	response := struct {
		*models.Publisher
		FileCount int `json:"file_count"`
	}{publisher, fileCount}

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

	// Filter by user's library access if user is in context
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

	// Augment with file counts
	type PublisherWithCount struct {
		*models.Publisher
		FileCount int `json:"file_count"`
	}
	result := make([]PublisherWithCount, len(publishers))
	for i, p := range publishers {
		fileCount, _ := h.publisherService.GetFileCount(ctx, p.ID)
		result[i] = PublisherWithCount{p, fileCount}
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

	// Fetch the publisher
	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	if params.Name == nil || *params.Name == publisher.Name {
		// No change, just return current
		fileCount, _ := h.publisherService.GetFileCount(ctx, id)
		response := struct {
			*models.Publisher
			FileCount int `json:"file_count"`
		}{publisher, fileCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	newName := strings.TrimSpace(*params.Name)
	if newName == "" {
		return errcodes.ValidationError("Publisher name cannot be empty")
	}

	// Check if a publisher with the new name already exists (case-insensitive)
	existing, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		Name:      &newName,
		LibraryID: &publisher.LibraryID,
	})
	if err == nil && existing.ID != id {
		// Merge into existing publisher
		err = h.publisherService.MergePublishers(ctx, existing.ID, id)
		if err != nil {
			return errors.WithStack(err)
		}

		// Return the target publisher
		fileCount, _ := h.publisherService.GetFileCount(ctx, existing.ID)
		response := struct {
			*models.Publisher
			FileCount int `json:"file_count"`
		}{existing, fileCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	// Simple rename
	publisher.Name = newName
	opts := UpdatePublisherOptions{Columns: []string{"name"}}
	err = h.publisherService.UpdatePublisher(ctx, publisher, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload
	publisher, err = h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	fileCount, _ := h.publisherService.GetFileCount(ctx, id)
	response := struct {
		*models.Publisher
		FileCount int `json:"file_count"`
	}{publisher, fileCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) files(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Publisher")
	}

	// Fetch the publisher to check library access
	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
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

	// Fetch the target publisher to check library access
	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Merge source publisher into target (this) publisher
	err = h.publisherService.MergePublishers(ctx, id, params.SourceID)
	if err != nil {
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

	// Fetch the publisher to check library access
	publisher, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(publisher.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.publisherService.DeletePublisher(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
