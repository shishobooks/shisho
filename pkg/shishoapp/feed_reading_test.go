package shishoapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addOtherUser adds a second user, "other", who can access every Library.
func (f *fixture) addOtherUser() int {
	f.t.Helper()
	var id int
	require.NoError(f.t, f.db.QueryRowContext(f.ctx,
		`INSERT INTO users (username, password_hash, role_id, is_active) VALUES ('other', ?, 1, TRUE) RETURNING id`,
		f.password).Scan(&id))
	f.exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)`, id)
	return id
}

// The user's own Reading Positions and Read-throughs travel in the feed,
// and nobody else's do, not even to an admin.
func TestFeed_CarriesTheUsersOwnPositionsAndReadThroughs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Audio")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1)`)
	userID := f.addUser()
	otherID := f.addOtherUser()
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	f.exec(`INSERT INTO reading_positions (user_id, file_id, seconds, percentage, source, occurred_at) VALUES (?, 5, 90.5, 0.25, 'web_reader', '2026-05-01 10:00:00')`, userID)
	f.exec(`INSERT INTO reading_positions (user_id, file_id, seconds, percentage, source, occurred_at) VALUES (?, 5, 10, 0.1, 'web_reader', '2026-05-01 10:00:00')`, otherID)
	f.exec(`INSERT INTO read_throughs (id, user_id, book_id, outcome, begin_day, paused) VALUES (3, ?, 1, 'underway', '2026-05-01', FALSE)`, userID)
	f.exec(`INSERT INTO read_throughs (id, user_id, book_id, outcome, begin_day) VALUES (4, ?, 1, 'underway', '2026-05-01')`, otherID)

	resp := f.changes(credential, cursor, 0)
	assert.Equal(t, []string{"reading_position:5", "read_through:3"}, summary(resp))
	seconds := 90.5
	assert.Equal(t, &FeedReadingPosition{
		FileID: 5, Seconds: &seconds, Percentage: 0.25,
		OccurredAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), Source: "web_reader",
	}, resp.Entries[0].ReadingPosition)
	begin := "2026-05-01"
	bookID := 1
	assert.Equal(t, &FeedReadThrough{ID: 3, BookID: &bookID, BeginDay: &begin, Outcome: "underway"}, resp.Entries[1].ReadThrough)
	assert.Zero(t, resp.Entries[0].LibraryID)

	// A change moves the record; deleting it says so.
	f.exec(`UPDATE reading_positions SET seconds = 120, percentage = 0.3, occurred_at = '2026-05-01 10:05:00' WHERE user_id = ?`, userID)
	f.exec(`UPDATE read_throughs SET outcome = 'finished', end_day = '2026-05-02' WHERE id = 3`)
	f.exec(`UPDATE read_throughs SET outcome = 'finished', end_day = '2026-05-02' WHERE id = 4`)
	next := f.changes(credential, resp.Cursor, 0)
	assert.Equal(t, []string{"reading_position:5", "read_through:3"}, summary(next))
	assert.InDelta(t, 120.0, *next.Entries[0].ReadingPosition.Seconds, 0.001)
	assert.Equal(t, "finished", next.Entries[1].ReadThrough.Outcome)

	f.exec(`DELETE FROM read_throughs`)
	f.exec(`DELETE FROM reading_positions`)
	last := f.changes(credential, next.Cursor, 0)
	assert.Equal(t, []string{"read_through:3-", "reading_position:5-"}, summary(last))

	// Writes that change nothing the app reads record nothing.
	f.exec(`INSERT INTO reading_positions (user_id, file_id, seconds, percentage, source, occurred_at) VALUES (?, 5, 1, 0.1, 'web_reader', '2026-05-01 10:00:00')`, userID)
	cursor = f.changes(credential, last.Cursor, 0).Cursor
	f.exec(`UPDATE reading_positions SET updated_at = '2030-01-01 00:00:00', reported_device = 'Kobo'`)
	assert.Empty(t, f.changes(credential, cursor, 0).Entries)
}

// A position reports the Device that sent it, so an installation can tell
// its own from another's.
func TestFeed_PositionNamesItsDevice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Audio")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1)`)
	userID := f.addUser()
	rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("reader", testPassword)})
	require.Equal(t, http.StatusOK, rec.Code)
	var signedIn SignInResponse
	require.NoError(t, decodeJSON(rec, &signedIn))
	f.exec(`INSERT INTO reading_positions (user_id, file_id, seconds, percentage, source, source_device_id, occurred_at) VALUES (?, 5, 1, 0.1, 'shisho_app', ?, '2026-05-01 10:00:00')`,
		userID, signedIn.DeviceID)

	resp := f.changes(signedIn.Credential, 0, 0)
	position := resp.Entries[len(resp.Entries)-1].ReadingPosition
	require.NotNil(t, position)
	assert.Equal(t, signedIn.DeviceID, *position.DeviceID)
	assert.Equal(t, "shisho_app", position.Source)
}

// A File carries Shisho's chapters, and editing one moves its Book.
func TestFeed_FileCarriesItsChapters(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Audio")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1)`)
	f.exec(`INSERT INTO chapters (id, file_id, sort_order, title, start_timestamp_ms) VALUES (11, 5, 1, 'Two', 60000), (10, 5, 0, 'One', 0)`)
	f.exec(`INSERT INTO chapters (id, file_id, parent_id, sort_order, title, start_timestamp_ms) VALUES (12, 5, 11, 0, 'Two, part one', 60000)`)
	f.addUser()
	credential := f.signIn()

	resp := f.changes(credential, 0, 0)
	book := resp.Entries[len(resp.Entries)-1].Book
	require.NotNil(t, book)
	require.Len(t, book.Files, 1)
	zero, minute := int64(0), int64(60000)
	parent := 11
	assert.Equal(t, []FeedChapter{
		{ID: 10, Title: "One", StartTimestampMs: &zero},
		{ID: 11, Title: "Two", StartTimestampMs: &minute},
		{ID: 12, ParentID: &parent, Title: "Two, part one", StartTimestampMs: &minute},
	}, book.Files[0].Chapters)

	for _, write := range []string{
		`UPDATE chapters SET title = 'The First' WHERE id = 10`,
		`UPDATE chapters SET start_timestamp_ms = 61000 WHERE id = 11`,
		`DELETE FROM chapters WHERE id = 12`,
		`INSERT INTO chapters (id, file_id, sort_order, title, start_timestamp_ms) VALUES (13, 5, 2, 'Three', 120000)`,
	} {
		cursor := resp.Cursor
		resp = f.changes(credential, cursor, 0)
		assert.Empty(t, resp.Entries)
		f.exec(write)
		resp = f.changes(credential, cursor, 0)
		assert.Equal(t, []string{"book:1"}, summary(resp), write)
	}
	f.exec(`UPDATE chapters SET title = title, updated_at = '2030-01-01 00:00:00'`)
	assert.Empty(t, f.changes(credential, resp.Cursor, 0).Entries)
}

func decodeJSON(rec interface{ Result() *http.Response }, into any) error {
	resp := rec.Result()
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(into)
}

// Each File carries its own cover's version and its narrators, since a
// Book's Files can have different covers and readers.
func TestFeed_FileCarriesItsCoverAndNarrators(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Audio")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, cover_image_filename, updated_at) VALUES
		(5, 1, 1, '/dune.epub', 'epub', 1, 'dune.epub.cover.jpg', '2026-01-02 03:04:05'),
		(6, 1, 1, '/dune.m4b', 'm4b', 1, 'dune.m4b.cover.jpg', '2026-02-03 04:05:06'),
		(7, 1, 1, '/dune2.m4b', 'm4b', 1, NULL, '2026-02-03 04:05:06')`)
	f.exec(`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Scott Brick', 'Brick, Scott', 'file'), (2, 1, 'Ann Reader', 'Reader, Ann', 'file')`)
	f.exec(`INSERT INTO narrators (id, file_id, person_id, sort_order) VALUES (1, 6, 2, 2), (2, 6, 1, 1)`)
	f.addUser()
	credential := f.signIn()

	resp := f.changes(credential, 0, 0)
	book := resp.Entries[len(resp.Entries)-1].Book
	require.NotNil(t, book)
	require.Len(t, book.Files, 3)
	assert.Equal(t, fmt.Sprintf("5-%d", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).Unix()), book.Files[0].CoverVersion)
	assert.Equal(t, fmt.Sprintf("6-%d", time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC).Unix()), book.Files[1].CoverVersion)
	assert.Empty(t, book.Files[2].CoverVersion)
	assert.Equal(t, []FeedNarrator{
		{PersonID: 1, Name: "Scott Brick", SortName: "Brick, Scott"},
		{PersonID: 2, Name: "Ann Reader", SortName: "Reader, Ann"},
	}, book.Files[1].Narrators)
	assert.Empty(t, book.Files[0].Narrators)

	for _, write := range []string{
		`INSERT INTO narrators (id, file_id, person_id, sort_order) VALUES (3, 7, 1, 1)`,
		`UPDATE narrators SET sort_order = 3 WHERE id = 1`,
		`UPDATE persons SET name = 'Scott Brick Jr.' WHERE id = 1`,
		`DELETE FROM narrators WHERE id = 3`,
	} {
		cursor := resp.Cursor
		f.exec(write)
		resp = f.changes(credential, cursor, 0)
		books := []string{}
		for _, kind := range summary(resp) {
			if strings.HasPrefix(kind, "book:") {
				books = append(books, kind)
			}
		}
		assert.Equal(t, []string{"book:1"}, books, write)
	}
}
