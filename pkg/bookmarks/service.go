package bookmarks

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// Service reads and writes users' Bookmarks. Every method is scoped to one
// user, so no user reaches another's.
type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// FileTarget is what a handler needs to know about a File before touching
// its Bookmarks.
type FileTarget struct {
	LibraryID int
	FileType  string
}

// RetrieveFileTarget returns the library and type of fileID, or
// sql.ErrNoRows when there is no such File.
func (svc *Service) RetrieveFileTarget(ctx context.Context, fileID int) (FileTarget, error) {
	var target FileTarget
	err := svc.db.NewSelect().
		Model((*models.File)(nil)).
		Column("f.library_id", "f.file_type").
		Where("f.id = ?", fileID).
		Scan(ctx, &target.LibraryID, &target.FileType)
	if errors.Is(err, sql.ErrNoRows) {
		return FileTarget{}, err
	}
	return target, errors.WithStack(err)
}

// List returns userID's Bookmarks in fileID in time order.
func (svc *Service) List(ctx context.Context, userID, fileID int) ([]*models.Bookmark, error) {
	bookmarks := []*models.Bookmark{}
	err := svc.db.NewSelect().
		Model(&bookmarks).
		Where("bm.user_id = ?", userID).
		Where("bm.file_id = ?", fileID).
		Order("bm.seconds ASC", "bm.id ASC").
		Scan(ctx)
	return bookmarks, errors.WithStack(err)
}

// Create adds a Bookmark for userID at seconds into fileID.
func (svc *Service) Create(ctx context.Context, userID, fileID int, seconds float64, note *string) (*models.Bookmark, error) {
	now := time.Now()
	bookmark := &models.Bookmark{
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userID,
		FileID:    fileID,
		Seconds:   seconds,
		Note:      normalizeNote(note),
	}
	_, err := svc.db.NewInsert().Model(bookmark).Exec(ctx)
	return bookmark, errors.WithStack(err)
}

// Retrieve returns userID's Bookmark id, or sql.ErrNoRows when userID has no
// such Bookmark, whether or not another user does.
func (svc *Service) Retrieve(ctx context.Context, userID, id int) (*models.Bookmark, error) {
	bookmark := &models.Bookmark{}
	err := svc.db.NewSelect().
		Model(bookmark).
		Where("bm.id = ?", id).
		Where("bm.user_id = ?", userID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return bookmark, errors.WithStack(err)
}

// UpdateNote replaces bookmark's note. A nil or blank note clears it.
func (svc *Service) UpdateNote(ctx context.Context, bookmark *models.Bookmark, note *string) error {
	bookmark.Note = normalizeNote(note)
	bookmark.UpdatedAt = time.Now()
	_, err := svc.db.NewUpdate().
		Model(bookmark).
		Column("note", "updated_at").
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// Delete removes bookmark.
func (svc *Service) Delete(ctx context.Context, bookmark *models.Bookmark) error {
	_, err := svc.db.NewDelete().
		Model(bookmark).
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// normalizeNote stores a blank note as none. The binder has trimmed it.
func normalizeNote(note *string) *string {
	if note == nil || *note == "" {
		return nil
	}
	return note
}
