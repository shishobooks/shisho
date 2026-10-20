package kobo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ScopedFile is a File the Device's Delivery Rules deliver, with the entry
// it goes out under and its hashes.
type ScopedFile struct {
	FileID          int
	FileHash        string
	FileSize        int64
	MetadataHash    string
	CoverModifiedAt *time.Time
	// EntryID, DeliveredFileType, and RevisionID are the entry (SyncEntry)
	// the File goes out under.
	EntryID           string
	DeliveredFileType string
	RevisionID        string
}

// SyncChanges contains the detected changes between two sync points.
type SyncChanges struct {
	Added   []ScopedFile
	Removed []ScopedFile
	Changed []ScopedFile
}

// Service provides sync operations for Kobo devices.
type Service struct {
	db            *bun.DB
	listService   *lists.Service
	deviceService *devices.Service
}

// NewService creates a new Kobo sync service.
func NewService(db *bun.DB) *Service {
	return &Service{db: db, listService: lists.NewService(db), deviceService: devices.NewService(db)}
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
					ID:                uuid.New().String(),
					SyncPointID:       sp.ID,
					FileID:            f.FileID,
					FileHash:          f.FileHash,
					FileSize:          f.FileSize,
					MetadataHash:      f.MetadataHash,
					CoverModifiedAt:   f.CoverModifiedAt,
					EntryID:           f.EntryID,
					DeliveredFileType: f.DeliveredFileType,
					RevisionID:        f.RevisionID,
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
		out[i] = scopedFileFromRow(b)
	}
	return out
}

func scopedFileFromRow(b *SyncPointBook) ScopedFile {
	return ScopedFile{
		FileID:            b.FileID,
		FileHash:          b.FileHash,
		FileSize:          b.FileSize,
		MetadataHash:      b.MetadataHash,
		CoverModifiedAt:   b.CoverModifiedAt,
		EntryID:           b.EntryID,
		DeliveredFileType: b.DeliveredFileType,
		RevisionID:        b.RevisionID,
	}
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

	// Entries are matched by id: one that holds another File now was handed
	// over, which goes out as a change under the same ids.
	prevMap := make(map[string]*SyncPointBook, len(sp.Books))
	for _, b := range sp.Books {
		prevMap[b.EntryID] = b
	}
	currMap := make(map[string]bool, len(currentFiles))
	for _, f := range currentFiles {
		currMap[f.EntryID] = true
	}

	for _, curr := range currentFiles {
		prev, exists := prevMap[curr.EntryID]
		if !exists {
			changes.Added = append(changes.Added, curr)
		} else if entryChanged(prev, curr) {
			changes.Changed = append(changes.Changed, curr)
		}
	}

	for _, prev := range sp.Books {
		if !currMap[prev.EntryID] {
			changes.Removed = append(changes.Removed, scopedFileFromRow(prev))
		}
	}

	return changes, nil
}

// entryChanged reports whether an entry has to go out again: it holds
// another File, its File or the File's metadata or cover changed, or it is
// delivered as another File Type. A snapshot row stored before entries had
// ids records KePub, which is all Kobo sync sent then.
func entryChanged(prev *SyncPointBook, curr ScopedFile) bool {
	return prev.FileID != curr.FileID ||
		prev.DeliveredFileType != curr.DeliveredFileType ||
		prev.FileHash != curr.FileHash ||
		prev.MetadataHash != curr.MetadataHash ||
		coverChanged(prev.CoverModifiedAt, curr.CoverModifiedAt)
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

// GetScopedFiles returns every File the Device's Delivery Rules deliver
// within its scope, with the entry each goes out under, first assigning
// entries (planEntries). A library or list scope the user cannot see
// delivers nothing. The user must be loaded with LibraryAccess, as the
// Device middleware does.
func (svc *Service) GetScopedFiles(ctx context.Context, user *models.User, device *devices.Device) ([]ScopedFile, error) {
	rules, err := svc.deviceService.Rules(ctx, device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var files []*models.File
	q, closed, err := svc.scopedFilesQuery(ctx, user, device.Scope(), rules, &files)
	if err != nil {
		return nil, err
	}
	if closed {
		files = nil
	} else {
		err = q.
			Relation("Book").
			Relation("Book.Authors", func(sq *bun.SelectQuery) *bun.SelectQuery {
				return sq.Order("a.sort_order ASC")
			}).
			Relation("Book.Authors.Person").
			Relation("Publisher").
			Order("f.id ASC").
			Scan(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to query scoped files")
		}
	}

	delivered := DeliveredFiles(rules, files)
	held, err := svc.assignEntries(ctx, device.ID, delivered)
	if err != nil {
		return nil, err
	}

	byID := make(map[int]*models.File, len(files))
	for _, f := range files {
		byID[f.ID] = f
	}
	result := make([]ScopedFile, 0, len(delivered))
	for _, d := range delivered {
		f := byID[d.FileID]
		entry, ok := held[d.FileID]
		if !ok {
			// Deleted since the scope query.
			continue
		}
		result = append(result, ScopedFile{
			FileID:            f.ID,
			FileHash:          computeFileHash(f),
			FileSize:          f.FilesizeBytes,
			MetadataHash:      computeMetadataHash(f.Book, f),
			CoverModifiedAt:   coverModifiedAt(ctx, f),
			EntryID:           entry.EntryID,
			DeliveredFileType: entry.DeliveredFileType,
			RevisionID:        entry.RevisionID,
		})
	}
	return result, nil
}

// DeliveredFiles returns the Files rules deliver from files, each with the
// File Type it goes as. files must hold every main File of the source types
// rules name for each of their books, as the scope query selects them, since
// a book's earliest matching rule decides which of its Files go
// (delivery.Resolve).
func DeliveredFiles(rules []delivery.Rule, files []*models.File) []DeliveredFile {
	var order []int
	byBook := map[int]*models.Book{}
	for _, f := range files {
		book, ok := byBook[f.BookID]
		if !ok {
			book = &models.Book{ID: f.BookID}
			byBook[f.BookID] = book
			order = append(order, f.BookID)
		}
		book.Files = append(book.Files, f)
	}
	var delivered []DeliveredFile
	for _, bookID := range order {
		for _, d := range delivery.Resolve(rules, byBook[bookID]).All() {
			delivered = append(delivered, DeliveredFile{FileID: d.File.ID, BookID: bookID, DeliveredFileType: d.DeliveredFileType})
		}
	}
	return delivered
}

// EntryInScope returns the Device's entry with entryID while the Device
// still delivers the entry's File as its type within its scope, and nil
// otherwise. The by-id Kobo routes (download, cover, metadata) call it, so a
// Device can only fetch what it syncs. The user must be loaded with
// LibraryAccess.
func (svc *Service) EntryInScope(ctx context.Context, user *models.User, device *devices.Device, entryID string) (*SyncEntry, error) {
	entry := new(SyncEntry)
	err := svc.db.NewSelect().
		Model(entry).
		Where("kse.device_id = ?", device.ID).
		Where("kse.entry_id = ?", entryID).
		Where("kse.file_id IS NOT NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	rules, err := svc.deviceService.Rules(ctx, device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var files []*models.File
	q, closed, err := svc.scopedFilesQuery(ctx, user, device.Scope(), rules, &files)
	if err != nil {
		return nil, err
	}
	if closed {
		return nil, nil
	}
	err = q.
		Where("f.book_id = (?)", svc.db.NewSelect().Model((*models.File)(nil)).Column("book_id").Where("id = ?", *entry.FileID)).
		Scan(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check entry scope")
	}
	for _, d := range DeliveredFiles(rules, files) {
		if d.FileID == *entry.FileID && d.DeliveredFileType == entry.DeliveredFileType {
			return entry, nil
		}
	}
	return nil, nil
}

// scopedFilesQuery builds the query for the main Files in scope whose File
// Type a rule in rules delivers, selecting into model. Its scope matches
// reach.Service.mainFiles, which previews what a Kobo receives; change both
// together. Supplements are excluded so only real editions sync to the
// device. closed is true when the scope can hold no files for this user (a missing scope id, an
// inaccessible library, or a list the user cannot view); callers then skip
// the query. Callers must pass a non-nil user (the handlers reject a request
// without one). The nil guard exists so a future caller that forgets fails
// closed with an error instead of failing open or panicking on a nil
// dereference.
func (svc *Service) scopedFilesQuery(ctx context.Context, user *models.User, scope devices.Scope, rules []delivery.Rule, model any) (q *bun.SelectQuery, closed bool, err error) {
	if user == nil {
		return nil, true, errors.New("scoped files query requires a user")
	}
	sourceTypes := delivery.SourceFileTypes(rules)
	if len(sourceTypes) == 0 {
		return nil, true, nil
	}

	q = svc.db.NewSelect().
		Model(model).
		Where("f.file_type IN (?)", bun.List(sourceTypes)).
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
