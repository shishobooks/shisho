package kobo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ScopedFile represents a file in the current sync scope with its hashes.
type ScopedFile struct {
	FileID       int
	FileHash     string
	FileSize     int64
	MetadataHash string
}

// SyncChanges contains the detected changes between two sync points.
type SyncChanges struct {
	Added   []ScopedFile
	Removed []ScopedFile
	Changed []ScopedFile
}

// Service provides sync operations for Kobo devices.
type Service struct {
	db *bun.DB
}

// NewService creates a new Kobo sync service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// CreateSyncPoint creates a new sync point with the given files and marks it as complete.
func (svc *Service) CreateSyncPoint(ctx context.Context, apiKeyID string, files []ScopedFile) (*SyncPoint, error) {
	now := time.Now()
	sp := &SyncPoint{
		ID:          uuid.New().String(),
		APIKeyID:    apiKeyID,
		CreatedAt:   now,
		CompletedAt: &now,
	}

	err := svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(sp).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if len(files) > 0 {
			books := make([]*SyncPointBook, len(files))
			for i, f := range files {
				books[i] = &SyncPointBook{
					ID:           uuid.New().String(),
					SyncPointID:  sp.ID,
					FileID:       f.FileID,
					FileHash:     f.FileHash,
					FileSize:     f.FileSize,
					MetadataHash: f.MetadataHash,
				}
			}
			_, err = tx.NewInsert().Model(&books).Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
			sp.Books = books
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return sp, nil
}

// GetSyncPointByID retrieves a sync point by ID with its Books relation loaded.
func (svc *Service) GetSyncPointByID(ctx context.Context, syncPointID string) (*SyncPoint, error) {
	sp := new(SyncPoint)
	err := svc.db.NewSelect().
		Model(sp).
		Relation("Books").
		Where("ksp.id = ?", syncPointID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return sp, nil
}

// GetLastSyncPoint returns the most recent completed sync point for an API key.
func (svc *Service) GetLastSyncPoint(ctx context.Context, apiKeyID string) (*SyncPoint, error) {
	sp := new(SyncPoint)
	err := svc.db.NewSelect().
		Model(sp).
		Relation("Books").
		Where("ksp.api_key_id = ?", apiKeyID).
		Where("ksp.completed_at IS NOT NULL").
		Order("ksp.completed_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return sp, nil
}

// DetectChanges compares currentFiles against the last sync point.
// If lastSyncPointID is empty, this is the first sync and all files are Added.
func (svc *Service) DetectChanges(ctx context.Context, lastSyncPointID string, currentFiles []ScopedFile) (*SyncChanges, error) {
	changes := &SyncChanges{}

	// First sync: all files are new.
	if lastSyncPointID == "" {
		changes.Added = make([]ScopedFile, len(currentFiles))
		copy(changes.Added, currentFiles)
		return changes, nil
	}

	// Load previous sync point books.
	sp, err := svc.GetSyncPointByID(ctx, lastSyncPointID)
	if err != nil {
		// If the sync point was deleted or is invalid, treat as fresh sync.
		changes.Added = make([]ScopedFile, len(currentFiles))
		copy(changes.Added, currentFiles)
		return changes, nil
	}

	// Build map of previous files by FileID.
	prevMap := make(map[int]*SyncPointBook, len(sp.Books))
	for _, b := range sp.Books {
		prevMap[b.FileID] = b
	}

	// Build map of current files by FileID.
	currMap := make(map[int]ScopedFile, len(currentFiles))
	for _, f := range currentFiles {
		currMap[f.FileID] = f
	}

	// Detect added and changed.
	for _, curr := range currentFiles {
		prev, exists := prevMap[curr.FileID]
		if !exists {
			changes.Added = append(changes.Added, curr)
		} else if prev.FileHash != curr.FileHash || prev.MetadataHash != curr.MetadataHash {
			changes.Changed = append(changes.Changed, curr)
		}
	}

	// Detect removed.
	for _, prev := range sp.Books {
		if _, exists := currMap[prev.FileID]; !exists {
			changes.Removed = append(changes.Removed, ScopedFile{
				FileID:       prev.FileID,
				FileHash:     prev.FileHash,
				FileSize:     prev.FileSize,
				MetadataHash: prev.MetadataHash,
			})
		}
	}

	return changes, nil
}

// GetScopedFiles queries files in scope, filtered by library access and file type (epub/cbz).
func (svc *Service) GetScopedFiles(ctx context.Context, userID int, scope *SyncScope) ([]ScopedFile, error) {
	// Load user with library access.
	user := new(models.User)
	err := svc.db.NewSelect().
		Model(user).
		Relation("LibraryAccess").
		Where("u.id = ?", userID).
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load user")
	}

	// Query files with relations.
	var files []models.File
	q := svc.db.NewSelect().
		Model(&files).
		Relation("Book").
		Relation("Book.Authors.Person").
		Relation("Book.BookSeries.Series").
		Relation("Publisher").
		Where("f.file_type IN (?)", bun.In([]string{models.FileTypeEPUB, models.FileTypeCBZ})).
		Where("f.file_role = ?", models.FileRoleMain)

	// Apply scope.
	switch scope.Type {
	case "library":
		if scope.LibraryID != nil {
			// Verify user has access to this library.
			if !user.HasLibraryAccess(*scope.LibraryID) {
				return []ScopedFile{}, nil
			}
			q = q.Where("f.library_id = ?", *scope.LibraryID)
		}
	case "list":
		if scope.ListID != nil {
			q = q.Join("JOIN list_books AS lb ON lb.book_id = f.book_id").
				Where("lb.list_id = ?", *scope.ListID)
		}
	default: // "all"
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			q = q.Where("f.library_id IN (?)", bun.In(libraryIDs))
		}
	}

	err = q.Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query scoped files")
	}

	// Convert to ScopedFile with computed hashes.
	result := make([]ScopedFile, len(files))
	for i, f := range files {
		result[i] = ScopedFile{
			FileID:       f.ID,
			FileHash:     computeFileHash(&f),
			FileSize:     f.FilesizeBytes,
			MetadataHash: computeMetadataHash(&f),
		}
	}

	return result, nil
}

// CleanupOldSyncPoints removes completed sync points older than the most recent one per API key.
// This prevents the database from growing unbounded.
// ClearAllSyncPoints deletes all sync points for an API key, forcing a fresh sync.
func (svc *Service) ClearAllSyncPoints(ctx context.Context, apiKeyID string) error {
	_, err := svc.db.NewDelete().
		Model((*SyncPoint)(nil)).
		Where("api_key_id = ?", apiKeyID).
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) CleanupOldSyncPoints(ctx context.Context, apiKeyID string) error {
	// Keep only the most recent completed sync point per API key
	_, err := svc.db.NewDelete().
		Model((*SyncPoint)(nil)).
		Where("api_key_id = ?", apiKeyID).
		Where("completed_at IS NOT NULL").
		Where("id NOT IN (?)",
			svc.db.NewSelect().
				Model((*SyncPoint)(nil)).
				ColumnExpr("id").
				Where("api_key_id = ?", apiKeyID).
				Where("completed_at IS NOT NULL").
				OrderExpr("created_at DESC").
				Limit(1),
		).
		Exec(ctx)
	return errors.WithStack(err)
}

// computeFileHash creates a hash from filepath and file size, truncated to 16 hex chars.
func computeFileHash(file *models.File) string {
	data := fmt.Sprintf("%s:%d", file.Filepath, file.FilesizeBytes)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])[:16]
}

// computeMetadataHash creates a hash from title, author names, and cover path, truncated to 16 hex chars.
func computeMetadataHash(file *models.File) string {
	var parts []string

	// Title from book.
	if file.Book != nil {
		parts = append(parts, file.Book.Title)
	}

	// Author names.
	if file.Book != nil {
		for _, a := range file.Book.Authors {
			if a.Person != nil {
				parts = append(parts, a.Person.Name)
			}
		}
	}

	// Cover path.
	if file.CoverImagePath != nil {
		parts = append(parts, *file.CoverImagePath)
	}

	data := strings.Join(parts, "\x00")
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])[:16]
}

// ShishoID returns a Shisho-prefixed ID for a file.
func ShishoID(fileID int) string {
	return fmt.Sprintf("shisho-%d", fileID)
}

// ParseShishoID parses a "shisho-{id}" format string and returns the file ID.
// Returns (0, false) if the format is invalid.
func ParseShishoID(id string) (int, bool) {
	if !strings.HasPrefix(id, "shisho-") {
		return 0, false
	}
	numStr := strings.TrimPrefix(id, "shisho-")
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, false
	}
	return n, true
}
