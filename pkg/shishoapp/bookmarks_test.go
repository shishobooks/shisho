package shishoapp

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createBookmark(localID string, fileID int, seconds float64, note any) map[string]any {
	payload := map[string]any{"local_id": localID, "file_id": fileID, "seconds": seconds}
	if note != nil {
		payload["note"] = note
	}
	return payload
}

// byLocal and byID name a Bookmark for a note or delete event.
func byLocal(localID string) map[string]any { return map[string]any{"local_id": localID} }
func byID(id int) map[string]any            { return map[string]any{"bookmark_id": id} }

func withNote(ref map[string]any, note string) map[string]any {
	out := map[string]any{"note": note}
	for k, v := range ref {
		out[k] = v
	}
	return out
}

type storedBookmark struct {
	ID        int
	UserID    int
	FileID    int
	Seconds   float64
	Note      sql.NullString
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (f *fixture) bookmarks() []storedBookmark {
	f.t.Helper()
	rows, err := f.db.QueryContext(f.ctx, `SELECT id, user_id, file_id, seconds, note, created_at, updated_at FROM bookmarks ORDER BY id`)
	require.NoError(f.t, err)
	defer rows.Close()
	out := []storedBookmark{}
	for rows.Next() {
		var b storedBookmark
		require.NoError(f.t, rows.Scan(&b.ID, &b.UserID, &b.FileID, &b.Seconds, &b.Note, &b.CreatedAt, &b.UpdatedAt))
		out = append(out, b)
	}
	require.NoError(f.t, rows.Err())
	return out
}

// The user's own Bookmarks travel in the feed, and nobody else's do. One
// the app made carries the app's id for it, so the app knows it as its own.
func TestFeed_CarriesTheUsersOwnBookmarks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	otherID := f.addOtherUser()
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	f.exec(`INSERT INTO bookmarks (id, user_id, file_id, seconds, note, created_at, updated_at) VALUES (7, ?, 5, 83.5, 'the twist', '2026-05-01 10:00:00', '2026-05-01 10:00:00')`, userID)
	f.exec(`INSERT INTO bookmarks (id, user_id, file_id, seconds, created_at, updated_at) VALUES (8, ?, 5, 10, '2026-05-01 10:00:00', '2026-05-01 10:00:00')`, otherID)

	resp := f.changes(credential, cursor, 0)
	require.Equal(t, []string{"bookmark:7"}, summary(resp))
	note := "the twist"
	assert.Equal(t, &FeedBookmark{
		ID: 7, FileID: 5, Seconds: 83.5, Note: &note,
		CreatedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
	}, resp.Entries[0].Bookmark)
	assert.Zero(t, resp.Entries[0].LibraryID)

	// Only a change the app shows moves it.
	f.exec(`UPDATE bookmarks SET updated_at = '2026-06-01 00:00:00' WHERE id = 7`)
	assert.Empty(t, f.changes(credential, resp.Cursor, 0).Entries)
	f.exec(`UPDATE bookmarks SET note = NULL WHERE id = 7`)
	next := f.changes(credential, resp.Cursor, 0)
	require.Equal(t, []string{"bookmark:7"}, summary(next))
	assert.Nil(t, next.Entries[0].Bookmark.Note)

	f.exec(`DELETE FROM bookmarks WHERE id = 7`)
	assert.Equal(t, []string{"bookmark:7-"}, summary(f.changes(credential, next.Cursor, 0)))

	// Bookmarks go with their File, and the feed says so.
	f.exec(`INSERT INTO bookmarks (id, user_id, file_id, seconds) VALUES (9, ?, 5, 20)`, userID)
	cursor = f.changes(credential, 0, 0).Cursor
	f.exec(`DELETE FROM files WHERE id = 5`)
	assert.Contains(t, summary(f.changes(credential, cursor, 0)), "bookmark:9-")
}

// A Bookmark made in the app is placed at the time it was made, and is the
// app's own in the feed. Sending it again under a new event changes nothing.
func TestJournal_BookmarkCreate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (6, 1, 1, '/dune.epub', 'epub', 1)`)
	f.addLibrary(2, "Hidden")
	f.addBook(2, 2, "Emma")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (7, 2, 2, '/emma.m4b', 'm4b', 1)`)
	userID := f.addUser(1)
	now := time.Now().UTC().Truncate(time.Second)
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor
	made := now.Add(-10 * time.Minute)

	resp := f.journal(credential, now,
		event("a", JournalEventBookmarkCreate, made, createBookmark("local-1", 5, 83.5, "  the twist  ")),
		event("b", JournalEventBookmarkCreate, now, createBookmark("local-1", 5, 83.5, "the twist")),
		event("c", JournalEventBookmarkCreate, now, createBookmark("local-2", 5, 12, "")),
		event("epub", JournalEventBookmarkCreate, now, createBookmark("local-3", 6, 12, nil)),
		event("hidden", JournalEventBookmarkCreate, now, createBookmark("local-4", 7, 12, nil)),
		event("no-local", JournalEventBookmarkCreate, now, map[string]any{"file_id": 5, "seconds": 3}),
		event("no-seconds", JournalEventBookmarkCreate, now, map[string]any{"local_id": "local-5", "file_id": 5}),
	)
	assert.Equal(t, []string{
		"a:accepted", "b:accepted", "c:accepted", "epub:refused", "hidden:refused", "no-local:refused", "no-seconds:refused",
	}, results(resp))

	stored := f.bookmarks()
	require.Len(t, stored, 2)
	assert.Equal(t, userID, stored[0].UserID)
	assert.InDelta(t, 83.5, stored[0].Seconds, 0.001)
	assert.Equal(t, "the twist", stored[0].Note.String)
	assert.True(t, made.Equal(stored[0].CreatedAt), "made %s, stored %s", made, stored[0].CreatedAt)
	assert.False(t, stored[1].Note.Valid, "a blank note is none")

	feed := f.changes(credential, cursor, 0)
	require.Equal(t, []string{"bookmark:" + strconv.Itoa(stored[0].ID), "bookmark:" + strconv.Itoa(stored[1].ID)}, summary(feed))
	require.NotNil(t, feed.Entries[0].Bookmark.LocalID)
	assert.Equal(t, "local-1", *feed.Entries[0].Bookmark.LocalID)

	// The same event again is a duplicate.
	again := f.journal(credential, now, event("a", JournalEventBookmarkCreate, made, createBookmark("local-1", 5, 83.5, nil)))
	assert.Equal(t, []string{"a:duplicate"}, results(again))
	assert.Len(t, f.bookmarks(), 2)
}

// A note's latest edit wins, wherever it was made; a delete wins over
// everything after it; and no user reaches another's Bookmarks.
func TestJournal_BookmarkNoteAndDelete(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser(1)
	otherID := f.addOtherUser()
	now := time.Now().UTC().Truncate(time.Second)
	f.accountSince(userID, now.Add(-2*time.Hour))
	credential := f.signIn()
	webEdit := now.Add(-30 * time.Minute)
	f.exec(`INSERT INTO bookmarks (id, user_id, file_id, seconds, note, created_at, updated_at) VALUES (20, ?, 5, 40, 'web', ?, ?)`,
		userID, now.Add(-time.Hour), webEdit)
	f.exec(`INSERT INTO bookmarks (id, user_id, file_id, seconds, note) VALUES (21, ?, 5, 50, 'theirs')`, otherID)

	resp := f.journal(credential, now,
		event("make", JournalEventBookmarkCreate, now.Add(-50*time.Minute), createBookmark("local-1", 5, 10, nil)),
		// Made offline before the web edit, so the web's note stays.
		event("stale", JournalEventBookmarkNote, now.Add(-40*time.Minute), withNote(byID(20), "app, earlier")),
		event("note-local", JournalEventBookmarkNote, now.Add(-20*time.Minute), withNote(byLocal("local-1"), " mine ")),
		event("theirs", JournalEventBookmarkNote, now, withNote(byID(21), "taken over")),
		event("theirs-delete", JournalEventBookmarkDelete, now, byID(21)),
		event("both", JournalEventBookmarkDelete, now, map[string]any{"bookmark_id": 20, "local_id": "local-1"}),
		event("neither", JournalEventBookmarkDelete, now, map[string]any{}),
	)
	assert.Equal(t, []string{
		"make:accepted", "stale:accepted", "note-local:accepted", "theirs:accepted", "theirs-delete:accepted",
		"both:refused", "neither:refused",
	}, results(resp))
	stored := f.bookmarks()
	require.Len(t, stored, 3)
	assert.Equal(t, "web", stored[0].Note.String)
	assert.Equal(t, "theirs", stored[1].Note.String)
	assert.Equal(t, "mine", stored[2].Note.String)
	assert.True(t, now.Add(-20*time.Minute).Equal(stored[2].UpdatedAt), "the edit's own time")

	resp = f.journal(credential, now,
		event("newer", JournalEventBookmarkNote, now.Add(-10*time.Minute), withNote(byID(20), "app, later")),
		event("delete-local", JournalEventBookmarkDelete, now.Add(-5*time.Minute), byLocal("local-1")),
		// After the delete: a note, and the create sent again, change nothing.
		event("after", JournalEventBookmarkNote, now, withNote(byLocal("local-1"), "too late")),
		event("remake", JournalEventBookmarkCreate, now, createBookmark("local-1", 5, 10, nil)),
		event("clear", JournalEventBookmarkNote, now, withNote(byID(20), "   ")),
		event("delete-gone", JournalEventBookmarkDelete, now, byID(999)),
		event("delete-never-made", JournalEventBookmarkDelete, now, byLocal("never-made")),
	)
	assert.Equal(t, []string{
		"newer:accepted", "delete-local:accepted", "after:accepted", "remake:accepted", "clear:accepted", "delete-gone:accepted",
		"delete-never-made:accepted",
	}, results(resp))
	stored = f.bookmarks()
	require.Len(t, stored, 2)
	assert.Equal(t, 20, stored[0].ID)
	assert.False(t, stored[0].Note.Valid, "a blank note clears it")
	assert.Equal(t, 21, stored[1].ID)

	resp = f.journal(credential, now, event("delete-web", JournalEventBookmarkDelete, now, byID(20)))
	assert.Equal(t, []string{"delete-web:accepted"}, results(resp))
	stored = f.bookmarks()
	require.Len(t, stored, 1)
	assert.Equal(t, otherID, stored[0].UserID)
}

// A phone whose clock runs a little fast sends times after the batch
// arrives, which are taken as its arrival. A note edited right after the
// Bookmark was made then has the same time as the create, and still applies,
// since a batch's events apply in order.
func TestJournal_BookmarkNoteAfterCreateFromAFastClock(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser(1)
	now := time.Now().UTC()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()
	fast := now.Add(2 * time.Minute)

	resp := f.journal(credential, fast,
		event("make", JournalEventBookmarkCreate, fast.Add(-2*time.Second), createBookmark("local-1", 5, 10, "first")),
		event("edit", JournalEventBookmarkNote, fast.Add(-time.Second), withNote(byLocal("local-1"), "second")),
	)
	assert.Equal(t, []string{"make:accepted", "edit:accepted"}, results(resp))
	stored := f.bookmarks()
	require.Len(t, stored, 1)
	assert.Equal(t, "second", stored[0].Note.String)
}

// A fault while applying a Bookmark event fails the batch, never refuses the
// event: a refused create makes the app forget the Bookmark. The event stays
// unclaimed, so the resent batch applies it.
func TestJournal_AFaultWhileApplyingABookmarkLeavesItToResend(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()
	made := event("b1", JournalEventBookmarkCreate, now, createBookmark("local-1", 5, 10, "note"))

	f.exec(`CREATE TRIGGER fail_links BEFORE INSERT ON app_journal_bookmarks BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	rec := f.journalRaw(credential, now, made)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM app_journal_events`))
	assert.Empty(t, f.bookmarks())

	f.exec(`DROP TRIGGER fail_links`)
	assert.Equal(t, []string{"b1:accepted"}, results(f.journal(credential, now, made)))
	assert.Len(t, f.bookmarks(), 1)

	f.exec(`CREATE TRIGGER fail_deletes BEFORE DELETE ON bookmarks BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	gone := event("b2", JournalEventBookmarkDelete, now, byLocal("local-1"))
	rec = f.journalRaw(credential, now, gone)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	f.exec(`DROP TRIGGER fail_deletes`)
	assert.Equal(t, []string{"b2:accepted"}, results(f.journal(credential, now, gone)))
	assert.Empty(t, f.bookmarks())
}

// A delete can reach the server before the create it follows, when the
// create's batch was given up on and sent again later. The create then
// makes nothing: the delete wins.
func TestJournal_ADeleteBeforeItsCreateWins(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()

	assert.Equal(t, []string{"gone:accepted"}, results(f.journal(credential, now,
		event("gone", JournalEventBookmarkDelete, now, byLocal("local-1")))))
	assert.Equal(t, []string{"made:accepted"}, results(f.journal(credential, now,
		event("made", JournalEventBookmarkCreate, now.Add(-time.Second), createBookmark("local-1", 5, 10, nil)))))
	assert.Empty(t, f.bookmarks())
}

// A note older than the Bookmark's last change is taken in and changes
// nothing, but the feed sends the Bookmark again, so the app that showed its
// own note while the edit waited learns the note that won.
func TestJournal_AStaleNoteSendsTheWinningNoteAgain(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now().UTC().Truncate(time.Second)
	f.accountSince(userID, now.Add(-2*time.Hour))
	credential := f.signIn()
	f.exec(`INSERT INTO bookmarks (id, user_id, file_id, seconds, note, created_at, updated_at) VALUES (20, ?, 5, 40, 'web', ?, ?)`,
		userID, now.Add(-time.Hour), now.Add(-10*time.Minute))
	cursor := f.changes(credential, 0, 0).Cursor

	assert.Equal(t, []string{"stale:accepted"}, results(f.journal(credential, now,
		event("stale", JournalEventBookmarkNote, now.Add(-20*time.Minute), withNote(byID(20), "offline")))))
	feed := f.changes(credential, cursor, 0)
	require.Equal(t, []string{"bookmark:20"}, summary(feed))
	assert.Equal(t, "web", *feed.Entries[0].Bookmark.Note)
}
