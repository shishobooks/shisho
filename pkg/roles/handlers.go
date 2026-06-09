package roles

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type handler struct {
	roleService *Service
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	params := CreateRolePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	role, err := h.roleService.Create(ctx, params.Name, params.Permissions)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, role)
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Role")
	}

	role, err := h.roleService.Retrieve(ctx, id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, role)
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListRolesQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	roles, total, err := h.roleService.List(ctx, ListOptions(params))
	if err != nil {
		return err
	}

	resp := ListRolesResponse{Items: roles, Total: total}

	return c.JSON(http.StatusOK, resp)
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Role")
	}

	params := UpdateRolePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	var permissions *[]PermissionInput
	if len(params.Permissions) > 0 {
		permissions = &params.Permissions
	}

	role, err := h.roleService.Update(ctx, id, params.Name, permissions)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, role)
}

func (h *handler) delete(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Role")
	}

	err = h.roleService.Delete(ctx, id)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
