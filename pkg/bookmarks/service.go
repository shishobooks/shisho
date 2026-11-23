package bookmarks

import (
	"context"
	"database/sql"
	"strings"
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

// Create adds a Bookmark for userID at seconds into fileID, made now.
func (svc *Service) Create(ctx context.Context, userID, fileID int, seconds float64, note *string) (*models.Bookmark, error) {
	return svc.CreateIn(ctx, svc.db, userID, fileID, seconds, note, time.Now())
}

// CreateIn adds a Bookmark through db, such as a transaction, made at
// madeAt, which can be earlier than now for one made offline.
func (svc *Service) CreateIn(ctx context.Context, db bun.IDB, userID, fileID int, seconds float64, note *string,
	madeAt time.Time) (*models.Bookmark, error) {
	bookmark := &models.Bookmark{
		CreatedAt: madeAt,
		UpdatedAt: madeAt,
		UserID:    userID,
		FileID:    fileID,
		Seconds:   seconds,
		Note:      NormalizeNote(note),
	}
	_, err := db.NewInsert().Model(bookmark).Exec(ctx)
	return bookmark, errors.WithStack(err)
}

// Retrieve returns userID's Bookmark id, or sql.ErrNoRows when userID has no
// such Bookmark, whether or not another user does.
func (svc *Service) Retrieve(ctx context.Context, userID, id int) (*models.Bookmark, error) {
	return svc.RetrieveIn(ctx, svc.db, userID, id)
}

// RetrieveIn is Retrieve through db, such as a transaction.
func (svc *Service) RetrieveIn(ctx context.Context, db bun.IDB, userID, id int) (*models.Bookmark, error) {
	bookmark := &models.Bookmark{}
	err := db.NewSelect().
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
	return svc.UpdateNoteIn(ctx, svc.db, bookmark, note, time.Now())
}

// UpdateNoteIn replaces bookmark's note through db, such as a transaction,
// as changed at changedAt.
func (svc *Service) UpdateNoteIn(ctx context.Context, db bun.IDB, bookmark *models.Bookmark, note *string, changedAt time.Time) error {
	bookmark.Note = NormalizeNote(note)
	bookmark.UpdatedAt = changedAt
	_, err := db.NewUpdate().
		Model(bookmark).
		Column("note", "updated_at").
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// Delete removes bookmark.
func (svc *Service) Delete(ctx context.Context, bookmark *models.Bookmark) error {
	return svc.DeleteIn(ctx, svc.db, bookmark)
}

// DeleteIn removes bookmark through db, such as a transaction.
func (svc *Service) DeleteIn(ctx context.Context, db bun.IDB, bookmark *models.Bookmark) error {
	_, err := db.NewDelete().
		Model(bookmark).
		WherePK().
		Exec(ctx)
	return errors.WithStack(err)
}

// NormalizeNote is a note as stored: trimmed, and none when blank.
func NormalizeNote(note *string) *string {
	if note == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*note)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
