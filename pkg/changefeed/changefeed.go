// Package changefeed reads the change log that SQLite triggers keep (ADR
// 0023): one entry per record the Shisho app sees, each moved to the next
// sequence number whenever the record changes. The triggers live in the
// migrations; this package pages through the log for a user and expires old
// deletion entries.
package changefeed

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// The kinds of record the log holds. The triggers write these names, and
// the feed sends them as they are.
const (
	KindBook    = "book"
	KindLibrary = "library"
)

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
}

type Service struct {
	db  *bun.DB
	now func() time.Time
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// Read runs fn in a read transaction, so the entries a page lists and the
// records read for them come from one snapshot.
func (s *Service) Read(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	return errors.WithStack(s.db.RunInTx(ctx, &sql.TxOptions{ReadOnly: true}, fn))
}

// Page returns up to limit entries after cursor that user may see, in seq
// order. Call it inside Read. An entry belongs to the Library it recorded,
// and user sees those of the Libraries they can access. A deletion entry of
// a Library that no longer exists is seen by everyone: it carries nothing
// but ids, and the access that would have shown it went with the Library.
func (s *Service) Page(ctx context.Context, tx bun.IDB, user *models.User, cursor int64, limit int) (*Page, error) {
	var expired int64
	err := tx.NewSelect().Model((*expiry)(nil)).Column("expired_through").Where("id = 1").Scan(ctx, &expired)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.WithStack(err)
	}
	// A cursor of 0 has seen nothing, so it can't have missed anything.
	if cursor > 0 && cursor < expired {
		return &Page{Entries: []*Entry{}, Cursor: cursor, StartOver: true}, nil
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
		return &Page{Entries: []*Entry{}, Cursor: cursor, StartOver: true}, nil
	}

	entries := []*Entry{}
	q := tx.NewSelect().Model(&entries).
		Where("cle.seq > ?", cursor).
		OrderExpr("cle.seq ASC").
		Limit(int64(limit) + 1)
	if ids := user.GetAccessibleLibraryIDs(); ids != nil {
		q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if len(ids) > 0 {
				q = q.Where("cle.library_id IN (?)", bun.List(ids))
			}
			return q.WhereOr("cle.deleted AND cle.library_id NOT IN (SELECT id FROM libraries)")
		})
	}
	if err := q.Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	page := &Page{Entries: entries, Cursor: cursor}
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
