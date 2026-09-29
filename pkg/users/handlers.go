package users

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
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

	resp := ListUsersResponse{Items: users, Total: total}

	return c.JSON(http.StatusOK, resp)
}

// directory lists active users' ids and usernames for the list sharing
// dialog. It needs only authentication.
func (h *handler) directory(c echo.Context) error {
	entries, err := h.userService.ListDirectory(c.Request().Context())
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, entries))
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

	currentUser, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	isSelf := currentUser.ID == id

	if isSelf {
		// Users in forced-reset flow can set a new password without re-entering
		// their temporary password. This is intentional: the admin has already
		// authenticated the user out-of-band by giving them temporary credentials,
		// so requiring the temp password again adds friction without meaningful
		// security benefit. The risk surface is limited to stolen JWT sessions
		// where the attacker would already have access to the account.
		if !currentUser.MustChangePassword {
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
		}
	} else if !currentUser.HasPermission(models.ResourceUsers, models.OperationWrite) {
		// Non-self reset requires users:write permission
		return errcodes.Forbidden("You don't have permission to reset other users' passwords")
	}

	requirePasswordReset := false
	if !isSelf {
		requirePasswordReset = params.RequirePasswordReset
	}

	err = h.userService.ResetPassword(ctx, id, params.NewPassword, requirePasswordReset)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deactivate(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("User")
	}

	// Prevent deactivating yourself
	currentUser, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	if currentUser.ID == id {
		return errcodes.ValidationError("You cannot deactivate your own account")
	}

	err = h.userService.Deactivate(ctx, id)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
