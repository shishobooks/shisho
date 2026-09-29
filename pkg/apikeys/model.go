package apikeys

import (
	"time"

	"github.com/uptrace/bun"
)

// APIKey represents a user's API key for programmatic access.
type APIKey struct {
	bun.BaseModel `bun:"table:api_keys,alias:ak" json:"-"`

	ID             string     `bun:"id,pk" json:"id"`
	UserID         int        `bun:"user_id,notnull" json:"user_id"`
	Name           string     `bun:"name,notnull" json:"name"`
	Key            string     `bun:"key,notnull,unique" json:"key"`
	CreatedAt      time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull" json:"updated_at"`
	LastAccessedAt *time.Time `bun:"last_accessed_at" json:"last_accessed_at"`

	Permissions []*APIKeyPermission `bun:"rel:has-many,join:id=api_key_id" json:"permissions"`
}

// APIKeyPermission represents a permission granted to an API key.
type APIKeyPermission struct {
	bun.BaseModel `bun:"table:api_key_permissions,alias:akp" json:"-"`

	ID         string    `bun:"id,pk" json:"id"`
	APIKeyID   string    `bun:"api_key_id,notnull" json:"api_key_id"`
	Permission string    `bun:"permission,notnull" json:"permission"`
	CreatedAt  time.Time `bun:"created_at,notnull" json:"created_at"`
}

// APIKeyShortURL represents a temporary short URL for eReader setup.
type APIKeyShortURL struct {
	bun.BaseModel `bun:"table:api_key_short_urls,alias:aksu" json:"-"`

	ID        string    `bun:"id,pk" json:"id"`
	APIKeyID  string    `bun:"api_key_id,notnull" json:"api_key_id"`
	ShortCode string    `bun:"short_code,notnull,unique" json:"short_code"`
	ExpiresAt time.Time `bun:"expires_at,notnull" json:"expires_at"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`

	APIKey *APIKey `bun:"rel:belongs-to,join:api_key_id=id" json:"-"`
}

// PermissionEReaderBrowser is the permission for accessing the eReader browser UI.
const PermissionEReaderBrowser = "ereader_browser"

// PermissionKoboSync is the permission for Kobo sync API access.
const PermissionKoboSync = "kobo_sync"

// HasPermission checks if the API key has a specific permission.
func (ak *APIKey) HasPermission(permission string) bool {
	for _, p := range ak.Permissions {
		if p.Permission == permission {
			return true
		}
	}
	return false
}

// PermissionStrings returns a list of permission strings.
func (ak *APIKey) PermissionStrings() []string {
	perms := make([]string, len(ak.Permissions))
	for i, p := range ak.Permissions {
		perms[i] = p.Permission
	}
	return perms
}
