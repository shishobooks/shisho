package shishoapp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readingFixture is a user with two Books in one Library, signed in from the
// app, whose account is old enough for events from the last day.
type readingFixture struct {
	*fixture
	userID     int
	credential string
	now        time.Time
}

func newReadingFixture(t *testing.T) *readingFixture {
	t.Helper()
	f := newFixture(t)
	f.addLibrary(1, "Books")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 1, "Emma")
	f.addBook(3, 1, "Ivanhoe")
	f.addLibrary(2, "Hidden")
	f.addBook(9, 2, "Secret")
	userID := f.addUser(1)
	now := time.Now().UTC().Truncate(time.Second)
	f.accountSince(userID, now.Add(-48*time.Hour))
	return &readingFixture{fixture: f, userID: userID, credential: f.signIn(), now: now}
}

// send sends events in one batch and returns their results.
func (f *readingFixture) send(events ...any) []string {
	f.t.Helper()
	return results(f.journal(f.credential, time.Now().UTC(), events...))
}

// ago is an instant d before the fixture's now.
func (f *readingFixture) ago(d time.Duration) time.Time { return f.now.Add(-d) }

func book(id int) map[string]any { return map[string]any{"book_id": id, "day": "2026-03-01"} }

// status is the user's Reading Status of bookID as the server derives it.
func (f *readingFixture) status(bookID int) string {
	f.t.Helper()
	var status sql.NullString
	require.NoError(f.t, f.db.QueryRowContext(f.ctx, `SELECT `+reading.StatusExpr(f.userID, strconv.Itoa(bookID))).Scan(&status))
	return status.String
}

// readThroughs lists the user's Read-throughs of bookID, oldest first, as
// "outcome", "outcome paused", or "outcome history".
func (f *readingFixture) readThroughs(bookID int) []string {
	f.t.Helper()
	rows, err := f.db.QueryContext(f.ctx, `SELECT outcome, paused, history_only, begin_day IS NULL FROM read_throughs
		WHERE user_id = ? AND book_id = ? ORDER BY id`, f.userID, bookID)
	require.NoError(f.t, err)
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var outcome string
		var paused, history, undated bool
		require.NoError(f.t, rows.Scan(&outcome, &paused, &history, &undated))
		switch {
		case paused:
			outcome += " paused"
		case history:
			outcome += " history"
		}
		out = append(out, outcome)
	}
	require.NoError(f.t, rows.Err())
	return out
}

// choose stamps the user's latest explicit status choice of bookID at at,
// with whether it was Reference, as a choice made on the web then does.
func (f *readingFixture) choose(bookID int, reference bool, at time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO user_books (user_id, book_id, reference, status_chosen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, book_id) DO UPDATE SET reference = excluded.reference, status_chosen_at = excluded.status_chosen_at`,
		f.userID, bookID, reference, at)
}

func (f *readingFixture) toRead() []int {
	f.t.Helper()
	rows, err := f.db.QueryContext(f.ctx, `SELECT book_id FROM user_books WHERE user_id = ? AND to_read_position IS NOT NULL
		ORDER BY to_read_position, book_id`, f.userID)
	require.NoError(f.t, err)
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var id int
		require.NoError(f.t, rows.Scan(&id))
		out = append(out, id)
	}
	return out
}

// Each Read-through action does what the web's status menu does, at the
// event's time.
func TestJournal_ReadThroughActions(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)

	assert.Equal(t, []string{"start:accepted", "pause:accepted", "resume:accepted", "finish:accepted"}, f.send(
		event("start", JournalEventReadThroughStart, f.ago(40*time.Minute), book(1)),
		event("pause", JournalEventReadThroughPause, f.ago(30*time.Minute), book(1)),
		event("resume", JournalEventReadThroughResume, f.ago(20*time.Minute), book(1)),
		event("finish", JournalEventReadThroughFinish, f.ago(10*time.Minute), book(1)),
	))
	assert.Equal(t, []string{"finished"}, f.readThroughs(1))
	assert.Equal(t, "read", f.status(1))
	var begin, end string
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT begin_day, end_day FROM read_throughs WHERE book_id = 1`).Scan(&begin, &end))
	assert.Equal(t, "2026-03-01", begin, "dated by the phone's day while the user has set no time zone")

	// Marking Did Not Finish needs something underway; Mark as read with
	// nothing underway records a finish, as on the web.
	assert.Equal(t, []string{"abandon:refused", "read:accepted"}, f.send(
		event("abandon", JournalEventReadThroughAbandon, f.ago(5*time.Minute), book(2)),
		event("read", JournalEventReadThroughFinish, f.ago(5*time.Minute), book(2)),
	))
	assert.Equal(t, []string{"finished"}, f.readThroughs(2))

	// Starting again and abandoning.
	assert.Equal(t, []string{"again:accepted", "dnf:accepted"}, f.send(
		event("again", JournalEventReadThroughStart, f.ago(4*time.Minute), book(2)),
		event("dnf", JournalEventReadThroughAbandon, f.ago(3*time.Minute), book(2)),
	))
	assert.Equal(t, []string{"finished", "abandoned"}, f.readThroughs(2))
	assert.Equal(t, "did_not_finish", f.status(2))

	// A Book the user can't reach is refused.
	assert.Equal(t, []string{"hidden:refused"}, f.send(event("hidden", JournalEventReadThroughStart, f.ago(time.Minute), book(9))))
}

// A delete names a Read-through by its id, or the one underway in a Book,
// as Mark as not started does. One gone already stays gone.
func TestJournal_ReadThroughDelete(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	f.send(
		event("start", JournalEventReadThroughStart, f.ago(time.Hour), book(1)),
		event("read", JournalEventReadThroughFinish, f.ago(time.Hour), book(2)),
	)
	var finishedID int
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT id FROM read_throughs WHERE book_id = 2`).Scan(&finishedID))

	assert.Equal(t, []string{"underway:accepted", "byID:accepted", "again:accepted", "both:refused"}, f.send(
		event("underway", JournalEventReadThroughDelete, f.ago(30*time.Minute), map[string]any{"book_id": 1, "day": "2026-03-01"}),
		event("byID", JournalEventReadThroughDelete, f.ago(30*time.Minute), map[string]any{"read_through_id": finishedID}),
		event("again", JournalEventReadThroughDelete, f.ago(20*time.Minute), map[string]any{"read_through_id": finishedID}),
		event("both", JournalEventReadThroughDelete, f.ago(20*time.Minute), map[string]any{"read_through_id": finishedID, "book_id": 2}),
	))
	assert.Empty(t, f.readThroughs(1))
	assert.Empty(t, f.readThroughs(2))

	// The Read-through underway began after the day of the delete, so it's
	// another pass than the one the phone deleted.
	f.send(event("later", JournalEventReadThroughStart, f.ago(time.Minute), map[string]any{"book_id": 3, "day": "2026-03-05"}))
	f.send(event("stale", JournalEventReadThroughDelete, f.ago(30*time.Second), map[string]any{"book_id": 3, "day": "2026-03-04"}))
	assert.Equal(t, []string{"underway"}, f.readThroughs(3))
}

// A status choice dated before the user's latest explicit choice elsewhere
// doesn't apply (ADR 0028): the later choice wins.
func TestJournal_AStatusChoiceBeforeALaterChoiceDoesntApply(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	require.NoError(t, f.h.reading.End(f.ctx, f.userID, 1, "finished", "2026-03-02"))

	assert.Equal(t, []string{"start:accepted", "toRead:accepted", "pause:accepted"}, f.send(
		event("start", JournalEventReadThroughStart, f.ago(time.Hour), book(1)),
		event("toRead", JournalEventToReadAdd, f.ago(time.Hour), book(1)),
		event("pause", JournalEventReadThroughPause, f.ago(time.Hour), book(1)),
	))
	assert.Equal(t, []string{"finished"}, f.readThroughs(1))
	assert.Equal(t, "read", f.status(1))
	assert.Empty(t, f.toRead())
}

// Reference ordering (ADR 0028), each case in both arrival orders where
// that matters.
func TestJournal_ReferenceOrdering(t *testing.T) {
	t.Parallel()

	// A finish made offline before Reference was chosen elsewhere is
	// recorded as history: a finished Read-through with no begin day, which
	// takes no Reading Sessions, sent before or after it. Reference and To
	// Read stand.
	t.Run("a finish before a later Reference choice is history only", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
			VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 6000)`)
		f.send(event("start", JournalEventReadThroughStart, f.ago(3*time.Hour), book(1)))
		before := session(f.ago(2*time.Hour), f.ago(2*time.Hour-10*time.Minute))
		assert.Equal(t, []string{"before:accepted"}, f.send(event("before", JournalEventReadingSession, f.ago(110*time.Minute), before)))
		// Reference on the web, after the phone's finish below.
		_, err := f.h.reading.BulkReference(f.ctx, f.userID, []int{1}) // Dissolves the start.
		require.NoError(t, err)

		today := map[string]any{"book_id": 1, "day": f.ago(time.Hour).Format(time.DateOnly)}
		assert.Equal(t, []string{"finish:accepted"}, f.send(event("finish", JournalEventReadThroughFinish, f.ago(time.Hour), today)))
		assert.Equal(t, []string{"finished history"}, f.readThroughs(1))
		assert.Equal(t, "reference", f.status(1))

		// A session sent after the finish, from the day it covers, stays
		// detached too.
		after := session(f.ago(90*time.Minute), f.ago(80*time.Minute))
		assert.Equal(t, []string{"after:accepted"}, f.send(event("after", JournalEventReadingSession, f.ago(80*time.Minute), after)))
		assert.Equal(t, 2, f.count(`SELECT COUNT(*) FROM reading_sessions WHERE read_through_id IS NULL`))

		// Removing Reference later doesn't make it take sessions.
		require.NoError(t, f.h.reading.RemoveReference(f.ctx, f.userID, 1))
		late := session(f.ago(70*time.Minute), f.ago(60*time.Minute))
		f.send(event("late", JournalEventReadingSession, f.ago(60*time.Minute), late))
		assert.Equal(t, 3, f.count(`SELECT COUNT(*) FROM reading_sessions WHERE read_through_id IS NULL`))
	})

	// A finish made offline before a later choice that wasn't Reference is
	// kept as history, unless that choice ended the pass itself.
	t.Run("a finish before a later non-Reference choice is kept as history", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		today := func(id int) map[string]any {
			return map[string]any{"book_id": id, "day": f.ago(time.Hour).Format(time.DateOnly)}
		}
		require.NoError(t, f.h.reading.AddToRead(f.ctx, f.userID, 1))
		require.NoError(t, f.h.reading.End(f.ctx, f.userID, 2, "finished", f.now.Format(time.DateOnly)))
		assert.Equal(t, []string{"start:accepted", "finish:accepted", "abandon:accepted"}, f.send(
			event("start", JournalEventReadThroughStart, f.ago(2*time.Hour), today(1)),
			event("finish", JournalEventReadThroughFinish, f.ago(time.Hour), today(1)),
			event("abandon", JournalEventReadThroughAbandon, f.ago(time.Hour), today(2)),
		))
		assert.Equal(t, []string{"finished history"}, f.readThroughs(1))
		assert.Equal(t, "to_read", f.status(1))
		assert.Equal(t, []string{"finished"}, f.readThroughs(2), "the web's Mark as read already ended it")
	})

	// Mark as read on the web dates the finish by the last reading, which
	// can be before an abandon the phone made offline: the web's later
	// choice still stands, and nothing is added.
	t.Run("a later web finish dated earlier still wins", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		monday := f.now.Add(-48 * time.Hour).Format(time.DateOnly)
		require.NoError(t, f.h.reading.Start(f.ctx, f.userID, 1, monday))
		f.exec(`UPDATE user_books SET status_chosen_at = ? WHERE user_id = ?`, f.ago(72*time.Hour), f.userID)
		require.NoError(t, f.h.reading.End(f.ctx, f.userID, 1, "finished", monday))
		tuesday := map[string]any{"book_id": 1, "day": f.now.Add(-24 * time.Hour).Format(time.DateOnly)}
		f.send(event("dnf", JournalEventReadThroughAbandon, f.ago(24*time.Hour), tuesday))
		assert.Equal(t, []string{"finished"}, f.readThroughs(1))
		assert.Equal(t, "read", f.status(1))
	})

	// A delete or finish of the Read-through underway leaves one started
	// after it alone, even the same day.
	t.Run("a late delete or finish leaves a later pass alone", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		today := map[string]any{"book_id": 1, "day": f.now.Format(time.DateOnly)}
		require.NoError(t, f.h.reading.Start(f.ctx, f.userID, 1, f.now.Format(time.DateOnly)))
		f.exec(`UPDATE user_books SET status_chosen_at = ? WHERE user_id = ?`, f.ago(3*time.Hour), f.userID)
		assert.Equal(t, []string{"delete:accepted", "abandon:accepted"}, f.send(
			event("delete", JournalEventReadThroughDelete, f.ago(2*time.Hour), today),
			event("abandon", JournalEventReadThroughAbandon, f.ago(2*time.Hour), today),
		))
		assert.Equal(t, []string{"underway", "abandoned history"}, f.readThroughs(1))
	})

	// A Reference choice made offline before a later explicit choice
	// elsewhere doesn't apply, including Remove from Reference.
	t.Run("a Reference choice before a later choice doesn't apply", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		require.NoError(t, f.h.reading.End(f.ctx, f.userID, 1, "finished", "2026-03-02"))
		f.choose(2, false, f.ago(10*time.Minute)) // Remove from Reference on the web.

		assert.Equal(t, []string{"mark:accepted", "mark2:accepted"}, f.send(
			event("mark", JournalEventReferenceMark, f.ago(time.Hour), book(1)),
			event("mark2", JournalEventReferenceMark, f.ago(time.Hour), book(2)),
		))
		assert.Equal(t, "read", f.status(1))
		assert.Empty(t, f.status(2))

		// Dated after the later choice, it applies.
		assert.Equal(t, []string{"now:accepted"}, f.send(event("now", JournalEventReferenceMark, f.ago(time.Minute), book(2))))
		assert.Equal(t, "reference", f.status(2))
	})

	// A start or resume made offline before a later Reference choice
	// doesn't apply: choosing Reference would have ended it.
	t.Run("a start before a later Reference choice doesn't apply", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		f.choose(1, true, f.ago(10*time.Minute))
		assert.Equal(t, []string{"start:accepted", "resume:accepted"}, f.send(
			event("start", JournalEventReadThroughStart, f.ago(time.Hour), book(1)),
			event("resume", JournalEventReadThroughResume, f.ago(time.Hour), book(1)),
		))
		assert.Empty(t, f.readThroughs(1))
		assert.Equal(t, "reference", f.status(1))
	})

	// The reverse: Reference chosen offline, Mark as read later on the web.
	// The Book shows Read.
	t.Run("Mark as read on the web after an offline Reference choice wins", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		at := f.ago(time.Hour)
		require.NoError(t, f.h.reading.End(f.ctx, f.userID, 1, "finished", "2026-03-02"))
		f.send(event("ref", JournalEventReferenceMark, at, book(1)))
		assert.Equal(t, "read", f.status(1))
	})

	// A choice that matches the Book's state is still the latest choice:
	// another phone's older one, arriving after it, doesn't apply.
	t.Run("a choice matching the state still orders later ones", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		f.choose(1, true, f.ago(2*time.Hour))
		assert.Equal(t, []string{"remove:accepted", "keep:accepted", "older:accepted"}, f.send(
			event("remove", JournalEventReferenceRemove, f.ago(90*time.Minute), book(1)),
			event("keep", JournalEventReferenceRemove, f.ago(30*time.Minute), book(1)),
			event("older", JournalEventReferenceMark, f.ago(time.Hour), book(1)),
		))
		assert.Empty(t, f.status(1))
	})

	// Equal instants: the event applies, as the later arrival.
	t.Run("an event at the same instant as the choice applies", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		at := f.ago(time.Hour)
		f.choose(1, true, at)
		assert.Equal(t, []string{"remove:accepted"}, f.send(event("remove", JournalEventReferenceRemove, at, book(1))))
		assert.Empty(t, f.status(1))
	})

	// Reading never starts or finishes a Read-through in a Reference Book,
	// whichever arrives first.
	t.Run("positions never start a Reference Book", func(t *testing.T) {
		t.Parallel()
		f := newReadingFixture(t)
		f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
			VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 600)`)
		listened := position(5, 120, 0.2)
		listened["listened_seconds"] = 90
		atEnd := position(5, 599, 0.998)
		atEnd["at_end"] = true
		assert.Equal(t, []string{"ref:accepted", "listen:accepted", "end:accepted"}, f.send(
			event("ref", JournalEventReferenceMark, f.ago(time.Hour), book(1)),
			event("listen", JournalEventReadingPosition, f.ago(50*time.Minute), listened),
			event("end", JournalEventReadingPosition, f.ago(40*time.Minute), atEnd),
		))
		assert.Empty(t, f.readThroughs(1))
		assert.Equal(t, "reference", f.status(1))
	})
}

// The latest Rating wins, by the instant each was made.
func TestJournal_Rating(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	rating := func(bookID int) sql.NullInt64 {
		var units sql.NullInt64
		err := f.db.QueryRowContext(f.ctx, `SELECT rating FROM user_books WHERE user_id = ? AND book_id = ?`, f.userID, bookID).Scan(&units)
		if err == sql.ErrNoRows {
			return sql.NullInt64{}
		}
		require.NoError(t, err)
		return units
	}

	assert.Equal(t, []string{"set:accepted", "clear:accepted", "set2:accepted", "bad:refused"}, f.send(
		event("set", JournalEventRatingSet, f.ago(time.Hour), map[string]any{"book_id": 1, "rating": 18}),
		event("clear", JournalEventRatingClear, f.ago(time.Hour), book(2)),
		event("set2", JournalEventRatingSet, f.ago(time.Hour), map[string]any{"book_id": 2, "rating": 8}),
		event("bad", JournalEventRatingSet, f.ago(time.Hour), map[string]any{"book_id": 3, "rating": 21}),
	))
	assert.Equal(t, int64(18), rating(1).Int64)
	assert.Equal(t, int64(8), rating(2).Int64)

	// Rated on the web since: an older Rating from the phone loses, and a
	// newer one wins.
	units := 12
	require.NoError(t, f.h.reading.SetRating(f.ctx, f.userID, 1, &units))
	f.send(event("older", JournalEventRatingClear, f.ago(30*time.Minute), book(1)))
	assert.Equal(t, int64(12), rating(1).Int64)
	f.send(event("newer", JournalEventRatingClear, f.now.Add(time.Hour), book(1)))
	assert.False(t, rating(1).Valid)
}

// To Read changes apply at their instants, and for its order the latest
// move of each Book wins.
func TestJournal_ToRead(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)

	assert.Equal(t, []string{"a:accepted", "b:accepted", "c:accepted", "top:accepted", "after:accepted"}, f.send(
		event("a", JournalEventToReadAdd, f.ago(5*time.Hour), book(1)),
		event("b", JournalEventToReadAdd, f.ago(5*time.Hour), book(2)),
		event("c", JournalEventToReadAdd, f.ago(5*time.Hour), book(3)),
		event("top", JournalEventToReadMove, f.ago(4*time.Hour), map[string]any{"book_id": 3}),
		event("after", JournalEventToReadMove, f.ago(4*time.Hour), map[string]any{"book_id": 1, "after_book_id": 2}),
	))
	assert.Equal(t, []int{3, 2, 1}, f.toRead())

	// The web moves Book 2 to the top. Phone moves made earlier, of Books 2
	// and 3 to the end, arrive late: Book 2's loses, as the web moved it
	// later, and Book 3's wins, as the web only shifted it.
	require.NoError(t, f.h.reading.ReorderToRead(f.ctx, f.userID, []int{2, 3, 1}))
	assert.Equal(t, []string{"two:accepted", "three:accepted"}, f.send(
		event("two", JournalEventToReadMove, f.ago(time.Hour), map[string]any{"book_id": 2, "after_book_id": 1}),
		event("three", JournalEventToReadMove, f.ago(time.Hour), map[string]any{"book_id": 3, "after_book_id": 1}),
	))
	assert.Equal(t, []int{2, 1, 3}, f.toRead())

	// A removal older than the Book's last move doesn't apply; a newer one
	// does.
	assert.Equal(t, []string{"stale:accepted", "fresh:accepted"}, f.send(
		event("stale", JournalEventToReadRemove, f.ago(2*time.Hour), book(2)),
		event("fresh", JournalEventToReadRemove, f.now.Add(time.Hour), book(1)),
	))
	assert.Equal(t, []int{2, 3}, f.toRead())
}

// The web's reorder marks only the Books it moved, not the ones it shifted.
func TestReorderToRead_StampsOnlyTheBooksMoved(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	for _, id := range []int{1, 2, 3} {
		require.NoError(t, f.h.reading.AddToRead(f.ctx, f.userID, id))
	}
	f.exec(`UPDATE user_books SET to_read_changed_at = NULL`)
	require.NoError(t, f.h.reading.ReorderToRead(f.ctx, f.userID, []int{3, 1, 2}))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM user_books WHERE to_read_changed_at IS NOT NULL`))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM user_books WHERE to_read_changed_at IS NOT NULL AND book_id = 3`))
}

// Lists made in the app are named by their local id until the feed brings
// the server's, and their Books follow the same latest-move rule.
func TestJournal_Lists(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	local := "6f0c8b7e-0000-4000-8000-000000000001"
	with := func(fields map[string]any) map[string]any {
		out := map[string]any{"local_id": local}
		for k, v := range fields {
			out[k] = v
		}
		return out
	}

	assert.Equal(t, []string{"create:accepted", "add1:accepted", "add2:accepted", "add3:accepted", "move:accepted",
		"rename:accepted", "hidden:refused"}, f.send(
		event("create", JournalEventListCreate, f.ago(5*time.Hour), map[string]any{"local_id": local, "name": "Summer", "is_ordered": true}),
		event("add1", JournalEventListAdd, f.ago(5*time.Hour), with(map[string]any{"book_id": 1})),
		event("add2", JournalEventListAdd, f.ago(5*time.Hour), with(map[string]any{"book_id": 2})),
		event("add3", JournalEventListAdd, f.ago(5*time.Hour), with(map[string]any{"book_id": 3})),
		event("move", JournalEventListMove, f.ago(4*time.Hour), with(map[string]any{"book_id": 3})),
		event("rename", JournalEventListRename, f.ago(4*time.Hour), with(map[string]any{"name": "Summer Reads"})),
		event("hidden", JournalEventListAdd, f.ago(4*time.Hour), with(map[string]any{"book_id": 9})),
	))
	var listID int
	var name string
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT id, name FROM lists WHERE user_id = ?`, f.userID).Scan(&listID, &name))
	assert.Equal(t, "Summer Reads", name)

	// The feed tells the app which List is its own.
	var doc *FeedList
	for _, entry := range f.changes(f.credential, 0, 0).Entries {
		if entry.List != nil {
			doc = entry.List
		}
	}
	require.NotNil(t, doc)
	require.NotNil(t, doc.LocalID)
	assert.Equal(t, local, *doc.LocalID)
	order := func() []int {
		var ids []int
		rows, err := f.db.QueryContext(f.ctx, `SELECT book_id FROM list_books WHERE list_id = ? ORDER BY sort_order`, listID)
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var id int
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		return ids
	}
	assert.Equal(t, []int{3, 1, 2}, order())

	// The web moves Book 3 to the end. Older phone moves arrive late: Book
	// 3's loses, as the web moved it later, and Book 1's wins, as the web
	// only shifted it. A removal made after Book 2 was last placed applies.
	require.NoError(t, f.h.lists.ReorderBooks(f.ctx, lists.ReorderBooksOptions{ListID: listID, BookIDs: []int{1, 2, 3}}))
	byID := map[string]any{"list_id": listID}
	assert.Equal(t, []string{"m3:accepted", "m1:accepted", "remove:accepted"}, f.send(
		event("m3", JournalEventListMove, f.ago(time.Hour), with(map[string]any{"book_id": 3})),
		event("m1", JournalEventListMove, f.ago(time.Hour), map[string]any{"list_id": listID, "book_id": 1, "after_book_id": 2}),
		event("remove", JournalEventListRemove, f.ago(time.Hour), map[string]any{"list_id": listID, "book_id": 2}),
	))
	assert.Equal(t, []int{1, 3}, order())

	// A delete wins: events after it change nothing, and the create sent
	// again doesn't bring it back.
	assert.Equal(t, []string{"delete:accepted", "addLate:accepted", "createAgain:accepted"}, f.send(
		event("delete", JournalEventListDelete, f.ago(30*time.Minute), byID),
		event("addLate", JournalEventListAdd, f.ago(20*time.Minute), with(map[string]any{"book_id": 1})),
		event("createAgain", JournalEventListCreate, f.ago(5*time.Hour), map[string]any{"local_id": local, "name": "Summer"}),
	))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM lists`))
}

// Lists the user doesn't own or may only view refuse what they can't do,
// and a name the user already uses is refused.
func TestJournal_ListPermissions(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	var ownerID int
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`INSERT INTO users (username, password_hash, role_id, is_active) VALUES ('owner', 'x', 1, TRUE) RETURNING id`).Scan(&ownerID))
	f.exec(`INSERT INTO lists (id, user_id, name, is_ordered, default_sort, created_at, updated_at)
		VALUES (7, ?, 'Shared', FALSE, 'added_at_desc', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, ownerID)
	f.exec(`INSERT INTO list_shares (list_id, user_id, permission, created_at) VALUES (7, ?, 'viewer', CURRENT_TIMESTAMP)`, f.userID)
	f.exec(`INSERT INTO lists (id, user_id, name, is_ordered, default_sort, created_at, updated_at)
		VALUES (8, ?, 'Mine', FALSE, 'added_at_desc', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, f.userID)

	shared := map[string]any{"list_id": 7}
	assert.Equal(t, []string{"add:refused", "rename:refused", "delete:refused", "taken:refused", "noRef:refused"}, f.send(
		event("add", JournalEventListAdd, f.ago(time.Hour), map[string]any{"list_id": 7, "book_id": 1}),
		event("rename", JournalEventListRename, f.ago(time.Hour), map[string]any{"list_id": 7, "name": "Mine now"}),
		event("delete", JournalEventListDelete, f.ago(time.Hour), shared),
		event("taken", JournalEventListCreate, f.ago(time.Hour), map[string]any{"local_id": "x1", "name": "Mine"}),
		event("noRef", JournalEventListAdd, f.ago(time.Hour), map[string]any{"local_id": "x1", "book_id": 1}),
	))
}

// Clearing a File's place removes only a place saved at or before the
// clear: one saved since by another reader stands. An automatic finish from
// before the clear is kept from then on.
func TestJournal_PositionClear(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
		VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 600)`)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
		VALUES (6, 1, 2, '/emma.m4b', 'm4b', 1, 600)`)
	listened := position(5, 120, 0.2)
	listened["listened_seconds"] = 90
	atEnd := position(5, 599, 0.998)
	atEnd["at_end"] = true
	f.send(
		event("listen", JournalEventReadingPosition, f.ago(3*time.Hour), listened),
		event("end", JournalEventReadingPosition, f.ago(2*time.Hour), atEnd),
		event("emma", JournalEventReadingPosition, f.ago(10*time.Minute), position(6, 60, 0.1)),
	)
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM reading_position_updates WHERE auto_finished`))

	assert.Equal(t, []string{"clear:accepted", "stale:accepted", "gone:refused"}, f.send(
		event("clear", JournalEventPositionClear, f.ago(time.Hour), map[string]any{"file_id": 5}),
		event("stale", JournalEventPositionClear, f.ago(time.Hour), map[string]any{"file_id": 6}),
		event("gone", JournalEventPositionClear, f.ago(time.Hour), map[string]any{"file_id": 99}),
	))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM reading_positions WHERE file_id = 5`))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM reading_positions WHERE file_id = 6`), "a place saved after the clear stands")
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM reading_position_updates WHERE auto_finished`))
	assert.Equal(t, []string{"finished"}, f.readThroughs(1))
}

// The stats route sends the user's Reading Stats, with the zone their days
// are in and which place is this Device.
func TestStats_SendsTheUsersReadingStats(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
		VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 6000)`)
	f.send(
		event("s", JournalEventReadingSession, f.ago(time.Hour), session(f.ago(2*time.Hour), f.ago(time.Hour))),
		event("read", JournalEventReadThroughFinish, f.ago(time.Hour), book(2)),
	)
	rec := f.do(call{path: "/mobile/stats", credential: f.credential})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp StatsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp.Year)
	assert.Equal(t, "UTC", resp.TimeZone)
	assert.Equal(t, 1, resp.Totals.Finished)
	assert.Equal(t, 3600, resp.Totals.ListeningSeconds)
	require.Len(t, resp.Places, 1)
	require.NotNil(t, resp.Places[0].DeviceID)
	assert.Equal(t, deviceIDFor(t, f.fixture, f.credential), *resp.Places[0].DeviceID)

	year := f.now.Year()
	rec = f.do(call{path: "/mobile/stats?year=" + strconv.Itoa(year), credential: f.credential})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// A reorder that names the Book dragged marks that Book, even where the
// orders alone can't tell it from its neighbor.
func TestReorderToRead_MarksTheBookNamedAsMoved(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	for _, id := range []int{1, 2, 3} {
		require.NoError(t, f.h.reading.AddToRead(f.ctx, f.userID, id))
	}
	f.exec(`UPDATE user_books SET to_read_changed_at = NULL`)
	moved := 1
	require.NoError(t, f.h.reading.ReorderToReadMoving(f.ctx, f.userID, []int{2, 1, 3}, &moved))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM user_books WHERE to_read_changed_at IS NOT NULL AND book_id = 1`))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM user_books WHERE to_read_changed_at IS NOT NULL`))
}

// Status changes made offline after reading offline apply in the order they
// happened, though the reading started the Read-through only when it
// arrived: a pause, then Mark as read, and Mark as not started on another.
func TestJournal_StatusChangesAfterReadingOffline(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
		VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 6000)`)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds)
		VALUES (6, 1, 2, '/emma.m4b', 'm4b', 1, 6000)`)
	listen := func(fileID int) map[string]any {
		p := position(fileID, 120, 0.02)
		p["listened_seconds"] = 90
		return p
	}
	assert.Equal(t, []string{"listen:accepted", "pause:accepted", "listen2:accepted", "notStarted:accepted"}, f.send(
		event("listen", JournalEventReadingPosition, f.ago(time.Hour), listen(5)),
		event("pause", JournalEventReadThroughPause, f.ago(50*time.Minute), book(1)),
		event("listen2", JournalEventReadingPosition, f.ago(time.Hour), listen(6)),
		event("notStarted", JournalEventReadThroughDelete, f.ago(50*time.Minute), map[string]any{"book_id": 2, "day": f.now.Format(time.DateOnly)}),
	))
	assert.Equal(t, []string{"underway paused"}, f.readThroughs(1))
	assert.Empty(t, f.readThroughs(2))

	today := map[string]any{"book_id": 1, "day": f.now.Format(time.DateOnly)}
	assert.Equal(t, []string{"read:accepted"}, f.send(event("read", JournalEventReadThroughFinish, f.ago(40*time.Minute), today)))
	assert.Equal(t, []string{"finished"}, f.readThroughs(1))
	assert.Equal(t, "read", f.status(1))
}

// A change sent late is stamped with when it happened, for ordering, and is
// still found by device sync, which looks for what changed since it last
// pulled.
func TestJournal_LateChangesAreFoundByDeviceSync(t *testing.T) {
	t.Parallel()
	f := newReadingFixture(t)
	since := time.Now()
	f.send(
		event("rate", JournalEventRatingSet, f.ago(2*time.Hour), map[string]any{"book_id": 1, "rating": 12}),
		event("read", JournalEventReadThroughFinish, f.ago(2*time.Hour), book(2)),
	)
	changed, err := f.h.reading.ChangedSince(f.ctx, f.userID, since)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{1, 2}, changed)
}
