package users

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// Service handles user operations.
type Service struct {
	db *bun.DB
}

// NewService creates a new users service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// CreateUserOptions contains options for creating a user.
type CreateUserOptions struct {
	Username             string
	Email                *string
	Password             string
	RoleID               int
	LibraryIDs           []int
	AllLibraryAccess     bool
	RequirePasswordReset bool
}

// Create creates a new user.
func (s *Service) Create(ctx context.Context, opts CreateUserOptions) (*models.User, error) {
	// Check if username already exists
	exists, err := s.db.NewSelect().
		Model((*models.User)(nil)).
		Where("username = ? COLLATE NOCASE", opts.Username).
		Exists(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if exists {
		return nil, errcodes.ValidationError("Username already exists")
	}

	// Check if email already exists (if provided)
	if opts.Email != nil && *opts.Email != "" {
		exists, err = s.db.NewSelect().
			Model((*models.User)(nil)).
			Where("email = ? COLLATE NOCASE", *opts.Email).
			Exists(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if exists {
			return nil, errcodes.ValidationError("Email already exists")
		}
	}

	// Verify role exists
	roleExists, err := s.db.NewSelect().
		Model((*models.Role)(nil)).
		Where("id = ?", opts.RoleID).
		Exists(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !roleExists {
		return nil, errcodes.ValidationError("Invalid role ID")
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(opts.Password)
	if err != nil {
		return nil, err
	}

	// Create user
	user := &models.User{
		Username:           opts.Username,
		Email:              opts.Email,
		PasswordHash:       hashedPassword,
		RoleID:             opts.RoleID,
		IsActive:           true,
		MustChangePassword: opts.RequirePasswordReset,
	}

	_, err = s.db.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Create library access
	if opts.AllLibraryAccess {
		access := &models.UserLibraryAccess{
			UserID:    user.ID,
			LibraryID: nil, // null = all libraries
		}
		_, err = s.db.NewInsert().Model(access).Exec(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		for _, libraryID := range opts.LibraryIDs {
			id := libraryID
			access := &models.UserLibraryAccess{
				UserID:    user.ID,
				LibraryID: &id,
			}
			_, err = s.db.NewInsert().Model(access).Exec(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}

	// Reload with relations
	return s.Retrieve(ctx, user.ID)
}

// Retrieve gets a user by ID.
func (s *Service) Retrieve(ctx context.Context, id int) (*models.User, error) {
	user := &models.User{}
	err := s.db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Relation("LibraryAccess").
		Where("u.id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, errcodes.NotFound("User")
	}
	return user, nil
}

// ListOptions contains options for listing users.
type ListOptions struct {
	Limit  int
	Offset int
}

// List returns a paginated list of users.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]*models.User, int, error) {
	users := []*models.User{}

	query := s.db.NewSelect().
		Model(&users).
		Relation("Role").
		Relation("LibraryAccess").
		Order("u.id ASC")

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	total, err := query.ScanAndCount(ctx)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return users, total, nil
}

// UpdateOptions contains options for updating a user.
type UpdateOptions struct {
	Columns             []string
	UpdateLibraryAccess bool
	AllLibraryAccess    bool
	LibraryIDs          []int
}

// Update updates a user.
func (s *Service) Update(ctx context.Context, user *models.User, opts UpdateOptions) error {
	if len(opts.Columns) > 0 {
		opts.Columns = append(opts.Columns, "updated_at")
		_, err := s.db.NewUpdate().
			Model(user).
			Column(opts.Columns...).
			WherePK().
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if opts.UpdateLibraryAccess {
		// Delete existing access
		_, err := s.db.NewDelete().
			Model((*models.UserLibraryAccess)(nil)).
			Where("user_id = ?", user.ID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create new access
		if opts.AllLibraryAccess {
			access := &models.UserLibraryAccess{
				UserID:    user.ID,
				LibraryID: nil,
			}
			_, err = s.db.NewInsert().Model(access).Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		} else {
			for _, libraryID := range opts.LibraryIDs {
				id := libraryID
				access := &models.UserLibraryAccess{
					UserID:    user.ID,
					LibraryID: &id,
				}
				_, err = s.db.NewInsert().Model(access).Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	}

	return nil
}

// ResetPassword changes a user's password.
func (s *Service) ResetPassword(ctx context.Context, userID int, newPassword string, requirePasswordReset bool) error {
	hashedPassword, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	_, err = s.db.NewUpdate().
		Model((*models.User)(nil)).
		Set("password_hash = ?", hashedPassword).
		Set("must_change_password = ?", requirePasswordReset).
		Set("updated_at = CURRENT_TIMESTAMP").
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// VerifyPassword checks if the password is correct for a user.
func (s *Service) VerifyPassword(ctx context.Context, userID int, password string) (bool, error) {
	user := &models.User{}
	err := s.db.NewSelect().
		Model(user).
		Column("password_hash").
		Where("id = ?", userID).
		Scan(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return auth.CheckPassword(password, user.PasswordHash), nil
}

// Deactivate deactivates a user (soft delete).
func (s *Service) Deactivate(ctx context.Context, userID int) error {
	_, err := s.db.NewUpdate().
		Model((*models.User)(nil)).
		Set("is_active = ?", false).
		Set("updated_at = CURRENT_TIMESTAMP").
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// CountUsers returns the total number of users.
func (s *Service) CountUsers(ctx context.Context) (int, error) {
	count, err := s.db.NewSelect().Model((*models.User)(nil)).Count(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return count, nil
}
