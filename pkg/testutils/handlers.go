package testutils

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type handler struct {
	db *bun.DB
}

// createUserRequest is the request body for creating a test user.
type createUserRequest struct {
	Username string  `json:"username" validate:"required"`
	Password string  `json:"password" validate:"required"`
	Email    *string `json:"email"`
}

// createUserResponse is the response body for creating a test user.
type createUserResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// createUser creates a test user with admin role.
// POST /test/users.
func (h *handler) createUser(c echo.Context) error {
	ctx := c.Request().Context()

	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Username and password are required")
	}

	// Get admin role
	role := &models.Role{}
	err := h.db.NewSelect().
		Model(role).
		Where("name = ?", models.RoleAdmin).
		Scan(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get admin role")
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		return errors.Wrap(err, "failed to hash password")
	}

	// Create user
	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashedPassword,
		RoleID:       role.ID,
		IsActive:     true,
	}

	_, err = h.db.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create user")
	}

	// Grant access to all libraries
	access := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: nil, // null = all libraries
	}
	_, err = h.db.NewInsert().Model(access).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to grant library access")
	}

	return c.JSON(http.StatusCreated, createUserResponse{
		ID:       user.ID,
		Username: user.Username,
	})
}

// deleteAllUsersResponse is the response body for deleting all users.
type deleteAllUsersResponse struct {
	Deleted int `json:"deleted"`
}

// deleteAllUsers deletes all users from the database.
// DELETE /test/users.
func (h *handler) deleteAllUsers(c echo.Context) error {
	ctx := c.Request().Context()

	// Delete library access first (foreign key constraint)
	_, err := h.db.NewDelete().
		Model((*models.UserLibraryAccess)(nil)).
		Where("1=1").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to delete library access")
	}

	// Delete all users
	result, err := h.db.NewDelete().
		Model((*models.User)(nil)).
		Where("1=1").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to delete users")
	}

	deleted, _ := result.RowsAffected()

	return c.JSON(http.StatusOK, deleteAllUsersResponse{
		Deleted: int(deleted),
	})
}
