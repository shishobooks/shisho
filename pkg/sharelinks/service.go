package sharelinks

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/linktoken"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// CreateOptions describes a new Share Link.
type CreateOptions struct {
	BookID          int
	CreatedByUserID int
	Label           *string
	ExpiresAt       *time.Time
}

// Create inserts a Share Link with a fresh token and returns it with its
// creator loaded.
func (svc *Service) Create(ctx context.Context, opts CreateOptions) (*models.ShareLink, error) {
	token, err := linktoken.Generate()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	link := &models.ShareLink{
		CreatedAt:       now,
		UpdatedAt:       now,
		Token:           token,
		BookID:          opts.BookID,
		CreatedByUserID: opts.CreatedByUserID,
		Label:           opts.Label,
		ExpiresAt:       opts.ExpiresAt,
	}
	if _, err := svc.db.NewInsert().Model(link).Exec(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return svc.retrieve(ctx, "sl.id = ?", link.ID)
}

// ListForBook returns every Share Link on a book from every creator, newest
// first, with creators and their library access loaded.
func (svc *Service) ListForBook(ctx context.Context, bookID int) ([]*models.ShareLink, error) {
	var links []*models.ShareLink
	err := svc.db.NewSelect().
		Model(&links).
		Relation("CreatedByUser.LibraryAccess").
		Where("sl.book_id = ?", bookID).
		Order("sl.created_at DESC", "sl.id DESC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return links, nil
}

// RetrieveForBook returns the Share Link with the given id on the given book,
// or errcodes.NotFound when the link does not exist or belongs to another
// book.
func (svc *Service) RetrieveForBook(ctx context.Context, bookID, linkID int) (*models.ShareLink, error) {
	link, err := svc.retrieve(ctx, "sl.id = ?", linkID)
	if err != nil {
		return nil, err
	}
	if link.BookID != bookID {
		return nil, errcodes.NotFound("Share Link")
	}
	return link, nil
}

// Revoke stamps revoked_at on the link. Revocation is permanent, so a link
// that is already revoked keeps its original time, even when two revokes
// race. It returns the link as stored.
func (svc *Service) Revoke(ctx context.Context, link *models.ShareLink) (*models.ShareLink, error) {
	if link.RevokedAt == nil {
		now := time.Now()
		_, err := svc.db.NewUpdate().
			Model((*models.ShareLink)(nil)).
			Set("revoked_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", link.ID).
			Where("revoked_at IS NULL").
			Exec(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return svc.retrieve(ctx, "sl.id = ?", link.ID)
}

// Delete removes a Share Link whatever its state.
func (svc *Service) Delete(ctx context.Context, link *models.ShareLink) error {
	_, err := svc.db.NewDelete().
		Model(link).
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// RecordOpen counts one opening of the recipient page and sets last used.
func (svc *Service) RecordOpen(ctx context.Context, linkID int) error {
	return svc.recordAccess(ctx, linkID, "open_count")
}

// RecordDownload counts one file download and sets last used.
func (svc *Service) RecordDownload(ctx context.Context, linkID int) error {
	return svc.recordAccess(ctx, linkID, "download_count")
}

// recordAccess increments counter in place, so concurrent recipients never
// lose a count. updated_at is left alone: it tracks changes the sharer made,
// and last_accessed_at tracks use.
func (svc *Service) recordAccess(ctx context.Context, linkID int, counter string) error {
	_, err := svc.db.NewUpdate().
		Model((*models.ShareLink)(nil)).
		Set("? = ? + 1", bun.Ident(counter), bun.Ident(counter)).
		Set("last_accessed_at = ?", time.Now()).
		Where("id = ?", linkID).
		Exec(ctx)
	return errors.WithStack(err)
}

// RetrieveByToken returns the Share Link with the given token, its creator,
// and the creator's library access, or errcodes.NotFound.
func (svc *Service) RetrieveByToken(ctx context.Context, token string) (*models.ShareLink, error) {
	return svc.retrieve(ctx, "sl.token = ?", token)
}

// retrieve loads one link with its creator and the creator's library access,
// which PausedReason needs.
func (svc *Service) retrieve(ctx context.Context, where string, arg any) (*models.ShareLink, error) {
	link := &models.ShareLink{}
	err := svc.db.NewSelect().
		Model(link).
		Relation("CreatedByUser.LibraryAccess").
		Where(where, arg).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("Share Link")
		}
		return nil, errors.WithStack(err)
	}
	return link, nil
}

// BookLibraryID returns the library a book belongs to, or errcodes.NotFound.
func (svc *Service) BookLibraryID(ctx context.Context, bookID int) (int, error) {
	var libraryID int
	err := svc.db.NewSelect().
		Table("books").
		Column("library_id").
		Where("id = ?", bookID).
		Scan(ctx, &libraryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errcodes.NotFound("Book")
		}
		return 0, errors.WithStack(err)
	}
	return libraryID, nil
}
