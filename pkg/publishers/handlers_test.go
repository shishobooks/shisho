package publishers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// setupHandlerTestDB creates an in-memory SQLite database using a named memory
// URI so that Bun's ScanAndCount (which opens a second connection for the COUNT
// query) sees the same database. Plain ":memory:" gives each connection its own
// private database, which causes "no such table" errors from ScanAndCount.
func setupHandlerTestDB(t *testing.T) *bun.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func newTestEcho(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	return e
}

func newTestHandler(db *bun.DB) *handler {
	return &handler{
		publisherService: NewService(db),
		aliasService:     aliases.NewService(db),
		searchService:    search.NewService(db),
	}
}

func seedPublisherWithFiles(t *testing.T, db *bun.DB, lib *models.Library, pubName string, fileNames []string) *models.Publisher {
	t.Helper()
	ctx := context.Background()

	publisher := &models.Publisher{LibraryID: lib.ID, Name: pubName}
	_, err := db.NewInsert().Model(publisher).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Book for " + pubName,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book for " + pubName,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for _, name := range fileNames {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      "/tmp/" + name + ".epub",
			FilesizeBytes: 1,
			PublisherID:   &publisher.ID,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}

	return publisher
}

func TestFiles_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	pub := seedPublisherWithFiles(t, db, lib, "Penguin", []string{"f1", "f2", "f3"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err := h.files(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestFiles_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	pub := seedPublisherWithFiles(t, db, lib, "HarperCollins", []string{"f1", "f2", "f3", "f4", "f5"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err := h.files(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 5, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestFiles_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	pub := seedPublisherWithFiles(t, db, lib, "Tor", []string{"f1"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pub.ID))

	err := h.files(c)
	require.NoError(t, err)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp, 2, "response should have exactly 2 keys")
	assert.Contains(t, resp, "items")
	assert.Contains(t, resp, "total")
}

func TestRetrieve_IncludesAncestorsAndDescendants(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	root := &models.Publisher{LibraryID: lib.ID, Name: "Root"}
	_, err := db.NewInsert().Model(root).Exec(ctx)
	require.NoError(t, err)

	middle := &models.Publisher{LibraryID: lib.ID, Name: "Middle", ParentID: &root.ID}
	_, err = db.NewInsert().Model(middle).Exec(ctx)
	require.NoError(t, err)

	leaf := &models.Publisher{LibraryID: lib.ID, Name: "Leaf", ParentID: &middle.ID}
	_, err = db.NewInsert().Model(leaf).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(middle.ID))

	err = h.retrieve(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Check ancestors (should be root)
	assert.Contains(t, resp, "ancestors")
	var ancestors []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	err = json.Unmarshal(resp["ancestors"], &ancestors)
	require.NoError(t, err)
	require.Len(t, ancestors, 1)
	assert.Equal(t, "Root", ancestors[0].Name)
	assert.Equal(t, root.ID, ancestors[0].ID)

	// Check descendant_ids (should be leaf)
	assert.Contains(t, resp, "descendant_ids")
	var descendantIDs []int
	err = json.Unmarshal(resp["descendant_ids"], &descendantIDs)
	require.NoError(t, err)
	require.Len(t, descendantIDs, 1)
	assert.Equal(t, leaf.ID, descendantIDs[0])
}

func TestUpdate_SetParent(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child"}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	body := fmt.Sprintf(`{"parent_id": %d}`, parent.ID)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(child.ID))

	err = h.update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the parent was set
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID)
	assert.Equal(t, parent.ID, *updated.ParentID)
}

func TestUpdate_ClearParent(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err := db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	child := &models.Publisher{LibraryID: lib.ID, Name: "Child", ParentID: &parent.ID}
	_, err = db.NewInsert().Model(child).Exec(ctx)
	require.NoError(t, err)

	e := newTestEcho(t)
	body := `{"parent_id": null}`
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(child.ID))

	err = h.update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the parent was cleared
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &child.ID})
	require.NoError(t, err)
	assert.Nil(t, updated.ParentID)
}

func TestUpdate_CycleRejected(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	pubA := &models.Publisher{LibraryID: lib.ID, Name: "A"}
	_, err := db.NewInsert().Model(pubA).Exec(ctx)
	require.NoError(t, err)

	pubB := &models.Publisher{LibraryID: lib.ID, Name: "B", ParentID: &pubA.ID}
	_, err = db.NewInsert().Model(pubB).Exec(ctx)
	require.NoError(t, err)

	// Try to set A's parent to B (would create A->B->A cycle)
	e := newTestEcho(t)
	body := fmt.Sprintf(`{"parent_id": %d}`, pubB.ID)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(pubA.ID))

	err = h.update(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		pub := &models.Publisher{LibraryID: lib.ID, Name: fmt.Sprintf("Publisher %d", i)}
		_, err := db.NewInsert().Model(pub).Exec(ctx)
		require.NoError(t, err)
	}

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.list(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Contains(t, resp, "items", "response should use 'items' key")
	assert.NotContains(t, resp, "publishers", "response should not use 'publishers' key")
	assert.Contains(t, resp, "total")

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestUpdate_RenameTriggersmerge_ParentIDStillApplied(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)
	ctx := context.Background()

	// Create the target publisher (the name we're renaming TO)
	target := &models.Publisher{LibraryID: lib.ID, Name: "Target"}
	_, err := db.NewInsert().Model(target).Exec(ctx)
	require.NoError(t, err)

	// Create the source publisher (the one being renamed/merged)
	source := &models.Publisher{LibraryID: lib.ID, Name: "Source"}
	_, err = db.NewInsert().Model(source).Exec(ctx)
	require.NoError(t, err)

	// Create a parent publisher to set as the parent of the merged result
	parent := &models.Publisher{LibraryID: lib.ID, Name: "Parent"}
	_, err = db.NewInsert().Model(parent).Exec(ctx)
	require.NoError(t, err)

	// Rename source to "Target" (triggering merge) and simultaneously set parent_id
	e := newTestEcho(t)
	body := fmt.Sprintf(`{"name": "Target", "parent_id": %d}`, parent.ID)
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(source.ID))

	err = h.update(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// The merge target should now have the parent set
	updated, err := h.publisherService.RetrievePublisher(ctx, RetrievePublisherOptions{ID: &target.ID})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentID, "parent_id should be set on merge target")
	assert.Equal(t, parent.ID, *updated.ParentID)
}
