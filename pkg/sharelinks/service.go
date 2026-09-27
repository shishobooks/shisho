package sharelinks

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// tokenBytes is the size of a Share Link token before encoding. 256 random
// bits make guessing unrealistic, so the public routes need no rate limiter.
const tokenBytes = 32

// tokenLength is the length of an encoded token: unpadded base64url of
// tokenBytes.
var tokenLength = base64.RawURLEncoding.EncodedLen(tokenBytes)

// GenerateToken returns a new random Share Link token: 32 bytes from a
// cryptographically secure source, base64url encoded without padding or a
// prefix.
func GenerateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", errors.WithStack(err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// wellFormedToken reports whether token could have come from GenerateToken,
// so a mistyped URL is refused without a database lookup.
func wellFormedToken(token string) bool {
	if len(token) != tokenLength {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}

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
	token, err := GenerateToken()
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
// first, with creators loaded.
func (svc *Service) ListForBook(ctx context.Context, bookID int) ([]*models.ShareLink, error) {
	var links []*models.ShareLink
	err := svc.db.NewSelect().
		Model(&links).
		Relation("CreatedByUser").
		Where("sl.book_id = ?", bookID).
		Order("sl.created_at DESC", "sl.id DESC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return links, nil
}

// RetrieveByToken returns the Share Link with the given token and its
// creator, or errcodes.NotFound.
func (svc *Service) RetrieveByToken(ctx context.Context, token string) (*models.ShareLink, error) {
	return svc.retrieve(ctx, "sl.token = ?", token)
}

func (svc *Service) retrieve(ctx context.Context, where string, arg any) (*models.ShareLink, error) {
	link := &models.ShareLink{}
	err := svc.db.NewSelect().
		Model(link).
		Relation("CreatedByUser").
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
