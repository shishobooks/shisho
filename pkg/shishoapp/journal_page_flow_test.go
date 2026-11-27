package shishoapp

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *fixture) pageFlow(userID, bookID int) sql.NullString {
	f.t.Helper()
	var flow sql.NullString
	err := f.db.QueryRowContext(f.ctx, `SELECT page_flow FROM user_books WHERE user_id = ? AND book_id = ?`, userID, bookID).Scan(&flow)
	if err == sql.ErrNoRows {
		return sql.NullString{}
	}
	require.NoError(f.t, err)
	return flow
}

// The user's Page flow for a Book travels with their own record of it, so
// a flow chosen on the web reaches the app.
func TestFeed_UserBookCarriesPageFlow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Comics")
	f.addBook(1, 1, "Nemo")
	userID := f.addUser()
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	f.exec(`INSERT INTO user_books (user_id, book_id, page_flow) VALUES (?, 1, 'scrolled')`, userID)
	resp := f.changes(credential, cursor, 0)
	require.Equal(t, []string{"user_book:1"}, summary(resp))
	flow := "scrolled"
	assert.Equal(t, &flow, resp.Entries[0].UserBook.PageFlow)

	// Changing it moves the record; when it was chosen alone doesn't.
	f.exec(`UPDATE user_books SET page_flow_updated_at = '2026-06-01 00:00:00' WHERE user_id = ?`, userID)
	assert.Empty(t, f.changes(credential, resp.Cursor, 0).Entries)
	f.exec(`UPDATE user_books SET page_flow = 'paginated' WHERE user_id = ?`, userID)
	next := f.changes(credential, resp.Cursor, 0)
	require.Equal(t, []string{"user_book:1"}, summary(next))
	flow = "paginated"
	assert.Equal(t, &flow, next.Entries[0].UserBook.PageFlow)
}

// A Page flow chosen in the app is saved for the Book, unless the user
// chose another later elsewhere: a late event then changes nothing, and
// the Book's record moves so the app shows the later choice again.
func TestJournal_PageFlow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Comics")
	f.addBook(1, 1, "Nemo")
	f.addLibrary(2, "Hidden")
	f.addBook(2, 2, "Emma")
	userID := f.addUser(1)
	now := time.Now().UTC().Truncate(time.Second)
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()

	resp := f.journal(credential, now,
		event("a", JournalEventPageFlow, now.Add(-20*time.Minute), map[string]any{"book_id": 1, "page_flow": "scrolled"}),
		event("hidden", JournalEventPageFlow, now, map[string]any{"book_id": 2, "page_flow": "scrolled"}),
		event("missing", JournalEventPageFlow, now, map[string]any{"book_id": 99, "page_flow": "scrolled"}),
		event("invalid", JournalEventPageFlow, now, map[string]any{"book_id": 1, "page_flow": "sideways"}),
	)
	assert.Equal(t, []string{"a:accepted", "hidden:refused", "missing:refused", "invalid:refused"}, results(resp))
	assert.Equal(t, sql.NullString{String: "scrolled", Valid: true}, f.pageFlow(userID, 1))
	assert.False(t, f.pageFlow(userID, 2).Valid)

	// Chosen on the web since: a choice made earlier on a phone that was
	// offline arrives late and loses, and the record moves.
	f.exec(`UPDATE user_books SET page_flow = 'paginated', page_flow_updated_at = ? WHERE user_id = ? AND book_id = 1`,
		now.Add(-5*time.Minute), userID)
	cursor := f.changes(credential, 0, 0).Cursor
	resp = f.journal(credential, now,
		event("late", JournalEventPageFlow, now.Add(-10*time.Minute), map[string]any{"book_id": 1, "page_flow": "scrolled"}),
	)
	assert.Equal(t, []string{"late:accepted"}, results(resp))
	assert.Equal(t, sql.NullString{String: "paginated", Valid: true}, f.pageFlow(userID, 1))
	assert.Equal(t, []string{"user_book:1"}, summary(f.changes(credential, cursor, 0)))

	// A later choice wins.
	resp = f.journal(credential, now,
		event("later", JournalEventPageFlow, now.Add(-time.Minute), map[string]any{"book_id": 1, "page_flow": "scrolled"}),
	)
	assert.Equal(t, []string{"later:accepted"}, results(resp))
	assert.Equal(t, sql.NullString{String: "scrolled", Valid: true}, f.pageFlow(userID, 1))
}
