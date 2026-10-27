package libraries

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// newLibraryPoster serves the library routes to an admin and returns a
// function that POSTs a JSON body to a path, returning the status and the
// decoded response.
func newLibraryPoster(t *testing.T, db *bun.DB) func(path, body string) (int, map[string]any) {
	t.Helper()
	admin := seedUser(context.Background(), t, db, models.RoleAdmin, true)
	e, _ := newDeleteTestServer(t, db, admin)

	return func(path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		e.ServeHTTP(rr, req)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		return rr.Code, resp
	}
}

func storedTagNamespaces(t *testing.T, db *bun.DB, id int) models.StringList {
	t.Helper()
	var lib models.Library
	require.NoError(t, db.NewSelect().Model(&lib).Where("l.id = ?", id).Scan(context.Background()))
	return lib.TagNamespaces
}

func TestCreateLibrary_TagNamespaces(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)

	status, resp := newLibraryPoster(t, db)("/libraries",
		`{"name":"Doujin","cover_aspect_ratio":"book","library_paths":["`+t.TempDir()+`"],"tag_namespaces":[" Circle ","Magazine"]}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, []any{"Circle", "Magazine"}, resp["tag_namespaces"])
	assert.Equal(t, models.StringList{"Circle", "Magazine"}, storedTagNamespaces(t, db, int(resp["id"].(float64))))
}

func TestCreateLibrary_TagNamespacesDefaultEmpty(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)

	status, resp := newLibraryPoster(t, db)("/libraries",
		`{"name":"Plain","cover_aspect_ratio":"book","library_paths":["`+t.TempDir()+`"]}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, []any{}, resp["tag_namespaces"])
}

func TestUpdateLibrary_TagNamespaces(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	seeded := seedLibraryWithContent(ctx, t, db, "Doujin")
	path := "/libraries/" + strconv.Itoa(seeded.LibraryID)
	post := newLibraryPoster(t, db)

	status, resp := post(path, `{"tag_namespaces":["Source","Circle"]}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, models.StringList{"Source", "Circle"}, storedTagNamespaces(t, db, seeded.LibraryID))

	// Omitting the field leaves the list alone.
	status, resp = post(path, `{"name":"Renamed"}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, models.StringList{"Source", "Circle"}, storedTagNamespaces(t, db, seeded.LibraryID))

	// An empty list clears it.
	status, resp = post(path, `{"tag_namespaces":[]}`)
	require.Equal(t, http.StatusOK, status, resp)
	assert.Equal(t, models.StringList{}, storedTagNamespaces(t, db, seeded.LibraryID))
	assert.Equal(t, []any{}, resp["tag_namespaces"])
}

func TestUpdateLibrary_TagNamespacesRejected(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body    string
		message string
	}{
		"blank":     {`{"tag_namespaces":["Circle","  "]}`, "Tag namespaces cannot be blank."},
		"colon":     {`{"tag_namespaces":["Circle: Foo"]}`, `Tag namespace "Circle: Foo" cannot contain a colon.`},
		"duplicate": {`{"tag_namespaces":["Circle","circle"]}`, `Tag namespace "circle" is listed more than once.`},
		"too long":  {`{"tag_namespaces":["` + strings.Repeat("a", 51) + `"]}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			seeded := seedLibraryWithContent(context.Background(), t, db, "Doujin")

			status, resp := newLibraryPoster(t, db)("/libraries/"+strconv.Itoa(seeded.LibraryID), tc.body)
			assert.Equal(t, http.StatusUnprocessableEntity, status)
			body, _ := resp["error"].(map[string]any)
			assert.Equal(t, "validation_error", body["code"])
			if tc.message != "" {
				assert.Equal(t, tc.message, body["message"])
			}
			assert.Equal(t, models.StringList{}, storedTagNamespaces(t, db, seeded.LibraryID))
		})
	}
}

// Reader pages read the namespaces from GET /user/libraries.
func TestListForUser_IncludesTagNamespaces(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	seeded := seedLibraryWithContent(ctx, t, db, "Doujin")
	_, err := db.NewUpdate().Model((*models.Library)(nil)).
		Set("tag_namespaces = ?", models.StringList{"Circle"}).
		Where("id = ?", seeded.LibraryID).Exec(ctx)
	require.NoError(t, err)

	h, c, rr := newListTestContext(t, db)
	require.NoError(t, h.listForUser(c))

	var summaries []LibrarySummary
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, []string{"Circle"}, summaries[0].TagNamespaces)
}
