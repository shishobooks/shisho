package shishoapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// event is one journal event as the app sends it.
func event(id, eventType string, at time.Time, payload any) map[string]any {
	return map[string]any{"id": id, "type": eventType, "at": at.Format(time.RFC3339Nano), "payload": payload}
}

// journal sends events in one batch with the device's clock at clock.
func (f *fixture) journal(credential string, clock time.Time, events ...any) JournalResponse {
	f.t.Helper()
	rec := f.journalRaw(credential, clock, events...)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp JournalResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(f.t, resp.Results, len(events))
	return resp
}

func (f *fixture) journalRaw(credential string, clock time.Time, events ...any) *httptest.ResponseRecorder {
	f.t.Helper()
	body, err := json.Marshal(map[string]any{"clock": clock.Format(time.RFC3339Nano), "events": events})
	require.NoError(f.t, err)
	rec := f.do(call{method: http.MethodPost, path: "/mobile/journal", body: string(body), credential: credential})
	return rec
}

// results names each result, as "id:result".
func results(resp JournalResponse) []string {
	out := make([]string, len(resp.Results))
	for i, r := range resp.Results {
		out[i] = r.ID + ":" + r.Result
	}
	return out
}

// audiobook adds a ten-minute audiobook, File 5 of Book 1 in Library 1.
func (f *fixture) audiobook() {
	f.t.Helper()
	f.addLibrary(1, "Audio")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, audiobook_duration_seconds) VALUES (5, 1, 1, '/dune.m4b', 'm4b', 1, 600)`)
}

// accountSince backdates the user's account, so events from before now are
// after it was made.
func (f *fixture) accountSince(userID int, at time.Time) {
	f.t.Helper()
	f.exec(`UPDATE users SET created_at = ? WHERE id = ?`, at.UTC(), userID)
}

func (f *fixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	require.NoError(f.t, f.db.QueryRowContext(f.ctx, query, args...).Scan(&n))
	return n
}

func position(fileID int, seconds, percentage float64) map[string]any {
	return map[string]any{"file_id": fileID, "seconds": seconds, "percentage": percentage}
}

// session is a Reading Session in the audiobook File.
func session(start, end time.Time) map[string]any {
	return map[string]any{
		"file_id": 5, "started_at": start.Format(time.RFC3339Nano), "ended_at": end.Format(time.RFC3339Nano),
		"start_percentage": 0.1, "end_percentage": 0.2,
	}
}

// Every event gets one result, and one the server doesn't understand or
// refuses never holds back the others.
func TestJournal_EachResultType(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	f.addLibrary(2, "Hidden")
	f.addBook(2, 2, "Emma")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (6, 2, 2, '/emma.m4b', 'm4b', 1)`)
	userID := f.addUser(1)
	now := time.Now()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()

	unknownField := position(5, 30, 0.05)
	unknownField["zoom"] = 2
	envelopeField := event("envelope", JournalEventReadingPosition, now, position(5, 30, 0.05))
	envelopeField["priority"] = "high"
	resp := f.journal(credential, now,
		event("ok", JournalEventReadingPosition, now.Add(-time.Minute), position(5, 30, 0.05)),
		event("highlight", "highlight", now, map[string]any{"file_id": 5}),
		event("field", JournalEventReadingPosition, now, unknownField),
		envelopeField,
		event("missing", JournalEventReadingPosition, now, position(99, 30, 0.05)),
		event("hidden", JournalEventReadingPosition, now, position(6, 30, 0.05)),
		event("too-far", JournalEventReadingPosition, now, position(5, 30, 2)),
		event("too-long", JournalEventReadingSession, now, session(now.Add(-25*time.Hour), now.Add(-time.Minute))),
		event("backwards", JournalEventReadingSession, now, session(now.Add(-time.Minute), now.Add(-2*time.Minute))),
		event("before-account", JournalEventReadingSession, now, session(now.Add(-3*time.Hour), now.Add(-2*time.Hour))),
		map[string]any{"type": JournalEventReadingPosition, "at": now.Format(time.RFC3339), "payload": position(5, 1, 0.1)},
	)
	assert.Equal(t, []string{
		"ok:accepted", "highlight:not_understood", "field:not_understood", "envelope:not_understood",
		"missing:refused", "hidden:refused", "too-far:refused", "too-long:refused", "backwards:refused",
		"before-account:refused", ":refused",
	}, results(resp))
	for _, r := range resp.Results[1:] {
		assert.NotEmpty(t, r.Reason, r.ID)
	}

	// The same event again is a duplicate, and the position it set stands.
	again := f.journal(credential, now, event("ok", JournalEventReadingPosition, now.Add(-time.Minute), position(5, 30, 0.05)))
	assert.Equal(t, []string{"ok:duplicate"}, results(again))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM reading_positions WHERE user_id = ?`, userID))
	// Events never understood or refused were never recorded, so a server
	// that understands them later applies them.
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM app_journal_events`))
}

// A malformed batch is refused whole, and the app keeps every event in it.
func TestJournal_MalformedBatchIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addUser()
	credential := f.signIn()

	rec := f.do(call{method: http.MethodPost, path: "/mobile/journal", body: `{"clock": "soon", "events": []}`, credential: credential})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	rec = f.do(call{method: http.MethodPost, path: "/mobile/journal", body: `{"events": []}`, credential: credential})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	rec = f.do(call{method: http.MethodPost, path: "/mobile/journal", body: `{"clock": "2026-01-01T00:00:00Z", "events": []}`})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Repeats are ignored by user, installation, and event id: after the user
// deleted what the event produced, and after the installation signed in
// again as a new Device.
func TestJournal_DedupesAcrossDeletesAndSignIns(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()
	listened := event("s1", JournalEventReadingSession, now, session(now.Add(-20*time.Minute), now.Add(-10*time.Minute)))

	assert.Equal(t, []string{"s1:accepted"}, results(f.journal(credential, now, listened)))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM reading_sessions`))
	f.exec(`DELETE FROM reading_sessions`)
	assert.Equal(t, []string{"s1:duplicate"}, results(f.journal(credential, now, listened)))

	rec := f.do(call{method: http.MethodDelete, path: "/mobile/device", credential: credential})
	require.Equal(t, http.StatusNoContent, rec.Code)
	again := f.signIn()
	assert.Equal(t, []string{"s1:duplicate"}, results(f.journal(again, now, listened)))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM reading_sessions`))

	// Another installation's event ids are its own.
	other := f.signInAs("install-2")
	assert.Equal(t, []string{"s1:accepted"}, results(f.journal(other, now, listened)))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM reading_sessions`))
}

// A clock far off is corrected by how far off it is when the batch arrives,
// as for the KOReader plugin. A little drift is left alone, and nothing is
// placed after the batch arrived.
func TestJournal_CorrectsTheClock(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now().UTC()
	f.accountSince(userID, now.Add(-48*time.Hour))
	credential := f.signIn()

	behind := 3 * time.Hour
	device := now.Add(-behind)
	resp := f.journal(credential, device,
		event("far", JournalEventReadingSession, device, session(device.Add(-20*time.Minute), device.Add(-10*time.Minute))))
	assert.Equal(t, []string{"far:accepted"}, results(resp))
	assert.InDelta(t, behind.Seconds(), resp.ClockOffsetSeconds, 5)
	var started, ended time.Time
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT started_at, ended_at FROM reading_sessions`).Scan(&started, &ended))
	assert.WithinDuration(t, now.Add(-20*time.Minute), started, 5*time.Second)
	assert.WithinDuration(t, now.Add(-10*time.Minute), ended, 5*time.Second)
	f.exec(`DELETE FROM reading_sessions`)

	drift := now.Add(-2 * time.Minute)
	f.journal(credential, drift,
		event("near", JournalEventReadingSession, drift, session(drift.Add(-20*time.Minute), drift.Add(-10*time.Minute))))
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT started_at FROM reading_sessions`).Scan(&started))
	assert.WithinDuration(t, drift.Add(-20*time.Minute), started, time.Second)
	f.exec(`DELETE FROM reading_sessions`)

	// A position from "the future" is now.
	f.journal(credential, now,
		event("future", JournalEventReadingPosition, now.Add(time.Hour), position(5, 30, 0.05)))
	var occurred time.Time
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT occurred_at FROM reading_positions`).Scan(&occurred))
	assert.WithinDuration(t, now, occurred, 5*time.Second)
}

// Listening sent days later is placed by its own time: a session on the
// Read-through underway when it happened, not the one underway now.
func TestJournal_PlacesLateListeningByItsTime(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now().UTC()
	f.accountSince(userID, now.AddDate(0, 0, -30))
	day := func(daysAgo int) string { return now.AddDate(0, 0, -daysAgo).Format("2006-01-02") }
	f.exec(`INSERT INTO read_throughs (id, user_id, book_id, outcome, begin_day, end_day, created_at, updated_at) VALUES (1, ?, 1, 'finished', ?, ?, ?, ?)`,
		userID, day(20), day(10), now, now)
	f.exec(`INSERT INTO read_throughs (id, user_id, book_id, outcome, begin_day, created_at, updated_at) VALUES (2, ?, 1, 'underway', ?, ?, ?)`,
		userID, day(5), now, now)
	credential := f.signIn()

	then := now.AddDate(0, 0, -15)
	resp := f.journal(credential, now,
		event("old", JournalEventReadingSession, then, session(then.Add(-30*time.Minute), then)))
	assert.Equal(t, []string{"old:accepted"}, results(resp))
	var readThrough int
	var source string
	var deviceID *string
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT read_through_id, source, source_device_id FROM reading_sessions`).Scan(&readThrough, &source, &deviceID))
	assert.Equal(t, 1, readThrough)
	assert.Equal(t, models.ReadingSourceShishoApp, source)
	require.NotNil(t, deviceID)

	// A position older than the one saved keeps the saved one.
	f.journal(credential, now, event("new", JournalEventReadingPosition, now.Add(-time.Minute), position(5, 300, 0.5)))
	f.journal(credential, now, event("stale", JournalEventReadingPosition, then, position(5, 10, 0.01)))
	var seconds float64
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT seconds FROM reading_positions`).Scan(&seconds))
	assert.InDelta(t, 300.0, seconds, 0.001)
}

// Listening starts and finishes a Read-through by the server's rules, as in
// the web player: a minute of it starts one, and reaching the end finishes
// it.
func TestJournal_ListeningStartsAndFinishesTheReadThrough(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now().UTC()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()

	brief := position(5, 20, 0.03)
	brief["listened_seconds"] = 20
	f.journal(credential, now, event("brief", JournalEventReadingPosition, now.Add(-10*time.Minute), brief))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM read_throughs`))

	sitting := position(5, 120, 0.2)
	sitting["listened_seconds"] = 90
	sitting["listening_since"] = now.Add(-5 * time.Minute).Format(time.RFC3339)
	f.journal(credential, now, event("minute", JournalEventReadingPosition, now.Add(-3*time.Minute), sitting))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM read_throughs WHERE outcome = 'underway'`))

	end := position(5, 599, 0.998)
	end["at_end"] = true
	f.journal(credential, now, event("end", JournalEventReadingPosition, now.Add(-time.Minute), end))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM read_throughs WHERE outcome = 'finished'`))
	var source string
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT source FROM reading_positions`).Scan(&source))
	assert.Equal(t, models.ReadingSourceShishoApp, source)
}

// signInAs signs the user in from another installation.
func (f *fixture) signInAs(installation string) string {
	f.t.Helper()
	body, err := json.Marshal(SignInPayload{
		Username: "reader", Password: testPassword, InstallationID: installation, DeviceName: "iPad", AppVersion: "1.0.0 (1)",
	})
	require.NoError(f.t, err)
	rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: string(body)})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp SignInResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Credential
}

// An event the server failed to apply was never taken in: the batch fails,
// the app keeps it, and sending it again applies it once.
func TestJournal_AFaultWhileApplyingLeavesTheEventToResend(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audiobook()
	userID := f.addUser()
	now := time.Now()
	f.accountSince(userID, now.Add(-time.Hour))
	credential := f.signIn()
	listened := event("s1", JournalEventReadingSession, now, session(now.Add(-20*time.Minute), now.Add(-10*time.Minute)))

	f.exec(`CREATE TRIGGER fail_sessions BEFORE INSERT ON reading_sessions BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	rec := f.journalRaw(credential, now, listened)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM app_journal_events`))

	f.exec(`DROP TRIGGER fail_sessions`)
	assert.Equal(t, []string{"s1:accepted"}, results(f.journal(credential, now, listened)))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM reading_sessions`))
}
