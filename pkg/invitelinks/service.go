// Package invitelinks serves Invite Links: single-use, expiring links an
// admin creates to add one user, with the role and Library access that user
// gets. Shisho never sends them; the admin passes the URL on. Whoever opens
// one picks their own username and password.
package invitelinks

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/linktoken"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/users"
	"github.com/uptrace/bun"
)

// errUnavailable is the one answer for an invite that can't be accepted,
// whatever the reason, so the person holding it is never told which. It is
// the same as retrieve's not-found error.
func errUnavailable() error {
	return errcodes.NotFound("Invite Link")
}

// Service reads and writes Invite Links.
type Service struct {
	db    *bun.DB
	users *users.Service
}

// NewService returns a Service over db.
func NewService(db *bun.DB) *Service {
	return &Service{db: db, users: users.NewService(db)}
}

// CreateOptions describes a new Invite Link.
type CreateOptions struct {
	CreatedByUserID  int
	RoleID           int
	AllLibraryAccess bool
	LibraryIDs       []int
	ExpiresAt        time.Time
}

// Create inserts an Invite Link with a fresh token and returns it with its
// relations loaded. An unknown role or Library is a validation error.
func (svc *Service) Create(ctx context.Context, opts CreateOptions) (*models.InviteLink, error) {
	roleExists, err := svc.db.NewSelect().Model((*models.Role)(nil)).Where("id = ?", opts.RoleID).Exists(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !roleExists {
		return nil, errcodes.ValidationError("That role doesn't exist.")
	}

	var libraryIDs []int
	if !opts.AllLibraryAccess {
		libraryIDs = slices.Compact(slices.Sorted(slices.Values(opts.LibraryIDs)))
	}
	if len(libraryIDs) > 0 {
		count, err := svc.db.NewSelect().Model((*models.Library)(nil)).Where("l.id IN (?)", bun.List(libraryIDs)).Count(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if count != int64(len(libraryIDs)) {
			return nil, errcodes.ValidationError("One of those libraries doesn't exist.")
		}
	}

	token, err := linktoken.Generate()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	link := &models.InviteLink{
		CreatedAt:        now,
		UpdatedAt:        now,
		Token:            token,
		CreatedByUserID:  opts.CreatedByUserID,
		RoleID:           opts.RoleID,
		AllLibraryAccess: opts.AllLibraryAccess,
		ExpiresAt:        opts.ExpiresAt,
	}
	err = svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(link).Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
		for _, libraryID := range libraryIDs {
			row := &models.InviteLinkLibrary{InviteLinkID: link.ID, LibraryID: libraryID}
			if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return svc.retrieve(ctx, svc.db, "il.id = ?", link.ID)
}

// ListPending returns the invites that can be accepted now, newest first.
// One whose creator can no longer add users is left out, since its link
// doesn't work.
func (svc *Service) ListPending(ctx context.Context) ([]*models.InviteLink, error) {
	var links []*models.InviteLink
	err := svc.selectWithRelations(svc.db, &links).
		Where("il.used_at IS NULL").
		Where("il.revoked_at IS NULL").
		Where("il.expires_at > ?", time.Now()).
		Order("il.created_at DESC", "il.id DESC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return slices.DeleteFunc(links, func(link *models.InviteLink) bool { return !creatorCanInvite(link) }), nil
}

// Revoke stops a pending invite. Revoking one already revoked keeps its
// original time; one already used can't be revoked. It returns
// errcodes.NotFound when no such invite exists.
func (svc *Service) Revoke(ctx context.Context, id int) error {
	link, err := svc.retrieve(ctx, svc.db, "il.id = ?", id)
	if err != nil {
		return err
	}
	if link.UsedAt != nil {
		return errcodes.InvalidState("This invite has already been used.")
	}
	now := time.Now()
	_, err = svc.db.NewUpdate().Model((*models.InviteLink)(nil)).
		Set("revoked_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("revoked_at IS NULL").
		Where("used_at IS NULL").
		Exec(ctx)
	return errors.WithStack(err)
}

// Lookup reports whether the invite with token can be accepted, and who sent
// it. The sender is left out when they can no longer add users, since asking
// them for a new invite wouldn't help. A token that matches no invite is
// errUnavailable().
func (svc *Service) Lookup(ctx context.Context, token string) (InviteResponse, error) {
	if !linktoken.WellFormed(token) {
		return InviteResponse{}, errUnavailable()
	}
	link, err := svc.retrieve(ctx, svc.db, "il.token = ?", token)
	if err != nil {
		return InviteResponse{}, err
	}
	resp := InviteResponse{Valid: resolves(link, time.Now())}
	if creatorCanInvite(link) {
		resp.InvitedBy = link.CreatedByUser.Username
	}
	return resp, nil
}

// AcceptOptions is the account the person accepting an invite chooses.
type AcceptOptions struct {
	Username string
	Email    *string
	Password string
}

// Accept creates the invite's user with its role and Library access and
// marks it used, in one transaction, and returns the new user's id. An
// invite that can't be accepted, including one another request used first,
// is errUnavailable(). A taken username or email leaves the invite pending.
func (svc *Service) Accept(ctx context.Context, token string, opts AcceptOptions) (int, error) {
	if !linktoken.WellFormed(token) {
		return 0, errUnavailable()
	}
	// Hash before the transaction: bcrypt takes a while, and the database
	// has a single connection that every other request waits on.
	passwordHash, err := auth.HashPassword(opts.Password)
	if err != nil {
		return 0, err
	}
	var userID int
	err = svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		link, err := svc.retrieve(ctx, tx, "il.token = ?", token)
		if err != nil {
			return err
		}
		now := time.Now()
		if !resolves(link, now) {
			return errUnavailable()
		}

		userID, err = svc.users.CreateInTx(ctx, tx, users.CreateUserOptions{
			Username:         opts.Username,
			Email:            opts.Email,
			Password:         opts.Password,
			RoleID:           link.RoleID,
			AllLibraryAccess: link.AllLibraryAccess,
			LibraryIDs:       link.LibraryIDs(),
		}, passwordHash)
		if err != nil {
			return err
		}

		// The conditions repeat the checks above, so of two requests racing
		// on one invite only one marks it used and the other rolls back.
		// Today the database's single connection already runs the two
		// transactions one after the other; this guard is what keeps the
		// invite single-use if that changes.
		result, err := tx.NewUpdate().Model((*models.InviteLink)(nil)).
			Set("used_at = ?", now).
			Set("used_by_user_id = ?", userID).
			Set("updated_at = ?", now).
			Where("id = ?", link.ID).
			Where("used_at IS NULL").
			Where("revoked_at IS NULL").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return errors.WithStack(err)
		}
		if affected == 0 {
			return errUnavailable()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return userID, nil
}

// resolves reports whether link can be accepted at now: it is pending and its
// creator can still add users.
func resolves(link *models.InviteLink, now time.Time) bool {
	return link.Pending(now) && creatorCanInvite(link)
}

// creatorCanInvite reports whether the invite's creator is active and can
// still add users. A deleted creator takes the invite with it. It needs
// CreatedByUser loaded with its role's permissions; a missing creator fails
// closed.
func creatorCanInvite(link *models.InviteLink) bool {
	creator := link.CreatedByUser
	return creator != nil && creator.IsActive && creator.HasPermission(models.ResourceUsers, models.OperationWrite)
}

func (svc *Service) selectWithRelations(db bun.IDB, model any) *bun.SelectQuery {
	return db.NewSelect().
		Model(model).
		Relation("Role").
		Relation("Libraries.Library").
		Relation("CreatedByUser").
		Relation("CreatedByUser.Role").
		Relation("CreatedByUser.Role.Permissions")
}

// retrieve loads one invite with the relations resolves and the response
// need, or errcodes.NotFound.
func (svc *Service) retrieve(ctx context.Context, db bun.IDB, where string, arg any) (*models.InviteLink, error) {
	link := &models.InviteLink{}
	err := svc.selectWithRelations(db, link).Where(where, arg).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errUnavailable()
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return link, nil
}
