package auth

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
	"golang.org/x/text/unicode/norm"
)

// LoadUserOptions selects the user LoadUser loads. Set exactly one of ID and
// Username.
type LoadUserOptions struct {
	ID *int
	// Username matches the way MatchUsername does.
	Username *string
	// IncludeInactive also returns a deactivated user. Every authenticating
	// path leaves it false; user administration sets it.
	IncludeInactive bool
}

// LoadUser loads one user with Role, Role.Permissions and LibraryAccess, the
// relations every permission and library access check reads. It is the one
// loader behind login, session and Basic Auth authentication, Device owners,
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
		q = MatchUsername(q, *opts.Username)
	}
	if !opts.IncludeInactive {
		q = q.Where("u.is_active = ?", true)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return user, nil
}

// MatchUsername narrows q, a select from users aliased u, to the user signing
// in as typed. Usernames compare in every letter's case (models.UsernameKey),
// with three tiers tried in order, so typed reaches at most one user:
//
//  1. The username exactly as typed, or in composed form: new usernames are
//     stored composed, and some keyboards send an accent as a separate mark.
//  2. The username's key, so any case of any letter matches.
//  3. For a user with no key, the username without English letters' case.
//
// Users have no key only when they upgraded holding usernames that differ
// just in the case of other letters ("Émile" and "émile"), which the old
// ASCII-only uniqueness allowed. Tier 1 signs each of them in by their exact
// username, and tier 3 keeps the English-letter matching they had before.
// users.Service refuses any new username that folds to theirs, so a lower
// tier never reaches a different user than a higher one.
func MatchUsername(q *bun.SelectQuery, typed string) *bun.SelectQuery {
	composed := norm.NFC.String(typed)
	key := models.UsernameKey(typed)
	return q.
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("u.username = ? COLLATE BINARY", typed).
				WhereOr("u.username = ? COLLATE BINARY", composed).
				WhereOr("u.username_key = ?", key).
				WhereOr("u.username_key IS NULL AND (u.username = ? COLLATE NOCASE OR u.username = ? COLLATE NOCASE)", typed, composed)
		}).
		OrderExpr("CASE WHEN u.username COLLATE BINARY IN (?, ?) THEN 0 WHEN u.username_key = ? THEN 1 ELSE 2 END", typed, composed, key).
		Limit(1)
}
