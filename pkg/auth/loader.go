package auth

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// LoadUserOptions selects the user LoadUser loads. Set exactly one of ID and
// Username.
type LoadUserOptions struct {
	ID *int
	// Username matches case-insensitively.
	Username *string
	// IncludeInactive also returns a deactivated user. Every authenticating
	// path leaves it false; user administration sets it.
	IncludeInactive bool
}

// LoadUser loads one user with Role, Role.Permissions and LibraryAccess, the
// relations every permission and library access check reads. It is the one
// loader behind login, session and Basic Auth authentication, API key owners,
// and user administration. When no user matches it returns an error wrapping
// sql.ErrNoRows.
func LoadUser(ctx context.Context, db bun.IDB, opts LoadUserOptions) (*models.User, error) {
	if (opts.ID == nil) == (opts.Username == nil) {
		return nil, errors.New("LoadUser needs exactly one of ID and Username")
	}

	user := &models.User{}
	q := db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Relation("LibraryAccess")
	if opts.ID != nil {
		q = q.Where("u.id = ?", *opts.ID)
	} else {
		q = q.Where("u.username = ? COLLATE NOCASE", *opts.Username)
	}
	if !opts.IncludeInactive {
		q = q.Where("u.is_active = ?", true)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return user, nil
}
