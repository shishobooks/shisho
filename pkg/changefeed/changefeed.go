// Package changefeed reads the change log that SQLite triggers keep (ADR
// 0023): one entry per record the Shisho app sees, each moved to the next
// sequence number whenever the record changes. The triggers live in the
// migrations; this package pages through the log for a user and expires old
// deletion entries.
package changefeed

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// The kinds of record the log holds. The triggers write these names, and
// the feed sends them as they are.
const (
	KindBook      = "book"
	KindLibrary   = "library"
	KindSeries    = "series"
	KindPerson    = "person"
	KindGenre     = "genre"
	KindTag       = "tag"
	KindPublisher = "publisher"
	// KindBookLeft is a deletion a Book leaves in the Library it moved out
	// of, so a user who can't access its new Library drops it.
	KindBookLeft = "book_left"
	// The user's own records, whose entries carry the user instead of a
	// Library. A Reading Position's record id is its File's, and a user
	// Book's its Book's.
	KindReadingPosition = "reading_position"
	KindReadThrough     = "read_through"
	KindUserBook        = "user_book"
	// A List's entry carries its owner as its user, and reaches everyone
	// it's shared with too.
	KindList = "list"
	// A Bookmark's entry carries its owner as its user.
	KindBookmark = "bookmark"
)

// catalogKinds are the kinds whose entries belong to a Library, which a
// Library's whole fetch sends.
var catalogKinds = []string{KindLibrary, KindBook, KindSeries, KindPerson, KindGenre, KindTag, KindPublisher}

const (
	// DeletionLifetime is how long a deletion entry stays in the log. An
	// installation that hasn't synced for longer starts over (ADR 0023).
	DeletionLifetime = 90 * 24 * time.Hour
	// expiryInterval is how often Expire actually runs when called through
	// ExpireIfDue.
	expiryInterval = time.Hour
	// sqliteTime is how CURRENT_TIMESTAMP, which the triggers write, formats
	// a time.
	sqliteTime = "2006-01-02 15:04:05"
)

// Entry is one record's place in the log.
type Entry struct {
	bun.BaseModel `bun:"table:change_log_entries,alias:cle"`

	Seq       int64     `bun:"seq,pk,autoincrement"`
	Kind      string    `bun:"kind,notnull"`
	RecordID  int       `bun:"record_id,notnull"`
	LibraryID *int      `bun:"library_id"`
	UserID    *int      `bun:"user_id"`
	Deleted   bool      `bun:"deleted,notnull"`
	ChangedAt time.Time `bun:"changed_at,notnull"`
}

// expiry is the one row recording the highest seq whose deletion entries
// have expired.
type expiry struct {
	bun.BaseModel `bun:"table:change_log_expiries,alias:cly"`

	ID             int       `bun:"id,pk"`
	ExpiredThrough int64     `bun:"expired_through,notnull"`
	ExpiredAt      time.Time `bun:"expired_at,notnull"`
}

// Page is a run of entries after a cursor, as one user may see them.
type Page struct {
	Entries []*Entry
	// Cursor is what the next page asks for entries after.
	Cursor int64
	// HasMore says another page has entries already.
	HasMore bool
	// StartOver says the cursor is from before deletion entries that have
	// since expired, so the installation may have missed a deletion and has
	// to sync again from the start.
	StartOver bool
	// ExpiredThrough is how far deletions had expired when the page was
	// read, which the installation sends back with its next cursor.
	ExpiredThrough int64
}

type Service struct {
	db  *bun.DB
	now func() time.Time
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// MoveOwn moves the entry of one of userID's own records to the end of the
// log through db, such as a transaction, so installations get the record
// again though nothing in it changed: as when the server keeps a Bookmark's
// note over an older edit an installation showed while it waited to send it.
func (s *Service) MoveOwn(ctx context.Context, db bun.IDB, kind string, recordID, userID int) error {
	if _, err := db.NewDelete().Model((*Entry)(nil)).
		Where("kind = ? AND record_id = ? AND user_id = ?", kind, recordID, userID).
		Exec(ctx); err != nil {
		return errors.WithStack(err)
	}
	_, err := db.NewInsert().Model(&Entry{Kind: kind, RecordID: recordID, UserID: &userID, ChangedAt: s.now().UTC()}).Exec(ctx)
	return errors.WithStack(err)
}

// Read runs fn in a read transaction, so the entries a page lists and the
// records read for them come from one snapshot.
func (s *Service) Read(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	return errors.WithStack(s.db.RunInTx(ctx, &sql.TxOptions{ReadOnly: true}, fn))
}

// Page returns up to limit entries after cursor that user may see, in seq
// order. Call it inside Read. A catalog entry belongs to the Library it
// recorded, and user sees those of the Libraries they can access. A deletion
// entry of a Library that no longer exists is seen by everyone: it carries
// nothing but ids, and the access that would have shown it went with the
// Library. An entry of a user's own record is seen by that user alone,
// whatever their role.
//
// seenExpired is the ExpiredThrough of the installation's last page.
func (s *Service) Page(ctx context.Context, tx bun.IDB, user *models.User, cursor, seenExpired int64, limit int) (*Page, error) {
	var expired int64
	err := tx.NewSelect().Model((*expiry)(nil)).Column("expired_through").Where("id = 1").Scan(ctx, &expired)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	// A cursor below an expired deletion may have missed it, unless those
	// deletions had already expired when the installation last synced: then
	// it was downloading from the start and never had what they deleted. A
	// cursor of 0 has seen nothing, so it can't have missed anything.
	if cursor > 0 && cursor < expired && seenExpired < expired {
		return &Page{Entries: []*Entry{}, Cursor: cursor, StartOver: true, ExpiredThrough: expired}, nil
	}
	// A cursor past every seq this log has handed out came from another
	// database, such as the one before a restore from backup, so the
	// installation's copy can't be trusted. sqlite_sequence keeps the
	// highest seq even after expiry deletes the newest entries.
	var handedOut sql.NullInt64
	err = tx.NewRaw(`SELECT seq FROM sqlite_sequence WHERE name = 'change_log_entries'`).Scan(ctx, &handedOut)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	if cursor > handedOut.Int64 {
		return &Page{Entries: []*Entry{}, Cursor: cursor, StartOver: true, ExpiredThrough: expired}, nil
	}

	entries := []*Entry{}
	ids := user.GetAccessibleLibraryIDs()
	q := tx.NewSelect().Model(&entries).
		Where("cle.seq > ?", cursor).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
				q = q.Where("cle.user_id IS NULL")
				if ids == nil {
					return q
				}
				return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
					if len(ids) > 0 {
						q = q.Where("cle.library_id IN (?)", bun.List(ids))
					}
					return q.WhereOr("cle.deleted AND cle.library_id NOT IN (SELECT id FROM libraries)")
				})
			})
			q = q.WhereOr("cle.user_id = ?", user.ID)
			return q.WhereOr("cle.kind = ? AND cle.record_id IN (SELECT list_id FROM list_shares WHERE user_id = ?)", KindList, user.ID)
		}).
		OrderExpr("cle.seq ASC").
		Limit(int64(limit) + 1)
	if err := q.Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	page := &Page{Entries: entries, Cursor: cursor, ExpiredThrough: expired}
	if len(entries) > limit {
		page.Entries = entries[:limit]
		page.HasMore = true
		page.Cursor = page.Entries[limit-1].Seq
		return page, nil
	}
	// Nothing after this page is visible to user, so the cursor moves past
	// the whole log and the next sync doesn't scan the hidden entries again.
	var last sql.NullInt64
	if err := tx.NewSelect().Model((*Entry)(nil)).ColumnExpr("MAX(seq)").Scan(ctx, &last); err != nil {
		return nil, errors.WithStack(err)
	}
	if last.Valid && last.Int64 > page.Cursor {
		page.Cursor = last.Int64
	}
	return page, nil
}

// Scope narrows a page to one Library or one List the user just gained,
// which the app fetches whole, since its entries are behind the app's
// cursor (ADR 0023). Exactly one is set.
type Scope struct {
	LibraryID int
	ListID    int
}

// ErrOutOfReach is a Scope naming a Library or List the user can't see.
var ErrOutOfReach = errors.New("out of reach")

// ScopedPage returns up to limit entries after cursor of the Library or
// List scope names, with ErrOutOfReach when access, the user's from Access,
// doesn't include it. Call it inside Read. It leaves out deletions, which a
// fetch from nothing has no use for, and the user's own records, which the
// feed itself carries.
func (s *Service) ScopedPage(ctx context.Context, tx bun.IDB, access *Access, scope Scope, cursor int64, limit int) (*Page, error) {
	entries := []*Entry{}
	q := tx.NewSelect().Model(&entries).
		Where("cle.seq > ?", cursor).
		Where("NOT cle.deleted").
		OrderExpr("cle.seq ASC").
		Limit(int64(limit) + 1)
	switch {
	case scope.LibraryID > 0 && slices.Contains(access.LibraryIDs, scope.LibraryID):
		q = q.Where("cle.library_id = ?", scope.LibraryID).Where("cle.kind IN (?)", bun.List(catalogKinds))
	case scope.ListID > 0 && slices.Contains(access.ListIDs, scope.ListID):
		q = q.Where("cle.kind = ?", KindList).Where("cle.record_id = ?", scope.ListID)
	default:
		return nil, ErrOutOfReach
	}
	if err := q.Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	page := &Page{Entries: entries, Cursor: cursor}
	if len(entries) > limit {
		page.Entries = entries[:limit]
		page.HasMore = true
	}
	if len(page.Entries) > 0 {
		page.Cursor = page.Entries[len(page.Entries)-1].Seq
	}
	return page, nil
}

// Access is what a user can see now, which the feed answers outside the
// cursor: a Library or List gained has no entries after it, and one lost
// sends nothing (ADR 0023).
type Access struct {
	LibraryIDs []int
	ListIDs    []int
}

// Access returns the Libraries and Lists user can see, each in id order.
func (s *Service) Access(ctx context.Context, tx bun.IDB, user *models.User) (*Access, error) {
	access := &Access{LibraryIDs: []int{}, ListIDs: []int{}}
	q := tx.NewSelect().Table("libraries").Column("id").Order("id")
	if ids := user.GetAccessibleLibraryIDs(); ids != nil {
		if len(ids) == 0 {
			q = q.Where("FALSE")
		} else {
			q = q.Where("id IN (?)", bun.List(ids))
		}
	}
	if err := q.Scan(ctx, &access.LibraryIDs); err != nil {
		return nil, errors.WithStack(err)
	}
	err := tx.NewRaw(`SELECT id FROM lists WHERE user_id = ?
		UNION SELECT list_id FROM list_shares WHERE user_id = ?
		ORDER BY 1`, user.ID, user.ID).Scan(ctx, &access.ListIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return access, nil
}

// Behind returns those of libraryIDs with a record at or before cursor
// that's still current, in id order: the ones a page after cursor won't
// send again.
func (s *Service) Behind(ctx context.Context, tx bun.IDB, libraryIDs []int, cursor int64) ([]int, error) {
	behind := []int{}
	if cursor == 0 || len(libraryIDs) == 0 {
		return behind, nil
	}
	err := tx.NewSelect().Table("libraries").Column("id").
		Where("id IN (?)", bun.List(libraryIDs)).
		Where(`EXISTS (SELECT 1 FROM change_log_entries cle
			WHERE cle.library_id = libraries.id AND cle.seq <= ? AND NOT cle.deleted AND cle.user_id IS NULL)`, cursor).
		Order("id").
		Scan(ctx, &behind)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return behind, nil
}

// ExpireIfDue runs Expire when it hasn't run for an hour, so the feed can
// call it on every sync without writing on every sync.
func (s *Service) ExpireIfDue(ctx context.Context) error {
	var last expiry
	err := s.db.NewSelect().Model(&last).Where("id = 1").Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.WithStack(err)
	}
	if err == nil && s.now().Sub(last.ExpiredAt) < expiryInterval {
		return nil
	}
	_, err = s.Expire(ctx)
	return err
}

// Expire deletes the deletion entries older than DeletionLifetime and
// records the highest seq it deleted, so a cursor below it starts over. It
// returns how many it deleted.
func (s *Service) Expire(ctx context.Context) (int, error) {
	now := s.now()
	cutoff := now.Add(-DeletionLifetime).UTC().Format(sqliteTime)
	var deleted []int64
	// The delete comes first, so the transaction takes the write lock before
	// it reads anything.
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		err := tx.NewDelete().Model((*Entry)(nil)).
			Where("deleted AND julianday(changed_at) < julianday(?)", cutoff).
			Returning("seq").
			Scan(ctx, &deleted)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.WithStack(err)
		}
		var through int64
		for _, seq := range deleted {
			through = max(through, seq)
		}
		_, err = tx.NewInsert().
			Model(&expiry{ID: 1, ExpiredThrough: through, ExpiredAt: now}).
			On("CONFLICT (id) DO UPDATE").
			Set("expired_through = MAX(cly.expired_through, EXCLUDED.expired_through)").
			Set("expired_at = EXCLUDED.expired_at").
			Exec(ctx)
		return errors.WithStack(err)
	})
	return len(deleted), err
}
