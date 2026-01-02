package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Permission resources.
const (
	ResourceLibraries = "libraries"
	ResourceBooks     = "books"
	ResourcePeople    = "people"
	ResourceSeries    = "series"
	ResourceUsers     = "users"
	ResourceJobs      = "jobs"
	ResourceConfig    = "config"
)

// Permission operations.
const (
	OperationRead  = "read"
	OperationWrite = "write"
)

// Predefined role names.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type Role struct {
	bun.BaseModel `bun:"table:roles,alias:r" tstype:"-"`

	ID          int           `bun:",pk,nullzero" json:"id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Name        string        `bun:",nullzero" json:"name"`
	IsSystem    bool          `json:"is_system"`
	Permissions []*Permission `bun:"rel:has-many,join:id=role_id" json:"permissions,omitempty" tstype:"Permission[]"`
}

type Permission struct {
	bun.BaseModel `bun:"table:permissions,alias:p" tstype:"-"`

	ID        int    `bun:",pk,nullzero" json:"id"`
	RoleID    int    `json:"role_id"`
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
}

// HasPermission checks if the role has a specific permission.
func (r *Role) HasPermission(resource, operation string) bool {
	for _, p := range r.Permissions {
		if p.Resource == resource && p.Operation == operation {
			return true
		}
	}
	return false
}
