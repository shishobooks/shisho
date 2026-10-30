package reading

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createFile(t *testing.T, f *positionsFixture, book *models.Book) *models.File {
	t.Helper()
	return f.addFile(book, fmt.Sprintf("book-%d.epub", book.ID))
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(body, &v))
	return v
}

func TestPositionRoutes_SaveReadAndFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	api := newReadingAPI(t, f.db, f.user)
	path := fmt.Sprintf("/files/%d/position", f.file.ID)

	empty := api.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, empty.Code, empty.Body.String())
	assert.JSONEq(t, `{"position": null}`, empty.Body.String())

	rec := api.do(http.MethodPost, path, map[string]any{
		"cfi": "epubcfi(/6/4!/4/2)", "percentage": 0.25,
		"session": map[string]any{
			"started_at": "2026-10-15T11:50:00Z", "ended_at": "2026-10-15T12:00:00Z",
			"start_percentage": 0.2, "end_percentage": 0.25,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	resp := decode[UpdatePositionResponse](t, rec.Body.Bytes())
	assert.True(t, resp.Accepted)
	assert.True(t, resp.Started)
	assert.Equal(t, f.book.ID, resp.BookID)
	assert.Equal(t, 1, f.count("reading_sessions"))

	got := decode[FilePositionResponse](t, api.do(http.MethodGet, path, nil).Body.Bytes())
	require.NotNil(t, got.Position)
	assert.Equal(t, "epubcfi(/6/4!/4/2)", *got.Position.CFI)
	assert.InDelta(t, 0.25, got.Position.Percentage, 1e-9)

	rec = api.do(http.MethodPost, path, map[string]any{"cfi": "epubcfi(/6/40!/4/2)", "percentage": 1, "at_end": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	resp = decode[UpdatePositionResponse](t, rec.Body.Bytes())
	require.NotNil(t, resp.FinishedReadThroughID)

	undo := api.ok(http.MethodPost, fmt.Sprintf("/read-throughs/%d/undo-finish", *resp.FinishedReadThroughID), nil)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(undo))

	again := api.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/undo-finish", *resp.FinishedReadThroughID), nil)
	assert.Equal(t, http.StatusUnprocessableEntity, again.Code, again.Body.String())
}

func TestPositionRoutes_RejectInvalidUpdates(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	api := newReadingAPI(t, f.db, f.user)
	path := fmt.Sprintf("/files/%d/position", f.file.ID)

	for name, body := range map[string]map[string]any{
		"no position":         {"percentage": 0.5},
		"percentage above 1":  {"cfi": "epubcfi(/6/4!/4/2)", "percentage": 1.5},
		"negative percentage": {"cfi": "epubcfi(/6/4!/4/2)", "percentage": -0.1},
		"session ends before it starts": {"cfi": "epubcfi(/6/4!/4/2)", "percentage": 0.5, "session": map[string]any{
			"started_at": "2026-10-15T12:00:00Z", "ended_at": "2026-10-15T11:00:00Z",
			"start_percentage": 0.4, "end_percentage": 0.5,
		}},
		"session ending in the future": {"cfi": "epubcfi(/6/4!/4/2)", "percentage": 0.5, "session": map[string]any{
			"started_at": "2999-01-01T11:00:00Z", "ended_at": "2999-01-01T12:00:00Z",
			"start_percentage": 0.4, "end_percentage": 0.5,
		}},
		"session longer than a day": {"cfi": "epubcfi(/6/4!/4/2)", "percentage": 0.5, "session": map[string]any{
			"started_at": "2026-10-01T00:00:00Z", "ended_at": "2026-10-03T00:00:00Z",
			"start_percentage": 0.4, "end_percentage": 0.5,
		}},
	} {
		rec := api.do(http.MethodPost, path, body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "%s: %s", name, rec.Body.String())
	}
	assert.Nil(t, f.position(f.file))
}

func TestPositionRoutes_FileAccess(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	other := createBook(t, f.db, "Elsewhere")
	otherFile := createFile(t, f, other)
	f.user.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.user.ID, LibraryID: &f.book.LibraryID}}
	api := newReadingAPI(t, f.db, f.user)

	body := map[string]any{"cfi": "epubcfi(/6/4!/4/2)", "percentage": 0.5}
	assert.Equal(t, http.StatusNotFound, api.do(http.MethodPost, "/files/9999/position", body).Code)
	assert.Equal(t, http.StatusNotFound, api.do(http.MethodGet, "/files/9999/position", nil).Code)
	assert.Equal(t, http.StatusForbidden, api.do(http.MethodPost, fmt.Sprintf("/files/%d/position", otherFile.ID), body).Code)
	assert.Equal(t, http.StatusForbidden, api.do(http.MethodGet, fmt.Sprintf("/files/%d/position", otherFile.ID), nil).Code)
}

func TestPositionRoutes_UndoFinishOfAnotherUsersReadThroughIsNotFound(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, result.FinishedReadThroughID)

	db := f.db
	api := newReadingAPI(t, db, createUser(t, db, "other", 2))
	rec := api.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/undo-finish", *result.FinishedReadThroughID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

// A failed File or history lookup is a 500, not a 404 or a 422.
func TestPositionRoutes_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, result.FinishedReadThroughID)
	api := newReadingAPI(t, f.db, f.user)

	_, err := f.db.Exec("DROP TABLE reading_position_updates")
	require.NoError(t, err)
	undo := api.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/undo-finish", *result.FinishedReadThroughID), nil)
	assert.Equal(t, http.StatusInternalServerError, undo.Code, undo.Body.String())

	_, err = f.db.Exec("PRAGMA foreign_keys = OFF; DROP TABLE files")
	require.NoError(t, err)
	path := fmt.Sprintf("/files/%d/position", f.file.ID)
	assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodGet, path, nil).Code)
	assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodPost, path, map[string]any{"cfi": "epubcfi(/6/4!/4/2)", "percentage": 0.5}).Code)
}
