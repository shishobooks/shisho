package users

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	userService *Service
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	params := CreateUserPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := h.userService.Create(ctx, CreateUserOptions(params))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, user)
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("User")
	}

	user, err := h.userService.Retrieve(ctx, id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListUsersQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	users, total, err := h.userService.List(ctx, ListOptions(params))
	if err != nil {
		return err
	}

	resp := struct {
		Users []*models.User `json:"users"`
		Total int            `json:"total"`
	}{users, total}

	return c.JSON(http.StatusOK, resp)
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("User")
	}

	params := UpdateUserPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := h.userService.Retrieve(ctx, id)
	if err != nil {
		return err
	}

	opts := UpdateOptions{Columns: []string{}}

	if params.Username != nil && *params.Username != user.Username {
		user.Username = *params.Username
		opts.Columns = append(opts.Columns, "username")
	}
	if params.Email != nil {
		user.Email = params.Email
		opts.Columns = append(opts.Columns, "email")
	}
	if params.RoleID != nil && *params.RoleID != user.RoleID {
		user.RoleID = *params.RoleID
		opts.Columns = append(opts.Columns, "role_id")
	}
	if params.IsActive != nil && *params.IsActive != user.IsActive {
		user.IsActive = *params.IsActive
		opts.Columns = append(opts.Columns, "is_active")
	}

	if params.LibraryIDs != nil || params.AllLibraryAccess != nil {
		opts.UpdateLibraryAccess = true
		if params.AllLibraryAccess != nil && *params.AllLibraryAccess {
			opts.AllLibraryAccess = true
		} else if params.LibraryIDs != nil {
			opts.LibraryIDs = *params.LibraryIDs
		}
	}

	err = h.userService.Update(ctx, user, opts)
	if err != nil {
		return err
	}

	user, err = h.userService.Retrieve(ctx, id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, user)
}

func (h *handler) resetPassword(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("User")
	}

	params := ResetPasswordPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Check if this is a self-reset
	currentUserID, _ := c.Get("user_id").(int)
	isSelf := currentUserID == id

	if isSelf {
		// Self-reset requires current password
		if params.CurrentPassword == nil || *params.CurrentPassword == "" {
			return errcodes.ValidationError("Current password is required when resetting your own password")
		}

		valid, err := h.userService.VerifyPassword(ctx, id, *params.CurrentPassword)
		if err != nil {
			return err
		}
		if !valid {
			return errcodes.ValidationError("Current password is incorrect")
		}
	} else {
		// Non-self reset requires users:write permission
		user, ok := c.Get("user").(*models.User)
		if !ok {
			return errcodes.Unauthorized("Authentication required")
		}
		if !user.HasPermission(models.ResourceUsers, models.OperationWrite) {
			return errcodes.Forbidden("You don't have permission to reset other users' passwords")
		}
	}

	err = h.userService.ResetPassword(ctx, id, params.NewPassword)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Password reset successfully"})
}

func (h *handler) deactivate(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("User")
	}

	// Prevent deactivating yourself
	currentUserID, _ := c.Get("user_id").(int)
	if currentUserID == id {
		return errcodes.ValidationError("You cannot deactivate your own account")
	}

	err = h.userService.Deactivate(ctx, id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "User deactivated successfully"})
}
