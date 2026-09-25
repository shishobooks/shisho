package roles

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ValidResources contains all valid resource names.
var ValidResources = []string{
	models.ResourceLibraries,
	models.ResourceBooks,
	models.ResourcePeople,
	models.ResourceSeries,
	models.ResourceUsers,
	models.ResourceJobs,
	models.ResourceConfig,
}

// ValidOperations contains all valid operation names.
var ValidOperations = []string{
	models.OperationRead,
	models.OperationWrite,
}

// Service handles role operations.
type Service struct {
	db *bun.DB
}

// NewService creates a new roles service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// validatePermission checks if a permission is valid.
func validatePermission(resource, operation string) error {
	validResource := false
	for _, r := range ValidResources {
		if r == resource {
			validResource = true
			break
		}
	}
	if !validResource {
		return errcodes.ValidationError("Invalid resource: " + resource)
	}

	validOperation := false
	for _, o := range ValidOperations {
		if o == operation {
			validOperation = true
			break
		}
	}
	if !validOperation {
		return errcodes.ValidationError("Invalid operation: " + operation)
	}

	return nil
}

// Create creates a new role.
func (s *Service) Create(ctx context.Context, name string, permissions []PermissionInput) (*models.Role, error) {
	// Check if name already exists
	exists, err := s.db.NewSelect().
		Model((*models.Role)(nil)).
		Where("name = ? COLLATE NOCASE", name).
		Exists(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if exists {
		return nil, errcodes.ValidationError("Role name already exists")
	}

	// Validate permissions
	for _, p := range permissions {
		if err := validatePermission(p.Resource, p.Operation); err != nil {
			return nil, err
		}
	}

	// Create role
	now := time.Now()
	role := &models.Role{
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
		IsSystem:  false,
	}

	_, err = s.db.NewInsert().Model(role).Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Create permissions
	for _, p := range permissions {
		perm := &models.Permission{
			RoleID:    role.ID,
			Resource:  p.Resource,
			Operation: p.Operation,
		}
		_, err = s.db.NewInsert().Model(perm).Exec(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Reload with relations
	return s.Retrieve(ctx, role.ID)
}

// Retrieve gets a role by ID.
func (s *Service) Retrieve(ctx context.Context, id int) (*models.Role, error) {
	role := &models.Role{}
	err := s.db.NewSelect().
		Model(role).
		Relation("Permissions").
		Where("r.id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, errcodes.NotFound("Role")
	}
	return role, nil
}

// ListOptions contains options for listing roles.
type ListOptions struct {
	Limit  int
	Offset int
}

// List returns a paginated list of roles.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]*models.Role, int, error) {
	roles := []*models.Role{}

	query := s.db.NewSelect().
		Model(&roles).
		Relation("Permissions").
		Order("r.id ASC")

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

	return roles, total, nil
}

// Update updates a role's name and/or permissions.
func (s *Service) Update(ctx context.Context, id int, name *string, permissions *[]PermissionInput) (*models.Role, error) {
	role, err := s.Retrieve(ctx, id)
	if err != nil {
		return nil, err
	}

	if role.IsSystem && name != nil {
		return nil, errcodes.Forbidden("Cannot rename system roles")
	}

	var columns []string
	if name != nil && *name != role.Name {
		// Check if new name already exists
		exists, err := s.db.NewSelect().
			Model((*models.Role)(nil)).
			Where("name = ? COLLATE NOCASE", *name).
			Where("id != ?", id).
			Exists(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if exists {
			return nil, errcodes.ValidationError("Role name already exists")
		}

		role.Name = *name
		columns = append(columns, "name")
	}

	if permissions != nil {
		for _, p := range *permissions {
			if err := validatePermission(p.Resource, p.Operation); err != nil {
				return nil, err
			}
		}
	}

	if len(columns) == 0 && permissions == nil {
		return role, nil
	}

	// Replace permissions and write the role in one transaction so a failed
	// write cannot leave new permissions under the old name (or vice versa).
	// A permissions change is a change to the role, so it bumps updated_at
	// even when the name is unchanged.
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if permissions != nil {
			_, err := tx.NewDelete().
				Model((*models.Permission)(nil)).
				Where("role_id = ?", id).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			for _, p := range *permissions {
				perm := &models.Permission{
					RoleID:    id,
					Resource:  p.Resource,
					Operation: p.Operation,
				}
				if _, err := tx.NewInsert().Model(perm).Exec(ctx); err != nil {
					return errors.WithStack(err)
				}
			}
		}

		role.UpdatedAt = time.Now()
		_, err := tx.NewUpdate().
			Model(role).
			Column(append(columns, "updated_at")...).
			WherePK().
			Exec(ctx)
		return errors.WithStack(err)
	})
	if err != nil {
		return nil, err
	}

	return s.Retrieve(ctx, id)
}

// Delete deletes a non-system role.
func (s *Service) Delete(ctx context.Context, id int) error {
	role, err := s.Retrieve(ctx, id)
	if err != nil {
		return err
	}

	if role.IsSystem {
		return errcodes.Forbidden("Cannot delete system roles")
	}

	// Check if any users have this role
	count, err := s.db.NewSelect().
		Model((*models.User)(nil)).
		Where("role_id = ?", id).
		Count(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if count > 0 {
		return errcodes.ValidationError("Cannot delete role that is assigned to users")
	}

	// Delete role (permissions are deleted via CASCADE)
	_, err = s.db.NewDelete().
		Model((*models.Role)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
