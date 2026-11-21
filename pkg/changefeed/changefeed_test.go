package changefeed

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func exec(t *testing.T, db *bun.DB, query string, args ...any) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), query, args...)
	require.NoError(t, err)
}

// page reads a page for an installation that hasn't seen any deletions
// expire.
func page(t *testing.T, s *Service, user *models.User, cursor int64) *Page {
	t.Helper()
	var p *Page
	require.NoError(t, s.Read(context.Background(), func(ctx context.Context, tx bun.Tx) error {
		var err error
		p, err = s.Page(ctx, tx, user, cursor, 0, 10)
		return err
	}))
	return p
}

func kinds(p *Page) []string {
	out := []string{}
	for _, e := range p.Entries {
		out = append(out, e.Kind+":"+strconv.Itoa(e.RecordID))
	}
	return out
}

func TestExpire_DeletesOldDeletionsAndStartsOldCursorsOver(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	s := NewService(db)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	exec(t, db, `INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'L', 'book')`)
	exec(t, db, `INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source) VALUES (1, 1, '/1', 'One', 'file', 'One', 'file'), (2, 1, '/2', 'Two', 'file', 'Two', 'file')`)
	exec(t, db, `DELETE FROM books WHERE id = 1`)
	var deletionSeq int64
	require.NoError(t, db.NewSelect().Model((*Entry)(nil)).Column("seq").Where("kind = 'book' AND record_id = 1").Scan(context.Background(), &deletionSeq))
	// The deletion happened 91 days ago, the other entries just now.
	exec(t, db, `UPDATE change_log_entries SET changed_at = ? WHERE deleted`, now.Add(-91*24*time.Hour).UTC().Format(sqliteTime))
	exec(t, db, `UPDATE change_log_entries SET changed_at = ? WHERE NOT deleted`, now.UTC().Format(sqliteTime))

	deleted, err := s.Expire(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	admin := &models.User{LibraryAccess: []*models.UserLibraryAccess{{}}}
	// A cursor from before the expired deletion starts over.
	assert.True(t, page(t, s, admin, deletionSeq-1).StartOver)
	// One past it, and one from the start, don't.
	assert.False(t, page(t, s, admin, deletionSeq).StartOver)
	fresh := page(t, s, admin, 0)
	assert.False(t, fresh.StartOver)
	assert.Equal(t, []string{"library:1", "book:2"}, kinds(fresh))

	// A later run with nothing to expire keeps the mark.
	deleted, err = s.Expire(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)
	assert.True(t, page(t, s, admin, deletionSeq-1).StartOver)
}

func TestExpireIfDue_RunsAtMostHourly(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	s := NewService(db)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	require.NoError(t, s.ExpireIfDue(context.Background()))

	exec(t, db, `INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'L', 'book')`)
	exec(t, db, `DELETE FROM libraries WHERE id = 1`)
	exec(t, db, `UPDATE change_log_entries SET changed_at = '2000-01-01 00:00:00'`)

	now = now.Add(30 * time.Minute)
	require.NoError(t, s.ExpireIfDue(context.Background()))
	count, err := db.NewSelect().Model((*Entry)(nil)).Count(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, count, "ran again within the hour")

	now = now.Add(31 * time.Minute)
	require.NoError(t, s.ExpireIfDue(context.Background()))
	count, err = db.NewSelect().Model((*Entry)(nil)).Count(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}
