package kobo

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// DeliveredFile is a File a Device's Delivery Rules deliver now, and the
// File Type they deliver it as.
type DeliveredFile struct {
	FileID            int
	BookID            int
	DeliveredFileType string
}

// handover is an entry given to another File of its book.
type handover struct {
	Entry *SyncEntry
	To    DeliveredFile
}

// entryPlan is what a sync does to a Device's entries (ADR 0012).
type entryPlan struct {
	// Kept entries still hold a File that is delivered as their type.
	Kept []*SyncEntry
	// HandedOver entries lost their File and go to another File of their
	// book delivered as the same type, which the Kobo shows as the same book
	// re-downloaded in place.
	HandedOver []handover
	// Removed entries lost their File with no File to take them.
	Removed []*SyncEntry
	// Added Files get new entries.
	Added []DeliveredFile
}

// planEntries decides what happens to a Device's entries, given every File
// its rules deliver now. An entry stays with its File while that File is
// still delivered as the entry's type, so a still-delivered File's entry is
// never handed to another File. An entry that lost its File goes to a File
// of the same book delivered as the same type that has no entry, the oldest
// entry first, or is removed. Every delivered File left without an entry gets
// a new one, so Files tied on a rule each hold their own, and a File whose
// delivered type changed loses its entry and gets a new one.
func planEntries(entries []*SyncEntry, delivered []DeliveredFile) entryPlan {
	var plan entryPlan
	byFile := make(map[int]DeliveredFile, len(delivered))
	for _, d := range delivered {
		byFile[d.FileID] = d
	}
	taken := map[int]bool{}

	var freed []*SyncEntry
	for _, e := range entries {
		if e.FileID != nil {
			if d, ok := byFile[*e.FileID]; ok && d.DeliveredFileType == e.DeliveredFileType {
				plan.Kept = append(plan.Kept, e)
				taken[d.FileID] = true
				continue
			}
		}
		freed = append(freed, e)
	}

	sort.SliceStable(freed, func(i, j int) bool {
		if !freed[i].CreatedAt.Equal(freed[j].CreatedAt) {
			return freed[i].CreatedAt.Before(freed[j].CreatedAt)
		}
		return freed[i].EntryID < freed[j].EntryID
	})
	candidates := handoverCandidates(delivered)
	for _, e := range freed {
		to, ok := handoverTarget(e, candidates, taken)
		if !ok {
			plan.Removed = append(plan.Removed, e)
			continue
		}
		taken[to.FileID] = true
		plan.HandedOver = append(plan.HandedOver, handover{Entry: e, To: to})
	}

	for _, d := range delivered {
		if !taken[d.FileID] {
			plan.Added = append(plan.Added, d)
		}
	}
	return plan
}

// candidateKey is the book and delivered File Type an entry belongs to.
type candidateKey struct {
	bookID        int
	deliveredType string
}

// handoverCandidates groups delivered by book and delivered File Type,
// each group lowest File id first, so finding a freed entry's target does
// not scan every delivered File.
func handoverCandidates(delivered []DeliveredFile) map[candidateKey][]DeliveredFile {
	candidates := map[candidateKey][]DeliveredFile{}
	for _, d := range delivered {
		key := candidateKey{d.BookID, d.DeliveredFileType}
		candidates[key] = append(candidates[key], d)
	}
	for _, group := range candidates {
		sort.Slice(group, func(i, j int) bool { return group[i].FileID < group[j].FileID })
	}
	return candidates
}

// handoverTarget returns the File, lowest id first, that a freed entry goes
// to: one of its book, delivered as its type, with no entry.
func handoverTarget(e *SyncEntry, candidates map[candidateKey][]DeliveredFile, taken map[int]bool) (DeliveredFile, bool) {
	if e.BookID == nil {
		return DeliveredFile{}, false
	}
	for _, d := range candidates[candidateKey{*e.BookID, e.DeliveredFileType}] {
		if !taken[d.FileID] {
			return d, true
		}
	}
	return DeliveredFile{}, false
}

// entryIDPrefix starts every entry id. Entries recorded before entries had
// ids of their own continue with the File's id; newEntryID continues with
// newEntryMarker, so the two forms never collide.
const (
	entryIDPrefix  = "shisho-"
	newEntryMarker = "e"
)

// newEntryID returns an id for a new entry, which can't match an id of the
// "shisho-{fileID}" form and has no "-" after its prefix, so a cover id's
// suffix can be split off (parseEntryID).
func newEntryID() string {
	return entryIDPrefix + newEntryMarker + strings.ReplaceAll(uuid.New().String(), "-", "")
}

// parseEntryID returns the entry id a Kobo request names: the id itself, or
// a cover id with its cache key suffix ("shisho-12-abcd1234"). It returns
// false for an id that is not Shisho's, such as a book from the Kobo store.
func parseEntryID(id string) (string, bool) {
	rest, ok := strings.CutPrefix(id, entryIDPrefix)
	if !ok {
		return "", false
	}
	key, _, _ := strings.Cut(rest, "-")
	if key == "" {
		return "", false
	}
	return entryIDPrefix + key, true
}

// deviceEntries returns every entry of deviceID.
func deviceEntries(ctx context.Context, db bun.IDB, deviceID string) ([]*SyncEntry, error) {
	var entries []*SyncEntry
	err := db.NewSelect().
		Model(&entries).
		Where("kse.device_id = ?", deviceID).
		Scan(ctx)
	return entries, errors.WithStack(err)
}

// bookEntries returns deviceID's entries that belong to one of bookIDs or
// hold one of fileIDs.
func bookEntries(ctx context.Context, db bun.IDB, deviceID string, bookIDs, fileIDs []int) ([]*SyncEntry, error) {
	var entries []*SyncEntry
	if len(bookIDs) == 0 && len(fileIDs) == 0 {
		return entries, nil
	}
	err := db.NewSelect().
		Model(&entries).
		Where("kse.device_id = ?", deviceID).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if len(bookIDs) > 0 {
				q = q.WhereOr("kse.book_id IN (?)", bun.List(bookIDs))
			}
			if len(fileIDs) > 0 {
				q = q.WhereOr("kse.file_id IN (?)", bun.List(fileIDs))
			}
			return q
		}).
		Scan(ctx)
	return entries, errors.WithStack(err)
}

// assignEntries applies planEntries to deviceID's entries for the Files its
// rules deliver now, and returns the entry each File holds, by File id.
func (svc *Service) assignEntries(ctx context.Context, deviceID string, delivered []DeliveredFile) (map[int]*SyncEntry, error) {
	held := make(map[int]*SyncEntry, len(delivered))
	err := svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		entries, err := deviceEntries(ctx, tx, deviceID)
		if err != nil {
			return err
		}
		// A File deleted since the scope query, as a scan can, gets no entry.
		delivered, err := existingFiles(ctx, tx, delivered)
		if err != nil {
			return err
		}
		plan := planEntries(entries, delivered)

		// Removals go first: a removed entry can hold a File another entry is
		// handed, and a File can hold only one entry.
		if len(plan.Removed) > 0 {
			ids := entryIDs(plan.Removed)
			_, err := tx.NewDelete().
				Model((*SyncEntry)(nil)).
				Where("device_id = ?", deviceID).
				Where("entry_id IN (?)", bun.List(ids)).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		for _, e := range plan.Kept {
			d := deliveredFileFor(e, delivered)
			if e.BookID == nil || *e.BookID != d.BookID {
				// The File moved to another book; the entry follows it.
				e.BookID = &d.BookID
				if err := updateEntry(ctx, tx, e, "book_id"); err != nil {
					return err
				}
			}
			held[d.FileID] = e
		}
		for _, h := range plan.HandedOver {
			h.Entry.FileID = &h.To.FileID
			h.Entry.BookID = &h.To.BookID
			if err := updateEntry(ctx, tx, h.Entry, "file_id", "book_id"); err != nil {
				return err
			}
			held[h.To.FileID] = h.Entry
		}
		if len(plan.Added) > 0 {
			now := time.Now()
			added := make([]*SyncEntry, len(plan.Added))
			for i, d := range plan.Added {
				added[i] = &SyncEntry{
					DeviceID:          deviceID,
					EntryID:           newEntryID(),
					BookID:            &d.BookID,
					FileID:            &d.FileID,
					DeliveredFileType: d.DeliveredFileType,
					// A random UUID, so nothing about the File feeds into it.
					RevisionID: uuid.New().String(),
					CreatedAt:  now,
				}
				held[d.FileID] = added[i]
			}
			if _, err := tx.NewInsert().Model(&added).Exec(ctx); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return held, nil
}

// existingFiles returns the Files of delivered that still exist.
func existingFiles(ctx context.Context, tx bun.Tx, delivered []DeliveredFile) ([]DeliveredFile, error) {
	if len(delivered) == 0 {
		return delivered, nil
	}
	ids := make([]int, len(delivered))
	for i, d := range delivered {
		ids[i] = d.FileID
	}
	var existing []int
	err := tx.NewSelect().
		Model((*models.File)(nil)).
		Column("f.id").
		Where("f.id IN (?)", bun.List(ids)).
		Scan(ctx, &existing)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	exists := make(map[int]bool, len(existing))
	for _, id := range existing {
		exists[id] = true
	}
	kept := make([]DeliveredFile, 0, len(existing))
	for _, d := range delivered {
		if exists[d.FileID] {
			kept = append(kept, d)
		}
	}
	return kept, nil
}

func deliveredFileFor(e *SyncEntry, delivered []DeliveredFile) DeliveredFile {
	for _, d := range delivered {
		if d.FileID == *e.FileID {
			return d
		}
	}
	return DeliveredFile{}
}

func updateEntry(ctx context.Context, tx bun.Tx, e *SyncEntry, columns ...string) error {
	_, err := tx.NewUpdate().Model(e).Column(columns...).WherePK().Exec(ctx)
	return errors.WithStack(err)
}

func entryIDs(entries []*SyncEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.EntryID
	}
	return ids
}

// EditionMove is an entry the next sync hands to another File of its book:
// the Kobo re-downloads the book in place, and may reopen it at the start
// when the two editions' chapters don't line up (ADR 0012).
type EditionMove struct {
	BookID int
	// FromFileID is nil when the File the entry held was deleted.
	FromFileID *int
	ToFileID   int
}

// EntryChanges is what the next sync would do to a Kobo's entries that the
// reader should hear about before it happens.
type EntryChanges struct {
	Moves []EditionMove
	// TypeSwitches are books whose entry would be removed while the book
	// gets an entry of another delivered File Type, such as KePub to EPUB.
	// The Kobo adds the book again as a new one, without its reading
	// position or highlights.
	TypeSwitches []TypeSwitch
}

// TypeSwitch is a book on a Kobo that its next sync switches from one
// delivered File Type to another.
type TypeSwitch struct {
	BookID int
	From   string
	To     string
}

// PlanChanges returns what the next sync of deviceID would do to its
// entries, given the Files its rules would deliver. With bookIDs, only the
// entries of those books (or holding one of delivered's Files) are planned,
// and delivered holds only those books' Files; without, every entry is.
func (svc *Service) PlanChanges(ctx context.Context, deviceID string, delivered []DeliveredFile, bookIDs []int) (*EntryChanges, error) {
	var entries []*SyncEntry
	var err error
	if bookIDs == nil {
		entries, err = deviceEntries(ctx, svc.db, deviceID)
	} else {
		fileIDs := make([]int, len(delivered))
		for i, d := range delivered {
			fileIDs[i] = d.FileID
		}
		entries, err = bookEntries(ctx, svc.db, deviceID, bookIDs, fileIDs)
	}
	if err != nil {
		return nil, err
	}
	plan := planEntries(entries, delivered)

	changes := &EntryChanges{Moves: []EditionMove{}, TypeSwitches: []TypeSwitch{}}
	for _, h := range plan.HandedOver {
		changes.Moves = append(changes.Moves, EditionMove{BookID: h.To.BookID, FromFileID: h.Entry.FileID, ToFileID: h.To.FileID})
	}
	addedTypes := map[int][]string{}
	for _, d := range plan.Added {
		addedTypes[d.BookID] = append(addedTypes[d.BookID], d.DeliveredFileType)
	}
	retyped := map[int]bool{}
	for _, e := range plan.Removed {
		if e.BookID == nil || retyped[*e.BookID] {
			continue
		}
		for _, t := range addedTypes[*e.BookID] {
			if t != e.DeliveredFileType {
				retyped[*e.BookID] = true
				changes.TypeSwitches = append(changes.TypeSwitches, TypeSwitch{BookID: *e.BookID, From: e.DeliveredFileType, To: t})
				break
			}
		}
	}
	return changes, nil
}
