package tags

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seedNamespacedLibrary creates a library with the given namespaces and one
// tag per name.
func seedNamespacedLibrary(t *testing.T, db *bun.DB, namespaces []string, names ...string) *models.Library {
	t.Helper()
	ctx := context.Background()
	lib := &models.Library{Name: "Doujin", CoverAspectRatio: "book", TagNamespaces: namespaces}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	searchService := search.NewService(db)
	for _, name := range names {
		tag := &models.Tag{LibraryID: lib.ID, Name: name}
		_, err := db.NewInsert().Model(tag).Exec(ctx)
		require.NoError(t, err)
		require.NoError(t, searchService.IndexTag(ctx, tag))
	}
	return lib
}

var namespacedTagNames = []string{
	"Full Color",
	"Circle: Moon Factory",
	"Circle:Apple Lab",
	"circle: Neko House",
	"Circle:",
	"Circles: Plural",
	"Star Wars: The Clone Wars",
	"Magazine: Comic Bloom",
}

func TestListTags_Namespace(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Circle", "Magazine"}, namespacedTagNames...)
	ns := "Circle"

	tags, total, err := NewService(db).ListTagsWithTotal(context.Background(), ListTagsOptions{
		LibraryID: &lib.ID,
		Namespace: &ns,
	})
	require.NoError(t, err)

	// Matched case-insensitively with or without the space, and sorted by the
	// name without its prefix. "Circle:" alone has no name after the colon.
	assert.Equal(t, []string{"Circle:Apple Lab", "Circle: Moon Factory", "circle: Neko House"}, tagNames(tags))
	assert.Equal(t, 3, total)
}

func TestListTags_ExcludeNamespaces(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Circle", "Magazine"}, namespacedTagNames...)

	tags, total, err := NewService(db).ListTagsWithTotal(context.Background(), ListTagsOptions{
		LibraryID:         &lib.ID,
		ExcludeNamespaces: []string{"Circle", "Magazine"},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"Circle:", "Circles: Plural", "Full Color", "Star Wars: The Clone Wars"}, tagNames(tags))
	assert.Equal(t, 4, total)
}

// LIKE wildcards in a namespace match only themselves.
func TestListTags_NamespaceEscapesLikeWildcards(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"50%_off"}, "50%_off: Sale", "50xyoff: Other", `50%_off\: Odd`)
	ns := "50%_off"

	tags, err := NewService(db).ListTags(context.Background(), ListTagsOptions{LibraryID: &lib.ID, Namespace: &ns})
	require.NoError(t, err)
	assert.Equal(t, []string{"50%_off: Sale"}, tagNames(tags))
}

// A multi-byte namespace splits at its character count, and only ASCII
// letters fold, as in app/utils/tagNamespaces.ts: "ÉCOLE" differs from
// "École" only in ASCII letters, while "école" differs in an accented one.
func TestListTags_NonASCIINamespace(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"École"}, "École:Zed", "École: Bar", "ÉCOLE: Shout", "école: Lower")
	ns := "École"

	tags, err := NewService(db).ListTags(context.Background(), ListTagsOptions{LibraryID: &lib.ID, Namespace: &ns})
	require.NoError(t, err)
	assert.Equal(t, []string{"École: Bar", "ÉCOLE: Shout", "École:Zed"}, tagNames(tags))
}

func TestCountByNamespace(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Magazine", "Circle", "Source"}, namespacedTagNames...)
	svc := NewService(db)
	ctx := context.Background()

	plain, counts, err := svc.CountByNamespace(ctx, ListTagsOptions{LibraryID: &lib.ID}, []string{"Magazine", "Circle", "Source"})
	require.NoError(t, err)
	assert.Equal(t, 4, plain)
	assert.Equal(t, []int{1, 3, 0}, counts)

	search := "neko"
	plain, counts, err = svc.CountByNamespace(ctx, ListTagsOptions{LibraryID: &lib.ID, Search: &search}, []string{"Magazine", "Circle", "Source"})
	require.NoError(t, err)
	assert.Equal(t, 0, plain)
	assert.Equal(t, []int{0, 1, 0}, counts)
}

func serveTags(t *testing.T, db *bun.DB, path string) *httptest.ResponseRecorder {
	t.Helper()
	return serveTagsAs(t, db, &models.User{ID: 1, LibraryAccess: []*models.UserLibraryAccess{{}}}, path)
}

func serveTagsAs(t *testing.T, db *bun.DB, user *models.User, path string) *httptest.ResponseRecorder {
	t.Helper()
	e := newTestEcho(t)
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	h := newTestHandler(db)
	g := e.Group("/tags", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetUser(c, user)
			return next(c)
		}
	})
	g.GET("", h.list)
	g.GET("/namespaces", h.listNamespaces)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) errcodes.ErrorBody {
	t.Helper()
	var resp errcodes.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Error
}

func TestListHandler_NamespaceParams(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Circle", "Magazine"}, namespacedTagNames...)
	base := "/tags?library_id=" + strconv.Itoa(lib.ID)

	listNames := func(path string) []string {
		rec := serveTags(t, db, path)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var resp ListTagsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		names := make([]string, len(resp.Items))
		for i, item := range resp.Items {
			names[i] = item.Name
		}
		return names
	}

	assert.Equal(t, []string{"Magazine: Comic Bloom"}, listNames(base+"&namespace=magazine"))
	assert.Equal(t, []string{"Circle:", "Circles: Plural", "Full Color", "Star Wars: The Clone Wars"},
		listNames(base+"&exclude_namespaces=true"))
	// Without either param the list is unchanged.
	assert.Len(t, listNames(base), len(namespacedTagNames))
}

func TestListHandler_NamespaceParamsRejected(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Circle"})

	for path, want := range map[string]struct {
		status int
		code   string
	}{
		"/tags?exclude_namespaces=true": {http.StatusUnprocessableEntity, "validation_error"},
		"/tags?library_id=" + strconv.Itoa(lib.ID) + "&exclude_namespaces=true&namespace=Circle": {http.StatusUnprocessableEntity, "validation_error"},
		"/tags?library_id=999999&exclude_namespaces=true":                                        {http.StatusNotFound, "not_found"},
	} {
		rec := serveTags(t, db, path)
		assert.Equal(t, want.status, rec.Code, path)
		assert.Equal(t, want.code, errorBody(t, rec).Code, path)
	}
}

func TestListNamespacesHandler(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Magazine", "Circle"}, namespacedTagNames...)

	rec := serveTags(t, db, "/tags/namespaces?library_id="+strconv.Itoa(lib.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `[
		{"namespace": null, "total": 4},
		{"namespace": "Magazine", "total": 1},
		{"namespace": "Circle", "total": 3}
	]`, rec.Body.String())

	rec = serveTags(t, db, "/tags/namespaces?library_id="+strconv.Itoa(lib.ID)+"&search=moon")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `[
		{"namespace": null, "total": 0},
		{"namespace": "Magazine", "total": 0},
		{"namespace": "Circle", "total": 1}
	]`, rec.Body.String())
}

// A library with no namespaces has one row, the plain tags.
func TestListNamespacesHandler_NoNamespaces(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, nil, "Circle: Foo", "Bar")

	rec := serveTags(t, db, "/tags/namespaces?library_id="+strconv.Itoa(lib.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `[{"namespace": null, "total": 2}]`, rec.Body.String())
}

func TestListNamespacesHandler_Errors(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := seedNamespacedLibrary(t, db, []string{"Circle"})

	rec := serveTags(t, db, "/tags/namespaces")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = serveTags(t, db, "/tags/namespaces?library_id=999999")
	assert.Equal(t, http.StatusNotFound, rec.Code)

	other := lib.ID + 1
	limited := &models.User{ID: 2, LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &other}}}
	rec = serveTagsAs(t, db, limited, "/tags/namespaces?library_id="+strconv.Itoa(lib.ID))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	// The list checks access before it looks the library up, so an
	// inaccessible library and a missing one answer alike.
	rec = serveTagsAs(t, db, limited, "/tags?exclude_namespaces=true&library_id="+strconv.Itoa(lib.ID))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec = serveTagsAs(t, db, limited, "/tags?exclude_namespaces=true&library_id=999999")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// A stored list that cannot be read is a server fault, not a missing
	// library.
	_, err := db.NewRaw("UPDATE libraries SET tag_namespaces = 'not json' WHERE id = ?", lib.ID).Exec(context.Background())
	require.NoError(t, err)
	rec = serveTags(t, db, "/tags/namespaces?library_id="+strconv.Itoa(lib.ID))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	rec = serveTags(t, db, "/tags?exclude_namespaces=true&library_id="+strconv.Itoa(lib.ID))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
