package libraries

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	libraryService *Service
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := CreateLibraryPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	library := &models.Library{
		Name:         params.Name,
		LibraryPaths: make([]*models.LibraryPath, 0, len(params.LibraryPaths)),
	}
	for _, path := range params.LibraryPaths {
		library.LibraryPaths = append(library.LibraryPaths, &models.LibraryPath{
			Filepath: path,
		})
	}

	err := h.libraryService.CreateLibrary(ctx, library)
	if err != nil {
		return errors.WithStack(err)
	}

	library, err = h.libraryService.RetrieveLibrary(ctx, RetrieveLibraryOptions{
		ID: &library.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, library))
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	library, err := h.libraryService.RetrieveLibrary(ctx, RetrieveLibraryOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, library))
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	// Bind params.
	params := ListLibrariesQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	libraries, total, err := h.libraryService.ListLibrariesWithTotal(ctx, ListLibrariesOptions{
		Limit:          &params.Limit,
		Offset:         &params.Offset,
		IncludeDeleted: params.Deleted,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	resp := struct {
		Libraries []*models.Library `json:"libraries"`
		Total     int               `json:"total"`
	}{libraries, total}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Library")
	}

	// Bind params.
	params := UpdateLibraryPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the library.
	library, err := h.libraryService.RetrieveLibrary(ctx, RetrieveLibraryOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Keep track of what's been changed.
	opts := UpdateLibraryOptions{Columns: []string{}}

	if params.Name != nil && *params.Name != library.Name {
		library.Name = *params.Name
		opts.Columns = append(opts.Columns, "name")
	}
	if params.LibraryPaths != nil {
		library.LibraryPaths = make([]*models.LibraryPath, 0, len(params.LibraryPaths))
		for _, path := range params.LibraryPaths {
			library.LibraryPaths = append(library.LibraryPaths, &models.LibraryPath{
				Filepath: path,
			})
		}
		opts.UpdateLibraryPaths = true
	}
	if params.Deleted != nil && (*params.Deleted && library.DeletedAt == nil || !*params.Deleted && library.DeletedAt != nil) {
		if *params.Deleted {
			library.DeletedAt = pointerutil.Time(time.Now())
		} else {
			library.DeletedAt = nil
		}
		opts.Columns = append(opts.Columns, "deleted_at")
	}

	// Update the model.
	err = h.libraryService.UpdateLibrary(ctx, library, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload the model.
	library, err = h.libraryService.RetrieveLibrary(ctx, RetrieveLibraryOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, library))
}
