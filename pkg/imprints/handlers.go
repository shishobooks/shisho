package imprints

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
	imprintService *Service
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Imprint")
	}

	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Get file count
	fileCount, err := h.imprintService.GetFileCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	response := struct {
		*models.Imprint
		FileCount int `json:"file_count"`
	}{imprint, fileCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListImprintsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	opts := ListImprintsOptions{
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

	imprints, total, err := h.imprintService.ListImprintsWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with file counts
	type ImprintWithCount struct {
		*models.Imprint
		FileCount int `json:"file_count"`
	}
	result := make([]ImprintWithCount, len(imprints))
	for i, imp := range imprints {
		fileCount, _ := h.imprintService.GetFileCount(ctx, imp.ID)
		result[i] = ImprintWithCount{imp, fileCount}
	}

	response := map[string]any{
		"imprints": result,
		"total":    total,
	}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Imprint")
	}

	params := UpdateImprintPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the imprint
	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	if params.Name == nil || *params.Name == imprint.Name {
		// No change, just return current
		fileCount, _ := h.imprintService.GetFileCount(ctx, id)
		response := struct {
			*models.Imprint
			FileCount int `json:"file_count"`
		}{imprint, fileCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	newName := strings.TrimSpace(*params.Name)
	if newName == "" {
		return errcodes.ValidationError("Imprint name cannot be empty")
	}

	// Check if an imprint with the new name already exists (case-insensitive)
	existing, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		Name:      &newName,
		LibraryID: &imprint.LibraryID,
	})
	if err == nil && existing.ID != id {
		// Merge into existing imprint
		err = h.imprintService.MergeImprints(ctx, existing.ID, id)
		if err != nil {
			return errors.WithStack(err)
		}

		// Return the target imprint
		fileCount, _ := h.imprintService.GetFileCount(ctx, existing.ID)
		response := struct {
			*models.Imprint
			FileCount int `json:"file_count"`
		}{existing, fileCount}
		return errors.WithStack(c.JSON(http.StatusOK, response))
	}

	// Simple rename
	imprint.Name = newName
	opts := UpdateImprintOptions{Columns: []string{"name"}}
	err = h.imprintService.UpdateImprint(ctx, imprint, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload
	imprint, err = h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	fileCount, _ := h.imprintService.GetFileCount(ctx, id)
	response := struct {
		*models.Imprint
		FileCount int `json:"file_count"`
	}{imprint, fileCount}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) files(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Imprint")
	}

	// Fetch the imprint to check library access
	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	files, err := h.imprintService.GetFiles(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, files))
}

func (h *handler) merge(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Imprint")
	}

	params := MergeImprintsPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the target imprint to check library access
	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Merge source imprint into target (this) imprint
	err = h.imprintService.MergeImprints(ctx, id, params.SourceID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteImprint(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Imprint")
	}

	// Fetch the imprint to check library access
	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	err = h.imprintService.DeleteImprint(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
