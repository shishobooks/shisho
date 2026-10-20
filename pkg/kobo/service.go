package kobo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ScopedFile represents a file in the current sync scope with its hashes.
type ScopedFile struct {
	FileID          int
	FileHash        string
	FileSize        int64
	MetadataHash    string
	CoverModifiedAt *time.Time
}

// SyncChanges contains the detected changes between two sync points.
type SyncChanges struct {
	Added   []ScopedFile
	Removed []ScopedFile
	Changed []ScopedFile
}

// Service provides sync operations for Kobo devices.
type Service struct {
	db          *bun.DB
	listService *lists.Service
}

// NewService creates a new Kobo sync service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db, listService: lists.NewService(db)}
}

// CreateSyncPoint creates a new in-progress sync point with the given files.
// The point is marked complete only after the final paginated sync response is
// emitted, via MarkSyncPointCompleted. CleanupOldSyncPoints and
// GetLastSyncPoint both filter on completed_at, so an abandoned in-progress
// snapshot is invisible to the next fresh sync.
func (svc *Service) CreateSyncPoint(ctx context.Context, deviceID string, files []ScopedFile) (*SyncPoint, error) {
	now := time.Now()
	sp := &SyncPoint{
		ID:        uuid.New().String(),
		DeviceID:  deviceID,
		CreatedAt: now,
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
					ID:              uuid.New().String(),
					SyncPointID:     sp.ID,
					FileID:          f.FileID,
					FileHash:        f.FileHash,
					FileSize:        f.FileSize,
					MetadataHash:    f.MetadataHash,
					CoverModifiedAt: f.CoverModifiedAt,
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

// MarkSyncPointCompleted stamps completed_at on a previously-created
// in-progress sync point, making it eligible to act as the prev baseline for
// future syncs and eligible for cleanup.
//
// deviceID is required at the SQL layer (defense-in-depth alongside the
// resolveSyncPoint ownership check) so a stale or forged sync-point ID can't
// finalize someone else's snapshot.
func (svc *Service) MarkSyncPointCompleted(ctx context.Context, deviceID, syncPointID string) error {
	now := time.Now()
	_, err := svc.db.NewUpdate().
		Model((*SyncPoint)(nil)).
		Set("completed_at = ?", now).
		Where("id = ?", syncPointID).
		Where("device_id = ?", deviceID).
		Exec(ctx)
	return errors.WithStack(err)
}

// ScopedFilesFromSnapshot rebuilds the ScopedFile list from a sync point's
// stored Books rows. Used during continuation pagination so we diff against
// the same frozen snapshot as the first page rather than re-querying live DB
// state (which could shift between pages).
func ScopedFilesFromSnapshot(books []*SyncPointBook) []ScopedFile {
	out := make([]ScopedFile, len(books))
	for i, b := range books {
		out[i] = ScopedFile{
			FileID:          b.FileID,
			FileHash:        b.FileHash,
			FileSize:        b.FileSize,
			MetadataHash:    b.MetadataHash,
			CoverModifiedAt: b.CoverModifiedAt,
		}
	}
	return out
}

// GetSyncPointByID retrieves a sync point by ID with its Books relation loaded.
//
// deviceID is enforced at the SQL layer so a token bearing another tenant's
// sync-point UUID can't surface that tenant's snapshot. Returns sql.ErrNoRows
// (wrapped) when no point matches both ID and owner.
func (svc *Service) GetSyncPointByID(ctx context.Context, deviceID, syncPointID string) (*SyncPoint, error) {
	sp := new(SyncPoint)
	err := svc.db.NewSelect().
		Model(sp).
		Relation("Books").
		Where("ksp.id = ?", syncPointID).
		Where("ksp.device_id = ?", deviceID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return sp, nil
}

// GetLastSyncPoint returns the most recent completed sync point for a Device.
func (svc *Service) GetLastSyncPoint(ctx context.Context, deviceID string) (*SyncPoint, error) {
	sp := new(SyncPoint)
	err := svc.db.NewSelect().
		Model(sp).
		Relation("Books").
		Where("ksp.device_id = ?", deviceID).
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
//
// deviceID scopes the prev-snapshot lookup so a forged or stale ID can't
// surface another tenant's snapshot as our baseline. If the named sync point
// doesn't exist or doesn't belong to this Device, we silently degrade to a
// fresh sync (logging at warn so the unexpected re-send is observable).
func (svc *Service) DetectChanges(ctx context.Context, deviceID, lastSyncPointID string, currentFiles []ScopedFile) (*SyncChanges, error) {
	log := logger.FromContext(ctx)
	changes := &SyncChanges{}

	// First sync: all files are new.
	if lastSyncPointID == "" {
		changes.Added = make([]ScopedFile, len(currentFiles))
		copy(changes.Added, currentFiles)
		return changes, nil
	}

	// Load previous sync point books.
	sp, err := svc.GetSyncPointByID(ctx, deviceID, lastSyncPointID)
	if err != nil {
		// If the sync point was deleted or is invalid, treat as fresh sync.
		log.Warn("kobo prev sync point not found, falling back to fresh sync", logger.Data{
			"device_id":          deviceID,
			"last_sync_point_id": lastSyncPointID,
			"error":              err.Error(),
		})
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
		} else if prev.FileHash != curr.FileHash || prev.MetadataHash != curr.MetadataHash || coverChanged(prev.CoverModifiedAt, curr.CoverModifiedAt) {
			changes.Changed = append(changes.Changed, curr)
		}
	}

	// Detect removed.
	for _, prev := range sp.Books {
		if _, exists := currMap[prev.FileID]; !exists {
			changes.Removed = append(changes.Removed, ScopedFile{
				FileID:          prev.FileID,
				FileHash:        prev.FileHash,
				FileSize:        prev.FileSize,
				MetadataHash:    prev.MetadataHash,
				CoverModifiedAt: prev.CoverModifiedAt,
			})
		}
	}

	return changes, nil
}

// coverChanged reports whether a file's cover was replaced since the
// previous sync point. A previous row without a cover time predates cover
// tracking, so it never counts as a change; otherwise the first sync after
// the upgrade would re-send every book. A cover added or removed also
// changes the cover filename, which the metadata hash already catches. A
// cover that could not be read at either sync also has no time, so a cover
// missing at one sync and back under the same name later goes unnoticed, as
// does a replacement within one mtime tick on a coarse filesystem. Both fail
// toward sending nothing rather than re-sending books.
func coverChanged(prev, curr *time.Time) bool {
	if prev == nil || curr == nil {
		return false
	}
	return !prev.Equal(*curr)
}

// EnsureRevisionIDs returns the RevisionId recorded for each file under
// deviceID, first recording a new one for any file that has none. A recorded
// RevisionId is reused for every later sync, and a new one is a random UUID
// so nothing about the file (path, size, metadata) feeds into it.
func (svc *Service) EnsureRevisionIDs(ctx context.Context, deviceID string, fileIDs []int) (map[int]string, error) {
	revs := make(map[int]string, len(fileIDs))
	if len(fileIDs) == 0 {
		return revs, nil
	}

	now := time.Now()
	err := svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// A paginated sync pages out a frozen snapshot, so a file may have been
		// deleted since. It gets no entry, and the caller drops it from the page.
		var existing []int
		err := tx.NewSelect().
			Model((*models.File)(nil)).
			Column("f.id").
			Where("f.id IN (?)", bun.List(fileIDs)).
			Scan(ctx, &existing)
		if err != nil {
			return errors.WithStack(err)
		}
		if len(existing) == 0 {
			return nil
		}

		entries := make([]*SyncEntry, len(existing))
		for i, fileID := range existing {
			entries[i] = &SyncEntry{
				DeviceID:   deviceID,
				FileID:     fileID,
				RevisionID: uuid.New().String(),
				CreatedAt:  now,
			}
		}
		_, err = tx.NewInsert().
			Model(&entries).
			On("CONFLICT (device_id, file_id) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		var recorded []*SyncEntry
		err = tx.NewSelect().
			Model(&recorded).
			Where("kse.device_id = ?", deviceID).
			Where("kse.file_id IN (?)", bun.List(fileIDs)).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, e := range recorded {
			revs[e.FileID] = e.RevisionID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return revs, nil
}

// GetScopedFiles queries all Kobo-compatible main files (EPUB, CBZ) in scope,
// filtered by library access. A library or list scope the user cannot see
// returns an empty slice rather than an error. Supplement files and non-Kobo
// formats (M4B, PDF) are excluded. A book with multiple compatible files (e.g.
// two EPUBs) will have all of them returned. Each gets its own content ID on
// the device. The user must be loaded with LibraryAccess, as the Device
// middleware does.
func (svc *Service) GetScopedFiles(ctx context.Context, user *models.User, scope devices.Scope) ([]ScopedFile, error) {
	var files []models.File
	q, closed, err := svc.scopedFilesQuery(ctx, user, scope, &files)
	if err != nil {
		return nil, err
	}
	if closed {
		return []ScopedFile{}, nil
	}

	err = q.
		Relation("Book").
		Relation("Book.Authors", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.Order("a.sort_order ASC")
		}).
		Relation("Book.Authors.Person").
		Relation("Book.BookSeries.Series").
		Relation("Publisher").
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query scoped files")
	}

	// Convert to ScopedFile with computed hashes.
	result := make([]ScopedFile, len(files))
	for i, f := range files {
		result[i] = ScopedFile{
			FileID:          f.ID,
			FileHash:        computeFileHash(&f),
			FileSize:        f.FilesizeBytes,
			MetadataHash:    computeMetadataHash(f.Book, &f),
			CoverModifiedAt: coverModifiedAt(ctx, &f),
		}
	}

	return result, nil
}

// FileInScope reports whether fileID is one of the files GetScopedFiles
// would sync for this user and scope. The by-id Kobo routes (download, cover,
// metadata) call it before loading the file, so a Device can only fetch what it
// syncs. The user must be loaded with LibraryAccess.
func (svc *Service) FileInScope(ctx context.Context, user *models.User, scope devices.Scope, fileID int) (bool, error) {
	q, closed, err := svc.scopedFilesQuery(ctx, user, scope, (*models.File)(nil))
	if err != nil {
		return false, err
	}
	if closed {
		return false, nil
	}
	exists, err := q.Where("f.id = ?", fileID).Exists(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to check file scope")
	}
	return exists, nil
}

// scopedFilesQuery builds the query for the Kobo-compatible main files (EPUB,
// CBZ) in scope, selecting into model. Supplements are excluded so only real
// editions sync to the device. closed is true when the scope can hold no
// files for this user (a missing scope id, an inaccessible library, or a list
// the user cannot view); callers then skip the query. Callers must pass a
// non-nil user (the handlers reject a request without one). The nil guard
// exists so a future caller that forgets fails closed with an error instead
// of failing open or panicking on a nil dereference.
func (svc *Service) scopedFilesQuery(ctx context.Context, user *models.User, scope devices.Scope, model any) (q *bun.SelectQuery, closed bool, err error) {
	if user == nil {
		return nil, true, errors.New("scoped files query requires a user")
	}

	q = svc.db.NewSelect().
		Model(model).
		Where("f.file_type IN (?)", bun.List([]string{models.FileTypeEPUB, models.FileTypeCBZ})).
		Where("f.file_role = ?", models.FileRoleMain)

	switch scope.Type {
	case devices.ScopeLibrary:
		// A library scope without an id fails closed rather than syncing
		// every library.
		if scope.LibraryID == nil || !user.HasLibraryAccess(*scope.LibraryID) {
			return nil, true, nil
		}
		q = q.Where("f.library_id = ?", *scope.LibraryID)
	case devices.ScopeList:
		// A list scope without an id fails closed, like the library scope.
		if scope.ListID == nil {
			return nil, true, nil
		}
		// Only the owner or a share recipient may sync the list. A list the
		// user cannot see, or one that does not exist, syncs nothing, the same
		// as an inaccessible library.
		canView, err := svc.listService.CanView(ctx, *scope.ListID, user.ID)
		if err != nil {
			return nil, false, errors.Wrap(err, "failed to check list visibility")
		}
		if !canView {
			return nil, true, nil
		}
		// A list can hold books from libraries the user cannot access (it may
		// be shared with them), so the list is not the access boundary.
		q = q.Join("JOIN list_books AS lb ON lb.book_id = f.book_id").
			Where("lb.list_id = ?", *scope.ListID)
		if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
			q = q.Where("f.library_id IN (?)", bun.List(libraryIDs))
		}
	default: // devices.ScopeAll
		if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
			q = q.Where("f.library_id IN (?)", bun.List(libraryIDs))
		}
	}

	return q, false, nil
}

// CleanupOldSyncPoints removes completed sync points older than the most recent one per Device.
// This prevents the database from growing unbounded.
func (svc *Service) CleanupOldSyncPoints(ctx context.Context, deviceID string) error {
	// Keep only the most recent completed sync point per Device
	_, err := svc.db.NewDelete().
		Model((*SyncPoint)(nil)).
		Where("device_id = ?", deviceID).
		Where("completed_at IS NOT NULL").
		Where("id NOT IN (?)",
			svc.db.NewSelect().
				Model((*SyncPoint)(nil)).
				ColumnExpr("id").
				Where("device_id = ?", deviceID).
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

// coverModifiedAt returns the mtime of the file's cover on disk, or nil when
// it has none. Scans only write a cover that is missing, so the mtime changes
// when a cover is uploaded or replaced, not on every rescan. It is truncated
// to the microsecond so it compares equal after a database round trip.
func coverModifiedAt(ctx context.Context, file *models.File) *time.Time {
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return nil
	}
	path := covers.FileCoverPath(file)
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.FromContext(ctx).Warn("kobo could not stat cover", logger.Data{
				"file_id": file.ID,
				"path":    path,
				"error":   err.Error(),
			})
		}
		return nil
	}
	t := info.ModTime().UTC().Truncate(time.Microsecond)
	return &t
}

// coverCacheKey returns the short suffix added to CoverImageID so the
// device's thumbnail cache refreshes when the book's metadata or its cover
// image changes. Without a cover time it is the metadata hash prefix alone.
func coverCacheKey(metadataHash string, coverModifiedAt *time.Time) string {
	key := metadataHash
	if coverModifiedAt != nil {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", metadataHash, coverModifiedAt.UnixNano())))
		key = hex.EncodeToString(h[:])
	}
	if len(key) > 8 {
		key = key[:8]
	}
	return key
}

// computeMetadataHash creates a hash from the title the Kobo shows (the
// file's name, or the book's title), author names, and cover filename,
// truncated to 16 hex chars. Used both as a sync-diff signal (a metadata
// change marks the book as Changed) and as the base of the CoverImageID
// suffix for the device thumbnail cache (see coverCacheKey). It takes the
// book separately because handleMetadata loads it with its own query rather
// than through the file's Book relation.
func computeMetadataHash(book *models.Book, file *models.File) string {
	var bookTitle string
	if book != nil {
		bookTitle = book.Title
	}
	var parts []string
	// A loaded book always contributes its title, even an empty one, so
	// hashes stored before file names counted stay the same.
	if title := file.ResolveTitle(bookTitle); book != nil || title != "" {
		parts = append(parts, title)
	}
	if book != nil {
		for _, a := range book.Authors {
			if a.Person != nil {
				parts = append(parts, a.Person.Name)
			}
		}
	}
	if file != nil && file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		parts = append(parts, *file.CoverImageFilename)
	}
	data := strings.Join(parts, "\x00")
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])[:16]
}

// ShishoID returns a Shisho-prefixed ID for a file.
func ShishoID(fileID int) string {
	return fmt.Sprintf("shisho-%d", fileID)
}

// ParseShishoID parses a "shisho-{id}" or "shisho-{id}-{suffix}" string and
// returns the file ID. The suffixed form is used for cover IDs (suffix = a
// short hash of the book metadata) so device-side thumbnail caches refresh
// when the underlying book changes; the cover handler still needs the bare
// file ID. Returns (0, false) if the format is invalid.
func ParseShishoID(id string) (int, bool) {
	if !strings.HasPrefix(id, "shisho-") {
		return 0, false
	}
	rest := strings.TrimPrefix(id, "shisho-")
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}
