package people

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/bookpreviews"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func previewTitles(previews []bookpreviews.PreviewBook) []string {
	out := make([]string, len(previews))
	for i, p := range previews {
		out[i] = p.Title
	}
	return out
}

func TestList_IncludesPreviewBooks(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	_ = seedPersonWithAuthoredBooks(t, db, lib, "Ann", []string{"One", "Two"})
	h := newTestHandler(db)

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	setBooksReader(c)

	require.NoError(t, h.list(c))

	var resp ListPeopleResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.ElementsMatch(t, []string{"One", "Two"}, previewTitles(resp.Items[0].PreviewBooks))
}

// setBooksReader signs in a user who can read books, so person rows carry
// preview covers, which come from Books Read routes.
func setBooksReader(c echo.Context) {
	auth.SetUser(c, &models.User{
		ID:            1,
		LibraryAccess: []*models.UserLibraryAccess{{}},
		Role: &models.Role{Permissions: []*models.Permission{
			{Resource: models.ResourceBooks, Operation: models.OperationRead},
		}},
	})
}

func TestList_OmitsPreviewBooksWithoutBooksRead(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	_ = seedPersonWithAuthoredBooks(t, db, lib, "Ann", []string{"One"})
	h := newTestHandler(db)

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	setAllAccessUser(c)

	require.NoError(t, h.list(c))

	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw.Items, 1)
	assert.JSONEq(t, "[]", string(raw.Items[0]["preview_books"]))
}
