package imprints

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
)

type handler struct {
	imprintService *Service
	aliasService   *aliases.Service
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

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	fileCount, err := h.imprintService.GetFileCount(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.ImprintConfig, id)

	response := struct {
		*models.Imprint
		FileCount int      `json:"file_count"`
		Aliases   []string `json:"aliases"`
	}{imprint, fileCount, aliasList}

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

	type ImprintWithCount struct {
		*models.Imprint
		FileCount int      `json:"file_count"`
		Aliases   []string `json:"aliases"`
	}
	result := make([]ImprintWithCount, len(imprints))
	for i, imp := range imprints {
		fileCount, _ := h.imprintService.GetFileCount(ctx, imp.ID)
		aliasList, _ := h.aliasService.ListAliases(ctx, aliases.ImprintConfig, imp.ID)
		result[i] = ImprintWithCount{imp, fileCount, aliasList}
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

	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	nameChanged := false
	var oldName string
	if params.Name != nil && *params.Name != imprint.Name {
		newName := strings.TrimSpace(*params.Name)
		if newName == "" {
			return errcodes.ValidationError("Imprint name cannot be empty")
		}

		existing, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
			Name:      &newName,
			LibraryID: &imprint.LibraryID,
		})
		if err == nil && existing.ID != id {
			err = h.imprintService.MergeImprints(ctx, existing.ID, id)
			if err != nil {
				return errors.WithStack(err)
			}

			fileCount, _ := h.imprintService.GetFileCount(ctx, existing.ID)
			aliasList, _ := h.aliasService.ListAliases(ctx, aliases.ImprintConfig, existing.ID)
			response := struct {
				*models.Imprint
				FileCount int      `json:"file_count"`
				Aliases   []string `json:"aliases"`
			}{existing, fileCount, aliasList}
			return errors.WithStack(c.JSON(http.StatusOK, response))
		}

		oldName = imprint.Name
		imprint.Name = newName
		opts := UpdateImprintOptions{Columns: []string{"name"}}
		err = h.imprintService.UpdateImprint(ctx, imprint, opts)
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
		if err := h.aliasService.SyncAliases(ctx, aliases.ImprintConfig, id, imprint.LibraryID, syncList); err != nil {
			return errors.WithStack(err)
		}
	} else if nameChanged {
		_ = h.aliasService.RemoveAlias(ctx, aliases.ImprintConfig, id, imprint.Name)
		log := logger.FromContext(ctx)
		if err := h.aliasService.AddAlias(ctx, aliases.ImprintConfig, id, oldName, imprint.LibraryID); err != nil {
			log.Warn("failed to add old name as alias after rename", logger.Data{"imprint_id": id, "old_name": oldName, "error": err.Error()})
		}
	}

	imprint, err = h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	fileCount, _ := h.imprintService.GetFileCount(ctx, id)
	aliasList, _ := h.aliasService.ListAliases(ctx, aliases.ImprintConfig, id)
	response := struct {
		*models.Imprint
		FileCount int      `json:"file_count"`
		Aliases   []string `json:"aliases"`
	}{imprint, fileCount, aliasList}

	return errors.WithStack(c.JSON(http.StatusOK, response))
}

func (h *handler) files(c echo.Context) error {
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

	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(imprint.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

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

	imprint, err := h.imprintService.RetrieveImprint(ctx, RetrieveImprintOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

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
